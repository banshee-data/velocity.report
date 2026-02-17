package sqlite

import (
	"errors"
	"testing"
	"time"
)

func TestIsSQLiteBusy(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "database is locked",
			err:      errors.New("database is locked (5) (SQLITE_BUSY)"),
			expected: true,
		},
		{
			name:     "SQLITE_BUSY",
			err:      errors.New("SQLITE_BUSY"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSQLiteBusy(tt.err)
			if result != tt.expected {
				t.Errorf("isSQLiteBusy(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestRetryOnBusy(t *testing.T) {
	t.Run("success on first try", func(t *testing.T) {
		callCount := 0
		err := retryOnBusy(func() error {
			callCount++
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if callCount != 1 {
			t.Errorf("expected 1 call, got %d", callCount)
		}
	})

	t.Run("success after retry", func(t *testing.T) {
		callCount := 0
		err := retryOnBusy(func() error {
			callCount++
			if callCount < 3 {
				return errors.New("database is locked (5) (SQLITE_BUSY)")
			}
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if callCount != 3 {
			t.Errorf("expected 3 calls, got %d", callCount)
		}
	})

	t.Run("non-busy error fails immediately", func(t *testing.T) {
		callCount := 0
		testErr := errors.New("some other error")
		err := retryOnBusy(func() error {
			callCount++
			return testErr
		})

		if err != testErr {
			t.Errorf("expected error %v, got %v", testErr, err)
		}
		if callCount != 1 {
			t.Errorf("expected 1 call, got %d", callCount)
		}
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		callCount := 0
		err := retryOnBusy(func() error {
			callCount++
			return errors.New("database is locked (5) (SQLITE_BUSY)")
		})

		if err == nil {
			t.Error("expected error, got nil")
		}
		if callCount != 5 {
			t.Errorf("expected 5 calls (max retries), got %d", callCount)
		}
	})

	t.Run("exponential backoff timing", func(t *testing.T) {
		callCount := 0
		delays := []time.Duration{}
		lastCall := time.Now()

		err := retryOnBusy(func() error {
			now := time.Now()
			if callCount > 0 {
				delays = append(delays, now.Sub(lastCall))
			}
			lastCall = now
			callCount++

			if callCount < 3 {
				return errors.New("database is locked (5) (SQLITE_BUSY)")
			}
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Verify delays are approximately exponential (with some tolerance)
		// Expected: 10ms, 20ms
		if len(delays) != 2 {
			t.Errorf("expected 2 delays, got %d", len(delays))
		}

		// Allow 50% tolerance for timing variations
		if len(delays) >= 1 && (delays[0] < 5*time.Millisecond || delays[0] > 15*time.Millisecond) {
			t.Errorf("first delay should be ~10ms, got %v", delays[0])
		}
		if len(delays) >= 2 && (delays[1] < 10*time.Millisecond || delays[1] > 30*time.Millisecond) {
			t.Errorf("second delay should be ~20ms, got %v", delays[1])
		}
	})
}
