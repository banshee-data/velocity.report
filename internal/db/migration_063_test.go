package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// databaseAt062 holds observations keyed by track and acquisition time: rows
// with and without a frame time, and two rows of track b that would share a
// frame once a missing frame is filled from the acquisition time.
func databaseAt062(t *testing.T) (*DB, func(uint)) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "at062.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := &DB{sqlDB}
	t.Cleanup(func() { _ = db.Close() })
	migrations, err := getMigrationsFS()
	if err != nil {
		t.Fatal(err)
	}
	migrateTo := func(version uint) {
		t.Helper()
		if err := db.MigrateTo(migrations, version); err != nil {
			t.Fatalf("migrate to %d: %v", version, err)
		}
	}
	migrateTo(62)
	for _, track := range []string{"a", "b"} {
		if _, err := db.Exec(`INSERT INTO lidar_tracks (track_id, sensor_id, frame_id, track_state, start_unix_nanos)
			VALUES (?, 's', 'world', 'confirmed', 1)`, track); err != nil {
			t.Fatal(err)
		}
	}
	for _, o := range []struct {
		track string
		ts    int64
		frame any
		box   any
	}{
		{"a", 100, 90, 4.5},
		{"a", 200, nil, nil},
		{"a", 300, 290, nil},
		{"b", 400, nil, nil},
		{"b", 450, 400, 3.9},
	} {
		if _, err := db.Exec(`INSERT INTO lidar_track_observations
			(track_id, ts_unix_nanos, frame_unix_nanos, frame_id, x, y, measurement_source, box_x, box_y, box_heading_rad, box_length, box_width)
			VALUES (?, ?, ?, 'world', 1, 2, 'medoid_v0', ?, ?, ?, ?, ?)`, o.track, o.ts, o.frame, o.box, o.box, o.box, o.box, o.box); err != nil {
			t.Fatalf("insert %s@%d: %v", o.track, o.ts, err)
		}
	}
	return db, migrateTo
}

// observationRows is every row, track by track in acquisition order, with
// its frame time and box length.
func observationRows(t *testing.T, db *DB) string {
	t.Helper()
	return strings.Join(queryStrings(t, db, `SELECT track_id || '@' || ts_unix_nanos || '/' || COALESCE(frame_unix_nanos, 'null')
		|| '/' || measurement_source || '/' || COALESCE(box_length, 'null')
		FROM lidar_track_observations ORDER BY track_id, ts_unix_nanos, frame_unix_nanos`), " ")
}

// primaryKey is the table's primary key columns in key order.
func primaryKey(t *testing.T, db *DB) string {
	t.Helper()
	return strings.Join(queryStrings(t, db, `SELECT name FROM pragma_table_info('lidar_track_observations') WHERE pk > 0 ORDER BY pk`), ",")
}

func frameNotNull(t *testing.T, db *DB) string {
	t.Helper()
	return strings.Join(queryStrings(t, db, `SELECT "notnull" FROM pragma_table_info('lidar_track_observations') WHERE name = 'frame_unix_nanos'`), "")
}

func observationSchemaObjects(t *testing.T, db *DB) string {
	t.Helper()
	return strings.Join(queryStrings(t, db, `SELECT type || ':' || name FROM sqlite_master
		WHERE tbl_name = 'lidar_track_observations' AND sql IS NOT NULL ORDER BY type, name`), ",")
}

func checkObservationIntegrity(t *testing.T, db *DB) {
	t.Helper()
	if check := queryStrings(t, db, `PRAGMA integrity_check`); strings.Join(check, ",") != "ok" {
		t.Fatalf("integrity check: %v", check)
	}
	if fk := queryStrings(t, db, `SELECT "table" FROM pragma_foreign_key_check`); len(fk) != 0 {
		t.Fatalf("foreign key check: %v", fk)
	}
}

func TestMigration063KeysObservationsByFrame(t *testing.T) {
	db, migrateTo := databaseAt062(t)
	objectsBefore := observationSchemaObjects(t, db)
	migrateTo(63)

	if got := primaryKey(t, db); got != "track_id,frame_unix_nanos" {
		t.Fatalf("primary key at 63: %s", got)
	}
	if got := frameNotNull(t, db); got != "1" {
		t.Fatalf("frame_unix_nanos notnull at 63: %s", got)
	}
	// A missing frame is the acquisition time. b@400 would share frame 400
	// with b@450, so the later acquisition is copied and b@400 is kept whole
	// as a reject.
	if got, want := observationRows(t, db), "a@100/90/medoid_v0/4.5 a@200/200/medoid_v0/null a@300/290/medoid_v0/null b@450/400/medoid_v0/3.9"; got != want {
		t.Fatalf("rows at 63:\n got %s\nwant %s", got, want)
	}
	rejects := queryStrings(t, db, `SELECT source_key || '|' || reason || '|' || (JSON_EXTRACT(row_json, '$.frame_unix_nanos') IS NULL)
		|| '|' || JSON_EXTRACT(row_json, '$.measurement_source') FROM lidar_migration_rejects
		WHERE migration = 63 AND source_table = 'lidar_track_observations'`)
	if want := "b@400|another row of the track at the same frame has a later acquisition time|1|medoid_v0"; len(rejects) != 1 || rejects[0] != want {
		t.Fatalf("rejects at 63: %v, want [%s]", rejects, want)
	}
	if got, want := observationSchemaObjects(t, db), objectsBefore+",trigger:lidar_track_observations_frame_from_acquisition"; got != want {
		t.Fatalf("schema objects at 63:\n got %s\nwant %s", got, want)
	}
	checkObservationIntegrity(t, db)

	// Two frames whose clusters share an acquisition time now keep a row each.
	for _, frame := range []int64{495, 505} {
		if _, err := db.Exec(`INSERT OR REPLACE INTO lidar_track_observations (track_id, ts_unix_nanos, frame_unix_nanos, frame_id)
			VALUES ('a', 500, ?, 'world')`, frame); err != nil {
			t.Fatal(err)
		}
	}
	if got := queryStrings(t, db, `SELECT COUNT(*) FROM lidar_track_observations WHERE track_id = 'a' AND ts_unix_nanos = 500`); got[0] != "2" {
		t.Fatalf("rows of a at acquisition 500: %v, want 2", got)
	}
}

func TestMigration063RollsBackToTheAcquisitionKey(t *testing.T) {
	db, migrateTo := databaseAt062(t)
	objectsBefore := observationSchemaObjects(t, db)
	migrateTo(63)
	for _, frame := range []int64{495, 505} {
		if _, err := db.Exec(`INSERT INTO lidar_track_observations (track_id, ts_unix_nanos, frame_unix_nanos, frame_id, x)
			VALUES ('a', 500, ?, 'world', ?)`, frame, frame); err != nil {
			t.Fatal(err)
		}
	}
	migrateTo(62)

	if got := primaryKey(t, db); got != "track_id,ts_unix_nanos" {
		t.Fatalf("primary key at 62: %s", got)
	}
	if got := frameNotNull(t, db); got != "0" {
		t.Fatalf("frame_unix_nanos notnull at 62: %s", got)
	}
	if got := observationSchemaObjects(t, db); got != objectsBefore {
		t.Fatalf("schema objects at 62:\n got %s\nwant %s", got, objectsBefore)
	}
	// The later of the two frames sharing acquisition 500 is kept, as INSERT
	// OR REPLACE kept the later write; the earlier one is a reject. b@400,
	// set aside on the way up, is back as it was, with no frame.
	if got, want := observationRows(t, db), "a@100/90/medoid_v0/4.5 a@200/200/medoid_v0/null a@300/290/medoid_v0/null a@500/505/legacy_centroid_v0/null b@400/null/medoid_v0/null b@450/400/medoid_v0/3.9"; got != want {
		t.Fatalf("rows at 62:\n got %s\nwant %s", got, want)
	}
	if got, want := observationRejects(t, db), "a@495|another frame of the track has the same acquisition time and is later|495.0"; got != want {
		t.Fatalf("rejects at 62:\n got %s\nwant %s", got, want)
	}
	checkObservationIntegrity(t, db)

	// Up again: a@495 is back at its frame and b@400 is set aside again.
	migrateTo(63)
	if got := primaryKey(t, db); got != "track_id,frame_unix_nanos" {
		t.Fatalf("primary key after a second up: %s", got)
	}
	if got, want := observationRows(t, db), "a@100/90/medoid_v0/4.5 a@200/200/medoid_v0/null a@300/290/medoid_v0/null a@500/495/legacy_centroid_v0/null a@500/505/legacy_centroid_v0/null b@450/400/medoid_v0/3.9"; got != want {
		t.Fatalf("rows after a second up:\n got %s\nwant %s", got, want)
	}
	if got, want := observationRejects(t, db), "b@400|another row of the track at the same frame has a later acquisition time|1.0"; got != want {
		t.Fatalf("rejects after a second up:\n got %s\nwant %s", got, want)
	}
	checkObservationIntegrity(t, db)
}

// observationRejects is every observation migration 63 has set aside: its
// key, the reason and its x.
func observationRejects(t *testing.T, db *DB) string {
	t.Helper()
	return strings.Join(queryStrings(t, db, `SELECT source_key || '|' || reason || '|' || JSON_EXTRACT(row_json, '$.x')
		FROM lidar_migration_rejects WHERE migration = 63 AND source_table = 'lidar_track_observations'
		ORDER BY source_key`), " ")
}

// The trigger that files an observation naming no frame lists the table's
// columns itself, so a column added to the table without it would be
// dropped from such a row.
func TestTrackObservationFrameTriggerNamesEveryColumn(t *testing.T) {
	db, err := NewDB(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	trigger := queryStrings(t, db, `SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'lidar_track_observations_frame_from_acquisition'`)
	if len(trigger) != 1 {
		t.Fatalf("trigger: %v", trigger)
	}
	body := trigger[0][strings.Index(trigger[0], "BEGIN"):]
	for _, column := range queryStrings(t, db, `SELECT name FROM pragma_table_xinfo('lidar_track_observations')`) {
		// Each column is named in the insert's list and read from NEW,
		// except the frame, which is filled from the acquisition time.
		named, read := strings.Count(body, column) >= 2, strings.Contains(body, "NEW."+column)
		if column == "frame_unix_nanos" {
			named, read = strings.Contains(body, column), strings.Count(body, "NEW.ts_unix_nanos") == 2
		}
		if !named || !read {
			t.Errorf("the trigger does not copy %s", column)
		}
	}
}
