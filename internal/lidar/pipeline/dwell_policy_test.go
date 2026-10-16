package pipeline

import (
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// The dwell tests run the whole callback, L3 to L5, on a settled background
// at 100 m, with the shipped L3 and L5 parameters (region overrides off). An
// object face 1.4 m wide and six channels tall stands in front of it. The
// background is settled and locked, with a count far above the freeze
// threshold, so L3 alone keeps the object foreground for the whole run; only
// the dwell policy can absorb it.

const (
	dwellBackgroundMetres = 100.0
	dwellObjectMetres     = 8.0
	dwellFramePeriod      = 100 * time.Millisecond
)

var dwellEpoch = time.Date(2026, time.October, 13, 0, 0, 0, 0, time.UTC)

func dwellFrameTime(frame int) time.Time {
	return dwellEpoch.Add(time.Duration(frame) * dwellFramePeriod)
}

type dwellScene struct {
	t        *testing.T
	bm       *l3grid.BackgroundManager
	tracker  *l5tracks.Tracker
	callback func(*l2frames.LiDARFrame)
	rng      *rand.Rand
	frame    int
}

func newDwellScene(t *testing.T, dwell *DwellPolicy) *dwellScene {
	t.Helper()
	params := l3grid.BackgroundConfigFromActiveTuning(config.MustLoadDefaultConfig()).ToBackgroundParams()
	params.DisableRegionOverrides = true
	bm := l3grid.NewBackgroundManagerDI("dwell-test", 40, 1800, params, nil)
	if bm == nil {
		t.Fatal("NewBackgroundManagerDI returned nil")
	}
	bm.SetReplayMode(true)
	for i := range bm.Grid.Cells {
		c := &bm.Grid.Cells[i]
		c.AverageRangeMeters, c.RangeSpreadMeters, c.TimesSeenCount = dwellBackgroundMetres, 0.05, 1000
		c.LockedBaseline, c.LockedSpread, c.LockedAtCount = dwellBackgroundMetres, 0.05, 1000
	}
	bm.Grid.SettlingComplete = true
	tracker := l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
	cfg := &TrackingPipelineConfig{BackgroundManager: bm, Tracker: tracker, SensorID: "dwell-test", Dwell: dwell}
	return &dwellScene{t: t, bm: bm, tracker: tracker, callback: cfg.NewFrameCallback(), rng: rand.New(rand.NewSource(1))}
}

// objectReturns is the object at the given range: channels 1 to 6 at
// elevations -5 to 0 degrees, azimuths 90 to 99.5 degrees in half-degree
// steps, each return with 1 cm of range noise. Even channels return 0.6 m
// deeper, so the object has a body (about 1.4 by 0.6 m in plan) rather than
// being a flat face, whose few centimetres of depth would straddle L4's
// thin-cluster aspect rule and drop it from some frames.
func (s *dwellScene) objectReturns(rangeMetres float64, ts time.Time) []l2frames.Point {
	pts := make([]l2frames.Point, 0, 120)
	for ch := 1; ch <= 6; ch++ {
		depth := 0.6 * float64(1-ch%2)
		for k := 0; k < 20; k++ {
			pts = append(pts, l2frames.Point{
				Channel: ch, Elevation: float64(ch) - 6, Azimuth: 90.05 + 0.5*float64(k),
				Distance: rangeMetres + depth + s.rng.NormFloat64()*0.01, Intensity: 60, Timestamp: ts,
			})
		}
	}
	return pts
}

// step plays one frame with the object at rangeMetres.
func (s *dwellScene) step(rangeMetres float64) {
	ts := dwellFrameTime(s.frame)
	pts := s.objectReturns(rangeMetres, ts)
	s.callback(&l2frames.LiDARFrame{
		FrameID: fmt.Sprintf("dwell-%d", s.frame), StartTimestamp: ts,
		Points: pts, PolarPoints: pointsToPolar(pts),
	})
	s.frame++
}

// frameState is what a frame left behind that the policy could change.
type frameState struct {
	foreground int64
	confirmed  int
	x, y       float32
}

func (s *dwellScene) state() frameState {
	st := frameState{foreground: s.bm.Grid.ForegroundCount}
	for _, track := range s.tracker.GetConfirmedTracks() {
		st.confirmed++
		st.x, st.y = track.X, track.Y
	}
	return st
}

// objectCell is the grid cell of the object's first return.
func (s *dwellScene) objectCell() l3grid.BackgroundCell {
	azBin := int(90.05 / 360 * float64(s.bm.Grid.AzimuthBins))
	return s.bm.Grid.Cells[s.bm.Grid.Idx(0, azBin)]
}

func absorbedDeleted(tracker *l5tracks.Tracker) int {
	n := 0
	for _, track := range tracker.GetAllTracks() {
		if track.ExpiryReason == l5tracks.ExpiryAbsorbed {
			n++
		}
	}
	return n
}

func TestDwellPolicy_AbsorbsAStationaryTrackPastT(t *testing.T) {
	dwell := &DwellPolicy{}
	s := newDwellScene(t, dwell)
	frames := int((DwellAbsorbAfter+3*time.Second)/dwellFramePeriod) + 1
	for i := 0; i < frames; i++ {
		s.step(dwellObjectMetres)
	}
	r := dwell.Report()
	if r.TracksAbsorbed != 1 || len(r.Absorptions) != 1 {
		t.Fatalf("absorbed %d tracks (%d records), want 1: %+v", r.TracksAbsorbed, len(r.Absorptions), r)
	}
	a := r.Absorptions[0]
	if a.StationarySeconds < DwellAbsorbAfter.Seconds() || a.StationarySeconds > DwellAbsorbAfter.Seconds()+0.15 {
		t.Errorf("absorbed after %.2f s stationary, want the first observed frame at or past %v", a.StationarySeconds, DwellAbsorbAfter)
	}
	if a.Returns != 120 || a.Cells != 120 || r.CellsAbsorbed != 120 || r.ReturnsAbsorbed != 120 {
		t.Errorf("absorbed %d returns into %d cells, want the object's 120 returns in 120 cells", a.Returns, a.Cells)
	}
	if r.Skipped != 0 {
		t.Errorf("%d due tracks skipped", r.Skipped)
	}
	if got := absorbedDeleted(s.tracker); got != 1 {
		t.Errorf("%d tracks ended as absorbed, want 1", got)
	}
	if c := s.objectCell(); math.Abs(float64(c.AverageRangeMeters)-dwellObjectMetres) > 0.05 {
		t.Errorf("object cell average %.2f m, want the object's %.0f m", c.AverageRangeMeters, dwellObjectMetres)
	}
	// From the frame after absorption on, the object is background: no
	// foreground, no clusters, and no ghost track born in its place.
	decided := int(time.Duration(a.FrameUnixNanos-dwellEpoch.UnixNano()) / dwellFramePeriod)
	if decided+1 >= frames {
		t.Fatalf("premise: absorption at frame %d leaves no frame after it", decided)
	}
	for i := 0; i < 30; i++ {
		s.step(dwellObjectMetres)
		if st := s.state(); st.foreground != 0 || st.confirmed != 0 {
			t.Fatalf("frame %d, after absorption at frame %d: %d foreground returns and %d confirmed tracks, want none",
				s.frame-1, decided, st.foreground, st.confirmed)
		}
	}
	if active := s.tracker.GetActiveTracks(); len(active) != 0 {
		t.Errorf("%d active tracks after absorption, want none", len(active))
	}
	if got := s.tracker.ContinuityStats().ExpiredByReason.Absorbed; got != 1 {
		t.Errorf("continuity counts %d absorbed expiries, want 1", got)
	}

	// A second replay of the same frames absorbs the same way.
	again := &DwellPolicy{}
	s2 := newDwellScene(t, again)
	for i := 0; i < frames+30; i++ {
		s2.step(dwellObjectMetres)
	}
	if !reflect.DeepEqual(again.Report(), dwell.Report()) {
		t.Errorf("a repeat replay absorbed differently:\n got %+v\nwant %+v", again.Report(), dwell.Report())
	}
}

func TestDwellPolicy_LeavesAMovingTrack(t *testing.T) {
	dwell := &DwellPolicy{}
	s := newDwellScene(t, dwell)
	frames := int((DwellAbsorbAfter+5*time.Second)/dwellFramePeriod) + 1
	sawConfirmed := false
	for i := 0; i < frames; i++ {
		// Away from the sensor at 1 m/s, twice StationarySpeedMps.
		s.step(dwellObjectMetres + 0.1*float64(i))
		sawConfirmed = sawConfirmed || s.state().confirmed > 0
	}
	if !sawConfirmed {
		t.Fatal("premise: the moving object was never a confirmed track")
	}
	if r := dwell.Report(); r.TracksAbsorbed != 0 || r.Skipped != 0 {
		t.Errorf("a moving track was due: %+v", r)
	}
	if got := absorbedDeleted(s.tracker); got != 0 {
		t.Errorf("%d moving tracks ended as absorbed", got)
	}
}

func TestDwellPolicy_LeavesAStationaryTrackUnderT(t *testing.T) {
	dwell := &DwellPolicy{}
	s := newDwellScene(t, dwell)
	frames := int((DwellAbsorbAfter - 5*time.Second) / dwellFramePeriod)
	for i := 0; i < frames; i++ {
		s.step(dwellObjectMetres)
	}
	if r := dwell.Report(); r.TracksAbsorbed != 0 {
		t.Errorf("a track stationary for under %v was absorbed: %+v", DwellAbsorbAfter, r)
	}
	if st := s.state(); st.confirmed != 1 || st.foreground == 0 {
		t.Errorf("after %d frames: %d confirmed tracks and %d foreground returns, want the object still tracked", frames, st.confirmed, st.foreground)
	}
	if c := s.objectCell(); c.AverageRangeMeters != dwellBackgroundMetres {
		t.Errorf("object cell average %.2f m, want the untouched background %.0f m", c.AverageRangeMeters, dwellBackgroundMetres)
	}
}

// Off, the policy changes nothing: the stationary object stays foreground and
// tracked past T. On, it changes nothing before it acts: every frame up to
// and including the deciding one leaves the same foreground and tracks.
func TestDwellPolicy_OffChangesNothing(t *testing.T) {
	frames := int((DwellAbsorbAfter+3*time.Second)/dwellFramePeriod) + 1
	off := newDwellScene(t, nil)
	dwell := &DwellPolicy{}
	on := newDwellScene(t, dwell)
	var offStates, onStates []frameState
	for i := 0; i < frames; i++ {
		off.step(dwellObjectMetres)
		on.step(dwellObjectMetres)
		offStates = append(offStates, off.state())
		onStates = append(onStates, on.state())
	}
	if st := off.state(); st.confirmed != 1 || st.foreground == 0 {
		t.Errorf("off, after %d frames: %d confirmed tracks and %d foreground returns, want the object still tracked", frames, st.confirmed, st.foreground)
	}
	if got := absorbedDeleted(off.tracker); got != 0 {
		t.Errorf("off: %d tracks ended as absorbed", got)
	}
	if got := off.tracker.ContinuityStats().ExpiredByReason.Absorbed; got != 0 {
		t.Errorf("off: continuity counts %d absorbed expiries", got)
	}
	if c := off.objectCell(); c.AverageRangeMeters != dwellBackgroundMetres {
		t.Errorf("off: object cell average %.2f m, want the untouched background", c.AverageRangeMeters)
	}

	r := dwell.Report()
	if r.TracksAbsorbed != 1 {
		t.Fatalf("premise: on, %d tracks absorbed, want 1", r.TracksAbsorbed)
	}
	decided := int(time.Duration(r.Absorptions[0].FrameUnixNanos-dwellEpoch.UnixNano()) / dwellFramePeriod)
	if !reflect.DeepEqual(onStates[:decided], offStates[:decided]) {
		t.Errorf("the policy changed a frame before it acted (decided at frame %d)", decided)
	}
	// The deciding frame's foreground was computed before the policy ran;
	// the track it ended is gone by the time the state is read.
	if onStates[decided].foreground != offStates[decided].foreground || onStates[decided].confirmed != 0 {
		t.Errorf("deciding frame: on %+v, off %+v; want the same foreground and the track ended", onStates[decided], offStates[decided])
	}
	if next := onStates[decided+1]; next.foreground != 0 || next.confirmed != 0 {
		t.Errorf("the frame after the decision still has the object: %+v", next)
	}
}

func TestInsideBox(t *testing.T) {
	box := &l4perception.OrientedBoundingBox{CenterX: 10, CenterY: 5, CenterZ: -2, Length: 4, Width: 2, Height: 1.5, HeadingRad: math.Pi / 2}
	cases := []struct {
		name    string
		x, y, z float64
		margin  float64
		want    bool
	}{
		{"centre", 10, 5, -1, 0, true},
		{"along the length, rotated onto +Y", 10, 6.9, -1, 0, true},
		{"past the length", 10, 7.1, -1, 0, false},
		{"past the length, inside the margin", 10, 7.1, -1, 0.2, true},
		{"across the width, rotated onto -X", 9.1, 5, -1, 0, true},
		{"past the width", 8.9, 5, -1, 0, false},
		{"below the lowest point", 10, 5, -2.1, 0, false},
		{"below, inside the margin", 10, 5, -2.1, 0.2, true},
		{"above the top", 10, 5, -0.4, 0, false},
		{"above, inside the margin", 10, 5, -0.4, 0.2, true},
	}
	for _, c := range cases {
		if got := insideBox(box, l4perception.WorldPoint{X: c.x, Y: c.y, Z: c.z}, c.margin); got != c.want {
			t.Errorf("%s: insideBox = %v, want %v", c.name, got, c.want)
		}
	}
}

// A longer T (the dwell_policy_t40 and _t60 replay experiments) holds the
// same object past 20 s and absorbs it at the first observed frame at or past
// its own T, and the report says which T ran.
func TestDwellPolicy_AbsorbsAtItsOwnT(t *testing.T) {
	for _, after := range []time.Duration{40 * time.Second, 60 * time.Second} {
		t.Run(after.String(), func(t *testing.T) {
			dwell := &DwellPolicy{AbsorbAfter: after}
			s := newDwellScene(t, dwell)
			before := int((after - time.Second) / dwellFramePeriod)
			for i := 0; i < before; i++ {
				s.step(dwellObjectMetres)
			}
			if r := dwell.Report(); r.TracksAbsorbed != 0 {
				t.Fatalf("absorbed before its T of %v (the default is %v): %+v", after, DwellAbsorbAfter, r)
			}
			if st := s.state(); st.confirmed != 1 || st.foreground == 0 {
				t.Fatalf("after %d frames: %d confirmed tracks and %d foreground returns, want the object still tracked",
					before, st.confirmed, st.foreground)
			}
			for i := before; i < int((after+3*time.Second)/dwellFramePeriod)+1; i++ {
				s.step(dwellObjectMetres)
			}
			r := dwell.Report()
			if r.AbsorbAfterSeconds != after.Seconds() {
				t.Errorf("report says T = %g s, want the %g s it ran at", r.AbsorbAfterSeconds, after.Seconds())
			}
			if r.TracksAbsorbed != 1 || len(r.Absorptions) != 1 {
				t.Fatalf("absorbed %d tracks (%d records), want 1: %+v", r.TracksAbsorbed, len(r.Absorptions), r)
			}
			if got := r.Absorptions[0].StationarySeconds; got < after.Seconds() || got > after.Seconds()+0.15 {
				t.Errorf("absorbed after %.2f s stationary, want the first observed frame at or past %v", got, after)
			}
			if got := absorbedDeleted(s.tracker); got != 1 {
				t.Errorf("%d tracks ended as absorbed, want 1", got)
			}
		})
	}
}

// A report carries the policy's constants and a list that marshals as [],
// never null, when nothing was absorbed. Its T is the one the policy runs
// at; the other constants are the same at every T.
func TestDwellPolicy_ReportCarriesItsConstants(t *testing.T) {
	r := (&DwellPolicy{}).Report()
	if r.AbsorbAfterSeconds != DwellAbsorbAfter.Seconds() || r.StationarySpeedMps != l5tracks.StationarySpeedMps ||
		r.StationaryDriftMetres != l5tracks.StationaryDriftMetres || r.BoxMarginMetres != dwellBoxMarginMetres {
		t.Errorf("report constants %+v", r)
	}
	if r.Absorptions == nil {
		t.Error("an empty report's absorptions are nil and would marshal as null")
	}
	if explicit := (&DwellPolicy{AbsorbAfter: DwellAbsorbAfter}).Report(); !reflect.DeepEqual(explicit, r) {
		t.Errorf("T set to the default reports %+v, want the default's %+v", explicit, r)
	}
	for _, after := range []time.Duration{40 * time.Second, 60 * time.Second} {
		got := (&DwellPolicy{AbsorbAfter: after}).Report()
		want := r
		want.AbsorbAfterSeconds = after.Seconds()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("T %v: report %+v, want %+v", after, got, want)
		}
	}
}
