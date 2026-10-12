package annotation

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// sourceReturn is a return as the parser names it.
type sourceReturn struct {
	key            uint64
	block, channel int
}

// recordKeyedFrames records frames the way a replay does: through the
// adapter, then foreground-only, either by the adapter itself (split
// streaming) or by the publisher's decimation of a full frame.
func recordKeyedFrames(t *testing.T, split bool, frames [][]sourceReturn, masks [][]bool) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vrlog")
	rec, err := recorder.NewRecorder(dir, "hesai-pandar40p")
	if err != nil {
		t.Fatal(err)
	}
	adapter := l9endpoints.NewFrameAdapter("hesai-pandar40p")
	adapter.SplitStreaming = split
	for i, rets := range frames {
		frame := &l2frames.LiDARFrame{StartTimestamp: time.Unix(0, int64(i+1)*100_000_000)}
		for j, r := range rets {
			frame.Points = append(frame.Points, l2frames.Point{
				X: float64(100*i + j), Y: 1, Z: 1, Intensity: 9,
				PacketKey: r.key, BlockID: r.block, Channel: r.channel,
			})
		}
		bundle := adapter.AdaptFrame(frame, masks[i], nil, nil, nil).(*l9endpoints.FrameBundle)
		if !split {
			bundle.PointCloud.ApplyDecimation(l9endpoints.DecimationForegroundOnly, 0)
		}
		if err := rec.Record(bundle); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

// locate names a pack return by its packet key, block and channel.
func locate(p *Pack, r uint64) sourceReturn {
	l := p.Manifest.ReturnLayout
	per := uint64(l.Blocks * l.Channels)
	within := r % per
	return sourceReturn{p.Packets[r/per], int(within / uint64(l.Channels)), int(within%uint64(l.Channels)) + 1}
}

// The rotation wraps inside packet 2, so its first five blocks end frame 0
// and its last five start frame 1. The pack lists it once.
var (
	keyedFrames = [][]sourceReturn{
		{{0x11, 0, 1}, {0x11, 0, 2}, {0x11, 7, 40}, {0x22, 0, 1}, {0x22, 4, 13}},
		{{0x22, 5, 1}, {0x22, 9, 40}, {0x33, 0, 1}, {0x33, 3, 3}},
		{{0x44, 0, 5}, {0x44, 9, 39}},
	}
	keyedMasks = [][]bool{{true, false, true, false, true}, {true, true, false, true}, {false, true}}
)

// Gate: every exported point carries a return, and the pack's packet table
// and return numbers name exactly the (packet key, block, channel) of the
// source point it holds.
func TestExportWritesReturnsThatNameTheSourceReturns(t *testing.T) {
	for _, split := range []bool{true, false} {
		src := recordKeyedFrames(t, split, keyedFrames, keyedMasks)
		pack, err := Export(ExportConfig{VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "pack"), Coverage: CoverageForegroundOnly})
		if err != nil {
			t.Fatalf("split=%v: export: %v", split, err)
		}
		if pack.Manifest.SchemaVersion != PackSchemaVersion || !pack.HasReturns() {
			t.Fatalf("split=%v: version %d pack, returns %v", split, pack.Manifest.SchemaVersion, pack.HasReturns())
		}
		if l := pack.Manifest.ReturnLayout; l == nil || *l != (ReturnLayout{"hesai-pandar40p", 10, 40}) {
			t.Fatalf("layout %+v", l)
		}
		if want := []uint64{0x11, 0x22, 0x33, 0x44}; !slices.Equal(pack.Packets, want) {
			t.Fatalf("split=%v: packet table %x, want %x: a packet shared at a frame boundary is listed once", split, pack.Packets, want)
		}
		for s := range pack.Samples {
			returns, err := pack.ReturnsAt(s)
			if err != nil {
				t.Fatal(err)
			}
			pts, _ := pack.PointsAt(s)
			var want []sourceReturn
			var wantX []float32
			for j, kept := range keyedMasks[s] {
				if kept {
					want = append(want, keyedFrames[s][j])
					wantX = append(wantX, float32(100*s+j))
				}
			}
			got := make([]sourceReturn, len(returns))
			for j, r := range returns {
				got[j] = locate(pack, r)
			}
			if !slices.Equal(got, want) || !slices.Equal(pts.X, wantX) {
				t.Fatalf("split=%v sample %d: returns name %v at x=%v, want %v at x=%v", split, s, got, pts.X, want, wantX)
			}
			// The window is the whole frame the operator saw, background too.
			smp := pack.Samples[s]
			first, last := keyedFrames[s][0], keyedFrames[s][len(keyedFrames[s])-1]
			if locate(pack, smp.ReturnStart) != first || locate(pack, smp.ReturnEnd-1) != last {
				t.Fatalf("split=%v sample %d: frame range [%d,%d) is not the frame's returns", split, s, smp.ReturnStart, smp.ReturnEnd)
			}
		}
		if a, b := pack.Samples[0], pack.Samples[1]; b.ReturnStart < a.ReturnEnd {
			t.Fatalf("frames overlap: [%d,%d) and [%d,%d)", a.ReturnStart, a.ReturnEnd, b.ReturnStart, b.ReturnEnd)
		}
	}

	// A window that starts after the shared packet's first frame lists it
	// first, and numbers from there.
	src := recordKeyedFrames(t, true, keyedFrames, keyedMasks)
	pack, err := Export(ExportConfig{VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "w"), Coverage: CoverageForegroundOnly, StartNs: 200_000_000})
	if err != nil {
		t.Fatal(err)
	}
	if want := []uint64{0x22, 0x33, 0x44}; !slices.Equal(pack.Packets, want) {
		t.Fatalf("windowed packet table %x, want %x", pack.Packets, want)
	}
	if pack.Samples[0].ReturnStart != 5*40 {
		t.Fatalf("windowed frame starts at return %d, want block 5 of the first packet (200)", pack.Samples[0].ReturnStart)
	}
}

// A recording made before return identity can still be read, but its points
// cannot be cited, and the operator is told how to get one that can.
func TestExportRefusesARecordingWithoutReturnIdentity(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vrlog")
	rec, err := recorder.NewRecorder(dir, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	pc := &l9endpoints.PointCloudFrame{
		TimestampNanos: 1000, X: []float32{1}, Y: []float32{1}, Z: []float32{1},
		Intensity: []uint8{1}, Classification: []uint8{1}, PointCount: 1,
	}
	if err := rec.Record(&l9endpoints.FrameBundle{TimestampNanos: 1000, PointCloud: pc}); err != nil {
		t.Fatal(err)
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "pack")
	_, err = Export(ExportConfig{VRLOGPath: dir, OutDir: out, Coverage: CoverageForegroundOnly})
	if !errors.Is(err, ErrNoReturnIdentity) {
		t.Fatalf("exported a recording without return identity: %v", err)
	}
	for _, phrase := range []string{"Re-record", "replaying the PCAP", "this build"} {
		if !strings.Contains(err.Error(), phrase) {
			t.Errorf("error %q does not tell the operator to %s", err, phrase)
		}
	}
	if _, statErr := OpenPack(out); statErr == nil {
		t.Error("a refused export left a pack behind")
	}
}

// A packet that comes back later than the next frame's start, or twice in
// one frame, leaves its returns ambiguous, and the export is refused.
func TestExportRefusesARepeatedPacket(t *testing.T) {
	for name, tc := range map[string]struct {
		frames [][]sourceReturn
		masks  [][]bool
		want   string
	}{
		"in a later frame": {
			[][]sourceReturn{{{0x11, 0, 1}}, {{0x22, 0, 1}}, {{0x11, 0, 2}}},
			[][]bool{{true}, {true}, {true}},
			"packet 0000000000000011 appeared earlier",
		},
		"twice in one frame": {
			[][]sourceReturn{{{0x11, 0, 1}, {0x22, 0, 1}, {0x11, 0, 2}}},
			[][]bool{{true, true, true}},
			"out of point order",
		},
		"inside the previous frame": {
			[][]sourceReturn{{{0x11, 0, 1}, {0x11, 5, 1}}, {{0x11, 2, 1}, {0x22, 0, 1}}},
			[][]bool{{true, true}, {true, true}},
			"two frames claim the same returns",
		},
	} {
		src := recordKeyedFrames(t, true, tc.frames, tc.masks)
		_, err := Export(ExportConfig{VRLOGPath: src, OutDir: filepath.Join(t.TempDir(), "pack"), Coverage: CoverageForegroundOnly})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error saying %q", name, err, tc.want)
		}
	}
}
