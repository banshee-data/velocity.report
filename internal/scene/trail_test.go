package scene

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// recordTrackedScene runs the shipped tracker over a short scene with misses
// and one track that ends, recording a VRLOG of its tracks the way the
// pipeline's adapter does and, beside it, the filter record a replay keeps.
// It returns the VRLOG path and the filter frames the tracker produced, so a
// test can run the smoother itself as the oracle.
func recordTrackedScene(t *testing.T, withRecord bool) (string, []l5tracks.FilterFrame) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "run.vrlog")
	rec, err := recorder.NewRecorder(dir, "hesai-pandar40p")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	tk := l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
	var frames []l5tracks.FilterFrame
	observers := l5tracks.FilterStepObservers{observerFunc(func(f l5tracks.FilterFrame) { frames = append(frames, f) })}
	var w *recorder.FilterRecordWriter
	if withRecord {
		w, err = recorder.NewFilterRecordWriter(dir, recorder.FilterRecordHeader{SensorID: "hesai-pandar40p", EstimatorID: "cv_kf_v1"})
		if err != nil {
			t.Fatal(err)
		}
		observers = append(observers, w)
	}
	tk.SetFilterStepObserver(observers)

	epoch := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for k := 0; k < 60; k++ {
		at := epoch.Add(time.Duration(k) * 100 * time.Millisecond)
		var clusters []l5tracks.WorldCluster
		for o := 0; o < 3; o++ {
			if (k+o)%7 == 0 || (o == 2 && k > 30) {
				continue
			}
			// Noisy constant velocity: the jitter is what a smoother removes.
			jitter := 0.15 * float32(math.Sin(float64(k*(o+3))))
			clusters = append(clusters, l5tracks.WorldCluster{
				ClusterID: int64(k*3 + o + 1), SensorID: "hesai-pandar40p",
				CentroidX: float32(4*o) + 0.8*float32(k) + jitter,
				CentroidY: float32(10*o) - 0.3*float32(k) - jitter,
			})
		}
		tk.Update(clusters, at)
		set := &l9endpoints.TrackSet{FrameID: uint64(k), TimestampNanos: at.UnixNano()}
		for _, tr := range tk.GetActiveTracks() {
			set.Tracks = append(set.Tracks, l9endpoints.Track{
				TrackID: tr.TrackID, SensorID: "hesai-pandar40p", State: l9endpoints.TrackStateConfirmed,
				X: tr.X, Y: tr.Y, Z: 0.8, VX: tr.VX, VY: tr.VY,
				SpeedMps: tr.Speed(), HeadingRad: tr.Heading(),
				BBoxLength: 4.4, BBoxWidth: 1.8, BBoxHeight: 1.5, BBoxHeadingRad: 0.3,
				BBoxCenterX: tr.X + 0.5, BBoxCenterY: tr.Y - 0.25, BBoxCenterValid: true,
				ObjectClass: "car", ClassConfidence: 0.9, Covariance4x4: tr.P[:],
			})
		}
		if err := rec.Record(&l9endpoints.FrameBundle{FrameID: uint64(k), TimestampNanos: at.UnixNano(),
			SensorID: "hesai-pandar40p", FrameType: l9endpoints.FrameTypeForeground, Tracks: set}); err != nil {
			t.Fatalf("Record %d: %v", k, err)
		}
	}
	if w != nil {
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	return dir, frames
}

type observerFunc func(l5tracks.FilterFrame)

func (f observerFunc) ObserveFilterFrame(frame l5tracks.FilterFrame) { f(frame) }

func smoothedOracle(t *testing.T, frames []l5tracks.FilterFrame) map[trailKey]l5tracks.FilterMoments {
	t.Helper()
	s, err := l5tracks.NewFixedLagSmoother(l5tracks.SmootherConfig{Lag: l5tracks.LagTrackEnd()})
	if err != nil {
		t.Fatal(err)
	}
	out := map[trailKey]l5tracks.FilterMoments{}
	take := func(states []l5tracks.SmoothedState) {
		for _, st := range states {
			out[trailKey{st.TrackID, st.FrameUnixNanos}] = st.Smoothed
		}
	}
	for _, f := range frames {
		take(s.Observe(f))
	}
	take(s.Flush())
	return out
}

func TestExportSmoothedTrailWritesTheSmoothersStates(t *testing.T) {
	src, frames := recordTrackedScene(t, true)
	oracle := smoothedOracle(t, frames)

	online := filepath.Join(t.TempDir(), "online")
	if _, err := Export(Options{VRLOGPath: src, OutDir: online, KeepTrackIDs: true}); err != nil {
		t.Fatalf("online export: %v", err)
	}
	smoothed := filepath.Join(t.TempDir(), "smoothed")
	res, err := Export(Options{VRLOGPath: src, OutDir: smoothed, KeepTrackIDs: true, SmoothedTrail: true})
	if err != nil {
		t.Fatalf("smoothed export: %v", err)
	}

	hdr := readHeader(t, smoothed)
	if hdr.TrailEstimator != "cv_kf_v1+rts_fixed_assignment_v1/final" {
		t.Errorf("trail_estimator %q", hdr.TrailEstimator)
	}
	if hdr.TrailSmoothing == nil {
		t.Fatal("trail_smoothing missing from a smoothed export")
	}
	sum := hdr.TrailSmoothing
	if sum.OnlineFallback != 0 || sum.Substituted == 0 {
		t.Errorf("substituted %d, online fallback %d: every track frame of this recording has a state", sum.Substituted, sum.OnlineFallback)
	}
	if sum.Final == 0 || sum.Final+sum.FixedLag != int(sum.Smoother.Released) || sum.Smoother.RevisionsWithoutEvidence != 0 {
		t.Errorf("summary %+v", *sum)
	}
	if res.Header.TrailSmoothing == nil || res.Header.TrailSmoothing.Substituted != sum.Substituted {
		t.Errorf("Result header does not carry the summary written: %+v", res.Header.TrailSmoothing)
	}
	if h := readHeader(t, online); h.TrailEstimator != TrailEstimatorOnline || h.TrailSmoothing != nil {
		t.Errorf("online header names %q with smoothing %v", h.TrailEstimator, h.TrailSmoothing)
	}

	onlineFrames, smoothedFrames := readFrames(t, online), readFrames(t, smoothed)
	if len(onlineFrames) != len(smoothedFrames) || len(onlineFrames) == 0 {
		t.Fatalf("%d online frames, %d smoothed", len(onlineFrames), len(smoothedFrames))
	}
	startNs, err := parseStartNs(hdr.StartNs)
	if err != nil {
		t.Fatal(err)
	}
	moved, tracks := 0, 0
	for i := range onlineFrames {
		a, b := onlineFrames[i], smoothedFrames[i]
		if a.Frame != b.Frame || a.TimeUs != b.TimeUs || len(a.Tracks) != len(b.Tracks) {
			t.Fatalf("frame %d: the smoothed export changed the frame itself: %+v vs %+v", i, a, b)
		}
		frameNs := startNs + a.TimeUs*1000
		for j := range a.Tracks {
			o, s := a.Tracks[j], b.Tracks[j]
			tracks++
			// Everything but the four state components is the tracker's.
			if o.ID != s.ID || o.Z != s.Z || o.Speed != s.Speed || o.MaxSpeed != s.MaxSpeed || o.Heading != s.Heading ||
				o.Length != s.Length || o.Width != s.Width || o.Height != s.Height || o.BoxYaw != s.BoxYaw ||
				*o.BoxX != *s.BoxX || *o.BoxY != *s.BoxY || o.Class != s.Class || o.Conf != s.Conf {
				t.Fatalf("frame %d track %s: a field other than x, y, vx, vy changed:\n online %+v\nsmooth %+v", i, o.ID, o, s)
			}
			want, ok := oracle[trailKey{o.ID, frameNs}]
			if !ok {
				t.Fatalf("frame %d track %s at %d: the oracle has no state", i, o.ID, frameNs)
			}
			if s.X != round2(want.X) || s.Y != round2(want.Y) || s.VX != round2(want.VX) || s.VY != round2(want.VY) {
				t.Fatalf("frame %d track %s: smoothed (%v, %v, %v, %v), the smoother says (%v, %v, %v, %v)",
					i, o.ID, s.X, s.Y, s.VX, s.VY, round2(want.X), round2(want.Y), round2(want.VX), round2(want.VY))
			}
			if o.X != s.X || o.Y != s.Y || o.VX != s.VX || o.VY != s.VY {
				moved++
			}
		}
	}
	if moved == 0 {
		t.Fatal("no track frame moved: the smoothed export is the online one")
	}
	if sum.Substituted != tracks {
		t.Errorf("substituted %d states for %d track frames exported at stride 1", sum.Substituted, tracks)
	}
	t.Logf("%d of %d track frames moved; %d final, %d fixed_lag", moved, tracks, sum.Final, sum.FixedLag)
}

// TestExportSmoothedTrailLeavesATracksLastStateAlone pins the RTS boundary:
// the last state of a track that ended has no later evidence, so the
// smoother returns its posterior, and the smoothed export must agree with
// the online one there, exactly.
func TestExportSmoothedTrailLeavesATracksLastStateAlone(t *testing.T) {
	src, frames := recordTrackedScene(t, true)
	online := filepath.Join(t.TempDir(), "online")
	if _, err := Export(Options{VRLOGPath: src, OutDir: online, KeepTrackIDs: true}); err != nil {
		t.Fatal(err)
	}
	smoothed := filepath.Join(t.TempDir(), "smoothed")
	if _, err := Export(Options{VRLOGPath: src, OutDir: smoothed, KeepTrackIDs: true, SmoothedTrail: true}); err != nil {
		t.Fatal(err)
	}
	// The chains that ended before the recording did (the third track).
	ended := map[string]int64{}
	for _, f := range frames {
		for _, e := range f.Ended {
			if e.Reason == l5tracks.ChainEndDeleted {
				ended[e.TrackID] = e.FrameUnixNanos
			}
		}
	}
	if len(ended) == 0 {
		t.Fatal("no track ended in the scene; the boundary is not exercised")
	}
	last := map[string]TrackJSON{}
	for _, f := range readFrames(t, online) {
		for _, tr := range f.Tracks {
			if _, ok := ended[tr.ID]; ok {
				last[tr.ID] = tr
			}
		}
	}
	lastSmoothed := map[string]TrackJSON{}
	for _, f := range readFrames(t, smoothed) {
		for _, tr := range f.Tracks {
			if _, ok := ended[tr.ID]; ok {
				lastSmoothed[tr.ID] = tr
			}
		}
	}
	for id, o := range last {
		s := lastSmoothed[id]
		if o.X != s.X || o.Y != s.Y || o.VX != s.VX || o.VY != s.VY {
			t.Errorf("track %s: its last state moved from (%v, %v, %v, %v) to (%v, %v, %v, %v) with no later evidence",
				id, o.X, o.Y, o.VX, o.VY, s.X, s.Y, s.VX, s.VY)
		}
	}
}

func TestExportSmoothedTrailNeedsTheFilterRecord(t *testing.T) {
	src, _ := recordTrackedScene(t, false)
	_, err := Export(Options{VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "out"), SmoothedTrail: true})
	if err == nil || !strings.Contains(err.Error(), recorder.FilterRecordFile) {
		t.Fatalf("err = %v, want a refusal naming the missing %s", err, recorder.FilterRecordFile)
	}
	// Without the option the same recording exports as it always did.
	res, err := Export(Options{VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "online")})
	if err != nil {
		t.Fatal(err)
	}
	if res.Header.TrailEstimator != TrailEstimatorOnline || res.Header.TrailSmoothing != nil {
		t.Fatalf("header %q %v", res.Header.TrailEstimator, res.Header.TrailSmoothing)
	}
}

func TestExportSmoothedTrailRefusesAnotherRecordingsRecord(t *testing.T) {
	// A filter record of a different scene beside this recording's chunks:
	// no track frame matches, and the export must say so rather than write
	// the online trail under a smoothed name.
	src, _ := recordTrackedScene(t, false)
	w, err := recorder.NewFilterRecordWriter(src, recorder.FilterRecordHeader{EstimatorID: "cv_kf_v1"})
	if err != nil {
		t.Fatal(err)
	}
	tk := l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
	tk.SetFilterStepObserver(w)
	other := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for k := 0; k < 20; k++ {
		tk.Update([]l5tracks.WorldCluster{{ClusterID: int64(k + 1), SensorID: "x", CentroidX: float32(k), CentroidY: 1}},
			other.Add(time.Duration(k)*100*time.Millisecond))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = Export(Options{VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "out"), SmoothedTrail: true})
	if err == nil || !strings.Contains(err.Error(), "matches none") {
		t.Fatalf("err = %v, want a refusal of a record that matches no track frame", err)
	}
}

func parseStartNs(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

func TestExportSmoothedTrailRefusesARecordItCannotRead(t *testing.T) {
	src, _ := recordTrackedScene(t, false)
	if err := os.WriteFile(filepath.Join(src, recorder.FilterRecordFile), []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(Options{VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "out"), SmoothedTrail: true}); err == nil ||
		!strings.Contains(err.Error(), "filter record") {
		t.Fatalf("err = %v, want the filter record's read error", err)
	}
}

func TestExportSmoothedTrailRefusesARecordWithNoStates(t *testing.T) {
	// A replay that stopped before its first frame leaves a header and
	// nothing else: there is no smoothed state to draw, and the export must
	// not fall back to the online trail under a smoothed name.
	src, _ := recordTrackedScene(t, false)
	w, err := recorder.NewFilterRecordWriter(src, recorder.FilterRecordHeader{EstimatorID: "cv_kf_v1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(Options{VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "out"), SmoothedTrail: true}); err == nil ||
		!strings.Contains(err.Error(), "released no states") {
		t.Fatalf("err = %v, want a refusal of a record with no states", err)
	}
}
