package l9endpoints

import (
	"sync"
	"testing"
	"time"
)

// countingRecorder counts the background snapshots written to a recording.
type countingRecorder struct {
	mu          sync.Mutex
	backgrounds []uint64 // the rotation count at which each was written
	rotations   uint64
}

func (r *countingRecorder) Record(f *FrameBundle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f.FrameType == FrameTypeBackground {
		r.backgrounds = append(r.backgrounds, r.rotations)
	} else {
		r.rotations++
	}
	return nil
}

// publishRotations publishes n foreground rotations through the live path,
// which records each one, and waits pause between them.
func publishRotations(p *Publisher, n int, pause time.Duration) {
	for i := 0; i < n; i++ {
		f := &FrameBundle{
			FrameID: uint64(i), TimestampNanos: int64(1_000_000_000 + i*100_000_000),
			SensorID: "test-sensor", FrameType: FrameTypeForeground,
		}
		p.Publish(f)
		if pause > 0 {
			time.Sleep(pause)
		}
	}
}

func newCadencePublisher(t *testing.T, every int) (*Publisher, *countingRecorder) {
	t.Helper()
	p := NewPublisher(Config{SensorID: "test-sensor", BackgroundEveryFrames: every})
	p.running.Store(true)
	t.Cleanup(func() { p.running.Store(false) })
	p.SetBackgroundManager(&liveBackgroundManager{})
	rec := &countingRecorder{}
	p.SetRecorder(rec)
	// Drain like a connected client, so the channel never decides the count.
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		for {
			select {
			case <-p.frameChan:
			case <-stop:
				return
			}
		}
	}()
	return p, rec
}

// The periodic background falls on the same rotations however fast the
// rotations arrive. On a wall-clock interval, a replay at analysis speed, 1x
// and 0.5x recorded 30, 69 and 136 snapshots for identical rotations, and an
// export that counted records cut each at a different rotation.
func TestBackgroundSnapshotsFallOnTheSameRotationsAtAnySpeed(t *testing.T) {
	fast, fastRec := newCadencePublisher(t, 10)
	publishRotations(fast, 35, 0)

	slow, slowRec := newCadencePublisher(t, 10)
	publishRotations(slow, 35, 2*time.Millisecond)

	fastRec.mu.Lock()
	slowRec.mu.Lock()
	defer fastRec.mu.Unlock()
	defer slowRec.mu.Unlock()
	// The first waits for a foreground timestamp, so it lands after rotation
	// 1 is published; the refresh then falls every tenth rotation.
	want := []uint64{1, 11, 21, 31}
	if len(fastRec.backgrounds) != len(want) || len(slowRec.backgrounds) != len(want) {
		t.Fatalf("backgrounds at rotations %v (fast) and %v (slow), want %v for both",
			fastRec.backgrounds, slowRec.backgrounds, want)
	}
	for i := range want {
		if fastRec.backgrounds[i] != want[i] || slowRec.backgrounds[i] != want[i] {
			t.Errorf("background %d at rotation %d (fast) and %d (slow), want %d",
				i, fastRec.backgrounds[i], slowRec.backgrounds[i], want[i])
		}
	}
}

// A full broadcast channel does not make the next rotation send, and record,
// the same snapshot again: how fast clients drain must not decide how many
// snapshots a recording holds.
func TestBackgroundSnapshotRecordedOnceWhenTheChannelIsFull(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor", BackgroundEveryFrames: 1000})
	p.running.Store(true)
	t.Cleanup(func() { p.running.Store(false) })
	p.SetBackgroundManager(&liveBackgroundManager{})
	rec := &countingRecorder{}
	p.SetRecorder(rec)
	for len(p.frameChan) < cap(p.frameChan) {
		p.frameChan <- &FrameBundle{}
	}

	publishRotations(p, 20, 0)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.backgrounds) != 1 {
		t.Errorf("recorded %d backgrounds over 20 rotations with the channel full, want 1 (the first)", len(rec.backgrounds))
	}
}

// Rotations replayed from a VRLOG are the recording's own, not the pipeline's,
// and do not advance the live refresh.
func TestReplayedFramesDoNotAdvanceTheBackgroundCadence(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor", BackgroundEveryFrames: 5})
	p.running.Store(true)
	t.Cleanup(func() { p.running.Store(false) })
	for i := 0; i < 20; i++ {
		p.publishInternal(&FrameBundle{FrameID: uint64(i), TimestampNanos: 1, FrameType: FrameTypeForeground}, true)
	}
	p.backgroundMu.Lock()
	defer p.backgroundMu.Unlock()
	if p.framesSinceBackground != 0 {
		t.Errorf("framesSinceBackground = %d after 20 replayed frames, want 0", p.framesSinceBackground)
	}
}
