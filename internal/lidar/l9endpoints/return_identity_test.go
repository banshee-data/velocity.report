package l9endpoints

import (
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/pb"
	"slices"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// keyedFrame is three packets' worth of returns as the parser emits them:
// packet order, then block, then channel. The first packet contributes only
// its last block, as when the previous frame took the rest of it.
//
//	point  packet  block  channel  return
//	0      A       9      39       0*400 + 9*40 + 38 = 398
//	1      A       9      40       399
//	2      B       0      1        400
//	3      B       0      2        401
//	4      B       5      17       400 + 200 + 16 = 616
//	5      C       0      3        802
func keyedFrame() (*l2frames.LiDARFrame, []uint64, []uint32) {
	const a, b, c = 0xA1, 0xB2, 0xC3
	type ret struct {
		key            uint64
		block, channel int
	}
	rets := []ret{{a, 9, 39}, {a, 9, 40}, {b, 0, 1}, {b, 0, 2}, {b, 5, 17}, {c, 0, 3}}
	frame := &l2frames.LiDARFrame{SensorID: "hesai-01", StartTimestamp: time.Unix(10, 0)}
	for i, r := range rets {
		frame.Points = append(frame.Points, l2frames.Point{
			X: float64(i), Y: float64(10 * i), Z: 1, Intensity: uint8(i),
			PacketKey: r.key, BlockID: r.block, Channel: r.channel,
		})
	}
	return frame, []uint64{a, b, c}, []uint32{398, 399, 400, 401, 616, 802}
}

func checkLayout(t *testing.T, pc *PointCloudFrame, keys []uint64) {
	t.Helper()
	if !pc.HasReturnIdentity() {
		t.Fatalf("no return identity: %+v", pc)
	}
	if !slices.Equal(pc.PacketKeys, keys) {
		t.Errorf("packet keys %x, want %x", pc.PacketKeys, keys)
	}
	if pc.ReturnStart != 398 || pc.ReturnEnd != 803 {
		t.Errorf("frame range [%d,%d), want [398,803): it covers every return, emitted or not", pc.ReturnStart, pc.ReturnEnd)
	}
	if pc.ReturnModel != "hesai-pandar40p" || pc.ReturnBlocks != 10 || pc.ReturnChannels != 40 {
		t.Errorf("layout %q %d×%d", pc.ReturnModel, pc.ReturnBlocks, pc.ReturnChannels)
	}
}

func TestAdapterNumbersEveryReturnOfAFullFrame(t *testing.T) {
	frame, keys, refs := keyedFrame()
	pc := toFrameBundle(t, NewFrameAdapter("hesai-01").AdaptFrame(frame, []bool{false, true}, nil, nil, nil)).PointCloud
	checkLayout(t, pc, keys)
	if !slices.Equal(pc.ReturnRefs, refs) {
		t.Fatalf("refs %v, want %v", pc.ReturnRefs, refs)
	}
	if pc.Classification[1] != 1 || pc.Classification[0] != 0 {
		t.Fatal("classification changed")
	}
	pc.Release()
	if pc.ReturnRefs != nil || pc.PacketKeys != nil {
		t.Fatal("release kept the identity of a frame whose slices went back to the pool")
	}
	pc.Release() // a second release must not pool the same array twice
}

// Foreground-only frames emit fewer points, but the packet table and range
// still describe the whole frame: packet A has no foreground and stays.
func TestAdapterNumbersForegroundOnlyPointsWithinTheWholeFrame(t *testing.T) {
	frame, keys, refs := keyedFrame()
	adapter := NewFrameAdapter("hesai-01")
	adapter.SplitStreaming = true
	mask := []bool{false, false, true, false, true, true}
	pc := toFrameBundle(t, adapter.AdaptFrame(frame, mask, nil, nil, nil)).PointCloud
	checkLayout(t, pc, keys)
	want := []uint32{refs[2], refs[4], refs[5]}
	if !slices.Equal(pc.ReturnRefs, want) || len(pc.X) != 3 || pc.X[1] != 4 {
		t.Fatalf("refs %v for %d points, want %v", pc.ReturnRefs, len(pc.X), want)
	}

	// A frame with no foreground still names its returns: it was seen.
	empty := toFrameBundle(t, adapter.AdaptFrame(frame, make([]bool, 6), nil, nil, nil)).PointCloud
	checkLayout(t, empty, keys)
	if len(empty.ReturnRefs) != 0 {
		t.Fatalf("refs %v for no points", empty.ReturnRefs)
	}
}

// One return without a packet key, or outside the layout, leaves the frame
// without identity on either path rather than with a partial one.
func TestAdapterLeavesIdentityEmptyWhenAnyReturnIsUnkeyed(t *testing.T) {
	for name, spoil := range map[string]func(*l2frames.Point){
		"no key":     func(p *l2frames.Point) { p.PacketKey = 0 },
		"channel 0":  func(p *l2frames.Point) { p.Channel = 0 },
		"channel 41": func(p *l2frames.Point) { p.Channel = 41 },
		"block 10":   func(p *l2frames.Point) { p.BlockID = 10 },
		"block -1":   func(p *l2frames.Point) { p.BlockID = -1 },
	} {
		for _, split := range []bool{false, true} {
			frame, _, _ := keyedFrame()
			spoil(&frame.Points[3]) // a background point: it still counts
			adapter := NewFrameAdapter("hesai-01")
			adapter.SplitStreaming = split
			pc := toFrameBundle(t, adapter.AdaptFrame(frame, []bool{true, true, true, false, true, true}, nil, nil, nil)).PointCloud
			if pc.HasReturnIdentity() || pc.PacketKeys != nil || pc.ReturnRefs != nil || pc.ReturnEnd != 0 || pc.ReturnModel != "" {
				t.Errorf("%s, split=%v: partial identity %+v", name, split, pc)
			}
			if len(pc.X) == 0 {
				t.Errorf("%s, split=%v: points dropped with the identity", name, split)
			}
		}
	}
}

// Every decimation mode keeps each surviving point's return number with it.
func TestDecimationKeepsReturnRefsAligned(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mode  DecimationMode
		ratio float32
	}{
		{"foreground only", DecimationForegroundOnly, 0},
		{"uniform", DecimationUniform, 0.5},
		{"voxel", DecimationVoxel, 0.5},
	} {
		frame := &l2frames.LiDARFrame{StartTimestamp: time.Unix(10, 0)}
		mask := make([]bool, 900)
		for i := range mask {
			// Points in pairs a centimetre apart, so voxels merge them.
			frame.Points = append(frame.Points, l2frames.Point{
				X: float64(i/2) + 0.01*float64(i%2), Y: 1, Z: 1,
				PacketKey: uint64(100 + i/400), BlockID: (i % 400) / 40, Channel: i%40 + 1,
			})
			mask[i] = i%3 != 0
		}
		pc := toFrameBundle(t, NewFrameAdapter("s").AdaptFrame(frame, mask, nil, nil, nil)).PointCloud
		// The return a coordinate came from, before decimation.
		byX := make(map[float32]uint32, len(pc.X))
		for i, x := range pc.X {
			byX[x] = pc.ReturnRefs[i]
		}
		keys, start, end := slices.Clone(pc.PacketKeys), pc.ReturnStart, pc.ReturnEnd
		before := pc.PointCount

		pc.ApplyDecimation(tc.mode, tc.ratio)

		if pc.PointCount >= before || pc.PointCount == 0 {
			t.Fatalf("%s: %d of %d points kept; the case does not decimate", tc.name, pc.PointCount, before)
		}
		if !pc.HasReturnIdentity() || len(pc.ReturnRefs) != pc.PointCount {
			t.Fatalf("%s: %d refs for %d points", tc.name, len(pc.ReturnRefs), pc.PointCount)
		}
		for i, x := range pc.X {
			if pc.ReturnRefs[i] != byX[x] {
				t.Fatalf("%s: point %d at x=%v carries return %d, it came from %d", tc.name, i, x, pc.ReturnRefs[i], byX[x])
			}
			if i > 0 && pc.ReturnRefs[i] <= pc.ReturnRefs[i-1] {
				t.Fatalf("%s: returns out of point order at %d", tc.name, i)
			}
		}
		if !slices.Equal(pc.PacketKeys, keys) || pc.ReturnStart != start || pc.ReturnEnd != end {
			t.Fatalf("%s: the frame's table or range changed with its points", tc.name)
		}
	}
}

// Refs that do not match the points cannot be followed through a decimation,
// so the identity goes rather than name the wrong returns.
func TestDecimationDropsMisalignedReturnRefs(t *testing.T) {
	pc := &PointCloudFrame{
		X: []float32{1, 2, 3}, Y: []float32{1, 2, 3}, Z: []float32{1, 2, 3},
		Intensity: []uint8{1, 2, 3}, Classification: []uint8{1, 0, 1}, PointCount: 3,
	}
	FixtureReturnIdentity(pc, 7)
	pc.ReturnRefs = pc.ReturnRefs[:2]
	if pc.HasReturnIdentity() {
		t.Fatal("two refs for three points counted as identity")
	}
	pc.ApplyDecimation(DecimationForegroundOnly, 0)
	if pc.PacketKeys != nil || pc.ReturnRefs != nil || pc.ReturnModel != "" || pc.PointCount != 2 {
		t.Fatalf("misaligned identity survived: %+v", pc)
	}
}

func TestFixtureReturnIdentityIsValid(t *testing.T) {
	for _, n := range []int{0, 1, 400, 401} {
		pc := &PointCloudFrame{X: make([]float32, n), Y: make([]float32, n), Z: make([]float32, n)}
		FixtureReturnIdentity(pc, 50)
		if !pc.HasReturnIdentity() {
			t.Fatalf("%d points: fixture identity invalid: %+v", n, pc)
		}
		if want := max(1, (n+399)/400); len(pc.PacketKeys) != want || pc.PacketKeys[0] != 50 {
			t.Fatalf("%d points: keys %v", n, pc.PacketKeys)
		}
	}
	var nilFrame *PointCloudFrame
	if nilFrame.HasReturnIdentity() {
		t.Fatal("nil frame has identity")
	}
	pc := &PointCloudFrame{X: make([]float32, 3)}
	FixtureReturnIdentity(pc, 1)
	pc.ReturnEnd = 401 // past one packet's returns
	if pc.HasReturnIdentity() {
		t.Fatal("a range past the packet table counted as identity")
	}
}

func TestStreamCarriesPacketKeysButNotReturns(t *testing.T) {
	pc := &PointCloudFrame{X: []float32{1, 2}, Y: []float32{0, 0}, Z: []float32{0, 0}, PointCount: 2}
	FixtureReturnIdentity(pc, 7<<20)
	bundle := &FrameBundle{FrameID: 1, TimestampNanos: 5, PointCloud: pc}
	out := frameBundleToProto(bundle, &pb.StreamRequest{IncludePoints: true})
	if len(out.PointCloud.PacketKeys) == 0 || out.PointCloud.PacketKeys[0] != pc.PacketKeys[0] {
		t.Fatalf("stream packet keys %v, want %v", out.PointCloud.PacketKeys, pc.PacketKeys)
	}
	if len(out.PointCloud.ReturnRefs) != 0 {
		t.Fatalf("stream carries %d per-point returns", len(out.PointCloud.ReturnRefs))
	}
}
