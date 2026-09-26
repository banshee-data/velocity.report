package sqlite

// InteractionStore persists behaviour interactions, per Section 10.3 of
// docs/plans/lidar-behaviour-analytics-plan.md: the event, instant and
// exposure-window records of internal/lidar/l8behaviour, one table each.
//
// Every row is JSON-first. The *_json column is the l8behaviour record as
// json.Marshal writes it, and is authoritative; the generated columns only
// index it. This store names no metric: metric ids and suppression tokens
// live inside the payload, keyed by the l8behaviour registry, so storage
// cannot drift from it.
//
// Rows are write-once per version. An event id digests the source, pair and
// every version axis, so a regeneration under a new version inserts beside
// the old rows. Re-inserting an identical interaction is a no-op, which keeps
// a deterministic re-run harmless; re-inserting the same id with different
// content is ErrInteractionConflict and writes nothing, because a derived row
// that silently changed under the same version is exactly the drift the
// version exists to prevent. Readers select one InteractionVersion; there is
// no query that returns events of two versions.
//
// Every interaction is validated as a whole record set before a write: the
// event, its instants and its windows must agree with one another
// (l8behaviour.FollowingInteraction.Validate). Reads differ in what they can
// check. Get and ListInteractionsOverlapping read whole record sets and
// validate each set the same way, so an instant or window edited by hand
// fails loudly rather than reaching a report. ListEvents and ListWindows read
// one table and validate each row on its own: they refuse a malformed row,
// but not one that disagrees with its rows in another table. A caller that
// depends on cross-table consistency, such as an aggregate over instants,
// reads whole record sets.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

// ErrInteractionConflict reports an attempt to store different content under
// an existing interaction id. A changed result needs a new version, not a
// replacement.
var ErrInteractionConflict = errors.New("interaction already stored with different content")

// InteractionStore owns lidar_interaction_events, lidar_interaction_instants
// and lidar_exposure_windows. It has no update method by design.
type InteractionStore struct{ db DBClient }

// NewInteractionStore returns a store over db.
func NewInteractionStore(db DBClient) *InteractionStore { return &InteractionStore{db: db} }

// InteractionVersionSummary is one version present for a source.
type InteractionVersionSummary struct {
	Version               l8behaviour.InteractionVersion
	Events                int
	LatestInsertedAtNanos int64
}

// queryer is the read surface shared by a database and a transaction.
type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Insert writes interactions in one transaction: all of them, or none.
func (s *InteractionStore) Insert(interactions ...l8behaviour.FollowingInteraction) error {
	return insertInteractions(s.db, interactions, time.Now().UnixNano())
}

func insertInteractions(db DBClient, interactions []l8behaviour.FollowingInteraction, insertedAtNanos int64) error {
	for _, fi := range interactions {
		if err := fi.Validate(); err != nil {
			return fmt.Errorf("refuse to store interaction: %w", err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin interaction insert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, fi := range interactions {
		if err := insertInteraction(tx, fi, insertedAtNanos); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit interaction insert: %w", err)
	}
	return nil
}

func insertInteraction(tx *sql.Tx, fi l8behaviour.FollowingInteraction, insertedAtNanos int64) error {
	event, instants, windows, err := encodeInteraction(fi)
	if err != nil {
		return err
	}
	id := fi.Event.EventID
	res, err := tx.Exec(`
		INSERT INTO lidar_interaction_events (event_id, event_json, inserted_at_ns)
		VALUES (?, ?, ?)
		ON CONFLICT (event_id) DO NOTHING`, id, string(event), insertedAtNanos)
	if err != nil {
		return fmt.Errorf("insert interaction event %s: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("insert interaction event %s: %w", id, err)
	} else if n == 0 {
		return sameAsStored(tx, id, event, instants, windows)
	}
	for i, raw := range instants {
		if _, err := tx.Exec(`
			INSERT INTO lidar_interaction_instants (event_id, capture_unix_nanos, instant_json)
			VALUES (?, ?, ?)`, id, fi.Instants[i].CaptureUnixNanos, string(raw)); err != nil {
			return fmt.Errorf("insert interaction instant %s at %d: %w", id, fi.Instants[i].CaptureUnixNanos, err)
		}
	}
	for i, raw := range windows {
		if _, err := tx.Exec(`
			INSERT INTO lidar_exposure_windows (window_id, event_id, window_json, inserted_at_ns)
			VALUES (?, ?, ?, ?)`, fi.Windows[i].WindowID, id, string(raw), insertedAtNanos); err != nil {
			return fmt.Errorf("insert exposure window %s: %w", fi.Windows[i].WindowID, err)
		}
	}
	return nil
}

// encodeInteraction marshals each record as it is stored. encoding/json
// sorts map keys, so equal content always encodes to equal bytes.
func encodeInteraction(fi l8behaviour.FollowingInteraction) (event []byte, instants, windows [][]byte, err error) {
	if event, err = json.Marshal(fi.Event); err != nil {
		return nil, nil, nil, fmt.Errorf("marshal interaction event %s: %w", fi.Event.EventID, err)
	}
	for _, in := range fi.Instants {
		raw, err := json.Marshal(in)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("marshal interaction instant %d: %w", in.CaptureUnixNanos, err)
		}
		instants = append(instants, raw)
	}
	for _, w := range fi.Windows {
		raw, err := json.Marshal(w)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("marshal exposure window %s: %w", w.WindowID, err)
		}
		windows = append(windows, raw)
	}
	return event, instants, windows, nil
}

// sameAsStored compares an interaction with the rows already stored under its
// id, byte for byte: nil when identical, ErrInteractionConflict otherwise.
func sameAsStored(q queryer, id string, event []byte, instants, windows [][]byte) error {
	storedEvent, storedInstants, storedWindows, err := readInteractionRows(q, id)
	if err != nil {
		return err
	}
	if !bytes.Equal(storedEvent, event) || !equalRows(storedInstants, instants) || !equalRows(storedWindows, windows) {
		return fmt.Errorf("%w: %s", ErrInteractionConflict, id)
	}
	return nil
}

func equalRows(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// readInteractionRows returns one event's stored payloads: the event, its
// instants in capture order and its windows in start order.
func readInteractionRows(q queryer, id string) (event []byte, instants, windows [][]byte, err error) {
	var raw string
	if err := q.QueryRow(`SELECT event_json FROM lidar_interaction_events WHERE event_id = ?`, id).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil, ErrNotFound
		}
		return nil, nil, nil, fmt.Errorf("read interaction event %s: %w", id, err)
	}
	if instants, err = readPayloads(q, `
		SELECT instant_json FROM lidar_interaction_instants
		 WHERE event_id = ?
		 ORDER BY capture_unix_nanos`, id); err != nil {
		return nil, nil, nil, fmt.Errorf("read interaction instants %s: %w", id, err)
	}
	if windows, err = readPayloads(q, `
		SELECT window_json FROM lidar_exposure_windows
		 WHERE event_id = ?
		 ORDER BY start_unix_nanos`, id); err != nil {
		return nil, nil, nil, fmt.Errorf("read exposure windows %s: %w", id, err)
	}
	return []byte(raw), instants, windows, nil
}

func readPayloads(q queryer, query string, args ...any) ([][]byte, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, []byte(raw))
	}
	return out, rows.Err()
}

// Get returns one stored interaction with its instants and windows,
// validated. A missing id is ErrNotFound.
func (s *InteractionStore) Get(eventID string) (l8behaviour.FollowingInteraction, error) {
	event, instants, windows, err := readInteractionRows(s.db, eventID)
	if err != nil {
		return l8behaviour.FollowingInteraction{}, err
	}
	return decodeInteraction(eventID, event, instants, windows)
}

// decodeInteraction decodes one interaction's stored payloads, instants in
// capture order and windows in start order, and validates the result.
func decodeInteraction(eventID string, event []byte, instants, windows [][]byte) (l8behaviour.FollowingInteraction, error) {
	var fi l8behaviour.FollowingInteraction
	if err := json.Unmarshal(event, &fi.Event); err != nil {
		return l8behaviour.FollowingInteraction{}, fmt.Errorf("decode stored interaction %s: %w", eventID, err)
	}
	for _, raw := range instants {
		var in l8behaviour.InteractionInstant
		if err := json.Unmarshal(raw, &in); err != nil {
			return l8behaviour.FollowingInteraction{}, fmt.Errorf("decode stored interaction instant %s: %w", eventID, err)
		}
		fi.Instants = append(fi.Instants, in)
	}
	for _, raw := range windows {
		var w l8behaviour.ExposureWindow
		if err := json.Unmarshal(raw, &w); err != nil {
			return l8behaviour.FollowingInteraction{}, fmt.Errorf("decode stored exposure window %s: %w", eventID, err)
		}
		fi.Windows = append(fi.Windows, w)
	}
	if err := fi.Validate(); err != nil {
		return l8behaviour.FollowingInteraction{}, fmt.Errorf("stored interaction %s does not validate: %w", eventID, err)
	}
	return fi, nil
}

// --- Capture windows ---------------------------------------------------------
//
// A scene records the capture window it shows (lidar_scenes.captured_start_ns
// and captured_end_ns) but not the analysis source its encounters were
// written under: the source id is a digest over the replay case, capture
// paths, capture digests and extractor, none of which a scene stores. These
// readers find a capture's analyses by time instead. An event belongs to a
// window only when its full capture interval lies inside it, so scene
// aggregates never count time outside the scene's declared bounds. Two
// sources covering one window, such as one capture extracted under two
// tunings, are never merged here: the caller
// lists them and chooses one.

// InteractionSourceSummary is one source with following events inside a
// capture window.
type InteractionSourceSummary struct {
	SourceID string
	Events   int
	// FirstUnixNanos and LastUnixNanos are the earliest start and latest end
	// of those events inside the window.
	FirstUnixNanos int64
	LastUnixNanos  int64
}

// captureWindowFilter is the WHERE clause fragment selecting following events
// whose capture interval lies wholly inside [start, end], both inclusive.
func captureWindowFilter(startUnixNanos, endUnixNanos int64) (string, []any, error) {
	if startUnixNanos <= 0 || endUnixNanos < startUnixNanos {
		return "", nil, fmt.Errorf("capture window %d to %d is not ordered", startUnixNanos, endUnixNanos)
	}
	return `interaction_type = ? AND start_unix_nanos >= ? AND end_unix_nanos <= ?`,
		[]any{l8behaviour.InteractionFollowing.String(), startUnixNanos, endUnixNanos}, nil
}

// SourcesOverlapping lists every source with following events inside a
// capture window, by source id.
func (s *InteractionStore) SourcesOverlapping(startUnixNanos, endUnixNanos int64) ([]InteractionSourceSummary, error) {
	window, args, err := captureWindowFilter(startUnixNanos, endUnixNanos)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT source_id, COUNT(*), MIN(start_unix_nanos), MAX(end_unix_nanos)
		  FROM lidar_interaction_events
		 WHERE `+window+`
		 GROUP BY source_id
		 ORDER BY source_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list interaction sources from %d to %d: %w", startUnixNanos, endUnixNanos, err)
	}
	defer rows.Close()
	var out []InteractionSourceSummary
	for rows.Next() {
		var sum InteractionSourceSummary
		if err := rows.Scan(&sum.SourceID, &sum.Events, &sum.FirstUnixNanos, &sum.LastUnixNanos); err != nil {
			return nil, fmt.Errorf("scan interaction source: %w", err)
		}
		out = append(out, sum)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interaction sources: %w", err)
	}
	return out, nil
}

// VersionsOverlapping is Versions restricted to a source's following events
// inside a capture window, most recently written first.
func (s *InteractionStore) VersionsOverlapping(sourceID string, startUnixNanos, endUnixNanos int64) ([]InteractionVersionSummary, error) {
	window, args, err := captureWindowFilter(startUnixNanos, endUnixNanos)
	if err != nil {
		return nil, err
	}
	return s.versions(sourceID, ` AND `+window, args...)
}

// ListInteractionsOverlapping returns a source's following interactions at
// exactly one version whose capture interval lies wholly inside a window: each event
// with its instants and windows, validated, in start then pair order. The
// three reads share one transaction, so a concurrent write or delete cannot
// leave an event without its evidence.
func (s *InteractionStore) ListInteractionsOverlapping(sourceID string, v l8behaviour.InteractionVersion,
	startUnixNanos, endUnixNanos int64) ([]l8behaviour.FollowingInteraction, error) {
	where, args, err := versionFilter(v)
	if err != nil {
		return nil, err
	}
	window, windowArgs, err := captureWindowFilter(startUnixNanos, endUnixNanos)
	if err != nil {
		return nil, err
	}
	selected := `FROM lidar_interaction_events WHERE source_id = ? AND ` + where + ` AND ` + window
	args = append(append([]any{sourceID}, args...), windowArgs...)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin interaction read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	events, err := readKeyedPayloads(tx, `
		SELECT event_id, event_json `+selected+`
		 ORDER BY start_unix_nanos, primary_track_id, secondary_track_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list interaction events for source %s: %w", sourceID, err)
	}
	instants, err := readKeyedPayloads(tx, `
		SELECT event_id, instant_json FROM lidar_interaction_instants
		 WHERE event_id IN (SELECT event_id `+selected+`)
		 ORDER BY event_id, capture_unix_nanos`, args...)
	if err != nil {
		return nil, fmt.Errorf("list interaction instants for source %s: %w", sourceID, err)
	}
	windows, err := readKeyedPayloads(tx, `
		SELECT event_id, window_json FROM lidar_exposure_windows
		 WHERE event_id IN (SELECT event_id `+selected+`)
		 ORDER BY event_id, start_unix_nanos`, args...)
	if err != nil {
		return nil, fmt.Errorf("list exposure windows for source %s: %w", sourceID, err)
	}

	byEvent := func(rows []keyedPayload) (map[string][][]byte, error) {
		out := map[string][][]byte{}
		for _, r := range rows {
			if _, ok := events.index[r.key]; !ok {
				return nil, fmt.Errorf("row belongs to event %s, which the version and window do not select", r.key)
			}
			out[r.key] = append(out[r.key], r.payload)
		}
		return out, nil
	}
	instantsOf, err := byEvent(instants.rows)
	if err != nil {
		return nil, err
	}
	windowsOf, err := byEvent(windows.rows)
	if err != nil {
		return nil, err
	}
	out := make([]l8behaviour.FollowingInteraction, 0, len(events.rows))
	for _, e := range events.rows {
		fi, err := decodeInteraction(e.key, e.payload, instantsOf[e.key], windowsOf[e.key])
		if err != nil {
			return nil, err
		}
		out = append(out, fi)
	}
	return out, nil
}

type keyedPayload struct {
	key     string
	payload []byte
}

type keyedPayloads struct {
	rows  []keyedPayload
	index map[string]int
}

// readKeyedPayloads reads (key, payload) rows in query order.
func readKeyedPayloads(q queryer, query string, args ...any) (keyedPayloads, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return keyedPayloads{}, err
	}
	defer rows.Close()
	out := keyedPayloads{index: map[string]int{}}
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return keyedPayloads{}, err
		}
		if _, dup := out.index[key]; !dup {
			out.index[key] = len(out.rows)
		}
		out.rows = append(out.rows, keyedPayload{key: key, payload: []byte(raw)})
	}
	return out, rows.Err()
}

// versionFilter is the WHERE clause fragment and arguments selecting exactly
// one version.
func versionFilter(v l8behaviour.InteractionVersion) (string, []any, error) {
	if err := v.Validate(); err != nil {
		return "", nil, err
	}
	return `estimate_stage = ? AND estimator_id = ? AND obs_model_id = ? AND method_id = ? AND param_hash = ?`,
		[]any{v.EstimateStage.String(), v.EstimatorID, v.ObsModelID, v.MethodID, v.ParamHash}, nil
}

// ListEvents returns a source's events at exactly one version, in start time
// then pair order. Instants and windows are not read; Get returns them. Each
// event is validated on its own, not against its instants.
func (s *InteractionStore) ListEvents(sourceID string, v l8behaviour.InteractionVersion) ([]l8behaviour.InteractionEvent, error) {
	where, args, err := versionFilter(v)
	if err != nil {
		return nil, err
	}
	payloads, err := readPayloads(s.db, `
		SELECT event_json FROM lidar_interaction_events
		 WHERE source_id = ? AND `+where+`
		 ORDER BY start_unix_nanos, primary_track_id, secondary_track_id`, append([]any{sourceID}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("list interaction events for source %s: %w", sourceID, err)
	}
	events := make([]l8behaviour.InteractionEvent, 0, len(payloads))
	for _, raw := range payloads {
		var ev l8behaviour.InteractionEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			return nil, fmt.Errorf("decode stored interaction event: %w", err)
		}
		if err := ev.Validate(); err != nil {
			return nil, fmt.Errorf("stored interaction %s does not validate: %w", ev.EventID, err)
		}
		events = append(events, ev)
	}
	return events, nil
}

// ListWindows returns a source's exposure windows of one kind and basis at
// exactly one version, in start order. A denominator reads basis observed.
// Each window is validated on its own, not against the instants it was cut
// from.
func (s *InteractionStore) ListWindows(sourceID string, v l8behaviour.InteractionVersion,
	kind l8behaviour.ExposureKind, basis l8behaviour.ObservationBasis) ([]l8behaviour.ExposureWindow, error) {
	where, args, err := versionFilter(v)
	if err != nil {
		return nil, err
	}
	if !kind.Valid() || !basis.Valid() {
		return nil, fmt.Errorf("exposure window query requires a kind and a basis")
	}
	payloads, err := readPayloads(s.db, `
		SELECT window_json FROM lidar_exposure_windows
		 WHERE source_id = ? AND kind = ? AND basis = ? AND `+where+`
		 ORDER BY start_unix_nanos, window_id`,
		append([]any{sourceID, kind.String(), basis.String()}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("list exposure windows for source %s: %w", sourceID, err)
	}
	windows := make([]l8behaviour.ExposureWindow, 0, len(payloads))
	for _, raw := range payloads {
		var w l8behaviour.ExposureWindow
		if err := json.Unmarshal(raw, &w); err != nil {
			return nil, fmt.Errorf("decode stored exposure window: %w", err)
		}
		if err := w.Validate(); err != nil {
			return nil, fmt.Errorf("stored exposure window does not validate: %w", err)
		}
		windows = append(windows, w)
	}
	return windows, nil
}

// Versions lists the versions stored for a source, most recently written
// first.
func (s *InteractionStore) Versions(sourceID string) ([]InteractionVersionSummary, error) {
	return s.versions(sourceID, "")
}

// versions lists a source's versions over the events a further WHERE
// fragment selects, most recently written first.
func (s *InteractionStore) versions(sourceID, and string, args ...any) ([]InteractionVersionSummary, error) {
	rows, err := s.db.Query(`
		SELECT estimate_stage, estimator_id, obs_model_id, method_id, param_hash
		     , COUNT(*), MAX(inserted_at_ns)
		  FROM lidar_interaction_events
		 WHERE source_id = ?`+and+`
		 GROUP BY estimate_stage, estimator_id, obs_model_id, method_id, param_hash
		 ORDER BY MAX(inserted_at_ns) DESC, estimate_stage, estimator_id, obs_model_id, method_id, param_hash`,
		append([]any{sourceID}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("list interaction versions for source %s: %w", sourceID, err)
	}
	defer rows.Close()
	var out []InteractionVersionSummary
	for rows.Next() {
		var sum InteractionVersionSummary
		var stage string
		if err := rows.Scan(&stage, &sum.Version.EstimatorID, &sum.Version.ObsModelID, &sum.Version.MethodID,
			&sum.Version.ParamHash, &sum.Events, &sum.LatestInsertedAtNanos); err != nil {
			return nil, fmt.Errorf("scan interaction version: %w", err)
		}
		if sum.Version.EstimateStage, err = l8behaviour.ParseEstimateStage(stage); err != nil {
			return nil, fmt.Errorf("stored interaction version: %w", err)
		}
		out = append(out, sum)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate interaction versions: %w", err)
	}
	return out, nil
}

// LatestVersion is the most recently written version at one stage, so a
// production reader asking for final never receives a fixed-lag version.
// ErrNotFound when the source has none at that stage.
func (s *InteractionStore) LatestVersion(sourceID string, stage l8behaviour.EstimateStage) (l8behaviour.InteractionVersion, error) {
	versions, err := s.Versions(sourceID)
	if err != nil {
		return l8behaviour.InteractionVersion{}, err
	}
	for _, v := range versions {
		if v.Version.EstimateStage == stage {
			return v.Version, nil
		}
	}
	return l8behaviour.InteractionVersion{}, ErrNotFound
}

// DeleteVersion removes one version of a source's interactions, with their
// instants and windows, in one transaction, and returns the events removed.
// It is how an operator reclaims space from superseded versions; derived rows
// are regenerated from persisted estimates, never edited. Children are deleted
// explicitly rather than left to ON DELETE CASCADE, because foreign-key
// enforcement is a per-connection pragma and this must not depend on which
// pooled connection runs it.
func (s *InteractionStore) DeleteVersion(sourceID string, v l8behaviour.InteractionVersion) (int64, error) {
	where, args, err := versionFilter(v)
	if err != nil {
		return 0, err
	}
	args = append([]any{sourceID}, args...)
	events := `SELECT event_id FROM lidar_interaction_events WHERE source_id = ? AND ` + where
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin interaction delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, child := range []string{"lidar_exposure_windows", "lidar_interaction_instants"} {
		if _, err := tx.Exec(`DELETE FROM `+child+` WHERE event_id IN (`+events+`)`, args...); err != nil {
			return 0, fmt.Errorf("delete %s for source %s: %w", child, sourceID, err)
		}
	}
	res, err := tx.Exec(`DELETE FROM lidar_interaction_events WHERE source_id = ? AND `+where, args...)
	if err != nil {
		return 0, fmt.Errorf("delete interaction version for source %s: %w", sourceID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit interaction delete: %w", err)
	}
	return n, nil
}
