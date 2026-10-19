package scene

import (
	"path/filepath"
	"testing"
	"time"

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
