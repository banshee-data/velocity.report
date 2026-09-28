//go:build pcap

package lidar

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	coredb "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func cliEvidence(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "evidence.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE lidar_track_estimates(source_id TEXT,stage TEXT,estimator_id TEXT,observation_model_id TEXT,param_hash TEXT,creation_sequence INTEGER,frame_unix_nanos INTEGER,x REAL,y REAL,vx REAL,vy REAL,estimate_id INTEGER)`,
		`CREATE TABLE lidar_capture_roots(root_id TEXT,path TEXT)`,
		`CREATE TABLE lidar_capture_files(root_id TEXT,rel_path TEXT,first_packet_ns INTEGER,last_packet_ns INTEGER,present INTEGER,probe_state TEXT)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	const base int64 = 1788466680 * 1_000_000_000
	capture := filepath.Join(dir, "capture.pcap")
	if err := os.WriteFile(capture, []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lidar_capture_roots VALUES('root',?)`, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO lidar_capture_files VALUES('root','capture.pcap',?,?,1,'ok')`, base-1_000_000_000, base+80_000_000_000); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		for track, x := range []float64{float64(i), float64(i) + 10} {
			if _, err := db.Exec(`INSERT INTO lidar_track_estimates VALUES('source','online','estimator','model','hash',?,?,?,0,10,0,?)`, track+1, base+int64(i)*100_000_000, x, i*2+track); err != nil {
				t.Fatal(err)
			}
		}
	}
	for track, x := range []float64{0, 10} {
		if _, err := db.Exec(`INSERT INTO lidar_track_estimates VALUES('source','online','estimator','model','hash',?,?,?,0,10,0,?)`, track+1, base+10_000_000_000, x, 100+track); err != nil {
			t.Fatal(err)
		}
	}
	return path, capture
}

func captureCommandJSON(t *testing.T, fn func() int) []byte {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "output-*.json")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = file
	defer func() { os.Stdout = old }()
	if code := fn(); code != 0 {
		t.Fatalf("command exited %d", code)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSegmentsCLIReportsTrafficAndTrackerFreeRandomWindows(t *testing.T) {
	db, capture := cliEvidence(t)
	var report struct {
		Windows []segments.Window `json:"windows"`
	}
	b := captureCommandJSON(t, func() int {
		return SegmentsMain([]string{"--db", db, "--source", "source", "--finder", "following", "--capture", capture, "--top", "1"})
	})
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Windows) != 1 || report.Windows[0].PairFrames != 5 || report.Windows[0].Capture != capture {
		t.Fatalf("following report: %+v", report.Windows)
	}
	b = captureCommandJSON(t, func() int {
		return SegmentsMain([]string{"--db", db, "--source", "source", "--finder", "following", "--top", "0"})
	})
	if err := json.Unmarshal(b, &report); err != nil || len(report.Windows) != 2 {
		t.Fatalf("unlimited indexed windows: %+v %v", report.Windows, err)
	}
	b = captureCommandJSON(t, func() int {
		return SegmentsMain([]string{"--db", db, "--source", "source", "--finder", "random", "--role", "held_out", "--capture", capture})
	})
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Windows) != 1 || report.Windows[0].OffsetSeconds < 35 {
		t.Fatalf("random report: %+v", report.Windows)
	}
	if code := silence(t, func() int {
		return SegmentsMain([]string{"--db", db, "--source", "source", "--finder", "random", "--role", "held_out"})
	}); code != 2 {
		t.Fatalf("unscoped random selection exited %d", code)
	}
}

func TestSegmentsCLIRejectsBadQueries(t *testing.T) {
	db, _ := cliEvidence(t)
	samplePCAP, err := filepath.Abs("../../lidar/l1packets/parse/sample_packet.pcapng")
	if err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"bad flag":         {"--not-a-flag"},
		"missing database": {"--finder", "following"},
		"negative top":     {"--db", db, "--source", "source", "--top", "-1"},
		"mixed selectors":  {"--db", db, "--run", "run", "--source", "source"},
		"missing file":     {"--db", filepath.Join(t.TempDir(), "absent.db"), "--source", "source"},
		"unknown source":   {"--db", db, "--source", "other"},
		"unknown finder":   {"--db", db, "--source", "source", "--finder", "unknown"},
		"relative capture": {"--db", db, "--source", "source", "--finder", "random", "--capture", "relative.pcap"},
		"missing capture":  {"--db", db, "--source", "source", "--finder", "random", "--capture", filepath.Join(t.TempDir(), "missing.pcap")},
		"short capture":    {"--db", db, "--source", "source", "--finder", "random", "--capture", samplePCAP},
	} {
		t.Run(name, func(t *testing.T) {
			code := silence(t, func() int { return SegmentsMain(args) })
			if code == 0 {
				t.Fatal("invalid query succeeded")
			}
		})
	}
	if code := silence(t, func() int { return SegmentsMain([]string{"--help"}) }); code != 0 {
		t.Fatalf("help exited %d", code)
	}
	output, err := os.CreateTemp(t.TempDir(), "closed-stdout")
	if err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = output
	code := SegmentsMain([]string{"--db", db, "--source", "source", "--finder", "following"})
	os.Stdout = old
	if code != 1 {
		t.Fatalf("closed report output exited %d", code)
	}
	if code := silence(t, func() int {
		return segmentsMainWithOpen([]string{"--db", db}, func(string) (*sqlite.SQLDB, error) {
			return nil, errors.New("cannot open database")
		})
	}); code != 1 {
		t.Fatalf("database open failure exited %d", code)
	}
	writer, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`DROP TABLE lidar_capture_files`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if code := silence(t, func() int {
		return SegmentsMain([]string{"--db", db, "--source", "source", "--finder", "following"})
	}); code != 1 {
		t.Fatalf("broken capture index exited %d", code)
	}
}

func TestSegmentsCLIReadsRunObservations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`CREATE TABLE lidar_run_tracks(run_id TEXT,track_id TEXT,is_split_candidate INTEGER,is_merge_candidate INTEGER)`,
		`CREATE TABLE lidar_tracks(track_id TEXT,max_speed_mps REAL)`,
		`CREATE TABLE lidar_track_observations(track_id TEXT,ts_unix_nanos INTEGER,frame_unix_nanos INTEGER,x REAL,y REAL,velocity_x REAL,velocity_y REAL)`,
		`CREATE TABLE lidar_capture_roots(root_id TEXT,path TEXT)`,
		`CREATE TABLE lidar_capture_files(root_id TEXT,rel_path TEXT,first_packet_ns INTEGER,last_packet_ns INTEGER,present INTEGER,probe_state TEXT)`,
		`INSERT INTO lidar_tracks VALUES('a',10),('b',10)`,
		`INSERT INTO lidar_run_tracks VALUES('run','a',0,0),('run','b',0,0)`,
		`INSERT INTO lidar_track_observations VALUES('a',100000000000,100000000000,0,0,10,0),('b',100000000000,100000000000,10,0,10,0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	b := captureCommandJSON(t, func() int { return SegmentsMain([]string{"--db", path, "--run", "run", "--finder", "following"}) })
	var report struct {
		Windows []segments.Window `json:"windows"`
	}
	if err := json.Unmarshal(b, &report); err != nil || len(report.Windows) != 1 {
		t.Fatalf("run report: %+v %v", report.Windows, err)
	}
	if code := silence(t, func() int { return SegmentsMain([]string{"--db", path, "--run", "run", "--finder", "random"}) }); code == 0 {
		t.Fatal("random run without source capture succeeded")
	}
}

func TestSegmentsCLIRandomRunUsesItsOwnIndexedCapture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.db")
	capture := filepath.Join(dir, "capture.pcap")
	if err := os.WriteFile(capture, []byte("fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	database, err := coredb.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	const base int64 = 1788466680 * 1_000_000_000
	for _, q := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO lidar_capture_roots(root_id,path,created_at_ns,updated_at_ns) VALUES('root',?,1,1)`, []any{dir}},
		{`INSERT INTO lidar_capture_files(capture_file_id,root_id,rel_path,size_bytes,modified_at_ns,first_packet_ns,last_packet_ns,probe_state,first_seen_at_ns,last_seen_at_ns) VALUES('file','root','capture.pcap',7,1,?,?, 'ok',1,1)`, []any{base, base + 80_000_000_000}},
		{`INSERT INTO lidar_run_records(run_id,created_at,source_type,source_path,sensor_id,status,duration_secs,total_frames,total_clusters,total_tracks,confirmed_tracks,processing_time_ms) VALUES('run',1,'pcap',?,'sensor','completed',0,0,0,0,0,0)`, []any{capture}},
	} {
		if _, err := database.Exec(q.query, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	b := captureCommandJSON(t, func() int {
		return SegmentsMain([]string{"--db", path, "--run", "run", "--finder", "random", "--role", "held_out"})
	})
	var report struct {
		Windows []segments.Window `json:"windows"`
	}
	if err := json.Unmarshal(b, &report); err != nil || len(report.Windows) != 1 || report.Windows[0].Capture != capture {
		t.Fatalf("run capture placement: %+v %v", report.Windows, err)
	}
}

func TestSegmentsCLIPlacesRandomWindowFromCaptureWithoutAnIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("real capture probe")
	}
	pcap, err := filepath.Abs("../../lidar/perf/pcap/kirk0.pcapng")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pcap); err != nil {
		t.Skip("kirk0 fixture unavailable")
	}
	db, _ := cliEvidence(t)
	b := captureCommandJSON(t, func() int {
		return SegmentsMain([]string{"--db", db, "--source", "source", "--finder", "random", "--role", "held_out", "--capture", pcap})
	})
	var report struct {
		Windows []segments.Window `json:"windows"`
	}
	if err := json.Unmarshal(b, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Windows) != 1 || report.Windows[0].Capture != pcap || report.Windows[0].OffsetSeconds < 35 {
		t.Fatalf("unindexed capture placement: %+v", report.Windows)
	}
}

func TestNamespaceReachesSegmentCommands(t *testing.T) {
	for _, command := range []string{"segments", "annotation-clip"} {
		if code := silence(t, func() int { return Main([]string{command, "-h"}) }); code != 0 {
			t.Errorf("Main(%s -h) = %d, want 0", command, code)
		}
		if code := silence(t, func() int { return Main([]string{command, "-nope"}) }); code != 2 {
			t.Errorf("Main(%s -nope) = %d, want 2", command, code)
		}
	}
}

// A replayed run keeps its tracks in its recording and stores no
// observations, so the command must read the recording to rank it.
func TestSegmentsCLIReadsAReplayedRunFromItsRecording(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run.db")
	recording := filepath.Join(dir, "recording")
	const base int64 = 1788466680 * 1_000_000_000
	rec, err := recorder.NewRecorder(recording, "sensor")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		ts := base + int64(i)*100_000_000
		tracks := []l9endpoints.Track{
			{TrackID: "follower", State: l9endpoints.TrackStateConfirmed, X: float32(i), VX: 10, MaxSpeedMps: 10},
			{TrackID: "leader", State: l9endpoints.TrackStateConfirmed, X: float32(i) + 10, VX: 10, MaxSpeedMps: 10},
		}
		if err := rec.Record(&l9endpoints.FrameBundle{FrameID: uint64(i), TimestampNanos: ts, SensorID: "sensor", Tracks: &l9endpoints.TrackSet{TimestampNanos: ts, Tracks: tracks}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := coredb.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	insertRun := func(id, vrlog string) {
		t.Helper()
		if _, err := database.Exec(`INSERT INTO lidar_run_records(run_id,created_at,source_type,source_path,sensor_id,status,duration_secs,total_frames,total_clusters,total_tracks,confirmed_tracks,processing_time_ms,vrlog_path) VALUES(?,1,'pcap','/captures/a.pcap','sensor','completed',0,0,0,0,0,0,?)`, id, vrlog); err != nil {
			t.Fatal(err)
		}
	}
	insertRun("replayed", recording)
	insertRun("lost", filepath.Join(dir, "absent"))
	insertRun("bare", "")
	if _, err := database.Exec(`INSERT INTO lidar_run_tracks(run_id,track_id,sensor_id,track_state,start_unix_nanos,is_split_candidate) VALUES('replayed','leader','sensor','confirmed',?,1)`, base); err != nil {
		t.Fatal(err)
	}
	report := func(args ...string) []segments.Window {
		t.Helper()
		b := captureCommandJSON(t, func() int { return SegmentsMain(append([]string{"--db", path}, args...)) })
		var out struct {
			Windows []segments.Window `json:"windows"`
		}
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out.Windows
	}
	if windows := report("--run", "replayed", "--finder", "following"); len(windows) != 1 || windows[0].PairFrames != 5 || windows[0].Source != "replayed" {
		t.Fatalf("replayed run was not ranked from its recording: %+v", windows)
	}
	if windows := report("--run", "replayed", "--finder", "split_flags"); len(windows) != 1 || windows[0].Events != 5 {
		t.Fatalf("split marks of a replayed run: %+v", windows)
	}
	// Nothing stored and nothing recorded is an empty ranking, not a fault.
	if windows := report("--run", "bare", "--finder", "following"); len(windows) != 0 {
		t.Fatalf("run without a series: %+v", windows)
	}
	for name, run := range map[string]string{"recording removed": "lost", "unknown run": "missing"} {
		if code := silence(t, func() int { return SegmentsMain([]string{"--db", path, "--run", run, "--finder", "following"}) }); code != 1 {
			t.Errorf("%s: exit %d, want 1", name, code)
		}
	}
}
