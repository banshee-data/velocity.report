package scene

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/lanes"
)

// Synthetic cars on a made-up plane: each moves along x (east) or against
// it for ten frames at its own speed.

const sceneT0 = int64(1_788_000_000_000_000_000)

func driveCars(acc *timelineAccumulator, prefix string, n int, vx, vy, y float64, firstMPH float64) {
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s%d", prefix, i)
		speed := (firstMPH + float64(i%10)) / mphPerMPS
		dx := vx * speed
		dy := vy * speed
		for f := 0; f < 10; f++ {
			ns := sceneT0 + int64(i)*int64(time.Second) + int64(f)*int64(100*time.Millisecond)
			x := -20 + dx*0.1*float64(f)
			if vx < 0 {
				x = 20 + dx*0.1*float64(f)
			}
			acc.observeAt(Frame{TimeUs: (ns - sceneT0) / 1000, Tracks: []TrackJSON{{
				ID: id, Class: "car", X: x, Y: y, VX: dx, VY: dy, Speed: speed, MaxSpeed: speed,
			}}}, ns)
		}
	}
}

func TestHeadingFallbackFindsTwoOpposedDirections(t *testing.T) {
	acc := newTimelineAccumulator(5)
	north := 0.0
	acc.sensorYBearing = &north // the sensor's +Y points north: +X is east
	driveCars(acc, "e", 30, 1, 0, -2, 20)
	driveCars(acc, "w", 20, -1, 0, 2, 25)
	driveCars(acc, "n", 3, 0, 1, 0, 15) // crossing traffic: in neither
	s := acc.summary().VehicleSpeed
	if s.TrackCount != 53 {
		t.Fatalf("headline %d tracks", s.TrackCount)
	}
	if len(s.Directions) != 2 {
		t.Fatalf("directions %+v", s.Directions)
	}
	byID := map[string]VehicleDirectionSummary{}
	for _, d := range s.Directions {
		byID[d.ID] = d
	}
	e, w := byID["heading:e"], byID["heading:w"]
	if e.Name != "eastbound" || e.TrackCount != 30 || e.Source != DirectionSourceHeading || e.SpeedLimit != nil || *e.BearingDeg != 90 {
		t.Fatalf("east %+v", e)
	}
	if w.Name != "westbound" || w.TrackCount != 20 || *w.BearingDeg != 270 {
		t.Fatalf("west %+v", w)
	}
	// Under fifty tracks the tails are not published.
	if e.P50 == nil || e.P85 == nil || e.P98 != nil || e.Max != nil {
		t.Fatalf("east tails %+v", e)
	}
	if len(e.Histogram) == 0 {
		t.Fatal("no histogram")
	}
}

func TestHeadingFallbackWithoutAPoseOrASecondMode(t *testing.T) {
	acc := newTimelineAccumulator(5)
	driveCars(acc, "e", 30, 1, 0, -2, 20)
	driveCars(acc, "w", 20, -1, 0, 2, 25)
	s := acc.summary().VehicleSpeed
	names := []string{}
	for _, d := range s.Directions {
		names = append(names, d.Name)
		if d.BearingDeg != nil {
			t.Fatalf("a bearing without a pose: %+v", d)
		}
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "Direction A,Direction B" {
		t.Fatalf("names %v", names)
	}
	one := newTimelineAccumulator(5)
	driveCars(one, "e", 40, 1, 0, -2, 20)
	driveCars(one, "w", 3, -1, 0, 2, 25) // under a fifth and under five
	if got := one.summary().VehicleSpeed.Directions; len(got) != 0 {
		t.Fatalf("one mode made directions: %+v", got)
	}
}

func TestDirectionsFromTheLaneSnapshot(t *testing.T) {
	lane := func(id string, x0, y0, x1, y1 float64, limit string) lanes.Feature {
		return lanes.Feature{FeatureID: id, RowID: "r", Kind: lanes.KindTravelLane, XYZ: []float64{x0, y0, 0, x1, y1, 0},
			Tags: map[string]string{"street": "KIRKHAM ST", "width_m": "3", "speed_limit": limit, "speed_limit_unit": "mph", "speed_limit_source": "sign", "speed_limit_effective_from": "2026-01-01"}}
	}
	snap, err := lanes.Build("808580f47", lanes.Request{
		RequestID: "r1", Writer: lanes.WriterPainter, SensorID: "s1", RecordID: "gms:rec",
		Pose: lanes.Pose{Rotation: [4]float64{1, 0, 0, 0}}, Author: "op",
		Features: []lanes.Feature{lane("gms:e", -30, -2, 30, -2, "25"), lane("gms:w", 30, 2, -30, 2, "30")},
	}, time.Unix(0, sceneT0-int64(time.Hour)), "")
	if err != nil {
		t.Fatal(err)
	}
	acc := newTimelineAccumulator(5)
	acc.lanes = lanes.NewHistory([]lanes.Snapshot{snap})
	driveCars(acc, "e", 30, 1, 0, -2, 20)
	driveCars(acc, "w", 20, -1, 0, 2, 25)
	driveCars(acc, "x", 6, 1, 0, 15, 20) // off every lane: in the headline only
	s := acc.summary().VehicleSpeed
	if s.TrackCount != 56 || len(s.Directions) != 2 {
		t.Fatalf("headline %d, directions %+v", s.TrackCount, s.Directions)
	}
	e := s.Directions[0]
	if e.ID != "kirkham-st:e" || e.Name != "Kirkham St eastbound" || e.Source != DirectionSourceLanes || e.TrackCount != 30 || e.SpeedLimit == nil || e.SpeedLimit.Value != 25 {
		t.Fatalf("eastbound %+v", e)
	}
	if w := s.Directions[1]; w.SpeedLimit == nil || w.SpeedLimit.Value != 30 || w.TrackCount != 20 {
		t.Fatalf("westbound %+v", w)
	}
}

// The export privacy case (D-34): a direction summary carries aggregates
// only, never a track id or a per-track speed.
func TestDirectionSummariesCarryNoTrackData(t *testing.T) {
	acc := newTimelineAccumulator(5)
	north := 0.0
	acc.sensorYBearing = &north
	driveCars(acc, "secret-e", 60, 1, 0, -2, 20)
	driveCars(acc, "secret-w", 55, -1, 0, 2, 25)
	b, err := json.Marshal(acc.summary().VehicleSpeed.Directions)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret") {
		t.Fatal("a track id reached the direction summaries")
	}
	var dirs []map[string]any
	if err := json.Unmarshal(b, &dirs); err != nil || len(dirs) != 2 {
		t.Fatalf("decode: %v %d", err, len(dirs))
	}
	allowed := map[string]bool{"id": true, "name": true, "bearing_deg": true, "source": true, "speed_limit": true, "track_count": true,
		"p50": true, "p85": true, "p98": true, "max": true, "histogram": true}
	for _, d := range dirs {
		for k := range d {
			if !allowed[k] {
				t.Fatalf("direction summary carries %q", k)
			}
		}
		for _, bucket := range d["histogram"].([]any) {
			for k := range bucket.(map[string]any) {
				if k != "start_mph" && k != "count" {
					t.Fatalf("histogram bucket carries %q", k)
				}
			}
		}
	}
}
