package l9endpoints

import (
	"testing"
	"time"
)

// settlingBackgroundManager reports a settling state the test controls.
type settlingBackgroundManager struct {
	seq     uint64
	settled bool
}

func (m *settlingBackgroundManager) GenerateBackgroundSnapshot() (interface{}, error) {
	return &BackgroundSnapshot{SequenceNumber: m.seq, X: []float32{1}, Y: []float32{2}, Z: []float32{3}}, nil
}
func (m *settlingBackgroundManager) GetBackgroundSequenceNumber() uint64 { return m.seq }
func (m *settlingBackgroundManager) IsSettlingComplete() bool            { return m.settled }

// Until the grid settles it is empty, so the snapshot already sent to a client
// carries no points. Waiting for the next interval then leaves the client
// showing foreground and boxes over nothing.
//
// That gap used to be invisible because settling took about as long as the
// interval. Now that a grid can settle on convergence in six seconds, it is
// most of half a minute: on 2026-08-27 settling finished at 5.9s and the
// background did not arrive until 24s later.
func TestBackgroundIsSentWhenSettlingCompletes(t *testing.T) {
	mgr := &settlingBackgroundManager{}
	p := NewPublisher(Config{SensorID: "test-sensor", BackgroundInterval: time.Minute})
	p.backgroundMgr = mgr

	// A snapshot has already gone out during warm-up, well inside the interval.
	p.backgroundMu.Lock()
	p.lastBackgroundSent = time.Now()
	p.backgroundMu.Unlock()

	if p.shouldSendBackground() {
		t.Fatal("sent a background mid-interval while still settling")
	}

	mgr.settled = true

	if !p.shouldSendBackground() {
		t.Error("no background sent when settling completed; the client renders foreground over an empty scene until the interval elapses")
	}
}

// The transition fires once, not on every frame after settling.
func TestSettlingBackgroundFiresOnlyOnTheTransition(t *testing.T) {
	mgr := &settlingBackgroundManager{settled: true}
	p := NewPublisher(Config{SensorID: "test-sensor", BackgroundInterval: time.Minute})
	p.backgroundMgr = mgr

	p.backgroundMu.Lock()
	p.lastBackgroundSent = time.Now()
	p.backgroundMu.Unlock()

	if !p.shouldSendBackground() {
		t.Fatal("the settled transition did not fire")
	}
	if p.shouldSendBackground() {
		t.Error("fired again while still settled; the background would be resent every frame")
	}
}

// A manager that does not model settling is unaffected.
func TestPublisherToleratesAManagerWithoutSettling(t *testing.T) {
	p := NewPublisher(Config{SensorID: "test-sensor", BackgroundInterval: time.Minute})
	p.backgroundMgr = &seqBackgroundManager{seq: 7}

	p.backgroundMu.Lock()
	p.lastBackgroundSent = time.Now()
	p.lastBackgroundSeq = 7
	p.backgroundMu.Unlock()

	if p.shouldSendBackground() {
		t.Error("sent a background for a manager that reports no settling state")
	}
}

// A replay carries its own recorded background and settles nothing. Reporting
// the live grid's warm-up over it put "SETTLING 0s" on the badge against a
// scene that was already complete.
func TestSettlingIsNotStampedOnReplayFrames(t *testing.T) {
	tests := []struct {
		mode      string
		wantStamp bool
	}{
		{"live", true},
		{"vrlog", false},
		{"pcap", false},
		{"pcap_analysis", false},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			s := NewServer(nil)
			s.SetSourceModeProvider(func() (string, bool) { return tt.mode, false })
			s.SetSettlingProvider(func() (bool, float32) { return true, 0 })

			settling, _ := s.currentSettling()
			mode, _ := s.currentSourceMode()
			stamped := mode == "live" && settling

			if stamped != tt.wantStamp {
				t.Errorf("source %q: stamped=%v, want %v", tt.mode, stamped, tt.wantStamp)
			}
		})
	}
}
