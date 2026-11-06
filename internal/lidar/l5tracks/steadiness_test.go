package l5tracks

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestSteadinessWindowEligibilityAndReset(t *testing.T) {
	tk := NewTracker(DefaultTrackerConfig())
	tr := &TrackedObject{TrackState: TrackConfirmed, HeadingJitterCount: 99}
	observe := func() {
		tk.observeBaselineSpeedJitter(tr, 2)
		tk.observeBaselineSpeedJitter(tr, 4)
		tk.observeBaselineHeadingJitter(tr, 0.1)
		tk.observeBaselineHeadingJitter(tr, 0.3)
		tk.observeBaselineCourse(tr, 0)
		tk.observeBaselineCourse(tr, 9)
	}
	observe()
	if tk.GetWindowBaseline().Steadiness != nil {
		t.Fatal("closed window produced steadiness")
	}
	tk.BeginTrackingBaseline()
	tr.TrackState = TrackTentative
	observe()
	empty := tk.GetWindowBaseline().Steadiness
	if empty.HeadingJitterDeg != nil || empty.SpeedJitterMps != nil || empty.CourseAlignmentP50Deg != nil || empty.CourseAlignmentP90Deg != nil || empty.SpeedJitterSamples != 0 {
		t.Fatalf("tentative updates appeared measured: %+v", empty)
	}
	tr.TrackState = TrackConfirmed
	observe()
	s := tk.GetWindowBaseline().Steadiness
	if s.HeadingJitterSamples != 2 || s.SpeedJitterSamples != 2 || s.CourseAlignmentSamples != 2 ||
		math.Abs(*s.HeadingJitterDeg-math.Sqrt(0.05)*180/math.Pi) > 1e-10 || math.Abs(*s.SpeedJitterMps-math.Sqrt(10)) > 1e-10 ||
		*s.CourseAlignmentP50Deg != CourseAlignmentBinWidthDeg || *s.CourseAlignmentP90Deg != 10*CourseAlignmentBinWidthDeg {
		t.Fatalf("incorrect pooled measures: %+v", s)
	}
	*s.SpeedJitterMps = 999
	if *tk.GetWindowBaseline().Steadiness.SpeedJitterMps == 999 {
		t.Fatal("summary aliases tracker state")
	}
	tk.BeginTrackingBaseline()
	if tk.GetWindowBaseline().Steadiness.SpeedJitterMps != nil || tr.HeadingJitterCount != 99 {
		t.Fatal("window reset kept samples or changed lifetime metrics")
	}
	observe()
	tk.Reset()
	if tk.GetWindowBaseline().Steadiness != nil || tk.baselineSteadiness.speedCount != 0 {
		t.Fatal("tracker reset retained the window")
	}
}

func TestSteadinessWindowUsesActualUpdatesAndRetainsEndedTracks(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.HitsToConfirm = 2
	cfg.MaxMissesConfirmed = 3
	cfg.DeletedTrackGracePeriod = 0
	scene := uncScene{frames: 40, period: 100 * time.Millisecond, seed: 3,
		bodies: []uncBody{uncCar(-30, 15, 8)}, noise: uncNoise{0.03, 0.08}}
	frames := scene.render()
	tk, plain := NewTracker(cfg), NewTracker(cfg)
	var headingStart, speedStart, courseStart int
	var headingSq, speedSq float64
	for i, frame := range frames {
		if i == 10 {
			tr := tk.GetActiveTracks()[0]
			headingStart, speedStart, courseStart = tr.HeadingJitterCount, tr.SpeedJitterCount, tr.CourseAlignmentCount
			headingSq, speedSq = tr.HeadingJitterSumSq, tr.SpeedJitterSumSq
			tk.BeginTrackingBaseline()
		}
		tk.Update(frame.clusters, frame.at)
		plain.Update(frame.clusters, frame.at)
		a, b := tk.GetActiveTracks(), plain.GetActiveTracks()
		if len(a) != 1 || len(b) != 1 || a[0].X != b[0].X || a[0].Y != b[0].Y || a[0].VX != b[0].VX || a[0].VY != b[0].VY {
			t.Fatalf("metrics changed the estimate at frame %d", i)
		}
	}
	tr := tk.GetActiveTracks()[0]
	s := tk.GetWindowBaseline().Steadiness
	if s.SpeedJitterSamples != tr.SpeedJitterCount-speedStart || s.HeadingJitterSamples != tr.HeadingJitterCount-headingStart ||
		s.CourseAlignmentSamples != tr.CourseAlignmentCount-courseStart || s.SpeedJitterSamples == 0 || s.CourseAlignmentSamples == 0 {
		t.Fatalf("window differs from observed update deltas: %+v", s)
	}
	if math.Abs(*s.SpeedJitterMps-math.Sqrt((tr.SpeedJitterSumSq-speedSq)/float64(s.SpeedJitterSamples))) > 1e-10 ||
		(s.HeadingJitterSamples > 0 && math.Abs(*s.HeadingJitterDeg-math.Sqrt((tr.HeadingJitterSumSq-headingSq)/float64(s.HeadingJitterSamples))*180/math.Pi) > 1e-10) {
		t.Fatal("window does not pool the original innovations")
	}
	for i := 1; i < 12; i++ {
		tk.Update(nil, frames[len(frames)-1].at.Add(time.Duration(i)*100*time.Millisecond))
	}
	if len(tk.Tracks) != 0 || !reflect.DeepEqual(s, tk.GetWindowBaseline().Steadiness) {
		t.Fatal("coasting or pruning changed the observed window")
	}
}
