//go:build pcap
// +build pcap

package main

import (
	"math"
	"testing"
)

func TestComputeClassStats_SingleClass(t *testing.T) {
	tracks := []*TrackExport{
		{Class: "vehicle", AvgSpeedMps: 10.0, DurationSecs: 5.0, Observations: 50},
		{Class: "vehicle", AvgSpeedMps: 20.0, DurationSecs: 10.0, Observations: 100},
	}

	stats := computeClassStats(tracks)

	if len(stats) != 1 {
		t.Fatalf("expected 1 class, got %d", len(stats))
	}
	v, ok := stats["vehicle"]
	if !ok {
		t.Fatal("expected 'vehicle' class in stats")
	}
	if v.Count != 2 {
		t.Errorf("expected count=2, got %d", v.Count)
	}
	// AvgSpeed should be mean of per-track AvgSpeedMps: (10+20)/2 = 15
	if math.Abs(float64(v.AvgSpeed)-15.0) > 0.01 {
		t.Errorf("expected AvgSpeed=15.0, got %f", v.AvgSpeed)
	}
	// AvgDuration: (5+10)/2 = 7.5
	if math.Abs(float64(v.AvgDuration)-7.5) > 0.01 {
		t.Errorf("expected AvgDuration=7.5, got %f", v.AvgDuration)
	}
	// AvgObservations: (50+100)/2 = 75
	if math.Abs(float64(v.AvgObservations)-75.0) > 0.01 {
		t.Errorf("expected AvgObservations=75.0, got %f", v.AvgObservations)
	}
}

func TestComputeClassStats_MultipleClasses(t *testing.T) {
	tracks := []*TrackExport{
		{Class: "vehicle", AvgSpeedMps: 12.0, DurationSecs: 4.0, Observations: 40},
		{Class: "pedestrian", AvgSpeedMps: 1.5, DurationSecs: 20.0, Observations: 200},
		{Class: "vehicle", AvgSpeedMps: 8.0, DurationSecs: 6.0, Observations: 60},
	}

	stats := computeClassStats(tracks)

	if len(stats) != 2 {
		t.Fatalf("expected 2 classes, got %d", len(stats))
	}

	v := stats["vehicle"]
	if v.Count != 2 {
		t.Errorf("vehicle: expected count=2, got %d", v.Count)
	}
	// (12+8)/2 = 10
	if math.Abs(float64(v.AvgSpeed)-10.0) > 0.01 {
		t.Errorf("vehicle: expected AvgSpeed=10.0, got %f", v.AvgSpeed)
	}

	p := stats["pedestrian"]
	if p.Count != 1 {
		t.Errorf("pedestrian: expected count=1, got %d", p.Count)
	}
	if math.Abs(float64(p.AvgSpeed)-1.5) > 0.01 {
		t.Errorf("pedestrian: expected AvgSpeed=1.5, got %f", p.AvgSpeed)
	}
}

func TestComputeClassStats_Empty(t *testing.T) {
	stats := computeClassStats(nil)
	if len(stats) != 0 {
		t.Errorf("expected empty stats for nil input, got %d entries", len(stats))
	}
}

func TestComputeSpeedStats_BasicValues(t *testing.T) {
	// 10 samples: 1..10
	samples := make([]float32, 10)
	for i := range samples {
		samples[i] = float32(i + 1)
	}

	stats := computeSpeedStats(samples)

	if stats.MinSpeed != 1.0 {
		t.Errorf("expected MinSpeed=1.0, got %f", stats.MinSpeed)
	}
	if stats.MaxSpeed != 10.0 {
		t.Errorf("expected MaxSpeed=10.0, got %f", stats.MaxSpeed)
	}
	// Avg: (1+2+...+10)/10 = 5.5
	if math.Abs(float64(stats.AvgSpeed)-5.5) > 0.01 {
		t.Errorf("expected AvgSpeed=5.5, got %f", stats.AvgSpeed)
	}
	// P50 uses floor-index: sorted[10/2] = sorted[5] = 6
	if stats.P50Speed != 6.0 {
		t.Errorf("expected P50Speed=6.0, got %f", stats.P50Speed)
	}
	// P85: floor(10*0.85) = floor(8.5) = 8 => sorted[8] = 9
	if stats.P85Speed != 9.0 {
		t.Errorf("expected P85Speed=9.0, got %f", stats.P85Speed)
	}
	// P95: floor(10*0.95) = floor(9.5) = 9 => sorted[9] = 10
	if stats.P95Speed != 10.0 {
		t.Errorf("expected P95Speed=10.0, got %f", stats.P95Speed)
	}
}

func TestComputeSpeedStats_Empty(t *testing.T) {
	stats := computeSpeedStats(nil)
	if stats.MinSpeed != 0 || stats.MaxSpeed != 0 || stats.AvgSpeed != 0 {
		t.Errorf("expected all zeros for nil input, got %+v", stats)
	}
}

func TestComputeSpeedStats_SingleElement(t *testing.T) {
	stats := computeSpeedStats([]float32{7.5})
	if stats.MinSpeed != 7.5 {
		t.Errorf("expected MinSpeed=7.5, got %f", stats.MinSpeed)
	}
	if stats.MaxSpeed != 7.5 {
		t.Errorf("expected MaxSpeed=7.5, got %f", stats.MaxSpeed)
	}
	if stats.AvgSpeed != 7.5 {
		t.Errorf("expected AvgSpeed=7.5, got %f", stats.AvgSpeed)
	}
	if stats.P50Speed != 7.5 {
		t.Errorf("expected P50Speed=7.5, got %f", stats.P50Speed)
	}
}

func TestComputeSpeedStats_AvgDiffersFromP50(t *testing.T) {
	// Skewed distribution: avg differs from median
	samples := []float32{1.0, 2.0, 3.0, 4.0, 100.0}

	stats := computeSpeedStats(samples)

	// Avg: (1+2+3+4+100)/5 = 22.0
	if math.Abs(float64(stats.AvgSpeed)-22.0) > 0.01 {
		t.Errorf("expected AvgSpeed=22.0, got %f", stats.AvgSpeed)
	}
	// P50: sorted[5/2] = sorted[2] = 3.0
	if stats.P50Speed != 3.0 {
		t.Errorf("expected P50Speed=3.0, got %f", stats.P50Speed)
	}
}

func TestComputeClassStats_UsesAvg(t *testing.T) {
	// Verify that ClassStats.AvgSpeed uses AvgSpeedMps (the running mean).
	tracks := []*TrackExport{
		{Class: "vehicle", AvgSpeedMps: 10.0},
		{Class: "vehicle", AvgSpeedMps: 20.0},
	}

	stats := computeClassStats(tracks)
	v := stats["vehicle"]

	// Should be mean of AvgSpeedMps: (10+20)/2 = 15
	if math.Abs(float64(v.AvgSpeed)-15.0) > 0.01 {
		t.Errorf("expected AvgSpeed=15.0 (from AvgSpeedMps), got %f", v.AvgSpeed)
	}
}
