package db

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// legacySelection is a row as migration 000054 stored it: the role and the
// finder as columns of their own beside the window document.
type legacySelection struct {
	segment, run, replayCase, role, finder, parameters, window string
}

func legacyWindow(id, finder, role, source string, edit func(map[string]any)) string {
	doc := map[string]any{
		"id": id, "finder": finder, "version": 1, "source": source, "role": role,
		"window_start_unix_nanos": int64(1_788_466_680_000_000_000),
		"window_end_unix_nanos":   int64(1_788_466_690_000_000_000),
		"peak_timestamp_ns":       int64(1_788_466_684_000_000_000),
		"score":                   12.5, "capture": "/captures/a.pcap", "offset_seconds": 45.3,
		"status": "candidate",
	}
	if edit != nil {
		edit(doc)
	}
	b, _ := json.Marshal(doc)
	return string(b)
}

const legacyParameters = `{"window_seconds":10,"min_speed":3,"random_seed":1}`

// The rows a database could hold after running 000054: the ones the
// application wrote, the one its finder default wrote wrongly, and ones that
// only a hand or another writer could have made.
func legacySelections() (kept, rejected []legacySelection, reasons map[string]string) {
	kept = []legacySelection{
		{"seg-ok", "run-a", "case-ok", "tuning", "following", legacyParameters, legacyWindow("seg-ok", "following", "tuning", "run-a", nil)},
		// The request default left the column empty; the document is right.
		{"seg-default", "run-a", "case-default", "tuning", "", legacyParameters, legacyWindow("seg-default", "following", "tuning", "run-a", nil)},
		{"seg-held", "run-b", "case-held", "held_out", "random", legacyParameters, legacyWindow("seg-held", "random", "held_out", "run-b", nil)},
		// 000054 had no foreign key, so a selection could outlive its run.
		{"seg-orphan", "run-deleted", "case-orphan", "tuning", "exposure", legacyParameters, legacyWindow("seg-orphan", "exposure", "tuning", "run-deleted", nil)},
	}
	rejected = []legacySelection{
		{"seg-not-json", "run-a", "case-not-json", "tuning", "following", legacyParameters, `{not json`},
		{"seg-array", "run-a", "case-array", "tuning", "following", legacyParameters, `["seg-array"]`},
		{"seg-bad-params", "run-a", "case-bad-params", "tuning", "following", `{`, legacyWindow("seg-bad-params", "following", "tuning", "run-a", nil)},
		{"seg-params-number", "run-a", "case-params-number", "tuning", "following", `10`, legacyWindow("seg-params-number", "following", "tuning", "run-a", nil)},
		{"seg-other-id", "run-a", "case-other-id", "tuning", "following", legacyParameters, legacyWindow("seg-someone-else", "following", "tuning", "run-a", nil)},
		{"seg-other-source", "run-a", "case-other-source", "tuning", "following", legacyParameters, legacyWindow("seg-other-source", "following", "tuning", "run-b", nil)},
		{"seg-role-differs", "run-a", "case-role-differs", "held_out", "following", legacyParameters, legacyWindow("seg-role-differs", "following", "tuning", "run-a", nil)},
		{"seg-finder-differs", "run-a", "case-finder-differs", "tuning", "exposure", legacyParameters, legacyWindow("seg-finder-differs", "following", "tuning", "run-a", nil)},
		{"seg-unknown-finder", "run-a", "case-unknown-finder", "tuning", "manual", legacyParameters, legacyWindow("seg-unknown-finder", "manual", "tuning", "run-a", nil)},
		{"seg-no-finder", "run-a", "case-no-finder", "tuning", "", legacyParameters, legacyWindow("seg-no-finder", "following", "tuning", "run-a", func(d map[string]any) { delete(d, "finder") })},
		{"seg-failure-held", "run-a", "case-failure-held", "held_out", "leader_changes", legacyParameters, legacyWindow("seg-failure-held", "leader_changes", "held_out", "run-a", nil)},
		{"seg-no-version", "run-a", "case-no-version", "tuning", "following", legacyParameters, legacyWindow("seg-no-version", "following", "tuning", "run-a", func(d map[string]any) { delete(d, "version") })},
		{"seg-inverted", "run-a", "case-inverted", "tuning", "following", legacyParameters, legacyWindow("seg-inverted", "following", "tuning", "run-a", func(d map[string]any) { d["window_end_unix_nanos"] = int64(1) })},
		{"seg-no-capture", "run-a", "case-no-capture", "tuning", "following", legacyParameters, legacyWindow("seg-no-capture", "following", "tuning", "run-a", func(d map[string]any) { d["capture"] = "" })},
		{"seg-no-case", "run-a", "case-missing", "tuning", "following", legacyParameters, legacyWindow("seg-no-case", "following", "tuning", "run-a", nil)},
	}
	reasons = map[string]string{
		"seg-not-json":       "window_json is not valid JSON",
		"seg-array":          "window_json is not an object",
		"seg-bad-params":     "parameters_json is not valid JSON",
		"seg-params-number":  "parameters_json is not an object",
		"seg-other-id":       "window_json names another segment",
		"seg-other-source":   "window_json names another source than run_id",
		"seg-role-differs":   "role disagrees with window_json",
		"seg-finder-differs": "finder disagrees with window_json",
		"seg-unknown-finder": "finder is not in the catalogue",
		"seg-no-finder":      "finder is not in the catalogue",
		"seg-failure-held":   "held-out window chosen by a failure-seeking finder",
		"seg-no-version":     "finder version is missing",
		"seg-inverted":       "window bounds are missing or inverted",
		"seg-no-capture":     "window has no capture",
		"seg-no-case":        "replay case does not exist",
	}
	return kept, rejected, reasons
}

func databaseAt054(t *testing.T) *DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "migration055.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := &DB{sqlDB}
	t.Cleanup(func() { db.Close() })
	migrations, err := getMigrationsFS()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateTo(migrations, 54); err != nil {
		t.Fatalf("migrate to 54: %v", err)
	}
	kept, rejected, _ := legacySelections()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	// 000054 enforced its one foreign key, so the row without a case can
	// only be written with enforcement off, as an older tool might have.
	exec(`PRAGMA foreign_keys = OFF`)
	for _, run := range []string{"run-a", "run-b"} {
		exec(`INSERT INTO lidar_run_records(run_id,created_at,source_type,sensor_id,status) VALUES(?,1,'pcap','sensor','completed')`, run)
	}
	for _, s := range append(append([]legacySelection{}, kept...), rejected...) {
		if s.replayCase != "case-missing" {
			exec(`INSERT INTO lidar_replay_cases(replay_case_id,sensor_id,pcap_file,created_at_ns) VALUES(?,'sensor','a.pcap',1)`, s.replayCase)
		}
		exec(`INSERT INTO lidar_segment_selections(segment_id,run_id,replay_case_id,role,finder,parameters_json,window_json,created_at_ns) VALUES(?,?,?,?,?,?,?,7)`,
			s.segment, s.run, s.replayCase, s.role, s.finder, s.parameters, s.window)
	}
	exec(`INSERT INTO lidar_replay_cases(replay_case_id,sensor_id,pcap_file,created_at_ns) VALUES('case-elsewhere','sensor','b.pcap',1)`)
	for _, job := range []struct{ id, kind, session, state string }{
		{"job-packed", "vrlog_record", "seg-ok", "completed"},
		{"job-queued", "vrlog_record", "seg-held", "queued"},
		{"job-other-case", "vrlog_record", "seg-default", "completed"},
		{"job-rejected-selection", "vrlog_record", "seg-not-json", "failed"},
		{"job-motion", "motion_pass", "ses-1", "completed"},
	} {
		exec(`INSERT INTO lidar_capture_jobs(job_id,kind,session_id,state,queued_at_ns) VALUES(?,?,?,?,1)`, job.id, job.kind, job.session, job.state)
	}
	for _, link := range []struct{ job, segment, replayCase, pack string }{
		{"job-packed", "seg-ok", "case-ok", "/var/lib/velocity-report/packs/clip-job-packed-123/pack"},
		{"job-queued", "seg-held", "case-held", ""},
		// A job that replayed another case than its selection's.
		{"job-other-case", "seg-default", "case-elsewhere", "/var/lib/velocity-report/packs/clip-job-other-case-9/pack"},
		{"job-rejected-selection", "seg-not-json", "case-not-json", ""},
		// Links to a selection and to a queue entry that do not exist.
		{"job-no-selection", "seg-never-chosen", "case-ok", ""},
		{"job-not-queued", "seg-orphan", "case-orphan", ""},
	} {
		exec(`INSERT INTO lidar_segment_clip_jobs(job_id,segment_id,replay_case_id,pack_dir) VALUES(?,?,?,?)`, link.job, link.segment, link.replayCase, link.pack)
	}
	exec(`PRAGMA foreign_keys = ON`)
	return db
}

func queryStrings(t *testing.T, db *DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.Query(query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s.String)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMigration055KeepsEveryLegacyRow(t *testing.T) {
	db := databaseAt054(t)
	migrations, err := getMigrationsFS()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateTo(migrations, 55); err != nil {
		t.Fatalf("migrate to 55: %v", err)
	}
	kept, rejected, reasons := legacySelections()

	wantKept := []string{}
	for _, s := range kept {
		wantKept = append(wantKept, s.segment)
	}
	sort.Strings(wantKept)
	if got := queryStrings(t, db, `SELECT segment_id FROM lidar_segment_selections ORDER BY segment_id`); strings.Join(got, ",") != strings.Join(wantKept, ",") {
		t.Fatalf("selections kept: %v, want %v", got, wantKept)
	}
	for _, s := range kept {
		var run sql.NullString
		var source, role, finder, capture, parameters, window string
		var version, document int
		var start, end, created int64
		if err := db.QueryRow(`SELECT run_id,source,role,finder,finder_version,capture,window_start_ns,window_end_ns,document_version,parameters_json,window_json,created_at_ns FROM lidar_segment_selections WHERE segment_id=?`, s.segment).
			Scan(&run, &source, &role, &finder, &version, &capture, &start, &end, &document, &parameters, &window, &created); err != nil {
			t.Fatalf("%s: %v", s.segment, err)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(window), &doc); err != nil {
			t.Fatalf("%s: %v", s.segment, err)
		}
		if source != s.run || role != s.role || finder != doc["finder"] || version != 1 || document != 1 ||
			capture != "/captures/a.pcap" || start != 1_788_466_680_000_000_000 || end != 1_788_466_690_000_000_000 ||
			parameters != s.parameters || window != s.window || created != 7 {
			t.Fatalf("%s did not carry over: source %q role %q finder %q version %d capture %q [%d, %d]", s.segment, source, role, finder, version, capture, start, end)
		}
		// A run that exists is linked; one that was deleted leaves its name.
		if wantRun := s.run != "run-deleted"; run.Valid != wantRun || (wantRun && run.String != s.run) {
			t.Fatalf("%s: run_id %+v", s.segment, run)
		}
	}

	// Nothing is dropped: a row the new rules refuse is kept whole, with why.
	for _, s := range rejected {
		var reason, row string
		if err := db.QueryRow(`SELECT reason,row_json FROM lidar_migration_rejects WHERE migration=55 AND source_table='lidar_segment_selections' AND source_key=?`, s.segment).Scan(&reason, &row); err != nil {
			t.Fatalf("%s was dropped: %v", s.segment, err)
		}
		if reason != reasons[s.segment] {
			t.Fatalf("%s: reason %q, want %q", s.segment, reason, reasons[s.segment])
		}
		var saved map[string]any
		if err := json.Unmarshal([]byte(row), &saved); err != nil {
			t.Fatalf("%s: %v", s.segment, err)
		}
		if saved["segment_id"] != s.segment || saved["run_id"] != s.run || saved["replay_case_id"] != s.replayCase || saved["role"] != s.role ||
			saved["finder"] != s.finder || saved["parameters_json"] != s.parameters || saved["window_json"] != s.window || saved["created_at_ns"] != float64(7) {
			t.Fatalf("%s was not kept whole: %v", s.segment, saved)
		}
	}
	var selectionRejects int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_migration_rejects WHERE source_table='lidar_segment_selections'`).Scan(&selectionRejects); err != nil || selectionRejects != len(rejected) {
		t.Fatalf("%d selections set aside, want %d: %v", selectionRejects, len(rejected), err)
	}
	// The replay case of a rejected selection is the operator's, and stays.
	if cases := queryStrings(t, db, `SELECT replay_case_id FROM lidar_replay_cases WHERE replay_case_id='case-not-json'`); len(cases) != 1 {
		t.Fatal("a rejected selection took its replay case with it")
	}

	if got := queryStrings(t, db, `SELECT job_id FROM lidar_segment_clip_jobs ORDER BY job_id`); strings.Join(got, ",") != "job-packed,job-queued" {
		t.Fatalf("clip jobs kept: %v", got)
	}
	// The old path was absolute and had no digest. It is cleared, and the
	// server finds the pack again by its job.
	var linked int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lidar_segment_clip_jobs WHERE pack_dir IS NOT NULL OR pack_digest IS NOT NULL`).Scan(&linked); err != nil || linked != 0 {
		t.Fatalf("%d pack links carried over: %v", linked, err)
	}
	for job, want := range map[string]string{
		"job-other-case":         "job replays another case than its selection",
		"job-rejected-selection": "selection was rejected",
		"job-no-selection":       "selection does not exist",
		"job-not-queued":         "capture job does not exist",
	} {
		var reason, row string
		if err := db.QueryRow(`SELECT reason,row_json FROM lidar_migration_rejects WHERE migration=55 AND source_table='lidar_segment_clip_jobs' AND source_key=?`, job).Scan(&reason, &row); err != nil {
			t.Fatalf("%s was dropped: %v", job, err)
		}
		if reason != want || !strings.Contains(row, job) {
			t.Fatalf("%s: reason %q, row %s", job, reason, row)
		}
	}
	// A clip job names its segment through the link, no longer through the
	// queue's session column. Other kinds keep their session.
	for job, want := range map[string]string{"job-packed": "", "job-queued": "", "job-motion": "ses-1"} {
		var session sql.NullString
		if err := db.QueryRow(`SELECT session_id FROM lidar_capture_jobs WHERE job_id=?`, job).Scan(&session); err != nil || session.String != want {
			t.Fatalf("%s: session %+v, want %q: %v", job, session, want, err)
		}
	}
	for name, query := range map[string]string{
		"foreign keys": `PRAGMA foreign_key_check`,
		"scratch":      `SELECT name FROM sqlite_temp_master WHERE name LIKE 'lidar_segment%'`,
		"leftovers":    `SELECT name FROM sqlite_master WHERE name LIKE 'lidar_segment%_new'`,
	} {
		if found := queryStrings(t, db, query); len(found) != 0 {
			t.Fatalf("%s after the migration: %v", name, found)
		}
	}
	if fk := queryStrings(t, db, `PRAGMA foreign_keys`); len(fk) != 1 || fk[0] != "1" {
		t.Fatalf("foreign keys were left off: %v", fk)
	}
}

func TestMigration055RollsBackToTheRowsItFound(t *testing.T) {
	db := databaseAt054(t)
	type row struct{ segment, run, replayCase, role, finder, parameters, window string }
	read := func() map[string]row {
		t.Helper()
		rows, err := db.Query(`SELECT segment_id,run_id,replay_case_id,role,finder,parameters_json,window_json FROM lidar_segment_selections`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]row{}
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.segment, &r.run, &r.replayCase, &r.role, &r.finder, &r.parameters, &r.window); err != nil {
				t.Fatal(err)
			}
			out[r.segment] = r
		}
		return out
	}
	before := read()
	linksBefore := queryStrings(t, db, `SELECT job_id || '|' || segment_id || '|' || replay_case_id FROM lidar_segment_clip_jobs ORDER BY job_id`)
	sessionsBefore := queryStrings(t, db, `SELECT job_id || '|' || COALESCE(session_id,'') FROM lidar_capture_jobs ORDER BY job_id`)

	migrations, err := getMigrationsFS()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateTo(migrations, 55); err != nil {
		t.Fatalf("migrate to 55: %v", err)
	}
	if err := db.MigrateTo(migrations, 54); err != nil {
		t.Fatalf("roll back to 54: %v", err)
	}
	after := read()
	if len(after) != len(before) {
		t.Fatalf("%d selections after the round trip, %d before", len(after), len(before))
	}
	for id, was := range before {
		now := after[id]
		// The one repair is kept: the finder the request default left empty.
		if id == "seg-default" {
			was.finder = "following"
		}
		if now != was {
			t.Fatalf("%s changed:\n was %+v\n now %+v", id, was, now)
		}
	}
	// Every link returns, including the ones that named a selection or a
	// job that did not exist: they were kept whole, so they go back whole.
	if got := queryStrings(t, db, `SELECT job_id || '|' || segment_id || '|' || replay_case_id FROM lidar_segment_clip_jobs ORDER BY job_id`); strings.Join(got, ",") != strings.Join(linksBefore, ",") {
		t.Fatalf("clip links after the round trip:\n got %v\nwant %v", got, linksBefore)
	}
	if got := queryStrings(t, db, `SELECT job_id || '|' || COALESCE(session_id,'') FROM lidar_capture_jobs ORDER BY job_id`); strings.Join(got, ",") != strings.Join(sessionsBefore, ",") {
		t.Fatalf("job sessions after the round trip:\n got %v\nwant %v", got, sessionsBefore)
	}
	if tables := queryStrings(t, db, `SELECT name FROM sqlite_master WHERE name LIKE '%migration_rejects%' OR name LIKE 'idx_lidar_segment%' OR name LIKE 'lidar_segment%_old'`); len(tables) != 0 {
		t.Fatalf("the rollback left %v", tables)
	}
	columns := queryStrings(t, db, `SELECT name FROM pragma_table_xinfo('lidar_segment_selections') ORDER BY cid`)
	if strings.Join(columns, ",") != "segment_id,run_id,replay_case_id,role,finder,parameters_json,window_json,created_at_ns" {
		t.Fatalf("columns after the rollback: %v", columns)
	}
	// And forward again, to the same place.
	if err := db.MigrateTo(migrations, 55); err != nil {
		t.Fatalf("migrate to 55 a second time: %v", err)
	}
	kept, rejected, _ := legacySelections()
	var selections, rejects int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM lidar_segment_selections), (SELECT COUNT(*) FROM lidar_migration_rejects WHERE source_table='lidar_segment_selections')`).Scan(&selections, &rejects); err != nil || selections != len(kept) || rejects != len(rejected) {
		t.Fatalf("second migration: %d kept, %d set aside, want %d and %d: %v", selections, rejects, len(kept), len(rejected), err)
	}
}

// A database that never ran the segment finder has nothing to carry over,
// and must arrive at the same schema.
func TestMigration055OnAnEmptyDatabase(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := &DB{sqlDB}
	defer db.Close()
	migrations, err := getMigrationsFS()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateTo(migrations, 55); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int{"lidar_segment_selections": 0, "lidar_segment_clip_jobs": 0, "lidar_migration_rejects": 0} {
		var rows int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&rows); err != nil || rows != want {
			t.Fatalf("%s: %d rows: %v", table, rows, err)
		}
	}
	indexes := queryStrings(t, db, `SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'idx_lidar_segment%' ORDER BY name`)
	if strings.Join(indexes, ",") != "idx_lidar_segment_clip_jobs_segment,idx_lidar_segment_selections_guard,idx_lidar_segment_selections_run" {
		t.Fatalf("indexes: %v", indexes)
	}
}
