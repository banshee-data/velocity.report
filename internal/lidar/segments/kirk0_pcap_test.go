//go:build pcap

package segments

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// The Python queue is the reference until the Go finder has seen real kirk0
// estimates. Compare every window and all of its pairing measurements.
func TestKirk0FollowingMatchesReferenceScript(t *testing.T) {
	if testing.Short() {
		t.Skip("real PCAP replay")
	}
	pcap, err := filepath.Abs("../perf/pcap/kirk0.pcapng")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pcap); err != nil {
		t.Skip("kirk0 fixture unavailable")
	}
	dir := t.TempDir()
	evidence := filepath.Join(dir, "evidence.db")
	cfg := replayeval.Config{PCAPFile: pcap, OutDir: filepath.Join(dir, "vrlog"), SensorID: "kirk0-segments", UDPPort: 2369, StartSeconds: 40, DurationSeconds: 20, WarmupSeconds: 35, RequireSettled: true, ObservationDBPath: evidence, ReplayCaseID: "kirk0-segment-parity", ObservationMaxSamplePoints: 64}
	cfg.ObservationCalibration = l4bobserve.Calibration{SensorID: cfg.SensorID, FromFrame: "sensor", ToFrame: "site", Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
	result, err := replayeval.Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.OpenReadOnly(evidence)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	points, source, err := LoadEstimates(db, result.ObservationSourceID, "online")
	if err != nil {
		t.Fatal(err)
	}
	windows, err := Find(points, "following", source, "tuning", DefaultParams(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) == 0 {
		t.Fatal("kirk0 had no following windows")
	}
	output := filepath.Join(dir, "python.json")
	script, _ := filepath.Abs("../../../scripts/lidar-following-windows.py")
	cmd := exec.Command("python3", script, evidence, "--stage", "online", "--source", source, "--top", "0", "--json", output)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("reference script: %v\n%s", err, b)
	}
	b, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		Windows []struct {
			StartNs       int64   `json:"window_start_unix_nanos"`
			PairFrames    int     `json:"pair_frames"`
			PairSeconds   float64 `json:"pair_seconds"`
			Pairs         int     `json:"pairs"`
			Followers     int     `json:"followers"`
			Leaders       int     `json:"leaders"`
			LeaderChanges int     `json:"leader_changes"`
			ClosestGapM   float64 `json:"closest_gap_m"`
		} `json:"windows"`
	}
	if err := json.Unmarshal(b, &reference); err != nil {
		t.Fatal(err)
	}
	if len(reference.Windows) != len(windows) {
		t.Fatalf("Go %d windows, Python %d", len(windows), len(reference.Windows))
	}
	for i, w := range windows {
		r := reference.Windows[i]
		if w.StartNs != r.StartNs || w.PairFrames != r.PairFrames || w.PairSeconds != r.PairSeconds || w.Pairs != r.Pairs || w.Followers != r.Followers || w.Leaders != r.Leaders || w.LeaderChanges != r.LeaderChanges || w.ClosestGapM != r.ClosestGapM {
			t.Fatalf("window %d: Go %+v, Python %+v", i, w, r)
		}
	}
}
