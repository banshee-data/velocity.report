package l9endpoints

import (
	"testing"
)

// newSuppressionTestPublisher returns a running publisher with a VRLOG replay
// marked active but no replay goroutine, so tests can drive Publish directly.
func newSuppressionTestPublisher(t *testing.T) *Publisher {
	t.Helper()
	p := NewPublisher(Config{SensorID: "test-sensor"})
	p.running.Store(true)
	t.Cleanup(func() { p.running.Store(false) })
	return p
}

func markVRLogActive(p *Publisher) {
	p.vrlogMu.Lock()
	p.vrlogActive = true
	p.vrlogMu.Unlock()
}

func drainFrames(p *Publisher) []*FrameBundle {
	var out []*FrameBundle
	for {
		select {
		case f := <-p.frameChan:
			out = append(out, f)
		default:
			return out
		}
	}
}

// TestPublishDropsLiveFramesDuringVRLogReplay covers the interleave defect:
// only background snapshots were suppressed during a VRLOG replay, so live
// foreground frames from the pipeline continued to reach the stream alongside
// the recorded ones.
func TestPublishDropsLiveFramesDuringVRLogReplay(t *testing.T) {
	p := newSuppressionTestPublisher(t)
	markVRLogActive(p)

	p.Publish(&FrameBundle{FrameID: 1, FrameType: FrameTypeFull})

	if got := drainFrames(p); len(got) != 0 {
		t.Errorf("published %d live frames during a VRLOG replay, want 0", len(got))
	}
}

// TestPublishAllowsLiveFramesWhenNoReplay verifies the gate is specific to
// replay and does not affect normal live streaming.
func TestPublishAllowsLiveFramesWhenNoReplay(t *testing.T) {
	p := newSuppressionTestPublisher(t)

	p.Publish(&FrameBundle{FrameID: 1, FrameType: FrameTypeFull})

	if got := drainFrames(p); len(got) != 1 {
		t.Errorf("published %d live frames with no replay active, want 1", len(got))
	}
}

// TestPublishAllowsReplayFramesDuringVRLogReplay verifies the replay's own
// frames still reach the stream.
func TestPublishAllowsReplayFramesDuringVRLogReplay(t *testing.T) {
	p := newSuppressionTestPublisher(t)
	markVRLogActive(p)

	p.publishReplay(&FrameBundle{FrameID: 7, FrameType: FrameTypeFull})

	got := drainFrames(p)
	if len(got) != 1 {
		t.Fatalf("published %d replay frames, want 1", len(got))
	}
	if got[0].FrameID != 7 {
		t.Errorf("FrameID = %d, want 7", got[0].FrameID)
	}
}

// TestRecorderDoesNotReceiveLiveFramesDuringVRLogReplay is the consequence that
// matters most: an attached recorder used to capture live frames arriving
// during a replay, mixing them into the recording.
func TestRecorderDoesNotReceiveLiveFramesDuringVRLogReplay(t *testing.T) {
	p := newSuppressionTestPublisher(t)
	rec := newMockRecorder()
	p.SetRecorder(rec)
	markVRLogActive(p)

	p.Publish(&FrameBundle{FrameID: 1, FrameType: FrameTypeFull})

	if frames := rec.Frames(); len(frames) != 0 {
		t.Errorf("recorder captured %d live frames during a VRLOG replay, want 0", len(frames))
	}
}
