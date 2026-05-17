//go:build generate
// +build generate

package graph

import (
	_ "github.com/99designs/gqlgen"
)

//go:generate go run github.com/99designs/gqlgen
