package db

import (
	"os"
	"testing"
)

// TestMigrate012Down tests the down migration for track quality metrics
func TestMigrate012Down(t *testing.T) {
	// Enable dev mode to use filesystem migrations
	origDevMode := DevMode
	DevMode = true
	defer func() { DevMode = origDevMode }()

	db := setupMigrationTestDB(t)
	defer cleanupTestDB(t, db)

	// Get migrations FS from filesystem
	migrationsFS := os.DirFS("migrations")

	// Migrate up to version 12
	t.Log("Migrating to version 12...")
	if err := db.MigrateTo(migrationsFS, 12); err != nil {
		t.Fatalf("Migration to version 12 failed: %v", err)
	}

	version, dirty, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("Failed to get version after up: %v", err)
	}
	t.Logf("After up: version=%d, dirty=%v", version, dirty)

	if version != 12 {
		t.Fatalf("Expected version 12 after up, got %d", version)
	}
	if dirty {
		t.Fatalf("Database should not be dirty after migration up")
	}

	// Verify the quality metrics columns exist
	var colCount int
	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM pragma_table_info('lidar_tracks')
		WHERE name IN ('track_length_meters', 'track_duration_secs', 'occlusion_count',
		               'max_occlusion_frames', 'spatial_coverage', 'noise_point_ratio')
	`).Scan(&colCount)
	if err != nil {
		t.Fatalf("Failed to check columns after up: %v", err)
	}
	if colCount != 6 {
		t.Fatalf("Expected 6 quality metric columns after up, found %d", colCount)
	}

	// Now migrate down
	t.Log("Migrating down to version 11...")
	if err := db.MigrateDown(migrationsFS); err != nil {
		t.Fatalf("Migration down failed: %v", err)
	}

	version, dirty, err = db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatalf("Failed to get version after down: %v", err)
	}
	t.Logf("After down: version=%d, dirty=%v", version, dirty)

	if version != 11 {
		t.Fatalf("Expected version 11 after down, got %d", version)
	}
	if dirty {
		t.Fatalf("Database should not be dirty after migration down")
	}

	// Verify the quality metrics columns are gone
	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM pragma_table_info('lidar_tracks')
		WHERE name IN ('track_length_meters', 'track_duration_secs', 'occlusion_count',
		               'max_occlusion_frames', 'spatial_coverage', 'noise_point_ratio')
	`).Scan(&colCount)
	if err != nil {
		t.Fatalf("Failed to check columns after down: %v", err)
	}
	if colCount != 0 {
		t.Fatalf("Expected 0 quality metric columns after down, found %d", colCount)
	}

	// Verify the quality index is gone
	var indexCount int
	err = db.QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type='index' AND name='idx_lidar_tracks_quality'
	`).Scan(&indexCount)
	if err != nil {
		t.Fatalf("Failed to check index after down: %v", err)
	}
	if indexCount != 0 {
		t.Fatalf("Expected idx_lidar_tracks_quality to be dropped after down")
	}

	t.Log("✓ Migration 000012 down successful!")
}
