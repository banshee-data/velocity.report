package pipeline

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// A parameter set's cluster size filters reach L4; unset, the defaults stand.
func TestClusterParams_TuningReachesTheFilters(t *testing.T) {
	want := l4perception.DefaultDBSCANParams()
	if got := (&TrackingPipelineConfig{}).clusterParams(); got.MaxClusterDiameter != want.MaxClusterDiameter ||
		got.MinClusterDiameter != want.MinClusterDiameter || got.MaxClusterAspectRatio != want.MaxClusterAspectRatio {
		t.Fatalf("unset: %+v, want the defaults %+v", got, want)
	}
	if !want.MeanCentre {
		t.Fatal("the defaults' clusters report the medoid; DBSCANParamsFromTuning lost cluster_position")
	}
	cfg := &TrackingPipelineConfig{MaxClusterDiameter: 20, MinClusterDiameter: 0.1, MaxClusterAspectRatio: 60, ClusterCentre: "mean"}
	got := cfg.clusterParams()
	if got.MaxClusterDiameter != 20 || got.MinClusterDiameter != 0.1 || got.MaxClusterAspectRatio != 60 || !got.MeanCentre {
		t.Fatalf("set: %+v", got)
	}
}
