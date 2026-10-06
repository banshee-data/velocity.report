package pipeline

import (
	"strings"
	"testing"
	"time"
)

// These tests drive a fake clock rather than sleeping.
//
// The reason is not tidiness. time.Sleep guarantees a minimum duration and not
// a maximum, so a sleep-based test can assert "this stage took at least 2ms"
// safely, but cannot safely assert which stage was longest: under the load of a
// full package run a 2ms sleep can outrun a 5ms one, and
// TestFrameTimerSlowestStage failed exactly that way. Driving the clock makes
// every duration exact, so these assert the timer's attribution logic — which
// is the actual subject — instead of the OS scheduler.
//
// TestFrameTimerUsesTheWallClockByDefault keeps the real time.Now path covered.

// fakeClock is a manually advanced clock.
type fakeClock struct{ t time.Time }

// newFakeClock starts at a fixed instant. The value is arbitrary but not the
// zero time, so a bug that left a timestamp unset shows up as a wildly wrong
// duration rather than a plausible one.
func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time { return c.t }

func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestFrameTimerStageSequencing(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	ft := newFrameTimerWithClock("test-001", clock.now)

	ft.Stage("alpha")
	clock.advance(3 * time.Millisecond)
	ft.Stage("beta")
	clock.advance(7 * time.Millisecond)
	ft.End()

	if len(ft.stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(ft.stages))
	}
	// Exact durations, which a sleep-based version could only bound from
	// below. A stage attributed to the wrong boundary now fails here.
	for i, want := range []stageTiming{
		{name: "alpha", duration: 3 * time.Millisecond},
		{name: "beta", duration: 7 * time.Millisecond},
	} {
		if ft.stages[i].name != want.name {
			t.Errorf("stage[%d] name = %q, want %q", i, ft.stages[i].name, want.name)
		}
		if ft.stages[i].duration != want.duration {
			t.Errorf("stage[%d] duration = %v, want %v", i, ft.stages[i].duration, want.duration)
		}
	}
}

func TestFrameTimerTotal(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	ft := newFrameTimerWithClock("test-002", clock.now)

	// Time before the first Stage() call belongs to no stage, and Total
	// deliberately excludes it. Advancing here pins that.
	clock.advance(50 * time.Millisecond)

	ft.Stage("a")
	clock.advance(2 * time.Millisecond)
	ft.Stage("b")
	clock.advance(4 * time.Millisecond)
	ft.End()

	if got, want := ft.Total(), 6*time.Millisecond; got != want {
		t.Errorf("Total() = %v, want %v (the 50ms before the first stage is not any stage's)", got, want)
	}
	if got, want := ft.TotalMs(), 6.0; got != want {
		t.Errorf("TotalMs() = %v, want %v", got, want)
	}
}

func TestFrameTimerFormat(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	ft := newFrameTimerWithClock("test-003", clock.now)

	ft.Stage("foreground")
	clock.advance(2100 * time.Microsecond)
	ft.Stage("cluster")
	clock.advance(8300 * time.Microsecond)
	ft.End()

	// The whole string, not substrings: with exact durations the formatting
	// itself — one decimal place, space separated, stage order preserved — is
	// checkable, and that is what the log consumer depends on.
	if got, want := ft.Format(), "foreground=2.1ms cluster=8.3ms"; got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

func TestFrameTimerFormatEmpty(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	ft := newFrameTimerWithClock("test-004", clock.now)
	ft.End()

	if ft.Format() != "" {
		t.Errorf("expected empty Format() for no stages, got: %q", ft.Format())
	}
	if ft.Total() != 0 {
		t.Errorf("expected zero Total() for no stages, got: %v", ft.Total())
	}
}

func TestFrameTimerSlowestStage(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	ft := newFrameTimerWithClock("test-005", clock.now)

	// The case that used to flake. With sleeps, "medium" could overrun "slow"
	// under load and be reported as the slowest; with an advanced clock the
	// ordering is exact.
	ft.Stage("fast")
	clock.advance(1 * time.Millisecond)
	ft.Stage("slow")
	clock.advance(5 * time.Millisecond)
	ft.Stage("medium")
	clock.advance(2 * time.Millisecond)
	ft.End()

	name, dur := ft.SlowestStage()
	if name != "slow" {
		t.Errorf("slowest stage = %q, want \"slow\"", name)
	}
	if want := 5 * time.Millisecond; dur != want {
		t.Errorf("slowest duration = %v, want %v", dur, want)
	}
}

func TestFrameTimerSlowestStagePrefersTheFirstOfEqualStages(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	ft := newFrameTimerWithClock("test-005b", clock.now)

	// A tie was unreachable while durations came from sleeps. SlowestStage
	// compares with a strict >, so the earliest of equal stages wins; pinning
	// it makes the report stable rather than dependent on iteration order.
	ft.Stage("first")
	clock.advance(4 * time.Millisecond)
	ft.Stage("second")
	clock.advance(4 * time.Millisecond)
	ft.End()

	if name, _ := ft.SlowestStage(); name != "first" {
		t.Errorf("slowest of two equal stages = %q, want the earlier \"first\"", name)
	}
}

func TestFrameTimerSlowestStageEmpty(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	ft := newFrameTimerWithClock("test-006", clock.now)
	ft.End()

	name, dur := ft.SlowestStage()
	if name != "" {
		t.Errorf("expected empty name for no stages, got %q", name)
	}
	if dur != 0 {
		t.Errorf("expected zero duration for no stages, got %v", dur)
	}
}

func TestFrameTimerEndIdempotent(t *testing.T) {
	t.Parallel()
	clock := newFakeClock()
	ft := newFrameTimerWithClock("test-007", clock.now)

	ft.Stage("only")
	clock.advance(1 * time.Millisecond)
	ft.End()
	clock.advance(9 * time.Millisecond)
	ft.End() // second call should be no-op

	if len(ft.stages) != 1 {
		t.Fatalf("expected 1 stage after double End(), got %d", len(ft.stages))
	}
	// And the second End must not have extended the stage it already closed,
	// which advancing the clock in between makes visible.
	if got, want := ft.stages[0].duration, 1*time.Millisecond; got != want {
		t.Errorf("stage duration = %v, want %v: the second End() re-measured it", got, want)
	}
}

func TestFrameTimerUsesTheWallClockByDefault(t *testing.T) {
	t.Parallel()
	// Everything above injects a clock, so without this nothing would notice
	// if newFrameTimer stopped wiring time.Now. Assertions are lower bounds
	// only, which is what a sleep can actually guarantee.
	ft := newFrameTimer("test-008")

	ft.Stage("real")
	time.Sleep(2 * time.Millisecond)
	ft.End()

	if len(ft.stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(ft.stages))
	}
	if got := ft.stages[0].duration; got < 2*time.Millisecond {
		t.Errorf("stage duration = %v, want at least the 2ms slept", got)
	}
	if !strings.Contains(ft.Format(), "real=") {
		t.Errorf("Format() = %q, want it to name the stage", ft.Format())
	}
}
