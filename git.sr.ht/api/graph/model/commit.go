package model

import (
	"bufio"
	"bytes"
	"context"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/object"
)

type Commit struct {
	Type    ObjectType `json:"type"`
	ID      string     `json:"id"`
	ShortID string     `json:"shortId"`
	Raw     string     `json:"raw"`

	commit *object.Commit
	repo   *RepoWrapper
}

func (Commit) IsObject() {}

func (c *Commit) Message() string {
	return c.commit.Message
}

func (c *Commit) Author() *Signature {
	return &Signature{
		Name:  c.commit.Author.Name,
		Email: c.commit.Author.Email,
		Time:  c.commit.Author.When,
	}
}

func (c *Commit) Committer() *Signature {
	return &Signature{
		Name:  c.commit.Committer.Name,
		Email: c.commit.Committer.Email,
		Time:  c.commit.Committer.When,
	}
}

func (c *Commit) DiffContext(ctx context.Context) string {
	parent, _ := c.commit.Parent(0)
	patch, _ := c.commit.PatchContext(ctx, parent)
	if patch != nil {
		return patch.String()
	}
	return ""
}

func (c *Commit) Tree() *Tree {
	obj, err := LookupObject(c.repo, c.commit.TreeHash)
	if err != nil {
		panic(err)
	}
	tree, _ := obj.(*Tree)
	return tree
}

func (c *Commit) Parents() []*Commit {
	var parents []*Commit
	for _, p := range c.commit.ParentHashes {
		obj, err := LookupObject(c.repo, p)
		if err != nil {
			panic(err)
		}
		parent, _ := obj.(*Commit)
		parents = append(parents, parent)
	}
	return parents
}

func (c *Commit) Trailers() []Trailer {
	var trailers []Trailer

	rd := bytes.NewBufferString(c.Message())
	scanner := bufio.NewScanner(rd)
	for scanner.Scan() {
		name, value, found := strings.Cut(scanner.Text(), ": ")
		if !found {
			// Reset the list
			trailers = []Trailer{}
			continue
		}
		trailers = append(trailers, Trailer{
			Name:  name,
			Value: value,
		})
	}

	return trailers
}

func CommitFromObject(repo *RepoWrapper, obj *object.Commit) *Commit {
	return &Commit{
		Type:    ObjectTypeCommit,
		ID:      obj.ID().String(),
		ShortID: obj.ID().String()[:7],

		commit: obj,
		repo:   repo,
	}
}
