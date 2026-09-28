package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

const selectorDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// databaseAt055 holds windows chosen before selectors existed: the rows
// migration 000055 kept from a database that ran 000054.
func databaseAt055(t *testing.T) (*DB, func(uint)) {
	t.Helper()
	db := databaseAt054(t)
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
	migrateTo(55)
	return db, migrateTo
}

// chooseWithSelector stores a new tuning window the way the segments API
// does after 000056: with the selector that chose it.
func chooseWithSelector(db *DB, id, replayCase, selector string) error {
	if _, err := db.Exec(`INSERT INTO lidar_replay_cases(replay_case_id,sensor_id,pcap_file,created_at_ns) VALUES(?,'sensor','a.pcap',1)`, replayCase); err != nil {
		return err
	}
	var chosenBy any
	if selector != "" {
		chosenBy = selector
	}
	_, err := db.Exec(`INSERT INTO lidar_segment_selections(segment_id,run_id,replay_case_id,parameters_json,window_json,selector_json,created_at_ns) VALUES(?,?,?,?,?,?,9)`,
		id, "run-a", replayCase, legacyParameters, legacyWindow(id, "following", "tuning", "run-a", nil), chosenBy)
	return err
}

func selectionRows(t *testing.T, db *DB) []string {
	t.Helper()
	return queryStrings(t, db, `SELECT segment_id || '|' || COALESCE(run_id, '') || '|' || replay_case_id || '|' || parameters_json || '|' || window_json || '|' || created_at_ns
		FROM lidar_segment_selections ORDER BY segment_id`)
}

func TestMigration056KeepsWindowsChosenBeforeSelectors(t *testing.T) {
	db, migrateTo := databaseAt055(t)
	before := selectionRows(t, db)
	if len(before) == 0 {
		t.Fatal("the fixture chose no windows")
	}
	migrateTo(56)
	if after := selectionRows(t, db); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Fatalf("selections changed:\n was %v\n now %v", before, after)
	}
	if named := queryStrings(t, db, `SELECT segment_id FROM lidar_segment_selections WHERE selector_json IS NOT NULL`); len(named) != 0 {
		t.Fatalf("windows chosen before selectors were given one: %v", named)
	}
	if problems := queryStrings(t, db, `SELECT "table" FROM pragma_foreign_key_check`); len(problems) != 0 {
		t.Fatalf("foreign key check: %v", problems)
	}
	if check := queryStrings(t, db, `PRAGMA integrity_check`); strings.Join(check, ",") != "ok" {
		t.Fatalf("integrity check: %v", check)
	}

	// From now on a window names the selector that chose it, and keeps it.
	if err := chooseWithSelector(db, "seg-unnamed", "case-unnamed", ""); err == nil || !strings.Contains(err.Error(), "names the selector") {
		t.Fatalf("a window without its selector: %v", err)
	}
	named := `{"id":"close_following","digest":"` + selectorDigest + `","held_out_eligible":false,"finder":"following"}`
	if err := chooseWithSelector(db, "seg-named", "case-named", named); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE lidar_segment_selections SET selector_json = NULL WHERE segment_id = 'seg-named'`); err == nil || !strings.Contains(err.Error(), "keeps the selector") {
		t.Fatalf("a window's selector was erased: %v", err)
	}
	// A window chosen before selectors can be given one later, under the
	// same rules: only erasing one is refused.
	if _, err := db.Exec(`UPDATE lidar_segment_selections SET selector_json = ? WHERE segment_id = 'seg-ok'`, named); err != nil {
		t.Fatalf("an old window could not be named: %v", err)
	}
}

func TestMigration056RollsBackToTheColumnsItFound(t *testing.T) {
	db, migrateTo := databaseAt055(t)
	columns := func() string {
		return strings.Join(queryStrings(t, db, `SELECT name FROM pragma_table_xinfo('lidar_segment_selections') ORDER BY cid`), ",")
	}
	columnsBefore := columns()
	migrateTo(56)
	if !strings.HasSuffix(columns(), ",selector_json") {
		t.Fatalf("columns at 56: %s", columns())
	}
	named := `{"id":"following","digest":"` + selectorDigest + `","held_out_eligible":true,"finder":"following"}`
	if err := chooseWithSelector(db, "seg-named", "case-named", named); err != nil {
		t.Fatal(err)
	}
	rowsAt56 := selectionRows(t, db)

	migrateTo(55)
	if got := columns(); got != columnsBefore {
		t.Fatalf("columns after the rollback: %s, want %s", got, columnsBefore)
	}
	// Every row stays, the one chosen with a selector as well; only its
	// selector goes with the column.
	if got := selectionRows(t, db); strings.Join(got, "\n") != strings.Join(rowsAt56, "\n") {
		t.Fatalf("rows after the rollback:\n got %v\nwant %v", got, rowsAt56)
	}
	if triggers := queryStrings(t, db, `SELECT name FROM sqlite_master WHERE type='trigger' AND tbl_name='lidar_segment_selections'`); len(triggers) != 0 {
		t.Fatalf("the rollback left %v", triggers)
	}
	migrateTo(56)
	if !strings.HasSuffix(columns(), ",selector_json") {
		t.Fatalf("columns at 56 a second time: %s", columns())
	}
}

func TestMigration056OnAnEmptyDatabase(t *testing.T) {
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
	if err := db.MigrateTo(migrations, 56); err != nil {
		t.Fatal(err)
	}
	triggers := queryStrings(t, db, `SELECT name FROM sqlite_master WHERE type='trigger' AND tbl_name='lidar_segment_selections' ORDER BY name`)
	if strings.Join(triggers, ",") != "lidar_segment_selections_keep_selector,lidar_segment_selections_name_selector" {
		t.Fatalf("triggers: %v", triggers)
	}
}
