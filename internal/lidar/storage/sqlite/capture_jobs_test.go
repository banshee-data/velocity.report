package sqlite

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupJobsDB applies migrations 039 and 040 so the tests run against the
// shipped schema rather than a hand-copied one.
func setupJobsDB(t *testing.T) *sql.DB {
	t.Helper()
	db := setupCaptureDB(t)
	migration := filepath.Join("..", "..", "..", "db", "migrations",
		"000043_create_lidar_capture_jobs.up.sql")
	schema, err := os.ReadFile(migration)
	if err != nil {
		t.Fatalf("read migration 040: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply migration 040: %v", err)
	}
	return db
}

// seedSession indexes and probes files, derives a session, and returns its id.
func seedSession(t *testing.T, store *CaptureStore) string {
	t.Helper()
	rootID := seedProbedRun(t, store, "/Volumes/lidar/lidar/s2", 3, 0)
	sessions, err := store.DeriveSessions(rootID)
	if err != nil {
		t.Fatalf("DeriveSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("derived %d sessions, want 1", len(sessions))
	}
	return sessions[0].SessionID
}

func samplePeriods(base time.Time) []MotionPeriod {
	return []MotionPeriod{
		{Type: PeriodStatic, Label: "static-0", StartNs: base.UnixNano(),
			EndNs: base.Add(2 * time.Minute).UnixNano(), DurationNs: int64(2 * time.Minute),
			StartSecs: 0, EndSecs: 120},
		{Type: PeriodMotion, Label: "motion-0", StartNs: base.Add(2 * time.Minute).UnixNano(),
			EndNs: base.Add(3 * time.Minute).UnixNano(), DurationNs: int64(time.Minute),
			StartSecs: 120, EndSecs: 180},
		{Type: PeriodStatic, Label: "static-1", StartNs: base.Add(3 * time.Minute).UnixNano(),
			EndNs: base.Add(10 * time.Minute).UnixNano(), DurationNs: int64(7 * time.Minute),
			StartSecs: 180, EndSecs: 600},
	}
}

func TestCaptureStorePeriodsRoundTrip(t *testing.T) {
	store := NewCaptureStore(setupJobsDB(t))
	sessionID := seedSession(t, store)
	base := time.Date(2026, 9, 2, 13, 20, 0, 0, time.UTC)

	if err := store.ReplaceSessionPeriods(sessionID, samplePeriods(base)); err != nil {
		t.Fatalf("ReplaceSessionPeriods: %v", err)
	}
	got, err := store.ListSessionPeriods(sessionID)
	if err != nil {
		t.Fatalf("ListSessionPeriods: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("stored %d periods, want 3", len(got))
	}
	for i, want := range []string{"static-0", "motion-0", "static-1"} {
		if got[i].Label != want {
			t.Errorf("period %d label = %q, want %q", i, got[i].Label, want)
		}
		if got[i].Ordinal != i {
			t.Errorf("period %d ordinal = %d, want %d", i, got[i].Ordinal, i)
		}
	}
	if got[2].DurationNs != int64(7*time.Minute) {
		t.Errorf("last period duration = %v, want 7m", time.Duration(got[2].DurationNs))
	}
}

func TestCaptureStoreReplacePeriodsIsWholesale(t *testing.T) {
	// A motion pass measures a whole session. A partial update would leave a
	// timeline describing no single run.
	store := NewCaptureStore(setupJobsDB(t))
	sessionID := seedSession(t, store)
	base := time.Date(2026, 9, 2, 13, 20, 0, 0, time.UTC)

	if err := store.ReplaceSessionPeriods(sessionID, samplePeriods(base)); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	shorter := samplePeriods(base)[:1]
	if err := store.ReplaceSessionPeriods(sessionID, shorter); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	got, err := store.ListSessionPeriods(sessionID)
	if err != nil {
		t.Fatalf("ListSessionPeriods: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("session holds %d periods after a re-run, want 1", len(got))
	}
}

func TestCaptureStoreGetPeriod(t *testing.T) {
	store := NewCaptureStore(setupJobsDB(t))
	sessionID := seedSession(t, store)
	base := time.Date(2026, 9, 2, 13, 20, 0, 0, time.UTC)
	if err := store.ReplaceSessionPeriods(sessionID, samplePeriods(base)); err != nil {
		t.Fatalf("ReplaceSessionPeriods: %v", err)
	}
	listed, _ := store.ListSessionPeriods(sessionID)

	got, err := store.GetPeriod(listed[2].PeriodID)
	if err != nil {
		t.Fatalf("GetPeriod: %v", err)
	}
	if got.Label != "static-1" {
		t.Errorf("label = %q, want static-1", got.Label)
	}
	if _, err := store.GetPeriod("per-nope"); err == nil {
		t.Error("GetPeriod found an absent period")
	}
}

func TestCaptureStoreEnqueueAndClaim(t *testing.T) {
	store := NewCaptureStore(setupJobsDB(t))
	sessionID := seedSession(t, store)

	job, err := store.EnqueueJob(JobKindMotionPass, sessionID, "root-1", "queued for 3 captures")
	if err != nil {
		t.Fatalf("EnqueueJob: %v", err)
	}
	if job.State != JobQueued {
		t.Errorf("state = %q, want %q", job.State, JobQueued)
	}

	claimed, err := store.ClaimNextJob()
	if err != nil {
		t.Fatalf("ClaimNextJob: %v", err)
	}
	if claimed.JobID != job.JobID {
		t.Errorf("claimed %q, want %q", claimed.JobID, job.JobID)
	}
	if claimed.State != JobRunning {
		t.Errorf("claimed state = %q, want %q", claimed.State, JobRunning)
	}
	if claimed.StartedAtNs == nil {
		t.Error("claimed job has no start time")
	}

	// An empty queue is a not-found, which the runner turns into "no work".
	if _, err := store.ClaimNextJob(); !errors.Is(err, ErrNotFound) {
		t.Errorf("claiming an empty queue = %v, want ErrNotFound", err)
	}
}

func TestCaptureStoreEnqueueCoalescesActiveWork(t *testing.T) {
	// A motion pass reads every byte of every file in the session. Two of them
	// over one volume halve each other's throughput to produce the same answer.
	store := NewCaptureStore(setupJobsDB(t))
	sessionID := seedSession(t, store)

	first, err := store.EnqueueJob(JobKindMotionPass, sessionID, "root-1", "")
	if err != nil {
		t.Fatalf("first EnqueueJob: %v", err)
	}
	second, err := store.EnqueueJob(JobKindMotionPass, sessionID, "root-1", "")
	if err != nil {
		t.Fatalf("second EnqueueJob: %v", err)
	}
	if second.JobID != first.JobID {
		t.Errorf("queued a second job %q alongside %q", second.JobID, first.JobID)
	}

	// Once the first has finished, a new request queues fresh work.
	if err := store.FinishJob(first.JobID, JobCompleted, ""); err != nil {
		t.Fatalf("FinishJob: %v", err)
	}
	third, err := store.EnqueueJob(JobKindMotionPass, sessionID, "root-1", "")
	if err != nil {
		t.Fatalf("third EnqueueJob: %v", err)
	}
	if third.JobID == first.JobID {
		t.Error("a finished job was returned instead of new work")
	}
}

func TestCaptureStoreJobProgressAndCompletion(t *testing.T) {
	store := NewCaptureStore(setupJobsDB(t))
	sessionID := seedSession(t, store)
	job, _ := store.EnqueueJob(JobKindMotionPass, sessionID, "root-1", "")

	if err := store.UpdateJobProgress(job.JobID, 2, 8, "reading capture 3 of 8"); err != nil {
		t.Fatalf("UpdateJobProgress: %v", err)
	}
	got, err := store.GetJob(job.JobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.ProgressCurrent != 2 || got.ProgressTotal != 8 {
		t.Errorf("progress = %d/%d, want 2/8", got.ProgressCurrent, got.ProgressTotal)
	}
	if got.Detail != "reading capture 3 of 8" {
		t.Errorf("detail = %q, want the reported text", got.Detail)
	}
	if got.Terminal() {
		t.Error("a running job reports as terminal")
	}

	if err := store.FinishJob(job.JobID, JobFailed, "capture truncated"); err != nil {
		t.Fatalf("FinishJob: %v", err)
	}
	got, _ = store.GetJob(job.JobID)
	if !got.Terminal() {
		t.Error("a failed job does not report as terminal")
	}
	if got.Error != "capture truncated" {
		t.Errorf("error = %q, want the recorded reason", got.Error)
	}
	if got.FinishedAtNs == nil {
		t.Error("a finished job has no finish time")
	}
}

func TestCaptureStoreCancelJob(t *testing.T) {
	store := NewCaptureStore(setupJobsDB(t))
	sessionID := seedSession(t, store)
	job, _ := store.EnqueueJob(JobKindMotionPass, sessionID, "root-1", "")

	if err := store.CancelJob(job.JobID); err != nil {
		t.Fatalf("CancelJob: %v", err)
	}
	got, _ := store.GetJob(job.JobID)
	if got.State != JobCancelled {
		t.Errorf("state = %q, want %q", got.State, JobCancelled)
	}
	// Cancelling a job that has already finished changes nothing and says so.
	if err := store.CancelJob(job.JobID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cancelling a finished job = %v, want ErrNotFound", err)
	}
	if err := store.CancelJob("job-nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("cancelling an unknown job = %v, want ErrNotFound", err)
	}
}

func TestCaptureStoreReleaseRunningJobs(t *testing.T) {
	// A crash mid-pass must not leave work that never runs again.
	store := NewCaptureStore(setupJobsDB(t))
	sessionID := seedSession(t, store)
	job, _ := store.EnqueueJob(JobKindMotionPass, sessionID, "root-1", "")
	if _, err := store.ClaimNextJob(); err != nil {
		t.Fatalf("ClaimNextJob: %v", err)
	}

	n, err := store.ReleaseRunningJobs()
	if err != nil {
		t.Fatalf("ReleaseRunningJobs: %v", err)
	}
	if n != 1 {
		t.Errorf("released %d jobs, want 1", n)
	}
	got, _ := store.GetJob(job.JobID)
	if got.State != JobQueued {
		t.Errorf("state = %q, want it back in the queue", got.State)
	}
	if got.StartedAtNs != nil {
		t.Error("a requeued job kept its old start time")
	}
}

func TestCaptureStoreListJobs(t *testing.T) {
	store := NewCaptureStore(setupJobsDB(t))
	sessionID := seedSession(t, store)

	first, _ := store.EnqueueJob(JobKindMotionPass, sessionID, "root-1", "")
	_ = store.FinishJob(first.JobID, JobCompleted, "")
	time.Sleep(2 * time.Millisecond)
	second, _ := store.EnqueueJob(JobKindMotionPass, sessionID, "root-1", "")

	jobs, err := store.ListJobs(sessionID, 0)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("listed %d jobs, want 2", len(jobs))
	}
	if jobs[0].JobID != second.JobID {
		t.Error("ListJobs is not newest-first")
	}

	if limited, err := store.ListJobs(sessionID, 1); err != nil || len(limited) != 1 {
		t.Errorf("ListJobs with a limit returned %d (%v), want 1", len(limited), err)
	}
	if all, err := store.ListJobs("", 0); err != nil || len(all) != 2 {
		t.Errorf("ListJobs across sessions returned %d (%v), want 2", len(all), err)
	}
}
