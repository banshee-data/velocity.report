package sweep

import (
	"io"
	"log"
	"time"
)

// discardLogger returns a logger that discards all output.
func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

// scenePCAPStart returns the scene's PCAP start offset, or 0 if not set.
func scenePCAPStart(scene *HINTScene) float64 {
	if scene != nil && scene.PCAPStartSecs != nil {
		return *scene.PCAPStartSecs
	}
	return 0
}

// scenePCAPDuration returns the scene's PCAP duration, or -1 (full file) if not set.
func scenePCAPDuration(scene *HINTScene) float64 {
	if scene != nil && scene.PCAPDurationSecs != nil {
		return *scene.PCAPDurationSecs
	}
	return -1
}

// temporalIoU computes the intersection-over-union of two time intervals.
func temporalIoU(aStart, aEnd, bStart, bEnd int64) float64 {
	// Find intersection
	intStart := aStart
	if bStart > intStart {
		intStart = bStart
	}

	intEnd := aEnd
	if bEnd < intEnd {
		intEnd = bEnd
	}

	if intStart >= intEnd {
		return 0 // no overlap
	}

	intersection := float64(intEnd - intStart)

	// Find union
	unionStart := aStart
	if bStart < unionStart {
		unionStart = bStart
	}

	unionEnd := aEnd
	if bEnd > unionEnd {
		unionEnd = bEnd
	}

	union := float64(unionEnd - unionStart)

	if union == 0 {
		return 0
	}

	return intersection / union
}

// failWithError sets the state to failed with the given error message.
func (rt *HINTTuner) failWithError(errMsg string) {
	rt.logger.Printf("[hint] Failed: %s", errMsg)
	rt.mu.Lock()
	rt.state.Status = "failed"
	rt.state.Error = errMsg
	rt.mu.Unlock()
	rt.stateCond.Broadcast()

	if rt.persister != nil {
		now := time.Now()
		if err := rt.persister.SaveSweepComplete(rt.sweepID, "failed", nil, nil, nil, now, errMsg, nil, nil, nil, "", ""); err != nil {
			rt.logger.Printf("[hint] Failed to persist error: %v", err)
		}
	}
}
