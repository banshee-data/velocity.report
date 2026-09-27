package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestLatestMigrationReverses applies every real migration, rolls the latest
// back, and requires the schema to be exactly what migrating a fresh database
// to the previous version produces; then rolls forward and requires the full
// schema again. It names no version and tolerates gaps in the numbering, so
// it holds each new migration's down file to the same standard as its up file
// without being edited when one lands.
func TestLatestMigrationReverses(t *testing.T) {
	open := func(name string) *DB {
		sqlDB, err := sql.Open("sqlite", filepath.Join(t.TempDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sqlDB.Close() })
		return &DB{sqlDB}
	}
	db := open("latest.db")
	migrationsFS, err := getMigrationsFS()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	latest, _, err := db.MigrateVersion(migrationsFS)
	if err != nil {
		t.Fatal(err)
	}
	full := getSchemaDefinition(t, db.DB)

	if err := db.MigrateDown(migrationsFS); err != nil {
		t.Fatalf("migrate down from %d: %v", latest, err)
	}
	prior, dirty, err := db.MigrateVersion(migrationsFS)
	if err != nil || dirty || prior >= latest {
		t.Fatalf("after down: version %d dirty %v err %v, want below %d", prior, dirty, err, latest)
	}
	previous := open("previous.db")
	if err := previous.MigrateTo(migrationsFS, prior); err != nil {
		t.Fatalf("migrate a fresh database to %d: %v", prior, err)
	}
	if down, want := getSchemaDefinition(t, db.DB), getSchemaDefinition(t, previous.DB); !schemasMatch(down, want) {
		t.Fatalf("schema after rolling back %d is not version %d's:\n%s\nwant:\n%s",
			latest, prior, formatSchema(down), formatSchema(want))
	}
	if err := db.MigrateUp(migrationsFS); err != nil {
		t.Fatalf("migrate up again: %v", err)
	}
	if again := getSchemaDefinition(t, db.DB); !schemasMatch(full, again) {
		t.Fatalf("schema after down and up differs:\n%s\nwant:\n%s", formatSchema(again), formatSchema(full))
	}
}
