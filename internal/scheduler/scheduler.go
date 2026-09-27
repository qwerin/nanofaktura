// Package scheduler runs periodic background jobs (recurring invoices,
// reminders, later bank sync, webhook retries …) inside the server process.
//
// A job is a plain function of the current time. The scheduler calls every
// registered job once at start and then on every tick (hourly by default);
// a job with Every > 0 is skipped until that much time has passed since its
// last run. Jobs run sequentially, one tick at a time; errors and panics are
// logged and never stop the scheduler. Jobs must be idempotent: a job may run
// again for the same period (restart, overlapping deploys), so it re-checks
// its state in a transaction before acting.
//
//	s := scheduler.New(nil, time.Hour, slog.Default())
//	s.Register(scheduler.Job{Name: "recurring", Run: jobs.Recurring})
//	go s.Start(ctx) // returns when ctx is cancelled
//
// Tests call the job functions directly with a chosen time, or RunOnce with
// an injected Clock.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Clock returns the current time (injectable for tests).
type Clock interface {
	Now() time.Time
}

// ClockFunc adapts a function to Clock.
type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time { return f() }

// JobFunc does one run of a job; now is the scheduler's current time.
type JobFunc func(ctx context.Context, now time.Time) error

// Job is a registered periodic job.
type Job struct {
	Name  string
	Run   JobFunc
	Every time.Duration // minimum interval between runs; 0 = every tick
}

// Scheduler runs registered jobs periodically.
type Scheduler struct {
	clock    Clock
	interval time.Duration
	log      *slog.Logger

	mu      sync.Mutex
	jobs    []Job
	lastRun map[string]time.Time
}

// New creates a scheduler ticking every interval (≤ 0 → 1 hour). clock nil =
// wall clock, log nil = slog.Default().
func New(clock Clock, interval time.Duration, log *slog.Logger) *Scheduler {
	if clock == nil {
		clock = ClockFunc(time.Now)
	}
	if interval <= 0 {
		interval = time.Hour
	}
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{clock: clock, interval: interval, log: log, lastRun: map[string]time.Time{}}
}

// Register adds jobs; they run in registration order.
func (s *Scheduler) Register(jobs ...Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, jobs...)
}

// Jobs returns the names of the registered jobs.
func (s *Scheduler) Jobs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, len(s.jobs))
	for i, j := range s.jobs {
		names[i] = j.Name
	}
	return names
}

// RunOnce runs every due job once and returns the errors by job name
// (they are also logged). It stops early when ctx is cancelled.
func (s *Scheduler) RunOnce(ctx context.Context) map[string]error {
	s.mu.Lock()
	jobs := append([]Job(nil), s.jobs...)
	s.mu.Unlock()
	errs := map[string]error{}
	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		now := s.clock.Now()
		s.mu.Lock()
		last, ran := s.lastRun[j.Name]
		s.mu.Unlock()
		if ran && j.Every > 0 && now.Sub(last) < j.Every {
			continue
		}
		start := time.Now()
		err := safeRun(ctx, j, now)
		s.mu.Lock()
		s.lastRun[j.Name] = now
		s.mu.Unlock()
		if err != nil {
			errs[j.Name] = err
			s.log.Error("scheduler job failed", "job", j.Name, "err", err)
		} else {
			s.log.Debug("scheduler job done", "job", j.Name, "took", time.Since(start))
		}
	}
	return errs
}

func safeRun(ctx context.Context, j Job, now time.Time) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return j.Run(ctx, now)
}

// Start runs the jobs immediately and then every interval until ctx is
// cancelled. It blocks; call it in a goroutine.
func (s *Scheduler) Start(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	s.RunOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RunOnce(ctx)
		}
	}
}
