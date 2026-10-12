package schedule

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func clock(h, m int) time.Time { return time.Date(2026, 10, 9, h, m, 0, 0, time.UTC) }

func TestWhen(t *testing.T) {
	checkIns := AtHours(9, 12, 15, 18, 21)
	if !checkIns(clock(9, 0)) || checkIns(clock(9, 5)) || checkIns(clock(10, 0)) {
		t.Fatal("AtHours")
	}
	every5 := EveryMinutes(5)
	if !every5(clock(3, 35)) || every5(clock(3, 36)) {
		t.Fatal("EveryMinutes")
	}
}

func TestTickSkipsAJobStillRunning(t *testing.T) {
	var runs atomic.Int32
	release := make(chan struct{})
	job := &Job{Name: "slow", When: EveryMinutes(1), Run: func(context.Context) error {
		runs.Add(1)
		<-release
		return nil
	}}
	s := &Scheduler{Jobs: []*Job{job}, Location: time.UTC, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	s.Tick(context.Background(), clock(9, 0))
	waitFor(t, func() bool { return runs.Load() == 1 })
	s.Tick(context.Background(), clock(9, 1))
	close(release)
	waitFor(t, func() bool { return !job.running.Load() })
	if runs.Load() != 1 {
		t.Fatalf("runs %d", runs.Load())
	}
	s.Tick(context.Background(), clock(9, 2))
	waitFor(t, func() bool { return runs.Load() == 2 })
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatal("condition not met")
}
