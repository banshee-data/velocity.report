// Package pcapsplit provides motion/static scene classification for LiDAR
// captures. Its core is a tag-free timeline builder that collapses per-frame
// motion samples into sustained motion and static periods using hysteresis.
//
// The package is consumed in two places:
//
//   - the --motion flag of pcap-analyse (velocity lidar pcap-analyse) reports
//     the timeline for an existing capture without writing any files.
//   - pcap-split (velocity lidar pcap-split) reuses the same classification to
//     cut a capture into per-segment PCAP files.
//
// Only the pure classification logic lives in this file; it has no libpcap
// dependency and is exercised by the default `go test ./...` suite. The PCAP
// read/write glue is built behind the `pcap` build tag in sibling files.
package pcapsplit

import "time"

// Period type labels used in MotionPeriod.Type and emitted in JSON/CSV output.
const (
	// MotionLabel marks a period during which the sensor was moving.
	MotionLabel = "motion"
	// StaticLabel marks a period during which the sensor was stationary.
	StaticLabel = "static"
)

// Default hysteresis thresholds. These mirror the pcap-split design defaults:
// a long settling requirement avoids over-segmenting on transient foreground
// (passing vehicles, wind-blown foliage), while a short motion trigger reacts
// promptly when the platform actually departs.
const (
	// DefaultSettlingSec is the sustained stability required to declare static.
	DefaultSettlingSec = 60.0
	// DefaultMotionTriggerSec is the sustained motion required to declare motion.
	DefaultMotionTriggerSec = 5.0
)

// FrameSample is a single per-frame motion observation. Moving is the raw
// detector result for that frame (e.g. BackgroundManager.CheckForSensorMovement);
// hysteresis is applied later by BuildTimeline, not by the caller.
type FrameSample struct {
	// T is the capture timestamp of the frame.
	T time.Time
	// Moving is the raw (un-smoothed) motion classification for the frame.
	Moving bool
}

// MotionPeriod is a contiguous run of frames sharing one classification.
// StartSecs/EndSecs are relative to the first sample; StartFrame/EndFrame are
// 0-based frame indices; StartTime/EndTime carry the absolute capture times so a
// splitter can map periods back to packets. For inner periods EndFrame/EndTime
// equal the next period's start; for the final period they are the last frame.
type MotionPeriod struct {
	Type         string    `json:"type"` // MotionLabel or StaticLabel
	StartSecs    float64   `json:"start_secs"`
	EndSecs      float64   `json:"end_secs"`
	DurationSecs float64   `json:"duration_secs"`
	StartFrame   int       `json:"start_frame"`
	EndFrame     int       `json:"end_frame"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
}

// TimelineConfig holds the hysteresis thresholds for BuildTimeline, plus two
// optional post-processing knobs.
type TimelineConfig struct {
	// SettlingSec is the sustained stability (seconds) required to transition
	// motion -> static. Short stops below this stay within the motion period,
	// which naturally bridges intersection waits.
	SettlingSec float64
	// MotionTriggerSec is the sustained motion (seconds) required to transition
	// static -> motion. Brief foreground spikes below this are ignored.
	MotionTriggerSec float64
	// MaxMotionGapSec, when > 0, additionally bridges any static period shorter
	// than this that is adjacent to motion, folding it into the motion segment.
	MaxMotionGapSec float64
	// MinSegmentSec, when > 0, merges any period shorter than this into a
	// neighbour so the output has no micro-segments.
	MinSegmentSec float64
}

// DefaultTimelineConfig returns the standard hysteresis thresholds with
// post-processing disabled (the raw-hysteresis view used by pcap-analyse).
func DefaultTimelineConfig() TimelineConfig {
	return TimelineConfig{
		SettlingSec:      DefaultSettlingSec,
		MotionTriggerSec: DefaultMotionTriggerSec,
	}
}

// BuildTimeline collapses time-ordered per-frame samples into motion/static
// periods. A state change is only committed once the contradicting signal has
// been sustained for the relevant threshold (SettlingSec for motion->static,
// MotionTriggerSec for static->motion); the cut is then dated to the moment the
// sustained run began, giving precise boundaries. Samples are assumed sorted by
// T. Returns nil for an empty input.
func BuildTimeline(samples []FrameSample, cfg TimelineConfig) []MotionPeriod {
	if len(samples) == 0 {
		return nil
	}

	settling := secsToDuration(cfg.SettlingSec)
	motionTrigger := secsToDuration(cfg.MotionTriggerSec)

	type transition struct {
		t      time.Time
		frame  int
		moving bool
	}

	state := samples[0].Moving
	transitions := []transition{{t: samples[0].T, frame: 0, moving: state}}

	var pendingSince time.Time
	var pendingFrame int
	pending := false

	for i, s := range samples {
		if s.Moving == state {
			// Signal agrees with the current state: any in-progress
			// contradiction was transient, so reset the candidate timer.
			pending = false
			continue
		}

		if !pending {
			pending = true
			pendingSince = s.T
			pendingFrame = i
		}

		// The threshold depends on the direction of the pending change.
		threshold := motionTrigger // static -> motion
		if state {
			threshold = settling // motion -> static
		}

		if s.T.Sub(pendingSince) >= threshold {
			state = !state
			transitions = append(transitions, transition{t: pendingSince, frame: pendingFrame, moving: state})
			pending = false
		}
	}

	t0 := samples[0].T
	end := samples[len(samples)-1].T
	lastFrame := len(samples) - 1

	periods := make([]MotionPeriod, 0, len(transitions))
	for i, tr := range transitions {
		segEnd := end
		segEndFrame := lastFrame
		if i+1 < len(transitions) {
			segEnd = transitions[i+1].t
			segEndFrame = transitions[i+1].frame
		}
		startSecs := tr.t.Sub(t0).Seconds()
		endSecs := segEnd.Sub(t0).Seconds()
		periods = append(periods, MotionPeriod{
			Type:         label(tr.moving),
			StartSecs:    startSecs,
			EndSecs:      endSecs,
			DurationSecs: endSecs - startSecs,
			StartFrame:   tr.frame,
			EndFrame:     segEndFrame,
			StartTime:    tr.t,
			EndTime:      segEnd,
		})
	}
	return refineTimeline(periods, cfg)
}

// label maps a moving flag to its period label.
func label(moving bool) string {
	if moving {
		return MotionLabel
	}
	return StaticLabel
}

// secsToDuration converts a non-negative seconds value to a Duration.
func secsToDuration(secs float64) time.Duration {
	if secs <= 0 {
		return 0
	}
	return time.Duration(secs * float64(time.Second))
}
