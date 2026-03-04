package pipeline

import (
	"strings"
	"testing"
	"time"
)

func TestFrameTimerStageSequencing(t *testing.T) {
	t.Parallel()
	ft := newFrameTimer("test-001")

	ft.Stage("alpha")
	time.Sleep(1 * time.Millisecond)
	ft.Stage("beta")
	time.Sleep(1 * time.Millisecond)
	ft.End()

	if len(ft.stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(ft.stages))
	}
	if ft.stages[0].name != "alpha" {
		t.Errorf("stage[0] name: expected alpha, got %s", ft.stages[0].name)
	}
	if ft.stages[1].name != "beta" {
		t.Errorf("stage[1] name: expected beta, got %s", ft.stages[1].name)
	}
	if ft.stages[0].duration <= 0 {
		t.Errorf("stage[0] duration should be positive, got %v", ft.stages[0].duration)
	}
	if ft.stages[1].duration <= 0 {
		t.Errorf("stage[1] duration should be positive, got %v", ft.stages[1].duration)
	}
}

func TestFrameTimerTotal(t *testing.T) {
	t.Parallel()
	ft := newFrameTimer("test-002")

	ft.Stage("a")
	time.Sleep(2 * time.Millisecond)
	ft.Stage("b")
	time.Sleep(2 * time.Millisecond)
	ft.End()

	total := ft.Total()
	if total < 4*time.Millisecond {
		t.Errorf("expected total >= 4ms, got %v", total)
	}
	if ft.TotalMs() < 4.0 {
		t.Errorf("expected TotalMs >= 4.0, got %.1f", ft.TotalMs())
	}
}

func TestFrameTimerFormat(t *testing.T) {
	t.Parallel()
	ft := newFrameTimer("test-003")

	ft.Stage("foreground")
	time.Sleep(1 * time.Millisecond)
	ft.Stage("cluster")
	time.Sleep(1 * time.Millisecond)
	ft.End()

	s := ft.Format()
	if !strings.Contains(s, "foreground=") {
		t.Errorf("Format() should contain 'foreground=', got: %s", s)
	}
	if !strings.Contains(s, "cluster=") {
		t.Errorf("Format() should contain 'cluster=', got: %s", s)
	}
	if !strings.Contains(s, "ms") {
		t.Errorf("Format() should contain 'ms', got: %s", s)
	}
}

func TestFrameTimerFormatEmpty(t *testing.T) {
	t.Parallel()
	ft := newFrameTimer("test-004")
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
	ft := newFrameTimer("test-005")

	ft.Stage("fast")
	time.Sleep(1 * time.Millisecond)
	ft.Stage("slow")
	time.Sleep(5 * time.Millisecond)
	ft.Stage("medium")
	time.Sleep(2 * time.Millisecond)
	ft.End()

	name, dur := ft.SlowestStage()
	if name != "slow" {
		t.Errorf("expected slowest stage 'slow', got %q", name)
	}
	if dur < 4*time.Millisecond {
		t.Errorf("expected slowest duration >= 4ms, got %v", dur)
	}
}

func TestFrameTimerSlowestStageEmpty(t *testing.T) {
	t.Parallel()
	ft := newFrameTimer("test-006")
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
	ft := newFrameTimer("test-007")

	ft.Stage("only")
	time.Sleep(1 * time.Millisecond)
	ft.End()
	ft.End() // second call should be no-op

	if len(ft.stages) != 1 {
		t.Fatalf("expected 1 stage after double End(), got %d", len(ft.stages))
	}
}
