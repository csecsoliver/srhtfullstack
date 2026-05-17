package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	gopath "path"
	"strings"
	"syscall"

	"git.sr.ht/~sircmpwn/core-go/client"
	coreconfig "git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/crypto"
)

func main() {
	cgi.Serve(http.HandlerFunc(handle))
}

func handle(rw http.ResponseWriter, req *http.Request) {
	// git.sr.ht-http-clone runs as a CGI script on $PATH_INFO=/user/repo/(HEAD|info/...|&c.) before authenticating,
	// with the goal of (a) 401ing if the repository isn't a+r,
	// (b) 302-redirecting, or (c) execing git-http-backend to handle the clone.

	repo_split := strings.SplitN(req.URL.Path, "/", 4) // "", "~user", "repo", "info/refs..."
	user, repo := repo_split[1], repo_split[2]
	path := gopath.Join(os.Getenv("GIT_PROJECT_ROOT"), user, repo)

	config := coreconfig.LoadConfig()

	origin, ok := config.Get("git.sr.ht", "origin")
	if !ok {
		rw.WriteHeader(500)
		return
	}

	crypto.InitCrypto(config)
	ctx := coreconfig.Context(context.Background(), config, "git.sr.ht")

	query := client.GraphQLQuery{
		Query: `
			query GetRepoOrRedir($path: String!) {
				repositoryByDiskPath(path: $path) {
					visibility
				}
				redirectByDiskPath(path: $path) {
					repository {
						owner { canonicalName }
						name
						visibility
					}
				}
			}
		`,
		Variables: map[string]interface{}{
			"path": path,
		},
	}
	var redir struct {
		RepositoryByDiskPath *struct {
			Visibility string `json:"visibility"`
		} `json:"repositoryByDiskPath"`
		RedirectByDiskPath *struct {
			Repository *struct {
				Owner struct {
					CanonicalName string `json:"canonicalName"`
				} `json:"owner"`
				Name       string `json:"name"`
				Visibility string `json:"visibility"`
			} `json:"repository"`
		} `json:"redirectByDiskPath"`
	}
	if err := client.Do(ctx, "", "git.sr.ht", query, &redir); err != nil {
		rw.WriteHeader(503)
		fmt.Fprintf(rw, "Error occured looking up pusher: %v\n", err)
	}

	if redir.RepositoryByDiskPath != nil {
		if redir.RepositoryByDiskPath.Visibility == "PRIVATE" {
			rw.WriteHeader(404)
			return
		}
	} else if redir.RedirectByDiskPath != nil && redir.RedirectByDiskPath.Repository != nil && redir.RedirectByDiskPath.Repository.Visibility != "PRIVATE" {
		if req.FormValue("service") == "git-upload-pack" {
			var header = rw.Header()
			header.Set("Pragma", "no-cache")
			header.Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
			header.Set("Content-Type", "application/x-git-upload-pack-advertisement")

			msg := fmt.Sprintf(`

NOTICE: This repository has moved.
Please update your remote to:

	%s/%s/%s

`, origin, redir.RedirectByDiskPath.Repository.Owner.CanonicalName, redir.RedirectByDiskPath.Repository.Name)
			fmt.Fprintf(rw, `001e# service=git-upload-pack
000000400000000000000000000000000000000000000000 HEAD%sagent=gitsrht
%04xERR %s0000`, "\000", 4+3+1+len(msg), msg)

		} else {
			rw.WriteHeader(404)
		}
		return
	} else {
		rw.WriteHeader(404)
		return
	}

	bin, err := exec.LookPath("git")
	if err != nil {
		rw.WriteHeader(500)
		return
	}
	syscall.Exec(bin, []string{"git", "http-backend"}, os.Environ())
	rw.WriteHeader(500)
}
