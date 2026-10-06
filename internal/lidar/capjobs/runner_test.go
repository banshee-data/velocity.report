package capjobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeStore is an in-memory queue with the behaviour the runner relies on.
type fakeStore struct {
	mu        sync.Mutex
	queue     []Job
	states    map[string]string
	progress  map[string]Progress
	errs      map[string]string
	released  int64
	claimFail error
	finishErr error
}

func newFakeStore(jobs ...Job) *fakeStore {
	s := &fakeStore{
		states:   map[string]string{},
		progress: map[string]Progress{},
		errs:     map[string]string{},
	}
	s.queue = append(s.queue, jobs...)
	for _, j := range jobs {
		s.states[j.JobID] = "queued"
	}
	return s
}

func (s *fakeStore) ClaimNextJob() (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimFail != nil {
		return Job{}, s.claimFail
	}
	if len(s.queue) == 0 {
		return Job{}, ErrNoWork
	}
	job := s.queue[0]
	s.queue = s.queue[1:]
	s.states[job.JobID] = "running"
	return job, nil
}

func (s *fakeStore) UpdateJobProgress(jobID string, current, total int64, detail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progress[jobID] = Progress{Current: current, Total: total, Detail: detail}
	return nil
}

func (s *fakeStore) FinishJob(jobID, state, jobErr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finishErr != nil {
		return s.finishErr
	}
	s.states[jobID] = state
	s.errs[jobID] = jobErr
	return nil
}

func (s *fakeStore) GetJob(jobID string) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[jobID]
	if !ok {
		return Job{}, errors.New("no such job")
	}
	return Job{JobID: jobID, State: state}, nil
}

func (s *fakeStore) ReleaseRunningJobs() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.released, nil
}

func (s *fakeStore) state(jobID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.states[jobID]
}

func (s *fakeStore) failure(jobID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.errs[jobID]
}

func (s *fakeStore) setState(jobID, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[jobID] = state
}

// waitForState polls until a job reaches the wanted state or the deadline passes.
func waitForState(t *testing.T, store *fakeStore, jobID, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if store.state(jobID) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s is %q, want %q", jobID, store.state(jobID), want)
}

func startRunner(t *testing.T, store *fakeStore, handler Handler) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{Store: store, Handler: handler, PollInterval: 5 * time.Millisecond}
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { cancel(); r.Wait() })
	return cancel
}

func TestRunnerCompletesAJob(t *testing.T) {
	store := newFakeStore(Job{JobID: "job-1", Kind: KindMotionPass, SessionID: "ses-1"})
	var seen Job
	startRunner(t, store, func(_ context.Context, job Job, report func(Progress)) error {
		seen = job
		report(Progress{Current: 5, Total: 10, Detail: "reading file 1 of 2"})
		return nil
	})

	waitForState(t, store, "job-1", StateCompleted)
	if seen.SessionID != "ses-1" {
		t.Errorf("handler saw session %q, want ses-1", seen.SessionID)
	}
	store.mu.Lock()
	p := store.progress["job-1"]
	store.mu.Unlock()
	if p.Current != 5 || p.Total != 10 || p.Detail == "" {
		t.Errorf("progress = %+v, want the reported values", p)
	}
}

func TestRunnerRecordsAFailure(t *testing.T) {
	store := newFakeStore(Job{JobID: "job-1", Kind: KindMotionPass})
	startRunner(t, store, func(context.Context, Job, func(Progress)) error {
		return errors.New("capture truncated")
	})

	waitForState(t, store, "job-1", StateFailed)
	if got := store.failure("job-1"); got != "capture truncated" {
		t.Errorf("recorded error = %q, want the handler's reason", got)
	}
}

func TestRunnerTreatsCancellationAsCancelledNotFailed(t *testing.T) {
	// A job stopped on purpose, or by a shutdown, must not read as a fault in
	// the operator's job list.
	store := newFakeStore(Job{JobID: "job-1", Kind: KindMotionPass})
	startRunner(t, store, func(context.Context, Job, func(Progress)) error {
		return context.Canceled
	})

	waitForState(t, store, "job-1", StateCancelled)
	if got := store.failure("job-1"); got != "" {
		t.Errorf("cancelled job recorded an error %q, want none", got)
	}
}

func TestRunnerStopsAJobCancelledWhileRunning(t *testing.T) {
	store := newFakeStore(Job{JobID: "job-1", Kind: KindMotionPass})
	handlerCtx := make(chan context.Context, 1)
	startRunner(t, store, func(ctx context.Context, _ Job, _ func(Progress)) error {
		handlerCtx <- ctx
		<-ctx.Done()
		return ctx.Err()
	})

	var ctx context.Context
	select {
	case ctx = <-handlerCtx:
	case <-time.After(3 * time.Second):
		t.Fatal("handler never started")
	}

	// A request handler cancels the job by recording the state; the runner has
	// to notice and stop the pass, or a five-minute read carries on regardless.
	store.setState("job-1", StateCancelled)
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the running handler was never cancelled")
	}
}

func TestRunnerDrainsSeveralJobsInOrder(t *testing.T) {
	store := newFakeStore(
		Job{JobID: "job-1", Kind: KindMotionPass},
		Job{JobID: "job-2", Kind: KindMotionPass},
		Job{JobID: "job-3", Kind: KindMotionPass},
	)
	var mu sync.Mutex
	var order []string
	startRunner(t, store, func(_ context.Context, job Job, _ func(Progress)) error {
		mu.Lock()
		order = append(order, job.JobID)
		mu.Unlock()
		return nil
	})

	for _, id := range []string{"job-1", "job-2", "job-3"} {
		waitForState(t, store, id, StateCompleted)
	}
	mu.Lock()
	defer mu.Unlock()
	for i, want := range []string{"job-1", "job-2", "job-3"} {
		if order[i] != want {
			t.Errorf("ran %v, want them in queue order", order)
			break
		}
	}
}

func TestRunnerRequeuesInterruptedWorkOnStart(t *testing.T) {
	// A crash mid-pass must not leave work that never runs again and never
	// says why.
	store := newFakeStore()
	store.released = 2
	var events []string
	var mu sync.Mutex

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &Runner{
		Store: store, PollInterval: 5 * time.Millisecond,
		Handler: func(context.Context, Job, func(Progress)) error { return nil },
		OnEvent: func(format string, args ...any) {
			mu.Lock()
			events = append(events, format)
			mu.Unlock()
		},
	}
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancel()
	r.Wait()

	mu.Lock()
	defer mu.Unlock()
	var found bool
	for _, e := range events {
		if len(e) > 0 && e[0] == 'c' && contains(e, "requeued") {
			found = true
		}
	}
	if !found {
		t.Errorf("events %v do not report the requeue", events)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestRunnerKeepsGoingAfterAClaimFailure(t *testing.T) {
	// A failing database should pause the runner, not spin it or stop it dead.
	store := newFakeStore(Job{JobID: "job-1", Kind: KindMotionPass})
	store.claimFail = errors.New("database is locked")

	startRunner(t, store, func(context.Context, Job, func(Progress)) error { return nil })
	time.Sleep(50 * time.Millisecond)

	// Clear the fault; the job must then be picked up.
	store.mu.Lock()
	store.claimFail = nil
	store.mu.Unlock()
	waitForState(t, store, "job-1", StateCompleted)
}

func TestRunnerRefusesToStartWithoutAHandler(t *testing.T) {
	r := &Runner{Store: newFakeStore()}
	if err := r.Start(context.Background()); err == nil {
		t.Fatal("Start succeeded with no handler")
	}
}

func TestRunnerStartIsIdempotent(t *testing.T) {
	store := newFakeStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &Runner{
		Store: store, PollInterval: 5 * time.Millisecond,
		Handler: func(context.Context, Job, func(Progress)) error { return nil },
	}
	if err := r.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := r.Start(ctx); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	cancel()
	r.Wait()
}

func TestRunnerStopsWhenTheContextEnds(t *testing.T) {
	store := newFakeStore()
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{
		Store: store, PollInterval: 20 * time.Millisecond,
		Handler: func(context.Context, Job, func(Progress)) error { return nil },
	}
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancel()

	done := make(chan struct{})
	go func() { r.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the runner did not stop when its context ended")
	}
}
