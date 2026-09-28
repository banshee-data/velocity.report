package segments

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite the ranking golden file under testdata")

// goldenSeries gives every finder something to rank: a lane whose follower
// changes leader, a second lane that follows for longer, one lateral jump,
// split flags in two windows, and two captures to draw random windows from.
func goldenSeries() ([]Point, []Capture) {
	points := laneWithLeaderChange("near", 0, 10)
	for _, track := range []struct {
		id string
		x0 float64
	}{{"follower", 0}, {"leader", 10}} {
		lane := moving(track.id, 30, track.x0, 200)
		for i := range lane {
			lane[i].Y = 20
		}
		points = append(points, lane...)
	}
	jump := moving("jump", 5, 0, 400)
	for i := range jump {
		jump[i].Y = 100
	}
	jump[2].Y = 101
	points = append(points, jump...)
	for _, flagged := range []struct {
		id           string
		first, count int
	}{{"split-a", 500, 3}, {"split-b", 600, 2}} {
		for i := 0; i < flagged.count; i++ {
			points = append(points, Point{Track: flagged.id, TimeNs: testBase + int64(flagged.first+i)*100_000_000, Y: 200, SplitFlag: true})
		}
	}
	captures := []Capture{
		{Path: "/captures/a.pcap", FirstNs: testBase - 40_000_000_000, LastNs: testBase + 75_000_000_000},
		{Path: "/captures/b.pcap", FirstNs: testBase + 100_000_000_000, LastNs: testBase + 180_000_000_000},
	}
	return points, captures
}

// The golden file pins what every finder returns for one series: which
// windows, in what order, with what identity, score and measurements. Other
// ways of ranking are built on Find, so a change here moves the identity of
// windows that people have already chosen and cut. Review the diff, then
// regenerate with go test ./internal/lidar/segments -run Golden -update.
func TestEveryFinderRanksTheGoldenSeriesAsBefore(t *testing.T) {
	points, captures := goldenSeries()
	runs := map[string]struct {
		finder, role string
		captures     []Capture
	}{
		"following as tuning":                   {"following", "tuning", captures},
		"following as held_out":                 {"following", "held_out", captures},
		"leader_changes as tuning":              {"leader_changes", "tuning", captures},
		"lateral_jump as tuning":                {"lateral_jump", "tuning", captures},
		"split_flags as tuning":                 {"split_flags", "tuning", captures},
		"exposure as tuning":                    {"exposure", "tuning", captures},
		"exposure as held_out":                  {"exposure", "held_out", captures},
		"random as tuning":                      {"random", "tuning", captures},
		"random as held_out":                    {"random", "held_out", captures},
		"random as tuning, without the capture": {"random", "tuning", nil},
	}
	got := map[string][]Window{}
	for name, run := range runs {
		windows, err := Find(points, run.finder, "golden", run.role, DefaultParams(), run.captures)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(windows) == 0 {
			t.Fatalf("%s ranked nothing, so the series no longer pins it", name)
		}
		got[name] = windows
	}
	b, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	path := filepath.Join("testdata", "rankings.golden.json")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (regenerate with go test ./internal/lidar/segments -run Golden -update)", err)
	}
	if !bytes.Equal(want, b) {
		t.Errorf("%s differs from what the finders return now: review the change, then regenerate "+
			"with go test ./internal/lidar/segments -run Golden -update", path)
	}
}
