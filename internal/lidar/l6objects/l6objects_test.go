package l6objects_test

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
)

// TestAliasesCompile verifies that type aliases resolve correctly.
func TestAliasesCompile(t *testing.T) {
	// TrackClassifier alias.
	tc := l6objects.NewTrackClassifier()
	if tc == nil {
		t.Fatal("NewTrackClassifier returned nil")
	}

	// TrackTrainingFilter alias.
	f := l6objects.DefaultTrackTrainingFilter()
	if f == nil {
		t.Fatal("DefaultTrackTrainingFilter returned nil")
	}

	// ComputeSpeedPercentiles alias.
	p50, p85, p95 := l6objects.ComputeSpeedPercentiles([]float32{1, 2, 3, 4, 5})
	if p50 <= 0 || p85 <= 0 || p95 <= 0 {
		t.Fatalf("ComputeSpeedPercentiles returned invalid: p50=%f p85=%f p95=%f", p50, p85, p95)
	}
}
