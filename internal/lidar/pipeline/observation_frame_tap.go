package pipeline

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
)

// ObservationFrameSink receives one foreground-complete l4bobserve.FrameRecord
// for every frame the callback is given, in sequence order: empty, unsettled,
// suppressed and failed frames included. Records arrive after L4 and before
// L5, and are built from L3's foreground and DBSCAN's membership, so neither
// track acceptance nor display filtering can remove evidence from them. The
// record is owned by the sink.
type ObservationFrameSink interface {
	ObserveFrame(l4bobserve.FrameRecord) error
}

// ObservationFrameFailureSink is an optional strict-mode extension. The live
// callback cannot return an error, so an offline replay uses this hook to
// retain a tap that could not start, or a frame whose lineage broke.
type ObservationFrameFailureSink interface {
	RecordObservationFrameFailure(error)
}

// observationFrameTap adapts the pipeline's stage boundaries to an
// l4bobserve.ExtractionBuilder. A nil tap hands out nil drafts, whose methods
// do nothing, so the disabled path costs one nil check per stage.
type observationFrameTap struct {
	sink    ObservationFrameSink
	builder *l4bobserve.ExtractionBuilder
}

// newObservationFrameTap returns nil when no sink is configured. A sink
// without explicit source and calibration identities is refused rather than
// given guessed ones: the failure is logged and, in strict mode, retained.
func newObservationFrameTap(cfg *TrackingPipelineConfig) *observationFrameTap {
	if isNilInterface(cfg.ObservationFrameSink) {
		return nil
	}
	builder, err := l4bobserve.NewExtractionBuilder(l4bobserve.Extraction{
		SourceID:        cfg.ObservationSourceID,
		CalibrationID:   cfg.ObservationCalibrationID,
		SensorID:        cfg.SensorID,
		CoordinateFrame: fmt.Sprintf("site/%s", cfg.SensorID),
	})
	if err != nil {
		err = fmt.Errorf("observation frame tap disabled: %w", err)
		opsf("%v", err)
		if strict, ok := cfg.ObservationFrameSink.(ObservationFrameFailureSink); ok {
			strict.RecordObservationFrameFailure(err)
		}
		return nil
	}
	return &observationFrameTap{sink: cfg.ObservationFrameSink, builder: builder}
}

func (t *observationFrameTap) begin(frame *l2frames.LiDARFrame) *l4bobserve.FrameDraft {
	if t == nil {
		return nil
	}
	return t.builder.BeginFrame(frame)
}

// foreground records L3's output and the settling state it was produced under.
func (t *observationFrameTap) foreground(draft *l4bobserve.FrameDraft, bm *l3grid.BackgroundManager, mask []bool, foreground int) {
	if draft == nil {
		return
	}
	state := l4bobserve.BackgroundSettling
	if bm.IsSettlingComplete() {
		state = l4bobserve.BackgroundSettled
	}
	draft.SetBackground(state)
	draft.SetForeground(mask, foreground)
}

// finish emits the frame once. The callback calls it after clustering so the
// record precedes L5, and defers it so every early return is still recorded.
func (t *observationFrameTap) finish(draft *l4bobserve.FrameDraft) {
	if t == nil || draft.Finished() {
		return
	}
	record, err := draft.Finish()
	if err != nil {
		// The record is a failed frame naming the broken stage; emit it so
		// the sequence stays dense, and make the fault visible.
		opsf("Observation frame %d recorded as failed: %v", record.Sequence, err)
		if strict, ok := t.sink.(ObservationFrameFailureSink); ok {
			strict.RecordObservationFrameFailure(err)
		}
	}
	if err := t.sink.ObserveFrame(record); err != nil {
		opsf("Failed to deliver observation frame %d: %v", record.Sequence, err)
	}
}
