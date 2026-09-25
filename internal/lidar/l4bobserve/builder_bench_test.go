package l4bobserve

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// BenchmarkFrameDraftKirk0Scale isolates the tap's own per-frame work at the
// scale measured on kirk0: about 69,000 L2 returns and 1,150 foreground
// returns in 180 packets. L3, the transform, filters and DBSCAN run once in
// setup; each iteration is only what enabling the tap adds to a frame.
func BenchmarkFrameDraftKirk0Scale(b *testing.B) {
	const returns, foregroundEvery = 69_000, 60
	start := time.Unix(1_700_000_000, 0)
	frame := &l2frames.LiDARFrame{FrameID: "bench", StartTimestamp: start, SpinComplete: true,
		PolarPoints: make([]l2frames.PointPolar, returns), ReceivedPackets: map[uint32]bool{}}
	mask := make([]bool, returns)
	var foreground []l2frames.PointPolar
	for i := range frame.PolarPoints {
		packet := uint32(1000 + i/384)
		frame.ReceivedPackets[packet] = true
		p := l2frames.PointPolar{Channel: 1 + i%40, Azimuth: float64(i%1800) * 0.2, Distance: 30,
			Intensity: uint8(i), Timestamp: start.UnixNano() + int64(i)*1400, BlockID: i % 10, UDPSequence: packet}
		if i%foregroundEvery == 0 {
			// Foreground returns fall in a few dense arcs at 10 m.
			mask[i] = true
			p.Azimuth, p.Distance = float64((i/foregroundEvery)%100)*0.05+float64((i/foregroundEvery)/100)*30, 10
		}
		frame.PolarPoints[i] = p
		if mask[i] {
			foreground = append(foreground, p)
		}
	}
	frame.EndTimestamp = time.Unix(0, frame.PolarPoints[returns-1].Timestamp)
	builder, err := NewExtractionBuilder(testExtraction())
	if err != nil {
		b.Fatal(err)
	}
	world := l4perception.TransformToWorld(foreground, nil, "lidar")
	// One pass stamps the lineage that the precomputed stage outputs carry.
	warm := builder.BeginFrame(frame)
	warm.SetForeground(mask, len(foreground))
	warm.Retain(world)
	kept := l4perception.DefaultHeightBandFilter().FilterVertical(append([]l4perception.WorldPoint(nil), world...))
	clusters, trace := l4perception.DBSCANWithTrace(kept, l4perception.DefaultDBSCANParams())
	warm.HeightBandFiltered(kept)
	warm.Clustered(trace, clusters)
	if record, err := warm.Finish(); err != nil || len(record.Clusters) == 0 {
		b.Fatalf("setup frame: %d clusters, %v", len(record.Clusters), err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := builder.BeginFrame(frame)
		d.SetBackground(BackgroundSettled)
		d.SetForeground(mask, len(foreground))
		d.Retain(world)
		d.HeightBandFiltered(kept)
		d.Clustered(trace, clusters)
		if _, err := d.Finish(); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(len(foreground)), "retained-pts/frame")
}
