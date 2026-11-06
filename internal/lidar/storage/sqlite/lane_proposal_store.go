package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// JobKindLaneProposals is the capture-queue kind that replays a clip, uploads
// its lane fields to Gerardus and asks for the proposals derived from them
// (docs/plans/lidar-lane-field-batch-plan.md).
const JobKindLaneProposals = "lane_proposals"

// LaneProposalRun is one lane_proposals job, or one run-form call, and once
// it has completed what it sent and what Gerardus derived. The lane field is
// not kept: only the ids Gerardus gave its parts.
type LaneProposalRun struct {
	ProposalRunID     string   `json:"proposal_run_id"`
	ClipID            string   `json:"clip_id,omitempty"`
	JobID             string   `json:"job_id,omitempty"`
	RunID             string   `json:"run_id,omitempty"`
	L16Token          string   `json:"l16_token,omitempty"`
	SensorID          string   `json:"sensor_id,omitempty"`
	RecordID          string   `json:"record_id,omitempty"`
	GerardusVersion   string   `json:"gerardus_version,omitempty"`
	FieldIDs          []string `json:"field_ids,omitempty"`
	WindowStartNs     int64    `json:"window_start_ns,omitempty"`
	WindowEndNs       int64    `json:"window_end_ns,omitempty"`
	TrackCount        int64    `json:"track_count"`
	PipelineBuild     string   `json:"pipeline_build,omitempty"`
	TuningFingerprint string   `json:"tuning_fingerprint,omitempty"`
	High              int      `json:"high"`
	Medium            int      `json:"medium"`
	Adopted           int      `json:"adopted"`
	Rejected          int      `json:"rejected"`
	Outstanding       int      `json:"outstanding"`
	DeriveSecs        float64  `json:"derive_secs"`
	CreatedAtNs       int64    `json:"created_at_ns"`
	CompletedAtNs     *int64   `json:"completed_at_ns,omitempty"`

	// The job's own state, read alongside when there is a job.
	State           string `json:"state,omitempty"`
	ProgressCurrent int64  `json:"progress_current"`
	ProgressTotal   int64  `json:"progress_total"`
	Detail          string `json:"detail,omitempty"`
	Error           string `json:"error,omitempty"`
}

// Completed reports whether the run's fields reached Gerardus.
func (r LaneProposalRun) Completed() bool { return r.CompletedAtNs != nil }

// Active reports whether the run's job is still queued or running.
func (r LaneProposalRun) Active() bool { return r.State == JobQueued || r.State == JobRunning }

// LaneProposalStore reads and writes lidar_clip_lane_proposals.
type LaneProposalStore struct{ db DBClient }

// NewLaneProposalStore returns a store over db.
func NewLaneProposalStore(db DBClient) *LaneProposalStore { return &LaneProposalStore{db: db} }

const laneProposalColumns = `p.proposal_run_id, COALESCE(p.clip_id, ''), COALESCE(p.job_id, ''), COALESCE(p.run_id, ''),
	COALESCE(p.l16_token, ''), COALESCE(p.sensor_id, ''), COALESCE(p.record_id, ''), COALESCE(p.gerardus_version, ''),
	COALESCE(p.field_ids_json, ''), COALESCE(p.window_start_ns, 0), COALESCE(p.window_end_ns, 0),
	COALESCE(p.track_count, 0), COALESCE(p.pipeline_build, ''), COALESCE(p.tuning_fingerprint, ''),
	COALESCE(p.high, 0), COALESCE(p.medium, 0), COALESCE(p.adopted, 0), COALESCE(p.rejected, 0),
	COALESCE(p.outstanding, 0), COALESCE(p.derive_secs, 0), p.created_at_ns, p.completed_at_ns,
	COALESCE(j.state, ''), COALESCE(j.progress_current, 0), COALESCE(j.progress_total, 0),
	COALESCE(j.detail, ''), COALESCE(j.error, '')`

func scanLaneProposalRun(row jobRow) (LaneProposalRun, error) {
	var r LaneProposalRun
	var fields string
	var completed sql.NullInt64
	if err := row.Scan(&r.ProposalRunID, &r.ClipID, &r.JobID, &r.RunID, &r.L16Token, &r.SensorID, &r.RecordID,
		&r.GerardusVersion, &fields, &r.WindowStartNs, &r.WindowEndNs, &r.TrackCount, &r.PipelineBuild,
		&r.TuningFingerprint, &r.High, &r.Medium, &r.Adopted, &r.Rejected, &r.Outstanding, &r.DeriveSecs,
		&r.CreatedAtNs, &completed, &r.State, &r.ProgressCurrent, &r.ProgressTotal, &r.Detail, &r.Error); err != nil {
		return LaneProposalRun{}, err
	}
	if fields != "" {
		if err := json.Unmarshal([]byte(fields), &r.FieldIDs); err != nil {
			return LaneProposalRun{}, fmt.Errorf("field ids of %s: %w", r.ProposalRunID, err)
		}
	}
	if completed.Valid {
		v := completed.Int64
		r.CompletedAtNs = &v
	}
	return r, nil
}

// EnqueueLaneProposals queues a lane_proposals job for a clip, or returns the
// one already queued or running for it. created says which. recordID, when
// set, is the accepted record an operator named to place the clip, for a site
// with several; the job places the fields by it rather than choosing.
func (s *LaneProposalStore) EnqueueLaneProposals(clipID, detail, recordID string) (CaptureJob, bool, error) {
	if clipID == "" {
		return CaptureJob{}, false, fmt.Errorf("clip id is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return CaptureJob{}, false, fmt.Errorf("begin lane proposals enqueue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM lidar_clips WHERE clip_id = ?`, clipID).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return CaptureJob{}, false, ErrNotFound
		}
		return CaptureJob{}, false, err
	}
	now := time.Now().UnixNano()
	job := CaptureJob{JobID: "job-" + uuid.NewString(), Kind: JobKindLaneProposals, State: JobQueued, Detail: detail, QueuedAtNs: now}
	// The first statement writes, so the transaction holds the write lock
	// before it looks for an active job: two requests cannot both find none.
	res, err := tx.Exec(`
		INSERT INTO lidar_capture_jobs (job_id, kind, state, detail, queued_at_ns)
		SELECT ?, ?, ?, ?, ?
		 WHERE NOT EXISTS (
			SELECT 1
			  FROM lidar_clip_lane_proposals p
			  JOIN lidar_capture_jobs j ON j.job_id = p.job_id
			 WHERE p.clip_id = ? AND j.state IN (?, ?))`,
		job.JobID, job.Kind, job.State, job.Detail, job.QueuedAtNs, clipID, JobQueued, JobRunning)
	if err != nil {
		return CaptureJob{}, false, fmt.Errorf("enqueue lane proposals: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		active, err := scanJobRow(tx.QueryRow(`
			SELECT j.job_id, j.kind, COALESCE(j.session_id, ''), COALESCE(j.root_id, ''), j.state,
			       j.progress_current, j.progress_total, j.detail, j.error, j.queued_at_ns,
			       j.started_at_ns, j.finished_at_ns
			  FROM lidar_clip_lane_proposals p
			  JOIN lidar_capture_jobs j ON j.job_id = p.job_id
			 WHERE p.clip_id = ? AND j.state IN (?, ?)
			 ORDER BY j.queued_at_ns DESC LIMIT 1`, clipID, JobQueued, JobRunning))
		if err != nil {
			return CaptureJob{}, false, fmt.Errorf("read active lane proposals job: %w", err)
		}
		return active, false, nil
	}
	if _, err := tx.Exec(`
		INSERT INTO lidar_clip_lane_proposals (proposal_run_id, clip_id, job_id, record_id, created_at_ns)
		VALUES (?, ?, ?, ?, ?)`, "lpr-"+uuid.NewString(), clipID, job.JobID, nullIfEmpty(recordID), now); err != nil {
		return CaptureJob{}, false, fmt.Errorf("link lane proposals job to clip: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CaptureJob{}, false, fmt.Errorf("commit lane proposals enqueue: %w", err)
	}
	return job, true, nil
}

// ForJob returns the row a lane_proposals job completes.
func (s *LaneProposalStore) ForJob(jobID string) (LaneProposalRun, error) {
	r, err := scanLaneProposalRun(s.db.QueryRow(`
		SELECT `+laneProposalColumns+`
		  FROM lidar_clip_lane_proposals p
		  LEFT JOIN lidar_capture_jobs j ON j.job_id = p.job_id
		 WHERE p.job_id = ?`, jobID))
	if err == sql.ErrNoRows {
		return LaneProposalRun{}, ErrNotFound
	}
	return r, err
}

// Complete records what a job or run-form call sent and derived. A row with
// a job is updated in place; one without (the run form) is inserted.
func (s *LaneProposalStore) Complete(r LaneProposalRun) (LaneProposalRun, error) {
	if r.RecordID == "" || len(r.FieldIDs) == 0 {
		return LaneProposalRun{}, fmt.Errorf("a completed lane proposals run names its record and fields")
	}
	fields, err := json.Marshal(r.FieldIDs)
	if err != nil {
		return LaneProposalRun{}, err
	}
	now := time.Now().UnixNano()
	r.CompletedAtNs = &now
	args := []any{nullIfEmpty(r.RunID), nullIfEmpty(r.L16Token), nullIfEmpty(r.SensorID), r.RecordID,
		nullIfEmpty(r.GerardusVersion), string(fields), r.WindowStartNs, r.WindowEndNs, r.TrackCount,
		nullIfEmpty(r.PipelineBuild), nullIfEmpty(r.TuningFingerprint), r.High, r.Medium, r.Adopted,
		r.Rejected, r.Outstanding, r.DeriveSecs, now}
	if r.JobID != "" {
		res, err := s.db.Exec(`
			UPDATE lidar_clip_lane_proposals
			   SET run_id = ?, l16_token = ?, sensor_id = ?, record_id = ?, gerardus_version = ?,
			       field_ids_json = ?, window_start_ns = ?, window_end_ns = ?, track_count = ?,
			       pipeline_build = ?, tuning_fingerprint = ?, high = ?, medium = ?, adopted = ?,
			       rejected = ?, outstanding = ?, derive_secs = ?, completed_at_ns = ?
			 WHERE job_id = ?`, append(args, r.JobID)...)
		if err != nil {
			return LaneProposalRun{}, fmt.Errorf("record lane proposals: %w", err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return LaneProposalRun{}, ErrNotFound
		}
		return r, nil
	}
	if r.ProposalRunID == "" {
		r.ProposalRunID = "lpr-" + uuid.NewString()
	}
	if r.CreatedAtNs == 0 {
		r.CreatedAtNs = now
	}
	if _, err := s.db.Exec(`
		INSERT INTO lidar_clip_lane_proposals
			(proposal_run_id, clip_id, run_id, l16_token, sensor_id, record_id, gerardus_version,
			 field_ids_json, window_start_ns, window_end_ns, track_count, pipeline_build,
			 tuning_fingerprint, high, medium, adopted, rejected, outstanding, derive_secs,
			 completed_at_ns, created_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		append([]any{r.ProposalRunID, nullIfEmpty(r.ClipID)}, append(args, r.CreatedAtNs)...)...); err != nil {
		return LaneProposalRun{}, fmt.Errorf("record lane proposals: %w", err)
	}
	return r, nil
}

// ClipLanes is a clip's newest completed lane proposals run and its newest
// job, which may be the same row or a later one still running or failed.
type ClipLanes struct {
	Latest *LaneProposalRun `json:"latest,omitempty"`
	Job    *LaneProposalRun `json:"job,omitempty"`
}

// LatestByClip returns each clip's newest completed run and newest job.
func (s *LaneProposalStore) LatestByClip() (map[string]ClipLanes, error) {
	rows, err := s.db.Query(`
		SELECT ` + laneProposalColumns + `
		  FROM lidar_clip_lane_proposals p
		  LEFT JOIN lidar_capture_jobs j ON j.job_id = p.job_id
		 WHERE p.clip_id IS NOT NULL
		 ORDER BY p.created_at_ns DESC`)
	if err != nil {
		return nil, fmt.Errorf("list lane proposals: %w", err)
	}
	defer rows.Close()
	out := map[string]ClipLanes{}
	for rows.Next() {
		r, err := scanLaneProposalRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan lane proposals: %w", err)
		}
		cl := out[r.ClipID]
		if cl.Job == nil && r.JobID != "" {
			row := r
			cl.Job = &row
		}
		if cl.Latest == nil && r.Completed() {
			row := r
			cl.Latest = &row
		}
		out[r.ClipID] = cl
	}
	return out, rows.Err()
}
