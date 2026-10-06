package network

import "time"

// PacingAnchor is the clock a paced replay measures itself against.
//
// A single-file replay anchors on its own first packet: capture time and wall
// time both start at zero when playback begins. A sequence cannot do that. If
// every file re-anchored, each join would reset the pacer's idea of "now", and
// the reader would sprint through the opening of the next file until the two
// clocks agreed again — which is exactly the burst that floods the frame
// builder and breaks tracks at a join.
//
// So the sequence anchors once, on the first step, and every later step
// measures against that same origin. Because consecutive captures abut in
// capture time, the elapsed capture time keeps growing across the join and the
// pacing stays continuous through it.
//
// A nil anchor means "anchor on this file", which is what a single-file replay
// wants and what every existing caller gets.
type PacingAnchor struct {
	// CaptureStart is the capture timestamp playback is measured from: the
	// sequence's effective start, after any StartSeconds offset.
	CaptureStart time.Time
	// WallStart is the wall-clock instant that capture timestamp was played at.
	WallStart time.Time
	// CumulativeYield is the total time the pacer has slept in backoff across
	// every step so far. It is subtracted from elapsed wall time, so it has to
	// carry across steps too — otherwise a later step reads the earlier steps'
	// yields as lateness and enters backoff it has not earned.
	CumulativeYield time.Duration
	// set records that the anchor holds a real origin. A zero CaptureStart is
	// indistinguishable from "not yet anchored" without it.
	set bool
}

// Anchored reports whether an origin has been established.
func (a *PacingAnchor) Anchored() bool { return a != nil && a.set }

// Anchor fixes the origin, if it is not already fixed. It is safe to call on a
// nil anchor, which does nothing: a single-file replay keeps its own clock.
func (a *PacingAnchor) Anchor(captureStart, wallStart time.Time) {
	if a == nil || a.set {
		return
	}
	a.CaptureStart = captureStart
	a.WallStart = wallStart
	a.set = true
}

// AddYield accumulates backoff sleep so later steps inherit it.
func (a *PacingAnchor) AddYield(d time.Duration) {
	if a == nil {
		return
	}
	a.CumulativeYield += d
}
