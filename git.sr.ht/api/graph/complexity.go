package graph

import (
	"git.sr.ht/~sircmpwn/core-go/model"

	"git.sr.ht/~sircmpwn/git.sr.ht/api/graph/api"
)

func cursorComplexity(c int, cursor *model.Cursor) int {
	if cursor != nil {
		return c * cursor.Count
	}
	return c
}

func ApplyComplexity(conf *api.Config) {
	conf.Complexity.Query.Repositories = func(c int, cursor *model.Cursor, filter *model.Filter) int {
		c = cursorComplexity(c, cursor)
		if filter != nil && filter.Count != nil {
			c *= *filter.Count
		}
		return c
	}
	conf.Complexity.Repository.Acls = func(c int, cursor *model.Cursor) int {
		return cursorComplexity(c, cursor)
	}
	conf.Complexity.Repository.Log = func(c int, cursor *model.Cursor, from *string) int {
		return cursorComplexity(c, cursor)
	}
	conf.Complexity.Repository.Objects = func(c int, ids []string) int {
		return c * len(ids)
	}
	conf.Complexity.Repository.References = func(c int, cursor *model.Cursor) int {
		return cursorComplexity(c, cursor)
	}
	conf.Complexity.Tree.Entries = func(c int, cursor *model.Cursor) int {
		return cursorComplexity(c, cursor)
	}
	conf.Complexity.User.Repositories = func(c int, cursor *model.Cursor, filter *model.Filter) int {
		c = cursorComplexity(c, cursor)
		if filter != nil && filter.Count != nil {
			c *= *filter.Count
		}
		return c
	}
}
