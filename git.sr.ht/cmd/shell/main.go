package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	gopath "path"
	"path/filepath"
	"regexp"
	"syscall"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/client"
	coreconfig "git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/crypto"
	"git.sr.ht/~sircmpwn/git.sr.ht/api/graph/model"
	"git.sr.ht/~sircmpwn/sourcehut-ssh/shell"
	_ "github.com/lib/pq"
	"github.com/vaughan0/go-ini"
)

const (
	ACCESS_NONE   = 0
	ACCESS_READ   = 1
	ACCESS_WRITE  = 2
	ACCESS_MANAGE = 4
)

func eacces(logger *log.Logger) {
	logger.Println("Access denied.")
	log.Println("Access denied.")
	log.Println()
	os.Exit(128)
}

func main() {
	// git.sr.ht-shell runs after we've authenticated the SSH session as an
	// authentic agent of a particular account, but before we've checked if
	// they have permission to perform the git operation they're trying to do.
	// Our job is to:
	//
	// 1. Find the repo they're trying to access, and handle redirects
	// 2. Check if they're allowed to do the thing they're trying to
	// 3. exec(2) into the git binary that does the rest of the work

	var (
		config ini.File
		err    error
		logger *log.Logger

		origin         string
		repos          string
		siteOwnerName  string
		siteOwnerEmail string

		cmd []string
	)

	// Initialization and set up, collect our runtime needs
	logger = shell.SetupShellLogger("git.sr.ht")
	args, err := shell.ParseShellArgs()
	if err != nil {
		logger.Fatalf("error parsing args %v: %s", os.Args, err.Error())
	}
	pusher := args.User

	config = coreconfig.LoadConfig()

	origin, ok := config.Get("git.sr.ht", "origin")
	if !ok {
		logger.Fatalf("No origin configured for git.sr.ht")
	}
	u, err := url.Parse(origin)
	if err != nil {
		logger.Fatalf("Invaild [git.sr.ht]origin: %s", err.Error())
	}
	origin = u.Host

	repos, ok = config.Get("git.sr.ht", "repos")
	if !ok {
		logger.Fatalf("No repo path configured for git.sr.ht")
	}

	sshUser, ok := config.Get("git.sr.ht", "ssh-user")
	if !ok {
		sshUser = "git"
	}

	siteOwnerName, _ = config.Get("sr.ht", "owner-name")
	siteOwnerEmail, _ = config.Get("sr.ht", "owner-email")

	// Grab the command the user is trying to execute
	cmd, err = shell.ParseSSHOrigCommand()
	if err != nil {
		logger.Fatalf("Unable to parse command: %v", err)
	}

	// Make sure it's a git command that we're expecting
	validCommands := []string{
		"git-receive-pack", "git-upload-pack", "git-upload-archive",
	}
	var valid bool
	for _, c := range validCommands {
		if len(cmd) > 0 && c == cmd[0] {
			valid = true
		}
	}

	if !valid {
		logger.Printf("Not permitting unacceptable command: %v", cmd)
		log.Printf("Hi %s! You've successfully authenticated, "+
			"but I do not provide an interactive shell. Bye!", pusher.Username)
		os.Exit(128)
	}

	// Validate the path that they're trying to access is in the repos directory
	path := gopath.Join("/", cmd[len(cmd)-1])
	path = gopath.Join(repos, path)
	absPath, err := filepath.Abs(path)
	if err != nil {
		logger.Fatalf("filepath.Abs(%s): %v", path, err)
	}
	cmd[len(cmd)-1] = absPath

	pushUuid, _ := os.LookupEnv("SRHT_PUSH")
	logger.Printf("SRHT_PUSH=%s pusher ID %d, repo path %s", pushUuid, pusher.Id, path)

	// Check what kind of access they're interested in
	needsAccess := ACCESS_READ
	if cmd[0] == "git-receive-pack" {
		needsAccess = ACCESS_WRITE
	}

	// Fetch the necessary info from SQL. This first query fetches:
	//
	// 1. Repository information, such as visibility (PUBLIC|UNLISTED|PRIVATE)
	// 2. Information about the repository owner's account
	// 3. Information about the pusher's account
	// 4. Any access control policies for that repo that apply to the pusher
	pgcs, ok := config.Get("git.sr.ht", "connection-string")
	if !ok {
		logger.Fatalf("No connection string configured for git.sr.ht: %v", err)
	}
	db, err := sql.Open("postgres", pgcs)
	if err != nil {
		logger.Fatalf("Failed to open a database connection: %v", err)
	}
	defer db.Close()

	crypto.InitCrypto(config)
	ctx := coreconfig.Context(context.Background(), config, "git.sr.ht")

	var (
		repoId         int
		repoName       string
		repoOwnerId    int
		repoOwnerName  string
		repoVisibility string
		accessGrant    *model.AccessMode
		autocreated    bool
	)
	row := db.QueryRow(`
		SELECT
			repo.id,
			repo.name,
			repo.owner_id,
			owner.username,
			repo.visibility,
			access.mode
		FROM repository repo
		JOIN "user" owner ON owner.id  = repo.owner_id
		LEFT JOIN access
			ON (access.repo_id = repo.id AND access.user_id = $1)
		WHERE
			repo.path = $2;
	`, pusher.Id, path)
	if err := row.Scan(&repoId, &repoName, &repoOwnerId, &repoOwnerName,
		&repoVisibility, &accessGrant); err != nil {

		logger.Printf("Lookup failed: %v", err)
		logger.Println("Looking up redirect")

		// If looking up the repo failed, it might have been renamed. Look for a
		// corresponding redirect, and grab all of the same information that we
		// need for the new repo while we're at it.
		row = db.QueryRow(`
			SELECT
				repo.id,
				repo.name,
				repo.owner_id,
				owner.username,
				repo.visibility,
				access.mode
			FROM repository repo
			JOIN "user" owner  ON owner.id  = repo.owner_id
			JOIN redirect      ON redirect.new_repo_id = repo.id
			LEFT JOIN access
				ON (access.repo_id = repo.id AND access.user_id = $1)
			WHERE
				redirect.path = $2;
		`, pusher.Id, path)

		if err := row.Scan(&repoId, &repoName, &repoOwnerId, &repoOwnerName,
			&repoVisibility, &accessGrant); err == sql.ErrNoRows {

			logger.Printf("Lookup failed: %v", err)

			// There wasn't a repo or a redirect by this name, so maybe the user
			// is pushing to a repo that doesn't exist. If so, autocreate it.
			//
			// If an error occurs at this step, we just log it internally and
			// tell the user we couldn't find the repo they're asking after.
			repoName = gopath.Base(path)
			repoOwnerName = gopath.Base(gopath.Dir(path))
			if repoOwnerName != "" {
				repoOwnerName = repoOwnerName[1:]
			}

			notFound := func(ctx string, errs ...error) {
				for _, err := range errs {
					logger.Printf("Error autocreating repo: %s: %v", ctx, err)
				}
				logger.Println("Repository not found.")
				log.Println("Repository not found.")
				log.Println()
				os.Exit(128)
			}

			if needsAccess == ACCESS_READ || repoOwnerName != pusher.Username {
				notFound("access", nil)
			}

			if needsAccess == ACCESS_WRITE {
				if matched, _ := regexp.MatchString(
					`^[A-Za-z0-9._-]+$`, repoName); !matched {

					log.Println("Name must match [A-Za-z0-9._-]+.")
					notFound("name policy", nil)
				}

				repoOwnerId = pusher.Id
				repoOwnerName = pusher.Username
				repoVisibility = "PRIVATE"

				query := client.GraphQLQuery{
					Query: `
						mutation CreateRepository($name: String!) {
							createRepository(name: $name, visibility: PRIVATE) {
								id
							}
						}
					`,
					Variables: map[string]interface{}{
						"name": repoName,
					},
				}
				resp := struct {
					CreateRepository struct {
						ID int `json:"id"`
					} `json:"createRepository"`
				}{}

				err := client.Do(ctx, pusher.Username, "git.sr.ht", query, &resp)
				if err != nil {
					notFound("create repository", err)
				}
				repoId = resp.CreateRepository.ID
				autocreated = true
				logger.Printf("Autocreated repo %s", path)
			}
		} else if err != nil {
			log.Println("A temporary error has occured. Please try again.")
			logger.Fatalf("Error occured looking up repo: %v", err)
		} else {
			if repoVisibility == "PRIVATE" && repoOwnerName != pusher.Username && accessGrant == nil {
				eacces(logger)
			}
			log.Printf("\033[93mNOTICE\033[0m: This repository has moved.")
			log.Printf("Please update your remote to:")
			log.Println()
			log.Printf("\t%s@%s:~%s/%s", sshUser, origin, repoOwnerName, repoName)
			log.Println()
			os.Exit(128)
		}
	}

	agrant := ""
	if accessGrant != nil {
		agrant = accessGrant.String()
	}
	logger.Printf("repo ID %d; name '%s'; owner ID %d; owner name '%s'; "+
		"visibility '%s'; pusher type '%s'; "+
		"access grant '%s'", repoId, repoName, repoOwnerId, repoOwnerName,
		repoVisibility, pusher.UserType, agrant)

	// We have everything we need, now we find out if the user is allowed
	// to do what they're trying to do. Suppressing the lint on the initial
	// assignment to clearly express the safe default in case the
	// if-else-nesting below ever contains a logic error.
	hasAccess := ACCESS_NONE //nolint:ineffassign
	if pusher.Id == repoOwnerId {
		hasAccess = ACCESS_READ | ACCESS_WRITE | ACCESS_MANAGE
	} else {
		if accessGrant == nil {
			switch repoVisibility {
			case "PUBLIC":
				fallthrough
			case "UNLISTED":
				hasAccess = ACCESS_READ
			case "PRIVATE":
				fallthrough
			default:
				hasAccess = ACCESS_NONE
			}
		} else {
			switch *accessGrant {
			case model.AccessModeRo:
				hasAccess = ACCESS_READ
			case model.AccessModeRw:
				hasAccess = ACCESS_READ | ACCESS_WRITE
			}
		}
	}

	if needsAccess&hasAccess != needsAccess {
		eacces(logger)
	}

	if pusher.UserType == auth.USER_TYPE_SUSPENDED {
		log.Println("Your account has been suspended.")
		log.Println()
		log.Printf("Please contact support: %s <%s>",
			siteOwnerName, siteOwnerEmail)
		log.Println()
		os.Exit(128)
	}

	// At this point, we know they're allowed to execute this operation. We
	// gather some of the information we've collected so far into a "push
	// context" so that steps later in the pipeline don't have to repeat our
	// lookups, then exec(2) into git.
	type RepoContext struct {
		Id           int    `json:"id"`
		Name         string `json:"name"`
		OwnerId      int    `json:"owner_id"`
		OwnerName    string `json:"owner_name"`
		Path         string `json:"path"`
		AbsolutePath string `json:"absolute_path"`
		Visibility   string `json:"visibility"`
		Autocreated  bool   `json:"autocreated"`
	}

	type UserContext struct {
		CanonicalName string `json:"canonical_name"`
		Name          string `json:"name"`
	}

	pushContext, _ := json.Marshal(struct {
		Repo RepoContext `json:"repo"`
		User UserContext `json:"user"`
	}{
		Repo: RepoContext{
			Id:           repoId,
			Name:         repoName,
			OwnerId:      repoOwnerId,
			OwnerName:    repoOwnerName,
			Path:         path,
			AbsolutePath: absPath,
			Visibility:   repoVisibility,
			Autocreated:  autocreated,
		},
		User: UserContext{
			CanonicalName: "~" + pusher.Username,
			Name:          pusher.Username,
		},
	})

	logger.Printf("Executing command: %v", cmd)
	bin, err := exec.LookPath(cmd[0])
	if err != nil {
		logger.Fatalf("exec.LookPath: %v", err)
	}
	db.Close()
	if err := syscall.Exec(bin, cmd, append(os.Environ(), fmt.Sprintf(
		"SRHT_PUSH_CTX=%s", string(pushContext)))); err != nil {

		logger.Fatalf("syscall.Exec: %v", err)
	}
}
