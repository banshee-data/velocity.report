package recorder

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// recordedFrames is the tracker's own filter record of a short scene with
// misses, so coasting, inflation and chain ends are all exercised: the same
// scene TestFilterStepObserverDoesNotChangeTracking runs.
type recordedFrames struct{ frames []l5tracks.FilterFrame }

func (r *recordedFrames) ObserveFilterFrame(frame l5tracks.FilterFrame) {
	r.frames = append(r.frames, frame)
}

func runRecordedScene(t *testing.T, observer l5tracks.FilterStepObserver) {
	t.Helper()
	tk := l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
	tk.SetFilterStepObserver(observer)
	epoch := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for k := 0; k < 60; k++ {
		var clusters []l5tracks.WorldCluster
		for o := 0; o < 3; o++ {
			if (k+o)%7 == 0 {
				continue
			}
			if o == 2 && k > 30 {
				continue // the third track ends, so a chain end is recorded
			}
			clusters = append(clusters, l5tracks.WorldCluster{
				ClusterID: int64(k*3 + o + 1), SensorID: "test",
				CentroidX: float32(4*o) + 0.8*float32(k)*0.1*float32(o+1),
				CentroidY: float32(10*o) + 0.02*float32(k%4),
			})
		}
		tk.Update(clusters, epoch.Add(time.Duration(k)*100*time.Millisecond))
	}
}

func TestFilterRecordRoundTripsTheTrackersRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run.vrlog")
	w, err := NewFilterRecordWriter(dir, FilterRecordHeader{SensorID: "test", EstimatorID: "cv_kf_v1"})
	if err != nil {
		t.Fatalf("NewFilterRecordWriter: %v", err)
	}
	want := &recordedFrames{}
	runRecordedScene(t, l5tracks.FilterStepObservers{w, want})
	frames, steps := w.Counts()
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !HasFilterRecord(dir) {
		t.Fatalf("%s has no %s after the recording", dir, FilterRecordFile)
	}
	if frames != int64(len(want.frames)) || frames != 60 {
		t.Fatalf("wrote %d frames, want %d", frames, len(want.frames))
	}
	var wantSteps int64
	var ends int
	for _, f := range want.frames {
		wantSteps += int64(len(f.Steps))
		ends += len(f.Ended)
	}
	if steps != wantSteps || wantSteps == 0 || ends == 0 {
		t.Fatalf("wrote %d steps, want %d, with %d chain ends", steps, wantSteps, ends)
	}

	var got []l5tracks.FilterFrame
	header, err := ReadFilterRecord(dir, func(frame l5tracks.FilterFrame) error {
		got = append(got, frame)
		return nil
	})
	if err != nil {
		t.Fatalf("ReadFilterRecord: %v", err)
	}
	if header.Version != filterRecordVersion || header.EstimatorID != "cv_kf_v1" || header.SensorID != "test" {
		t.Fatalf("header %+v", header)
	}
	if len(got) != len(want.frames) {
		t.Fatalf("read %d frames, want %d", len(got), len(want.frames))
	}
	for i := range want.frames {
		w := want.frames[i]
		for s := range w.Steps {
			// The solid-body reading is the one field the sidecar does not
			// carry; everything else must come back bit for bit.
			w.Steps[s].SolidBody = nil
		}
		if !reflect.DeepEqual(got[i], w) {
			t.Fatalf("frame %d differs after the round trip:\n got %+v\nwant %+v", i, got[i], w)
		}
	}
}

func TestFilterRecordSmoothsTheSameAfterTheRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run.vrlog")
	w, err := NewFilterRecordWriter(dir, FilterRecordHeader{EstimatorID: "cv_kf_v1"})
	if err != nil {
		t.Fatalf("NewFilterRecordWriter: %v", err)
	}
	direct, err := l5tracks.NewFixedLagSmoother(l5tracks.SmootherConfig{Lag: l5tracks.LagTrackEnd()})
	if err != nil {
		t.Fatal(err)
	}
	var want []l5tracks.SmoothedState
	collect := observerFunc(func(frame l5tracks.FilterFrame) { want = append(want, direct.Observe(frame)...) })
	runRecordedScene(t, l5tracks.FilterStepObservers{w, collect})
	want = append(want, direct.Flush()...)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	replayed, err := l5tracks.NewFixedLagSmoother(l5tracks.SmootherConfig{Lag: l5tracks.LagTrackEnd()})
	if err != nil {
		t.Fatal(err)
	}
	var got []l5tracks.SmoothedState
	if _, err := ReadFilterRecord(dir, func(frame l5tracks.FilterFrame) error {
		got = append(got, replayed.Observe(frame)...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got = append(got, replayed.Flush()...)
	if len(got) != len(want) || len(got) == 0 {
		t.Fatalf("%d states from the record, %d from the tracker", len(got), len(want))
	}
	for i := range want {
		if got[i].Smoothed != want[i].Smoothed || got[i].Stage != want[i].Stage || got[i].TrackID != want[i].TrackID {
			t.Fatalf("state %d: record gives %+v at %s, tracker %+v at %s", i, got[i].Smoothed, got[i].Stage, want[i].Smoothed, want[i].Stage)
		}
	}
}

type observerFunc func(l5tracks.FilterFrame)

func (f observerFunc) ObserveFilterFrame(frame l5tracks.FilterFrame) { f(frame) }

func TestFilterRecordRefusesAnUnnamedEstimatorAndAMissingFile(t *testing.T) {
	if _, err := NewFilterRecordWriter(t.TempDir(), FilterRecordHeader{}); err == nil {
		t.Fatal("a header without an estimator was accepted")
	}
	dir := t.TempDir()
	if HasFilterRecord(dir) {
		t.Fatal("an empty directory reports a filter record")
	}
	if _, err := ReadFilterRecord(dir, func(l5tracks.FilterFrame) error { return nil }); err == nil {
		t.Fatal("a missing sidecar read without error")
	}
	if err := os.WriteFile(filepath.Join(dir, FilterRecordFile), []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFilterRecord(dir, func(l5tracks.FilterFrame) error { return nil }); err == nil {
		t.Fatal("a corrupt sidecar read without error")
	}
}

func TestFilterRecordCloseIsIdempotentAndDropsLateFrames(t *testing.T) {
	dir := t.TempDir()
	w, err := NewFilterRecordWriter(dir, FilterRecordHeader{EstimatorID: "cv_kf_v1"})
	if err != nil {
		t.Fatal(err)
	}
	w.ObserveFilterFrame(l5tracks.FilterFrame{FrameUnixNanos: 1})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	w.ObserveFilterFrame(l5tracks.FilterFrame{FrameUnixNanos: 2})
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	var n int
	if _, err := ReadFilterRecord(dir, func(l5tracks.FilterFrame) error { n++; return nil }); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("read %d frames, want the one written before Close", n)
	}
}
