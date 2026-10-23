package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// JobKindCaptureExport is the capture-queue kind that cuts a clip's window to
// one standalone capture.
const JobKindCaptureExport = "capture_export"

// ClipExtract is one capture_export job for a clip and, once it has completed,
// the file it wrote.
type ClipExtract struct {
	JobID  string `json:"job_id"`
	ClipID string `json:"clip_id"`
	// RelPath is relative to the replay directory, with forward slashes. It
	// and SHA256 are empty until the job completes.
	RelPath          string          `json:"rel_path,omitempty"`
	SHA256           string          `json:"sha256,omitempty"`
	SizeBytes        int64           `json:"size_bytes,omitempty"`
	PacketCount      int64           `json:"packet_count,omitempty"`
	LidarPacketCount int64           `json:"lidar_packet_count,omitempty"`
	FrameCount       int64           `json:"frame_count,omitempty"`
	FirstPacketNs    int64           `json:"first_packet_ns,omitempty"`
	LastPacketNs     int64           `json:"last_packet_ns,omitempty"`
	ClosedBy         string          `json:"closed_by,omitempty"`
	RubricVersion    int             `json:"rubric_version"`
	Sidecar          json.RawMessage `json:"sidecar,omitempty"`
	CreatedAtNs      int64           `json:"created_at_ns"`
	CompletedAtNs    *int64          `json:"completed_at_ns,omitempty"`

	// The job's own state, read alongside.
	State           string `json:"state"`
	ProgressCurrent int64  `json:"progress_current"`
	ProgressTotal   int64  `json:"progress_total"`
	Detail          string `json:"detail,omitempty"`
	Error           string `json:"error,omitempty"`
}

// Completed reports whether the extract's file exists.
func (e ClipExtract) Completed() bool { return e.RelPath != "" && e.SHA256 != "" }

const clipExtractColumns = `e.job_id, e.clip_id, COALESCE(e.rel_path, ''), COALESCE(e.sha256, ''),
	COALESCE(e.size_bytes, 0), COALESCE(e.packet_count, 0), COALESCE(e.lidar_packet_count, 0),
	COALESCE(e.frame_count, 0), COALESCE(e.first_packet_ns, 0), COALESCE(e.last_packet_ns, 0),
	COALESCE(e.closed_by, ''), e.rubric_version, COALESCE(e.sidecar_json, ''), e.created_at_ns,
	e.completed_at_ns, j.state, j.progress_current, j.progress_total, j.detail, j.error`

func scanClipExtract(row jobRow) (ClipExtract, error) {
	var e ClipExtract
	var sidecar string
	var completed sql.NullInt64
	if err := row.Scan(&e.JobID, &e.ClipID, &e.RelPath, &e.SHA256, &e.SizeBytes, &e.PacketCount,
		&e.LidarPacketCount, &e.FrameCount, &e.FirstPacketNs, &e.LastPacketNs, &e.ClosedBy,
		&e.RubricVersion, &sidecar, &e.CreatedAtNs, &completed, &e.State, &e.ProgressCurrent,
		&e.ProgressTotal, &e.Detail, &e.Error); err != nil {
		return ClipExtract{}, err
	}
	if sidecar != "" {
		e.Sidecar = json.RawMessage(sidecar)
	}
	if completed.Valid {
		v := completed.Int64
		e.CompletedAtNs = &v
	}
	return e, nil
}

// EnqueueClipExtract queues a capture_export job for a clip, or returns the one
// already queued or running for it. created says which.
func (s *ClipStore) EnqueueClipExtract(clipID string, rubricVersion int, detail string) (CaptureJob, bool, error) {
	if clipID == "" {
		return CaptureJob{}, false, fmt.Errorf("clip id is required")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return CaptureJob{}, false, fmt.Errorf("begin extract enqueue: %w", err)
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
	job := CaptureJob{
		JobID: "job-" + uuid.NewString(), Kind: JobKindCaptureExport, State: JobQueued,
		Detail: detail, QueuedAtNs: now,
	}
	// The first statement writes, so the transaction holds the write lock
	// before it looks for an active export: two requests cannot both find none.
	res, err := tx.Exec(`
		INSERT INTO lidar_capture_jobs (job_id, kind, state, detail, queued_at_ns)
		SELECT ?, ?, ?, ?, ?
		 WHERE NOT EXISTS (
			SELECT 1
			  FROM lidar_clip_extracts e
			  JOIN lidar_capture_jobs j ON j.job_id = e.job_id
			 WHERE e.clip_id = ? AND j.state IN (?, ?))`,
		job.JobID, job.Kind, job.State, job.Detail, job.QueuedAtNs,
		clipID, JobQueued, JobRunning)
	if err != nil {
		return CaptureJob{}, false, fmt.Errorf("enqueue extract: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		active, err := scanJobRow(tx.QueryRow(`
			SELECT j.job_id, j.kind, COALESCE(j.session_id, ''), COALESCE(j.root_id, ''), j.state,
			       j.progress_current, j.progress_total, j.detail, j.error, j.queued_at_ns,
			       j.started_at_ns, j.finished_at_ns
			  FROM lidar_clip_extracts e
			  JOIN lidar_capture_jobs j ON j.job_id = e.job_id
			 WHERE e.clip_id = ? AND j.state IN (?, ?)
			 ORDER BY j.queued_at_ns DESC LIMIT 1`, clipID, JobQueued, JobRunning))
		if err != nil {
			return CaptureJob{}, false, fmt.Errorf("read active extract: %w", err)
		}
		return active, false, nil
	}
	if _, err := tx.Exec(`
		INSERT INTO lidar_clip_extracts (job_id, clip_id, rubric_version, created_at_ns)
		VALUES (?, ?, ?, ?)`, job.JobID, clipID, rubricVersion, now); err != nil {
		return CaptureJob{}, false, fmt.Errorf("link extract to clip: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CaptureJob{}, false, fmt.Errorf("commit extract enqueue: %w", err)
	}
	return job, true, nil
}

// ClipExtractForJob returns the extract a capture_export job writes.
func (s *ClipStore) ClipExtractForJob(jobID string) (ClipExtract, error) {
	return scanClipExtract(s.db.QueryRow(`
		SELECT `+clipExtractColumns+`
		  FROM lidar_clip_extracts e
		  JOIN lidar_capture_jobs j ON j.job_id = e.job_id
		 WHERE e.job_id = ?`, jobID))
}

// CompleteClipExtract records the file a job wrote. The job's own state is
// finished separately, by the runner.
func (s *ClipStore) CompleteClipExtract(e ClipExtract) error {
	sidecar := any(nil)
	if len(e.Sidecar) > 0 {
		sidecar = string(e.Sidecar)
	}
	res, err := s.db.Exec(`
		UPDATE lidar_clip_extracts
		   SET rel_path = ?, sha256 = ?, size_bytes = ?, packet_count = ?,
		       lidar_packet_count = ?, frame_count = ?, first_packet_ns = ?,
		       last_packet_ns = ?, closed_by = ?, rubric_version = ?, sidecar_json = ?,
		       completed_at_ns = ?
		 WHERE job_id = ?`,
		e.RelPath, e.SHA256, e.SizeBytes, e.PacketCount, e.LidarPacketCount, e.FrameCount,
		e.FirstPacketNs, e.LastPacketNs, nullIfEmpty(e.ClosedBy), e.RubricVersion, sidecar,
		time.Now().UnixNano(), e.JobID)
	if err != nil {
		return fmt.Errorf("record extract: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// ClipExtracts lists a clip's extracts, newest first.
func (s *ClipStore) ClipExtracts(clipID string) ([]ClipExtract, error) {
	rows, err := s.db.Query(`
		SELECT `+clipExtractColumns+`
		  FROM lidar_clip_extracts e
		  JOIN lidar_capture_jobs j ON j.job_id = e.job_id
		 WHERE e.clip_id = ?
		 ORDER BY e.created_at_ns DESC`, clipID)
	if err != nil {
		return nil, fmt.Errorf("list extracts: %w", err)
	}
	defer rows.Close()
	out := []ClipExtract{}
	for rows.Next() {
		e, err := scanClipExtract(rows)
		if err != nil {
			return nil, fmt.Errorf("scan extract: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LatestClipExtract returns the clip's newest completed extract, or
// ErrNotFound.
func (s *ClipStore) LatestClipExtract(clipID string) (ClipExtract, error) {
	return scanClipExtract(s.db.QueryRow(`
		SELECT `+clipExtractColumns+`
		  FROM lidar_clip_extracts e
		  JOIN lidar_capture_jobs j ON j.job_id = e.job_id
		 WHERE e.clip_id = ? AND j.state = ? AND e.rel_path IS NOT NULL
		 ORDER BY e.completed_at_ns DESC LIMIT 1`, clipID, JobCompleted))
}

// datasetIDPattern is the dataset's slug rule: lowercase ASCII words joined by
// single hyphens, as huggingface-dataset-layout.md states it.
var datasetIDPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidDatasetID reports whether id may name a published survey.
func ValidDatasetID(id string) bool {
	return len(id) <= 64 && datasetIDPattern.MatchString(id)
}

// ClipPublishing is the publishing state an operator sets on a clip. It
// replaces what is stored: an empty DatasetID or a nil azimuth clears it.
type ClipPublishing struct {
	DatasetID       string   `json:"dataset_id"`
	NorthAzimuthDeg *float64 `json:"north_azimuth_deg"`
	Storage         string   `json:"storage"`
}

// ErrDatasetIDTaken means another clip already publishes under the id.
var ErrDatasetIDTaken = fmt.Errorf("dataset id is already used by another clip")

// ErrNoExtract means a clip was asked to replay an extract it does not have.
var ErrNoExtract = fmt.Errorf("the clip has no completed extract")

// SetClipPublishing stores a clip's publishing state.
func (s *ClipStore) SetClipPublishing(clipID string, p ClipPublishing) error {
	if p.DatasetID != "" && !ValidDatasetID(p.DatasetID) {
		return fmt.Errorf("dataset_id must be lowercase words joined by hyphens, at most 64 characters")
	}
	if p.NorthAzimuthDeg != nil && (*p.NorthAzimuthDeg < 0 || *p.NorthAzimuthDeg >= 360) {
		return fmt.Errorf("north_azimuth_deg must be in [0, 360)")
	}
	switch p.Storage {
	case ClipStorageSources:
	case ClipStorageExtract:
		if _, err := s.LatestClipExtract(clipID); err == ErrNotFound {
			return ErrNoExtract
		} else if err != nil {
			return err
		}
	default:
		return fmt.Errorf("storage must be %s or %s", ClipStorageSources, ClipStorageExtract)
	}
	if p.DatasetID != "" {
		var other string
		err := s.db.QueryRow(`SELECT clip_id FROM lidar_clips WHERE dataset_id = ? AND clip_id != ?`,
			p.DatasetID, clipID).Scan(&other)
		if err == nil {
			return ErrDatasetIDTaken
		} else if err != sql.ErrNoRows {
			return err
		}
	}
	res, err := s.db.Exec(`
		UPDATE lidar_clips
		   SET dataset_id = ?, north_azimuth_deg = ?, storage = ?, updated_at_ns = ?
		 WHERE clip_id = ?`,
		nullIfEmpty(p.DatasetID), nullFloat64(p.NorthAzimuthDeg), p.Storage,
		time.Now().UnixNano(), clipID)
	if err != nil {
		return fmt.Errorf("set clip publishing: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// PublishingClips returns the clips marked for the dataset, by dataset id.
func (s *ClipStore) PublishingClips() ([]*Clip, error) {
	rows, err := s.db.Query(`SELECT clip_id FROM lidar_clips WHERE dataset_id IS NOT NULL ORDER BY dataset_id`)
	if err != nil {
		return nil, fmt.Errorf("list publishing clips: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	clips := make([]*Clip, 0, len(ids))
	for _, id := range ids {
		clip, err := s.GetClip(id)
		if err != nil {
			return nil, err
		}
		clips = append(clips, clip)
	}
	return clips, nil
}

// PeriodClip is the clip cut from a motion period, as the Captures page needs
// it to say whether the period has been located.
type PeriodClip struct {
	ClipID          string   `json:"clip_id"`
	DatasetID       string   `json:"dataset_id,omitempty"`
	Located         bool     `json:"located"`
	Lat             *float64 `json:"origin_lat,omitempty"`
	Lon             *float64 `json:"origin_lon,omitempty"`
	NorthAzimuthDeg *float64 `json:"north_azimuth_deg,omitempty"`
}

// PeriodClips returns, for each period that has one, the clip cut from it.
// When several were, the newest located clip wins, else the newest clip.
func (s *ClipStore) PeriodClips(periodIDs []string) (map[string]PeriodClip, error) {
	out := map[string]PeriodClip{}
	if len(periodIDs) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(periodIDs)), ",")
	args := make([]any, len(periodIDs))
	for i, id := range periodIDs {
		args[i] = id
	}
	rows, err := s.db.Query(`
		SELECT source_period_id, clip_id, COALESCE(dataset_id, ''), geographic_status,
		       origin_lat, origin_lon, north_azimuth_deg
		  FROM lidar_clips
		 WHERE source_period_id IN (`+placeholders+`)
		 ORDER BY (geographic_status = '`+GeoLocated+`') ASC, created_at_ns ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("period clips: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var periodID, status string
		var pc PeriodClip
		var lat, lon, az sql.NullFloat64
		if err := rows.Scan(&periodID, &pc.ClipID, &pc.DatasetID, &status, &lat, &lon, &az); err != nil {
			return nil, err
		}
		pc.Located = status == GeoLocated && lat.Valid && lon.Valid
		if pc.Located {
			la, lo := lat.Float64, lon.Float64
			pc.Lat, pc.Lon = &la, &lo
		}
		if az.Valid {
			v := az.Float64
			pc.NorthAzimuthDeg = &v
		}
		// Rows arrive unlocated first and oldest first, so the last one kept is
		// the newest located clip, or the newest clip when none is located.
		out[periodID] = pc
	}
	return out, rows.Err()
}
