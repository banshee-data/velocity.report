package lidar

import (
	"testing"
	"time"
)

func TestDBSCANClusterer_NewDefaultDBSCANClusterer(t *testing.T) {
	clusterer := NewDefaultDBSCANClusterer()
	if clusterer == nil {
		t.Fatal("expected non-nil clusterer")
	}

	params := clusterer.GetParams()
	if params.Eps != DefaultDBSCANEps {
		t.Errorf("expected Eps=%f, got %f", DefaultDBSCANEps, params.Eps)
	}
	if params.MinPts != DefaultDBSCANMinPts {
		t.Errorf("expected MinPts=%d, got %d", DefaultDBSCANMinPts, params.MinPts)
	}
}

func TestDBSCANClusterer_NewDBSCANClusterer(t *testing.T) {
	eps := 0.8
	minPts := 15
	clusterer := NewDBSCANClusterer(eps, minPts)
	if clusterer == nil {
		t.Fatal("expected non-nil clusterer")
	}

	params := clusterer.GetParams()
	if params.Eps != eps {
		t.Errorf("expected Eps=%f, got %f", eps, params.Eps)
	}
	if params.MinPts != minPts {
		t.Errorf("expected MinPts=%d, got %d", minPts, params.MinPts)
	}
}

func TestDBSCANClusterer_SetParams(t *testing.T) {
	clusterer := NewDefaultDBSCANClusterer()

	newParams := ClusteringParams{
		Eps:    1.0,
		MinPts: 20,
	}
	clusterer.SetParams(newParams)

	gotParams := clusterer.GetParams()
	if gotParams.Eps != newParams.Eps {
		t.Errorf("expected Eps=%f, got %f", newParams.Eps, gotParams.Eps)
	}
	if gotParams.MinPts != newParams.MinPts {
		t.Errorf("expected MinPts=%d, got %d", newParams.MinPts, gotParams.MinPts)
	}
}

func TestDBSCANClusterer_Cluster_EmptyInput(t *testing.T) {
	clusterer := NewDefaultDBSCANClusterer()
	clusters := clusterer.Cluster(nil, "test-sensor", time.Now())
	if clusters != nil {
		t.Errorf("expected nil for empty input, got %d clusters", len(clusters))
	}
}

func TestDBSCANClusterer_Cluster_Determinism(t *testing.T) {
	// Create a set of test points
	points := []WorldPoint{
		{X: 5.0, Y: 5.0, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 5.1, Y: 5.1, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 5.2, Y: 5.0, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 5.0, Y: 5.2, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 5.1, Y: 5.2, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 5.2, Y: 5.2, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 5.0, Y: 5.1, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 5.2, Y: 5.1, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 5.1, Y: 5.0, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 5.05, Y: 5.05, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 5.15, Y: 5.15, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 5.25, Y: 5.05, Z: 0.5, Intensity: 100, SensorID: "test"},
		// Second cluster
		{X: 10.0, Y: 10.0, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 10.1, Y: 10.1, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 10.2, Y: 10.0, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 10.0, Y: 10.2, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 10.1, Y: 10.2, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 10.2, Y: 10.2, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 10.0, Y: 10.1, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 10.2, Y: 10.1, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 10.1, Y: 10.0, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 10.05, Y: 10.05, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 10.15, Y: 10.15, Z: 0.5, Intensity: 100, SensorID: "test"},
		{X: 10.25, Y: 10.05, Z: 0.5, Intensity: 100, SensorID: "test"},
	}

	clusterer := NewDefaultDBSCANClusterer()
	timestamp := time.Now()

	// Run clustering multiple times
	run1 := clusterer.Cluster(points, "test-sensor", timestamp)
	run2 := clusterer.Cluster(points, "test-sensor", timestamp)
	run3 := clusterer.Cluster(points, "test-sensor", timestamp)

	// Verify we get clusters
	if len(run1) == 0 {
		t.Fatal("expected at least one cluster")
	}

	// Verify results are identical across runs
	if len(run1) != len(run2) || len(run1) != len(run3) {
		t.Errorf("inconsistent cluster counts: %d, %d, %d", len(run1), len(run2), len(run3))
	}

	// Verify cluster order is identical (deterministic sorting)
	for i := range run1 {
		if run1[i].CentroidX != run2[i].CentroidX || run1[i].CentroidY != run2[i].CentroidY {
			t.Errorf("cluster %d mismatch between run1 and run2", i)
		}
		if run1[i].CentroidX != run3[i].CentroidX || run1[i].CentroidY != run3[i].CentroidY {
			t.Errorf("cluster %d mismatch between run1 and run3", i)
		}
	}

	// Verify clusters are sorted by X, then Y
	for i := 1; i < len(run1); i++ {
		prev := run1[i-1]
		curr := run1[i]
		if prev.CentroidX > curr.CentroidX {
			t.Errorf("clusters not sorted by X: cluster %d (X=%f) > cluster %d (X=%f)",
				i-1, prev.CentroidX, i, curr.CentroidX)
		}
		if prev.CentroidX == curr.CentroidX && prev.CentroidY > curr.CentroidY {
			t.Errorf("clusters with same X not sorted by Y: cluster %d (Y=%f) > cluster %d (Y=%f)",
				i-1, prev.CentroidY, i, curr.CentroidY)
		}
	}
}

func TestDBSCANClusterer_Cluster_SingleCluster(t *testing.T) {
	// Create a tight cluster of points
	points := []WorldPoint{
		{X: 5.0, Y: 5.0, Z: 0.5, Intensity: 100},
		{X: 5.1, Y: 5.0, Z: 0.5, Intensity: 100},
		{X: 5.0, Y: 5.1, Z: 0.5, Intensity: 100},
		{X: 5.1, Y: 5.1, Z: 0.5, Intensity: 100},
		{X: 5.2, Y: 5.0, Z: 0.5, Intensity: 100},
		{X: 5.0, Y: 5.2, Z: 0.5, Intensity: 100},
		{X: 5.2, Y: 5.2, Z: 0.5, Intensity: 100},
		{X: 5.1, Y: 5.2, Z: 0.5, Intensity: 100},
		{X: 5.2, Y: 5.1, Z: 0.5, Intensity: 100},
		{X: 5.05, Y: 5.05, Z: 0.5, Intensity: 100},
		{X: 5.15, Y: 5.15, Z: 0.5, Intensity: 100},
		{X: 5.25, Y: 5.25, Z: 0.5, Intensity: 100},
	}

	clusterer := NewDefaultDBSCANClusterer()
	clusters := clusterer.Cluster(points, "test-sensor", time.Now())

	if len(clusters) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(clusters))
	}

	if len(clusters) > 0 {
		cluster := clusters[0]
		if cluster.PointsCount != len(points) {
			t.Errorf("expected %d points in cluster, got %d", len(points), cluster.PointsCount)
		}
	}
}

func TestDBSCANClusterer_Interface(t *testing.T) {
	// Compile-time check that DBSCANClusterer implements ClustererInterface
	var _ ClustererInterface = (*DBSCANClusterer)(nil)
}
