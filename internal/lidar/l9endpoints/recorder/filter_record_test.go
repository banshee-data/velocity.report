package recorder

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// writeSidecar writes lines as a gzipped sidecar in dir, so a reader test can
// hand ReadFilterRecord what a writer never produces.
func writeSidecar(t *testing.T, dir string, lines ...string) {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	for _, l := range lines {
		if _, err := gz.Write([]byte(l + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FilterRecordFile), b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFilterRecordReaderRefusesWhatAWriterNeverWrites(t *testing.T) {
	header := fmt.Sprintf(`{"version":%d,"estimator_id":"cv_kf_v1"}`, filterRecordVersion)
	for _, tc := range []struct {
		name  string
		lines []string
		want  string
	}{
		{"a header that is not JSON", []string{"not json"}, "read header"},
		{"another version", []string{fmt.Sprintf(`{"version":%d,"estimator_id":"cv_kf_v1"}`, filterRecordVersion+1)}, "this reader knows"},
		{"a header without an estimator", []string{fmt.Sprintf(`{"version":%d}`, filterRecordVersion)}, "names no estimator"},
		{"a frame that is not JSON", []string{header, "{"}, "line 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSidecar(t, dir, tc.lines...)
			_, err := ReadFilterRecord(dir, func(l5tracks.FilterFrame) error { return nil })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one saying %q", err, tc.want)
			}
		})
	}
}

func TestFilterRecordReaderStopsAtTheVisitorsError(t *testing.T) {
	dir := t.TempDir()
	w, err := NewFilterRecordWriter(dir, FilterRecordHeader{EstimatorID: "cv_kf_v1"})
	if err != nil {
		t.Fatal(err)
	}
	w.ObserveFilterFrame(l5tracks.FilterFrame{FrameUnixNanos: 1})
	w.ObserveFilterFrame(l5tracks.FilterFrame{FrameUnixNanos: 2})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	stop := errors.New("stop")
	var n int
	_, err = ReadFilterRecord(dir, func(l5tracks.FilterFrame) error { n++; return stop })
	if !errors.Is(err, stop) || n != 1 {
		t.Fatalf("err = %v after %d frames, want the visitor's error after the first", err, n)
	}
}

func TestFilterRecordWriterReportsWhereItCannotWrite(t *testing.T) {
	header := FilterRecordHeader{EstimatorID: "cv_kf_v1"}
	// The VRLOG directory's parent is a file, so the directory cannot exist.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFilterRecordWriter(filepath.Join(file, "run.vrlog"), header); err == nil {
		t.Fatal("a VRLOG directory under a file was accepted")
	}
	// The sidecar's name is taken by a directory.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, FilterRecordFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if HasFilterRecord(dir) {
		t.Fatal("a directory in the sidecar's place reports a filter record")
	}
	if _, err := NewFilterRecordWriter(dir, header); err == nil {
		t.Fatal("a sidecar path held by a directory was accepted")
	}
}

func TestFilterRecordWriterKeepsTheFirstErrorOfTheRecording(t *testing.T) {
	dir := t.TempDir()
	w, err := NewFilterRecordWriter(dir, FilterRecordHeader{EstimatorID: "cv_kf_v1"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := w.Path(), filepath.Join(dir, FilterRecordFile); got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
	// JSON has no NaN, so a frame carrying one cannot be written; the frame
	// is not counted, later frames are dropped, and Close says why.
	nan := float32(math.NaN())
	w.ObserveFilterFrame(l5tracks.FilterFrame{FrameUnixNanos: 7, Steps: []l5tracks.FilterStep{
		{TrackID: "t", FrameUnixNanos: 7, Posterior: l5tracks.FilterMoments{X: nan}},
	}})
	w.ObserveFilterFrame(l5tracks.FilterFrame{FrameUnixNanos: 8})
	if frames, _ := w.Counts(); frames != 0 {
		t.Fatalf("counted %d frames after a failed write, want 0", frames)
	}
	if err := w.Close(); err == nil || !strings.Contains(err.Error(), "write frame at 7") {
		t.Fatalf("Close: %v, want the failed frame's error", err)
	}
}

func TestFilterRecordWriterReportsAFailedFlush(t *testing.T) {
	dir := t.TempDir()
	w, err := NewFilterRecordWriter(dir, FilterRecordHeader{EstimatorID: "cv_kf_v1"})
	if err != nil {
		t.Fatal(err)
	}
	w.ObserveFilterFrame(l5tracks.FilterFrame{FrameUnixNanos: 1})
	// The file is gone from under the buffered writer, as when a volume is
	// unmounted mid-recording: the flush at Close is where it shows.
	if err := w.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err == nil {
		t.Fatal("Close succeeded with its file already closed")
	}
}
