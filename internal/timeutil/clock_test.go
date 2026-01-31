package timeutil

import (
	"testing"
	"time"
)

func TestRealClock_Now(t *testing.T) {
	clock := RealClock{}
	before := time.Now()
	now := clock.Now()
	after := time.Now()

	if now.Before(before) || now.After(after) {
		t.Errorf("Now() = %v, expected between %v and %v", now, before, after)
	}
}

func TestRealClock_Since(t *testing.T) {
	clock := RealClock{}
	past := time.Now().Add(-time.Second)
	d := clock.Since(past)

	if d < time.Second {
		t.Errorf("Since() returned %v, expected >= 1s", d)
	}
}

func TestRealClock_Until(t *testing.T) {
	clock := RealClock{}
	future := time.Now().Add(time.Hour)
	d := clock.Until(future)

	if d < 59*time.Minute {
		t.Errorf("Until() returned %v, expected >= 59m", d)
	}
}

func TestRealClock_NewTimer(t *testing.T) {
	clock := RealClock{}
	timer := clock.NewTimer(10 * time.Millisecond)
	defer timer.Stop()

	select {
	case <-timer.C():
		// Timer fired as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("timer did not fire")
	}
}

func TestRealClock_NewTicker(t *testing.T) {
	clock := RealClock{}
	ticker := clock.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	select {
	case <-ticker.C():
		// Ticker fired as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("ticker did not fire")
	}
}

func TestMockClock_Now(t *testing.T) {
	fixedTime := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	clock := NewMockClock(fixedTime)
	now := clock.Now()

	if !now.Equal(fixedTime) {
		t.Errorf("got %v, want %v", now, fixedTime)
	}
}

func TestMockClock_Set(t *testing.T) {
	clock := NewMockClock(time.Time{})
	newTime := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	clock.Set(newTime)

	if !clock.Now().Equal(newTime) {
		t.Errorf("got %v, want %v", clock.Now(), newTime)
	}
}

func TestMockClock_Advance(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewMockClock(start)
	clock.Advance(time.Hour)
	expected := start.Add(time.Hour)

	if !clock.Now().Equal(expected) {
		t.Errorf("got %v, want %v", clock.Now(), expected)
	}
}

func TestMockClock_Since(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewMockClock(now)
	past := now.Add(-5 * time.Minute)
	d := clock.Since(past)

	if d != 5*time.Minute {
		t.Errorf("got %v, want 5m", d)
	}
}

func TestMockClock_Until(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := NewMockClock(now)
	future := now.Add(10 * time.Minute)
	d := clock.Until(future)

	if d != 10*time.Minute {
		t.Errorf("got %v, want 10m", d)
	}
}

func TestMockClock_Sleep(t *testing.T) {
	clock := NewMockClock(time.Now())
	clock.Sleep(time.Second)
	clock.Sleep(2 * time.Second)
	sleeps := clock.Sleeps()

	if len(sleeps) != 2 {
		t.Fatalf("got %d sleeps, want 2", len(sleeps))
	}

	if sleeps[0] != time.Second {
		t.Errorf("first sleep: got %v, want 1s", sleeps[0])
	}

	if sleeps[1] != 2*time.Second {
		t.Errorf("second sleep: got %v, want 2s", sleeps[1])
	}
}

func TestMockClock_Timer(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewMockClock(start)
	timer := clock.NewTimer(5 * time.Minute)

	// Timer should not fire yet
	select {
	case <-timer.C():
		t.Error("timer fired too early")
	default:
		// Expected
	}

	// Advance past timer
	clock.Advance(6 * time.Minute)

	// Timer should have fired
	select {
	case <-timer.C():
		// Expected
	default:
		t.Error("timer did not fire after advance")
	}
}

func TestMockClock_Timer_Stop(t *testing.T) {
	clock := NewMockClock(time.Now())
	timer := clock.NewTimer(time.Minute)
	wasActive := timer.Stop()

	if !wasActive {
		t.Error("Stop should return true for active timer")
	}

	// Advance and verify timer doesn't fire
	clock.Advance(2 * time.Minute)

	select {
	case <-timer.C():
		t.Error("stopped timer should not fire")
	default:
		// Expected
	}
}

func TestMockClock_Ticker(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewMockClock(start)
	ticker := clock.NewTicker(time.Minute)

	// Ticker should not tick yet
	select {
	case <-ticker.C():
		t.Error("ticker fired too early")
	default:
	}

	// Advance to first tick
	clock.Advance(time.Minute)

	select {
	case <-ticker.C():
		// Expected
	default:
		t.Error("ticker did not fire after first interval")
	}
}

func TestMockClock_Ticker_Stop(t *testing.T) {
	clock := NewMockClock(time.Now())
	ticker := clock.NewTicker(time.Second)
	ticker.Stop()
	clock.Advance(5 * time.Second)

	select {
	case <-ticker.C():
		t.Error("stopped ticker should not tick")
	default:
		// Expected
	}
}

func TestMockClock_After(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := NewMockClock(start)
	ch := clock.After(time.Hour)

	// Should not receive yet
	select {
	case <-ch:
		t.Error("After channel received too early")
	default:
	}

	// Advance past duration
	clock.Advance(2 * time.Hour)

	select {
	case <-ch:
		// Expected
	default:
		t.Error("After channel did not receive after advance")
	}
}

func TestMockTimer_Reset(t *testing.T) {
	clock := NewMockClock(time.Now())
	timer := clock.NewTimer(time.Minute)

	// Stop and reset
	timer.Stop()
	timer.Reset(30 * time.Second)

	// Should not fire yet
	select {
	case <-timer.C():
		t.Error("timer fired too early after reset")
	default:
	}
}

func TestMockTicker_Trigger(t *testing.T) {
	clock := NewMockClock(time.Now())
	ticker := clock.NewTicker(time.Hour).(*MockTicker)
	now := clock.Now()
	ticker.Trigger(now)

	select {
	case received := <-ticker.C():
		if !received.Equal(now) {
			t.Errorf("got %v, want %v", received, now)
		}
	default:
		t.Error("Trigger did not send tick")
	}
}

func TestMockTicker_Reset(t *testing.T) {
	clock := NewMockClock(time.Now())
	ticker := clock.NewTicker(time.Second).(*MockTicker)
	ticker.Stop()
	ticker.Reset(time.Minute)

	if ticker.stopped {
		t.Error("ticker should not be stopped after Reset")
	}

	if ticker.duration != time.Minute {
		t.Errorf("got duration %v, want 1m", ticker.duration)
	}
}
