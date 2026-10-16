package db

import (
	"database/sql"

	_ "modernc.org/sqlite"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// InsertRegionSnapshot persists a region snapshot into the lidar_bg_regions table
// and returns the new region_set_id.
func (db *DB) InsertRegionSnapshot(s *l3grid.RegionSnapshot) (int64, error) {
	if s == nil {
		return 0, nil
	}
	stmt := `INSERT INTO lidar_bg_regions (snapshot_id, sensor_id, created_unix_nanos, region_count, regions_json, variance_data_json, settling_frames, grid_hash, source_path)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := db.Exec(stmt, s.SnapshotID, s.SensorID, s.CreatedUnixNanos, s.RegionCount, s.RegionsJSON, s.VarianceDataJSON, s.SettlingFrames, s.GridHash, s.SourcePath)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetRegionSnapshotByGridHash returns a region snapshot matching the given grid hash, or nil if none.
// This is used to restore regions when processing a PCAP that matches a previously seen scene.
func (db *DB) GetRegionSnapshotByGridHash(sensorID, gridHash string) (*l3grid.RegionSnapshot, error) {
	if gridHash == "" {
		return nil, nil
	}
	q := `SELECT region_set_id, snapshot_id, sensor_id, created_unix_nanos, region_count, regions_json, variance_data_json, settling_frames, grid_hash, source_path
		  FROM lidar_bg_regions WHERE sensor_id = ? AND grid_hash = ? ORDER BY region_set_id DESC LIMIT 1`

	row := db.QueryRow(q, sensorID, gridHash)
	return scanRegionSnapshot(row)
}

// GetRegionSnapshotBySourcePath returns a region snapshot matching the given source path (e.g., PCAP filename), or nil if none.
// This is the preferred method for PCAP restoration as source path matching is more reliable than scene hash during early settling.
func (db *DB) GetRegionSnapshotBySourcePath(sensorID, sourcePath string) (*l3grid.RegionSnapshot, error) {
	if sourcePath == "" {
		return nil, nil
	}
	q := `SELECT region_set_id, snapshot_id, sensor_id, created_unix_nanos, region_count, regions_json, variance_data_json, settling_frames, grid_hash, source_path
		  FROM lidar_bg_regions WHERE sensor_id = ? AND source_path = ? ORDER BY region_set_id DESC LIMIT 1`

	row := db.QueryRow(q, sensorID, sourcePath)
	return scanRegionSnapshot(row)
}

// GetLatestRegionSnapshot returns the most recent region snapshot for the given sensor_id, or nil if none.
func (db *DB) GetLatestRegionSnapshot(sensorID string) (*l3grid.RegionSnapshot, error) {
	q := `SELECT region_set_id, snapshot_id, sensor_id, created_unix_nanos, region_count, regions_json, variance_data_json, settling_frames, grid_hash, source_path
		  FROM lidar_bg_regions WHERE sensor_id = ? ORDER BY region_set_id DESC LIMIT 1`

	row := db.QueryRow(q, sensorID)
	return scanRegionSnapshot(row)
}

// scanRegionSnapshot scans a row into a RegionSnapshot struct.
func scanRegionSnapshot(row *sql.Row) (*l3grid.RegionSnapshot, error) {
	var regionSetID int64
	var snapshotID int64
	var sensor string
	var createdUnix int64
	var regionCount int
	var regionsJSON string
	var varianceJSON sql.NullString
	var settlingFrames sql.NullInt64
	var gridHash sql.NullString
	var sourcePath sql.NullString

	if err := row.Scan(&regionSetID, &snapshotID, &sensor, &createdUnix, &regionCount, &regionsJSON, &varianceJSON, &settlingFrames, &gridHash, &sourcePath); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	snap := &l3grid.RegionSnapshot{
		RegionSetID:      &regionSetID,
		SnapshotID:       snapshotID,
		SensorID:         sensor,
		CreatedUnixNanos: createdUnix,
		RegionCount:      regionCount,
		RegionsJSON:      regionsJSON,
		VarianceDataJSON: varianceJSON.String,
		SettlingFrames:   int(settlingFrames.Int64),
		GridHash:         gridHash.String,
		SourcePath:       sourcePath.String,
	}
	return snap, nil
}

// GetRegionSnapshotByID returns the region snapshot with the given
// region_set_id, or nil if there is none.
func (db *DB) GetRegionSnapshotByID(regionSetID int64) (*l3grid.RegionSnapshot, error) {
	q := `SELECT region_set_id, snapshot_id, sensor_id, created_unix_nanos, region_count, regions_json, variance_data_json, settling_frames, grid_hash, source_path
		  FROM lidar_bg_regions WHERE region_set_id = ?`

	row := db.QueryRow(q, regionSetID)
	return scanRegionSnapshot(row)
}

// RegionSnapshotSummary is one lidar_bg_regions row without its JSON
// payloads, with the reason recorded on the grid snapshot it links to.
type RegionSnapshotSummary struct {
	RegionSetID      int64
	SnapshotID       int64
	SensorID         string
	CreatedUnixNanos int64
	RegionCount      int
	SettlingFrames   int
	SourcePath       string
	SnapshotReason   string
}

// ListRegionSnapshots lists every region snapshot in region_set_id order,
// without reading the cell lists or the per-cell variance.
func (db *DB) ListRegionSnapshots() ([]RegionSnapshotSummary, error) {
	rows, err := db.Query(`SELECT r.region_set_id, COALESCE(r.snapshot_id, 0), r.sensor_id, r.created_unix_nanos,
		  r.region_count, r.settling_frames, COALESCE(r.source_path, ''), COALESCE(s.snapshot_reason, '')
		  FROM lidar_bg_regions r LEFT JOIN lidar_bg_snapshot s ON s.snapshot_id = r.snapshot_id
		  ORDER BY r.region_set_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RegionSnapshotSummary
	for rows.Next() {
		var s RegionSnapshotSummary
		if err := rows.Scan(&s.RegionSetID, &s.SnapshotID, &s.SensorID, &s.CreatedUnixNanos,
			&s.RegionCount, &s.SettlingFrames, &s.SourcePath, &s.SnapshotReason); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
