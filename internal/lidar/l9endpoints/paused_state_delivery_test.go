package l9endpoints

import "testing"

// Playback state reaches a client only on a frame, and pausing stops frames —
// so the transition to paused is the one state change that can never report
// itself unless something sends a frame for it.
//
// Observed on 2026-08-31 against a live server: paused at frame 2518 of 6846,
// rate 2, while the visualiser went on showing playback. Pause looked dead
// because the server was already paused, and rate changes looked ignored
// because the new rate rides on a frame too.

func TestPausingSendsOneFrame(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})

	p.SetVRLogPaused(true)

	p.vrlogMu.Lock()
	sendOne := p.vrlogSendOneFrame
	p.vrlogMu.Unlock()

	if !sendOne {
		t.Error("pausing queued no frame; the client cannot learn it is paused")
	}
}

// Resuming needs no nudge: frames start flowing again and carry the state.
func TestResumingDoesNotQueueAFrame(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})

	p.SetVRLogPaused(true)
	p.vrlogMu.Lock()
	p.vrlogSendOneFrame = false
	p.vrlogMu.Unlock()

	p.SetVRLogPaused(false)

	p.vrlogMu.Lock()
	sendOne := p.vrlogSendOneFrame
	p.vrlogMu.Unlock()

	if sendOne {
		t.Error("resuming queued a frame; playback resumes and carries the state anyway")
	}
}

// A rate change while paused must be visible, for the same reason.
func TestRateChangeWhilePausedSendsOneFrame(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})

	p.SetVRLogPaused(true)
	p.vrlogMu.Lock()
	p.vrlogSendOneFrame = false
	p.vrlogMu.Unlock()

	p.SetVRLogRate(2.0)

	p.vrlogMu.Lock()
	sendOne := p.vrlogSendOneFrame
	p.vrlogMu.Unlock()

	if !sendOne {
		t.Error("a rate change while paused queued no frame; the change is invisible")
	}
}

// While playing, a rate change needs no nudge.
func TestRateChangeWhilePlayingQueuesNothing(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor"})

	p.SetVRLogRate(2.0)

	p.vrlogMu.Lock()
	sendOne := p.vrlogSendOneFrame
	p.vrlogMu.Unlock()

	if sendOne {
		t.Error("a rate change while playing queued a frame unnecessarily")
	}
}
