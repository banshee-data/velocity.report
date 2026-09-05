package scene

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

const baseNs = int64(1765057342040018330)

// writeVRLOG records a VRLOG whose frames carry the given timestamps, three
// tracks each, and returns its path.
func writeVRLOG(t *testing.T, timestamps []int64) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "src.vrlog")
	rec, err := recorder.NewRecorder(dir, "hesai-pandar40p")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	for i, ts := range timestamps {
		set := &l9endpoints.TrackSet{FrameID: uint64(i), TimestampNanos: ts}
		for k := 0; k < 3; k++ {
			set.Tracks = append(set.Tracks, l9endpoints.Track{
				TrackID:  fmt.Sprintf("source-track-%d", k),
				SensorID: "hesai-pandar40p",
				State:    l9endpoints.TrackStateConfirmed,
				// 1.23456 exercises rounding; 0.8 must survive exactly.
				X: 1.23456, Y: -2.98765, Z: 0.8,
				VX: 11.111, VY: -0.555,
				SpeedMps: 12.3456, HeadingRad: 0.123456,
				BBoxLength: 4.4444, BBoxWidth: 1.8888, BBoxHeight: 1.5555,
				BBoxHeadingRad:  0.987654,
				ObjectClass:     "car",
				ClassConfidence: 0.87654,
				Covariance4x4:   []float32{1, 2, 3, 4},
			})
		}
		if err := rec.Record(&l9endpoints.FrameBundle{
			FrameID: uint64(i), TimestampNanos: ts, SensorID: "hesai-pandar40p",
			FrameType: l9endpoints.FrameTypeForeground, Tracks: set,
		}); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return dir
}

func evenTimestamps(n int) []int64 {
	ts := make([]int64, n)
	for i := range ts {
		ts[i] = baseNs + int64(i)*100_000_000
	}
	return ts
}

// readFrames decodes every NDJSON line an export wrote, in chunk order.
func readFrames(t *testing.T, outDir string) []Frame {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(outDir, "frames", "*.ndjson.gz"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no chunk files in %s (err %v)", outDir, err)
	}
	var out []Frame
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			t.Fatalf("open %s: %v", p, err)
		}
		gz, err := gzip.NewReader(f)
		if err != nil {
			t.Fatalf("gzip %s: %v", p, err)
		}
		dec := json.NewDecoder(gz)
		for dec.More() {
			var fr Frame
			if err := dec.Decode(&fr); err != nil {
				t.Fatalf("decode %s: %v", p, err)
			}
			out = append(out, fr)
		}
		_ = gz.Close()
		_ = f.Close()
	}
	return out
}

func readHeader(t *testing.T, outDir string) Header {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(outDir, "header.json"))
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	var h Header
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	return h
}

func TestExportStrideRetainsEveryNthFrame(t *testing.T) {
	src := writeVRLOG(t, evenTimestamps(20))
	for _, stride := range []int{1, 2, 5} {
		out := filepath.Join(t.TempDir(), "out")
		res, err := Export(Options{VRLOGPath: src, OutDir: out, Stride: stride})
		if err != nil {
			t.Fatalf("stride %d: %v", stride, err)
		}
		want := 20 / stride
		if res.Header.FrameCount != want {
			t.Errorf("stride %d: retained %d frames, want %d", stride, res.Header.FrameCount, want)
		}
		frames := readFrames(t, out)
		for i, f := range frames {
			wantUs := int64(i*stride) * 100_000
			if f.TimeUs != wantUs {
				t.Errorf("stride %d frame %d: t %d us, want %d", stride, i, f.TimeUs, wantUs)
			}
		}
	}
}

func TestExportPreservesTimestampsExactly(t *testing.T) {
	// Deliberately uneven, as real rotations are: 198, 203, 201 ms.
	ts := []int64{baseNs, baseNs + 198_000_000, baseNs + 401_000_000, baseNs + 602_000_000}
	src := writeVRLOG(t, ts)
	out := filepath.Join(t.TempDir(), "out")
	if _, err := Export(Options{VRLOGPath: src, OutDir: out}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	frames := readFrames(t, out)
	if len(frames) != len(ts) {
		t.Fatalf("got %d frames, want %d", len(frames), len(ts))
	}
	// Relative microseconds must reproduce the uneven source intervals exactly.
	for i, f := range frames {
		wantUs := (ts[i] - ts[0]) / 1000
		if f.TimeUs != wantUs {
			t.Errorf("frame %d: t %d us, want %d (intervals must survive intact)", i, f.TimeUs, wantUs)
		}
	}
}

func TestExportRoundsGeometryButNotTime(t *testing.T) {
	src := writeVRLOG(t, evenTimestamps(2))
	out := filepath.Join(t.TempDir(), "out")
	if _, err := Export(Options{VRLOGPath: src, OutDir: out}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	tr := readFrames(t, out)[0].Tracks[0]
	for _, c := range []struct {
		name string
		got  float64
		want float64
	}{
		{"x", tr.X, 1.23},
		{"y", tr.Y, -2.99},
		{"z", tr.Z, 0.8},
		{"speed", tr.Speed, 12.35},
		{"length", tr.Length, 4.44},
		{"heading", tr.Heading, 0.123},
		{"box yaw", tr.BoxYaw, 0.988},
	} {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestExportDropsUnneededTrackFields(t *testing.T) {
	src := writeVRLOG(t, evenTimestamps(2))
	out := filepath.Join(t.TempDir(), "out")
	if _, err := Export(Options{VRLOGPath: src, OutDir: out}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(out, "frames", "chunk_0000.ndjson.gz"))
	if err != nil {
		t.Fatalf("read chunk: %v", err)
	}
	_ = b
	frames := readFrames(t, out)
	raw, err := json.Marshal(frames[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, banned := range []string{"covariance", "Covariance", "hits", "misses", "sensor_id"} {
		if strings.Contains(string(raw), banned) {
			t.Errorf("exported frame carries %q; the viewer does not need it", banned)
		}
	}
}

func TestExportRekeysTrackIDsLocally(t *testing.T) {
	src := writeVRLOG(t, evenTimestamps(4))
	out := filepath.Join(t.TempDir(), "out")
	if _, err := Export(Options{VRLOGPath: src, OutDir: out}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	frames := readFrames(t, out)
	first := frames[0].Tracks[0].ID
	if strings.Contains(first, "source-track") {
		t.Errorf("track ID %q leaks the source identifier", first)
	}
	// Stable within the part, so trails stay continuous.
	if got := frames[3].Tracks[0].ID; got != first {
		t.Errorf("track ID changed across frames: %q then %q", first, got)
	}
}

func TestExportChunkRollover(t *testing.T) {
	src := writeVRLOG(t, evenTimestamps(25))
	out := filepath.Join(t.TempDir(), "out")
	res, err := Export(Options{VRLOGPath: src, OutDir: out, ChunkFrames: 10})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if res.Chunks != 3 {
		t.Errorf("got %d chunks, want 3 (10+10+5)", res.Chunks)
	}
	var idx ChunkIndex
	b, err := os.ReadFile(filepath.Join(out, "index.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	if len(idx.Chunks) != 3 {
		t.Fatalf("index lists %d chunks, want 3", len(idx.Chunks))
	}
	if idx.Chunks[0].Frames != 10 || idx.Chunks[2].Frames != 5 {
		t.Errorf("chunk frame counts = %d,%d,%d; want 10,10,5",
			idx.Chunks[0].Frames, idx.Chunks[1].Frames, idx.Chunks[2].Frames)
	}
	// Index timestamps must bracket their chunk and advance across chunks.
	for i, c := range idx.Chunks {
		if c.StartUs > c.EndUs {
			t.Errorf("chunk %d: start %d after end %d", i, c.StartUs, c.EndUs)
		}
		if i > 0 && c.StartUs <= idx.Chunks[i-1].EndUs {
			t.Errorf("chunk %d does not advance past chunk %d", i, i-1)
		}
	}
}

func TestExportDropsLeadingNonMonotonicFrame(t *testing.T) {
	// The documented artefact: frame 0 carries a wall-clock stamp from the end
	// of the recording, every later frame carries capture time.
	ts := evenTimestamps(10)
	ts[0] = baseNs + 999_000_000_000
	src := writeVRLOG(t, ts)

	out := filepath.Join(t.TempDir(), "out")
	res, err := Export(Options{VRLOGPath: src, OutDir: out})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if res.DroppedNonMonotonic != 1 {
		t.Errorf("dropped %d frames, want exactly 1", res.DroppedNonMonotonic)
	}
	if res.Header.FrameCount != 9 {
		t.Errorf("retained %d frames, want 9", res.Header.FrameCount)
	}
	if res.Header.DurationSec <= 0 {
		t.Errorf("duration %.3f s; dropping the bad frame should give a positive span",
			res.Header.DurationSec)
	}
	frames := readFrames(t, out)
	for i := 1; i < len(frames); i++ {
		if frames[i].TimeUs <= frames[i-1].TimeUs {
			t.Fatalf("frame %d does not advance: %d then %d", i, frames[i-1].TimeUs, frames[i].TimeUs)
		}
	}
}

func TestExportRefusesThoroughlyMixedTimeDomains(t *testing.T) {
	// Alternating domains: not one bad frame, a broken recording.
	ts := make([]int64, 40)
	for i := range ts {
		if i%2 == 0 {
			ts[i] = baseNs + int64(i)*100_000_000
		} else {
			ts[i] = baseNs + 999_000_000_000 + int64(i)*100_000_000
		}
	}
	src := writeVRLOG(t, ts)
	out := filepath.Join(t.TempDir(), "out")
	if _, err := Export(Options{VRLOGPath: src, OutDir: out}); err == nil {
		t.Fatal("expected an error for a recording with mixed time domains")
	} else if !strings.Contains(err.Error(), "non-monotonic") {
		t.Errorf("error %q does not explain the time-domain problem", err)
	}
}

func TestExportHeaderMetadata(t *testing.T) {
	src := writeVRLOG(t, evenTimestamps(12))
	out := filepath.Join(t.TempDir(), "out")
	if _, err := Export(Options{
		VRLOGPath: src, OutDir: out, Stride: 2, Site: "soma1", Title: "SoMa 1",
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	h := readHeader(t, out)
	if h.Version != FormatVersion {
		t.Errorf("version %d, want %d", h.Version, FormatVersion)
	}
	if h.Export != KindTracks {
		t.Errorf("export kind %q, want %q", h.Export, KindTracks)
	}
	if h.FrameStride != 2 {
		t.Errorf("frame_stride %d, want 2", h.FrameStride)
	}
	if h.ChunkEncoding != "gzip" {
		t.Errorf("chunk_encoding %q, want gzip", h.ChunkEncoding)
	}
	if h.Site != "soma1" || h.Title != "SoMa 1" {
		t.Errorf("site/title = %q/%q", h.Site, h.Title)
	}
	if h.SourceVRLOGSHA256 == "" {
		t.Error("source_vrlog_sha256 missing; an export must name the run it came from")
	}
	if h.DurationSec <= 0 {
		t.Errorf("duration_sec %.3f, want positive", h.DurationSec)
	}
}

func TestExportRejectsMissingAndEmptySources(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	if _, err := Export(Options{VRLOGPath: filepath.Join(t.TempDir(), "nope.vrlog"), OutDir: out}); err == nil {
		t.Error("expected an error for a source that does not exist")
	}

	src := writeVRLOG(t, evenTimestamps(4))
	if _, err := Export(Options{VRLOGPath: src, OutDir: out, StartFrame: 99}); err == nil {
		t.Error("expected an error when the start frame is past the end of the recording")
	}
}

func TestExportFrameRangeBounds(t *testing.T) {
	src := writeVRLOG(t, evenTimestamps(30))
	out := filepath.Join(t.TempDir(), "out")
	res, err := Export(Options{VRLOGPath: src, OutDir: out, StartFrame: 5, FrameCount: 10})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if res.Header.FrameCount != 10 {
		t.Fatalf("retained %d frames, want 10", res.Header.FrameCount)
	}
	frames := readFrames(t, out)
	// The window is rebased, so its own first frame is offset zero.
	if frames[0].TimeUs != 0 {
		t.Errorf("first frame t %d us, want 0 (window is rebased to its own start)", frames[0].TimeUs)
	}
	if h := readHeader(t, out); h.StartNs != "1765057342540018330" {
		t.Errorf("start_ns %q, want the absolute time of source frame 5", h.StartNs)
	}
}

func TestEncodeBase36(t *testing.T) {
	for _, c := range []struct {
		in   int
		want string
	}{{0, "0"}, {9, "9"}, {10, "a"}, {35, "z"}, {36, "10"}, {1295, "zz"}} {
		if got := encodeBase36(c.in); got != c.want {
			t.Errorf("encodeBase36(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
