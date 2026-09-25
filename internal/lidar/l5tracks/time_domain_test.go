package l5tracks

import (
	"math"
	"testing"
	"time"
)

// Capture-time behaviour of the estimator. Every timestamp below is synthetic
// capture time; none of these tests read or depend on the wall clock, which is
// the point: the tracker's only notion of elapsed time is what it is given.

var tdEpoch = time.Unix(1_750_000_000, 0)

func tdAt(secs float64) time.Time {
	return tdEpoch.Add(time.Duration(secs * float64(time.Second)))
}

// tdCluster is a point-like cluster at (x, y). tsSecs <= 0 leaves the
// cluster without its own acquisition time, as a hand-built cluster would be.
func tdCluster(x, y float32, tsSecs float64) WorldCluster {
	c := WorldCluster{CentroidX: x, CentroidY: y, SensorID: "time-domain"}
	if tsSecs > 0 {
		c.TSUnixNanos = tdAt(tsSecs).UnixNano()
	}
	return c
}

// soleActive returns the tracker's only non-deleted track.
func soleActive(t *testing.T, tk *Tracker) *TrackedObject {
	t.Helper()
	var found *TrackedObject
	for _, track := range tk.Tracks {
		if track.TrackState == TrackDeleted {
			continue
		}
		if found != nil {
			t.Fatalf("more than one active track")
		}
		found = track
	}
	if found == nil {
		t.Fatalf("no active track")
	}
	return found
}

// movingTrack seeds a tracker with one tentative track at the origin of Y=10
// moving at vx along X, observed at capture time startSecs.
func movingTrack(t *testing.T, cfg TrackerConfig, startSecs float64, vx float32) (*Tracker, *TrackedObject) {
	t.Helper()
	tk := NewTracker(cfg)
	tk.Update([]WorldCluster{tdCluster(0, 10, 0)}, tdAt(startSecs))
	track := soleActive(t, tk)
	track.VX = vx
	return tk, track
}

// A frame stamped with the previous frame's capture time has zero elapsed
// time. It must not move a track, and it is counted.
func TestDuplicateCaptureTimestampPredictsNothing(t *testing.T) {
	tk, track := movingTrack(t, DefaultTrackerConfig(), 1, 10)
	tk.Update(nil, tdAt(1))
	if track.X != 0 {
		t.Fatalf("a zero-interval frame moved the track to X=%v", track.X)
	}
	if got := tk.TimeDomainStats().DuplicateTimestamps; got != 1 {
		t.Fatalf("DuplicateTimestamps = %d, want 1", got)
	}
}

// A capture clock that steps backwards used to become a negative dt: the
// state ran backwards and process noise was subtracted from the covariance,
// which can leave it indefinite. The interval is now zero, the step is
// counted, and the next frame measures from the new timestamp rather than
// from the old high-water mark.
func TestBackwardCaptureTimestampNeverPredictsBackwards(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.OcclusionCovInflation = 0 // isolate prediction from miss handling
	tk, track := movingTrack(t, cfg, 2, 10)
	before := track.P

	tk.Update(nil, tdAt(1.9))

	if track.X != 0 {
		t.Fatalf("a backward step moved the track to X=%v; the interval must be zero, not negative", track.X)
	}
	for i := 0; i < 4; i++ {
		if track.P[i*4+i] < before[i*4+i] {
			t.Fatalf("P[%d,%d] shrank from %v to %v: process noise was subtracted", i, i, before[i*4+i], track.P[i*4+i])
		}
	}
	stats := tk.TimeDomainStats()
	if stats.BackwardTimestamps != 1 || math.Abs(stats.MaxBackwardStepSecs-0.1) > 1e-9 {
		t.Fatalf("backward step not recorded: %+v", stats)
	}
	if tk.LastUpdateNanos != tdAt(1.9).UnixNano() {
		t.Fatalf("frame clock not re-anchored at the new timestamp")
	}

	tk.Update(nil, tdAt(2.0))
	if math.Abs(float64(track.X)-1.0) > 1e-5 {
		t.Fatalf("after re-anchoring, a 0.1 s frame predicted to X=%v, want 1.0", track.X)
	}
}

// Transport gaps longer than MaxPredictDt are clamped by default, which
// predicts a moving object only part of the way. The clamp stays, but the
// unclamped gap is now recorded so the under-prediction is visible.
func TestLongCaptureGapIsRecordedAndClampedByDefault(t *testing.T) {
	cfg := DefaultTrackerConfig()
	tk, track := movingTrack(t, cfg, 1, 10)

	tk.Update(nil, tdAt(4)) // a 3 s transport gap

	stats := tk.TimeDomainStats()
	if math.Abs(stats.LastGapSecs-3) > 1e-9 || math.Abs(stats.MaxGapSecs-3) > 1e-9 || stats.ClampedGaps != 1 {
		t.Fatalf("gap not recorded: %+v", stats)
	}
	want := 10 * cfg.MaxPredictDt
	if math.Abs(float64(track.X-want)) > 1e-5 {
		t.Fatalf("default prediction reached X=%v, want the clamped %v", track.X, want)
	}
}

// CaptureGapPrediction covers the whole gap, in MaxPredictDt steps, so the
// result is the prediction the same interval of ordinary frames would have
// produced.
func TestCaptureGapPredictionCoversTheWholeGap(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.CaptureGapPrediction = true
	cfg.OcclusionCovInflation = 0
	tk, track := movingTrack(t, cfg, 1, 10)

	reference := *track
	manual := NewTracker(cfg)
	for i := 0; i < 6; i++ {
		manual.predict(&reference, cfg.MaxPredictDt)
	}

	tk.Update(nil, tdAt(4))

	if math.Abs(float64(track.X)-30) > 1e-4 {
		t.Fatalf("gap prediction reached X=%v, want 30 (10 m/s for 3 s)", track.X)
	}
	if track.X != reference.X || track.P != reference.P {
		t.Fatalf("gap prediction differs from six 0.5 s steps:\n got X=%v P=%v\nwant X=%v P=%v",
			track.X, track.P, reference.X, reference.P)
	}
	if got := tk.TimeDomainStats().ClampedGaps; got != 1 {
		t.Fatalf("ClampedGaps = %d, want 1: the gap exceeded MaxPredictDt even though it was not clamped", got)
	}
}

// A clock that jumps by years must not stall the frame in a sub-step loop.
// The gap is clamped once, at the frame, to the sub-step limit: every track is
// predicted across exactly that limit (never part of it), each state is
// re-anchored at the frame time as a MaxPredictDt clamp would be, and the
// truncation is counted once for the frame rather than once per track.
func TestCaptureGapPredictionIsBounded(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.CaptureGapPrediction = true
	cfg.MaxMisses = 100
	tk := NewTracker(cfg)
	tk.Update([]WorldCluster{tdCluster(0, 10, 0), tdCluster(0, -10, 0)}, tdAt(1))
	var tracks []*TrackedObject
	for _, track := range tk.Tracks {
		track.VX = 1
		tracks = append(tracks, track)
	}
	if len(tracks) != 2 {
		t.Fatalf("got %d tracks, want 2", len(tracks))
	}

	later := tdAt(1 + 10*365*24*3600) // ten years later
	tk.Update(nil, later)

	if got := tk.TimeDomainStats().TruncatedGapPredictions; got != 1 {
		t.Fatalf("TruncatedGapPredictions = %d, want 1 for the frame, not one per track", got)
	}
	limit := float64(maxGapPredictionSteps) * float64(cfg.MaxPredictDt)
	for _, track := range tracks {
		if math.Abs(float64(track.X)-limit) > 1e-2 {
			t.Fatalf("track predicted to X=%v, want the whole %v s limit at 1 m/s", track.X, limit)
		}
		if track.StateUnixNanos != later.UnixNano() {
			t.Fatalf("StateUnixNanos = %d, want the frame time %d", track.StateUnixNanos, later.UnixNano())
		}
	}
}

// Coast age is elapsed capture time, not a frame count: the same interval
// reached in four frames or in one gives the same age, and closing it records
// the whole unobserved interval.
func TestCoastAgeFollowsCaptureTimeNotFrameCount(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.MaxMisses = 100

	frequent, frequentTrack := movingTrack(t, cfg, 1, 0)
	for _, s := range []float64{1.1, 1.2, 1.3, 1.4} {
		frequent.Update(nil, tdAt(s))
	}
	sparse, sparseTrack := movingTrack(t, cfg, 1, 0)
	sparse.Update(nil, tdAt(1.4))

	for name, track := range map[string]*TrackedObject{"frequent": frequentTrack, "sparse": sparseTrack} {
		if math.Abs(float64(track.CoastAgeSecs)-0.4) > 1e-5 {
			t.Errorf("%s: CoastAgeSecs = %v, want 0.4", name, track.CoastAgeSecs)
		}
	}
	if frequentTrack.Misses != 4 || sparseTrack.Misses != 1 {
		t.Fatalf("misses = %d and %d, want 4 and 1: the two streams differ only in frame count",
			frequentTrack.Misses, sparseTrack.Misses)
	}

	frequent.Update([]WorldCluster{tdCluster(0, 10, 0)}, tdAt(1.5))
	if frequentTrack.CoastAgeSecs != 0 || math.Abs(float64(frequentTrack.MaxCoastAgeSecs)-0.5) > 1e-5 {
		t.Fatalf("re-observation: CoastAgeSecs=%v MaxCoastAgeSecs=%v, want 0 and 0.5",
			frequentTrack.CoastAgeSecs, frequentTrack.MaxCoastAgeSecs)
	}
	if frequentTrack.LastObservedUnixNanos != tdAt(1.5).UnixNano() {
		t.Fatalf("LastObservedUnixNanos not moved to the observing frame")
	}
}

// With the bounds at zero, the default, expiry is the frame-count rule alone:
// a tentative track survives ten seconds of capture time in two frames.
func TestCaptureTimeExpiryIsOffByDefault(t *testing.T) {
	cfg := DefaultTrackerConfig()
	if cfg.MaxCoastSecsTentative != 0 || cfg.MaxCoastSecsConfirmed != 0 ||
		cfg.CaptureGapPrediction || cfg.MeasurementTimePrediction {
		t.Fatalf("a capture-time option is on by default: %+v", cfg)
	}
	tk, track := movingTrack(t, cfg, 1, 0)
	tk.Update(nil, tdAt(6))
	tk.Update(nil, tdAt(11))
	if track.TrackState == TrackDeleted {
		t.Fatal("the frame-count rule alone should keep a track with two misses alive")
	}
	if got := tk.TimeDomainStats().ExpiredByCoastAge; got != 0 {
		t.Fatalf("ExpiredByCoastAge = %d with the bounds disabled", got)
	}
}

// With a bound set, a track that has gone unobserved for longer than it is
// expired before association: a cluster arriving after the bound has lapsed
// seeds a new track instead of reviving the old hypothesis.
func TestCaptureTimeExpiryPrecedesAssociation(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.MaxCoastSecsTentative = 0.3
	tk, track := movingTrack(t, cfg, 1, 0)

	tk.Update(nil, tdAt(1.2))
	if track.TrackState == TrackDeleted {
		t.Fatal("expired at 0.2 s against a 0.3 s bound")
	}
	tk.Update([]WorldCluster{tdCluster(0, 10, 0)}, tdAt(1.5))

	if track.TrackState != TrackDeleted || track.EndUnixNanos != tdAt(1.5).UnixNano() {
		t.Fatalf("track outlived its capture-time bound: state=%s", track.TrackState)
	}
	if tk.TracksCreated != 2 {
		t.Fatalf("TracksCreated = %d, want 2: the late cluster must seed a new track", tk.TracksCreated)
	}
	stats := tk.TimeDomainStats()
	if stats.ExpiredByCoastAge != 1 || stats.ExpiredByMisses != 0 {
		t.Fatalf("deletion attributed to the wrong rule: %+v", stats)
	}
}

// The tentative and confirmed bounds are separate, as the miss budgets are.
func TestCaptureTimeExpiryUsesTheConfirmedBound(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.MaxCoastSecsTentative = 0.2
	cfg.MaxCoastSecsConfirmed = 1.0
	tk := NewTracker(cfg)
	for i := 0; i < cfg.HitsToConfirm; i++ {
		tk.Update([]WorldCluster{tdCluster(0, 10, 0)}, tdAt(1+0.1*float64(i)))
	}
	track := soleActive(t, tk)
	if track.TrackState != TrackConfirmed {
		t.Fatalf("setup: track is %s, want confirmed", track.TrackState)
	}
	last := 1 + 0.1*float64(cfg.HitsToConfirm-1)

	tk.Update(nil, tdAt(last+0.5))
	if track.TrackState == TrackDeleted {
		t.Fatal("a confirmed track was expired by the tentative bound")
	}
	tk.Update(nil, tdAt(last+1.0))
	if track.TrackState != TrackDeleted {
		t.Fatal("a confirmed track outlived the confirmed bound")
	}
}

// AdvanceMisses ages tracks by the skipped frame's capture time and applies
// the bound, but must not move the frame clock: nothing was predicted.
func TestAdvanceMissesAppliesTheCaptureTimeBound(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.MaxMisses = 100
	cfg.MaxCoastSecsTentative = 0.5
	tk, track := movingTrack(t, cfg, 1, 0)
	frameClock := tk.LastUpdateNanos

	tk.AdvanceMisses(tdAt(1.3))
	if track.TrackState == TrackDeleted || math.Abs(float64(track.CoastAgeSecs)-0.3) > 1e-5 {
		t.Fatalf("after 0.3 s: state=%s age=%v", track.TrackState, track.CoastAgeSecs)
	}
	tk.AdvanceMisses(tdAt(1.6))
	if track.TrackState != TrackDeleted {
		t.Fatal("AdvanceMisses did not apply the capture-time bound")
	}
	if tk.LastUpdateNanos != frameClock {
		t.Fatal("AdvanceMisses moved the frame clock without predicting")
	}
	stats := tk.TimeDomainStats()
	if stats.AdvancedFrames != 2 || stats.ExpiredByCoastAge != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

// Q3 scene: an object at a constant 5 m/s whose scan time within the
// rotation alternates between the start and the end of the sweep, as an
// object at the azimuth wrap does. Measured positions are exact for the
// instant each was taken, so any residual is timing.
func q3Stream(frames int) (clusters []WorldCluster, frameTimes []time.Time) {
	const period, lateOffset, speed = 0.1, 0.09, 5.0
	for k := 0; k < frames; k++ {
		frameStart := 1 + period*float64(k)
		offset := 0.0
		if k%2 == 0 {
			offset = lateOffset
		}
		measured := frameStart + offset
		clusters = append(clusters, tdCluster(float32(speed*(measured-1)), 10, measured))
		frameTimes = append(frameTimes, tdAt(frameStart))
	}
	return clusters, frameTimes
}

func meanAbsInnovation(t *testing.T, cfg TrackerConfig, frames, tail int) (float64, *TrackedObject) {
	t.Helper()
	clusters, times := q3Stream(frames)
	tk := NewTracker(cfg)
	var sum float64
	var track *TrackedObject
	for k := range clusters {
		tk.Update([]WorldCluster{clusters[k]}, times[k])
		track = soleActive(t, tk)
		if k >= frames-tail {
			if !track.LastResidual.Valid {
				t.Fatalf("frame %d: no residual recorded", k)
			}
			sum += math.Hypot(float64(track.LastResidual.InnovationX), float64(track.LastResidual.InnovationY))
		}
	}
	return sum / float64(tail), track
}

// By default the filter treats every cluster as observed at the frame start,
// so an alternating scan offset reads as a 0.45 m position oscillation. With
// MeasurementTimePrediction the update is taken at the cluster's own time and
// the oscillation is recognised as timing, not geometry.
func TestMeasurementTimePredictionRemovesIntraFrameTimingResidual(t *testing.T) {
	const frames, tail = 40, 10
	frameMode, frameTrack := meanAbsInnovation(t, DefaultTrackerConfig(), frames, tail)

	cfg := DefaultTrackerConfig()
	cfg.MeasurementTimePrediction = true
	measurementMode, measurementTrack := meanAbsInnovation(t, cfg, frames, tail)

	if frameMode < 0.2 {
		t.Fatalf("frame-time residual %.3f m: the scene no longer exercises intra-frame timing", frameMode)
	}
	if measurementMode > frameMode/10 {
		t.Fatalf("measurement-time residual %.3f m against frame-time %.3f m: the timing error was not removed", measurementMode, frameMode)
	}
	if frameTrack.StateUnixNanos != tdAt(1+0.1*(frames-1)).UnixNano() {
		t.Fatal("frame mode: the state should refer to the frame start")
	}
	if measurementTrack.StateUnixNanos != measurementTrack.LastMeasurementUnixNanos {
		t.Fatal("measurement mode: the state should refer to the measurement's time after an update")
	}
	t.Logf("mean |innovation| over the last %d frames: frame time %.3f m, measurement time %.4f m", tail, frameMode, measurementMode)
}

// The choice is explicit: by default a cluster's own timestamp is evidence
// only and never changes the estimate.
func TestFrameTimeModeIgnoresClusterTimestampsForPrediction(t *testing.T) {
	clusters, times := q3Stream(20)
	run := func(stripTimestamps bool) []float32 {
		tk := NewTracker(DefaultTrackerConfig())
		var trace []float32
		for k, c := range clusters {
			if stripTimestamps {
				c.TSUnixNanos = 0
			}
			tk.Update([]WorldCluster{c}, times[k])
			track := soleActive(t, tk)
			trace = append(trace, track.X, track.VX, track.P[0])
		}
		return trace
	}
	with, without := run(false), run(true)
	for i := range with {
		if with[i] != without[i] {
			t.Fatalf("frame-time mode changed its estimate with cluster timestamps present (index %d: %v vs %v)", i, with[i], without[i])
		}
	}
}

// A measurement stamped before the time a track's state refers to is never
// retrodicted: the interval is zero and counted.
func TestMeasurementBeforeStateTimeIsNotPredictedBackwards(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.MeasurementTimePrediction = true
	tk, track := movingTrack(t, cfg, 1, 10)

	tk.Update([]WorldCluster{tdCluster(1, 10, 1.05)}, tdAt(1.1))
	if got := tk.TimeDomainStats().MeasurementIntervalsClamped; got != 1 {
		t.Fatalf("MeasurementIntervalsClamped = %d, want 1", got)
	}
	if track.StateUnixNanos != tdAt(1.1).UnixNano() {
		t.Fatal("the state was moved back to the earlier measurement time")
	}
}

// Reset clears the capture-time diagnostics with everything else.
func TestResetClearsTimeDomainStats(t *testing.T) {
	tk, _ := movingTrack(t, DefaultTrackerConfig(), 1, 0)
	tk.Update(nil, tdAt(0.5))
	if tk.TimeDomainStats() == (TimeDomainStats{}) {
		t.Fatal("setup recorded nothing")
	}
	tk.Reset()
	if got := tk.TimeDomainStats(); got != (TimeDomainStats{}) {
		t.Fatalf("stats survived Reset: %+v", got)
	}
}
