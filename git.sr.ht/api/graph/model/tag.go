package model

import (
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Tag struct {
	Type    ObjectType `json:"type"`
	ID      string     `json:"id"`
	ShortID string     `json:"shortId"`
	Name    string     `json:"name"`

	repo *RepoWrapper
	tag  *object.Tag
}

func (Tag) IsObject() {}

func TagFromObject(repo *RepoWrapper, obj *object.Tag) Object {
	return &Tag{
		Type:    ObjectTypeTag,
		ID:      obj.Hash.String(),
		ShortID: obj.Hash.String()[:7],
		Name:    obj.Name,

		tag:  obj,
		repo: repo,
	}
}

func (t *Tag) Message() string {
	return t.tag.Message
}

func (t *Tag) Target() Object {
	obj, err := LookupObject(t.repo, t.tag.Target)
	if err != nil {
		panic(err)
	}
	return obj
}

func (t *Tag) Tagger() *Signature {
	return &Signature{
		Name:  t.tag.Tagger.Name,
		Email: t.tag.Tagger.Email,
		Time:  t.tag.Tagger.When,
	}
}
