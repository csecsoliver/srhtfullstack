package graph

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"git.sr.ht/~emersion/go-emailthreads"
	"github.com/emersion/go-message/mail"

	"git.sr.ht/~sircmpwn/lists.sr.ht/api/graph/model"
)

type Resolver struct{}

var (
	listNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

func ACLInputBits(input model.ACLInput) uint {
	var bits uint
	if input.Browse {
		bits |= model.ACCESS_BROWSE
	}
	if input.Reply {
		bits |= model.ACCESS_REPLY
	}
	if input.Post {
		bits |= model.ACCESS_POST
	}
	if input.Moderate {
		bits |= model.ACCESS_MODERATE
	}
	return bits
}

func getMailText(mr *mail.Reader) (string, error) {
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return "", fmt.Errorf("cannot find text/plain part")
		} else if err != nil {
			return "", err
		}

		if ih, ok := part.Header.(*mail.InlineHeader); ok {
			if t, _, _ := ih.ContentType(); t == "text/plain" {
				b, err := io.ReadAll(part.Body)
				return strings.ReplaceAll(string(b), "\r\n", "\n"), err
			}
		}
	}
}

func toThreadBlockList(out *[]*model.ThreadBlock, blocks []*emailthreads.Block,
	parent *emailthreads.Block, sources map[*emailthreads.Message]*model.Email,
	indexes map[*emailthreads.Block]int) {

	for _, block := range blocks {
		threadBlock := &model.ThreadBlock{
			Key:    fmt.Sprintf("%v:%v-%v", sources[block.Source].ID, block.SourceStart, block.SourceEnd),
			Body:   block.Body(),
			Source: sources[block.Source],
			SourceRange: &model.ByteRange{
				Start: block.SourceStart,
				End:   block.SourceEnd,
			},
		}

		if parent != nil {
			i := indexes[parent]
			threadBlock.Parent = &i
			if block.ParentStart >= 0 {
				threadBlock.ParentRange = &model.ByteRange{
					Start: block.ParentStart,
					End:   block.ParentEnd,
				}
			}
		}

		indexes[block] = len(*out)
		*out = append(*out, threadBlock)

		toThreadBlockList(out, block.Children, block, sources, indexes)
		for _, child := range block.Children {
			threadBlock.Children = append(threadBlock.Children, indexes[child])
		}
	}
}
