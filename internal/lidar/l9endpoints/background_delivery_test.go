package l9endpoints

import "testing"

// Background frames carry scene state that no later frame reproduces: the client
// renders the last one it received until another arrives. Two places used to
// discard them, and either one leaves the previous source's settled grid on
// screen underneath the new source's foreground.

// TestCoalescingNeverDiscardsABackgroundFrame covers the replay catch-up path.
// vrlogMode turns coalescing on at the start of a VRLOG load — exactly when the
// background clear and the recording's own background are published — so the
// two frames that establish the new scene were the ones being dropped.
func TestCoalescingNeverDiscardsABackgroundFrame(t *testing.T) {
	bg := &FrameBundle{FrameID: 2, FrameType: FrameTypeBackground, Background: &BackgroundSnapshot{SequenceNumber: 7}}

	t.Run("background queued behind foreground survives", func(t *testing.T) {
		ch := make(chan *FrameBundle, 10)
		ch <- bg
		ch <- &FrameBundle{FrameID: 3, FrameType: FrameTypeForeground}
		ch <- &FrameBundle{FrameID: 4, FrameType: FrameTypeForeground}

		got, _ := coalesceBufferedFrames(ch, &FrameBundle{FrameID: 1, FrameType: FrameTypeForeground})
		if got.FrameType != FrameTypeBackground {
			t.Fatalf("coalescing skipped past the background frame, returning frame %d (%v)", got.FrameID, got.FrameType)
		}
		// The newer foreground frames must still be queued for the next pass,
		// not consumed alongside it.
		if len(ch) != 2 {
			t.Errorf("queue has %d frames, want 2 still pending", len(ch))
		}
	})

	t.Run("a background already in hand is not coalesced away", func(t *testing.T) {
		ch := make(chan *FrameBundle, 10)
		ch <- &FrameBundle{FrameID: 3, FrameType: FrameTypeForeground}

		got, skipped := coalesceBufferedFrames(ch, bg)
		if got.FrameType != FrameTypeBackground {
			t.Errorf("held background was replaced by frame %d", got.FrameID)
		}
		if skipped != 0 {
			t.Errorf("skipped = %d, want 0", skipped)
		}
	})

	t.Run("foreground still coalesces to the newest", func(t *testing.T) {
		ch := make(chan *FrameBundle, 10)
		for i := 2; i <= 5; i++ {
			ch <- &FrameBundle{FrameID: uint64(i), FrameType: FrameTypeForeground}
		}
		got, skipped := coalesceBufferedFrames(ch, &FrameBundle{FrameID: 1, FrameType: FrameTypeForeground})
		if got.FrameID != 5 {
			t.Errorf("got frame %d, want the newest (5)", got.FrameID)
		}
		if skipped != 4 {
			t.Errorf("skipped = %d, want 4", skipped)
		}
	})
}

// TestSlowClientStillReceivesBackgroundFrames covers the other loss path: the
// publisher drops frames for a client whose queue is full. During replay
// start-up the queue fills quickly, so a background could be dropped before it
// ever reached the stream.
func TestSlowClientStillReceivesBackgroundFrames(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})
	client := &clientStream{id: "slow", frameCh: make(chan *FrameBundle, 2)}

	// Fill the client's queue.
	for i := 0; i < 2; i++ {
		if !p.enqueueForClient(client, &FrameBundle{FrameID: uint64(i), FrameType: FrameTypeForeground}) {
			t.Fatalf("setup: frame %d was refused before the queue was full", i)
		}
	}

	// A foreground frame is legitimately dropped: a later one supersedes it.
	if p.enqueueForClient(client, &FrameBundle{FrameID: 9, FrameType: FrameTypeForeground}) {
		t.Error("a full queue accepted a foreground frame; back-pressure is not working")
	}

	// A background frame must get through by evicting the oldest queued frame.
	bg := &FrameBundle{FrameID: 10, FrameType: FrameTypeBackground, Background: &BackgroundSnapshot{SequenceNumber: 7}}
	if !p.enqueueForClient(client, bg) {
		t.Fatal("background frame dropped for a slow client: the client keeps rendering the previous scene")
	}

	var delivered bool
	for len(client.frameCh) > 0 {
		if f := <-client.frameCh; f.FrameType == FrameTypeBackground {
			delivered = true
		}
	}
	if !delivered {
		t.Error("background frame never reached the client queue")
	}
}
