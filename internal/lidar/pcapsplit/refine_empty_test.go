package pcapsplit

import (
	"testing"
	"time"
)

func TestRefineTimeline_Empty(t *testing.T) {
	if got := refineTimeline(nil, TimelineConfig{MaxMotionGapSec: 30}); got != nil {
		t.Errorf("refineTimeline(nil) = %v, want nil", got)
	}
	if got := coalesce(nil, time.Time{}); got != nil {
		t.Errorf("coalesce(nil) = %v, want nil", got)
	}
}
