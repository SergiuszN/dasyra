package app

import (
	"context"
	"sync"
)

// Background runs work after the webhook was answered, on the app's context instead of the request's,
// and lets a shutdown wait for it.
type Background struct {
	ctx context.Context
	wg  sync.WaitGroup
}

func NewBackground(ctx context.Context) *Background {
	return &Background{ctx: ctx}
}

func (b *Background) Go(f func(ctx context.Context)) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		f(b.ctx)
	}()
}

// Wait blocks until all work is done.
func (b *Background) Wait() { b.wg.Wait() }
