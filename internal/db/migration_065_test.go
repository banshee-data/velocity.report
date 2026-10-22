package db

import (
	"reflect"
	"testing"
)

func TestMigration065MovesMisplacedClassesToQualityFlags(t *testing.T) {
	db := setupEmptyTestDB(t)
	defer db.Close()
	migrations, err := getMigrationsFS()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateTo(migrations, 64); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO lidar_runs(run_id,created_at,source_type,sensor_id,status) VALUES('run',1,'pcap','sensor','completed')`)
	for _, row := range []struct{ id, class, flags string }{
		{"split", "split", "noisy"}, {"dedup", " split ", "good, split"}, {"merge", "merge", ""}, {"class", "car", "good"}, {"whitespace", " merge\t ", "good,\tmerge"},
	} {
		exec(`INSERT INTO lidar_run_tracks(run_id,track_id,sensor_id,track_state,start_unix_nanos,user_label,quality_label,label_source,labeler_id,labeled_at,label_confidence) VALUES('run',?,'sensor','confirmed',1,?,?,'human_manual','reviewer',123,0.9)`, row.id, row.class, row.flags)
	}
	before := queryStrings(t, db, `SELECT track_id||'|'||label_source||'|'||labeler_id||'|'||labeled_at||'|'||label_confidence FROM lidar_run_tracks ORDER BY track_id`)
	if err := db.MigrateTo(migrations, 65); err != nil {
		t.Fatal(err)
	}
	after := queryStrings(t, db, `SELECT track_id||'|'||label_source||'|'||labeler_id||'|'||labeled_at||'|'||label_confidence FROM lidar_run_tracks ORDER BY track_id`)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("provenance changed: %v -> %v", before, after)
	}
	got := queryStrings(t, db, `SELECT track_id||'|'||COALESCE(user_label,'')||'|'||COALESCE(quality_label,'') FROM lidar_run_tracks ORDER BY track_id`)
	want := []string{"class|car|good", "dedup||good, split", "merge||merge", "split||noisy,split", "whitespace||good,\tmerge"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("verdicts: %v, want %v", got, want)
	}
	// Old software already understands canonical quality flags; a rollback may
	// retain them without reconstructing a misplaced class or losing its meaning.
	if err := db.MigrateTo(migrations, 64); err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateTo(migrations, 65); err != nil {
		t.Fatal(err)
	}
	if again := queryStrings(t, db, `SELECT track_id||'|'||COALESCE(user_label,'')||'|'||COALESCE(quality_label,'') FROM lidar_run_tracks ORDER BY track_id`); !reflect.DeepEqual(got, again) {
		t.Fatal("reapplying duplicated a quality flag")
	}
}
