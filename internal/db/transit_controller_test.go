package db

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestTransitController_IsEnabled(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 1, "test-model")
	tc := NewTransitController(worker)

	if !tc.IsEnabled() {
		t.Error("TransitController should be enabled by default")
	}
}

func TestTransitController_SetEnabled(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 1, "test-model")
	tc := NewTransitController(worker)

	// Test disabling
	tc.SetEnabled(false)
	if tc.IsEnabled() {
		t.Error("TransitController should be disabled after SetEnabled(false)")
	}

	// Test re-enabling
	tc.SetEnabled(true)
	if !tc.IsEnabled() {
		t.Error("TransitController should be enabled after SetEnabled(true)")
	}
}

func TestTransitController_SetEnabled_ThreadSafety(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 1, "test-model")
	tc := NewTransitController(worker)

	var wg sync.WaitGroup
	// Launch multiple goroutines to toggle the enabled state
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(val bool) {
			defer wg.Done()
			tc.SetEnabled(val)
			_ = tc.IsEnabled()
		}(i%2 == 0)
	}

	wg.Wait()
	// Test passes if no race condition detected
}

func TestTransitController_TriggerManualRun(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 1, "test-model")
	tc := NewTransitController(worker)

	// Should not block or panic
	tc.TriggerManualRun()
	tc.TriggerManualRun() // Should coalesce
}

func TestTransitController_TriggerManualRun_BufferSaturation(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 1, "test-model")
	tc := NewTransitController(worker)

	// Send multiple triggers rapidly - should not block
	done := make(chan bool)
	go func() {
		for i := 0; i < 10; i++ {
			tc.TriggerManualRun()
		}
		done <- true
	}()

	select {
	case <-done:
		// Success - no blocking
	case <-time.After(100 * time.Millisecond):
		t.Error("TriggerManualRun blocked unexpectedly")
	}
}

func TestTransitController_GetStatus(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 1, "test-model")
	tc := NewTransitController(worker)

	// Test initial status
	status := tc.GetStatus()
	if !status.Enabled {
		t.Error("Expected initial status to be enabled")
	}
	if status.RunCount != 0 {
		t.Errorf("Expected initial run count to be 0, got %d", status.RunCount)
	}
	if !status.IsHealthy {
		t.Error("Expected initial status to be healthy")
	}

	// Test status after disabling
	tc.SetEnabled(false)
	status = tc.GetStatus()
	if status.Enabled {
		t.Error("Expected status to be disabled")
	}
}

func TestTransitController_GetStatus_WithError(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 1, "test-model")
	tc := NewTransitController(worker)

	// Record a run with an error
	tc.recordRun(context.DeadlineExceeded)

	status := tc.GetStatus()
	if status.LastRunError == "" {
		t.Error("Expected LastRunError to be set")
	}
	if status.IsHealthy {
		t.Error("Expected status to be unhealthy when error is present")
	}
	if status.RunCount != 1 {
		t.Errorf("Expected run count to be 1, got %d", status.RunCount)
	}
}

func TestTransitController_recordRun(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 1, "test-model")
	tc := NewTransitController(worker)

	// Record successful run
	tc.recordRun(nil)

	status := tc.GetStatus()
	if status.RunCount != 1 {
		t.Errorf("Expected run count to be 1, got %d", status.RunCount)
	}
	if status.LastRunError != "" {
		t.Error("Expected no error after successful run")
	}
	if status.LastRunAt.IsZero() {
		t.Error("Expected LastRunAt to be set")
	}

	// Record another run
	tc.recordRun(nil)
	status = tc.GetStatus()
	if status.RunCount != 2 {
		t.Errorf("Expected run count to be 2, got %d", status.RunCount)
	}
}

func TestTransitController_Run_ContextCancellation(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 1, "test-model")
	worker.Interval = 10 * time.Millisecond
	tc := NewTransitController(worker)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error)
	go func() {
		done <- tc.Run(ctx)
	}()

	// Wait a bit to let it start
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Run did not terminate after context cancellation")
	}
}

func TestTransitController_Run_DisabledSkipsExecution(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 1, "test-model")
	worker.Interval = 50 * time.Millisecond
	tc := NewTransitController(worker)
	tc.SetEnabled(false) // Disable before starting

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go func() {
		_ = tc.Run(ctx)
	}()

	time.Sleep(150 * time.Millisecond)

	status := tc.GetStatus()
	// Should have 0 runs since it was disabled
	if status.RunCount != 0 {
		t.Errorf("Expected 0 runs when disabled, got %d", status.RunCount)
	}
}

func TestTransitController_Run_ManualTrigger(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 1, "test-model")
	worker.Interval = 1 * time.Hour // Long interval so periodic doesn't trigger
	tc := NewTransitController(worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = tc.Run(ctx)
	}()

	// Wait for initial run to complete
	time.Sleep(100 * time.Millisecond)
	initialCount := tc.GetStatus().RunCount

	// Trigger manual run
	tc.TriggerManualRun()
	time.Sleep(100 * time.Millisecond)

	status := tc.GetStatus()
	if status.RunCount <= initialCount {
		t.Errorf("Expected run count to increase after manual trigger, got %d (was %d)", status.RunCount, initialCount)
	}
}

func TestTransitController_Run_Concurrency(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewTransitWorker(db, 1, "test-model")
	worker.Interval = 50 * time.Millisecond
	tc := NewTransitController(worker)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go func() {
		_ = tc.Run(ctx)
	}()

	// Simulate concurrent API calls
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			time.Sleep(time.Duration(i*10) * time.Millisecond)
			if i%3 == 0 {
				tc.SetEnabled(i%2 == 0)
			}
			if i%3 == 1 {
				tc.TriggerManualRun()
			}
			_ = tc.GetStatus()
		}(i)
	}

	wg.Wait()
	// Test passes if no race condition or panic
}
