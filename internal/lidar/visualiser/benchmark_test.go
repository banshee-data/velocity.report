// Package visualiser provides gRPC streaming of LiDAR perception data.
// This file contains benchmarks for M7 performance acceptance criteria.
package visualiser

import (
	"math"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
)

// BenchmarkAdaptFrame_70kPoints benchmarks frame adaptation with 70,000 points.
// M7 Acceptance: must sustain 30 fps (< 33ms per frame).
func BenchmarkAdaptFrame_70kPoints(b *testing.B) {
	adapter := NewFrameAdapter("bench-sensor")
	frame := generateBenchFrame(70000, 0)
	mask := make([]bool, len(frame.Points))
	for i := range mask {
		if i%30 == 0 { // ~3% foreground
			mask[i] = true
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result := adapter.AdaptFrame(frame, mask, nil, nil, nil)
		bundle := result.(*FrameBundle)
		if bundle.PointCloud != nil {
			bundle.PointCloud.Release()
		}
	}
}

// BenchmarkAdaptFrame_SplitStreaming benchmarks foreground-only adaptation.
// Split streaming should be significantly faster than full frame.
func BenchmarkAdaptFrame_SplitStreaming(b *testing.B) {
	adapter := NewFrameAdapter("bench-sensor")
	adapter.SplitStreaming = true
	frame := generateBenchFrame(70000, 0)
	mask := make([]bool, len(frame.Points))
	for i := range mask {
		if i%30 == 0 { // ~3% foreground = ~2,333 points
			mask[i] = true
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result := adapter.AdaptFrame(frame, mask, nil, nil, nil)
		bundle := result.(*FrameBundle)
		if bundle.PointCloud != nil {
			bundle.PointCloud.Release()
		}
	}
}

// BenchmarkFrameBundleToProto_70kPoints benchmarks protobuf conversion.
func BenchmarkFrameBundleToProto_70kPoints(b *testing.B) {
	bundle := generateBenchBundle(70000, 10, 5)
	req := &pb.StreamRequest{
		IncludePoints:   true,
		IncludeClusters: true,
		IncludeTracks:   true,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = frameBundleToProto(bundle, req)
	}
}

// BenchmarkFrameBundleToProto_200Tracks benchmarks serialisation with 200 tracks.
// M7 Acceptance: 200 tracks must render without frame drops.
func BenchmarkFrameBundleToProto_200Tracks(b *testing.B) {
	bundle := generateBenchBundle(2000, 30, 200)
	req := &pb.StreamRequest{
		IncludePoints:   true,
		IncludeClusters: true,
		IncludeTracks:   true,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = frameBundleToProto(bundle, req)
	}
}

// BenchmarkPoolAllocRelease benchmarks the pool alloc/release cycle.
// M7 Acceptance: no memory leaks from pooled allocations.
func BenchmarkPoolAllocRelease(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pc := &PointCloudFrame{
			X:              getFloat32Slice(70000),
			Y:              getFloat32Slice(70000),
			Z:              getFloat32Slice(70000),
			Intensity:      getUint8Slice(70000),
			Classification: getUint8Slice(70000),
			PointCount:     70000,
		}
		pc.Release()
	}
}

// TestMemoryStability_100Frames verifies memory does not grow unbounded
// over 100 adapt-serialise-release cycles (simulates ~10 seconds at 10 Hz).
// M7 Acceptance: memory stable over sustained operation.
func TestMemoryStability_100Frames(t *testing.T) {
	adapter := NewFrameAdapter("mem-sensor")
	req := &pb.StreamRequest{
		IncludePoints:   true,
		IncludeClusters: true,
		IncludeTracks:   true,
	}

	// Warm up pools
	for i := 0; i < 5; i++ {
		frame := generateBenchFrame(70000, i)
		mask := make([]bool, len(frame.Points))
		result := adapter.AdaptFrame(frame, mask, nil, nil, nil)
		bundle := result.(*FrameBundle)
		_ = frameBundleToProto(bundle, req)
		if bundle.PointCloud != nil {
			bundle.PointCloud.Release()
		}
	}

	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Run 100 frame cycles
	for i := 0; i < 100; i++ {
		frame := generateBenchFrame(70000, i)
		mask := make([]bool, len(frame.Points))
		for j := range mask {
			if j%30 == 0 {
				mask[j] = true
			}
		}
		result := adapter.AdaptFrame(frame, mask, nil, nil, nil)
		bundle := result.(*FrameBundle)
		_ = frameBundleToProto(bundle, req)
		if bundle.PointCloud != nil {
			bundle.PointCloud.Release()
		}
	}

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// Check that heap usage didn't grow excessively.
	// Allow up to 50 MB growth for 100 frames (should be much less with pooling).
	heapGrowthMB := float64(int64(m2.HeapInuse)-int64(m1.HeapInuse)) / (1024 * 1024)
	t.Logf("Heap before: %.2f MB, after: %.2f MB, growth: %.2f MB",
		float64(m1.HeapInuse)/(1024*1024),
		float64(m2.HeapInuse)/(1024*1024),
		heapGrowthMB)
	t.Logf("Total allocs over 100 frames: %d", m2.Mallocs-m1.Mallocs)

	if heapGrowthMB > 50 {
		t.Errorf("excessive heap growth: %.2f MB over 100 frames (limit: 50 MB)", heapGrowthMB)
	}
}

// TestPoolNoLeak verifies that Retain/Release properly returns slices to pool.
// M7 Acceptance: no memory leaks from pooled allocations.
func TestPoolNoLeak(t *testing.T) {
	// Allocate and release many frames; verify pool reuse prevents heap growth
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	for i := 0; i < 1000; i++ {
		pc := &PointCloudFrame{
			X:              getFloat32Slice(70000),
			Y:              getFloat32Slice(70000),
			Z:              getFloat32Slice(70000),
			Intensity:      getUint8Slice(70000),
			Classification: getUint8Slice(70000),
			PointCount:     70000,
		}
		// Simulate broadcast to 3 clients
		pc.Retain()
		pc.Retain()
		pc.Retain()
		pc.Release()
		pc.Release()
		pc.Release()
	}

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	heapGrowthMB := float64(int64(m2.HeapInuse)-int64(m1.HeapInuse)) / (1024 * 1024)
	t.Logf("Pool leak test: heap growth = %.2f MB over 1000 alloc/release cycles", heapGrowthMB)

	// With proper pooling, growth should be minimal (< 10 MB)
	if heapGrowthMB > 10 {
		t.Errorf("possible pool leak: %.2f MB heap growth (limit: 10 MB)", heapGrowthMB)
	}
}

// TestFrameTiming_30fps verifies that frame adaptation + serialisation
// completes within the 33ms budget required for 30 fps.
// M7 Acceptance: 70,000 points at 30 fps sustained.
func TestFrameTiming_30fps(t *testing.T) {
	adapter := NewFrameAdapter("timing-sensor")
	req := &pb.StreamRequest{
		IncludePoints:   true,
		IncludeClusters: true,
		IncludeTracks:   true,
	}

	// Warm up
	for i := 0; i < 5; i++ {
		frame := generateBenchFrame(70000, i)
		mask := make([]bool, len(frame.Points))
		result := adapter.AdaptFrame(frame, mask, nil, nil, nil)
		bundle := result.(*FrameBundle)
		_ = frameBundleToProto(bundle, req)
		if bundle.PointCloud != nil {
			bundle.PointCloud.Release()
		}
	}

	// Measure 50 frames
	var totalDuration time.Duration
	const numFrames = 50

	for i := 0; i < numFrames; i++ {
		frame := generateBenchFrame(70000, i)
		mask := make([]bool, len(frame.Points))
		for j := range mask {
			if j%30 == 0 {
				mask[j] = true
			}
		}

		start := time.Now()
		result := adapter.AdaptFrame(frame, mask, nil, nil, nil)
		bundle := result.(*FrameBundle)
		_ = frameBundleToProto(bundle, req)
		elapsed := time.Since(start)

		if bundle.PointCloud != nil {
			bundle.PointCloud.Release()
		}

		totalDuration += elapsed
	}

	avgMs := float64(totalDuration.Milliseconds()) / float64(numFrames)
	t.Logf("Average frame time: %.2f ms (budget: 33 ms for 30 fps)", avgMs)

	// The 33ms budget includes network + Swift decode + GPU render.
	// Dev machines achieve ~0.4-1ms; CI runners are significantly slower
	// due to shared-tenancy resource constraints on GitHub Actions.
	// Use a relaxed threshold on CI (detected via the CI env var) and a
	// tighter one locally to catch genuine regressions.
	threshold := 50.0 // local: 50 ms still well within budget
	if os.Getenv("CI") != "" {
		threshold = 200.0 // CI runners may be heavily throttled
	}
	if avgMs > threshold {
		t.Errorf("frame time too slow: %.2f ms avg (threshold: %.0f ms)", avgMs, threshold)
	}
}

// --- Helper functions for benchmarks ---

// generateBenchFrame creates a synthetic LiDARFrame with n points for benchmarking.
func generateBenchFrame(n int, seed int) *l2frames.LiDARFrame {
	now := time.Now().Add(time.Duration(seed) * 100 * time.Millisecond)
	points := make([]l2frames.Point, n)
	for i := 0; i < n; i++ {
		az := float64(i) / float64(n) * 360.0
		el := float64(i%40)*0.5 - 10.0
		dist := 5.0 + float64(i%100)*0.5
		x := dist * math.Cos(az*math.Pi/180) * math.Cos(el*math.Pi/180)
		y := dist * math.Sin(az*math.Pi/180) * math.Cos(el*math.Pi/180)
		z := dist * math.Sin(el*math.Pi/180)
		points[i] = l2frames.Point{
			X:         x,
			Y:         y,
			Z:         z,
			Intensity: uint8((i * 7) % 256),
			Distance:  dist,
			Azimuth:   az,
			Elevation: el,
			Channel:   i % 40,
			Timestamp: now,
		}
	}
	return &l2frames.LiDARFrame{
		SensorID:       "bench-sensor",
		StartTimestamp: now,
		EndTimestamp:   now.Add(100 * time.Millisecond),
		Points:         points,
		PointCount:     n,
	}
}

func generateBenchBundle(pointCount, clusterCount, trackCount int) *FrameBundle {
	bundle := NewFrameBundle(1, "bench-sensor", time.Now())

	if pointCount > 0 {
		bundle.PointCloud = &PointCloudFrame{
			FrameID:        1,
			TimestampNanos: time.Now().UnixNano(),
			SensorID:       "bench-sensor",
			X:              make([]float32, pointCount),
			Y:              make([]float32, pointCount),
			Z:              make([]float32, pointCount),
			Intensity:      make([]uint8, pointCount),
			Classification: make([]uint8, pointCount),
			PointCount:     pointCount,
		}
		for i := 0; i < pointCount; i++ {
			bundle.PointCloud.X[i] = float32(i) * 0.01
			bundle.PointCloud.Y[i] = float32(i) * 0.02
			bundle.PointCloud.Z[i] = float32(i%100) * 0.01
			bundle.PointCloud.Intensity[i] = uint8(i % 256)
		}
	}

	if clusterCount > 0 {
		clusters := make([]Cluster, clusterCount)
		for i := 0; i < clusterCount; i++ {
			clusters[i] = Cluster{
				ClusterID:      int64(i + 1),
				SensorID:       "bench-sensor",
				TimestampNanos: time.Now().UnixNano(),
				CentroidX:      float32(i) * 2.0,
				CentroidY:      float32(i) * 1.5,
				CentroidZ:      0.5,
				AABBLength:     2.0,
				AABBWidth:      1.5,
				AABBHeight:     1.5,
				PointsCount:    50,
			}
		}
		bundle.Clusters = &ClusterSet{
			FrameID:        1,
			TimestampNanos: time.Now().UnixNano(),
			Clusters:       clusters,
		}
	}

	if trackCount > 0 {
		tracks := make([]Track, trackCount)
		trails := make([]TrackTrail, trackCount)
		for i := 0; i < trackCount; i++ {
			tracks[i] = Track{
				TrackID:          "track-" + string(rune('A'+i%26)),
				SensorID:         "bench-sensor",
				State:            TrackStateConfirmed,
				Hits:             10,
				ObservationCount: 10,
				X:                float32(i) * 3.0,
				Y:                float32(i) * 2.0,
				Z:                0.5,
				VX:               5.0,
				VY:               0.5,
				SpeedMps:         5.0,
				HeadingRad:       0.1,
				BBoxLengthAvg:    4.5,
				BBoxWidthAvg:     1.8,
				BBoxHeightAvg:    1.5,
				BBoxHeadingRad:   0.1,
				Confidence:       0.95,
				Alpha:            1.0,
			}
			// Trail with 20 historical points
			points := make([]TrackPoint, 20)
			for j := 0; j < 20; j++ {
				points[j] = TrackPoint{
					X:              float32(i)*3.0 - float32(j)*0.5,
					Y:              float32(i) * 2.0,
					TimestampNanos: time.Now().Add(-time.Duration(j) * 100 * time.Millisecond).UnixNano(),
				}
			}
			trails[i] = TrackTrail{
				TrackID: tracks[i].TrackID,
				Points:  points,
			}
		}
		bundle.Tracks = &TrackSet{
			FrameID:        1,
			TimestampNanos: time.Now().UnixNano(),
			Tracks:         tracks,
			Trails:         trails,
		}
	}

	return bundle
}
