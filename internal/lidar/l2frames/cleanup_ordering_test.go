package l2frames

import (
	"sync"
	"testing"
	"time"
)

// Live frames are not delivered as they complete. A finished rotation goes into
// frameBuffer to allow for out-of-order backfill packets, and waits there until
// a cleanup tick finds it older than the buffer timeout. With a 250ms tick and a
// 500ms timeout at 10 Hz, a tick typically releases two or three rotations at
// once — and the batch used to come straight from a map range, so the pipeline
// received them in arbitrary order.
//
// The tracker's Kalman filters and every track timestamp assume time moves
// forward. Close already sorts its flush for this reason; the path every live
// frame actually takes has to do the same.
func TestCleanupDeliversFramesInCaptureOrder(t *testing.T) {
	var mu sync.Mutex
	var got []time.Time

	fb := &FrameBuilder{
		sensorID:       "order-test",
		minFramePoints: 1,
		bufferTimeout:  10 * time.Millisecond,
		// cleanupFrames reschedules itself on this interval. Left at zero the
		// rescheduled timer fires immediately and spins for the rest of the run.
		cleanupInterval: time.Hour,
		frameBuffer:     make(map[string]*LiDARFrame),
		closeCh:         make(chan struct{}),
	}
	fb.frameCallback = func(frame *LiDARFrame) {
		mu.Lock()
		got = append(got, frame.StartTimestamp)
		mu.Unlock()
	}
	fb.frameCh = make(chan *LiDARFrame, 64)
	fb.frameDone = make(chan struct{})
	go fb.frameCallbackWorker()
	t.Cleanup(fb.Close)

	// Twelve rotations, already old enough to be released by one tick. Insert
	// them under keys whose natural ordering does not match capture order, so a
	// map range cannot accidentally produce the right answer.
	base := time.Unix(1_700_000_000, 0)
	stale := time.Now().Add(-time.Second)
	for i := 0; i < 12; i++ {
		id := string(rune('z'-i)) + "-frame"
		fb.frameBuffer[id] = &LiDARFrame{
			FrameID:        id,
			SensorID:       "order-test",
			PointCount:     5000,
			StartTimestamp: base.Add(time.Duration(i) * 100 * time.Millisecond),
			EndWallTime:    stale,
		}
	}

	fb.cleanupFrames()

	// Drain the callback queue.
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n == 12 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 12 {
		t.Fatalf("delivered %d frames, want 12", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Before(got[i-1]) {
			t.Fatalf("frame %d (%v) was delivered after frame %d (%v): the pipeline received rotations out of capture order",
				i, got[i], i-1, got[i-1])
		}
	}
}
