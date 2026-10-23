package db

import "testing"

// Every run recorded before migration 67 had its VRLOG written after
// background was stripped, so it reads as foreground; a new run may be full.
func TestMigration067RunFrameModeRoundTrip(t *testing.T) {
	db := setupEmptyTestDB(t)
	defer db.Close()
	migrations, err := getMigrationsFS()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateTo(migrations, 66); err != nil {
		t.Fatal(err)
	}
	must := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO lidar_runs(run_id,created_at,source_type,sensor_id) VALUES('old',1,'pcap','hesai')`)
	if err := db.MigrateTo(migrations, 67); err != nil {
		t.Fatal(err)
	}

	if got := queryStrings(t, db, `SELECT frame_mode FROM lidar_runs WHERE run_id='old'`); len(got) != 1 || got[0] != "foreground" {
		t.Fatalf("an existing run reads %v, want foreground", got)
	}
	must(`INSERT INTO lidar_runs(run_id,created_at,source_type,sensor_id,frame_mode) VALUES('new',2,'pcap','hesai','full')`)
	if _, err := db.Exec(`UPDATE lidar_runs SET frame_mode='background' WHERE run_id='old'`); err == nil {
		t.Error("an unknown frame mode was accepted")
	}

	if err := db.MigrateTo(migrations, 66); err != nil {
		t.Fatal(err)
	}
	if got := queryStrings(t, db, `SELECT run_id FROM lidar_runs ORDER BY run_id`); len(got) != 2 {
		t.Fatalf("runs after rolling back: %v", got)
	}
}
