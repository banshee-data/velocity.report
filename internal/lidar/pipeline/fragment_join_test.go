package pipeline

import (
	"math"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func TestMotionHintsRequireRecentObservedMovingVehicles(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cfg := config.L4FragmentJoin{Enabled: true, MaxGapMetres: 2, MarginMetres: .15, MaxAgeSecs: .2, MinSpeedMps: 2, MaxPositionStddevMetres: .75}
	for _, name := range []string{"accepted", "tentative", "coasting", "stopped", "stale", "backwards", "pedestrian", "uncertain", "nonfinite", "weak-direction", "process-noise-bound", "coupled-noise-bound", "invalid-noise"} {
		t.Run(name, func(t *testing.T) {
			tr := &l5tracks.TrackedObject{TrackID: "vehicle", CreationSequence: 1, TrackMeasurement: l5tracks.TrackMeasurement{TrackState: l5tracks.TrackConfirmed, ObjectClass: "car", ObservationCount: 20},
				StateUnixNanos: now.UnixNano(), LastObservedUnixNanos: now.UnixNano(), LastSupport: l5tracks.SupportObserved, VX: 5, BoxX: 10, BoxLength: 4, BoxWidth: 2,
				P: [16]float32{.01, 0, 0, 0, 0, .01, 0, 0, 0, 0, .01, 0, 0, 0, 0, .01}}
			noiseCfg := l5tracks.DefaultTrackerConfig()
			at := now.Add(100 * time.Millisecond)
			switch name {
			case "tentative":
				tr.TrackState = l5tracks.TrackTentative
			case "coasting":
				tr.Misses = 1
			case "stopped":
				tr.VX = 0
			case "stale":
				at = now.Add(time.Second)
			case "backwards":
				at = now.Add(-time.Second)
			case "pedestrian":
				tr.ObjectClass = "pedestrian"
			case "uncertain":
				tr.P[0] = 10
			case "nonfinite":
				tr.VX = float32(math.NaN())
			case "process-noise-bound":
				tr.P[0], tr.P[5] = .56, .56
			case "coupled-noise-bound":
				tr.P[0], tr.P[5] = .562, .562
				noiseCfg.ProcessNoisePos, noiseCfg.ProcessNoiseVel, noiseCfg.CoupledProcessNoise = 0, 5, true
			case "invalid-noise":
				noiseCfg.ProcessNoisePos = float32(math.NaN())
			case "weak-direction":
				tr.P[0], tr.P[5], tr.P[1], tr.P[4] = .4, .4, .3, .3
			}
			got := motionFragmentHints([]*l5tracks.TrackedObject{tr}, at, cfg, noiseCfg)
			if name == "accepted" {
				if len(got) != 1 || math.Abs(got[0].Rect.X-10.5) > 1e-6 {
					t.Fatalf("hint not at common capture time: %+v", got)
				}
			} else if len(got) != 0 {
				t.Fatal("unsafe motion hint accepted")
			}
		})
	}
}

// Exercise the real callback and observation tap: joining must change both
// detections and immutable membership, before association, by the same amount.
func TestMotionJoinWiringKeepsObservationMembershipCoherent(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[enabled], func(t *testing.T) {
			now := time.Unix(1_700_000_000, 0)
			sink := &memoryFrameSink{}
			cfg := tapConfig(t, sink)
			cfg.RemoveGround = false
			seed := *cfg
			seed.Tracker = nil
			seed.NewFrameCallback()(ended(makeStableFrame("seed", now, 20)))
			track := &l5tracks.TrackedObject{TrackID: "vehicle", CreationSequence: 1, TrackMeasurement: l5tracks.TrackMeasurement{TrackState: l5tracks.TrackConfirmed, ObjectClass: "car", ObservationCount: 20},
				StateUnixNanos: now.UnixNano(), LastObservedUnixNanos: now.UnixNano(), LastSupport: l5tracks.SupportObserved, VY: 5, BoxY: -5.9, BoxHeadingRad: math.Pi / 2, BoxLength: 4, BoxWidth: 2,
				P: [16]float32{.01, 0, 0, 0, 0, .01, 0, 0, 0, 0, .01, 0, 0, 0, 0, .01}}
			cfg.Tracker = &mockTrackerCov{confirmedTracks: []*l5tracks.TrackedObject{track}}
			cfg.FragmentJoin = &config.L4FragmentJoin{Enabled: enabled, MaxGapMetres: 2, MarginMetres: .15, MaxAgeSecs: .2, MinSpeedMps: 2, MaxPositionStddevMetres: .75}
			stats := &FragmentJoinStats{}
			cfg.OnFragmentJoin = stats.Observe
			cfg.FragmentJoinTrackerConfig = func() l5tracks.TrackerConfig { return l5tracks.DefaultTrackerConfig() }
			capture := &clusterCapture{}
			cfg.VisualiserAdapter = capture
			cfg.VisualiserPublisher = discardPublisher{}
			ts := now.Add(100 * time.Millisecond)
			var points []l2frames.Point
			for _, distance := range []float64{5, 6.7} {
				for i := 0; i < 12; i++ {
					points = append(points, l2frames.Point{Channel: fgChannel, Azimuth: fgAzimuth, Distance: distance + float64(i)*.02, Intensity: 80, Timestamp: ts})
				}
			}
			cfg.NewFrameCallback()(ended(&l2frames.LiDARFrame{FrameID: "fragments", StartTimestamp: ts, Points: points, PolarPoints: pointsToPolar(points)}))
			want := 2
			if enabled {
				want = 1
			}
			if len(capture.frames) != 1 || len(capture.frames[0]) != want {
				t.Fatalf("published partition: %+v", capture.frames)
			}
			last := sink.records[len(sink.records)-1]
			if len(last.Clusters) != want {
				t.Fatalf("evidence has %d clusters, want %d", len(last.Clusters), want)
			}
			members := 0
			for _, c := range last.Clusters {
				members += len(c.Members)
			}
			if enabled && (stats.Frames != 1 || stats.FramesWithJoins != 1 || stats.ComponentsJoined != 1) {
				t.Fatalf("joining counters: %+v", stats)
			}
			if !enabled && stats.Frames != 0 {
				t.Fatal("disabled joining emitted a counter")
			}
			if members != 24 || len(sink.failures) != 0 {
				t.Fatalf("membership=%d failures=%v", members, sink.failures)
			}
		})
	}
}

func TestMotionHintOrderDoesNotMutateTrackerSnapshot(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cfg := config.L4FragmentJoin{MaxAgeSecs: .2, MinSpeedMps: 2, MaxPositionStddevMetres: .75}
	first := &l5tracks.TrackedObject{TrackID: "first", CreationSequence: 1, TrackMeasurement: l5tracks.TrackMeasurement{TrackState: l5tracks.TrackConfirmed, ObjectClass: "car", ObservationCount: 20}, StateUnixNanos: now.UnixNano(), LastObservedUnixNanos: now.UnixNano(), LastSupport: l5tracks.SupportObserved, VX: 5, BoxLength: 4, BoxWidth: 2, P: [16]float32{.01, 0, 0, 0, 0, .01, 0, 0, 0, 0, .01, 0, 0, 0, 0, .01}}
	second := *first
	second.TrackID = "second"
	second.CreationSequence = 2
	second.BoxY = 10
	snapshot := []*l5tracks.TrackedObject{&second, first}
	hints := motionFragmentHints(snapshot, now.Add(100*time.Millisecond), cfg, l5tracks.DefaultTrackerConfig())
	if len(hints) != 2 || hints[0].ID != "first" || hints[1].ID != "second" || snapshot[0] != &second {
		t.Fatal("hint order varied with snapshot order or mutated its input")
	}
}
