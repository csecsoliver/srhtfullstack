package updatehook

import (
	"log"

	coreconfig "git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/crypto"
	"github.com/vaughan0/go-ini"
)

// HookContext contains global state that all hooks need access to.
type HookContext struct {
	logger      *log.Logger
	config      ini.File
	origin      string
	buildOrigin string
	pgcs        string
}

func NewContext(logger *log.Logger) *HookContext {
	config := coreconfig.LoadConfig()
	crypto.InitCrypto(config)

	var ok bool
	origin, ok := config.Get("git.sr.ht", "origin")
	if !ok {
		logger.Fatalf("No origin configured for git.sr.ht")
	}
	pgcs, ok := config.Get("git.sr.ht", "connection-string")
	if !ok {
		logger.Fatalf("No connection string configured for git.sr.ht")
	}
	buildOrigin, _ := config.Get("builds.sr.ht", "origin") // Optional

	return &HookContext{
		logger:      logger,
		config:      config,
		origin:      origin,
		buildOrigin: buildOrigin,
		pgcs:        pgcs,
	}
}

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

type PushContext struct {
	Repo RepoContext `json:"repo"`
	User UserContext `json:"user"`
}
