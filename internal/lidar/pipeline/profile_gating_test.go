package pipeline

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
)

// profileTracker observes which stages a profile actually reached. It counts
// the two calls that mark the L4→L5 and L5→L6 boundaries: RecordFrameStats is
// made only once clustering has run, and UpdateClassification only once the
// classifier has labelled a track.
type profileTracker struct {
	mockTrackerCov
	classifyCalls int
}

func (p *profileTracker) UpdateClassification(trackID, objectClass string, confidence float32, model string) {
	p.classifyCalls++
}

// newProfileTracker returns a tracker carrying one confirmed track with enough
// observations to be classified, so the L6 boundary is reachable without
// having to drive a real tracker to confirmation.
func newProfileTracker() *profileTracker {
	pt := &profileTracker{}
	pt.confirmedTracks = []*l5tracks.TrackedObject{{
		TrackID:          "profile-track-1",
		TrackMeasurement: l5tracks.TrackMeasurement{ObservationCount: 10},
	}}
	return pt
}

// runProfileFrames drives a settled background model and then one frame
// carrying foreground, which is the shortest path that reaches every gate.
func runProfileFrames(t *testing.T, profile config.Profile) *profileTracker {
	t.Helper()

	sensorID := "profile-" + string(profile) + "-" + t.Name()
	tracker := newProfileTracker()

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: makeTestBgManager(t, sensorID),
		Tracker:           tracker,
		Classifier:        l6objects.NewTrackClassifier(),
		RemoveGround:      true,
		Profile:           profile,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}
	cb(makeForegroundFrame("fg-1", now.Add(600*time.Millisecond), 20.0, 5.0))

	return tracker
}

// TestProfileGatesStopThePipelineAtTheRightLayer is the test the failed CI
// baseline should have had. A baseline recorded against a pipeline that
// silently stopped at L3 read as an 8-second "full pipeline" run for three
// months; these assertions are what make each profile's depth observable
// rather than inferred from timings.
func TestProfileGatesStopThePipelineAtTheRightLayer(t *testing.T) {
	tests := []struct {
		profile      config.Profile
		wantClusters bool // RecordFrameStats: clustering ran (L4)
		wantTracking bool // Update: tracker consumed clusters (L5)
		wantClassify bool // UpdateClassification: classifier labelled (L6)
	}{
		{config.ProfileL3Only, false, false, false},
		{config.ProfileDetect, true, false, false},
		{config.ProfileTrack, true, true, false},
		{config.ProfileFull, true, true, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.profile), func(t *testing.T) {
			tracker := runProfileFrames(t, tc.profile)

			if got := tracker.frameStatsCalls > 0; got != tc.wantClusters {
				t.Errorf("clustering ran = %v, want %v (RecordFrameStats calls: %d)",
					got, tc.wantClusters, tracker.frameStatsCalls)
			}
			if got := tracker.updateCalls > 0; got != tc.wantTracking {
				t.Errorf("tracking ran = %v, want %v (Update calls: %d)",
					got, tc.wantTracking, tracker.updateCalls)
			}
			if got := tracker.classifyCalls > 0; got != tc.wantClassify {
				t.Errorf("classification ran = %v, want %v (UpdateClassification calls: %d)",
					got, tc.wantClassify, tracker.classifyCalls)
			}
		})
	}
}

// TestUnsetProfileRunsEverything pins the back-compatibility promise. Every
// TrackingPipelineConfig built before profiles existed leaves the field at its
// zero value, and must keep running the whole stack.
func TestUnsetProfileRunsEverything(t *testing.T) {
	tracker := runProfileFrames(t, "")

	if tracker.frameStatsCalls == 0 {
		t.Error("an unset profile skipped clustering")
	}
	if tracker.updateCalls == 0 {
		t.Error("an unset profile skipped tracking")
	}
	if tracker.classifyCalls == 0 {
		t.Error("an unset profile skipped classification")
	}
}

// TestForegroundStillRunsUnderL3Only checks the l3-only gate stops the
// pipeline *after* the background model has done its work, not before. A
// profile that skipped the background update would leave the grid unsettled
// and be useless for the background tuning it exists to serve.
func TestForegroundStillRunsUnderL3Only(t *testing.T) {
	sensorID := "l3-only-settles-" + t.Name()
	bgMgr := makeTestBgManager(t, sensorID)

	cfg := &TrackingPipelineConfig{
		SensorID:          sensorID,
		BackgroundManager: bgMgr,
		Tracker:           newProfileTracker(),
		Profile:           config.ProfileL3Only,
	}
	cb := cfg.NewFrameCallback()

	now := time.Now()
	for i := 0; i < 5; i++ {
		cb(makeStableFrame("seed-"+string(rune('A'+i)), now.Add(time.Duration(i)*100*time.Millisecond), 20.0))
	}

	if !bgMgr.IsSettlingComplete() {
		t.Error("the background model did not settle under l3-only; L3 must still run in full")
	}
	if bgMgr.Grid == nil {
		t.Fatal("l3-only left no background grid")
	}
}
