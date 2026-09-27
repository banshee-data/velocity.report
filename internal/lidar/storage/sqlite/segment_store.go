package sqlite

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// JobKindSegmentClip is the capture-queue kind that cuts an annotation pack
// from a chosen segment.
const JobKindSegmentClip = "vrlog_record"

// SegmentSelection is a window an operator chose for annotation, with the
// replay case made from it.
//
// WindowJSON is the record of what the finder chose. Role, Finder, Capture and
// the rest are generated from it by the database, so they are read here and
// never written: a column cannot say one thing and the document another.
type SegmentSelection struct {
	SegmentID string
	// RunID is empty once the run has been deleted. Source keeps the identity
	// the segment ID was computed from.
	RunID          string
	Source         string
	ReplayCaseID   string
	Role           string
	Finder         string
	FinderVersion  int
	Capture        string
	WindowStartNs  int64
	WindowEndNs    int64
	ParametersJSON string
	WindowJSON     string
	CreatedAtNs    int64
}

// SegmentClipJob links a queued clip to the segment it cuts and, once it has
// finished, to the pack it made.
type SegmentClipJob struct {
	JobID     string
	SegmentID string
	State     string
	// PackDir is relative to the annotation packs directory, with forward
	// slashes. It is empty until the pack exists, as is PackDigest.
	PackDir    string
	PackDigest string
	QueuedAtNs int64
}

// SegmentStatus is what the ranking shows beside a chosen window.
type SegmentStatus struct {
	ReplayCaseID string
	JobID        string
	JobState     string
	PackDir      string
}

// SegmentStore persists chosen annotation segments and their clip jobs.
type SegmentStore struct {
	db DBClient
}

// NewSegmentStore creates a SegmentStore.
func NewSegmentStore(db DBClient) *SegmentStore {
	return &SegmentStore{db: db}
}

const segmentSelectionColumns = `segment_id, COALESCE(run_id, ''), source, replay_case_id, role, finder,
	finder_version, capture, window_start_ns, window_end_ns, parameters_json, window_json, created_at_ns`

func scanSegmentSelection(row jobRow) (SegmentSelection, error) {
	var s SegmentSelection
	err := row.Scan(&s.SegmentID, &s.RunID, &s.Source, &s.ReplayCaseID, &s.Role, &s.Finder,
		&s.FinderVersion, &s.Capture, &s.WindowStartNs, &s.WindowEndNs, &s.ParametersJSON,
		&s.WindowJSON, &s.CreatedAtNs)
	return s, err
}

// InsertSelection records a chosen window against its replay case. The
// database refuses a document that is not valid JSON, that names another
// segment or source, or whose role and finder may not go together.
func (s *SegmentStore) InsertSelection(segmentID, runID, replayCaseID string, parametersJSON, windowJSON []byte) error {
	if segmentID == "" || runID == "" || replayCaseID == "" {
		return fmt.Errorf("segment, run and replay case are required")
	}
	if _, err := s.db.Exec(`
		INSERT INTO lidar_segment_selections
			(segment_id, run_id, replay_case_id, parameters_json, window_json, created_at_ns)
		VALUES (?, ?, ?, ?, ?, ?)`,
		segmentID, runID, replayCaseID, string(parametersJSON), string(windowJSON),
		time.Now().UnixNano()); err != nil {
		return fmt.Errorf("record segment selection: %w", err)
	}
	return nil
}

// Selection returns a chosen segment, or ErrNotFound.
func (s *SegmentStore) Selection(segmentID string) (SegmentSelection, error) {
	return scanSegmentSelection(s.db.QueryRow(`
		SELECT `+segmentSelectionColumns+`
		  FROM lidar_segment_selections WHERE segment_id = ?`, segmentID))
}

// SelectionForCase returns the segment a replay case was made from, or
// ErrNotFound for a case that was authored by hand.
func (s *SegmentStore) SelectionForCase(replayCaseID string) (SegmentSelection, error) {
	return scanSegmentSelection(s.db.QueryRow(`
		SELECT `+segmentSelectionColumns+`
		  FROM lidar_segment_selections WHERE replay_case_id = ?`, replayCaseID))
}

// HasRandomHeldOut reports whether a capture already has a held-out window
// that was drawn at random. A traffic window may only be held out after one.
func (s *SegmentStore) HasRandomHeldOut(capture string) (bool, error) {
	var found bool
	err := s.db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM lidar_segment_selections
			 WHERE role = 'held_out' AND finder = 'random' AND capture = ?)`,
		capture).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("look for a random held-out window: %w", err)
	}
	return found, nil
}

// Status returns what is known about a chosen segment: its case, its latest
// clip job and that job's pack. It returns ErrNotFound for a window that has
// not been chosen.
func (s *SegmentStore) Status(segmentID string) (SegmentStatus, error) {
	var st SegmentStatus
	err := s.db.QueryRow(`
		SELECT s.replay_case_id, COALESCE(j.job_id, ''), COALESCE(j.state, ''),
		       COALESCE(c.pack_dir, '')
		  FROM lidar_segment_selections s
		  LEFT JOIN lidar_segment_clip_jobs c ON c.segment_id = s.segment_id
		  LEFT JOIN lidar_capture_jobs j ON j.job_id = c.job_id
		 WHERE s.segment_id = ?
		 ORDER BY j.queued_at_ns DESC LIMIT 1`, segmentID).Scan(
		&st.ReplayCaseID, &st.JobID, &st.JobState, &st.PackDir)
	return st, err
}

// EnqueueClip queues a clip of the segment, or returns the clip already
// queued or running for it.
//
// The job and its link to the segment are written in one transaction, so a
// worker can never claim a clip job that does not yet say what it cuts. The
// job's session is left empty: that column names a capture session, and the
// link table is what names the segment.
func (s *SegmentStore) EnqueueClip(segmentID, detail string) (CaptureJob, error) {
	if segmentID == "" {
		return CaptureJob{}, fmt.Errorf("segment is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return CaptureJob{}, fmt.Errorf("begin clip enqueue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	job := CaptureJob{
		JobID: "job-" + uuid.NewString(), Kind: JobKindSegmentClip, State: JobQueued,
		Detail: detail, QueuedAtNs: time.Now().UnixNano(),
	}
	// The transaction's first statement is a write, so it holds the write
	// lock before it looks for an active clip. Two requests for the same
	// segment cannot both find none.
	res, err := tx.Exec(`
		INSERT INTO lidar_capture_jobs (job_id, kind, state, detail, queued_at_ns)
		SELECT ?, ?, ?, ?, ?
		 WHERE NOT EXISTS (
			SELECT 1
			  FROM lidar_segment_clip_jobs c
			  JOIN lidar_capture_jobs j ON j.job_id = c.job_id
			 WHERE c.segment_id = ? AND j.state IN (?, ?))`,
		job.JobID, job.Kind, job.State, job.Detail, job.QueuedAtNs,
		segmentID, JobQueued, JobRunning)
	if err != nil {
		return CaptureJob{}, fmt.Errorf("enqueue clip: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		active, err := scanJobRow(tx.QueryRow(`
			SELECT j.job_id, j.kind, COALESCE(j.session_id, ''), COALESCE(j.root_id, ''), j.state,
			       j.progress_current, j.progress_total, j.detail, j.error, j.queued_at_ns,
			       j.started_at_ns, j.finished_at_ns
			  FROM lidar_segment_clip_jobs c
			  JOIN lidar_capture_jobs j ON j.job_id = c.job_id
			 WHERE c.segment_id = ? AND j.state IN (?, ?)
			 ORDER BY j.queued_at_ns DESC LIMIT 1`, segmentID, JobQueued, JobRunning))
		if err != nil {
			return CaptureJob{}, fmt.Errorf("read active clip: %w", err)
		}
		return active, nil
	}
	if _, err := tx.Exec(`
		INSERT INTO lidar_segment_clip_jobs (job_id, segment_id) VALUES (?, ?)`,
		job.JobID, segmentID); err != nil {
		return CaptureJob{}, fmt.Errorf("link clip to segment: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CaptureJob{}, fmt.Errorf("commit clip enqueue: %w", err)
	}
	return job, nil
}

// ClipJob returns a clip job with the selection it cuts. The replay case is
// reached through the selection, so a job cannot name one case and show
// another segment's status.
func (s *SegmentStore) ClipJob(jobID string) (SegmentClipJob, SegmentSelection, error) {
	var job SegmentClipJob
	err := s.db.QueryRow(`
		SELECT c.job_id, c.segment_id, j.state, COALESCE(c.pack_dir, ''), COALESCE(c.pack_digest, ''),
		       j.queued_at_ns
		  FROM lidar_segment_clip_jobs c
		  JOIN lidar_capture_jobs j ON j.job_id = c.job_id
		 WHERE c.job_id = ?`, jobID).Scan(
		&job.JobID, &job.SegmentID, &job.State, &job.PackDir, &job.PackDigest, &job.QueuedAtNs)
	if err != nil {
		return SegmentClipJob{}, SegmentSelection{}, err
	}
	selection, err := s.Selection(job.SegmentID)
	if err != nil {
		return SegmentClipJob{}, SegmentSelection{}, fmt.Errorf("clip job's selection: %w", err)
	}
	return job, selection, nil
}

// LinkPack records the pack a clip job made. The directory is relative to
// the annotation packs directory, so the link survives that directory being
// moved, and the digest says which pack it was.
func (s *SegmentStore) LinkPack(jobID, packDir, packDigest string) error {
	if packDir == "" || packDigest == "" {
		return fmt.Errorf("a pack is named by its directory and its digest")
	}
	res, err := s.db.Exec(`
		UPDATE lidar_segment_clip_jobs SET pack_dir = ?, pack_digest = ? WHERE job_id = ?`,
		packDir, packDigest, jobID)
	if err != nil {
		return fmt.Errorf("link pack to clip job: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// UnlinkedFinishedClips returns the clip jobs that completed without a pack
// link. There are two ways to be one: the link was cleared when the schema
// changed, or the process stopped between writing the pack and recording it.
func (s *SegmentStore) UnlinkedFinishedClips() ([]SegmentClipJob, error) {
	rows, err := s.db.Query(`
		SELECT c.job_id, c.segment_id, j.state, j.queued_at_ns
		  FROM lidar_segment_clip_jobs c
		  JOIN lidar_capture_jobs j ON j.job_id = c.job_id
		 WHERE c.pack_dir IS NULL AND j.state = ?
		 ORDER BY j.queued_at_ns`, JobCompleted)
	if err != nil {
		return nil, fmt.Errorf("list unlinked clips: %w", err)
	}
	defer rows.Close()
	jobs := []SegmentClipJob{}
	for rows.Next() {
		var job SegmentClipJob
		if err := rows.Scan(&job.JobID, &job.SegmentID, &job.State, &job.QueuedAtNs); err != nil {
			return nil, fmt.Errorf("scan unlinked clip: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
