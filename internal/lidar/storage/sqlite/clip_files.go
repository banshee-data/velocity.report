package sqlite

import (
	"fmt"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
)

// ClipCapture is one capture in a clip's ordered file list.
type ClipCapture struct {
	Ordinal int `json:"ordinal"`
	// CaptureFileID links to the capture index when the file is known to it.
	// It is advisory: a case stays valid when the index is rebuilt.
	CaptureFileID string `json:"capture_file_id,omitempty"`
	// PCAPFile is the path, and is what replay actually opens.
	PCAPFile string `json:"pcap_file"`
}

// SetClipFiles replaces the ordered capture association.
func (s *ClipStore) SetClipFiles(clipID string, files []ClipCapture) error {
	return s.replaceClipFiles(clipID, files, time.Now().UnixNano())
}

func (s *ClipStore) replaceClipFiles(clipID string, files []ClipCapture, updatedAt any) error {
	if clipID == "" {
		return fmt.Errorf("clip id is required")
	}
	if len(files) == 0 {
		return fmt.Errorf("a clip needs at least one capture file")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin case file transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
		DELETE FROM lidar_clip_captures WHERE clip_id = ?`, clipID); err != nil {
		return fmt.Errorf("clear case files: %w", err)
	}
	for i, f := range files {
		if strings.TrimSpace(f.PCAPFile) == "" {
			return fmt.Errorf("capture %d of the case has no path", i)
		}
		if _, err := tx.Exec(`
			INSERT INTO lidar_clip_captures
				(clip_id, ordinal, capture_file_id, pcap_file)
			VALUES (?, ?, ?, ?)`,
			clipID, i, nullIfEmpty(f.CaptureFileID), f.PCAPFile); err != nil {
			return fmt.Errorf("insert case file %d: %w", i, err)
		}
	}

	result, err := tx.Exec(`UPDATE lidar_clips SET updated_at_ns = ? WHERE clip_id = ?`, updatedAt, clipID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit case files: %w", err)
	}
	return nil
}

// ClipFiles returns the clip’s capture rows in replay order.
func (s *ClipStore) ClipFiles(clipID string) ([]ClipCapture, error) {
	rows, err := s.db.Query(`
		SELECT ordinal, COALESCE(capture_file_id, ''), pcap_file
		  FROM lidar_clip_captures
		 WHERE clip_id = ? ORDER BY ordinal`, clipID)
	if err != nil {
		return nil, fmt.Errorf("list case files: %w", err)
	}
	defer rows.Close()

	files := []ClipCapture{}
	for rows.Next() {
		var f ClipCapture
		if err := rows.Scan(&f.Ordinal, &f.CaptureFileID, &f.PCAPFile); err != nil {
			return nil, fmt.Errorf("scan case file: %w", err)
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		var exists int
		if err := s.db.QueryRow(`SELECT 1 FROM lidar_clips WHERE clip_id = ?`, clipID).Scan(&exists); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// ClipPaths is the case's captures as an ordered path list, the shape replay
// wants.
func (s *ClipStore) ClipPaths(clipID string) ([]string, error) {
	files, err := s.ClipFiles(clipID)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.PCAPFile)
	}
	return paths, nil
}

// SetClipSession links a case to the session and motion period it came from.
// Both are advisory, so a case outlives the session being re-derived.
func (s *ClipStore) SetClipSession(clipID, sessionIDValue, periodIDValue string) error {
	res, err := s.db.Exec(`
		UPDATE lidar_clips SET session_id = ?, source_period_id = ?, updated_at_ns = ?
		 WHERE clip_id = ?`,
		nullIfEmpty(sessionIDValue), nullIfEmpty(periodIDValue),
		time.Now().UnixNano(), clipID)
	if err != nil {
		return fmt.Errorf("link case to session: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// ClipSequenceExtent is a capture's packet-time extent as the case validator
// needs it. The caller supplies these because obtaining them means reading the
// files, which the storage layer does not do.
type ClipSequenceExtent struct {
	PCAPFile    string
	FirstPacket time.Time
	LastPacket  time.Time
	PacketCount uint64
}

// ValidateClipSequence checks that a case's captures form one continuous
// stream, and returns the sequence so a caller can report its joins.
//
// A case whose files do not abut is not a case: replaying it would present two
// unrelated stretches to the pipeline as if they were one recording. Rejecting
// it at authoring time is the only point at which the operator can still do
// something about it.
func ValidateClipSequence(extents []ClipSequenceExtent) (*capseq.Sequence, error) {
	if len(extents) == 0 {
		return nil, fmt.Errorf("a clip needs at least one capture file")
	}
	segments := make([]capseq.Segment, 0, len(extents))
	for _, e := range extents {
		segments = append(segments, capseq.Segment{
			Path:        e.PCAPFile,
			FirstPacket: e.FirstPacket,
			LastPacket:  e.LastPacket,
			PacketCount: e.PacketCount,
		})
	}
	seq, err := capseq.Build(segments, capseq.DefaultTolerances())
	if err != nil {
		return nil, fmt.Errorf("sequencing the case's captures: %w", err)
	}
	if !seq.Continuous() {
		broken := seq.BrokenSeams()[0]
		return nil, fmt.Errorf(
			"the case's captures do not form one continuous stream: the join %s → %s is %s (gap %v)",
			broken.Before, broken.After, broken.Grade, broken.Gap)
	}
	return seq, nil
}
