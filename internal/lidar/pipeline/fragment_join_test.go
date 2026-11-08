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
			got := motionFragmentHints([]*l5tracks.TrackedObject{tr}, at, cfg, noiseCfg, false)
			// Track-conditioned membership widens footprints; it must never
			// admit a body the gate refuses.
			if widened := motionFragmentHints([]*l5tracks.TrackedObject{tr}, at, cfg, noiseCfg, true); len(widened) != len(got) {
				t.Fatalf("membership changed which bodies give hints: %d against %d", len(widened), len(got))
			}
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
	hints := motionFragmentHints(snapshot, now.Add(100*time.Millisecond), cfg, l5tracks.DefaultTrackerConfig(), false)
	if len(hints) != 2 || hints[0].ID != "first" || hints[1].ID != "second" || snapshot[0] != &second {
		t.Fatal("hint order varied with snapshot order or mutated its input")
	}
}

// Track-conditioned membership grows the footprint by two standard deviations
// of the projected position along and across the body, and nothing else.
func TestMotionHintTrackMembershipWidensByProjectedCovariance(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	at := now.Add(100 * time.Millisecond)
	cfg := config.L4FragmentJoin{MaxAgeSecs: .2, MinSpeedMps: 2, MaxPositionStddevMetres: .75}
	noiseCfg := l5tracks.DefaultTrackerConfig()
	noiseCfg.ProcessNoisePos, noiseCfg.CoupledProcessNoise = 0, false
	for _, tc := range []struct {
		name                  string
		heading               float32
		xx, xy, yy            float32
		wantLength, wantWidth float64
	}{
		// Along x: 2·2·0.2 on the length, 2·2·0.3 on the width.
		{"along-x", 0, .04, 0, .09, 4.8, 3.2},
		// Turned a quarter: the length reads the y variance.
		{"along-y", math.Pi / 2, .04, 0, .09, 5.2, 2.8},
		// Correlated at 45°: along 0.05+0.04 = 0.09, across 0.05-0.04 = 0.01.
		{"correlated", math.Pi / 4, .05, .04, .05, 4 + 4*.3, 2 + 4*.1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := &l5tracks.TrackedObject{TrackID: "vehicle", CreationSequence: 1, TrackMeasurement: l5tracks.TrackMeasurement{TrackState: l5tracks.TrackConfirmed, ObjectClass: "truck", ObservationCount: 20},
				StateUnixNanos: now.UnixNano(), LastObservedUnixNanos: now.UnixNano(), LastSupport: l5tracks.SupportObserved, VX: 5, BoxX: 10, BoxHeadingRad: tc.heading, BoxLength: 4, BoxWidth: 2,
				P: [16]float32{tc.xx, tc.xy, 0, 0, tc.xy, tc.yy, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}
			off := motionFragmentHints([]*l5tracks.TrackedObject{tr}, at, cfg, noiseCfg, false)
			on := motionFragmentHints([]*l5tracks.TrackedObject{tr}, at, cfg, noiseCfg, true)
			if len(off) != 1 || len(on) != 1 {
				t.Fatalf("hints: off %d, on %d", len(off), len(on))
			}
			if off[0].Rect.Length != 4 || off[0].Rect.Width != 2 {
				t.Fatalf("off footprint is not the predicted box: %+v", off[0].Rect)
			}
			got := on[0].Rect
			// float32 covariance entries carry their rounding into the root.
			if math.Abs(got.Length-tc.wantLength) > 1e-6 || math.Abs(got.Width-tc.wantWidth) > 1e-6 {
				t.Fatalf("widened footprint %.6f x %.6f, want %.6f x %.6f", got.Length, got.Width, tc.wantLength, tc.wantWidth)
			}
			if got.X != off[0].Rect.X || got.Y != off[0].Rect.Y || got.HeadingRad != off[0].Rect.HeadingRad || on[0].ID != off[0].ID {
				t.Fatalf("membership moved the footprint: %+v against %+v", got, off[0].Rect)
			}
		})
	}
}

func TestAxisStddevsClampsRoundingBelowZero(t *testing.T) {
	// A singular covariance: all its variance lies along the 45° axis, and
	// the across projection is zero in exact arithmetic.
	along, across := axisStddevs(.5, .5, .5, math.Pi/4)
	if math.IsNaN(along) || math.IsNaN(across) || across > 1e-7 || math.Abs(along-1) > 1e-9 {
		t.Fatalf("along %v across %v", along, across)
	}
}

// A truck's far fragment lies just beyond the predicted box, by less than the
// body's own position uncertainty: the shipped join leaves it apart, and
// track-conditioned membership joins it, through the real callback and the
// observation tap, with every member kept.
func TestTrackMembershipJoinsFragmentBeyondThePredictedBox(t *testing.T) {
	for _, membership := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[membership], func(t *testing.T) {
			now := time.Unix(1_700_000_000, 0)
			sink := &memoryFrameSink{}
			cfg := tapConfig(t, sink)
			cfg.RemoveGround = false
			seed := *cfg
			seed.Tracker = nil
			seed.NewFrameCallback()(ended(makeStableFrame("seed", now, 20)))
			// Projected to the frame, the 3 m box (the shortest a hint takes)
			// is centred at y = -5.1 and runs 1.65 m either way with the
			// margin: the near fragment (5.00 to 5.22 m) is inside it, the far
			// one (6.70 to 6.92 m) crosses its end by up to 0.17 m. A position
			// sigma of about 0.2 m widens each end by about 0.4 m.
			track := &l5tracks.TrackedObject{TrackID: "vehicle", CreationSequence: 1, TrackMeasurement: l5tracks.TrackMeasurement{TrackState: l5tracks.TrackConfirmed, ObjectClass: "truck", ObservationCount: 20},
				StateUnixNanos: now.UnixNano(), LastObservedUnixNanos: now.UnixNano(), LastSupport: l5tracks.SupportObserved, VY: 5, BoxY: -5.6, BoxHeadingRad: math.Pi / 2, BoxLength: 3, BoxWidth: 2,
				P: [16]float32{.04, 0, 0, 0, 0, .04, 0, 0, 0, 0, .01, 0, 0, 0, 0, .01}}
			cfg.Tracker = &mockTrackerCov{confirmedTracks: []*l5tracks.TrackedObject{track}}
			cfg.FragmentJoin = &config.L4FragmentJoin{Enabled: true, MaxGapMetres: 2, MarginMetres: .15, MaxAgeSecs: .2, MinSpeedMps: 2, MaxPositionStddevMetres: .75}
			cfg.FragmentJoinTrackMembership = membership
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
			want, joined := 2, 0
			if membership {
				want, joined = 1, 1
			}
			if len(capture.frames) != 1 || len(capture.frames[0]) != want {
				t.Fatalf("published partition: %+v", capture.frames)
			}
			last := sink.records[len(sink.records)-1]
			members := 0
			for _, c := range last.Clusters {
				members += len(c.Members)
			}
			if len(last.Clusters) != want || members != 24 || len(sink.failures) != 0 {
				t.Fatalf("evidence clusters=%d members=%d failures=%v", len(last.Clusters), members, sink.failures)
			}
			if stats.Frames != 1 || stats.ComponentsJoined != joined {
				t.Fatalf("joining counters: %+v", stats)
			}
		})
	}
}
