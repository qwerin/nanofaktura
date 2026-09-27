package scheduler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunOnce(t *testing.T) {
	now := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	s := New(ClockFunc(func() time.Time { return now }), time.Hour, quiet())
	var got []time.Time
	var hourly, daily int
	s.Register(
		Job{Name: "a", Run: func(_ context.Context, n time.Time) error { got = append(got, n); hourly++; return nil }},
		Job{Name: "b", Every: 24 * time.Hour, Run: func(context.Context, time.Time) error { daily++; return nil }},
		Job{Name: "fail", Run: func(context.Context, time.Time) error { return errors.New("boom") }},
		Job{Name: "panic", Run: func(context.Context, time.Time) error { panic("oops") }},
	)
	if names := s.Jobs(); len(names) != 4 || names[0] != "a" {
		t.Fatalf("jobs %v", names)
	}
	errs := s.RunOnce(context.Background())
	if len(errs) != 2 || errs["fail"] == nil || errs["panic"] == nil {
		t.Fatalf("errs %v", errs)
	}
	now = now.Add(time.Hour)
	s.RunOnce(context.Background())
	if hourly != 2 || daily != 1 {
		t.Fatalf("hourly %d daily %d", hourly, daily)
	}
	if !got[1].Equal(now) {
		t.Fatalf("job got time %v, want %v", got[1], now)
	}
	now = now.Add(23 * time.Hour)
	s.RunOnce(context.Background())
	if daily != 2 {
		t.Fatalf("daily %d", daily)
	}
}

func TestStartStopsOnCancel(t *testing.T) {
	s := New(nil, 10*time.Millisecond, quiet())
	var n atomic.Int32
	s.Register(Job{Name: "count", Run: func(context.Context, time.Time) error { n.Add(1); return nil }})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Start(ctx); close(done) }()
	deadline := time.After(2 * time.Second)
	for n.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("job did not run on start and tick")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestRunOnceCancelled(t *testing.T) {
	s := New(nil, 0, quiet())
	ran := false
	s.Register(Job{Name: "x", Run: func(context.Context, time.Time) error { ran = true; return nil }})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.RunOnce(ctx)
	if ran {
		t.Fatal("job ran on a cancelled context")
	}
}
