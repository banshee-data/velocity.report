// Package capjobs runs background work over the capture index.
//
// The only job so far is the motion pass, and it is the reason the package
// exists: classifying a session means reading every byte of every capture in
// it, which on a field volume is several gigabytes and several minutes. That
// cannot happen inside a request.
//
// The runner holds no libpcap dependency. The pass itself is injected, so the
// queueing, progress, cancellation and failure handling are exercised by the
// default test suite rather than only where a capture happens to exist.
package capjobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Store is the persistence a Runner needs.
type Store interface {
	ClaimNextJob() (Job, error)
	UpdateJobProgress(jobID string, current, total int64, detail string) error
	FinishJob(jobID, state, jobErr string) error
	GetJob(jobID string) (Job, error)
	ReleaseRunningJobs() (int64, error)
}

// Job is one unit of queued work, in the shape the runner needs.
type Job struct {
	JobID     string
	Kind      string
	SessionID string
	RootID    string
	State     string
}

// Job states and kinds, mirroring the store's vocabulary without importing it.
const (
	KindMotionPass = "motion_pass"

	StateCompleted = "completed"
	StateFailed    = "failed"
	StateCancelled = "cancelled"
)

// ErrNoWork is what a Store returns from ClaimNextJob when the queue is empty.
// Any error the runner cannot distinguish from an empty queue is treated as
// one, so a transient read failure pauses the runner rather than spinning.
var ErrNoWork = errors.New("capjobs: no queued work")

// Progress reports how far a running job has got.
type Progress struct {
	Current int64
	Total   int64
	Detail  string
}

// Handler performs one job. It must honour ctx: a cancelled context means the
// job was cancelled or the process is stopping, and returning promptly is the
// difference between a clean shutdown and a five-minute wait.
type Handler func(ctx context.Context, job Job, report func(Progress)) error

// Runner drains the job queue.
//
// Concurrency defaults to one, and that is not timidity. A motion pass is
// bounded by reading 700 MB captures off a single external volume; running two
// at once halves the throughput of both and finishes no sooner.
type Runner struct {
	Store   Store
	Handler Handler
	// Concurrency is how many jobs run at once. Zero means one.
	Concurrency int
	// PollInterval is how long to wait after finding an empty queue. Zero
	// means one second.
	PollInterval time.Duration
	// OnEvent, when set, receives a line per job transition, for logging.
	OnEvent func(format string, args ...any)

	mu      sync.Mutex
	started bool
	wg      sync.WaitGroup
}

// Start begins draining the queue and returns immediately. It is idempotent.
//
// Jobs left running by a process that stopped are returned to the queue first:
// without that a crash mid-pass leaves work that never runs again and never
// says why.
func (r *Runner) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return nil
	}
	r.started = true
	r.mu.Unlock()

	if r.Store == nil || r.Handler == nil {
		return fmt.Errorf("capjobs: runner needs a store and a handler")
	}

	if n, err := r.Store.ReleaseRunningJobs(); err != nil {
		r.event("capture jobs: could not requeue interrupted work: %v", err)
	} else if n > 0 {
		r.event("capture jobs: requeued %d job(s) interrupted by a restart", n)
	}

	workers := r.Concurrency
	if workers <= 0 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.loop(ctx)
		}()
	}
	return nil
}

// Wait blocks until every worker has stopped, which happens when the context
// passed to Start is cancelled.
func (r *Runner) Wait() { r.wg.Wait() }

// loop claims and runs jobs until the context is cancelled.
func (r *Runner) loop(ctx context.Context) {
	interval := r.PollInterval
	if interval <= 0 {
		interval = time.Second
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		if ctx.Err() != nil {
			return
		}
		job, err := r.Store.ClaimNextJob()
		if err != nil {
			// An empty queue and a read failure are handled the same way,
			// deliberately: both mean "nothing to do right now", and spinning
			// on a failing database helps nobody.
			if !errors.Is(err, ErrNoWork) {
				r.event("capture jobs: could not claim work: %v", err)
			}
			if !sleepCtx(ctx, timer, interval) {
				return
			}
			continue
		}
		r.run(ctx, job)
	}
}

// run performs one job and records its outcome.
func (r *Runner) run(ctx context.Context, job Job) {
	r.event("capture jobs: %s %s started (session %s)", job.Kind, job.JobID, job.SessionID)

	// A job cancelled while running is stopped by cancelling its own context.
	// The store is the source of truth for that, so the runner watches it.
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopWatch := r.watchForCancellation(jobCtx, job.JobID, cancel)
	defer stopWatch()

	report := func(p Progress) {
		if err := r.Store.UpdateJobProgress(job.JobID, p.Current, p.Total, p.Detail); err != nil {
			r.event("capture jobs: progress update failed for %s: %v", job.JobID, err)
		}
	}

	err := r.Handler(jobCtx, job, report)
	stopWatch()

	switch {
	case err == nil:
		if finishErr := r.Store.FinishJob(job.JobID, StateCompleted, ""); finishErr != nil {
			r.event("capture jobs: could not mark %s complete: %v", job.JobID, finishErr)
		}
		r.event("capture jobs: %s %s completed", job.Kind, job.JobID)
	case errors.Is(err, context.Canceled):
		// Cancelled is not failed. A job stopped on purpose, or by a shutdown,
		// should not read as a fault in the operator's job list.
		if finishErr := r.Store.FinishJob(job.JobID, StateCancelled, ""); finishErr != nil {
			r.event("capture jobs: could not mark %s cancelled: %v", job.JobID, finishErr)
		}
		r.event("capture jobs: %s %s cancelled", job.Kind, job.JobID)
	default:
		if finishErr := r.Store.FinishJob(job.JobID, StateFailed, err.Error()); finishErr != nil {
			r.event("capture jobs: could not mark %s failed: %v", job.JobID, finishErr)
		}
		r.event("capture jobs: %s %s failed: %v", job.Kind, job.JobID, err)
	}
}

// watchForCancellation polls the job's state so a cancellation recorded by a
// request handler reaches the running pass. It returns a function that stops
// the watch, safe to call more than once.
func (r *Runner) watchForCancellation(ctx context.Context, jobID string, cancel context.CancelFunc) func() {
	done := make(chan struct{})
	var once sync.Once
	stop := func() { once.Do(func() { close(done) }) }

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				job, err := r.Store.GetJob(jobID)
				if err != nil {
					continue
				}
				if job.State == StateCancelled {
					cancel()
					return
				}
			}
		}
	}()
	return stop
}

// sleepCtx waits for the interval or the context, reporting whether the wait
// completed rather than the context ending.
func sleepCtx(ctx context.Context, timer *time.Timer, interval time.Duration) bool {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(interval)
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (r *Runner) event(format string, args ...any) {
	if r.OnEvent != nil {
		r.OnEvent(format, args...)
	}
}
