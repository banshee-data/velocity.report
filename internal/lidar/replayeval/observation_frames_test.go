package replayeval

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
)

func emptyObservedFrame(sequence uint64) l4bobserve.FrameRecord {
	return l4bobserve.FrameRecord{Sequence: sequence,
		Disposition: l4bobserve.Disposition{Kind: l4bobserve.DispositionObserved},
		Payload:     l4bobserve.PayloadPoints | l4bobserve.PayloadMembership,
		Points: l4bobserve.RetainedPoints{Fields: l4bobserve.FieldAcquisitionTime | l4bobserve.FieldIntensity |
			l4bobserve.FieldChannel | l4bobserve.FieldSourceOrdinal},
		Stages: []l4bobserve.StageCount{{Stage: l4bobserve.StageL2Frame, Output: l4bobserve.KnownCount(0)}}}
}

func newStrictFrameSink(deliver func(l4bobserve.FrameRecord) error) *strictObservationFrameSink {
	return &strictObservationFrameSink{deliver: deliver, stream: l4bobserve.NewStreamValidator(l4bobserve.ForegroundComplete())}
}

// The replay must fail, not deliver a stream with a hole, when a record is
// out of sequence, the caller refuses one, or the tap reports broken lineage.
func TestStrictObservationFrameSinkRetainsTheFirstFailure(t *testing.T) {
	var delivered []uint64
	deliver := func(r l4bobserve.FrameRecord) error { delivered = append(delivered, r.Sequence); return nil }

	sink := newStrictFrameSink(deliver)
	if err := sink.ObserveFrame(emptyObservedFrame(0)); err != nil {
		t.Fatal(err)
	}
	if err := sink.ObserveFrame(emptyObservedFrame(2)); err == nil || !strings.Contains(err.Error(), "explicit gap") {
		t.Fatalf("hole in the sequence = %v", err)
	}
	if err := sink.ObserveFrame(emptyObservedFrame(1)); err != nil {
		t.Fatalf("later frames after a retained failure should not repeat it: %v", err)
	}
	frames, err := sink.result()
	if frames != 1 || err == nil || len(delivered) != 1 {
		t.Fatalf("frames=%d err=%v delivered=%v", frames, err, delivered)
	}

	refused := errors.New("caller full")
	sink = newStrictFrameSink(func(l4bobserve.FrameRecord) error { return refused })
	if err := sink.ObserveFrame(emptyObservedFrame(0)); !errors.Is(err, refused) {
		t.Fatalf("caller error = %v", err)
	}
	if _, err := sink.result(); !errors.Is(err, refused) {
		t.Fatalf("retained error = %v", err)
	}

	lineage := errors.New("acquisition lineage broken")
	sink = newStrictFrameSink(deliver)
	sink.RecordObservationFrameFailure(nil)
	sink.RecordObservationFrameFailure(lineage)
	sink.RecordObservationFrameFailure(errors.New("later"))
	if _, err := sink.result(); !errors.Is(err, lineage) {
		t.Fatalf("retained lineage failure = %v", err)
	}
	if frames, err := (*strictObservationFrameSink)(nil).result(); frames != 0 || err != nil {
		t.Fatal("a disabled tap reported a result")
	}
}

// deliver is the caller's code, and a callback that reads the sink's result or
// reports a failure while handling a frame must not deadlock against it.
func TestStrictObservationFrameSinkCallbackMayReenter(t *testing.T) {
	var sink *strictObservationFrameSink
	reported := errors.New("reported from the callback")
	sink = newStrictFrameSink(func(r l4bobserve.FrameRecord) error {
		if frames, err := sink.result(); err != nil || frames != int(r.Sequence) {
			t.Errorf("result inside deliver = %d, %v", frames, err)
		}
		if r.Sequence == 1 {
			sink.RecordObservationFrameFailure(reported)
		}
		return nil
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for seq := uint64(0); seq < 3; seq++ {
			_ = sink.ObserveFrame(emptyObservedFrame(seq))
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ObserveFrame deadlocked on a callback that re-entered the sink")
	}
	if frames, err := sink.result(); frames != 2 || !errors.Is(err, reported) {
		t.Fatalf("frames=%d err=%v, want 2 delivered then the reported failure retained", frames, err)
	}
}
