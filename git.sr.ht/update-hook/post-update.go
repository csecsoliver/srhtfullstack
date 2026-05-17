package updatehook

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"git.sr.ht/~sircmpwn/core-go/client"
	coreconfig "git.sr.ht/~sircmpwn/core-go/config"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	goredis "github.com/go-redis/redis/v8"
	_ "github.com/lib/pq"
)

func (h *HookContext) printAutocreateInfo(context PushContext) {
	log.Println("\n\t\033[93mNOTICE\033[0m")
	log.Printf(`
	You have pushed to a repository which did not exist. %[2]s/%[3]s
	has been created automatically. You can re-configure or delete this
	repository at the following URL:

	%[1]s/%[2]s/%[3]s/settings/info

`, h.origin, context.User.CanonicalName, context.Repo.Name)
}

type DbInfo struct {
	RepoId        int
	RepoName      string
	Visibility    string
	OwnerUsername string
}

func (h *HookContext) fetchInfoForPush(db *sql.DB, username string, repoId int, repoName string,
	repoVisibility string, newDescription *string, newVisibility *string) (DbInfo, error) {
	var dbinfo = DbInfo{
		RepoId:     repoId,
		RepoName:   repoName,
		Visibility: repoVisibility,
	}

	type RepoInput struct {
		Description *string `json:"description,omitempty"`
		Visibility  *string `json:"visibility,omitempty"`
	}

	input := RepoInput{newDescription, newVisibility}

	// TODO:
	// This should probably run through a special code path that touches
	// the repository updated timestamp and returns the info we need to
	// submit builds (e.g. repo owner username), which will save us a
	// database round-trip. As it is this does not work if the pusher is
	// not the repo owner.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = coreconfig.Context(ctx, h.config, "git.sr.ht")
	err := client.Do(ctx, username, "git.sr.ht", client.GraphQLQuery{
		Query: `
		mutation UpdateRepository($id: Int!, $input: RepoInput!) {
			updateRepository(id: $id, input: $input) { id }
		}`,
		Variables: map[string]interface{}{
			"id":    repoId,
			"input": input,
		},
	}, struct{}{})
	if err != nil {
		h.logger.Printf("Error updating repository: %v", err)
		return dbinfo, fmt.Errorf("failed to update repository: %v", err)
	}

	if newVisibility != nil {
		dbinfo.Visibility = *newVisibility
	}

	// Looking up the owner's username is required to submit build jobs on
	// their behalf for this push (their username may differ from the
	// pusher name).
	query := db.QueryRow(`
		SELECT "user".username
		FROM "user"
		JOIN repository r ON r.owner_id = "user".id
		WHERE r.id = $1
	`, repoId)

	err = query.Scan(&dbinfo.OwnerUsername)
	return dbinfo, err
}

func (h *HookContext) parseUpdatables() (*string, *string) {
	loadOptions(h.logger, h.config)
	var desc, vis *string

	if newDescription, ok := options["description"]; ok {
		desc = &newDescription
	}
	if newVisibility, ok := options["visibility"]; ok {
		newVisibility = strings.ToUpper(newVisibility)
		vis = &newVisibility
	}
	return desc, vis
}

func (h *HookContext) PostUpdate() {
	var pcontext PushContext
	refs := os.Args[1:]

	contextJson, ctxOk := os.LookupEnv("SRHT_PUSH_CTX")
	pushUuid, pushOk := os.LookupEnv("SRHT_PUSH")
	if !ctxOk || !pushOk {
		h.logger.Fatal("Missing required variables in environment, " +
			"configuration error?")
	}

	h.logger.Printf("Running post-update for push %s", pushUuid)

	if err := json.Unmarshal([]byte(contextJson), &pcontext); err != nil {
		h.logger.Fatalf("unmarshal SRHT_PUSH_CTX: %v", err)
	}

	newDescription, newVisibility := h.parseUpdatables()
	if pcontext.Repo.Autocreated && newVisibility == nil {
		h.printAutocreateInfo(pcontext)
	}

	loadOptions(h.logger, h.config)

	oids := make(map[string]interface{})
	repo, err := git.PlainOpen(pcontext.Repo.AbsolutePath)
	if err != nil {
		h.logger.Fatalf("git.PlainOpen(%q): %v", pcontext.Repo.AbsolutePath, err)
	}

	db, err := sql.Open("postgres", h.pgcs)
	if err != nil {
		h.logger.Fatalf("Failed to open a database connection: %v", err)
	}
	defer db.Close()

	dbinfo, err := h.fetchInfoForPush(db, pcontext.Repo.OwnerName, pcontext.Repo.Id, pcontext.Repo.Name, pcontext.Repo.Visibility, newDescription, newVisibility)
	if err != nil {
		h.logger.Fatalf("Failed to fetch info from database: %v", err)
	}

	redisHost, ok := h.config.Get("sr.ht", "redis-host")
	if !ok {
		redisHost = "redis://localhost:6379"
	}
	ropts, err := goredis.ParseURL(redisHost)
	if err != nil {
		h.logger.Fatalf("Failed to parse redis host: %v", err)
	}
	nbuilds := 0
	redis := goredis.NewClient(ropts)
	for _, refname := range refs {
		var newref string
		var newobj object.Object
		updateKey := fmt.Sprintf("update.%s.%s", pushUuid, refname)
		update, err := redis.Get(context.Background(), updateKey).Result()
		if update == "" || err != nil {
			h.logger.Println("redis.Get: missing key")
			continue
		} else {
			parts := strings.Split(update, ":")
			newref = parts[1]
		}

		newobj, err = repo.Object(plumbing.AnyObject, plumbing.NewHash(newref))
		if err == plumbing.ErrObjectNotFound {
			continue
		}

		if tag, ok := newobj.(*object.Tag); ok {
			newobj, err = repo.CommitObject(tag.Target)
			if err != nil {
				h.logger.Printf("new tag cannot be resovled: %v", err)
				continue
			}
		}

		commit, ok := newobj.(*object.Commit)
		if !ok {
			h.logger.Println("Skipping non-commit new ref")
			continue
		}

		if _, ok := oids[commit.Hash.String()]; ok {
			continue
		}
		oids[commit.Hash.String()] = nil

		if h.buildOrigin != "" && nbuilds < 4 {
			submitter := &GitBuildSubmitter{
				BuildOrigin: h.buildOrigin,
				Commit:      commit,
				GitOrigin:   h.origin,
				OwnerName:   dbinfo.OwnerUsername,
				PusherName:  pcontext.User.Name,
				RepoName:    dbinfo.RepoName,
				Repository:  repo,
				Visibility:  dbinfo.Visibility,
				Ref:         refname,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			ctx = coreconfig.Context(ctx, h.config, "git.sr.ht")
			results, err := h.SubmitBuild(ctx, submitter)
			if err != nil {
				h.logger.Printf("Error submitting build job: %v", err)
				log.Printf("Error submitting build job: %v", err)
				continue
			}
			if len(results) == 0 {
				continue
			} else if len(results) == 1 {
				log.Println("\033[1mBuild started:\033[0m")
			} else {
				log.Println("\033[1mBuilds started:\033[0m")
			}
			h.logger.Printf("Submitted %d builds for %s",
				len(results), refname)
			for _, result := range results {
				log.Printf("\033[94m%s\033[0m [%s]", result.Url, result.Name)
			}
			nbuilds += len(results)
		}
	}

	// Check if HEAD's dangling (i.e. the default branch doesn't exist)
	// if so, try to find a branch from this push to set as the default
	// if none were found, set the first branch in iteration order as default
	head, err := repo.Reference("HEAD", false)
	if err != nil {
		h.logger.Fatalf("repo.Reference(\"HEAD\"): %v", err)
	}

	danglingHead := false
	if _, err = repo.Reference(head.Target(), false); err != nil {
		danglingHead = true
	}

	if danglingHead {
		h.logger.Printf("HEAD dangling at %s", head.Target())

		cbk := func(ref *plumbing.Reference) error {
			if ref == nil {
				return nil
			}

			h.logger.Printf("Setting HEAD to %s", ref.Name())
			log.Printf("Default branch updated to %s", ref.Name()[len("refs/heads/"):])
			repo.Storer.SetReference(plumbing.NewSymbolicReference("HEAD", ref.Name()))
			danglingHead = false
			return storer.ErrStop
		}
		for _, refName := range refs {
			if !strings.HasPrefix(refName, "refs/heads/") {
				continue
			}

			ref, _ := repo.Reference(plumbing.ReferenceName(refName), false)
			if cbk(ref) != nil {
				break
			}
		}

		if danglingHead {
			if branches, _ := repo.Branches(); branches != nil {
				branches.ForEach(cbk)
			}
		}
	}
}
