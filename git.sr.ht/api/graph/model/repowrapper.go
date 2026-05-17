package model

import (
	"sync"

	"github.com/go-git/go-git/v5"
)

type RepoWrapper struct {
	*git.Repository
	sync.Mutex
	Obj *Repository
}

func WrapRepo(obj *Repository, repo *git.Repository) *RepoWrapper {
	return &RepoWrapper{repo, sync.Mutex{}, obj}
}
