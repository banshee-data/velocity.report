package sqlite

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Motion period types, mirroring pcapsplit's vocabulary.
const (
	PeriodMotion = "motion"
	PeriodStatic = "static"
)

// Job kinds and states.
const (
	JobKindMotionPass = "motion_pass"

	JobQueued    = "queued"
	JobRunning   = "running"
	JobCompleted = "completed"
	JobFailed    = "failed"
	JobCancelled = "cancelled"
)

// MotionPeriod is one contiguous stretch of a session sharing a
// motion/static classification.
//
// Periods are session-scoped rather than file-scoped. Classifying one capture
// file at a time restarts the background model at every boundary, which invents
// motion at each file head and hides real motion inside a file; and a static
// stretch spanning a boundary cannot be expressed at all.
type MotionPeriod struct {
	PeriodID    string  `json:"period_id"`
	SessionID   string  `json:"session_id"`
	Ordinal     int     `json:"ordinal"`
	Type        string  `json:"type"`
	Label       string  `json:"label"`
	StartNs     int64   `json:"start_ns"`
	EndNs       int64   `json:"end_ns"`
	DurationNs  int64   `json:"duration_ns"`
	StartSecs   float64 `json:"start_secs"`
	EndSecs     float64 `json:"end_secs"`
	StartFrame  *int    `json:"start_frame,omitempty"`
	EndFrame    *int    `json:"end_frame,omitempty"`
	CreatedAtNs int64   `json:"created_at_ns"`
}

// CaptureJob is one unit of background work over the capture index.
type CaptureJob struct {
	JobID           string `json:"job_id"`
	Kind            string `json:"kind"`
	SessionID       string `json:"session_id,omitempty"`
	RootID          string `json:"root_id,omitempty"`
	State           string `json:"state"`
	ProgressCurrent int64  `json:"progress_current"`
	ProgressTotal   int64  `json:"progress_total"`
	Detail          string `json:"detail,omitempty"`
	Error           string `json:"error,omitempty"`
	QueuedAtNs      int64  `json:"queued_at_ns"`
	StartedAtNs     *int64 `json:"started_at_ns,omitempty"`
	FinishedAtNs    *int64 `json:"finished_at_ns,omitempty"`
}

// Terminal reports whether the job has finished, one way or another.
func (j CaptureJob) Terminal() bool {
	return j.State == JobCompleted || j.State == JobFailed || j.State == JobCancelled
}

// periodID derives a period's identifier from its session and position, so
// re-running a motion pass replaces rows rather than accumulating them.
func periodID(sessionIDValue string, ordinal int) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00period\x00%d", sessionIDValue, ordinal))
	return "per-" + hex.EncodeToString(sum[:10])
}

// ReplaceSessionPeriods writes a session's motion timeline, replacing any
// previous one. A motion pass is a whole-session measurement, so a partial
// update would leave a timeline that describes no single run.
func (s *CaptureStore) ReplaceSessionPeriods(sessionIDValue string, periods []MotionPeriod) error {
	now := time.Now().UnixNano()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin period transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
		DELETE FROM lidar_capture_motion_periods WHERE session_id = ?`, sessionIDValue); err != nil {
		return fmt.Errorf("clear session periods: %w", err)
	}

	for i, p := range periods {
		if _, err := tx.Exec(`
			INSERT INTO lidar_capture_motion_periods
				(period_id, session_id, ordinal, period_type, label, start_ns, end_ns,
				 duration_ns, start_secs, end_secs, start_frame, end_frame, created_at_ns)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			periodID(sessionIDValue, i), sessionIDValue, i, p.Type, p.Label,
			p.StartNs, p.EndNs, p.DurationNs, p.StartSecs, p.EndSecs,
			p.StartFrame, p.EndFrame, now); err != nil {
			return fmt.Errorf("insert period %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit periods: %w", err)
	}
	return nil
}

// ListSessionPeriods returns a session's motion timeline in order.
func (s *CaptureStore) ListSessionPeriods(sessionIDValue string) ([]MotionPeriod, error) {
	rows, err := s.db.Query(`
		SELECT period_id, session_id, ordinal, period_type, label, start_ns, end_ns,
		       duration_ns, start_secs, end_secs, start_frame, end_frame, created_at_ns
		  FROM lidar_capture_motion_periods
		 WHERE session_id = ? ORDER BY ordinal`, sessionIDValue)
	if err != nil {
		return nil, fmt.Errorf("list session periods: %w", err)
	}
	defer rows.Close()

	periods := []MotionPeriod{}
	for rows.Next() {
		var p MotionPeriod
		if err := rows.Scan(&p.PeriodID, &p.SessionID, &p.Ordinal, &p.Type, &p.Label,
			&p.StartNs, &p.EndNs, &p.DurationNs, &p.StartSecs, &p.EndSecs,
			&p.StartFrame, &p.EndFrame, &p.CreatedAtNs); err != nil {
			return nil, fmt.Errorf("scan period: %w", err)
		}
		periods = append(periods, p)
	}
	return periods, rows.Err()
}

// GetPeriod returns one period by identifier.
func (s *CaptureStore) GetPeriod(periodIDValue string) (MotionPeriod, error) {
	row := s.db.QueryRow(`
		SELECT period_id, session_id, ordinal, period_type, label, start_ns, end_ns,
		       duration_ns, start_secs, end_secs, start_frame, end_frame, created_at_ns
		  FROM lidar_capture_motion_periods WHERE period_id = ?`, periodIDValue)
	var p MotionPeriod
	if err := row.Scan(&p.PeriodID, &p.SessionID, &p.Ordinal, &p.Type, &p.Label,
		&p.StartNs, &p.EndNs, &p.DurationNs, &p.StartSecs, &p.EndSecs,
		&p.StartFrame, &p.EndFrame, &p.CreatedAtNs); err != nil {
		return MotionPeriod{}, err
	}
	return p, nil
}

// EnqueueJob records a queued job and returns it.
//
// A session already carrying queued or running work of the same kind returns
// that job instead of a second one: a motion pass reads every byte of every
// file in the session, and running two over one volume halves the throughput of
// both to produce the same answer twice.
func (s *CaptureStore) EnqueueJob(kind, sessionIDValue, rootID, detail string) (CaptureJob, error) {
	if existing, err := s.activeJobFor(kind, sessionIDValue); err == nil {
		return existing, nil
	} else if err != ErrNotFound {
		return CaptureJob{}, err
	}

	job := CaptureJob{
		JobID:      "job-" + uuid.NewString(),
		Kind:       kind,
		SessionID:  sessionIDValue,
		RootID:     rootID,
		State:      JobQueued,
		Detail:     detail,
		QueuedAtNs: time.Now().UnixNano(),
	}
	if _, err := s.db.Exec(`
		INSERT INTO lidar_capture_jobs
			(job_id, kind, session_id, root_id, state, detail, queued_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		job.JobID, job.Kind, nullIfEmpty(job.SessionID), nullIfEmpty(job.RootID),
		job.State, job.Detail, job.QueuedAtNs); err != nil {
		return CaptureJob{}, fmt.Errorf("enqueue job: %w", err)
	}
	return job, nil
}

// activeJobFor returns a queued or running job of the given kind for a session.
func (s *CaptureStore) activeJobFor(kind, sessionIDValue string) (CaptureJob, error) {
	row := s.db.QueryRow(`
		SELECT job_id, kind, COALESCE(session_id, ''), COALESCE(root_id, ''), state,
		       progress_current, progress_total, detail, error, queued_at_ns,
		       started_at_ns, finished_at_ns
		  FROM lidar_capture_jobs
		 WHERE kind = ? AND session_id IS ? AND state IN (?, ?)
		 ORDER BY queued_at_ns LIMIT 1`,
		kind, nullIfEmpty(sessionIDValue), JobQueued, JobRunning)
	return scanJobRow(row)
}

// ClaimNextJob moves the oldest queued job to running and returns it. It
// reports ErrNotFound when the queue is empty.
func (s *CaptureStore) ClaimNextJob() (CaptureJob, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return CaptureJob{}, fmt.Errorf("begin claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRow(`
		SELECT job_id FROM lidar_capture_jobs
		 WHERE state = ? ORDER BY queued_at_ns LIMIT 1`, JobQueued)
	var jobID string
	if err := row.Scan(&jobID); err != nil {
		return CaptureJob{}, err
	}
	now := time.Now().UnixNano()
	if _, err := tx.Exec(`
		UPDATE lidar_capture_jobs SET state = ?, started_at_ns = ? WHERE job_id = ?`,
		JobRunning, now, jobID); err != nil {
		return CaptureJob{}, fmt.Errorf("claim job: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CaptureJob{}, fmt.Errorf("commit claim: %w", err)
	}
	return s.GetJob(jobID)
}

// UpdateJobProgress records how far a running job has got.
func (s *CaptureStore) UpdateJobProgress(jobID string, current, total int64, detail string) error {
	if _, err := s.db.Exec(`
		UPDATE lidar_capture_jobs
		   SET progress_current = ?, progress_total = ?, detail = ?
		 WHERE job_id = ?`, current, total, detail, jobID); err != nil {
		return fmt.Errorf("update job progress: %w", err)
	}
	return nil
}

// FinishJob moves a job to a terminal state.
func (s *CaptureStore) FinishJob(jobID, state, jobErr string) error {
	res, err := s.db.Exec(`
		UPDATE lidar_capture_jobs SET state = ?, error = ?, finished_at_ns = ?
		 WHERE job_id = ?`, state, jobErr, time.Now().UnixNano(), jobID)
	if err != nil {
		return fmt.Errorf("finish job: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// CancelJob cancels a job that has not finished. A running job is marked
// cancelled here and the runner notices; a job already terminal is left alone.
func (s *CaptureStore) CancelJob(jobID string) error {
	res, err := s.db.Exec(`
		UPDATE lidar_capture_jobs SET state = ?, finished_at_ns = ?
		 WHERE job_id = ? AND state IN (?, ?)`,
		JobCancelled, time.Now().UnixNano(), jobID, JobQueued, JobRunning)
	if err != nil {
		return fmt.Errorf("cancel job: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetJob returns one job by identifier.
func (s *CaptureStore) GetJob(jobID string) (CaptureJob, error) {
	row := s.db.QueryRow(`
		SELECT job_id, kind, COALESCE(session_id, ''), COALESCE(root_id, ''), state,
		       progress_current, progress_total, detail, error, queued_at_ns,
		       started_at_ns, finished_at_ns
		  FROM lidar_capture_jobs WHERE job_id = ?`, jobID)
	return scanJobRow(row)
}

// ListJobs returns jobs newest first, optionally scoped to a session.
func (s *CaptureStore) ListJobs(sessionIDValue string, limit int) ([]CaptureJob, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT job_id, kind, COALESCE(session_id, ''), COALESCE(root_id, ''), state,
		       progress_current, progress_total, detail, error, queued_at_ns,
		       started_at_ns, finished_at_ns
		  FROM lidar_capture_jobs`
	args := []any{}
	if sessionIDValue != "" {
		query += ` WHERE session_id = ?`
		args = append(args, sessionIDValue)
	}
	query += ` ORDER BY queued_at_ns DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	jobs := []CaptureJob{}
	for rows.Next() {
		var j CaptureJob
		if err := rows.Scan(&j.JobID, &j.Kind, &j.SessionID, &j.RootID, &j.State,
			&j.ProgressCurrent, &j.ProgressTotal, &j.Detail, &j.Error,
			&j.QueuedAtNs, &j.StartedAtNs, &j.FinishedAtNs); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// ReleaseRunningJobs returns jobs left running by a process that stopped to the
// queue. Without this a crash mid-pass leaves work that never runs again and
// never reports why.
func (s *CaptureStore) ReleaseRunningJobs() (int64, error) {
	res, err := s.db.Exec(`
		UPDATE lidar_capture_jobs SET state = ?, started_at_ns = NULL
		 WHERE state = ?`, JobQueued, JobRunning)
	if err != nil {
		return 0, fmt.Errorf("release running jobs: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// jobRow is the row shape both GetJob and activeJobFor scan.
type jobRow interface {
	Scan(dest ...any) error
}

func scanJobRow(row jobRow) (CaptureJob, error) {
	var j CaptureJob
	if err := row.Scan(&j.JobID, &j.Kind, &j.SessionID, &j.RootID, &j.State,
		&j.ProgressCurrent, &j.ProgressTotal, &j.Detail, &j.Error,
		&j.QueuedAtNs, &j.StartedAtNs, &j.FinishedAtNs); err != nil {
		return CaptureJob{}, err
	}
	return j, nil
}

// nullIfEmpty maps an empty string to NULL so the "IS ?" comparison in
// activeJobFor matches rows with no session.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
