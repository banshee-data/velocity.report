package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// Migration 000052 adds a state_model discriminator to lidar_track_estimates
// and the lidar_track_solid_bodies table. The discriminator's default must
// describe every row written before it existed, and the rollback must leave
// those rows readable and exactly as they were.
func TestSolidBodyMigrationRoundTripPreservesPointEstimates(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "solid_body_migration.db"))
	if err != nil {
		t.Fatal(err)
	}
	database := &DB{sqlDB}
	defer database.Close()
	migrations, err := getMigrationsFS()
	if err != nil {
		t.Fatal(err)
	}

	// A point estimate written by the schema before the discriminator.
	if err := database.MigrateTo(migrations, 49); err != nil {
		t.Fatalf("migrate to 49: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO lidar_track_estimates
		(estimate_id, track_id, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos,
		 estimator_id, observation_model_id, param_hash, stage, measurement_source, creation_sequence, x, y, vx, vy, covariance_json, inserted_at_ns)
		VALUES ('e-1', 't', 'o', 's', 'c', 100, 101, 'cv_kf_v1', 'medoid_v0', 'p', 'online', 'medoid_v0', 1, 1, 2, 3, 4, '[1]', 1)`); err != nil {
		t.Fatalf("insert pre-migration estimate: %v", err)
	}

	if err := database.MigrateTo(migrations, 52); err != nil {
		t.Fatalf("migrate to 52: %v", err)
	}
	var stateModel string
	if err := database.QueryRow(`SELECT state_model FROM lidar_track_estimates WHERE estimate_id = 'e-1'`).Scan(&stateModel); err != nil {
		t.Fatal(err)
	}
	if stateModel != "cv_cartesian_v1" {
		t.Fatalf("pre-existing point estimate reads state model %q", stateModel)
	}
	if !solidBodyMigrationHasTable(t, database, "lidar_track_solid_bodies") {
		t.Fatal("lidar_track_solid_bodies missing after 000052")
	}

	if err := database.MigrateTo(migrations, 49); err != nil {
		t.Fatalf("roll back to 49: %v", err)
	}
	if solidBodyMigrationHasTable(t, database, "lidar_track_solid_bodies") {
		t.Fatal("lidar_track_solid_bodies survived the rollback")
	}
	if solidBodyMigrationHasColumn(t, database, "lidar_track_estimates", "state_model") {
		t.Fatal("state_model survived the rollback")
	}
	var x, y float64
	if err := database.QueryRow(`SELECT x, y FROM lidar_track_estimates WHERE estimate_id = 'e-1'`).Scan(&x, &y); err != nil {
		t.Fatalf("point estimate lost in the rollback: %v", err)
	}
	if x != 1 || y != 2 {
		t.Fatalf("point estimate changed by the rollback: x=%v y=%v", x, y)
	}
}

func solidBodyMigrationHasTable(t *testing.T, database *DB, table string) bool {
	t.Helper()
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func solidBodyMigrationHasColumn(t *testing.T, database *DB, table, column string) bool {
	t.Helper()
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}
