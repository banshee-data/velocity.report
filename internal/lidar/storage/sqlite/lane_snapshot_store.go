package sqlite

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/lanes"
)

// ErrLaneRequestConflict is returned when a request id is reused with other
// content: a retry must be the same request.
var ErrLaneRequestConflict = errors.New("lane snapshot request id reused with different content")

// LaneSnapshotStore keeps the append-only lane snapshots
// (lidar_lane_snapshots, lidar_lane_snapshot_lanes).
type LaneSnapshotStore struct{ db DBClient }

// NewLaneSnapshotStore returns a store over db.
func NewLaneSnapshotStore(db DBClient) *LaneSnapshotStore { return &LaneSnapshotStore{db: db} }

// Put writes a snapshot of the request, superseding the site and sensor's
// latest. A repeat of a stored request (same site and request id, same
// content) returns the stored snapshot and true, writing nothing; the same
// id with other content is ErrLaneRequestConflict. Invalid requests wrap
// lanes.ErrInvalid.
func (s *LaneSnapshotStore) Put(l16 string, req lanes.Request, now time.Time) (lanes.Snapshot, bool, error) {
	if err := req.Validate(); err != nil {
		return lanes.Snapshot{}, false, err
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return lanes.Snapshot{}, false, err
	}
	var storedReq, storedSnap string
	err = s.db.QueryRow(`SELECT request_json, snapshot_json FROM lidar_lane_snapshots WHERE l16_token = ? AND request_id = ?`,
		l16, req.RequestID).Scan(&storedReq, &storedSnap)
	switch {
	case err == nil:
		if !bytes.Equal([]byte(storedReq), reqJSON) {
			return lanes.Snapshot{}, false, ErrLaneRequestConflict
		}
		var snap lanes.Snapshot
		if err := json.Unmarshal([]byte(storedSnap), &snap); err != nil {
			return lanes.Snapshot{}, false, err
		}
		return snap, true, nil
	case !errors.Is(err, sql.ErrNoRows):
		return lanes.Snapshot{}, false, fmt.Errorf("read lane snapshot request: %w", err)
	}

	supersedes := ""
	if latest, err := s.Latest(l16, req.SensorID); err != nil {
		return lanes.Snapshot{}, false, err
	} else if latest != nil {
		supersedes = latest.SnapshotID
		// Never earlier than the snapshot it supersedes, so order by time
		// is order of writing.
		if !now.After(latest.FetchedAt) {
			now = latest.FetchedAt.Add(time.Nanosecond)
		}
	}
	snap, err := lanes.Build(l16, req, now, supersedes)
	if err != nil {
		return lanes.Snapshot{}, false, err
	}
	snapJSON, err := json.Marshal(snap)
	if err != nil {
		return lanes.Snapshot{}, false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return lanes.Snapshot{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO lidar_lane_snapshots
		(snapshot_id, l16_token, sensor_id, writer, request_id, record_id, features_digest, fetched_at_ns, supersedes, snapshot_json, request_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.SnapshotID, l16, snap.SensorID, snap.Writer, snap.RequestID, snap.RecordID, snap.FeaturesDigest,
		snap.FetchedAt.UnixNano(), nullString(supersedes), string(snapJSON), string(reqJSON)); err != nil {
		return lanes.Snapshot{}, false, fmt.Errorf("insert lane snapshot: %w", err)
	}
	for _, l := range snap.Lanes {
		laneJSON, err := json.Marshal(l)
		if err != nil {
			return lanes.Snapshot{}, false, err
		}
		if _, err := tx.Exec(`INSERT INTO lidar_lane_snapshot_lanes (snapshot_id, feature_id, row_id, lane_json) VALUES (?, ?, ?, ?)`,
			snap.SnapshotID, l.FeatureID, l.RowID, string(laneJSON)); err != nil {
			return lanes.Snapshot{}, false, fmt.Errorf("insert lane: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return lanes.Snapshot{}, false, err
	}
	return snap, false, nil
}

// All returns a site and sensor's snapshots, oldest first. An empty sensor
// reads every sensor's.
func (s *LaneSnapshotStore) All(l16, sensorID string) ([]lanes.Snapshot, error) {
	q := `SELECT snapshot_json FROM lidar_lane_snapshots WHERE l16_token = ?`
	args := []any{l16}
	if sensorID != "" {
		q += ` AND sensor_id = ?`
		args = append(args, sensorID)
	}
	q += ` ORDER BY fetched_at_ns, snapshot_id`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list lane snapshots: %w", err)
	}
	defer rows.Close()
	var out []lanes.Snapshot
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var snap lanes.Snapshot
		if err := json.Unmarshal([]byte(raw), &snap); err != nil {
			return nil, fmt.Errorf("decode lane snapshot: %w", err)
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

// Latest is the newest snapshot of a site and sensor, or nil.
func (s *LaneSnapshotStore) Latest(l16, sensorID string) (*lanes.Snapshot, error) {
	all, err := s.All(l16, sensorID)
	if err != nil || len(all) == 0 {
		return nil, err
	}
	return &all[len(all)-1], nil
}

// History is every lane version a site and sensor's snapshots hold.
func (s *LaneSnapshotStore) History(l16, sensorID string) (*lanes.History, error) {
	all, err := s.All(l16, sensorID)
	if err != nil {
		return nil, err
	}
	return lanes.NewHistory(all), nil
}
