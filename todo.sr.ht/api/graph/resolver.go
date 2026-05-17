package graph

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"git.sr.ht/~sircmpwn/todo.sr.ht/api/graph/model"
)

type Resolver struct{}

var (
	trackerNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

func aclBits(input model.ACLInput) uint {
	var bits uint
	if input.Browse {
		bits |= model.ACCESS_BROWSE
	}
	if input.Submit {
		bits |= model.ACCESS_SUBMIT
	}
	if input.Comment {
		bits |= model.ACCESS_COMMENT
	}
	if input.Edit {
		bits |= model.ACCESS_EDIT
	}
	if input.Triage {
		bits |= model.ACCESS_TRIAGE
	}
	return bits
}

func parseColor(input string) ([3]byte, error) {
	var color [3]byte
	if !strings.HasPrefix(input, "#") || len(input) != 7 {
		return color, fmt.Errorf("invalid color format")
	}
	if n, err := hex.Decode(color[:], []byte(input[1:])); err != nil || n != 3 {
		return color, fmt.Errorf("invalid color format")
	}
	return color, nil
}
