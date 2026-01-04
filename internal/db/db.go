package db

import (
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/tailscale/tailsql/server/tailsql"
	_ "modernc.org/sqlite"
	"tailscale.com/tsweb"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"gonum.org/v1/gonum/stat"
)

// compile-time assertion: ensure DB implements lidar.BgStore (InsertBgSnapshot)
var _ lidar.BgStore = (*DB)(nil)

type DB struct {
	*sql.DB
}

// ListRecentBgSnapshots returns the last N BgSnapshots for a sensor_id, ordered by most recent.
func (db *DB) ListRecentBgSnapshots(sensorID string, limit int) ([]*lidar.BgSnapshot, error) {
	q := `SELECT snapshot_id, sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, ring_elevations_json, grid_blob, changed_cells_count, snapshot_reason
		  FROM lidar_bg_snapshot WHERE sensor_id = ? ORDER BY snapshot_id DESC LIMIT ?`
	rows, err := db.Query(q, sensorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []*lidar.BgSnapshot
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
		snap := &lidar.BgSnapshot{
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

// schema.sql contains the SQL statements for creating the database schema.
// It defines tables such as radar_data, radar_objects, radar_commands, and radar_command_log which store radar event and command information.
// The schema is embedded directly into the binary and executed when a new database is created
// via the NewDB function, ensuring consistent schema across all deployments.
//
// CRITICAL: schema.sql MUST be kept in sync with the latest migration version.
// When creating a fresh database, we verify that schema.sql matches the schema produced
// by applying all migrations. If they differ, database initialization fails with a clear
// error message. This prevents silently creating databases with incomplete schemas.
// To regenerate schema.sql from migrations, export the schema from a migrated database:
//   sqlite3 migrated.db .schema > internal/db/schema.sql

//go:embed schema.sql
var schemaSQL string

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DevMode controls whether to use filesystem or embedded migrations.
// Set to true in development for hot-reloading, false in production.
var DevMode = false

// getMigrationsFS returns the appropriate filesystem for migrations.
// In dev mode, uses the local filesystem for hot-reloading.
// In production, uses the embedded filesystem.
func getMigrationsFS() (fs.FS, error) {
	if DevMode {
		// Development: use local filesystem
		return os.DirFS("internal/db/migrations"), nil
	}
	// Production: use embedded filesystem
	// The embed directive includes "migrations/*.sql", so we need to extract just the migrations subdir
	subFS, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to create sub-filesystem for embedded migrations directory %q: %w", "migrations", err)
	}
	return subFS, nil
}

// applyPragmas applies essential SQLite PRAGMAs for performance and concurrency.
// These settings are extracted from schema.sql and applied to all databases
// regardless of whether they were created from scratch or via migrations.
func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA busy_timeout = 5000",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute %q: %w", pragma, err)
		}
	}

	return nil
}

func NewDB(path string) (*DB, error) {
	return NewDBWithMigrationCheck(path, true)
}

// NewDBWithMigrationCheck opens a database and optionally checks for pending migrations.
// If checkMigrations is true and migrations are pending, returns an error prompting user to run migrations.
func NewDBWithMigrationCheck(path string, checkMigrations bool) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	dbWrapper := &DB{db}

	// Apply essential PRAGMAs for all databases, regardless of how they were created.
	// These settings are critical for performance and concurrency:
	// - WAL mode allows concurrent reads and writes
	// - busy_timeout prevents immediate "database is locked" errors
	// - NORMAL synchronous mode balances safety and performance
	// - MEMORY temp_store improves query performance
	if err := applyPragmas(db); err != nil {
		return nil, fmt.Errorf("failed to apply PRAGMAs: %w", err)
	}

	// Check if schema_migrations table exists
	var schemaMigrationsExists bool
	err = db.QueryRow(`
		SELECT COUNT(*) > 0
		FROM sqlite_master
		WHERE type='table' AND name='schema_migrations'
	`).Scan(&schemaMigrationsExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check for schema_migrations table: %w", err)
	}

	// Get migrations filesystem
	migrationsFS, err := getMigrationsFS()
	if err != nil {
		return nil, fmt.Errorf("failed to get migrations filesystem: %w", err)
	}

	// Case 1: Database with migration history - check if migrations are needed
	if schemaMigrationsExists {
		if checkMigrations {
			shouldExit, err := dbWrapper.CheckAndPromptMigrations(migrationsFS)
			if shouldExit {
				return nil, err
			}
		}
		return dbWrapper, nil
	}

	// Case 2: Database without schema_migrations table
	// Check if this is a legacy database (has tables) or a fresh database
	var tableCount int
	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
	`).Scan(&tableCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count tables: %w", err)
	}

	isLegacyDB := (tableCount > 0)

	// Case 2a: Legacy database without migration history - detect and baseline
	if isLegacyDB && checkMigrations {
		log.Printf("⚠️  Database exists but has no schema_migrations table!")
		log.Printf("   Attempting to detect schema version...")

		detectedVersion, matchScore, differences, err := dbWrapper.DetectSchemaVersion(migrationsFS)
		if err != nil {
			return nil, fmt.Errorf("failed to detect schema version: %w", err)
		}

		log.Printf("   Schema detection results:")
		log.Printf("   - Best match: version %d (score: %d%%)", detectedVersion, matchScore)

		if matchScore == 100 {
			// Perfect match - baseline at this version
			log.Printf("   - Perfect match! Baselining at version %d", detectedVersion)
			if err := dbWrapper.BaselineAtVersion(detectedVersion); err != nil {
				return nil, fmt.Errorf("failed to baseline at version %d: %w", detectedVersion, err)
			}

			// Check if more migrations are needed
			latestVersion, err := GetLatestMigrationVersion(migrationsFS)
			if err != nil {
				return nil, fmt.Errorf("failed to get latest version: %w", err)
			}

			if detectedVersion < latestVersion {
				log.Printf("")
				log.Printf("   Database has been baselined at version %d", detectedVersion)
				log.Printf("   There are %d additional migrations available (up to version %d)",
					latestVersion-detectedVersion, latestVersion)
				log.Printf("")
				log.Printf("   To apply remaining migrations, run:")
				log.Printf("      velocity-report migrate up")
				log.Printf("")
				return nil, fmt.Errorf("database baselined at version %d, but migrations to version %d are available. Please run migrations", detectedVersion, latestVersion)
			}

			log.Printf("   Database is up to date!")
			return dbWrapper, nil
		}

		// Not a perfect match - show differences and ask user
		log.Printf("   - No perfect match found (best: %d%%)", matchScore)
		log.Printf("")
		log.Printf("   Schema differences from version %d:", detectedVersion)
		for _, diff := range differences {
			log.Printf("     %s", diff)
		}
		log.Printf("")
		log.Printf("   The current schema does not exactly match any known migration version.")
		log.Printf("   Closest match is version %d with %d%% similarity.", detectedVersion, matchScore)
		log.Printf("")
		log.Printf("   Options:")
		log.Printf("   1. Baseline at version %d and apply remaining migrations:", detectedVersion)
		log.Printf("      velocity-report migrate baseline %d", detectedVersion)
		log.Printf("      velocity-report migrate up")
		log.Printf("")
		log.Printf("   2. Manually inspect the differences and adjust your schema")
		log.Printf("")
		return nil, fmt.Errorf("schema does not match any known version (best match: v%d at %d%%). Manual intervention required", detectedVersion, matchScore)
	}

	// Case 2b: Fresh database - initialize with schema.sql and baseline at latest version
	_, err = db.Exec(schemaSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
	}

	log.Println("ran database initialisation script")

	// Get latest migration version
	latestVersion, err := GetLatestMigrationVersion(migrationsFS)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest migration version: %w", err)
	}

	// Verify that schema.sql is in sync with the latest migration version
	// by comparing the schema we just created with what the migrations would produce.
	// This prevents incorrect baselining if schema.sql is out of date.
	schemaFromSQL, err := dbWrapper.GetDatabaseSchema()
	if err != nil {
		return nil, fmt.Errorf("failed to get schema from schema.sql: %w", err)
	}

	schemaFromMigrations, err := dbWrapper.GetSchemaAtMigration(migrationsFS, latestVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema at migration v%d: %w", latestVersion, err)
	}

	score, differences := CompareSchemas(schemaFromSQL, schemaFromMigrations)
	if score != 100 {
		log.Printf("⚠️  WARNING: schema.sql is out of sync with migrations!")
		log.Printf("   Schema from schema.sql differs from migration v%d (similarity: %d%%)", latestVersion, score)
		log.Printf("   Differences:")
		for _, diff := range differences {
			log.Printf("     %s", diff)
		}
		log.Printf("")
		log.Printf("   This indicates that schema.sql needs to be updated to match the latest migrations.")
		log.Printf("   Please run the schema consistency test or regenerate schema.sql from migrations.")
		log.Printf("")
		return nil, fmt.Errorf("schema.sql is out of sync with migration v%d (similarity: %d%%). Cannot baseline safely", latestVersion, score)
	}

	// Schema is consistent - safe to baseline at latest version
	if err := dbWrapper.BaselineAtVersion(latestVersion); err != nil {
		return nil, fmt.Errorf("failed to baseline fresh database at version %d: %w", latestVersion, err)
	}

	// Verify baseline was successful
	currentVersion, _, err := dbWrapper.MigrateVersion(migrationsFS)
	if err != nil {
		return nil, fmt.Errorf("failed to verify baseline: %w", err)
	}
	if currentVersion != latestVersion {
		return nil, fmt.Errorf("baseline verification failed: expected version %d, got %d", latestVersion, currentVersion)
	}

	return dbWrapper, nil
}

// OpenDB opens a database connection without running schema initialization.
// This is useful for migration commands that manage schema independently.
// Note: PRAGMAs are still applied for performance and concurrency.
func OpenDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// Apply PRAGMAs even for migration commands
	if err := applyPragmas(db); err != nil {
		return nil, fmt.Errorf("failed to apply PRAGMAs: %w", err)
	}

	return &DB{db}, nil
}

func (db *DB) RecordRadarObject(rawRadarJSON string) error {
	var err error
	if rawRadarJSON == "" {
		return fmt.Errorf("rawRadarJSON cannot be empty")
	}

	_, err = db.Exec(
		`INSERT INTO radar_objects (raw_event) VALUES (?)`, rawRadarJSON,
	)
	if err != nil {
		return err
	}
	return nil
}

// InsertBgSnapshot persists a Background snapshot into the lidar_bg_snapshot table
// and returns the new snapshot_id.
func (db *DB) InsertBgSnapshot(s *lidar.BgSnapshot) (int64, error) {
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
func (db *DB) GetLatestBgSnapshot(sensorID string) (*lidar.BgSnapshot, error) {
	q := `SELECT snapshot_id, sensor_id, taken_unix_nanos, rings, azimuth_bins, params_json, ring_elevations_json, grid_blob, changed_cells_count, snapshot_reason
		  FROM lidar_bg_snapshot WHERE sensor_id = ? ORDER BY snapshot_id DESC LIMIT 1` // nolint:lll

	row := db.QueryRow(q, sensorID)
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

	snap := &lidar.BgSnapshot{
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
	KeepID      int64   // the snapshot ID to keep (oldest)
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

		// Keep the oldest (first) snapshot
		keepID := ids[0]
		deleteIDs := ids[1:]

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

type RadarObject struct {
	Classifier   string
	StartTime    time.Time
	EndTime      time.Time
	DeltaTimeMs  int64
	MaxSpeed     float64
	MinSpeed     float64
	SpeedChange  float64
	MaxMagnitude int64
	AvgMagnitude int64
	TotalFrames  int64
	FramesPerMps float64
	Length       float64
}

func (e *RadarObject) String() string {
	return fmt.Sprintf(
		"Classifier: %s, StartTime: %s, EndTime: %s, DeltaTimeMs: %d, MaxSpeed: %f, MinSpeed: %f, SpeedChange: %f, MaxMagnitude: %d, AvgMagnitude: %d, TotalFrames: %d, FramesPerMps: %f, Length: %f",
		e.Classifier,
		e.StartTime,
		e.EndTime,
		e.DeltaTimeMs,
		e.MaxSpeed,
		e.MinSpeed,
		e.SpeedChange,
		e.MaxMagnitude,
		e.AvgMagnitude,
		e.TotalFrames,
		e.FramesPerMps,
		e.Length,
	)
}

func (db *DB) RadarObjects() ([]RadarObject, error) {
	rows, err := db.Query(`SELECT classifier, start_time, end_time, delta_time_ms, max_speed, min_speed,
			speed_change, max_magnitude, avg_magnitude, total_frames,
			frames_per_mps, length_m FROM radar_objects ORDER BY write_timestamp DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var radar_objects []RadarObject
	for rows.Next() {
		var r RadarObject

		var startTimeFloat, endTimeFloat float64

		if err := rows.Scan(
			&r.Classifier,
			&startTimeFloat,
			&endTimeFloat,
			&r.DeltaTimeMs,
			&r.MaxSpeed,
			&r.MinSpeed,
			&r.SpeedChange,
			&r.MaxMagnitude,
			&r.AvgMagnitude,
			&r.TotalFrames,
			&r.FramesPerMps,
			&r.Length,
		); err != nil {
			return nil, err
		}

		// Convert float values to seconds and nanoseconds
		startTimeSeconds := int64(startTimeFloat)
		startTimeNanos := int64(math.Round((startTimeFloat-float64(startTimeSeconds))*1e6) * 1e3) // Round to microseconds, then convert to nanoseconds
		endTimeSeconds := int64(endTimeFloat)
		endTimeNanos := int64(math.Round((endTimeFloat-float64(endTimeSeconds))*1e6) * 1e3) // Round to microseconds, then convert to nanoseconds

		// Assign the converted times to the RadarObject
		r.StartTime = time.Unix(startTimeSeconds, startTimeNanos).UTC()
		r.EndTime = time.Unix(endTimeSeconds, endTimeNanos).UTC()

		radar_objects = append(radar_objects, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return radar_objects, nil
}

// RadarObjectsRollupRow represents an aggregate row for radar object rollup.
type RadarObjectsRollupRow struct {
	Classifier string
	StartTime  time.Time
	Count      int64
	P50Speed   float64
	P85Speed   float64
	P98Speed   float64
	MaxSpeed   float64
}

func (e *RadarObjectsRollupRow) String() string {
	return fmt.Sprintf(
		"Classifier: %s, StartTime: %s, Count: %d, P50Speed: %f, P85Speed: %f, P98Speed: %f, MaxSpeed: %f",
		e.Classifier,
		e.StartTime,
		e.Count,
		e.P50Speed,
		e.P85Speed,
		e.P98Speed,
		e.MaxSpeed,
	)
}

// RadarStatsResult combines time-aggregated metrics with an optional histogram.
type RadarStatsResult struct {
	Metrics   []RadarObjectsRollupRow
	Histogram map[float64]int64 // bucket start (mps) -> count; nil if histogram not requested
}

// RadarObjectRollupRange aggregates all radar_objects into time buckets and optionally computes a histogram.
// dataSource may be either "radar_objects" (default) or "radar_data_transits".
// If histBucketSize > 0, a histogram is computed; histMax (if > 0) clips histogram values above that threshold.
// Both histBucketSize and histMax are in meters-per-second (mps).
func (db *DB) RadarObjectRollupRange(startUnix, endUnix, groupSeconds int64, minSpeed float64, dataSource string, modelVersion string, histBucketSize, histMax float64) (*RadarStatsResult, error) {
	if endUnix <= startUnix {
		return nil, fmt.Errorf("end must be greater than start")
	}
	// groupSeconds == 0 is allowed and treated as the 'all' aggregation (single bucket).
	if groupSeconds < 0 {
		return nil, fmt.Errorf("groupSeconds must be non-negative")
	}

	// default minimum speed (meters per second) if caller passes 0
	if minSpeed <= 0 {
		minSpeed = 2.2352 // 2.2352 mps ≈ 5 mph
	}

	// default data source
	if dataSource == "" {
		dataSource = "radar_objects"
	}

	var rows *sql.Rows
	var err error
	switch dataSource {
	case "radar_objects":
		rows, err = db.Query(`SELECT write_timestamp, max_speed FROM radar_objects WHERE max_speed > ? AND write_timestamp BETWEEN ? AND ?`, minSpeed, startUnix, endUnix)
	case "radar_data_transits":
		// radar_data_transits stores transit_start_unix and transit_max_speed
		if modelVersion == "" {
			modelVersion = "rebuild-full"
		}
		rows, err = db.Query(`SELECT transit_start_unix, transit_max_speed FROM radar_data_transits WHERE model_version = ? AND transit_max_speed > ? AND transit_start_unix BETWEEN ? AND ?`, modelVersion, minSpeed, startUnix, endUnix)
	default:
		return nil, fmt.Errorf("unsupported dataSource: %s", dataSource)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// map: bucketStart -> []speeds
	buckets := make(map[int64][]float64)
	// track max speed per bucket
	bucketMax := make(map[int64]float64)
	// collect all speeds for histogram (if requested)
	var allSpeedsForHist []float64
	if histBucketSize > 0 {
		allSpeedsForHist = make([]float64, 0)
	}

	// Special-case: groupSeconds == 0 means 'all' -- aggregate all rows into a single bucket.
	if groupSeconds == 0 {
		var allSpeeds []float64
		var allMax float64
		var minTs int64 = 0
		for rows.Next() {
			var tsFloat float64
			var spd float64
			if err := rows.Scan(&tsFloat, &spd); err != nil {
				return nil, err
			}
			ts := int64(math.Round(tsFloat))
			allSpeeds = append(allSpeeds, spd)
			if histBucketSize > 0 {
				allSpeedsForHist = append(allSpeedsForHist, spd)
			}
			if allMax == 0 || spd > allMax {
				allMax = spd
			}
			if minTs == 0 || ts < minTs {
				minTs = ts
			}
		}

		// Determine bucket start: midnight (00:00:00) UTC of minTs (or startUnix if no rows)
		var bucketStart int64
		if minTs == 0 {
			bucketStart = time.Unix(startUnix, 0).UTC().Truncate(24 * time.Hour).Unix()
		} else {
			bucketStart = time.Unix(minTs, 0).UTC().Truncate(24 * time.Hour).Unix()
		}

		if len(allSpeeds) > 0 {
			buckets[bucketStart] = allSpeeds
			bucketMax[bucketStart] = allMax
		}
	} else {
		for rows.Next() {
			var tsFloat float64
			var spd float64
			if err := rows.Scan(&tsFloat, &spd); err != nil {
				return nil, err
			}
			ts := int64(math.Round(tsFloat))

			// compute bucket start aligned to startUnix
			offset := ts - startUnix
			if offset < 0 {
				offset = 0
			}
			bucketOffset := (offset / groupSeconds) * groupSeconds
			bucketStart := startUnix + bucketOffset

			buckets[bucketStart] = append(buckets[bucketStart], spd)
			if histBucketSize > 0 {
				allSpeedsForHist = append(allSpeedsForHist, spd)
			}
			if curr, ok := bucketMax[bucketStart]; !ok || spd > curr {
				bucketMax[bucketStart] = spd
			}
		}
	}

	aggregated := []RadarObjectsRollupRow{}

	// collect and sort bucket starts
	keys := make([]int64, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, bucketStart := range keys {
		speeds := buckets[bucketStart]

		agg := RadarObjectsRollupRow{
			Classifier: "all",
			StartTime:  time.Unix(bucketStart, 0).UTC(),
		}

		if len(speeds) > 0 {
			agg.MaxSpeed = bucketMax[bucketStart]
			agg.Count = int64(len(speeds))

			sorted := make([]float64, len(speeds))
			copy(sorted, speeds)
			sort.Float64s(sorted)

			agg.P50Speed = stat.Quantile(0.5, stat.Empirical, sorted, nil)
			agg.P85Speed = stat.Quantile(0.85, stat.Empirical, sorted, nil)
			agg.P98Speed = stat.Quantile(0.98, stat.Empirical, sorted, nil)
		} else {
			agg.MaxSpeed = 0
			agg.Count = 0
			agg.P50Speed = 0
			agg.P85Speed = 0
			agg.P98Speed = 0
		}

		aggregated = append(aggregated, agg)
	}

	// Compute histogram if requested
	var histogram map[float64]int64
	if histBucketSize > 0 && len(allSpeedsForHist) > 0 {
		histogram = make(map[float64]int64)
		for _, spd := range allSpeedsForHist {
			// skip values above histMax if a max was provided
			if histMax > 0 && spd > histMax {
				continue
			}
			// compute bin start aligned to histBucketSize
			binIdx := math.Floor(spd / histBucketSize)
			binStart := binIdx * histBucketSize
			histogram[binStart] = histogram[binStart] + 1
		}
	}

	return &RadarStatsResult{
		Metrics:   aggregated,
		Histogram: histogram,
	}, nil
}

func (db *DB) RecordRawData(rawDataJSON string) error {
	var err error
	if rawDataJSON == "" {
		return fmt.Errorf("rawDataJSON cannot be empty")
	}

	_, err = db.Exec(`INSERT INTO radar_data (raw_event) VALUES (?)`, rawDataJSON)
	if err != nil {
		return err
	}
	return nil
}

type Event struct {
	Magnitude sql.NullFloat64
	Uptime    sql.NullFloat64
	Speed     sql.NullFloat64
}

func (e *Event) String() string {
	return fmt.Sprintf("Uptime: %f, Magnitude: %f, Speed: %f", e.Uptime.Float64, e.Magnitude.Float64, e.Speed.Float64)
}

type EventAPI struct {
	Magnitude *float64 `json:"Magnitude,omitempty"`
	Uptime    *float64 `json:"Uptime,omitempty"`
	Speed     *float64 `json:"Speed,omitempty"`
}

func EventToAPI(e Event) EventAPI {
	var mag, up, spd *float64
	if e.Magnitude.Valid {
		mag = &e.Magnitude.Float64
	}
	if e.Uptime.Valid {
		up = &e.Uptime.Float64
	}
	if e.Speed.Valid {
		spd = &e.Speed.Float64
	}
	return EventAPI{
		Magnitude: mag,
		Uptime:    up,
		Speed:     spd,
	}
}

func (db *DB) Events() ([]Event, error) {
	rows, err := db.Query("SELECT uptime, magnitude, speed FROM radar_data ORDER BY uptime DESC LIMIT 500")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var uptime, magnitude, speed sql.NullFloat64
		if err := rows.Scan(&uptime, &magnitude, &speed); err != nil {
			return nil, err
		}
		events = append(events, Event{
			Uptime:    uptime,
			Magnitude: magnitude,
			Speed:     speed,
		},
		)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

// TableStats contains size and row count information for a database table.
type TableStats struct {
	Name     string  `json:"name"`
	RowCount int64   `json:"row_count"`
	SizeMB   float64 `json:"size_mb"`
}

// DatabaseStats contains overall database statistics.
type DatabaseStats struct {
	TotalSizeMB float64      `json:"total_size_mb"`
	Tables      []TableStats `json:"tables"`
}

// GetDatabaseStats returns size and row count information for all tables in the database.
// Uses SQLite's dbstat virtual table to get accurate size information.
func (db *DB) GetDatabaseStats() (*DatabaseStats, error) {
	// Get total database size using page_count * page_size
	var totalPages, pageSize int64
	row := db.QueryRow("SELECT page_count, page_size FROM pragma_page_count(), pragma_page_size()")
	if err := row.Scan(&totalPages, &pageSize); err != nil {
		// Fallback: try individual pragmas
		if err := db.QueryRow("PRAGMA page_count").Scan(&totalPages); err != nil {
			return nil, fmt.Errorf("failed to get page count: %w", err)
		}
		if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
			return nil, fmt.Errorf("failed to get page size: %w", err)
		}
	}
	totalSizeMB := float64(totalPages*pageSize) / (1024 * 1024)

	// Get list of tables
	tablesQuery := `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`
	rows, err := db.Query(tablesQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}
	defer rows.Close()

	var tableNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}
		tableNames = append(tableNames, name)
	}

	// Get stats for each table
	var tables []TableStats
	for _, tableName := range tableNames {
		var rowCount int64
		// Build the COUNT(*) query dynamically with a quoted table name.
		// SQL/SQLite prepared statements only parameterize values, not identifiers,
		// so table names cannot be bound as parameters. Here tableName comes from
		// sqlite_master (trusted metadata), and %q applies proper SQLite identifier
		// quoting, so this is not a SQL injection risk.
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %q", tableName)
		if err := db.QueryRow(countQuery).Scan(&rowCount); err != nil {
			// Table might be empty or have issues, continue with 0
			rowCount = 0
		}

		// Get size using dbstat virtual table (if available)
		var sizeMB float64
		sizeQuery := `SELECT COALESCE(SUM(pgsize), 0) / 1048576.0 FROM dbstat WHERE name = ?`
		if err := db.QueryRow(sizeQuery, tableName).Scan(&sizeMB); err != nil {
			// dbstat might not be available, estimate from row count
			sizeMB = 0
		}

		tables = append(tables, TableStats{
			Name:     tableName,
			RowCount: rowCount,
			SizeMB:   math.Round(sizeMB*100) / 100, // Round to 2 decimal places
		})
	}

	// Sort tables by size descending
	sort.Slice(tables, func(i, j int) bool {
		return tables[i].SizeMB > tables[j].SizeMB
	})

	return &DatabaseStats{
		TotalSizeMB: math.Round(totalSizeMB*100) / 100,
		Tables:      tables,
	}, nil
}

func (db *DB) AttachAdminRoutes(mux *http.ServeMux) {
	debug := tsweb.Debugger(mux)
	// create a tailSQL instance and point it to our DB
	tsql, err := tailsql.NewServer(tailsql.Options{
		RoutePrefix: "/debug/tailsql/",
	})
	if err != nil {
		log.Fatalf("failed to create tailsql server: %v", err)
	}
	tsql.SetDB("sqlite://radar.db", db.DB, &tailsql.DBOptions{
		Label: "Radar DB",
	})

	// mount the tailSQL server on the debug /tailsql path
	debug.Handle("tailsql/", "SQL live debugging", tsql.NewMux())

	debug.Handle("db-stats", "Database table sizes and disk usage (JSON)", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stats, err := db.GetDatabaseStats()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get database stats: %v", err), http.StatusInternalServerError)
			return
		}
		if err := json.NewEncoder(w).Encode(stats); err != nil {
			http.Error(w, fmt.Sprintf("Failed to encode stats: %v", err), http.StatusInternalServerError)
			return
		}
	}))

	debug.Handle("backup", "Create and download a backup of the database now", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unixTime := time.Now().Unix()
		backupPath := fmt.Sprintf("backup-%d.db", unixTime)
		if _, err := db.DB.Exec("VACUUM INTO ?", backupPath); err != nil {
			http.Error(w, fmt.Sprintf("Failed to create backup: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", backupPath))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Encoding", "gzip")

		// Send the backup file to the client
		backupFile, err := os.Open(backupPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to open backup file: %v", err), http.StatusInternalServerError)
			return
		}

		// close the backup file after sending it
		// and remove it from the filesystem
		defer func() {
			backupFile.Close()
			if err := os.Remove(backupPath); err != nil {
				log.Printf("Failed to remove backup file: %v", err)
			}
		}()

		gzipWriter := gzip.NewWriter(w)
		defer gzipWriter.Close()
		if _, err := gzipWriter.Write([]byte{}); err != nil {
			// Need to write something to initialize the gzip header
			http.Error(w, fmt.Sprintf("Failed to initialize gzip writer: %v", err), http.StatusInternalServerError)
			return
		}

		// Copy the backup file content to the gzip writer
		if _, err := io.Copy(gzipWriter, backupFile); err != nil {
			http.Error(w, fmt.Sprintf("Failed to write backup file: %v", err), http.StatusInternalServerError)
			return
		}
	}))
}
