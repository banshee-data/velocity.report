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
