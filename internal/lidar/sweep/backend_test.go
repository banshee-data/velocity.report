package sweep

import (
	"context"
	"testing"
	"time"
)

func TestWaitForPCAPDone_NilChannel(t *testing.T) {
	err := WaitForPCAPDone(context.Background(), nil)
	if err != nil {
		t.Errorf("expected nil error for nil channel, got %v", err)
	}
}

func TestWaitForPCAPDone_ClosedChannel(t *testing.T) {
	ch := make(chan struct{})
	close(ch)

	err := WaitForPCAPDone(context.Background(), ch)
	if err != nil {
		t.Errorf("expected nil error for closed channel, got %v", err)
	}
}

func TestWaitForPCAPDone_ChannelClosedDuringWait(t *testing.T) {
	ch := make(chan struct{})

	go func() {
		time.Sleep(10 * time.Millisecond)
		close(ch)
	}()

	err := WaitForPCAPDone(context.Background(), ch)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestWaitForPCAPDone_ContextCancelled(t *testing.T) {
	ch := make(chan struct{}) // never closes
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := WaitForPCAPDone(ctx, ch)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestWaitForPCAPDone_ContextTimeout(t *testing.T) {
	ch := make(chan struct{}) // never closes
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := WaitForPCAPDone(ctx, ch)
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestPCAPReplayConfig_Fields(t *testing.T) {
	cfg := PCAPReplayConfig{
		PCAPFile:         "/path/to/test.pcap",
		StartSeconds:     5.0,
		DurationSeconds:  30.0,
		MaxRetries:       3,
		AnalysisMode:     true,
		SpeedMode:        "realtime",
		DisableRecording: true,
	}

	if cfg.PCAPFile != "/path/to/test.pcap" {
		t.Errorf("PCAPFile mismatch: got %s", cfg.PCAPFile)
	}
	if cfg.StartSeconds != 5.0 {
		t.Errorf("StartSeconds mismatch: got %f", cfg.StartSeconds)
	}
	if cfg.DurationSeconds != 30.0 {
		t.Errorf("DurationSeconds mismatch: got %f", cfg.DurationSeconds)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries mismatch: got %d", cfg.MaxRetries)
	}
	if !cfg.AnalysisMode {
		t.Error("AnalysisMode should be true")
	}
	if cfg.SpeedMode != "realtime" {
		t.Errorf("SpeedMode mismatch: got %s", cfg.SpeedMode)
	}
	if !cfg.DisableRecording {
		t.Error("DisableRecording should be true")
	}
}
