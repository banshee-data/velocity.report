package l9endpoints

import (
	"testing"
	"time"
)

// TestReplayEndNotifiesOwner covers the automatic teardown: a VRLOG that plays
// to the end of its recording announces the fact, so the owner can restore live
// input. Without this the recording stayed the pipeline's data source
// indefinitely — the live listener was never restarted, so plugging the sensor
// back in produced nothing, and the replay slot stayed claimed so no new replay
// could start.
func TestReplayEndNotifiesOwner(t *testing.T) {
	pub := newVRLogBackgroundTestPublisher(t)

	ended := make(chan struct{}, 4)
	pub.SetOnReplayEnded(func() { ended <- struct{}{} })

	// One frame, then EOF on the next read.
	reader := newMockFrameReader([]*FrameBundle{
		{FrameID: 1, FrameType: FrameTypeForeground, TimestampNanos: 1000},
	})

	// Drain so the replay loop is never blocked on a full channel.
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-pub.frameChan:
			case <-stop:
				return
			}
		}
	}()
	t.Cleanup(func() { close(stop) })

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}
	t.Cleanup(pub.StopVRLogReplay)

	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("replay reached the end of the recording without announcing it")
	}

	// The announcement fires once on arrival, not repeatedly while parked at
	// the end, or the owner would be told to return to live on every loop pass.
	select {
	case <-ended:
		t.Error("end-of-replay announced more than once")
	case <-time.After(300 * time.Millisecond):
	}
}

// TestReplayEndCallbackDoesNotDeadlock guards the reason the callback runs on
// its own goroutine: it fires from inside the replay loop, and its natural
// implementation stops the replay, which waits on that very goroutine.
func TestReplayEndCallbackDoesNotDeadlock(t *testing.T) {
	pub := newVRLogBackgroundTestPublisher(t)

	stopped := make(chan struct{})
	pub.SetOnReplayEnded(func() {
		pub.StopVRLogReplay()
		close(stopped)
	})

	reader := newMockFrameReader([]*FrameBundle{
		{FrameID: 1, FrameType: FrameTypeForeground, TimestampNanos: 1000},
	})

	drain := make(chan struct{})
	go func() {
		for {
			select {
			case <-pub.frameChan:
			case <-drain:
				return
			}
		}
	}()
	t.Cleanup(func() { close(drain) })

	if err := pub.StartVRLogReplay(reader); err != nil {
		t.Fatalf("StartVRLogReplay failed: %v", err)
	}

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stopping the replay from the end-of-replay callback deadlocked")
	}
}
