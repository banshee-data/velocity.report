package db

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const captureTriggers058 = "lidar_capture_jobs_check_insert,lidar_capture_jobs_check_update," +
	"lidar_capture_motion_periods_check_insert,lidar_capture_motion_periods_check_update"

// captureDatabaseAt057 holds the capture queue and a motion timeline as they stood
// before 000058: a motion pass over a session, the clip jobs 000055 linked,
// one clip job an interrupted enqueue left without its link, and the
// session's periods.
func captureDatabaseAt057(t *testing.T) (*DB, func(uint)) {
	t.Helper()
	db, migrateTo := databaseAt055(t)
	migrateTo(57)
	for _, stmt := range []string{
		// 000055 cleared session_id only where it found a link, so this job
		// still names its segment there.
		`INSERT INTO lidar_capture_jobs(job_id,kind,session_id,state,queued_at_ns) VALUES('job-unlinked','vrlog_record','seg-lost','failed',1)`,
		`INSERT INTO lidar_capture_roots(root_id,path,created_at_ns,updated_at_ns) VALUES('root-1','/captures',1,1)`,
		`INSERT INTO lidar_capture_sessions(session_id,root_id,start_ns,end_ns,derived_at_ns) VALUES('ses-1','root-1',0,600000000000,1)`,
		`INSERT INTO lidar_capture_motion_periods(period_id,session_id,ordinal,period_type,start_ns,end_ns,duration_ns,start_secs,end_secs,created_at_ns) VALUES('per-0','ses-1',0,'static',0,120000000000,120000000000,0,120,1)`,
		`INSERT INTO lidar_capture_motion_periods(period_id,session_id,ordinal,period_type,start_ns,end_ns,duration_ns,start_secs,end_secs,created_at_ns) VALUES('per-1','ses-1',1,'motion',120000000000,180000000000,60000000000,120,180,1)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return db, migrateTo
}

func jobRows(t *testing.T, db *DB) []string {
	t.Helper()
	return queryStrings(t, db, `SELECT job_id || '|' || kind || '|' || COALESCE(session_id, '') || '|' || state
		FROM lidar_capture_jobs ORDER BY job_id`)
}

func periodRows(t *testing.T, db *DB) []string {
	t.Helper()
	return queryStrings(t, db, `SELECT period_id || '|' || start_ns || '|' || end_ns || '|' || duration_ns
		FROM lidar_capture_motion_periods ORDER BY period_id`)
}

func captureTriggers(t *testing.T, db *DB) string {
	t.Helper()
	return strings.Join(queryStrings(t, db, `SELECT name FROM sqlite_master
		WHERE type = 'trigger' AND tbl_name IN ('lidar_capture_jobs', 'lidar_capture_motion_periods')
		ORDER BY name`), ",")
}

// accepts and refuses run one write at 000058 and say what it was for.
func accepts(t *testing.T, db *DB, what, stmt string, args ...any) {
	t.Helper()
	if _, err := db.Exec(stmt, args...); err != nil {
		t.Fatalf("%s was refused: %v", what, err)
	}
}

func refuses(t *testing.T, db *DB, what, reason, stmt string, args ...any) {
	t.Helper()
	if _, err := db.Exec(stmt, args...); err == nil || !strings.Contains(err.Error(), reason) {
		t.Fatalf("%s: got %v, want an error naming %q", what, err, reason)
	}
}

func TestMigration058KeepsTheQueueAndTheTimeline(t *testing.T) {
	db, migrateTo := captureDatabaseAt057(t)
	jobs, periods := jobRows(t, db), periodRows(t, db)
	borrowed := queryStrings(t, db, `SELECT job_id FROM lidar_capture_jobs WHERE kind = 'vrlog_record' AND session_id IS NOT NULL`)
	if strings.Join(borrowed, ",") != "job-unlinked" {
		t.Fatalf("clip jobs that kept a borrowed session: %v, want job-unlinked", borrowed)
	}

	migrateTo(58)
	if got := captureTriggers(t, db); got != captureTriggers058 {
		t.Fatalf("triggers at 58: %s", got)
	}
	if got := jobRows(t, db); !slices.Equal(got, jobs) {
		t.Fatalf("jobs changed:\n was %v\n now %v", jobs, got)
	}
	if got := periodRows(t, db); !slices.Equal(got, periods) {
		t.Fatalf("periods changed:\n was %v\n now %v", periods, got)
	}
	if problems := queryStrings(t, db, `SELECT "table" FROM pragma_foreign_key_check`); len(problems) != 0 {
		t.Fatalf("foreign key check: %v", problems)
	}
	if check := queryStrings(t, db, `PRAGMA integrity_check`); strings.Join(check, ",") != "ok" {
		t.Fatalf("integrity check: %v", check)
	}

	// The worker still moves those jobs through their states: a requeue after
	// a restart names neither kind nor session_id.
	accepts(t, db, "requeuing a clip job that kept a borrowed session",
		`UPDATE lidar_capture_jobs SET state = 'queued', started_at_ns = NULL WHERE job_id = ?`, borrowed[0])
	accepts(t, db, "clearing a borrowed session",
		`UPDATE lidar_capture_jobs SET session_id = NULL WHERE job_id = ?`, borrowed[0])
}

func TestMigration058RefusesUnknownKindsAndClipSessions(t *testing.T) {
	db, migrateTo := captureDatabaseAt057(t)
	migrateTo(58)
	const kind, clip = "motion_pass or a vrlog_record", "not in session_id"

	accepts(t, db, "a motion pass over a session",
		`INSERT INTO lidar_capture_jobs(job_id,kind,session_id,root_id,state,queued_at_ns) VALUES('job-pass','motion_pass','ses-1','root-1','queued',2)`)
	accepts(t, db, "a clip job without a session",
		`INSERT INTO lidar_capture_jobs(job_id,kind,state,queued_at_ns) VALUES('job-clip','vrlog_record','queued',2)`)
	refuses(t, db, "a job of another kind", kind,
		`INSERT INTO lidar_capture_jobs(job_id,kind,state,queued_at_ns) VALUES('job-sweep','sweep','queued',2)`)
	refuses(t, db, "a clip job that names a session", clip,
		`INSERT INTO lidar_capture_jobs(job_id,kind,session_id,state,queued_at_ns) VALUES('job-clip-2','vrlog_record','seg-ok','queued',2)`)
	refuses(t, db, "a job changed to another kind", kind,
		`UPDATE lidar_capture_jobs SET kind = 'sweep' WHERE job_id = 'job-pass'`)
	refuses(t, db, "a motion pass turned into a clip job", clip,
		`UPDATE lidar_capture_jobs SET kind = 'vrlog_record' WHERE job_id = 'job-pass'`)
	refuses(t, db, "a session given to a clip job", clip,
		`UPDATE lidar_capture_jobs SET session_id = 'ses-1' WHERE job_id = 'job-clip'`)
	accepts(t, db, "a motion pass moved to another session",
		`UPDATE lidar_capture_jobs SET session_id = 'ses-2' WHERE job_id = 'job-pass'`)
}

func TestMigration058RefusesPeriodsThatDisagreeWithTheirBounds(t *testing.T) {
	db, migrateTo := captureDatabaseAt057(t)
	migrateTo(58)
	const insert = `INSERT INTO lidar_capture_motion_periods(period_id,session_id,ordinal,period_type,start_ns,end_ns,duration_ns,start_secs,end_secs,created_at_ns)
		VALUES(?,'ses-1',?,'motion',?,?,?,0,0,3)`
	const backwards, askew = "ends no earlier than it starts", "lasts end_ns - start_ns"

	accepts(t, db, "a period of one instant", insert, "per-instant", 2, 300000000000, 300000000000, 0)
	refuses(t, db, "a period that ends before it starts", backwards, insert, "per-backwards", 3, 400000000000, 300000000000, -100000000000)
	refuses(t, db, "a duration that disagrees with the bounds", askew, insert, "per-askew", 4, 300000000000, 400000000000, 90000000000)
	refuses(t, db, "an end moved before the start", backwards,
		`UPDATE lidar_capture_motion_periods SET end_ns = start_ns - 1, duration_ns = -1 WHERE period_id = 'per-0'`)
	refuses(t, db, "a duration edited alone", askew,
		`UPDATE lidar_capture_motion_periods SET duration_ns = duration_ns + 1 WHERE period_id = 'per-0'`)
	accepts(t, db, "a period moved whole",
		`UPDATE lidar_capture_motion_periods SET start_ns = start_ns + 10, end_ns = end_ns + 10 WHERE period_id = 'per-0'`)
	accepts(t, db, "a period relabelled", `UPDATE lidar_capture_motion_periods SET label = 'static-0' WHERE period_id = 'per-0'`)
}

func TestMigration058RollsBackToTheRulesItFound(t *testing.T) {
	db, migrateTo := captureDatabaseAt057(t)
	migrateTo(58)
	jobs, periods := jobRows(t, db), periodRows(t, db)

	migrateTo(57)
	if got := captureTriggers(t, db); got != "" {
		t.Fatalf("the rollback left %s", got)
	}
	if got := jobRows(t, db); !slices.Equal(got, jobs) {
		t.Fatalf("jobs after the rollback:\n got %v\nwant %v", got, jobs)
	}
	if got := periodRows(t, db); !slices.Equal(got, periods) {
		t.Fatalf("periods after the rollback:\n got %v\nwant %v", got, periods)
	}
	// What 000058 refuses is accepted again.
	accepts(t, db, "a job of another kind at 57",
		`INSERT INTO lidar_capture_jobs(job_id,kind,state,queued_at_ns) VALUES('job-sweep','sweep','queued',2)`)

	// Rules for new writes only: going up again keeps the row they would refuse.
	migrateTo(58)
	if got := captureTriggers(t, db); got != captureTriggers058 {
		t.Fatalf("triggers at 58 a second time: %s", got)
	}
	if kinds := queryStrings(t, db, `SELECT kind FROM lidar_capture_jobs WHERE job_id = 'job-sweep'`); len(kinds) != 1 {
		t.Fatalf("the job written at 57 is gone: %v", kinds)
	}
}

func TestMigration058OnAnEmptyDatabase(t *testing.T) {
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
	if err := db.MigrateTo(migrations, 58); err != nil {
		t.Fatal(err)
	}
	if got := captureTriggers(t, db); got != captureTriggers058 {
		t.Fatalf("triggers: %s", got)
	}
}
