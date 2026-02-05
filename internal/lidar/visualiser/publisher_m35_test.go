package visualiser

import (
	"testing"
	"time"
)

// mockBackgroundManager implements BackgroundManagerInterface for testing
type mockBackgroundManager struct {
	snapshot       *BackgroundSnapshot
	sequenceNumber uint64
	generateError  error
}

func (m *mockBackgroundManager) GenerateBackgroundSnapshot() (interface{}, error) {
	if m.generateError != nil {
		return nil, m.generateError
	}
	return m.snapshot, nil
}

func (m *mockBackgroundManager) GetBackgroundSequenceNumber() uint64 {
	return m.sequenceNumber
}

func TestPublisher_ShouldSendBackground(t *testing.T) {
	tests := []struct {
		name               string
		backgroundMgr      BackgroundManagerInterface
		lastBackgroundSeq  uint64
		lastBackgroundSent time.Time
		backgroundInterval time.Duration
		currentSeq         uint64
		expectSend         bool
	}{
		{
			name:          "No background manager",
			backgroundMgr: nil,
			expectSend:    false,
		},
		{
			name: "Never sent before",
			backgroundMgr: &mockBackgroundManager{
				sequenceNumber: 1,
			},
			lastBackgroundSeq:  0,
			lastBackgroundSent: time.Time{},
			expectSend:         true, // First send
		},
		{
			name: "Sequence changed",
			backgroundMgr: &mockBackgroundManager{
				sequenceNumber: 2,
			},
			lastBackgroundSeq:  1,
			lastBackgroundSent: time.Now(),
			expectSend:         true,
		},
		{
			name: "Interval elapsed",
			backgroundMgr: &mockBackgroundManager{
				sequenceNumber: 1,
			},
			lastBackgroundSeq:  1,
			lastBackgroundSent: time.Now().Add(-31 * time.Second),
			backgroundInterval: 30 * time.Second,
			expectSend:         true,
		},
		{
			name: "No need to send",
			backgroundMgr: &mockBackgroundManager{
				sequenceNumber: 1,
			},
			lastBackgroundSeq:  1,
			lastBackgroundSent: time.Now().Add(-10 * time.Second),
			backgroundInterval: 30 * time.Second,
			expectSend:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.BackgroundInterval = tt.backgroundInterval
			if cfg.BackgroundInterval == 0 {
				cfg.BackgroundInterval = 30 * time.Second
			}

			p := NewPublisher(cfg)
			p.backgroundMgr = tt.backgroundMgr
			p.lastBackgroundSeq = tt.lastBackgroundSeq
			p.lastBackgroundSent = tt.lastBackgroundSent

			got := p.shouldSendBackground()
			if got != tt.expectSend {
				t.Errorf("shouldSendBackground() = %v, want %v", got, tt.expectSend)
			}
		})
	}
}

func TestPublisher_SendBackgroundSnapshot(t *testing.T) {
	cfg := DefaultConfig()
	p := NewPublisher(cfg)
	p.running.Store(true)

	// Test with no background manager
	err := p.sendBackgroundSnapshot()
	if err != nil {
		t.Errorf("Expected no error with nil background manager, got %v", err)
	}

	// Test with successful snapshot generation
	snapshot := &BackgroundSnapshot{
		SequenceNumber: 1,
		TimestampNanos: time.Now().UnixNano(),
		X:              []float32{1, 2, 3},
		Y:              []float32{1, 2, 3},
		Z:              []float32{1, 2, 3},
		Confidence:     []uint32{10, 20, 30},
		GridMetadata: GridMetadata{
			Rings:            40,
			AzimuthBins:      1800,
			SettlingComplete: true,
		},
	}

	p.backgroundMgr = &mockBackgroundManager{
		snapshot:       snapshot,
		sequenceNumber: 1,
	}

	err = p.sendBackgroundSnapshot()
	if err != nil {
		t.Errorf("sendBackgroundSnapshot() error = %v, want nil", err)
	}

	// Check that sequence number was updated
	if p.lastBackgroundSeq != 1 {
		t.Errorf("Expected lastBackgroundSeq = 1, got %d", p.lastBackgroundSeq)
	}

	// Check that timestamp was updated
	if p.lastBackgroundSent.IsZero() {
		t.Error("Expected lastBackgroundSent to be set")
	}
}

func TestPublisher_Publish_SetsFrameType(t *testing.T) {
	cfg := DefaultConfig()
	p := NewPublisher(cfg)
	p.running.Store(true)

	// Create a frame bundle
	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test-sensor",
		PointCloud: &PointCloudFrame{
			PointCount: 100,
		},
	}

	// Publish frame
	p.Publish(frame)

	// Check that frame type was set to foreground
	if frame.FrameType != FrameTypeForeground {
		t.Errorf("Expected FrameType to be FrameTypeForeground, got %v", frame.FrameType)
	}
}

func TestPublisher_Publish_SetsBackgroundSeq(t *testing.T) {
	cfg := DefaultConfig()
	p := NewPublisher(cfg)
	p.running.Store(true)

	// Initialize background sent time to prevent sending background snapshot
	p.lastBackgroundSent = time.Now()
	p.lastBackgroundSeq = 42

	p.backgroundMgr = &mockBackgroundManager{
		sequenceNumber: 42,
	}

	frame := &FrameBundle{
		FrameID:        1,
		TimestampNanos: time.Now().UnixNano(),
		SensorID:       "test-sensor",
	}

	p.Publish(frame)

	// Check that background sequence was set
	if frame.BackgroundSeq != 42 {
		t.Errorf("Expected BackgroundSeq = 42, got %d", frame.BackgroundSeq)
	}
}

func TestBackgroundSnapshot_Fields(t *testing.T) {
	// Test that BackgroundSnapshot can be created and fields accessed
	snapshot := &BackgroundSnapshot{
		SequenceNumber: 1,
		TimestampNanos: 123456789,
		X:              []float32{1.0, 2.0},
		Y:              []float32{1.0, 2.0},
		Z:              []float32{0.0, 0.0},
		Confidence:     []uint32{10, 20},
		GridMetadata: GridMetadata{
			Rings:            40,
			AzimuthBins:      1800,
			RingElevations:   []float32{-15.0, -14.0},
			SettlingComplete: true,
		},
	}

	if snapshot.SequenceNumber != 1 {
		t.Errorf("Expected SequenceNumber = 1, got %d", snapshot.SequenceNumber)
	}
	if len(snapshot.X) != 2 {
		t.Errorf("Expected 2 points, got %d", len(snapshot.X))
	}
	if snapshot.GridMetadata.Rings != 40 {
		t.Errorf("Expected 40 rings, got %d", snapshot.GridMetadata.Rings)
	}
}

func TestFrameType_Constants(t *testing.T) {
	// Verify frame type constants
	if FrameTypeFull != 0 {
		t.Errorf("FrameTypeFull should be 0, got %d", FrameTypeFull)
	}
	if FrameTypeForeground != 1 {
		t.Errorf("FrameTypeForeground should be 1, got %d", FrameTypeForeground)
	}
	if FrameTypeBackground != 2 {
		t.Errorf("FrameTypeBackground should be 2, got %d", FrameTypeBackground)
	}
	if FrameTypeDelta != 3 {
		t.Errorf("FrameTypeDelta should be 3, got %d", FrameTypeDelta)
	}
}
