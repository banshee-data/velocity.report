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
		oldSleep := retrySleep
		defer func() { retrySleep = oldSleep }()

		callCount := 0
		delays := []time.Duration{}
		retrySleep = func(d time.Duration) {
			delays = append(delays, d)
		}

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

		expected := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}
		if len(delays) != len(expected) {
			t.Fatalf("delays = %v, want %v", delays, expected)
		}
		for i := range expected {
			if delays[i] != expected[i] {
				t.Errorf("delay %d = %v, want %v", i, delays[i], expected[i])
			}
		}
	})
}
