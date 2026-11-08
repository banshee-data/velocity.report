// Package motion holds the offline motion-model estimators of the state
// estimation plan's Phase 4: constant velocity (CV), constant acceleration
// (CA) with its coast rule, and an interacting multiple model (IMM) over the
// two.
//
// Offline only. The plan's Section 7.3 prototypes CA and IMM against stored
// observations and gates any move from CV behind G-EST-1. Nothing in this
// package is called by the live tracker, it reads no tuning key, and it
// changes no TrackerConfig default. Its input is a track's observation
// sequence (Observation), which ObservationsFromFilterFrames builds from the
// live tracker's filter-step record (l5tracks.FilterFrame); its output is a
// per-step record a G-EST-1 analysis reads (internal/lidar/l8analytics).
//
// Every estimator works in one six-state space per track,
//
//	x = [x, y, vx, vy, ax, ay]
//
// so a CV and a CA estimate of the same track are comparable row for row and
// an IMM can mix them like with like. CV is the same space with acceleration
// held at zero: its transition maps acceleration to zero and its process noise
// gives acceleration no variance, so after any CV prediction the acceleration
// rows and columns of P are exactly zero.
//
// Arithmetic is float64 throughout. The live tracker's float32 state is
// widened on the way in and never narrowed.
//
// Time is capture time in Unix nanoseconds carried on each observation. The
// package does not import the time package and never reads the host clock, so
// a replay's result cannot depend on its pacing. TestMotionDoesNotReadTheClock
// holds the rule.
//
// The mathematics, with references, is in data/maths/motion-models-maths.md.
package motion
