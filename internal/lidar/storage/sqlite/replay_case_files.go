package sqlite

import (
	"fmt"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
)

// ReplayCaseFile is one capture in a replay case's ordered file list.
type ReplayCaseFile struct {
	Ordinal int `json:"ordinal"`
	// CaptureFileID links to the capture index when the file is known to it.
	// It is advisory: a case stays valid when the index is rebuilt.
	CaptureFileID string `json:"capture_file_id,omitempty"`
	// PCAPFile is the path, and is what replay actually opens.
	PCAPFile string `json:"pcap_file"`
}

// SetCaseFiles replaces a replay case's file list.
//
// pcap_file on the case is kept in step with ordinal 0. It is the read-only
// projection existing clients still read, and keeping it truthful is cheaper
// than auditing every one of them; it goes in v0.6.1.
func (s *ReplayCaseStore) SetCaseFiles(replayCaseID string, files []ReplayCaseFile) error {
	if replayCaseID == "" {
		return fmt.Errorf("replay case id is required")
	}
	if len(files) == 0 {
		return fmt.Errorf("a replay case needs at least one capture file")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin case file transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
		DELETE FROM lidar_replay_case_files WHERE replay_case_id = ?`, replayCaseID); err != nil {
		return fmt.Errorf("clear case files: %w", err)
	}
	for i, f := range files {
		if strings.TrimSpace(f.PCAPFile) == "" {
			return fmt.Errorf("capture %d of the case has no path", i)
		}
		if _, err := tx.Exec(`
			INSERT INTO lidar_replay_case_files
				(replay_case_id, ordinal, capture_file_id, pcap_file)
			VALUES (?, ?, ?, ?)`,
			replayCaseID, i, nullIfEmpty(f.CaptureFileID), f.PCAPFile); err != nil {
			return fmt.Errorf("insert case file %d: %w", i, err)
		}
	}
	if _, err := tx.Exec(`
		UPDATE lidar_replay_cases SET pcap_file = ?, updated_at_ns = ?
		 WHERE replay_case_id = ?`,
		files[0].PCAPFile, time.Now().UnixNano(), replayCaseID); err != nil {
		return fmt.Errorf("update legacy pcap_file projection: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit case files: %w", err)
	}
	return nil
}

// CaseFiles returns a replay case's captures in order.
//
// A case predating the file list, or one written by a client that only knows
// pcap_file, still answers: the migration backfilled a single row per case, and
// this falls back to pcap_file if even that is absent.
func (s *ReplayCaseStore) CaseFiles(replayCaseID string) ([]ReplayCaseFile, error) {
	rows, err := s.db.Query(`
		SELECT ordinal, COALESCE(capture_file_id, ''), pcap_file
		  FROM lidar_replay_case_files
		 WHERE replay_case_id = ? ORDER BY ordinal`, replayCaseID)
	if err != nil {
		return nil, fmt.Errorf("list case files: %w", err)
	}
	defer rows.Close()

	files := []ReplayCaseFile{}
	for rows.Next() {
		var f ReplayCaseFile
		if err := rows.Scan(&f.Ordinal, &f.CaptureFileID, &f.PCAPFile); err != nil {
			return nil, fmt.Errorf("scan case file: %w", err)
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(files) > 0 {
		return files, nil
	}

	var legacy string
	if err := s.db.QueryRow(`
		SELECT pcap_file FROM lidar_replay_cases WHERE replay_case_id = ?`,
		replayCaseID).Scan(&legacy); err != nil {
		return nil, err
	}
	if legacy == "" {
		return nil, nil
	}
	return []ReplayCaseFile{{Ordinal: 0, PCAPFile: legacy}}, nil
}

// CasePaths is the case's captures as an ordered path list, the shape replay
// wants.
func (s *ReplayCaseStore) CasePaths(replayCaseID string) ([]string, error) {
	files, err := s.CaseFiles(replayCaseID)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.PCAPFile)
	}
	return paths, nil
}

// SetCaseSession links a case to the session and motion period it came from.
// Both are advisory, so a case outlives the session being re-derived.
func (s *ReplayCaseStore) SetCaseSession(replayCaseID, sessionIDValue, periodIDValue string) error {
	res, err := s.db.Exec(`
		UPDATE lidar_replay_cases SET session_id = ?, source_period_id = ?, updated_at_ns = ?
		 WHERE replay_case_id = ?`,
		nullIfEmpty(sessionIDValue), nullIfEmpty(periodIDValue),
		time.Now().UnixNano(), replayCaseID)
	if err != nil {
		return fmt.Errorf("link case to session: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// CaseSequenceExtent is a capture's packet-time extent as the case validator
// needs it. The caller supplies these because obtaining them means reading the
// files, which the storage layer does not do.
type CaseSequenceExtent struct {
	PCAPFile    string
	FirstPacket time.Time
	LastPacket  time.Time
	PacketCount uint64
}

// ValidateCaseSequence checks that a case's captures form one continuous
// stream, and returns the sequence so a caller can report its joins.
//
// A case whose files do not abut is not a case: replaying it would present two
// unrelated stretches to the pipeline as if they were one recording. Rejecting
// it at authoring time is the only point at which the operator can still do
// something about it.
func ValidateCaseSequence(extents []CaseSequenceExtent) (*capseq.Sequence, error) {
	if len(extents) == 0 {
		return nil, fmt.Errorf("a replay case needs at least one capture file")
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
