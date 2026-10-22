package l9endpoints

import (
	"testing"
)

// A recording should open with the background, so a replay of it has a scene
// from its first frame rather than foreground over nothing until the refresh
// interval happens to fire. Recordings inherit whatever order they were
// captured in: one on disk carries its background at frame 2 and another at
// frame 116, and the second is the one that looks broken.
func TestRecordingOpensWithABackgroundSnapshot(t *testing.T) {
	pub := NewPublisher(Config{SensorID: "test-sensor", BackgroundEveryFrames: 600})
	pub.backgroundMgr = &settlingBackgroundManager{seq: 3, settled: true}
	pub.running.Store(true)

	// A background inherits the most recent foreground timestamp, so one has to
	// have been seen. In this flow the settling pass has already run, which is
	// what makes the snapshot available at the moment recording starts.
	pub.lastForegroundTimestamp.Store(1_000_000_000)

	rec := &capturingRecorder{}
	pub.SetRecorder(rec)

	if err := pub.SendBackgroundSnapshot(); err != nil {
		t.Fatalf("SendBackgroundSnapshot: %v", err)
	}

	// The snapshot reaches the recorder directly, ahead of the client channel,
	// which is what puts it first in the file.
	if len(rec.frames) == 0 {
		t.Fatal("nothing was recorded; a recording started here would open with foreground")
	}
	first := rec.frames[0]
	if first.FrameType != FrameTypeBackground {
		t.Errorf("first recorded frame is %v, want a background", first.FrameType)
	}
	if first.Background == nil {
		t.Error("the first recorded frame carries no background snapshot")
	}
}

// A replay owns the stream, so a live snapshot must not be injected into one.
func TestNoBackgroundSnapshotDuringReplay(t *testing.T) {
	pub := NewPublisher(Config{SensorID: "test-sensor", BackgroundEveryFrames: 600})
	pub.backgroundMgr = &settlingBackgroundManager{seq: 3, settled: true}
	pub.running.Store(true)
	rec := &capturingRecorder{}
	pub.SetRecorder(rec)
	pub.vrlogMu.Lock()
	pub.vrlogActive = true
	pub.vrlogMu.Unlock()

	if err := pub.SendBackgroundSnapshot(); err != nil {
		t.Fatalf("SendBackgroundSnapshot: %v", err)
	}

	if len(rec.frames) != 0 {
		t.Fatalf("recorded %d frames during a replay; the replay owns the stream", len(rec.frames))
	}
}

// capturingRecorder stands in for a VRLOG recorder.
type capturingRecorder struct{ frames []*FrameBundle }

func (r *capturingRecorder) Record(frame *FrameBundle) error {
	r.frames = append(r.frames, frame)
	return nil
}

// Before any foreground frame the background has no timestamp to inherit, so
// the snapshot is deferred rather than written with a meaningless one. Recording
// from a cold pipeline therefore cannot open with a background, which is why the
// settle-before-recording flow runs a settling pass first.
func TestBackgroundSnapshotIsDeferredBeforeAnyForegroundFrame(t *testing.T) {
	pub := NewPublisher(Config{SensorID: "test-sensor", BackgroundEveryFrames: 600})
	pub.backgroundMgr = &settlingBackgroundManager{seq: 3, settled: true}
	pub.running.Store(true)

	rec := &capturingRecorder{}
	pub.SetRecorder(rec)

	if err := pub.SendBackgroundSnapshot(); err != nil {
		t.Fatalf("SendBackgroundSnapshot: %v", err)
	}
	if len(rec.frames) != 0 {
		t.Errorf("recorded %d frames with no foreground timestamp to inherit", len(rec.frames))
	}
}
