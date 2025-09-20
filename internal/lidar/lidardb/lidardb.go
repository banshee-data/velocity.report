package lidardb

import (
	"database/sql"
	_ "embed"
	"log"

	lidar "github.com/banshee-data/velocity.report/internal/lidar"

	_ "modernc.org/sqlite"
)

type LidarDB struct {
	*sql.DB
}

// schema.sql contains the SQL statements for creating the lidar database schema.
// It defines tables for storing lidar packets, extracted point data, and session information.
//
//go:embed schema.sql
var schemaSQL string

func NewLidarDB(path string) (*LidarDB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(schemaSQL)
	if err != nil {
		return nil, err
	}

	log.Println("initialized lidar database schema")

	return &LidarDB{db}, nil
}

// InsertBgSnapshot persists a Background snapshot into the lidar_bg_snapshot table
// and returns the new snapshot_id.
func (ldb *LidarDB) InsertBgSnapshot(s *lidar.BgSnapshot) (int64, error) {
	if s == nil {
		return 0, nil
	}
	stmt := `INSERT INTO lidar_bg_snapshot (sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, grid_blob, changed_cells_count, snapshot_reason)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := ldb.Exec(stmt, s.SensorID, s.TakenUnixNanos, s.Rings, s.AzimuthBins, s.ParamsJSON, s.GridBlob, s.ChangedCellsCount, s.SnapshotReason)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetLatestBgSnapshot returns the most recent BgSnapshot for the given sensor_id, or nil if none.
func (ldb *LidarDB) GetLatestBgSnapshot(sensorID string) (*lidar.BgSnapshot, error) {
	q := `SELECT snapshot_id, sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, grid_blob, changed_cells_count, snapshot_reason
		  FROM lidar_bg_snapshot WHERE sensor_id = ? ORDER BY snapshot_id DESC LIMIT 1` // nolint:lll

	row := ldb.QueryRow(q, sensorID)
	var snapID int64
	var sensor string
	var takenUnix int64
	var rings int
	var azBins int
	var paramsJSON sql.NullString
	var blob []byte
	var changed int
	var reason sql.NullString

	if err := row.Scan(&snapID, &sensor, &takenUnix, &rings, &azBins, &paramsJSON, &blob, &changed, &reason); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	snap := &lidar.BgSnapshot{
		SnapshotID:        &snapID,
		SensorID:          sensor,
		TakenUnixNanos:    takenUnix,
		Rings:             rings,
		AzimuthBins:       azBins,
		ParamsJSON:        paramsJSON.String,
		GridBlob:          blob,
		ChangedCellsCount: changed,
		SnapshotReason:    reason.String,
	}
	return snap, nil
}
