package pipeline

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// maskedEmptyAdapter records how frames that would be published empty reach
// an adapter that can take their foreground mask.
type maskedEmptyAdapter struct {
	mockVisualiserAdapter
	masks [][]bool
}

func (m *maskedEmptyAdapter) AdaptEmptyFrameWithMask(frame *l2frames.LiDARFrame, mask []bool) interface{} {
	m.masks = append(m.masks, mask)
	return struct{}{}
}

// A frame with nothing in the foreground still reaches the adapter with its
// mask, so full-frames mode can send its points; before a mask exists the
// plain empty frame is used.
func TestEmptyFramesCarryTheirMaskToAnAdapterThatTakesIt(t *testing.T) {
	sensorID := "full-frames-empty-" + t.Name()
	adapter := &maskedEmptyAdapter{}
	cfg := &TrackingPipelineConfig{
		SensorID:            sensorID,
		BackgroundManager:   makeTestBgManager(t, sensorID),
		VisualiserPublisher: &mockVisualiserPublisher{},
		VisualiserAdapter:   adapter,
	}
	cb := cfg.NewFrameCallback()
	frame := makeStableFrame("quiet", time.Now(), 20.0)
	cb(frame)

	if len(adapter.masks) != 1 || adapter.adaptEmptyCalls != 0 {
		t.Fatalf("masked calls %d, plain calls %d; want one masked", len(adapter.masks), adapter.adaptEmptyCalls)
	}
	if got := len(adapter.masks[0]); got != len(frame.Points) {
		t.Errorf("mask covers %d points, want %d", got, len(frame.Points))
	}

	noModel := &maskedEmptyAdapter{}
	cfg = &TrackingPipelineConfig{
		SensorID:            sensorID + "-none",
		VisualiserPublisher: &mockVisualiserPublisher{},
		VisualiserAdapter:   noModel,
	}
	cfg.NewFrameCallback()(makeStableFrame("no-model", time.Now(), 20.0))
	if len(noModel.masks) != 0 || noModel.adaptEmptyCalls != 1 {
		t.Errorf("without a background model: masked %d, plain %d; want one plain", len(noModel.masks), noModel.adaptEmptyCalls)
	}
}
