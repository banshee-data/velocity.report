package main

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// groundTrack is one synthetic track: where it is and its lowest return at
// frame i.
type groundTrack struct {
	id     string
	state  l5tracks.TrackState
	misses int
	at     func(i int) (x, y, z, vx, vy float64)
}

// tapFrames feeds n frames at 10 Hz, the first warm of them unscored.
func tapFrames(g *groundReporter, n, warm int, tracks ...groundTrack) {
	t0 := time.Unix(1_700_000_000, 0)
	for i := 0; i < n; i++ {
		var objs []*l5tracks.TrackedObject
		for _, tr := range tracks {
			x, y, z, vx, vy := tr.at(i)
			o := &l5tracks.TrackedObject{TrackID: tr.id, X: float32(x), Y: float32(y), VX: float32(vx), VY: float32(vy),
				LatestZ: float32(z), Misses: tr.misses}
			o.TrackState = tr.state
			objs = append(objs, o)
		}
		f := replayeval.TrackerTapFrame{At: t0.Add(time.Duration(i) * 100 * time.Millisecond), Scored: i >= warm, Tracks: objs}
		g.observe(f)
		// A second tap in the same frame (AdvanceMisses) adds no sample.
		g.observe(f)
	}
}

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.6f, want %.6f ± %g", name, got, want, tol)
	}
}

// On a 4 % grade rising along +x, a vehicle driving along +x on the road has
// a lowest return that climbs 0.04 m a metre in the sensor frame and not at
// all above the surface: the surface explains it. One driving across the
// grade is flat in both, and the grade is along the first and across the
// second.
func TestGroundReportSeparatesTheGradeFromTheBody(t *testing.T) {
	plane := l3grid.GroundSurface{A: 0.04, C: -2.3, GradientMetre: 0.04, Support: 500}
	surface := &l3grid.RegionalGroundSurface{CellMetres: 10, Global: plane}
	along := groundTrack{id: "along", state: l5tracks.TrackConfirmed, at: func(i int) (float64, float64, float64, float64, float64) {
		x := -20 + float64(i)
		return x, 3, plane.HeightAt(x, 3), 10, 0
	}}
	across := groundTrack{id: "across", state: l5tracks.TrackConfirmed, at: func(i int) (float64, float64, float64, float64, float64) {
		y := -20 + float64(i)
		return 5, y, plane.HeightAt(5, y), 0, 10
	}}
	slow := groundTrack{id: "slow", state: l5tracks.TrackConfirmed, at: func(i int) (float64, float64, float64, float64, float64) {
		x := float64(i) * 0.1
		return x, -3, plane.HeightAt(x, -3), 1, 0
	}}
	tentative := groundTrack{id: "tentative", state: l5tracks.TrackTentative, at: along.at}
	g := newGroundReporter()
	tapFrames(g, 50, 10, along, across, slow, tentative)
	r := g.report(surface)
	s := r.Summary

	if s.ScoredFrames != 40 {
		t.Errorf("scored frames = %d, want 40", s.ScoredFrames)
	}
	if s.ConfirmedSeen != 3 || s.MovingTracks != 2 || len(r.Tracks) != 2 {
		t.Fatalf("confirmed %d, moving %d, rows %d; want 3, 2, 2", s.ConfirmedSeen, s.MovingTracks, len(r.Tracks))
	}
	byID := map[string]groundTrackRow{}
	for _, row := range r.Tracks {
		byID[row.TrackID] = row
	}
	a, c := byID["along"], byID["across"]
	if a.Samples != 40 {
		t.Errorf("along samples = %d, want 40 (unscored and duplicate taps left out)", a.Samples)
	}
	near(t, "along slope in the sensor frame", a.SlopeSensor, 0.04, 1e-6)
	near(t, "along slope above the surface", a.SlopeSurface, 0, 1e-6)
	near(t, "along grade", a.GradeAlong, 0.04, 1e-9)
	if a.Class != groundClassExplained {
		t.Errorf("along class = %s, want %s", a.Class, groundClassExplained)
	}
	near(t, "across slope in the sensor frame", c.SlopeSensor, 0, 1e-6)
	if c.Class != groundClassFlat {
		t.Errorf("across class = %s, want %s", c.Class, groundClassFlat)
	}
	near(t, "fall line bearing", s.Surface.FallLineBearingDeg, 180, 1e-9)
	near(t, "planar speed scale error", s.Surface.PlanarSpeedScaleError, 1-math.Cos(math.Atan(0.04)), 1e-12)
	near(t, "band height error at 30 m", s.Surface.BandHeightError30m, 1.2, 1e-9)
	// Two equal flows at right angles have no axis to speak of.
	near(t, "axis concentration", s.Course.AxisConcentration, 0, 1e-6)
	near(t, "grade along, median of 0.04 and 0", s.Course.GradeAlongMedian, 0.02, 1e-9)
	near(t, "grade across, median of 0 and 0.04", s.Course.GradeAcrossMedian, 0.02, 1e-9)
	if s.Classes[groundClassExplained] != 1 || s.Classes[groundClassFlat] != 1 || len(s.ListedTrackIDs) != 0 {
		t.Errorf("classes %v listed %v; want one explained, one flat, none listed", s.Classes, s.ListedTrackIDs)
	}
}

// A lowest return that climbs faster than the grade climbs above the surface
// too, and one that stays level in the sensor frame on a grade falls away
// from it: the surface explains neither, so both are listed, the larger
// change first. One flow is its own axis, and the fall line lies along it.
func TestGroundReportListsWhatTheSurfaceDoesNotExplain(t *testing.T) {
	plane := l3grid.GroundSurface{A: 0.04, C: -2.3, GradientMetre: 0.04, Support: 500}
	surface := &l3grid.RegionalGroundSurface{CellMetres: 10, Global: plane}
	climber := groundTrack{id: "climber", state: l5tracks.TrackConfirmed, at: func(i int) (float64, float64, float64, float64, float64) {
		x := -20 + float64(i)
		return x, 0, plane.HeightAt(x, 0) + 0.05*float64(i), 10, 0
	}}
	level := groundTrack{id: "level", state: l5tracks.TrackConfirmed, at: func(i int) (float64, float64, float64, float64, float64) {
		return -20 + float64(i), 4, -2.3, 10, 0
	}}
	g := newGroundReporter()
	tapFrames(g, 40, 0, level, climber)
	s := g.report(surface).Summary
	if s.Classes[groundClassBoth] != 1 || s.Classes[groundClassSurfaceOnly] != 1 {
		t.Fatalf("classes %v; want the climber in both and the level track above the surface only", s.Classes)
	}
	if len(s.ListedTrackIDs) != 2 || s.ListedTrackIDs[0] != "climber" || s.ListedTrackIDs[1] != "level" {
		t.Errorf("listed %v, want [climber level]", s.ListedTrackIDs)
	}
	near(t, "axis", s.Course.AxisDeg, 0, 1e-6)
	near(t, "axis concentration", s.Course.AxisConcentration, 1, 1e-9)
	near(t, "fall line to the axis", s.Course.FallLineToAxisDeg, 0, 1e-6)
}

// The departure is read where the moving tracks go: a cell raised 0.3 m off
// the global plane departs by 0.3 m there and by nothing elsewhere.
func TestGroundReportReadsTheCellPlanesWhereTheTracksGo(t *testing.T) {
	var points []l3grid.PointASC
	for x := 0; x < 100; x++ {
		for y := 0; y < 10; y++ {
			z := -3.0
			if x >= 50 && x < 60 {
				z = -2.7
			}
			points = append(points, l3grid.PointASC{X: float64(x) + 0.5, Y: float64(y) + 0.5, Z: z})
		}
	}
	surface, err := l3grid.FitRegionalGroundSurface(points, 10)
	if err != nil {
		t.Fatal(err)
	}
	drive := groundTrack{id: "drive", state: l5tracks.TrackConfirmed, at: func(i int) (float64, float64, float64, float64, float64) {
		x := 0.5 + float64(i)
		return x, 5, surface.HeightAt(x, 5), 10, 0
	}}
	g := newGroundReporter()
	tapFrames(g, 100, 0, drive)
	d := g.report(&surface).Summary.Departure
	if d.Samples != 100 {
		t.Fatalf("departure samples = %d, want 100", d.Samples)
	}
	near(t, "departure max", d.MaxMetres, 0.3, 1e-6)
	near(t, "departure p50", d.P50Metres, 0, 1e-6)
	near(t, "own-cell share", d.OwnCellShare, 1, 1e-9)
}

// Without a fitted surface the report still reads the tracks in the sensor
// frame, and explains nothing.
func TestGroundReportWithoutASurface(t *testing.T) {
	climber := groundTrack{id: "climber", state: l5tracks.TrackConfirmed, at: func(i int) (float64, float64, float64, float64, float64) {
		return float64(i), 0, -2.3 + 0.05*float64(i), 10, 0
	}}
	g := newGroundReporter()
	tapFrames(g, 30, 0, climber)
	s := g.report(nil).Summary
	if s.Surface != nil || s.Departure.Samples != 0 {
		t.Fatalf("surface %+v departure %+v; want neither without a fit", s.Surface, s.Departure)
	}
	if s.Classes[groundClassBoth] != 1 {
		t.Errorf("classes %v; want the climber unexplained", s.Classes)
	}
}

func TestGroundReportHelpers(t *testing.T) {
	near(t, "percentile 50 of 1..4", percentile([]float64{4, 1, 3, 2}, 50), 2.5, 1e-12)
	near(t, "percentile 100", percentile([]float64{4, 1, 3, 2}, 100), 4, 1e-12)
	near(t, "percentile of none", percentile(nil, 90), 0, 0)
	near(t, "separation of 10° and 190°", axialSeparationDeg(10*math.Pi/180, 190*math.Pi/180), 0, 1e-9)
	near(t, "separation of 0° and 135°", axialSeparationDeg(0, 135*math.Pi/180), 45, 1e-9)
	slope, se := slopeFit([]float64{0, 1, 2, 3}, []float64{1, 3, 5, 7})
	near(t, "slope", slope, 2, 1e-12)
	near(t, "standard error of an exact line", se, 0, 1e-12)
	if notableSlope(0.02, 0.001, 4) {
		t.Error("a 0.08 m change over 4 m counted as notable")
	}
	if !notableSlope(0.02, 0.001, 10) {
		t.Error("a 0.2 m change over 10 m at 20 standard errors not notable")
	}
	if notableSlope(0.02, 0.015, 10) {
		t.Error("a slope inside two standard errors counted as notable")
	}
}
