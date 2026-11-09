package network

import "time"

// WindowAction is what a ReplayWindow says to do with one packet.
type WindowAction int

const (
	// WindowSkip: the packet is before the window; read on.
	WindowSkip WindowAction = iota
	// WindowKeep: the packet is inside the window.
	WindowKeep
	// WindowStop: the packet is past the window; stop reading this file.
	WindowStop
)

// ReplayWindow is the capture-time window ReadPCAPFile applies to one file,
// held as a value so that a second reader of the same file — pcap-split's
// writer, for instance — admits exactly the packets the replay did.
//
// The window is anchored on the first packet offered to Admit, which must be
// the file's first packet after any BPF filter and packet-offset seek:
//
//   - start = first + startSeconds when startSeconds > 0, else first;
//   - end   = start + durationSeconds when durationSeconds > 0, else none.
//
// Packets before start are skipped until the first packet at or after it; from
// then on the window is open, so a later out-of-order packet earlier than
// start is kept. The first packet strictly after end stops the read, and
// nothing after it is admitted. A packet exactly at end is kept: the window is
// [start, end] in capture time, closed at both ends.
type ReplayWindow struct {
	startSeconds    float64
	durationSeconds float64

	anchored bool
	skipping bool
	start    time.Time
	end      time.Time
}

// NewReplayWindow returns the window for one file. startSeconds <= 0 means the
// first packet; durationSeconds <= 0 means to the end of the file.
func NewReplayWindow(startSeconds, durationSeconds float64) *ReplayWindow {
	return &ReplayWindow{
		startSeconds:    startSeconds,
		durationSeconds: durationSeconds,
		skipping:        startSeconds > 0,
	}
}

// Skipping reports whether the window has yet to open: every packet offered so
// far was before start, or none has been offered.
func (w *ReplayWindow) Skipping() bool { return w.skipping }

// Admit decides what to do with the packet captured at t. Packets must be
// offered in file order.
func (w *ReplayWindow) Admit(t time.Time) WindowAction {
	if !w.anchored {
		w.anchored = true
		w.start = t
		if w.startSeconds > 0 {
			w.start = t.Add(time.Duration(w.startSeconds * float64(time.Second)))
		}
		if w.durationSeconds > 0 {
			w.end = w.start.Add(time.Duration(w.durationSeconds * float64(time.Second)))
		}
	}
	if w.skipping {
		if t.Before(w.start) {
			return WindowSkip
		}
		w.skipping = false
	}
	if !w.end.IsZero() && t.After(w.end) {
		return WindowStop
	}
	return WindowKeep
}
