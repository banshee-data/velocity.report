package server

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
)

// minimalFrame is a valid foreground-complete record carrying no points: a
// frame failed at L3, which is enough to exercise the capture's plumbing.
func minimalFrame(seq uint64) l4bobserve.FrameRecord {
	start := int64(1_700_000_000_000_000_000) + int64(seq)*100_000_000
	return l4bobserve.FrameRecord{Sequence: seq, SensorFrameID: "f", FrameUnixNanos: start,
		CaptureStartUnixNanos: start, CaptureEndUnixNanos: start + 99_000_000,
		Disposition: l4bobserve.Disposition{Kind: l4bobserve.DispositionFailed, Stage: l4bobserve.StageL3Foreground, Reason: "test"},
		Stages:      []l4bobserve.StageCount{{Stage: l4bobserve.StageL2Frame, Output: l4bobserve.KnownCount(0)}}}
}

func TestLiveObservationCaptureIsOffByDefault(t *testing.T) {
	f := serveFlags.Lookup("lidar-observation-dir")
	if f == nil || f.DefValue != "" || *lidarObservationDir != "" {
		t.Fatalf("the live observation capture is not off by default: %+v", f)
	}
}

// While the pipeline is live every frame is committed under live identities
// and the declared live policy; the first frame from another source closes
// the capture cleanly, and nothing after it is recorded.
func TestLiveObservationCaptureEndsAtTheFirstNonLiveFrame(t *testing.T) {
	root := t.TempDir()
	var replay atomic.Bool
	var logged []string
	logf := func(format string, args ...any) { logged = append(logged, format) }
	setup, err := startLiveObservationCapture(root, "hesai-live", []byte(`{"version":2}`), liveObservationPolicy(lidarFrameChCapacity),
		func() bool { return !replay.Load() }, logf)
	if err != nil {
		t.Fatal(err)
	}
	c := setup.capture
	if !strings.HasPrefix(setup.sourceID, "source/live/v1/") || setup.calibrationID == "" {
		t.Fatalf("identities = %+v", setup)
	}
	for seq := range uint64(3) {
		if err := c.ObserveFrame(minimalFrame(seq)); err != nil {
			t.Fatal(err)
		}
	}
	replay.Store(true)
	for seq := uint64(3); seq < 5; seq++ {
		if err := c.ObserveFrame(minimalFrame(seq)); err != nil {
			t.Fatalf("a frame after the capture ended = %v", err)
		}
	}
	c.End("server shutting down") // already ended: a no-op
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("containers = %v, %v", entries, err)
	}
	r, err := vrlog.Open(filepath.Join(root, entries[0].Name()), vrlog.Options{})
	if err != nil {
		t.Fatal(err)
	}
	st, m := r.Status(), r.Manifest()
	if !st.Closed || st.Frames != 3 || m.Capture.SourceType != "live" || m.Extraction.SourceID != setup.sourceID ||
		!strings.HasPrefix(m.Extraction.ExtractorID, "l4.dbscan_xy/v1/sha256:") {
		t.Fatalf("status %+v, manifest %+v", st, m.Extraction)
	}
	if p := m.Commit; p.ShedAfter != 50*time.Millisecond || p.UpstreamQueueAge != 3200*time.Millisecond || p.Strict {
		t.Fatalf("declared live policy = %+v", p)
	}
	if _, ok := m.MetadataObject("tuning"); !ok {
		t.Fatal("the effective tuning is not embedded")
	}
	if len(logged) != 2 || !strings.Contains(logged[1], "closed") {
		t.Fatalf("log = %q", logged)
	}
}

func TestLiveObservationCaptureRefusesAnUnusableDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := startLiveObservationCapture(file, "hesai-live", []byte(`{}`), liveObservationPolicy(lidarFrameChCapacity),
		func() bool { return true }, t.Logf); err == nil {
		t.Fatal("a capture was created under a file")
	}
}
