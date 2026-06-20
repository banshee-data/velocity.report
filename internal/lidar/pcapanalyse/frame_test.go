//go:build pcap
// +build pcap

package pcapanalyse

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
)

// tightBuilder builds an analysisFrameBuilder with a small noise fraction so a
// close cluster against a far background is unambiguously foreground — letting
// the test drive the full L3→L6 path inside processCurrentFrame deterministically
// without depending on the production tuning.
func tightBuilder(r *AnalysisResult) *analysisFrameBuilder {
	params := l3grid.BackgroundParams{
		BackgroundUpdateFraction:       0.5,
		ClosenessSensitivityMultiplier: 2.0,
		SafetyMarginMetres:             0.3,
		NeighbourConfirmationCount:     2,
		NoiseRelativeFraction:          0.02,
		SeedFromFirstObservation:       true,
		FreezeDurationNanos:            int64(time.Second),
		ForegroundMinClusterPoints:     10,
		ForegroundDBSCANEps:            1.0,
	}
	return &analysisFrameBuilder{
		points:        make([]l2frames.PointPolar, 0, 8000),
		bgManager:     l3grid.NewBackgroundManager("s", 40, 1800, params, nil),
		tracker:       l5tracks.NewTracker(l5tracks.DefaultTrackerConfig()),
		classifier:    l6objects.NewTrackClassifier(),
		config:        Config{SensorID: "s", ExportTraining: true, Benchmark: true},
		result:        r,
		benchmarkMode: true,
	}
}

func bgFrame(dist float64, ts int64) []l2frames.PointPolar {
	pts := make([]l2frames.PointPolar, 0, 40*180)
	for ch := range 40 {
		for az := 0; az < 360; az += 2 {
			pts = append(pts, l2frames.PointPolar{
				Channel: ch, Azimuth: float64(az), Elevation: float64(ch) - 20,
				Distance: dist, Timestamp: ts,
			})
		}
	}
	return pts
}

// fgFrame is a 20 m background plus a tight 8 m cluster (a 12 m deviation, well
// past the ~0.5 m closeness threshold), lifted off the ground plane.
func fgFrame(ts int64) []l2frames.PointPolar {
	pts := bgFrame(20, ts)
	for i := range 200 {
		pts = append(pts, l2frames.PointPolar{
			Channel: 20, Azimuth: 90.0 + float64(i%10)*0.1, Elevation: 12 + float64(i/10)*0.2,
			Distance: 8.0, Timestamp: ts,
		})
	}
	return pts
}

func feedFrame(fb *analysisFrameBuilder, pts []l2frames.PointPolar, ts int64) {
	fb.mu.Lock()
	fb.points = pts
	fb.frameStartTime = time.Unix(0, ts)
	fb.processCurrentFrame()
	fb.frameCount++
	fb.mu.Unlock()
}

func TestProcessCurrentFrame_ForegroundPipeline(t *testing.T) {
	r := &AnalysisResult{TracksByClass: map[string]int{}}
	fb := tightBuilder(r)

	base := time.Now().UnixNano()
	step := int64(100_000_000)
	for i := range 8 { // settle background at 20 m
		feedFrame(fb, bgFrame(20, base+int64(i)*step), base+int64(i)*step)
	}
	for i := 8; i < 22; i++ { // sustained 8 m cluster -> foreground, clusters, track
		feedFrame(fb, fgFrame(base+int64(i)*step), base+int64(i)*step)
	}

	if r.ForegroundPoints == 0 {
		t.Errorf("expected foreground points from the synthetic 8 m cluster")
	}
	t.Logf("foreground=%d clusters=%d tracks=%d trainingFrames=%d",
		r.ForegroundPoints, r.TotalClusters, len(fb.tracker.GetAllTracks()), len(fb.trainingFrames))
}

func TestAddPointsPolar_FrameWrapAndFinalise(t *testing.T) {
	r := &AnalysisResult{TracksByClass: map[string]int{}}
	fb := newAnalysisFrameBuilder(Config{SensorID: "s"}, r)
	ts := time.Now().UnixNano()
	fb.AddPointsPolar(nil) // empty -> early return
	var sweep []l2frames.PointPolar
	for az := 0; az < 360; az += 4 {
		sweep = append(sweep, l2frames.PointPolar{Channel: az % 40, Azimuth: float64(az), Distance: 5, Timestamp: ts})
	}
	fb.AddPointsPolar(sweep)
	fb.AddPointsPolar([]l2frames.PointPolar{{Channel: 0, Azimuth: 5, Distance: 5, Timestamp: ts}})
	fb.finalise()
	if fb.frameCount == 0 {
		t.Error("expected at least one completed frame")
	}
}
