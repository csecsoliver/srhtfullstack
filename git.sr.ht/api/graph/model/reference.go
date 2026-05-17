package model

import (
	"github.com/go-git/go-git/v5/plumbing"
)

type Reference struct {
	Repo *Repository
	Ref  *plumbing.Reference
}

func (r *Reference) Follow() (Object, error) {
	repo := r.Repo.Repo()
	repo.Lock()
	ref, err := repo.Reference(r.Ref.Name(), true)
	repo.Unlock()
	if err != nil {
		panic(err)
	}
	obj, err := LookupObject(repo, ref.Hash())
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (r *Reference) Name() string {
	return string(r.Ref.Name())
}

func (r *Reference) Target() string {
	if r.Ref.Type() == plumbing.HashReference {
		return r.Ref.Hash().String()
	} else {
		return string(r.Ref.Target())
	}
}
