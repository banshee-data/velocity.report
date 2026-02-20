package debug

import "testing"

// TestRecordGatingRegion_WhenDisabled exercises the !c.enabled guard in RecordGatingRegion.
func TestRecordGatingRegion_WhenDisabled(t *testing.T) {
	c := NewDebugCollector()
	c.SetEnabled(false)
	c.RecordGatingRegion("t1", 1, 2, 3, 4, 0.5)
	if f := c.Emit(); f != nil {
		t.Error("expected nil frame when disabled")
	}
}

// TestRecordGatingRegion_WithoutBeginFrame exercises the c.current==nil guard.
func TestRecordGatingRegion_WithoutBeginFrame(t *testing.T) {
	c := NewDebugCollector()
	c.SetEnabled(true)
	c.RecordGatingRegion("t1", 1, 2, 3, 4, 0.5)
	if f := c.Emit(); f != nil {
		t.Error("expected nil frame without BeginFrame")
	}
}

// TestRecordInnovation_WhenDisabled exercises the !c.enabled guard in RecordInnovation.
func TestRecordInnovation_WhenDisabled(t *testing.T) {
	c := NewDebugCollector()
	c.SetEnabled(false)
	c.RecordInnovation("t1", 1, 2, 3, 4, 0.5)
	if f := c.Emit(); f != nil {
		t.Error("expected nil frame when disabled")
	}
}

// TestRecordInnovation_WithoutBeginFrame exercises the c.current==nil guard.
func TestRecordInnovation_WithoutBeginFrame(t *testing.T) {
	c := NewDebugCollector()
	c.SetEnabled(true)
	c.RecordInnovation("t1", 1, 2, 3, 4, 0.5)
	if f := c.Emit(); f != nil {
		t.Error("expected nil frame without BeginFrame")
	}
}

// TestRecordPrediction_WhenDisabled exercises the !c.enabled guard in RecordPrediction.
func TestRecordPrediction_WhenDisabled(t *testing.T) {
	c := NewDebugCollector()
	c.SetEnabled(false)
	c.RecordPrediction("t1", 1, 2, 3, 4)
	if f := c.Emit(); f != nil {
		t.Error("expected nil frame when disabled")
	}
}

// TestRecordPrediction_WithoutBeginFrame exercises the c.current==nil guard.
func TestRecordPrediction_WithoutBeginFrame(t *testing.T) {
	c := NewDebugCollector()
	c.SetEnabled(true)
	c.RecordPrediction("t1", 1, 2, 3, 4)
	if f := c.Emit(); f != nil {
		t.Error("expected nil frame without BeginFrame")
	}
}
