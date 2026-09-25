package l5tracks

// The estimator's time domain.
//
// Every temporal quantity in this package derives from capture time: the
// timestamp the caller passes to Update or AdvanceMisses (the pipeline passes
// LiDARFrame.StartTimestamp) and the acquisition time each WorldCluster
// carries in TSUnixNanos. That covers the prediction interval, a track's
// coast age and capture-time expiry, the deleted-track grace period and
// render fade, history and heading-episode timestamps, track duration, and
// LastMeasurementUnixNanos. Nothing here reads the host's wall clock, and
// TestL5TracksDoesNotReadTheWallClock scans the non-test sources to keep it
// that way.
//
// The property this buys is the one replay depends on: the same capture fed
// at any pace, on any machine, produces the same tracks. Wall time belongs to
// the operator and runtime concerns outside this package: replay pacing, the
// replay throttle, frame-builder cleanup, benchmark timing, logs and audit
// fields. Those may decide which frames reach the tracker; they never decide
// how much time the tracker believes has passed. The full boundary, and the
// L1 timestamp modes that feed it, are described in
// docs/lidar/architecture/time-domain-model.md.
//
// Two clocks exist inside the domain and are kept distinct. The frame clock
// is the timestamp passed to Update; LastUpdateNanos holds it. The state
// clock is the capture time a track's Kalman state refers to,
// StateUnixNanos. By default they are the same instant for every track after
// each frame. Under MeasurementTimePrediction an associated track's state is
// moved to its measurement's acquisition time, so the two diverge by the
// intra-frame offset at which the object was scanned.

// maxGapPredictionSteps bounds CaptureGapPrediction's sub-stepping. At the
// default MaxPredictDt of 0.5 s it covers ten minutes of capture. A gap that
// long is a restart or a clock step rather than an occlusion; expiry, not
// prediction, is the right response to it, and an unbounded loop over a
// clock that jumped by years would stall the frame. A longer interval is
// clamped to the cap before any track is predicted, exactly as MaxPredictDt
// clamps it by default, and the frame is counted once in
// TimeDomainStats.TruncatedGapPredictions.
const maxGapPredictionSteps = 1200

// TimeDomainStats describes the capture-time stream the tracker consumed.
// It is diagnostic only: nothing in the estimator reads it back, so it cannot
// change a track. Gaps are unclamped capture-time intervals between
// consecutive Update calls, in seconds.
type TimeDomainStats struct {
	// Frames counts Update calls; AdvancedFrames counts AdvanceMisses calls,
	// which age tracks without predicting them.
	Frames         int64 `json:"frames"`
	AdvancedFrames int64 `json:"advanced_frames"`

	// DuplicateTimestamps counts frames stamped with the previous frame's
	// capture time. Their prediction interval is zero, as it always was.
	DuplicateTimestamps int64 `json:"duplicate_timestamps"`
	// BackwardTimestamps counts frames stamped earlier than the previous
	// frame. Their prediction interval is forced to zero rather than run
	// backwards; MaxBackwardStepSecs is the largest such step.
	BackwardTimestamps  int64   `json:"backward_timestamps"`
	MaxBackwardStepSecs float64 `json:"max_backward_step_secs"`

	// LastGapSecs and MaxGapSecs are the most recent and the largest forward
	// gap. ClampedGaps counts gaps longer than MaxPredictDt: without
	// CaptureGapPrediction those frames predicted only MaxPredictDt of the
	// interval, so a coasting track lagged its object by the rest.
	LastGapSecs float64 `json:"last_gap_secs"`
	MaxGapSecs  float64 `json:"max_gap_secs"`
	ClampedGaps int64   `json:"clamped_gaps"`
	// TruncatedGapPredictions counts frames whose CaptureGapPrediction gap
	// exceeded maxGapPredictionSteps × MaxPredictDt and was clamped to it; the
	// remainder went unpredicted. Counted once per frame, not per track.
	TruncatedGapPredictions int64 `json:"truncated_gap_predictions"`

	// MeasurementIntervalsClamped counts, under MeasurementTimePrediction,
	// per-track intervals that would have been negative (a measurement or
	// frame stamped before the time the track's state already referred to)
	// and were forced to zero. The state is never predicted backwards.
	MeasurementIntervalsClamped int64 `json:"measurement_intervals_clamped"`

	// ExpiredByMisses and ExpiredByCoastAge count deletions by rule. The
	// frame-count rule is checked first where both apply in one call.
	ExpiredByMisses   int64 `json:"expired_by_misses"`
	ExpiredByCoastAge int64 `json:"expired_by_coast_age"`
}

// TimeDomainStats returns a snapshot of the capture-time diagnostics.
func (t *Tracker) TimeDomainStats() TimeDomainStats {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.timeStats
}

// frameInterval converts a frame's capture time into the prediction interval
// and records the unclamped gap. Caller holds the lock and has not yet moved
// LastUpdateNanos.
//
// The first frame has no predecessor and keeps the historical 0.1 s default.
// A duplicate timestamp yields zero, as it always did. A timestamp earlier
// than the previous frame's also yields zero: before this rule a backwards
// step became a negative interval, which ran every track backwards and
// subtracted process noise from its covariance, and a covariance that has had
// noise subtracted can stop being positive definite. The reference is then
// re-anchored at the new timestamp by the caller, as it always was, so a
// stepped clock resumes ordinary intervals on the next frame instead of
// freezing prediction until capture time regains the old high-water mark.
//
// The arithmetic for the ordinary case is deliberately the expression the
// tracker has always used, float32 of the nanosecond difference divided in
// float32: a result one ulp away would move every committed baseline.
func (t *Tracker) frameInterval(nowNanos int64) float32 {
	t.timeStats.Frames++
	var dt float32
	hasPrevious := t.LastUpdateNanos > 0
	if hasPrevious {
		deltaNanos := nowNanos - t.LastUpdateNanos
		switch {
		case deltaNanos == 0:
			t.timeStats.DuplicateTimestamps++
			t.timeStats.LastGapSecs = 0
			return 0
		case deltaNanos < 0:
			step := float64(-deltaNanos) / 1e9
			t.timeStats.BackwardTimestamps++
			t.timeStats.LastGapSecs = 0
			if step > t.timeStats.MaxBackwardStepSecs {
				t.timeStats.MaxBackwardStepSecs = step
			}
			tracef("Backward capture timestamp: step=%.6fs prediction interval forced to zero", step)
			return 0
		}
		gap := float64(deltaNanos) / 1e9
		t.timeStats.LastGapSecs = gap
		if gap > t.timeStats.MaxGapSecs {
			t.timeStats.MaxGapSecs = gap
		}
		dt = float32(deltaNanos) / 1e9
	} else {
		dt = 0.1 // Default 100ms for first frame
	}
	// Clamp dt to MaxPredictDt so throttle-induced gaps (e.g. 250 ms at
	// 12 fps cap) don't create an inflated time step for association gating.
	// Predict() also clamps independently, but the raw dt flows into
	// associate() where it affects implied-speed plausibility checks
	// (task 7.1). CaptureGapPrediction keeps a real gap whole instead, so both
	// the prediction and the plausibility check span the time that passed.
	if dt > t.Config.MaxPredictDt {
		if !hasPrevious || !t.Config.CaptureGapPrediction {
			dt = t.Config.MaxPredictDt
		} else if limit := t.gapPredictionLimit(); dt > limit {
			dt = limit
			t.timeStats.TruncatedGapPredictions++
		}
		if hasPrevious {
			t.timeStats.ClampedGaps++
			tracef("Capture gap exceeds max_predict_dt: gap=%.3fs max_predict_dt=%.3fs capture_gap_prediction=%t",
				t.timeStats.LastGapSecs, t.Config.MaxPredictDt, t.Config.CaptureGapPrediction)
		}
	}
	return dt
}

// trackInterval is the interval Step 1 predicts one track across. By default
// it is the frame interval for every track. Under MeasurementTimePrediction it
// runs from the time the track's state refers to, which for a track updated
// last frame is its measurement time rather than that frame's start.
func (t *Tracker) trackInterval(track *TrackedObject, frameDt float32, nowNanos int64) float32 {
	if !t.Config.MeasurementTimePrediction || track.StateUnixNanos <= 0 {
		return frameDt
	}
	return t.stateInterval(track, nowNanos)
}

// stateInterval is the capture time from a track's state to targetNanos,
// never negative, clamped to MaxPredictDt unless CaptureGapPrediction is set.
func (t *Tracker) stateInterval(track *TrackedObject, targetNanos int64) float32 {
	deltaNanos := targetNanos - track.StateUnixNanos
	if deltaNanos < 0 {
		t.timeStats.MeasurementIntervalsClamped++
		return 0
	}
	dt := float32(deltaNanos) / 1e9
	if dt > t.Config.MaxPredictDt {
		if !t.Config.CaptureGapPrediction {
			dt = t.Config.MaxPredictDt
		} else if limit := t.gapPredictionLimit(); dt > limit {
			dt = limit
		}
	}
	return dt
}

// gapPredictionLimit is the longest interval CaptureGapPrediction predicts
// across: maxGapPredictionSteps sub-steps of MaxPredictDt. Intervals are
// clamped to it before any track is predicted, so predictSpan always covers
// the whole interval it is given. The part of a gap beyond the limit is
// dropped at the frame, counted once, and each state re-anchored at the frame
// time, just as a MaxPredictDt clamp is by default.
func (t *Tracker) gapPredictionLimit() float32 {
	return float32(maxGapPredictionSteps) * t.Config.MaxPredictDt
}

// predictSpan predicts a track across dt. Ordinarily that is one predict()
// call, which clamps to MaxPredictDt itself. Under CaptureGapPrediction a
// longer interval is covered in steps of at most MaxPredictDt, so the
// covariance cap and the finite-state guard act per step exactly as they do
// across the same interval of ordinary frames.
func (t *Tracker) predictSpan(track *TrackedObject, dt float32) {
	step := t.Config.MaxPredictDt
	if !t.Config.CaptureGapPrediction || dt <= step || step <= 0 {
		t.predict(track, dt)
		return
	}
	// The callers clamp dt to gapPredictionLimit, so at most
	// maxGapPredictionSteps whole steps are needed. The bound is exact: when a
	// step that is not a binary fraction leaves float residue after the last
	// step, that residue (nanoseconds) is not worth another predict.
	remaining := dt
	for n := 0; remaining > 0 && track.TrackState != TrackDeleted && n < maxGapPredictionSteps; n++ {
		s := step
		if remaining < s {
			s = remaining
		}
		t.predict(track, s)
		remaining -= s
	}
}

// coastAgeNanos is how long, in capture time, a track has gone without an
// accepted observation. A clock that stepped backwards reads as zero rather
// than negative.
func coastAgeNanos(track *TrackedObject, nowNanos int64) int64 {
	if track.LastObservedUnixNanos <= 0 || nowNanos <= track.LastObservedUnixNanos {
		return 0
	}
	return nowNanos - track.LastObservedUnixNanos
}

// maxCoastSecs is the capture-time bound for a lifecycle state, or zero when
// the bound is disabled.
func (t *Tracker) maxCoastSecs(state TrackState) float32 {
	if state == TrackConfirmed {
		return t.Config.MaxCoastSecsConfirmed
	}
	return t.Config.MaxCoastSecsTentative
}

// observeCoastAge refreshes a track's coast age at nowNanos and reports
// whether it has reached its capture-time bound.
func (t *Tracker) observeCoastAge(track *TrackedObject, nowNanos int64) (expired bool) {
	age := float32(coastAgeNanos(track, nowNanos)) / 1e9
	track.CoastAgeSecs = age
	bound := t.maxCoastSecs(track.TrackState)
	return bound > 0 && age >= bound
}

// markObserved records an accepted observation at the capture time the
// track's posterior now refers to, and the unobserved interval it closes.
func markObserved(track *TrackedObject) {
	observed := track.StateUnixNanos
	if track.LastObservedUnixNanos > 0 && observed > track.LastObservedUnixNanos {
		if gap := float32(observed-track.LastObservedUnixNanos) / 1e9; gap > track.MaxCoastAgeSecs {
			track.MaxCoastAgeSecs = gap
		}
	}
	if observed > track.LastObservedUnixNanos {
		track.LastObservedUnixNanos = observed
	}
	track.CoastAgeSecs = 0
}

// deleteExpired marks a track deleted at nowNanos and counts the rule.
func (t *Tracker) deleteExpired(track *TrackedObject, nowNanos int64, byCoastAge bool) {
	prevState := track.TrackState
	track.TrackState = TrackDeleted
	track.EndUnixNanos = nowNanos
	if byCoastAge {
		t.timeStats.ExpiredByCoastAge++
		diagf("Track deleted after capture-time coast: track_id=%s previous_state=%s coast_age=%.3fs bound=%.3fs misses=%d",
			track.TrackID, prevState, track.CoastAgeSecs, t.maxCoastSecs(prevState), track.Misses)
		return
	}
	t.timeStats.ExpiredByMisses++
}
