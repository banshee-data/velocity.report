package lidar

import (
	"bytes"
	"context"
	"log"
	"sync"
	"testing"
	"time"
)

// mockPersister implements Persister for testing
type mockPersister struct {
	mu           sync.Mutex
	persistCount int
	reasons      []string
	err          error
}

func (m *mockPersister) Persist(store BgStore, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persistCount++
	m.reasons = append(m.reasons, reason)
	return m.err
}

func (m *mockPersister) getPersistCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.persistCount
}

func (m *mockPersister) getReasons() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.reasons...)
}

// mockBgStore implements BgStore for testing
type mockBgStore struct{}

func (m *mockBgStore) InsertBgSnapshot(s *BgSnapshot) (int64, error) {
	return 1, nil
}

func TestNewBackgroundFlusher(t *testing.T) {
	persister := &mockPersister{}
	store := &mockBgStore{}

	cfg := BackgroundFlusherConfig{
		Manager:  persister,
		Store:    store,
		Interval: 10 * time.Second,
		Reason:   "test_flush",
	}

	flusher := NewBackgroundFlusher(cfg)

	if flusher.manager != persister {
		t.Error("expected manager to be set")
	}
	if flusher.store != store {
		t.Error("expected store to be set")
	}
	if flusher.interval != 10*time.Second {
		t.Errorf("expected interval 10s, got %v", flusher.interval)
	}
	if flusher.reason != "test_flush" {
		t.Errorf("expected reason 'test_flush', got %q", flusher.reason)
	}
}

func TestNewBackgroundFlusher_DefaultReason(t *testing.T) {
	cfg := BackgroundFlusherConfig{
		Manager:  &mockPersister{},
		Store:    &mockBgStore{},
		Interval: 10 * time.Second,
	}

	flusher := NewBackgroundFlusher(cfg)

	if flusher.reason != "periodic_flush" {
		t.Errorf("expected default reason 'periodic_flush', got %q", flusher.reason)
	}
}

func TestBackgroundFlusher_Run_ZeroInterval(t *testing.T) {
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	cfg := BackgroundFlusherConfig{
		Manager:  &mockPersister{},
		Store:    &mockBgStore{},
		Interval: 0,
		Logger:   logger,
	}

	flusher := NewBackgroundFlusher(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := flusher.Run(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !bytes.Contains(logBuf.Bytes(), []byte("interval is zero")) {
		t.Error("expected log message about zero interval")
	}
}

func TestBackgroundFlusher_Run_PeriodicFlush(t *testing.T) {
	persister := &mockPersister{}
	store := &mockBgStore{}

	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	cfg := BackgroundFlusherConfig{
		Manager:  persister,
		Store:    store,
		Interval: 50 * time.Millisecond,
		Reason:   "test",
		Logger:   logger,
	}

	flusher := NewBackgroundFlusher(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Millisecond)
	defer cancel()

	err := flusher.Run(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should have flushed at least 2-3 times (50ms interval over 180ms)
	// Plus final flush on context cancellation
	count := persister.getPersistCount()
	if count < 2 {
		t.Errorf("expected at least 2 flushes, got %d", count)
	}

	reasons := persister.getReasons()
	// Last reason should be "final_flush"
	if len(reasons) > 0 && reasons[len(reasons)-1] != "final_flush" {
		t.Errorf("expected last reason to be 'final_flush', got %q", reasons[len(reasons)-1])
	}
}

func TestBackgroundFlusher_Stop(t *testing.T) {
	persister := &mockPersister{}
	store := &mockBgStore{}

	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	cfg := BackgroundFlusherConfig{
		Manager:  persister,
		Store:    store,
		Interval: 1 * time.Hour, // Long interval so we control timing
		Logger:   logger,
	}

	flusher := NewBackgroundFlusher(cfg)

	// Run in background
	runDone := make(chan error, 1)
	go func() {
		runDone <- flusher.Run(context.Background())
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	if !flusher.IsRunning() {
		t.Error("expected flusher to be running")
	}

	// Stop it
	flusher.Stop()

	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("flusher did not stop in time")
	}

	if flusher.IsRunning() {
		t.Error("expected flusher to not be running after Stop()")
	}

	// Should have called final flush
	reasons := persister.getReasons()
	if len(reasons) == 0 || reasons[len(reasons)-1] != "final_flush" {
		t.Error("expected final_flush on stop")
	}
}

func TestBackgroundFlusher_Stop_NotRunning(t *testing.T) {
	cfg := BackgroundFlusherConfig{
		Manager:  &mockPersister{},
		Store:    &mockBgStore{},
		Interval: 1 * time.Hour,
	}

	flusher := NewBackgroundFlusher(cfg)

	// Stop when not running should not panic
	flusher.Stop()
}

func TestBackgroundFlusher_Stop_MultipleTimes(t *testing.T) {
	persister := &mockPersister{}
	store := &mockBgStore{}

	cfg := BackgroundFlusherConfig{
		Manager:  persister,
		Store:    store,
		Interval: 1 * time.Hour,
	}

	flusher := NewBackgroundFlusher(cfg)

	// Run in background
	go flusher.Run(context.Background())
	time.Sleep(50 * time.Millisecond)

	// Stop multiple times should not panic
	flusher.Stop()
	flusher.Stop()
	flusher.Stop()
}

func TestBackgroundFlusher_IsRunning(t *testing.T) {
	cfg := BackgroundFlusherConfig{
		Manager:  &mockPersister{},
		Store:    &mockBgStore{},
		Interval: 1 * time.Hour,
	}

	flusher := NewBackgroundFlusher(cfg)

	if flusher.IsRunning() {
		t.Error("expected flusher to not be running initially")
	}

	go flusher.Run(context.Background())
	time.Sleep(50 * time.Millisecond)

	if !flusher.IsRunning() {
		t.Error("expected flusher to be running after Run()")
	}

	flusher.Stop()

	if flusher.IsRunning() {
		t.Error("expected flusher to not be running after Stop()")
	}
}

func TestBackgroundFlusher_FlushNow(t *testing.T) {
	persister := &mockPersister{}
	store := &mockBgStore{}

	cfg := BackgroundFlusherConfig{
		Manager:  persister,
		Store:    store,
		Interval: 1 * time.Hour, // Long interval
		Reason:   "manual",
	}

	flusher := NewBackgroundFlusher(cfg)

	// FlushNow should work even when not running
	flusher.FlushNow()

	count := persister.getPersistCount()
	if count != 1 {
		t.Errorf("expected 1 flush after FlushNow(), got %d", count)
	}

	reasons := persister.getReasons()
	if len(reasons) != 1 || reasons[0] != "manual" {
		t.Errorf("expected reason 'manual', got %v", reasons)
	}
}

func TestBackgroundFlusher_Run_AlreadyRunning(t *testing.T) {
	cfg := BackgroundFlusherConfig{
		Manager:  &mockPersister{},
		Store:    &mockBgStore{},
		Interval: 1 * time.Hour,
	}

	flusher := NewBackgroundFlusher(cfg)

	// Start first Run
	go flusher.Run(context.Background())
	time.Sleep(50 * time.Millisecond)

	// Second Run should return immediately
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := flusher.Run(ctx)
	if err != nil {
		t.Errorf("unexpected error from second Run(): %v", err)
	}

	flusher.Stop()
}

func TestBackgroundFlusher_NilManager(t *testing.T) {
	cfg := BackgroundFlusherConfig{
		Manager:  nil,
		Store:    &mockBgStore{},
		Interval: 50 * time.Millisecond,
	}

	flusher := NewBackgroundFlusher(cfg)

	// Should not panic with nil manager
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := flusher.Run(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBackgroundFlusher_NilStore(t *testing.T) {
	cfg := BackgroundFlusherConfig{
		Manager:  &mockPersister{},
		Store:    nil,
		Interval: 50 * time.Millisecond,
	}

	flusher := NewBackgroundFlusher(cfg)

	// Should not panic with nil store
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := flusher.Run(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
