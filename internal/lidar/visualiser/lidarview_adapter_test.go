package visualiser

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// mockForwarder implements ForegroundForwarder for testing.
type mockForwarder struct {
	forwardedPoints []lidar.PointPolar
	callCount       int
}

func (m *mockForwarder) ForwardForeground(points []lidar.PointPolar) {
	m.forwardedPoints = append(m.forwardedPoints, points...)
	m.callCount++
}

func TestNewLidarViewAdapter(t *testing.T) {
	forwarder := &mockForwarder{}
	adapter := NewLidarViewAdapter(forwarder)

	if adapter == nil {
		t.Fatal("NewLidarViewAdapter returned nil")
	}
	if adapter.forwarder != forwarder {
		t.Error("Adapter forwarder not set correctly")
	}
}

func TestPublishFrameBundle_NilForwarder(t *testing.T) {
	adapter := NewLidarViewAdapter(nil)
	bundle := NewFrameBundle(1, "test-sensor", time.Now())
	points := []lidar.PointPolar{
		{Azimuth: 0, Distance: 10, Intensity: 100},
	}

	// Should not panic
	adapter.PublishFrameBundle(bundle, points)
}

func TestPublishFrameBundle_NilBundle(t *testing.T) {
	forwarder := &mockForwarder{}
	adapter := NewLidarViewAdapter(forwarder)
	points := []lidar.PointPolar{
		{Azimuth: 0, Distance: 10, Intensity: 100},
	}

	// With nil bundle but valid points, it should still forward (LidarView-only mode)
	adapter.PublishFrameBundle(nil, points)

	if forwarder.callCount != 1 {
		t.Errorf("Forwarder should be called even with nil bundle, got %d calls", forwarder.callCount)
	}
	if len(forwarder.forwardedPoints) != 1 {
		t.Errorf("Expected 1 forwarded point, got %d", len(forwarder.forwardedPoints))
	}
}

func TestPublishFrameBundle_EmptyPoints(t *testing.T) {
	forwarder := &mockForwarder{}
	adapter := NewLidarViewAdapter(forwarder)
	bundle := NewFrameBundle(1, "test-sensor", time.Now())

	adapter.PublishFrameBundle(bundle, []lidar.PointPolar{})

	if forwarder.callCount != 0 {
		t.Error("Forwarder should not be called with empty points")
	}
}

func TestPublishFrameBundle_ValidPoints(t *testing.T) {
	forwarder := &mockForwarder{}
	adapter := NewLidarViewAdapter(forwarder)
	bundle := NewFrameBundle(1, "test-sensor", time.Now())

	points := []lidar.PointPolar{
		{Azimuth: 0, Distance: 10, Intensity: 100, Channel: 1},
		{Azimuth: 1, Distance: 15, Intensity: 150, Channel: 2},
		{Azimuth: 2, Distance: 20, Intensity: 200, Channel: 3},
	}

	adapter.PublishFrameBundle(bundle, points)

	if forwarder.callCount != 1 {
		t.Errorf("Expected 1 forwarder call, got %d", forwarder.callCount)
	}

	if len(forwarder.forwardedPoints) != 3 {
		t.Errorf("Expected 3 forwarded points, got %d", len(forwarder.forwardedPoints))
	}

	// Verify points are preserved correctly
	for i, pt := range forwarder.forwardedPoints {
		if pt.Azimuth != points[i].Azimuth {
			t.Errorf("Point %d: azimuth mismatch: got %.2f, want %.2f",
				i, pt.Azimuth, points[i].Azimuth)
		}
		if pt.Distance != points[i].Distance {
			t.Errorf("Point %d: distance mismatch: got %.2f, want %.2f",
				i, pt.Distance, points[i].Distance)
		}
		if pt.Intensity != points[i].Intensity {
			t.Errorf("Point %d: intensity mismatch: got %d, want %d",
				i, pt.Intensity, points[i].Intensity)
		}
	}
}

func TestPublishFrameBundle_MultipleCallsAccumulate(t *testing.T) {
	forwarder := &mockForwarder{}
	adapter := NewLidarViewAdapter(forwarder)

	bundle1 := NewFrameBundle(1, "test-sensor", time.Now())
	points1 := []lidar.PointPolar{
		{Azimuth: 0, Distance: 10, Intensity: 100},
	}

	bundle2 := NewFrameBundle(2, "test-sensor", time.Now())
	points2 := []lidar.PointPolar{
		{Azimuth: 1, Distance: 15, Intensity: 150},
		{Azimuth: 2, Distance: 20, Intensity: 200},
	}

	adapter.PublishFrameBundle(bundle1, points1)
	adapter.PublishFrameBundle(bundle2, points2)

	if forwarder.callCount != 2 {
		t.Errorf("Expected 2 forwarder calls, got %d", forwarder.callCount)
	}

	if len(forwarder.forwardedPoints) != 3 {
		t.Errorf("Expected 3 total forwarded points, got %d", len(forwarder.forwardedPoints))
	}
}
