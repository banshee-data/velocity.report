package main

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// scanAnchorNS is an arbitrary non-zero capture start. firstScanNS == 0 is the
// builder's "not yet anchored" sentinel, which makes scanTimeSeconds report 0
// and drops every scan into the warmup phase, so tests that need real scan
// times must anchor away from zero.
const scanAnchorNS int64 = 1_000_000_000

// testConfig is a scene config with short, round durations so scan-phase
// boundaries are easy to reason about in tests.
func testConfig() config {
	return config{
		VoxelMeters:   1.0,
		MinRange:      1.0,
		MaxRange:      90.0,
		WarmupSeconds: 1.0,
		LoopSeconds:   2.0,
		LoopFPS:       10,
		SensorFPS:     10,
	}
}

func TestMakeVoxelKeyBinsNearbyPointsTogether(t *testing.T) {
	const voxel = 1.0

	// Two points inside the same 1m cell must share a key.
	a := makeVoxelKey(1.1, 2.2, 3.3, voxel)
	b := makeVoxelKey(1.9, 2.9, 3.9, voxel)
	if a != b {
		t.Errorf("points in the same cell got different keys: %d vs %d", a, b)
	}

	// A point one cell over must not.
	c := makeVoxelKey(2.1, 2.2, 3.3, voxel)
	if a == c {
		t.Errorf("points in different cells share key %d", a)
	}
}

func TestMakeVoxelKeyDistinguishesAxes(t *testing.T) {
	const voxel = 1.0
	// Shifting along each axis independently must produce distinct keys, or
	// the bit packing has collapsed two axes together.
	base := makeVoxelKey(0, 0, 0, voxel)
	seen := map[voxelKey]string{base: "base"}
	for _, tc := range []struct {
		name    string
		x, y, z float64
	}{
		{"x", 5, 0, 0},
		{"y", 0, 5, 0},
		{"z", 0, 0, 5},
	} {
		k := makeVoxelKey(tc.x, tc.y, tc.z, voxel)
		if prev, dup := seen[k]; dup {
			t.Errorf("%s shift collided with %s (key %d)", tc.name, prev, k)
		}
		seen[k] = tc.name
	}
}

func TestMakeVoxelKeyHandlesNegativeCoordinates(t *testing.T) {
	const voxel = 1.0
	// Negative coordinates floor towards -inf; -0.5 and 0.5 are different cells.
	if makeVoxelKey(-0.5, 0, 0, voxel) == makeVoxelKey(0.5, 0, 0, voxel) {
		t.Error("cells either side of the origin share a key")
	}
}

func TestNewSceneBuilderComputesSubsampleStride(t *testing.T) {
	tests := []struct {
		name       string
		sensorFPS  float64
		loopFPS    float64
		wantStride int
	}{
		{"10Hz sensor at 0.5 fps output", 10, 0.5, 20},
		{"equal rates emit every scan", 10, 10, 1},
		// A loop rate above the sensor rate cannot emit more often than the
		// sensor scans, so the stride floors at 1.
		{"loop faster than sensor floors at 1", 10, 40, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			cfg.SensorFPS, cfg.LoopFPS = tc.sensorFPS, tc.loopFPS

			b := newSceneBuilder(cfg)

			if b.subsample != tc.wantStride {
				t.Errorf("subsample = %d, want %d", b.subsample, tc.wantStride)
			}
			if want := int64(1e9 / tc.sensorFPS); b.scanDurationNS != want {
				t.Errorf("scanDurationNS = %d, want %d", b.scanDurationNS, want)
			}
		})
	}
}

func TestScanTimeSecondsIsZeroBeforeFirstScan(t *testing.T) {
	b := newSceneBuilder(testConfig())
	if got := b.scanTimeSeconds(12345); got != 0 {
		t.Errorf("scanTimeSeconds before any point = %v, want 0", got)
	}
}

func TestScanTimeSecondsMeasuresFromFirstScan(t *testing.T) {
	b := newSceneBuilder(testConfig())
	b.firstScanNS = 1_000_000_000

	if got := b.scanTimeSeconds(3_500_000_000); got != 2.5 {
		t.Errorf("scanTimeSeconds = %v, want 2.5", got)
	}
}

// polarPoint builds a point at a given range and timestamp; azimuth and
// elevation are fixed so range filtering is the only variable.
func polarPoint(distance float64, tsNS int64) l2frames.PointPolar {
	return l2frames.PointPolar{
		Distance:  distance,
		Azimuth:   45,
		Elevation: 0,
		Intensity: 128,
		Timestamp: tsNS,
	}
}

func TestAddPointsPolarIgnoresEmptyBatch(t *testing.T) {
	b := newSceneBuilder(testConfig())
	b.AddPointsPolar(nil)

	if b.pointsSeen != 0 {
		t.Errorf("pointsSeen = %d, want 0", b.pointsSeen)
	}
}

func TestAddPointsPolarRangeFilters(t *testing.T) {
	b := newSceneBuilder(testConfig())

	b.AddPointsPolar([]l2frames.PointPolar{
		polarPoint(0.5, 1_000),  // below MinRange
		polarPoint(10, 1_000),   // kept
		polarPoint(95, 1_000),   // above MaxRange
		polarPoint(89.9, 1_000), // kept
	})

	if b.pointsSeen != 4 {
		t.Errorf("pointsSeen = %d, want 4", b.pointsSeen)
	}
	if b.pointsDrop != 2 {
		t.Errorf("pointsDrop = %d, want 2 (out-of-range points)", b.pointsDrop)
	}
	if len(b.scanPts) != 2 {
		t.Errorf("accumulated %d points, want 2 in-range", len(b.scanPts))
	}
}

func TestAddPointsPolarAnchorsFirstScan(t *testing.T) {
	b := newSceneBuilder(testConfig())
	const first = 5_000_000_000

	b.AddPointsPolar([]l2frames.PointPolar{polarPoint(10, first)})

	if b.firstScanNS != first {
		t.Errorf("firstScanNS = %d, want %d", b.firstScanNS, first)
	}
	if b.scanStartNS != first {
		t.Errorf("scanStartNS = %d, want %d", b.scanStartNS, first)
	}
}

func TestAddPointsPolarFinalisesScanAtTimestampBoundary(t *testing.T) {
	cfg := testConfig()
	b := newSceneBuilder(cfg)
	// 10 Hz sensor: each scan spans 100ms.
	const t0 = 1_000_000_000

	b.AddPointsPolar([]l2frames.PointPolar{
		polarPoint(10, t0),
		polarPoint(11, t0+50_000_000), // same scan
	})
	if b.scanIdx != 0 {
		t.Fatalf("scanIdx = %d, want 0 before the boundary", b.scanIdx)
	}

	// Crossing 100ms closes the first scan.
	b.AddPointsPolar([]l2frames.PointPolar{polarPoint(12, t0+100_000_000)})

	if b.scanIdx != 1 {
		t.Errorf("scanIdx = %d, want 1 after crossing the scan duration", b.scanIdx)
	}
	// Warmup phase: the closed scan's points went into the background map.
	if len(b.bgVoxels) == 0 {
		t.Error("no background voxels recorded from the warmup scan")
	}
}

func TestFinalizeScanWarmupFillsBackgroundFirstWriteWins(t *testing.T) {
	b := newSceneBuilder(testConfig())
	b.firstScanNS = 1_000
	b.scanStartNS = 1_000 // t = 0, inside warmup

	// Two points in the same voxel: the first must win.
	b.scanPts = []cartPoint{
		{X: 1, Y: 1, Z: 1, Intensity: 0.5, Voxel: 42},
		{X: 2, Y: 2, Z: 2, Intensity: 0.9, Voxel: 42},
	}
	b.finalizeScanLocked()

	if len(b.bgVoxels) != 1 {
		t.Fatalf("bgVoxels = %d, want 1", len(b.bgVoxels))
	}
	if got := b.bgVoxels[42]; got[3] != 0.5 {
		t.Errorf("voxel intensity = %v, want 0.5 (first write wins)", got[3])
	}
	if b.pointsKept != 1 {
		t.Errorf("pointsKept = %d, want 1", b.pointsKept)
	}
	// The scan buffer is reused, so it must be reset.
	if len(b.scanPts) != 0 {
		t.Errorf("scanPts = %d, want it truncated after finalise", len(b.scanPts))
	}
}

func TestFinalizeScanLoopEmitsForegroundOnly(t *testing.T) {
	cfg := testConfig()
	cfg.WarmupSeconds = 1.0
	cfg.SensorFPS, cfg.LoopFPS = 10, 10 // stride 1: emit every scan
	b := newSceneBuilder(cfg)

	// firstScanNS == 0 is the "not yet anchored" sentinel, which makes
	// scanTimeSeconds report 0; anchor it so scan times are real.
	b.firstScanNS = scanAnchorNS
	// Background already holds voxel 7.
	b.bgVoxels[7] = scenePoint{0, 0, 0, 0}
	b.loopBaseScan = 0

	// t = 1.5s → past warmup, inside the loop window.
	b.scanStartNS = scanAnchorNS + 1_500_000_000
	b.scanPts = []cartPoint{
		{X: 1, Y: 1, Z: 1, Intensity: 0.3, Voxel: 7},  // background, skipped
		{X: 9, Y: 9, Z: 9, Intensity: 0.7, Voxel: 99}, // foreground, emitted
	}
	b.finalizeScanLocked()

	if len(b.loopFrames) != 1 {
		t.Fatalf("loopFrames = %d, want 1", len(b.loopFrames))
	}
	frame := b.loopFrames[0]
	if len(frame.Moving) != 1 {
		t.Fatalf("moving points = %d, want 1 (background filtered out)", len(frame.Moving))
	}
	if frame.Moving[0][3] != 0.7 {
		t.Errorf("emitted intensity = %v, want 0.7", frame.Moving[0][3])
	}
	// Frame time is relative to the end of warmup.
	if frame.T != 0.5 {
		t.Errorf("frame T = %v, want 0.5 (1.5s minus 1s warmup)", frame.T)
	}
}

func TestFinalizeScanLoopHonoursSubsampleStride(t *testing.T) {
	cfg := testConfig()
	cfg.WarmupSeconds = 1.0
	cfg.SensorFPS, cfg.LoopFPS = 10, 5 // stride 2: emit every other scan
	b := newSceneBuilder(cfg)
	b.firstScanNS = scanAnchorNS
	b.loopBaseScan = 0
	b.scanStartNS = scanAnchorNS + 1_500_000_000

	// scanIdx increments to 1 (odd, relative to base) → skipped.
	b.scanPts = []cartPoint{{X: 1, Y: 1, Z: 1, Voxel: 1}}
	b.finalizeScanLocked()
	if len(b.loopFrames) != 0 {
		t.Fatalf("loopFrames = %d, want 0 on an off-stride scan", len(b.loopFrames))
	}

	// scanIdx increments to 2 (even) → emitted.
	b.scanPts = []cartPoint{{X: 1, Y: 1, Z: 1, Voxel: 1}}
	b.finalizeScanLocked()
	if len(b.loopFrames) != 1 {
		t.Errorf("loopFrames = %d, want 1 on an on-stride scan", len(b.loopFrames))
	}
}

func TestFinalizeScanIgnoresScansPastLoopWindow(t *testing.T) {
	cfg := testConfig() // warmup 1s + loop 2s = 3s
	b := newSceneBuilder(cfg)
	b.firstScanNS = scanAnchorNS
	b.scanStartNS = scanAnchorNS + 10_000_000_000 // t = 10s, well past the window

	b.scanPts = []cartPoint{{X: 1, Y: 1, Z: 1, Voxel: 1}}
	b.finalizeScanLocked()

	if len(b.bgVoxels) != 0 {
		t.Errorf("bgVoxels = %d, want 0 past the loop window", len(b.bgVoxels))
	}
	if len(b.loopFrames) != 0 {
		t.Errorf("loopFrames = %d, want 0 past the loop window", len(b.loopFrames))
	}
}

func TestSnapshotStaticReturnsEverythingWhenTargetUnset(t *testing.T) {
	b := newSceneBuilder(testConfig())
	for i := 0; i < 10; i++ {
		b.bgVoxels[voxelKey(i)] = scenePoint{float32(i), 0, 0, 0}
	}

	for _, target := range []int{0, -1, 10, 50} {
		got := b.snapshotStatic(target)
		if len(got) != 10 {
			t.Errorf("snapshotStatic(%d) returned %d points, want all 10", target, len(got))
		}
	}
}

func TestSnapshotStaticSubsamplesToApproximateTarget(t *testing.T) {
	b := newSceneBuilder(testConfig())
	for i := 0; i < 100; i++ {
		b.bgVoxels[voxelKey(i)] = scenePoint{float32(i), 0, 0, 0}
	}

	got := b.snapshotStatic(10)

	if len(got) != 10 {
		t.Errorf("snapshotStatic(10) returned %d points, want 10", len(got))
	}
}

func TestSnapshotStaticIsDeterministic(t *testing.T) {
	// Map iteration order is random, so the sort over voxel keys is what makes
	// the hero scene byte-stable between runs.
	b := newSceneBuilder(testConfig())
	for i := 0; i < 100; i++ {
		b.bgVoxels[voxelKey(i)] = scenePoint{float32(i), 0, 0, 0}
	}

	first := b.snapshotStatic(20)
	for range 5 {
		got := b.snapshotStatic(20)
		if len(got) != len(first) {
			t.Fatalf("length changed between calls: %d vs %d", len(got), len(first))
		}
		for i := range got {
			if got[i] != first[i] {
				t.Fatalf("point %d changed between calls: %v vs %v", i, got[i], first[i])
			}
		}
	}
}

func TestSnapshotStaticEmptyBackground(t *testing.T) {
	b := newSceneBuilder(testConfig())
	if got := b.snapshotStatic(100); len(got) != 0 {
		t.Errorf("snapshotStatic on an empty background returned %d points, want 0", len(got))
	}
}

func TestDirOf(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"nested path", "public_html/src/data/hero-scene.json", "public_html/src/data"},
		{"single directory", "out/scene.json", "out"},
		{"bare filename has no directory", "scene.json", "."},
		{"absolute path", "/tmp/scene.json", "/tmp"},
		{"trailing slash", "out/", "out"},
		{"empty", "", "."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := dirOf(tc.in); got != tc.want {
				t.Errorf("dirOf(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSceneBuilderNoOpMethods(t *testing.T) {
	// SetMotorSpeed and the packetStats methods exist to satisfy the network
	// interfaces; they must stay safe no-ops.
	b := newSceneBuilder(testConfig())
	b.SetMotorSpeed(600)

	var s packetStats
	s.AddPacket(1)
	s.AddDropped()
	s.AddPoints(10)
	s.LogStats(true)
}
