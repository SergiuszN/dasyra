// Package schedule runs the periodic jobs inside the process, no cron needed.
package schedule

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// When tells whether a job is due at a minute of the server's wall clock.
type When func(t time.Time) bool

// EveryMinutes is due when the minute divides by n.
func EveryMinutes(n int) When {
	return func(t time.Time) bool { return t.Minute()%n == 0 }
}

// AtHours is due on the full hours given.
func AtHours(hours ...int) When {
	return func(t time.Time) bool { return t.Minute() == 0 && slices.Contains(hours, t.Hour()) }
}

type Job struct {
	Name    string
	When    When
	Run     func(ctx context.Context) error
	running atomic.Bool
}

// Scheduler checks the jobs once a minute. A job still running from its previous turn is skipped,
// so a slow digest never piles up.
type Scheduler struct {
	Jobs     []*Job
	Location *time.Location
	Log      *slog.Logger
	wg       sync.WaitGroup
}

// Run ticks until stop is done; the jobs run on work, so a shutdown lets them finish.
func (s *Scheduler) Run(stop, work context.Context) {
	for {
		now := time.Now().In(s.Location)
		next := now.Truncate(time.Minute).Add(time.Minute)
		select {
		case <-stop.Done():
			return
		case <-time.After(next.Sub(now)):
			s.Tick(work, next)
		}
	}
}

// Tick starts the jobs due at t.
func (s *Scheduler) Tick(ctx context.Context, t time.Time) {
	for _, job := range s.Jobs {
		if job.When(t) && job.running.CompareAndSwap(false, true) {
			s.wg.Add(1)
			go s.run(ctx, job)
		}
	}
}

// Wait blocks until the running jobs are done, for a graceful shutdown.
func (s *Scheduler) Wait() { s.wg.Wait() }

func (s *Scheduler) run(ctx context.Context, job *Job) {
	defer s.wg.Done()
	defer job.running.Store(false)
	defer func() {
		if r := recover(); r != nil {
			s.Log.Error("Job panicked", "job", job.Name, "panic", r)
		}
	}()
	if err := job.Run(ctx); err != nil {
		s.Log.Error("Job failed", "job", job.Name, "error", err)
	}
}
