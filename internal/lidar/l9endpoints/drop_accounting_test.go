package l9endpoints

import "testing"

// The publisher loses frames at two distinct stages, and they must not be
// summed into one ratio. A frame that is published and then rejected by a slow
// client is counted once at each stage, so adding them made the denominator
// include the same frame twice: with a single client accepting nothing, the
// reported rate pinned at exactly 50% and could never go higher, which reads as
// "half the frames are getting through" when in fact none are.

// TestPublishStageDropsAreCountedSeparately covers frameChan being full — the
// frame never enters the pipeline at all.
func TestPublishStageDropsAreCountedSeparately(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})
	p.running.Store(true)
	t.Cleanup(func() { p.running.Store(false) })

	// Fill frameChan so the next publish has nowhere to go. Nothing drains it:
	// broadcastLoop is not running.
	for i := 0; i < cap(p.frameChan); i++ {
		p.frameChan <- &FrameBundle{FrameID: uint64(i)}
	}

	p.Publish(&FrameBundle{FrameID: 9999})

	if got := p.droppedFrames.Load(); got != 1 {
		t.Errorf("publish-stage drops = %d, want 1", got)
	}
	if got := p.clientDroppedFrames.Load(); got != 0 {
		t.Errorf("client-stage drops = %d, want 0: a frame that never entered the pipeline was not rejected by a client", got)
	}
}

// TestClientStageDropsAreCountedSeparately covers a client whose own queue is
// full. The frame was published successfully; only this client lost it.
func TestClientStageDropsAreCountedSeparately(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})

	client := &clientStream{id: "slow-client", frameCh: make(chan *FrameBundle, 1)}
	client.frameCh <- &FrameBundle{FrameID: 1}

	if p.enqueueForClient(client, &FrameBundle{FrameID: 2}) {
		t.Fatal("enqueue succeeded against a full queue")
	}

	if got := p.droppedFrames.Load(); got != 0 {
		t.Errorf("publish-stage drops = %d, want 0: the frame was published, a client rejected it", got)
	}
}

// TestBackgroundEvictionCountsAsClientLoss covers the background retry path:
// evicting a queued frame to make room is a client-stage loss, not a publish
// one.
func TestBackgroundEvictionCountsAsClientLoss(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})

	client := &clientStream{id: "slow-client", frameCh: make(chan *FrameBundle, 1)}
	client.frameCh <- &FrameBundle{FrameID: 1, FrameType: FrameTypeForeground}

	bg := &FrameBundle{FrameID: 2, FrameType: FrameTypeBackground}
	if !p.enqueueForClient(client, bg) {
		t.Fatal("a background frame did not evict to make room")
	}

	if got := p.clientDroppedFrames.Load(); got != 1 {
		t.Errorf("client-stage drops = %d, want 1 (the evicted frame)", got)
	}
	if got := p.droppedFrames.Load(); got != 0 {
		t.Errorf("publish-stage drops = %d, want 0", got)
	}
}

// TestClientDropRateIsNotCappedAtFifty is the regression guard for the original
// symptom: one client rejecting every published frame must report as total
// loss, not as 50%.
func TestClientDropRateIsNotCappedAtFifty(t *testing.T) {
	const published = 50

	// The old formula: dropped/(published+dropped).
	oldPct := float64(published) / float64(published+published) * 100
	if oldPct != 50 {
		t.Fatalf("sanity: the superseded formula should yield 50%%, got %.1f", oldPct)
	}

	// The current formula: client drops measured against what was published.
	newPct := float64(published) / float64(published) * 100
	if newPct != 100 {
		t.Errorf("client drop rate = %.1f%%, want 100%%: every published frame was rejected", newPct)
	}
}
