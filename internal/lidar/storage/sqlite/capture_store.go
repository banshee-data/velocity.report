package sqlite

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capindex"
	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
)

// CaptureRoot is a configured capture volume as the index records it.
type CaptureRoot struct {
	RootID        string `json:"root_id"`
	Path          string `json:"path"`
	Label         string `json:"label,omitempty"`
	Enabled       bool   `json:"enabled"`
	LastScanAtNs  *int64 `json:"last_scan_at_ns,omitempty"`
	LastScanState string `json:"last_scan_state"`
	LastScanError string `json:"last_scan_error,omitempty"`
	CreatedAtNs   int64  `json:"created_at_ns"`
	UpdatedAtNs   int64  `json:"updated_at_ns"`
	// ScanInProgress is process-local status supplied by the server. It is not
	// persisted: a restart cannot truthfully claim a cancelled probe continues.
	ScanInProgress bool `json:"scan_in_progress,omitempty"`
}

// Scan states a root can be in.
const (
	ScanStateNever       = "never"
	ScanStateOK          = "ok"
	ScanStateUnreachable = "unreachable"
	ScanStateError       = "error"
)

// Probe states a capture file can be in.
const (
	ProbeStatePending = "pending"
	ProbeStateOK      = "ok"
	ProbeStateFailed  = "failed"
)

// CaptureFile is one capture file as the index records it.
type CaptureFile struct {
	CaptureFileID string `json:"capture_file_id"`
	RootID        string `json:"root_id"`
	RelPath       string `json:"rel_path"`
	SizeBytes     int64  `json:"size_bytes"`
	ModifiedAtNs  int64  `json:"modified_at_ns"`
	ContentTag    string `json:"content_tag,omitempty"`
	FirstPacketNs *int64 `json:"first_packet_ns,omitempty"`
	LastPacketNs  *int64 `json:"last_packet_ns,omitempty"`
	PacketCount   *int64 `json:"packet_count,omitempty"`
	UDPPort       *int   `json:"udp_port,omitempty"`
	ProbeState    string `json:"probe_state"`
	ProbeError    string `json:"probe_error,omitempty"`
	ProbedAtNs    *int64 `json:"probed_at_ns,omitempty"`
	Present       bool   `json:"present"`
	FirstSeenAtNs int64  `json:"first_seen_at_ns"`
	LastSeenAtNs  int64  `json:"last_seen_at_ns"`
	SessionID     string `json:"session_id,omitempty"`
}

// CaptureSession is a derived run of contiguous capture files.
type CaptureSession struct {
	SessionID   string `json:"session_id"`
	RootID      string `json:"root_id"`
	Label       string `json:"label,omitempty"`
	SensorID    string `json:"sensor_id,omitempty"`
	FileCount   int    `json:"file_count"`
	StartNs     int64  `json:"start_ns"`
	EndNs       int64  `json:"end_ns"`
	CoveredNs   int64  `json:"covered_ns"`
	LostNs      int64  `json:"lost_ns"`
	WorstSeam   string `json:"worst_seam"`
	SizeBytes   int64  `json:"size_bytes"`
	DerivedAtNs int64  `json:"derived_at_ns"`
}

// CaptureStore persists the capture index.
type CaptureStore struct {
	db DBClient
}

// NewCaptureStore creates a CaptureStore.
func NewCaptureStore(db DBClient) *CaptureStore {
	return &CaptureStore{db: db}
}

// RootID derives a root's stable identifier from its path, so restarting the
// process with the same configuration addresses the same rows.
func RootID(path string) string {
	sum := sha256.Sum256([]byte(path))
	return "root-" + hex.EncodeToString(sum[:8])
}

// captureFileID derives a file's stable identifier from its root and relative
// path. Deriving rather than generating means a re-scan updates the existing
// row instead of orphaning whatever referenced it.
func captureFileID(rootID, relPath string) string {
	sum := sha256.Sum256([]byte(rootID + "\x00" + relPath))
	return "cap-" + hex.EncodeToString(sum[:12])
}

// sessionID derives a session's identifier from its root and the file it starts
// at, so re-deriving an unchanged session keeps its identity — and with it any
// label an operator gave it.
func sessionID(rootID, firstRelPath string) string {
	sum := sha256.Sum256([]byte(rootID + "\x00session\x00" + firstRelPath))
	return "ses-" + hex.EncodeToString(sum[:10])
}

// UpsertRoot records a configured root, preserving the scan state of one
// already known. Roots come from process configuration, so this is called at
// startup rather than from a request handler.
func (s *CaptureStore) UpsertRoot(path, label string, enabled bool) (CaptureRoot, error) {
	id := RootID(path)
	now := time.Now().UnixNano()
	_, err := s.db.Exec(`
		INSERT INTO lidar_capture_roots
			(root_id, path, label, enabled, last_scan_state, created_at_ns, updated_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (root_id) DO UPDATE SET
			label = excluded.label,
			enabled = excluded.enabled,
			updated_at_ns = excluded.updated_at_ns`,
		id, path, label, boolToInt(enabled), ScanStateNever, now, now)
	if err != nil {
		return CaptureRoot{}, fmt.Errorf("upsert capture root %s: %w", path, err)
	}
	return s.GetRoot(id)
}

// GetRoot returns one root by identifier.
func (s *CaptureStore) GetRoot(rootID string) (CaptureRoot, error) {
	row := s.db.QueryRow(`
		SELECT root_id, path, label, enabled, last_scan_at_ns, last_scan_state,
		       last_scan_error, created_at_ns, updated_at_ns
		  FROM lidar_capture_roots WHERE root_id = ?`, rootID)
	var r CaptureRoot
	var enabled int
	if err := row.Scan(&r.RootID, &r.Path, &r.Label, &enabled, &r.LastScanAtNs,
		&r.LastScanState, &r.LastScanError, &r.CreatedAtNs, &r.UpdatedAtNs); err != nil {
		return CaptureRoot{}, err
	}
	r.Enabled = enabled != 0
	return r, nil
}

// ListRoots returns the configured roots, oldest first.
func (s *CaptureStore) ListRoots() ([]CaptureRoot, error) {
	rows, err := s.db.Query(`
		SELECT root_id, path, label, enabled, last_scan_at_ns, last_scan_state,
		       last_scan_error, created_at_ns, updated_at_ns
		  FROM lidar_capture_roots ORDER BY created_at_ns, path`)
	if err != nil {
		return nil, fmt.Errorf("list capture roots: %w", err)
	}
	defer rows.Close()

	roots := []CaptureRoot{}
	for rows.Next() {
		var r CaptureRoot
		var enabled int
		if err := rows.Scan(&r.RootID, &r.Path, &r.Label, &enabled, &r.LastScanAtNs,
			&r.LastScanState, &r.LastScanError, &r.CreatedAtNs, &r.UpdatedAtNs); err != nil {
			return nil, fmt.Errorf("scan capture root: %w", err)
		}
		r.Enabled = enabled != 0
		roots = append(roots, r)
	}
	return roots, rows.Err()
}

// MarkScanned records the outcome of a scan against a root.
func (s *CaptureStore) MarkScanned(rootID, state, scanErr string) error {
	now := time.Now().UnixNano()
	_, err := s.db.Exec(`
		UPDATE lidar_capture_roots
		   SET last_scan_at_ns = ?, last_scan_state = ?, last_scan_error = ?, updated_at_ns = ?
		 WHERE root_id = ?`, now, state, scanErr, now, rootID)
	if err != nil {
		return fmt.Errorf("mark capture root scanned: %w", err)
	}
	return nil
}

// IndexedFiles returns what the index holds for a root, in the shape drift
// detection wants.
func (s *CaptureStore) IndexedFiles(rootID string) ([]capindex.Indexed, error) {
	rows, err := s.db.Query(`
		SELECT rel_path, size_bytes, modified_at_ns, content_tag, present, probe_state
		  FROM lidar_capture_files WHERE root_id = ? ORDER BY rel_path`, rootID)
	if err != nil {
		return nil, fmt.Errorf("list indexed files: %w", err)
	}
	defer rows.Close()

	out := []capindex.Indexed{}
	for rows.Next() {
		var f capindex.Indexed
		var modNs int64
		var present int
		var probeState string
		if err := rows.Scan(&f.RelPath, &f.SizeBytes, &modNs, &f.ContentTag, &present, &probeState); err != nil {
			return nil, fmt.Errorf("scan indexed file: %w", err)
		}
		f.ModifiedAt = time.Unix(0, modNs)
		f.Present = present != 0
		f.NeedsProbe = probeState != ProbeStateOK
		out = append(out, f)
	}
	return out, rows.Err()
}

// ApplyScan writes a scan's findings: every file found is upserted as present,
// and anything the index held but the scan did not see is marked absent.
//
// A file whose bytes moved has its probe reset, because the packet extent
// recorded for the old bytes says nothing about the new ones. A file that was
// merely touched keeps its extent.
func (s *CaptureStore) ApplyScan(rootID string, found []capindex.File, drift capindex.Drift) error {
	now := time.Now().UnixNano()
	reprobe := make(map[string]bool, len(drift.Changed))
	for _, c := range drift.Changed {
		if c.NeedsProbe() {
			reprobe[c.RelPath] = true
		}
	}
	for _, a := range drift.Added {
		reprobe[a.RelPath] = true
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin scan transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, f := range found {
		id := captureFileID(rootID, f.RelPath)
		if reprobe[f.RelPath] {
			_, err = tx.Exec(`
				INSERT INTO lidar_capture_files
					(capture_file_id, root_id, rel_path, size_bytes, modified_at_ns, content_tag,
					 probe_state, present, first_seen_at_ns, last_seen_at_ns)
				VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
				ON CONFLICT (capture_file_id) DO UPDATE SET
					size_bytes = excluded.size_bytes,
					modified_at_ns = excluded.modified_at_ns,
					content_tag = CASE WHEN excluded.content_tag = ''
						THEN lidar_capture_files.content_tag ELSE excluded.content_tag END,
					first_packet_ns = NULL, last_packet_ns = NULL, packet_count = NULL,
					udp_port = NULL, probed_at_ns = NULL, probe_error = '', session_id = NULL,
					probe_state = ?, present = 1, last_seen_at_ns = excluded.last_seen_at_ns`,
				id, rootID, f.RelPath, f.SizeBytes, f.ModifiedAt.UnixNano(), f.ContentTag,
				ProbeStatePending, now, now, ProbeStatePending)
		} else {
			_, err = tx.Exec(`
				INSERT INTO lidar_capture_files
					(capture_file_id, root_id, rel_path, size_bytes, modified_at_ns, content_tag,
					 probe_state, present, first_seen_at_ns, last_seen_at_ns)
				VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
				ON CONFLICT (capture_file_id) DO UPDATE SET
					size_bytes = excluded.size_bytes,
					modified_at_ns = excluded.modified_at_ns,
					content_tag = CASE WHEN excluded.content_tag = ''
						THEN lidar_capture_files.content_tag ELSE excluded.content_tag END,
					present = 1, last_seen_at_ns = excluded.last_seen_at_ns`,
				id, rootID, f.RelPath, f.SizeBytes, f.ModifiedAt.UnixNano(), f.ContentTag,
				ProbeStatePending, now, now)
		}
		if err != nil {
			return fmt.Errorf("upsert capture file %s: %w", f.RelPath, err)
		}
	}

	for _, m := range drift.Missing {
		if _, err := tx.Exec(`
			UPDATE lidar_capture_files SET present = 0, session_id = NULL
			 WHERE capture_file_id = ?`, captureFileID(rootID, m.RelPath)); err != nil {
			return fmt.Errorf("mark capture file absent %s: %w", m.RelPath, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit scan: %w", err)
	}
	return nil
}

// RecordProbe stores a file's packet-time extent.
func (s *CaptureStore) RecordProbe(rootID, relPath string, firstNs, lastNs int64, count int64, udpPort int) error {
	now := time.Now().UnixNano()
	_, err := s.db.Exec(`
		UPDATE lidar_capture_files
		   SET first_packet_ns = ?, last_packet_ns = ?, packet_count = ?, udp_port = ?,
		       probe_state = ?, probe_error = '', probed_at_ns = ?
		 WHERE capture_file_id = ?`,
		firstNs, lastNs, count, udpPort, ProbeStateOK, now, captureFileID(rootID, relPath))
	if err != nil {
		return fmt.Errorf("record probe for %s: %w", relPath, err)
	}
	return nil
}

// RecordProbeFailure stores why a file could not be probed, so a scan does not
// retry it silently for ever.
func (s *CaptureStore) RecordProbeFailure(rootID, relPath, reason string) error {
	now := time.Now().UnixNano()
	_, err := s.db.Exec(`
		UPDATE lidar_capture_files
		   SET probe_state = ?, probe_error = ?, probed_at_ns = ?
		 WHERE capture_file_id = ?`,
		ProbeStateFailed, reason, now, captureFileID(rootID, relPath))
	if err != nil {
		return fmt.Errorf("record probe failure for %s: %w", relPath, err)
	}
	return nil
}

// ListFiles returns a root's indexed files, present ones first by packet time
// and then by path for those not yet probed.
func (s *CaptureStore) ListFiles(rootID string) ([]CaptureFile, error) {
	query := `
		SELECT capture_file_id, root_id, rel_path, size_bytes, modified_at_ns, content_tag,
		       first_packet_ns, last_packet_ns, packet_count, udp_port, probe_state,
		       probe_error, probed_at_ns, present, first_seen_at_ns, last_seen_at_ns,
		       COALESCE(session_id, '')
		  FROM lidar_capture_files`
	args := []any{}
	if rootID != "" {
		query += ` WHERE root_id = ?`
		args = append(args, rootID)
	}
	query += ` ORDER BY present DESC, first_packet_ns, rel_path`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list capture files: %w", err)
	}
	defer rows.Close()

	files := []CaptureFile{}
	for rows.Next() {
		var f CaptureFile
		var present int
		if err := rows.Scan(&f.CaptureFileID, &f.RootID, &f.RelPath, &f.SizeBytes, &f.ModifiedAtNs,
			&f.ContentTag, &f.FirstPacketNs, &f.LastPacketNs, &f.PacketCount, &f.UDPPort,
			&f.ProbeState, &f.ProbeError, &f.ProbedAtNs, &present, &f.FirstSeenAtNs,
			&f.LastSeenAtNs, &f.SessionID); err != nil {
			return nil, fmt.Errorf("scan capture file: %w", err)
		}
		f.Present = present != 0
		files = append(files, f)
	}
	return files, rows.Err()
}

// ProbedFiles returns a root's present, successfully probed files in the shape
// session derivation wants.
func (s *CaptureStore) ProbedFiles(rootID string) ([]capindex.Probed, error) {
	rows, err := s.db.Query(`
		SELECT rel_path, first_packet_ns, last_packet_ns, COALESCE(packet_count, 0), size_bytes
		  FROM lidar_capture_files
		 WHERE root_id = ? AND present = 1 AND probe_state = ?
		       AND first_packet_ns IS NOT NULL AND last_packet_ns IS NOT NULL
		 ORDER BY first_packet_ns`, rootID, ProbeStateOK)
	if err != nil {
		return nil, fmt.Errorf("list probed files: %w", err)
	}
	defer rows.Close()

	out := []capindex.Probed{}
	for rows.Next() {
		var p capindex.Probed
		var firstNs, lastNs, count int64
		if err := rows.Scan(&p.RelPath, &firstNs, &lastNs, &count, &p.SizeBytes); err != nil {
			return nil, fmt.Errorf("scan probed file: %w", err)
		}
		p.FirstPacket = time.Unix(0, firstNs)
		p.LastPacket = time.Unix(0, lastNs)
		p.PacketCount = uint64(count)
		out = append(out, p)
	}
	return out, rows.Err()
}

// ReplaceSessions re-derives a root's sessions, replacing whatever was there.
//
// Labels survive: a session that still starts at the same file keeps its
// identity, and with it the site name an operator gave it. A session whose
// first file changed is a different session and starts unlabelled.
func (s *CaptureStore) ReplaceSessions(rootID string, sessions []capindex.Session) error {
	now := time.Now().UnixNano()

	labels := map[string]string{}
	existing, err := s.ListSessions(rootID)
	if err != nil {
		return err
	}
	for _, e := range existing {
		if e.Label != "" {
			labels[e.SessionID] = e.Label
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin session transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM lidar_capture_sessions WHERE root_id = ?`, rootID); err != nil {
		return fmt.Errorf("clear sessions: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE lidar_capture_files SET session_id = NULL WHERE root_id = ?`, rootID); err != nil {
		return fmt.Errorf("clear file session links: %w", err)
	}

	for _, sess := range sessions {
		if len(sess.Files) == 0 {
			continue
		}
		id := sessionID(rootID, sess.Files[0].RelPath)
		if _, err := tx.Exec(`
			INSERT INTO lidar_capture_sessions
				(session_id, root_id, label, file_count, start_ns, end_ns, covered_ns,
				 lost_ns, worst_seam, size_bytes, derived_at_ns)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, rootID, labels[id], len(sess.Files), sess.Start.UnixNano(), sess.End.UnixNano(),
			int64(sess.Covered), int64(sess.Lost), string(sess.Worst), sess.SizeBytes, now); err != nil {
			return fmt.Errorf("insert session: %w", err)
		}
		for _, f := range sess.Files {
			if _, err := tx.Exec(`
				UPDATE lidar_capture_files SET session_id = ? WHERE capture_file_id = ?`,
				id, captureFileID(rootID, f.RelPath)); err != nil {
				return fmt.Errorf("link file %s to session: %w", f.RelPath, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sessions: %w", err)
	}
	return nil
}

// ListSessions returns derived sessions, newest first. An empty rootID lists
// every root's.
func (s *CaptureStore) ListSessions(rootID string) ([]CaptureSession, error) {
	query := `
		SELECT session_id, root_id, label, sensor_id, file_count, start_ns, end_ns,
		       covered_ns, lost_ns, worst_seam, size_bytes, derived_at_ns
		  FROM lidar_capture_sessions`
	args := []any{}
	if rootID != "" {
		query += ` WHERE root_id = ?`
		args = append(args, rootID)
	}
	query += ` ORDER BY start_ns DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	sessions := []CaptureSession{}
	for rows.Next() {
		var sess CaptureSession
		if err := rows.Scan(&sess.SessionID, &sess.RootID, &sess.Label, &sess.SensorID,
			&sess.FileCount, &sess.StartNs, &sess.EndNs, &sess.CoveredNs, &sess.LostNs,
			&sess.WorstSeam, &sess.SizeBytes, &sess.DerivedAtNs); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

// SetSessionLabel names a session, which is how a site — broadway_columbus, say
// — gets attached to a run of files the clock grouped.
func (s *CaptureStore) SetSessionLabel(sessionIDValue, label, sensorID string) error {
	res, err := s.db.Exec(`
		UPDATE lidar_capture_sessions SET label = ?, sensor_id = ? WHERE session_id = ?`,
		label, sensorID, sessionIDValue)
	if err != nil {
		return fmt.Errorf("label session: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// SessionFiles returns a session's files in packet-time order.
func (s *CaptureStore) SessionFiles(sessionIDValue string) ([]CaptureFile, error) {
	rows, err := s.db.Query(`
		SELECT capture_file_id, root_id, rel_path, size_bytes, modified_at_ns, content_tag,
		       first_packet_ns, last_packet_ns, packet_count, udp_port, probe_state,
		       probe_error, probed_at_ns, present, first_seen_at_ns, last_seen_at_ns,
		       COALESCE(session_id, '')
		  FROM lidar_capture_files WHERE session_id = ? ORDER BY first_packet_ns`, sessionIDValue)
	if err != nil {
		return nil, fmt.Errorf("list session files: %w", err)
	}
	defer rows.Close()

	files := []CaptureFile{}
	for rows.Next() {
		var f CaptureFile
		var present int
		if err := rows.Scan(&f.CaptureFileID, &f.RootID, &f.RelPath, &f.SizeBytes, &f.ModifiedAtNs,
			&f.ContentTag, &f.FirstPacketNs, &f.LastPacketNs, &f.PacketCount, &f.UDPPort,
			&f.ProbeState, &f.ProbeError, &f.ProbedAtNs, &present, &f.FirstSeenAtNs,
			&f.LastSeenAtNs, &f.SessionID); err != nil {
			return nil, fmt.Errorf("scan session file: %w", err)
		}
		f.Present = present != 0
		files = append(files, f)
	}
	return files, rows.Err()
}

// DeriveSessions re-derives a root's sessions from its probed files.
func (s *CaptureStore) DeriveSessions(rootID string) ([]CaptureSession, error) {
	probedFiles, err := s.ProbedFiles(rootID)
	if err != nil {
		return nil, err
	}
	if err := s.ReplaceSessions(rootID, capindex.Sessions(probedFiles, capseq.DefaultTolerances())); err != nil {
		return nil, err
	}
	return s.ListSessions(rootID)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
