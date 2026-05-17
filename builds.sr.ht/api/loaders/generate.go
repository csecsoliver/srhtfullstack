//go:build generate
// +build generate

package loaders

import (
	_ "github.com/vektah/dataloaden"
)

//go:generate ./gen UsersByIDLoader int api/graph/model.User
//go:generate ./gen UsersByNameLoader string api/graph/model.User
//go:generate ./gen JobsByIDLoader int api/graph/model.Job
//go:generate ./gen JobGroupsByIDLoader int api/graph/model.JobGroup
