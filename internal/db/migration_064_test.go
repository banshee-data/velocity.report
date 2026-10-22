package db

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// This fixture includes existing selections, jobs and rejected historical rows,
// rather than just checking that ALTER TABLE succeeds on an empty database.
func TestMigration064PreservesClipRowsAndRelationships(t *testing.T) {
	db, migrateTo := databaseAt055(t)
	migrateTo(63)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO lidar_replay_case_files(replay_case_id,ordinal,pcap_file) VALUES('case-elsewhere',0,'first.pcap'),('case-elsewhere',1,'second.pcap')`)
	exec(`INSERT INTO lidar_replay_evaluations(evaluation_id,replay_case_id,reference_run_id,candidate_run_id,created_at) VALUES('eval-preserved','case-elsewhere','run-a','run-b',1)`)
	exec(`INSERT INTO lidar_replay_annotations(annotation_id,replay_case_id,class_label,start_timestamp_ns,created_at_ns) VALUES('label-preserved','case-elsewhere','car',100,1)`)
	// Make every expected backfill explicit before the snapshot. The migration
	// must do the same for a real pre-cutover database, even if its association
	// table is empty. Existing multi-file associations must not be overwritten.
	missing := queryStrings(t, db, `SELECT replay_case_id FROM lidar_replay_cases c WHERE NOT EXISTS(SELECT 1 FROM lidar_replay_case_files f WHERE f.replay_case_id=c.replay_case_id) ORDER BY replay_case_id`)
	for i, request := range []string{
		`{"scene_id":"case-elsewhere","data_source":"pcap","other":42}`,
		`{"replay_case_id":"case-elsewhere","data_source":"pcap"}`,
		`{"clip_id":"case-elsewhere","data_source":"capture"}`,
		`not-json`,
	} {
		exec(`INSERT INTO lidar_tuning_sweeps(sweep_id,sensor_id,request,started_at) VALUES(?,'sensor',?,1)`, fmt.Sprintf("sweep-%d", i), request)
	}
	names := map[string]string{"lidar_replay_cases": "lidar_clips", "lidar_replay_case_files": "lidar_clip_captures", "lidar_replay_evaluations": "lidar_clip_evaluations", "lidar_run_records": "lidar_runs"}
	tables := queryStrings(t, db, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name != 'schema_migrations' ORDER BY name`)
	before := map[string][]string{}
	for _, table := range tables {
		before[table] = snapshotClipTable(t, db, table)
	}
	migrateTo(64)
	for _, table := range tables {
		current := table
		if renamed := names[table]; renamed != "" {
			current = renamed
		}
		got := snapshotClipTable(t, db, current)
		if table == "lidar_replay_case_files" {
			// Remove only the newly backfilled rows, then compare every original row.
			kept := []string{}
			for _, row := range got {
				var value map[string]any
				if err := json.Unmarshal([]byte(row), &value); err != nil {
					t.Fatal(err)
				}
				isBackfill := false
				for _, id := range missing {
					if value["clip_id"] == id {
						isBackfill = true
						break
					}
				}
				if !isBackfill {
					kept = append(kept, row)
				}
			}
			if len(got) != len(before[table])+len(missing) {
				t.Fatalf("capture rows: %d, want %d", len(got), len(before[table])+len(missing))
			}
			got = kept
		}
		if !reflect.DeepEqual(before[table], got) {
			t.Errorf("%s values changed:\nwas %v\nnow %v", table, before[table], got)
		}
	}
	for _, id := range missing {
		paths := queryStrings(t, db, fmt.Sprintf(`SELECT pcap_file FROM lidar_clip_captures WHERE clip_id='%s'`, id))
		if len(paths) != 1 || paths[0] != "a.pcap" {
			t.Errorf("backfill %s: %v", id, paths)
		}
	}
	if bad := queryStrings(t, db, `SELECT name FROM pragma_table_info('lidar_clips') WHERE name='pcap_file'`); len(bad) != 0 {
		t.Fatal("clip path projection survived")
	}
	if bad := queryStrings(t, db, `SELECT name FROM sqlite_master WHERE type IN ('table','index') AND (name LIKE '%replay_case%' OR name='lidar_run_records' OR name LIKE '%replay_evaluations%')`); len(bad) != 0 {
		t.Fatalf("old schema names survived: %v", bad)
	}
	if bad := queryStrings(t, db, `SELECT "table" FROM pragma_foreign_key_check`); len(bad) != 0 {
		t.Fatalf("foreign keys: %v", bad)
	}
	if got := queryStrings(t, db, `PRAGMA integrity_check`); !reflect.DeepEqual(got, []string{"ok"}) {
		t.Fatalf("integrity: %v", got)
	}
	// Rollback restores the old vocabulary and derives its path from the first
	// capture. Reapplying the migration must preserve the canonical associations.
	stable := snapshotClipTable(t, db, "lidar_clip_captures")
	migrateTo(63)
	migrateTo(64)
	if got := snapshotClipTable(t, db, "lidar_clip_captures"); !reflect.DeepEqual(got, stable) {
		t.Fatal("round trip changed captures")
	}
}

func snapshotClipTable(t *testing.T, db *DB, table string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT * FROM "` + table + `"`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		row := map[string]any{}
		for i, col := range cols {
			if (table == "lidar_replay_cases" || table == "lidar_clips") && col == "pcap_file" {
				continue
			}
			col = strings.ReplaceAll(col, "replay_case_id", "clip_id")
			if table == "lidar_tuning_sweeps" && col == "request" {
				if text, ok := values[i].(string); ok {
					var request map[string]any
					if json.Unmarshal([]byte(text), &request) == nil && request != nil {
						if request["clip_id"] == nil {
							if request["replay_case_id"] != nil {
								request["clip_id"] = request["replay_case_id"]
							} else if request["scene_id"] != nil {
								request["clip_id"] = request["scene_id"]
							}
						}
						delete(request, "replay_case_id")
						delete(request, "scene_id")
						if request["data_source"] == "pcap" {
							request["data_source"] = "capture"
						}
						encoded, err := json.Marshal(request)
						if err != nil {
							t.Fatal(err)
						}
						values[i] = string(encoded)
					}
				}
			}
			row[col] = values[i]
		}
		b, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}
