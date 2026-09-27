package segments

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func fixtureDB(t *testing.T, ddl string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestReadEstimatesRejectsAmbiguousVersions(t *testing.T) {
	db := fixtureDB(t, `CREATE TABLE lidar_track_estimates(estimate_id TEXT,creation_sequence INTEGER,frame_unix_nanos INTEGER,x REAL,y REAL,vx REAL,vy REAL,stage TEXT,source_id TEXT,estimator_id TEXT,observation_model_id TEXT,param_hash TEXT);`)
	insert := func(id, source, model string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO lidar_track_estimates VALUES(?,1,100,0,0,10,0,'online',?,'est',?,'hash')`, id, source, model); err != nil {
			t.Fatal(err)
		}
	}
	insert("a", "source-a", "model-a")
	insert("b", "source-b", "model-a")
	if _, _, err := LoadEstimates(db, "", "online"); err == nil {
		t.Fatal("accepted several sources")
	}
	points, source, err := LoadEstimates(db, "source-a", "online")
	if err != nil || source != "source-a" || len(points) != 1 {
		t.Fatalf("read: %q %+v %v", source, points, err)
	}
	insert("c", "source-a", "model-b")
	if _, _, err := LoadEstimates(db, "source-a", "online"); err == nil {
		t.Fatal("mixed estimator versions")
	}
}

func TestReadRunScopesFlagsAndIndexedCapture(t *testing.T) {
	db := fixtureDB(t, `CREATE TABLE lidar_run_tracks(run_id TEXT,track_id TEXT,is_split_candidate INTEGER,is_merge_candidate INTEGER);
CREATE TABLE lidar_tracks(track_id TEXT,max_speed_mps REAL);
CREATE TABLE lidar_track_observations(track_id TEXT,ts_unix_nanos INTEGER,frame_unix_nanos INTEGER,x REAL,y REAL,velocity_x REAL,velocity_y REAL);
CREATE TABLE lidar_capture_roots(root_id TEXT,path TEXT);
CREATE TABLE lidar_capture_files(root_id TEXT,rel_path TEXT,first_packet_ns INTEGER,last_packet_ns INTEGER,present INTEGER,probe_state TEXT);`)
	for _, q := range []string{
		`INSERT INTO lidar_tracks VALUES('a',10),('b',10)`,
		`INSERT INTO lidar_run_tracks VALUES('run-a','a',1,0),('run-b','b',0,0)`,
		`INSERT INTO lidar_track_observations VALUES('a',100,101,1,2,3,4),('b',100,101,5,6,7,8)`,
		`INSERT INTO lidar_capture_roots VALUES('root','/captures')`,
		`INSERT INTO lidar_capture_files VALUES('root','a.pcap',99,200,1,'ok'),('root','b.pcap',99,200,0,'ok')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	points, err := LoadRun(db, "run-a")
	if err != nil || len(points) != 1 || points[0].Track != "a" || !points[0].SplitFlag || points[0].TimeNs != 101 {
		t.Fatalf("run: %+v %v", points, err)
	}
	captures, err := CapturesForRange(db, 100, 150)
	if err != nil || len(captures) != 1 || captures[0].Path != "/captures/a.pcap" {
		t.Fatalf("captures: %+v %v", captures, err)
	}
}

func TestEstimateReaderSelectsOneSourceAndSkipsIncompleteRows(t *testing.T) {
	db := fixtureDB(t, `CREATE TABLE lidar_track_estimates(estimate_id TEXT,creation_sequence INTEGER,frame_unix_nanos INTEGER,x REAL,y REAL,vx REAL,vy REAL,stage TEXT,source_id TEXT,estimator_id TEXT,observation_model_id TEXT,param_hash TEXT);`)
	for _, q := range []string{
		`INSERT INTO lidar_track_estimates VALUES('a',1,100,0,0,10,0,'online','source','est','model','hash')`,
		`INSERT INTO lidar_track_estimates VALUES('b',2,200,NULL,0,10,0,'online','source','est','model','hash')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	points, source, err := LoadEstimates(db, "", "")
	if err != nil || source != "source" || len(points) != 1 || points[0].Track != "1" {
		t.Fatalf("automatic source: %q %+v %v", source, points, err)
	}
	if _, _, err := LoadEstimates(db, "missing", "online"); err == nil {
		t.Fatal("unknown source was accepted")
	}
	if _, err := db.Exec(`INSERT INTO lidar_track_estimates VALUES('c','bad',300,0,0,10,0,'online','source','est','model','hash')`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadEstimates(db, "source", "online"); err == nil {
		t.Fatal("unreadable creation sequence was accepted")
	}
}

func TestReadersReportMissingTablesAndUnreadableCaptureBounds(t *testing.T) {
	db := fixtureDB(t, `CREATE TABLE lidar_capture_roots(root_id TEXT,path TEXT);
CREATE TABLE lidar_capture_files(root_id TEXT,rel_path TEXT,first_packet_ns TEXT,last_packet_ns TEXT,present INTEGER,probe_state TEXT);`)
	if _, _, err := LoadEstimates(db, "source", "online"); err == nil {
		t.Fatal("missing estimate table went unnoticed")
	}
	if _, err := LoadRun(db, ""); err == nil {
		t.Fatal("empty run ID was accepted")
	}
	if _, err := LoadRun(db, "run"); err == nil {
		t.Fatal("missing run tables went unnoticed")
	}
	if _, err := db.Exec(`INSERT INTO lidar_capture_roots VALUES('root','/captures')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lidar_capture_files VALUES('root','bad.pcap','100','bad',1,'ok')`); err != nil {
		t.Fatal(err)
	}
	if _, err := CapturesForRange(db, 0, 300); err == nil {
		t.Fatal("unreadable capture bound was accepted")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadEstimates(db, "source", "online"); err == nil {
		t.Fatal("closed evidence database was accepted")
	}
}

func TestEstimateReaderReportsMalformedEvidenceSchema(t *testing.T) {
	t.Run("null source", func(t *testing.T) {
		db := fixtureDB(t, `CREATE TABLE lidar_track_estimates(source_id TEXT,stage TEXT); INSERT INTO lidar_track_estimates VALUES(NULL,'online');`)
		if _, _, err := LoadEstimates(db, "", "online"); err == nil {
			t.Fatal("null source was accepted")
		}
	})
	t.Run("missing version columns", func(t *testing.T) {
		db := fixtureDB(t, `CREATE TABLE lidar_track_estimates(source_id TEXT,stage TEXT); INSERT INTO lidar_track_estimates VALUES('s','online');`)
		if _, _, err := LoadEstimates(db, "s", "online"); err == nil {
			t.Fatal("missing version fields were accepted")
		}
	})
	t.Run("missing coordinate columns", func(t *testing.T) {
		db := fixtureDB(t, `CREATE TABLE lidar_track_estimates(source_id TEXT,stage TEXT,estimator_id TEXT,observation_model_id TEXT,param_hash TEXT); INSERT INTO lidar_track_estimates VALUES('s','online','e','m','h');`)
		if _, _, err := LoadEstimates(db, "s", "online"); err == nil {
			t.Fatal("missing coordinates were accepted")
		}
	})
}

func TestRunAndCaptureReadersReportMalformedEvidenceSchema(t *testing.T) {
	db := fixtureDB(t, `CREATE TABLE lidar_run_tracks(run_id TEXT,track_id TEXT,is_split_candidate INTEGER,is_merge_candidate INTEGER);
CREATE TABLE lidar_tracks(track_id TEXT,max_speed_mps REAL);
CREATE TABLE lidar_track_observations(track_id TEXT,ts_unix_nanos INTEGER,frame_unix_nanos TEXT,x REAL,y REAL,velocity_x REAL,velocity_y REAL);
INSERT INTO lidar_tracks VALUES('a',10);
INSERT INTO lidar_run_tracks VALUES('run','a',0,0);
INSERT INTO lidar_track_observations VALUES('a',100,'bad',1,2,3,4);`)
	if _, err := LoadRun(db, "run"); err == nil {
		t.Fatal("invalid run timestamp was accepted")
	}
	if _, err := CapturesForRange(db, 0, 100); err == nil {
		t.Fatal("missing capture index was accepted")
	}
}

func TestRunReaderReportsEvaluationErrorAfterFirstRow(t *testing.T) {
	db := fixtureDB(t, `CREATE TABLE lidar_run_tracks(run_id TEXT,track_id TEXT,is_split_candidate INTEGER,is_merge_candidate INTEGER);
CREATE TABLE lidar_tracks(track_id TEXT,max_speed_mps REAL);
CREATE TABLE raw_observations(track_id TEXT,ts_unix_nanos INTEGER,frame_unix_nanos INTEGER,payload TEXT);
CREATE VIEW lidar_track_observations AS SELECT track_id,ts_unix_nanos,frame_unix_nanos,json_extract(payload,'$.x') AS x,0 AS y,1 AS velocity_x,0 AS velocity_y FROM raw_observations;
INSERT INTO lidar_tracks VALUES('a',10);
INSERT INTO lidar_run_tracks VALUES('run','a',0,0);
INSERT INTO raw_observations VALUES('a',100,100,'{"x":1}'),('a',200,200,'invalid json');`)
	if _, err := LoadRun(db, "run"); err == nil {
		t.Fatal("late row evaluation error was ignored")
	}
}
