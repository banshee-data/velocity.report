package monitor

import (
	"sync"
	"testing"
	"time"
)

func TestNewPacketStats(t *testing.T) {
	stats := NewPacketStats()

	if stats == nil {
		t.Fatal("NewPacketStats returned nil")
	}

	// Check that uptime is recent
	uptime := stats.GetUptime()
	if uptime > 100*time.Millisecond {
		t.Errorf("Uptime too large for new stats: %v", uptime)
	}
}

func TestPacketStats_AddPacket(t *testing.T) {
	stats := NewPacketStats()

	// Add a packet
	stats.AddPacket(1262) // Typical lidar packet size

	// Get stats and check values
	packets, bytes, dropped, points, duration := stats.GetAndReset()

	if packets != 1 {
		t.Errorf("Expected 1 packet, got %d", packets)
	}

	if bytes != 1262 {
		t.Errorf("Expected 1262 bytes, got %d", bytes)
	}

	if dropped != 0 {
		t.Errorf("Expected 0 dropped packets, got %d", dropped)
	}

	if points != 0 {
		t.Errorf("Expected 0 points, got %d", points)
	}

	if duration <= 0 {
		t.Errorf("Expected positive duration, got %v", duration)
	}
}

func TestPacketStats_AddDropped(t *testing.T) {
	stats := NewPacketStats()

	// Add dropped packets
	stats.AddDropped()
	stats.AddDropped()

	// Get stats and check values
	packets, _, dropped, _, _ := stats.GetAndReset()

	if dropped != 2 {
		t.Errorf("Expected 2 dropped packets, got %d", dropped)
	}

	if packets != 0 {
		t.Errorf("Expected 0 packets, got %d", packets)
	}
}

func TestPacketStats_AddPoints(t *testing.T) {
	stats := NewPacketStats()

	// Add points
	stats.AddPoints(400) // Typical points per packet
	stats.AddPoints(100)

	// Get stats and check values
	_, _, _, points, _ := stats.GetAndReset()

	if points != 500 {
		t.Errorf("Expected 500 points, got %d", points)
	}
}

func TestPacketStats_GetAndReset(t *testing.T) {
	stats := NewPacketStats()

	// Add some data
	stats.AddPacket(1262)
	stats.AddDropped()
	stats.AddPoints(400)

	// Get and reset
	packets1, bytes1, dropped1, points1, duration1 := stats.GetAndReset()

	if packets1 != 1 || bytes1 != 1262 || dropped1 != 1 || points1 != 400 {
		t.Errorf("First GetAndReset: expected (1, 1262, 1, 400), got (%d, %d, %d, %d)",
			packets1, bytes1, dropped1, points1)
	}

	if duration1 <= 0 {
		t.Errorf("Expected positive duration, got %v", duration1)
	}

	// Second call should return zeros
	packets2, bytes2, dropped2, points2, duration2 := stats.GetAndReset()

	if packets2 != 0 || bytes2 != 0 || dropped2 != 0 || points2 != 0 {
		t.Errorf("Second GetAndReset: expected all zeros, got (%d, %d, %d, %d)",
			packets2, bytes2, dropped2, points2)
	}

	if duration2 <= 0 {
		t.Errorf("Expected positive duration even after reset, got %v", duration2)
	}
}

func TestPacketStats_LogStats(t *testing.T) {
	stats := NewPacketStats()

	// Add some data
	stats.AddPacket(1262)
	stats.AddPoints(400)

	// Log stats with parsing enabled
	stats.LogStats(true)

	// Check that snapshot was created
	snapshot := stats.GetLatestSnapshot()
	if snapshot == nil {
		t.Fatal("Expected snapshot after LogStats, got nil")
	}

	if !snapshot.ParseEnabled {
		t.Error("Expected ParseEnabled to be true")
	}

	if snapshot.PacketsPerSec <= 0 {
		t.Errorf("Expected positive packets per sec, got %f", snapshot.PacketsPerSec)
	}

	if snapshot.MBPerSec <= 0 {
		t.Errorf("Expected positive MB per sec, got %f", snapshot.MBPerSec)
	}

	if snapshot.PointsPerSec <= 0 {
		t.Errorf("Expected positive points per sec, got %f", snapshot.PointsPerSec)
	}
}

func TestPacketStats_GetLatestSnapshot(t *testing.T) {
	stats := NewPacketStats()

	// Initially should return nil
	snapshot := stats.GetLatestSnapshot()
	if snapshot != nil {
		t.Error("Expected nil snapshot initially, got non-nil")
	}

	// Add data and log stats
	stats.AddPacket(1262)
	stats.LogStats(false)

	// Now should have snapshot
	snapshot = stats.GetLatestSnapshot()
	if snapshot == nil {
		t.Fatal("Expected snapshot after LogStats, got nil")
	}

	if snapshot.ParseEnabled {
		t.Error("Expected ParseEnabled to be false")
	}
}

func TestPacketStats_ThreadSafety(t *testing.T) {
	stats := NewPacketStats()

	// Test concurrent access
	var wg sync.WaitGroup
	numGoroutines := 50
	incrementsPerGoroutine := 10

	// Start multiple goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				stats.AddPacket(100)
				stats.AddDropped()
				stats.AddPoints(10)

				// Also test reads during writes
				_ = stats.GetUptime()
				_ = stats.GetLatestSnapshot()
			}
		}()
	}

	wg.Wait()

	// Get final values
	packets, bytes, dropped, points, _ := stats.GetAndReset()

	expectedPackets := int64(numGoroutines * incrementsPerGoroutine)
	expectedBytes := int64(numGoroutines * incrementsPerGoroutine * 100)
	expectedDropped := int64(numGoroutines * incrementsPerGoroutine)
	expectedPoints := int64(numGoroutines * incrementsPerGoroutine * 10)

	if packets != expectedPackets {
		t.Errorf("Expected packets %d, got %d", expectedPackets, packets)
	}

	if bytes != expectedBytes {
		t.Errorf("Expected bytes %d, got %d", expectedBytes, bytes)
	}

	if dropped != expectedDropped {
		t.Errorf("Expected dropped %d, got %d", expectedDropped, dropped)
	}

	if points != expectedPoints {
		t.Errorf("Expected points %d, got %d", expectedPoints, points)
	}
}

func TestFormatWithCommas(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{123, "123"},
		{1234, "1,234"},
		{12345, "12,345"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
		{12345678, "12,345,678"},
	}

	for _, test := range tests {
		result := FormatWithCommas(test.input)
		if result != test.expected {
			t.Errorf("FormatWithCommas(%d): expected %s, got %s",
				test.input, test.expected, result)
		}
	}
}

func BenchmarkPacketStats_AddPacket(b *testing.B) {
	stats := NewPacketStats()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stats.AddPacket(1262)
		}
	})
}

func BenchmarkPacketStats_GetLatestSnapshot(b *testing.B) {
	stats := NewPacketStats()

	// Add some data first
	stats.AddPacket(1262)
	stats.LogStats(false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = stats.GetLatestSnapshot()
	}
}
