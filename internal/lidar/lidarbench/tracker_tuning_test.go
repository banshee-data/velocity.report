//go:build pcap

package lidarbench

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// TestTrackerFollowsTheTuningConfig checks the L5 block of the supplied config
// is the one the benchmark's tracker and classifier are built from. The tracker
// was once built from the defaults file whatever config was passed, so a
// benchmark of a tuning arm timed the default L5 parameters while its document
// carried the arm's fingerprint.
func TestTrackerFollowsTheTuningConfig(t *testing.T) {
	tuning := config.MustLoadDefaultConfig()
	l5 := tuning.L5.CvKfV1
	l5.MaxTracks += 7
	l5.HitsToConfirm += 3
	l5.GatingDistanceSquared *= 2
	l5.MinObservationsForClassification += 4

	fb := newAnalysisFrameBuilder(Config{SensorID: "test", Tuning: tuning}, &result{})

	got := fb.tracker.Config
	if got.MaxTracks != l5.MaxTracks || got.HitsToConfirm != l5.HitsToConfirm ||
		got.GatingDistanceSquared != float32(l5.GatingDistanceSquared) {
		t.Errorf("tracker built with max_tracks=%d hits_to_confirm=%d gating_distance_squared=%g, "+
			"want the supplied %d, %d, %g",
			got.MaxTracks, got.HitsToConfirm, got.GatingDistanceSquared,
			l5.MaxTracks, l5.HitsToConfirm, l5.GatingDistanceSquared)
	}
	if want := l5tracks.TrackerConfigFromTuning(l5); !reflect.DeepEqual(got, want) {
		t.Errorf("tracker config differs from TrackerConfigFromTuning of the supplied block:\n got %+v\nwant %+v", got, want)
	}
	if fb.classifier.MinObservations != l5.MinObservationsForClassification {
		t.Errorf("classifier min_observations = %d, want the supplied %d",
			fb.classifier.MinObservations, l5.MinObservationsForClassification)
	}
}

// TestTrackerTuningReachesTheMeasuredWorkload drives the fixture capture twice
// and checks an L5 value changes what the benchmark measures, not only how the
// tracker is configured. No track can be confirmed when confirmation needs
// more hits than the replay has frames.
func TestTrackerTuningReachesTheMeasuredWorkload(t *testing.T) {
	cfg := benchConfig(t)
	_, defaults, err := runBenchmark(cfg)
	if err != nil {
		t.Fatalf("default run: %v", err)
	}
	if defaults.Work.DistinctConfirmedTracks == 0 {
		t.Fatal("the default run confirmed no tracks; the fixture cannot show the difference")
	}

	cfg = benchConfig(t)
	cfg.Tuning.L5.CvKfV1.HitsToConfirm = defaults.Work.Frames + 1
	_, unconfirmable, err := runBenchmark(cfg)
	if err != nil {
		t.Fatalf("run with hits_to_confirm=%d: %v", cfg.Tuning.L5.CvKfV1.HitsToConfirm, err)
	}
	if n := unconfirmable.Work.DistinctConfirmedTracks; n != 0 {
		t.Errorf("hits_to_confirm=%d still confirmed %d tracks (defaults confirmed %d); "+
			"the benchmark is not running the supplied L5 block",
			cfg.Tuning.L5.CvKfV1.HitsToConfirm, n, defaults.Work.DistinctConfirmedTracks)
	}
}

// TestReducedProfilesBuildWithoutAnL5Block covers the committed profiles that
// switch L5 off. They carry no cv_kf_v1 block and the active L5 block is
// zero, so the builder must not dereference it; the zero tracker it builds is
// never run, because the profile gate stops before L5.
func TestReducedProfilesBuildWithoutAnL5Block(t *testing.T) {
	for _, name := range []string{"l3-only", "detect"} {
		t.Run(name, func(t *testing.T) {
			tuning, err := config.LoadTuningConfig(filepath.Join("..", "..", "..", "config", "profiles", name+".json"))
			if err != nil {
				t.Fatalf("load profile %s: %v", name, err)
			}
			if tuning.L5.CvKfV1 != nil || tuning.Profile().RunsLayer(5) {
				t.Fatalf("profile %s runs L5; this test needs one that does not", name)
			}

			fb := newAnalysisFrameBuilder(Config{SensorID: "test", Tuning: tuning}, &result{})
			if fb.tracker == nil || fb.classifier == nil {
				t.Fatal("builder left the tracker or classifier nil")
			}
			if !reflect.DeepEqual(fb.tracker.Config, l5tracks.TrackerConfig{}) {
				t.Errorf("tracker config = %+v without an L5 block, want the zero config", fb.tracker.Config)
			}
			if fb.profile.RunsLayer(5) {
				t.Errorf("builder profile %s runs L5", fb.profile)
			}
		})
	}
}
