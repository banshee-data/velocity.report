//go:build pcap

package lidar

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	coredb "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// laneFieldDB writes a database holding one synthetic run of five cars.
func laneFieldDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sensor.db")
	d, err := coredb.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := sqlite.NewRunStore(d).InsertRun(&sqlite.Run{RunID: "run-1", SensorID: "s1", SourceType: "pcap", SourcePath: "/captures/a.pcapng", Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	const t0 = int64(1_788_000_000_000_000_000)
	exec(`UPDATE lidar_runs SET frame_start_ns = ?, frame_end_ns = ? WHERE run_id = 'run-1'`, t0, t0+10_000_000_000)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("car-%d", i)
		exec(`INSERT INTO lidar_tracks (track_id, sensor_id, frame_id, track_state, start_unix_nanos) VALUES (?, 's1', 'f', 'confirmed', ?)`, id, t0)
		exec(`INSERT INTO lidar_run_tracks (run_id, track_id, sensor_id, track_state, start_unix_nanos, object_class) VALUES ('run-1', ?, 's1', 'confirmed', ?, 'truck')`, id, t0)
		for j := 0; j < 20; j++ {
			ts := t0 + int64(i)*1_000_000_000 + int64(j)*100_000_000
			exec(`INSERT INTO lidar_track_observations (track_id, ts_unix_nanos, frame_id, x, y, velocity_x, velocity_y, speed_mps, frame_unix_nanos, box_x, box_y, box_heading_rad, box_length, box_width)
				VALUES (?, ?, 'f', -3.1, ?, -7, 0, 7, ?, -3.1, ?, 3.14159, 6, 2.4)`, id, ts, 2+0.1*float64(j), ts, 2+0.1*float64(j))
		}
	}
	return path
}

func TestLaneFieldCLIWritesTheRoutesBytes(t *testing.T) {
	dbPath := laneFieldDB(t)
	out := filepath.Join(t.TempDir(), "fields")
	if code := LaneFieldMain([]string{"--db", dbPath, "--run", "run-1", "--out", out}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	ro, err := sqlite.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	for _, p := range l8analytics.LaneFieldParts {
		got, err := os.ReadFile(filepath.Join(out, "run-1.lane-field."+string(p)+".json"))
		if err != nil {
			t.Fatal(err)
		}
		f, err := sqlite.NewRunStore(ro).RunLaneField("run-1", p)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := f.JSON()
		if !bytes.Equal(got, want) {
			t.Fatalf("part %s: the file differs from the route's bytes", p)
		}
		if p == l8analytics.LaneFieldPartAll && (f.CellCount() == 0 || f.Header.TrackCount != 5) {
			t.Fatalf("whole run: %d cells, %d tracks", f.CellCount(), f.Header.TrackCount)
		}
	}
}

func TestLaneFieldCLIOnePartAndErrors(t *testing.T) {
	dbPath := laneFieldDB(t)
	out := t.TempDir()
	if code := LaneFieldMain([]string{"--db", dbPath, "--run", "run-1", "--out", out, "--part", "second_half"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	entries, _ := os.ReadDir(out)
	if len(entries) != 1 || entries[0].Name() != "run-1.lane-field.second_half.json" {
		t.Fatalf("wrote %v", entries)
	}
	for _, c := range []struct {
		args []string
		want int
	}{
		{[]string{"--db", dbPath, "--run", "run-1"}, 2},
		{[]string{"--db", dbPath, "--run", "../x", "--out", out}, 2},
		{[]string{"--db", dbPath, "--run", "run-1", "--out", out, "--part", "middle"}, 2},
		{[]string{"--db", dbPath, "--run", "nope", "--out", out}, 1},
		{[]string{"-h"}, 0},
	} {
		if code := LaneFieldMain(c.args); code != c.want {
			t.Errorf("%v: exit %d, want %d", c.args, code, c.want)
		}
	}
}
