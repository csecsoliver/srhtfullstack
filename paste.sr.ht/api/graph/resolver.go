package graph

import (
	"context"
	"io"
)

type Resolver struct{}

// via https://github.com/dolmen-go/contextio
// Apache 2.0
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (n int, err error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func NewContextReader(ctx context.Context, r io.Reader) io.Reader {
	return &contextReader{ctx, r}
}
