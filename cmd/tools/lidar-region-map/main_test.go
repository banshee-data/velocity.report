package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

// evidenceWithMap writes a region map for a two-ring, four-bin grid (90
// degree bins, ring elevations 0 and 30 degrees, every cell at 10 m except
// cell 2 at 20 m) into a fresh evidence database, once per reason given.
func evidenceWithMap(t *testing.T, reasons ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "observations.db")
	database, err := db.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	bg := l3grid.NewBackgroundManagerDI("map-tool", 2, 4, l3grid.BackgroundParams{NoiseRelativeFraction: 0.01}, nil)
	if err := bg.SetRingElevations([]float64{0, 30}); err != nil {
		t.Fatal(err)
	}
	for i := range bg.Grid.Cells {
		bg.Grid.Cells[i] = l3grid.BackgroundCell{AverageRangeMeters: 10, TimesSeenCount: 40}
		bg.Grid.RegionMgr.SettlingMetrics.VariancePerCell[i] = float64(i)
	}
	bg.Grid.Cells[2].AverageRangeMeters = 20
	bg.Grid.RegionMgr.SettlingMetrics.FramesSampled = 30
	if err := bg.Grid.RegionMgr.IdentifyRegions(bg.Grid, 50); err != nil {
		t.Fatal(err)
	}
	for i, reason := range reasons {
		if err := bg.PersistAt(database, reason, time.Unix(int64(100+i), 0)); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestDumpWritesOneRowPerCellInTheSensorFrame(t *testing.T) {
	path := evidenceWithMap(t, l3grid.SnapshotReasonRegionsIdentified, l3grid.SnapshotReasonReplayEnd)
	out := filepath.Join(t.TempDir(), "regions.csv")
	var stdout bytes.Buffer
	if err := run([]string{"-db", path, "-out", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "reason=regions_identified") ||
		!strings.Contains(stdout.String(), "cells by own tercile: stable=2 variable=3 volatile=3") {
		t.Fatalf("summary:\n%s", stdout.String())
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 9 {
		t.Fatalf("got %d rows, want a header and 8 cells", len(rows))
	}
	col := map[string]int{}
	for i, name := range rows[0] {
		col[name] = i
	}
	num := func(row []string, name string) float64 {
		v, err := strconv.ParseFloat(row[col[name]], 64)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return v
	}
	near := func(name string, got, want float64) {
		if math.Abs(got-want) > 1e-6 {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
	// Cell 2: ring 0, bin 2, centre 225 degrees, 20 m, level.
	c := rows[1+2]
	near("azimuth_deg", num(c, "azimuth_deg"), 225)
	near("elevation_deg", num(c, "elevation_deg"), 0)
	near("x_m", num(c, "x_m"), 20*math.Sin(225*math.Pi/180))
	near("y_m", num(c, "y_m"), 20*math.Cos(225*math.Pi/180))
	// Cell 5: ring 1, bin 1, centre 135 degrees, 10 m at 30 degrees up.
	c = rows[1+5]
	if c[col["ring"]] != "1" || c[col["azimuth_bin"]] != "1" {
		t.Fatalf("cell 5 is ring %s bin %s", c[col["ring"]], c[col["azimuth_bin"]])
	}
	h := 10 * math.Cos(30*math.Pi/180)
	near("x_m", num(c, "x_m"), h*math.Sin(135*math.Pi/180))
	near("y_m", num(c, "y_m"), h*math.Cos(135*math.Pi/180))
	near("z_m", num(c, "z_m"), 5)
	near("settling_variance", num(c, "settling_variance"), 5)
	if c[col["cell_tercile"]] != "volatile" || c[col["region_id"]] == "-1" {
		t.Fatalf("cell 5: tercile %q region %s", c[col["cell_tercile"]], c[col["region_id"]])
	}
}

func TestDumpJSONCarriesProvenance(t *testing.T) {
	path := evidenceWithMap(t, l3grid.SnapshotReasonReplayEnd)
	out := filepath.Join(t.TempDir(), "regions.json")
	if err := run([]string{"-db", path, "-reason", l3grid.SnapshotReasonReplayEnd, "-out", out}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		RegionSetID    int64             `json:"region_set_id"`
		SnapshotReason string            `json:"snapshot_reason"`
		Rings          int               `json:"rings"`
		Cells          []json.RawMessage `json:"cells"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.RegionSetID != 1 || got.SnapshotReason != l3grid.SnapshotReasonReplayEnd || got.Rings != 2 || len(got.Cells) != 8 {
		t.Fatalf("JSON map: %+v", got)
	}
}

func TestDumpRefusesAnAmbiguousSelection(t *testing.T) {
	path := evidenceWithMap(t, l3grid.SnapshotReasonRegionsIdentified, l3grid.SnapshotReasonRegionsIdentified)
	out := filepath.Join(t.TempDir(), "regions.csv")
	err := run([]string{"-db", path, "-out", out}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "2 region sets") || !strings.Contains(err.Error(), "region_set=2") {
		t.Fatalf("ambiguous selection: %v", err)
	}
	if err := run([]string{"-db", path, "-region-set", "2", "-out", out}, &bytes.Buffer{}); err != nil {
		t.Fatalf("named region set: %v", err)
	}
	var list bytes.Buffer
	if err := run([]string{"-db", path, "-list"}, &list); err != nil || strings.Count(list.String(), "region_set=") != 2 {
		t.Fatalf("list: %v\n%s", err, list.String())
	}
	if err := run([]string{"-db", filepath.Join(t.TempDir(), "missing.db"), "-list"}, &bytes.Buffer{}); err == nil {
		t.Fatal("a missing database was opened")
	}
}
