package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// ListRecentBgSnapshots returns the last N BgSnapshots for a sensor_id, ordered by most recent.
func (db *DB) ListRecentBgSnapshots(sensorID string, limit int) ([]*l3grid.BgSnapshot, error) {
	q := `SELECT snapshot_id, sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, ring_elevations_json, grid_blob, changed_cells_count, snapshot_reason
		  FROM lidar_bg_snapshot WHERE sensor_id = ? ORDER BY snapshot_id DESC LIMIT ?`
	rows, err := db.Query(q, sensorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []*l3grid.BgSnapshot
	for rows.Next() {
		var snapID int64
		var sensor string
		var takenUnix int64
		var rings int
		var azBins int
		var paramsJSON sql.NullString
		var ringElevations sql.NullString
		var blob []byte
		var changed int
		var reason sql.NullString
		if err := rows.Scan(&snapID, &sensor, &takenUnix, &rings, &azBins, &paramsJSON, &ringElevations, &blob, &changed, &reason); err != nil {
			return nil, err
		}
		snap := &l3grid.BgSnapshot{
			SnapshotID:         &snapID,
			SensorID:           sensor,
			TakenUnixNanos:     takenUnix,
			Rings:              rings,
			AzimuthBins:        azBins,
			ParamsJSON:         paramsJSON.String,
			RingElevationsJSON: ringElevations.String,
			GridBlob:           blob,
			ChangedCellsCount:  changed,
			SnapshotReason:     reason.String,
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

// DeleteDuplicateBgSnapshots removes duplicate snapshots for a given sensor_id.
// Duplicates are defined as sharing the same grid_blob content, regardless of timestamp.
// This deduplicates history, keeping only the most recent snapshot (highest ID) for each unique grid configuration.
func (db *DB) DeleteDuplicateBgSnapshots(sensorID string) (int64, error) {
	// SQLite specific query to keep only the max rowid (snapshot_id) for each unique grid_blob.
	// We group only by grid_blob to collapse identical historical snapshots.
	q := `DELETE FROM lidar_bg_snapshot
          WHERE sensor_id = ? AND snapshot_id NOT IN (
             SELECT MAX(snapshot_id)
             FROM lidar_bg_snapshot
             WHERE sensor_id = ?
             GROUP BY grid_blob
          )`
	res, err := db.Exec(q, sensorID, sensorID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// InsertBgSnapshot persists a Background snapshot into the lidar_bg_snapshot table
// and returns the new snapshot_id.
func (db *DB) InsertBgSnapshot(s *l3grid.BgSnapshot) (int64, error) {
	if s == nil {
		return 0, nil
	}
	stmt := `INSERT INTO lidar_bg_snapshot (sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, ring_elevations_json, grid_blob, changed_cells_count, snapshot_reason)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := db.Exec(stmt, s.SensorID, s.TakenUnixNanos, s.Rings, s.AzimuthBins, s.ParamsJSON, s.RingElevationsJSON, s.GridBlob, s.ChangedCellsCount, s.SnapshotReason)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetLatestBgSnapshot returns the most recent BgSnapshot for the given sensor_id, or nil if none.
func (db *DB) GetLatestBgSnapshot(sensorID string) (*l3grid.BgSnapshot, error) {
	q := `SELECT snapshot_id, sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, ring_elevations_json, grid_blob, changed_cells_count, snapshot_reason
		  FROM lidar_bg_snapshot WHERE sensor_id = ? ORDER BY snapshot_id DESC LIMIT 1` // nolint:lll

	row := db.QueryRow(q, sensorID)
	return scanBgSnapshot(row)
}

// GetBgSnapshotByID returns a BgSnapshot by its snapshot_id, or nil if not found.
func (db *DB) GetBgSnapshotByID(snapshotID int64) (*l3grid.BgSnapshot, error) {
	if snapshotID <= 0 {
		return nil, nil
	}
	q := `SELECT snapshot_id, sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, ring_elevations_json, grid_blob, changed_cells_count, snapshot_reason
		  FROM lidar_bg_snapshot WHERE snapshot_id = ?` // nolint:lll

	row := db.QueryRow(q, snapshotID)
	return scanBgSnapshot(row)
}

// scanBgSnapshot scans a row into a BgSnapshot struct.
func scanBgSnapshot(row *sql.Row) (*l3grid.BgSnapshot, error) {
	var snapID int64
	var sensor string
	var takenUnix int64
	var rings int
	var azBins int
	var paramsJSON sql.NullString
	var ringElevations sql.NullString
	var blob []byte
	var changed int
	var reason sql.NullString

	if err := row.Scan(&snapID, &sensor, &takenUnix, &rings, &azBins, &paramsJSON, &ringElevations, &blob, &changed, &reason); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	snap := &l3grid.BgSnapshot{
		SnapshotID:         &snapID,
		SensorID:           sensor,
		TakenUnixNanos:     takenUnix,
		Rings:              rings,
		AzimuthBins:        azBins,
		ParamsJSON:         paramsJSON.String,
		RingElevationsJSON: ringElevations.String,
		GridBlob:           blob,
		ChangedCellsCount:  changed,
		SnapshotReason:     reason.String,
	}
	return snap, nil
}

// DuplicateSnapshotGroup represents a group of snapshots with the same grid_blob hash.
type DuplicateSnapshotGroup struct {
	BlobHash    string  // hex-encoded hash of grid_blob
	Count       int     // number of snapshots with this hash
	SnapshotIDs []int64 // list of snapshot IDs with this hash
	KeepID      int64   // the snapshot ID to keep (most recent)
	DeleteIDs   []int64 // snapshot IDs that would be deleted
	BlobBytes   int     // size of the blob in bytes
	SensorID    string  // sensor ID for this group
}

// CountUniqueBgSnapshotHashes counts the total number of unique grid_blob hashes
// for a sensor, including both duplicates and singletons.
func (db *DB) CountUniqueBgSnapshotHashes(sensorID string) (int, error) {
	q := `SELECT grid_blob
		  FROM lidar_bg_snapshot
		  WHERE sensor_id = ?`

	rows, err := db.Query(q, sensorID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	hashSet := make(map[string]struct{})
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return 0, err
		}
		h := sha256.Sum256(blob)
		hashHex := hex.EncodeToString(h[:])
		hashSet[hashHex] = struct{}{}
	}

	return len(hashSet), nil
}

// FindDuplicateBgSnapshots finds groups of snapshots with identical grid_blob data.
// Returns groups where Count > 1 (i.e., duplicates exist).
func (db *DB) FindDuplicateBgSnapshots(sensorID string) ([]DuplicateSnapshotGroup, error) {
	// SQLite doesn't have a native hash function, so we'll do this in Go
	// First, get all snapshots for this sensor
	q := `SELECT snapshot_id, grid_blob
		  FROM lidar_bg_snapshot
		  WHERE sensor_id = ?
		  ORDER BY snapshot_id ASC`

	rows, err := db.Query(q, sensorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Group by blob hash
	type snapshotInfo struct {
		id       int64
		blobSize int
	}
	hashGroups := make(map[string][]snapshotInfo)

	for rows.Next() {
		var snapID int64
		var blob []byte
		if err := rows.Scan(&snapID, &blob); err != nil {
			return nil, err
		}

		// Compute hash of the blob
		h := sha256.Sum256(blob)
		hashHex := hex.EncodeToString(h[:])

		hashGroups[hashHex] = append(hashGroups[hashHex], snapshotInfo{
			id:       snapID,
			blobSize: len(blob),
		})
	}

	// Convert to result format, filtering for duplicates only
	var result []DuplicateSnapshotGroup
	for hash, infos := range hashGroups {
		if len(infos) <= 1 {
			continue // No duplicates
		}

		ids := make([]int64, len(infos))
		for i, info := range infos {
			ids[i] = info.id
		}

		// Keep the most recent snapshot to match DeleteDuplicateBgSnapshots.
		keepID := ids[len(ids)-1]
		deleteIDs := ids[:len(ids)-1]

		result = append(result, DuplicateSnapshotGroup{
			BlobHash:    hash,
			Count:       len(infos),
			SnapshotIDs: ids,
			KeepID:      keepID,
			DeleteIDs:   deleteIDs,
			BlobBytes:   infos[0].blobSize,
			SensorID:    sensorID,
		})
	}

	return result, nil
}

// DeleteBgSnapshots deletes snapshots by their IDs. Returns the number of rows deleted.
func (db *DB) DeleteBgSnapshots(snapshotIDs []int64) (int64, error) {
	if len(snapshotIDs) == 0 {
		return 0, nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(snapshotIDs))
	args := make([]interface{}, len(snapshotIDs))
	for i, id := range snapshotIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	q := fmt.Sprintf("DELETE FROM lidar_bg_snapshot WHERE snapshot_id IN (%s)",
		strings.Join(placeholders, ","))

	res, err := db.Exec(q, args...)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}
