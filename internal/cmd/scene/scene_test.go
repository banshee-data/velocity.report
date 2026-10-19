package scene

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// recordedVRLOG writes a two-frame recording with one track and no filter
// record, as a live recording or one made before the sidecar existed.
func recordedVRLOG(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "run.vrlog")
	rec, err := recorder.NewRecorder(dir, "hesai-pandar40p")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC).UnixNano()
	for i := 0; i < 2; i++ {
		ts := base + int64(i)*100_000_000
		if err := rec.Record(&l9endpoints.FrameBundle{FrameID: uint64(i), TimestampNanos: ts, SensorID: "hesai-pandar40p",
			FrameType: l9endpoints.FrameTypeForeground,
			Tracks: &l9endpoints.TrackSet{FrameID: uint64(i), TimestampNanos: ts, Tracks: []l9endpoints.Track{
				{TrackID: "t", State: l9endpoints.TrackStateConfirmed, X: 1, Y: 2},
			}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestExportSmoothedTrailFlag(t *testing.T) {
	src := recordedVRLOG(t)
	out := filepath.Join(t.TempDir(), "out")
	// A background export has no trail to smooth: a usage error, before
	// anything is read.
	if code := exportMain([]string{"--vrlog", src, "--out", out, "--export", "background", "--smoothed-trail"}); code != 2 {
		t.Fatalf("--smoothed-trail with --export background exited %d, want 2", code)
	}
	// A recording without a filter record cannot be smoothed: the export
	// fails rather than writing the online trail under a smoothed name.
	if code := exportMain([]string{"--vrlog", src, "--out", out, "--smoothed-trail"}); code != 1 {
		t.Fatalf("--smoothed-trail on a recording without a filter record exited %d, want 1", code)
	}
	// Without the flag the same recording exports.
	if code := exportMain([]string{"--vrlog", src, "--out", out}); code != 0 {
		t.Fatalf("plain export exited %d", code)
	}
}

// trackedVRLOG runs the shipped tracker over one vehicle and records its
// tracks with the filter record beside them, as a replay does.
func trackedVRLOG(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "run.vrlog")
	rec, err := recorder.NewRecorder(dir, "hesai-pandar40p")
	if err != nil {
		t.Fatal(err)
	}
	w, err := recorder.NewFilterRecordWriter(dir, recorder.FilterRecordHeader{SensorID: "hesai-pandar40p", EstimatorID: "cv_kf_v1"})
	if err != nil {
		t.Fatal(err)
	}
	tk := l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
	tk.SetFilterStepObserver(w)
	epoch := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for k := 0; k < 30; k++ {
		at := epoch.Add(time.Duration(k) * 100 * time.Millisecond)
		tk.Update([]l5tracks.WorldCluster{{ClusterID: int64(k + 1), SensorID: "hesai-pandar40p",
			CentroidX: 0.8 * float32(k), CentroidY: 2}}, at)
		set := &l9endpoints.TrackSet{FrameID: uint64(k), TimestampNanos: at.UnixNano()}
		for _, tr := range tk.GetActiveTracks() {
			set.Tracks = append(set.Tracks, l9endpoints.Track{TrackID: tr.TrackID, State: l9endpoints.TrackStateConfirmed,
				X: tr.X, Y: tr.Y, VX: tr.VX, VY: tr.VY, BBoxLength: 4.4, BBoxWidth: 1.8, BBoxHeight: 1.5})
		}
		if err := rec.Record(&l9endpoints.FrameBundle{FrameID: uint64(k), TimestampNanos: at.UnixNano(),
			SensorID: "hesai-pandar40p", FrameType: l9endpoints.FrameTypeForeground, Tracks: set}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestExportSmoothedTrailFromAReplaysRecord(t *testing.T) {
	src := trackedVRLOG(t)
	if code := exportMain([]string{"--vrlog", src, "--out", filepath.Join(t.TempDir(), "out"), "--smoothed-trail"}); code != 0 {
		t.Fatalf("--smoothed-trail on a replay's recording exited %d, want 0", code)
	}
}

func TestExportHelpExitsCleanly(t *testing.T) {
	if code := exportMain([]string{"-h"}); code != 0 {
		t.Fatalf("-h exited %d, want 0", code)
	}
}
