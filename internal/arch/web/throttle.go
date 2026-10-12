package web

import (
	"context"
	"sync"
	"time"
)

// Throttle keeps a pause between any two searches across all users: engines ban bursts much faster than slow traffic.
type Throttle struct {
	Interval time.Duration

	mu   sync.Mutex
	last time.Time
}

// Wait blocks until Interval passed since the previous call; callers queue up behind each other.
func (t *Throttle) Wait(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if wait := time.Until(t.last.Add(t.Interval)); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
		}
	}
	t.last = time.Now()
}
