package l4perception

import (
	"reflect"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
)

func TestRetainedClusterEvidenceBoundedDeterministicAndOwned(t *testing.T) {
	points := make([]WorldPoint, 30)
	labels := make([]int, len(points))
	for i := range points {
		points[i] = WorldPoint{X: 10 + float64(i%5)*0.1, Y: float64(i/5) * 0.1, Z: 1,
			Timestamp: time.Unix(100, int64(i)), SensorID: "s", Intensity: uint8(i)}
		labels[i] = 1
	}
	cfg := config.MustLoadDefaultConfig()
	cfg.L4.DbscanXyV1.MaxSamplePoints = 7
	params := DBSCANParamsFromTuning(cfg.L4.DbscanXyV1)
	a := buildClusters(points, labels, 1, params)
	b := buildClusters(points, labels, 1, params)
	if len(a) != 1 || !reflect.DeepEqual(a, b) || len(a[0].RetainedPoints) != 7 {
		t.Fatal("retention not bounded/reproducible")
	}
	for i, p := range a[0].RetainedPoints {
		if p.Timestamp.IsZero() || p.SensorID != "s" || a[0].SamplePoints[i] != [3]float32{float32(p.X), float32(p.Y), float32(p.Z)} {
			t.Fatal("retained point lost metadata or display coordinates")
		}
	}
	params.MaxSamplePoints = 0
	off := buildClusters(points, labels, 1, params)[0]
	a[0].RetainedPoints = nil
	a[0].SamplePoints = nil
	if !reflect.DeepEqual(a[0], off) {
		t.Fatal("retention changed raw geometry")
	}
	params.MaxSamplePoints = 1025 // Defensive clamp for internal callers bypassing config validation.
	all := buildClusters(points, labels, 1, params)[0]
	points[0].X = 999
	if all.RetainedPoints[0].X == 999 || len(all.RetainedPoints) != 30 {
		t.Fatal("uncapped evidence aliases input")
	}
	if b[0].RetainedPoints[0].X == 999 {
		t.Fatal("sample aliases input")
	}
}
