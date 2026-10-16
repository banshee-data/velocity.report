package l5tracks

import "time"

// The stationary clock and the end of an absorbed track: L5's half of the
// pipeline's dwell policy (pipeline.DwellPolicy). L5 says how long a track
// has stood still; the pipeline decides when that is long enough, asks L3 to
// absorb the object's returns, and then ends the track here.

// EndAbsorbed deletes a track whose object the background model has absorbed,
// with ExpiryAbsorbed: the pipeline's dwell policy calls it once the object's
// returns are background, so the track would otherwise coast for its miss
// budget as a box over nothing and could take up a fragment of the object as
// its continuation. A deleted track never re-enters association, so it ends
// here and is not absorbed twice. It returns false, changing nothing, for a
// track that is unknown or already deleted.
func (t *Tracker) EndAbsorbed(trackID string, at time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	track, ok := t.Tracks[trackID]
	if !ok || track.TrackState == TrackDeleted {
		return false
	}
	t.deleteExpired(track, at.UnixNano(), ExpiryAbsorbed)
	return true
}

// observeStationary advances a track's stationary clock on a frame it was
// observed, from the state the update left. A track moving faster than
// StationarySpeedMps is not in a run. One at or under it continues its run
// while it stays within StationaryDriftMetres of where the run began, and
// otherwise begins a run here. Coasted frames do not touch the clock: there
// is no evidence of motion either way, and the dwell policy acts only on a
// track observed in the frame it acts.
func observeStationary(track *TrackedObject, nowNanos int64) {
	if track.Speed() > StationarySpeedMps {
		track.stationaryRun = false
		return
	}
	if track.stationaryRun {
		dx := track.X - track.stationaryAnchorX
		dy := track.Y - track.stationaryAnchorY
		if dx*dx+dy*dy <= StationaryDriftMetres*StationaryDriftMetres {
			return
		}
	}
	track.stationaryRun = true
	track.stationarySinceNanos = nowNanos
	track.stationaryAnchorX, track.stationaryAnchorY = track.X, track.Y
}

// StationaryFor is how long, at frame time atNanos, the track has been in its
// current stationary run (observeStationary): zero while it moves. It reads
// the track's own fields, so it answers on a snapshot from GetConfirmedTracks
// as on the live track.
func (track *TrackedObject) StationaryFor(atNanos int64) time.Duration {
	if !track.stationaryRun || atNanos <= track.stationarySinceNanos {
		return 0
	}
	return time.Duration(atNanos - track.stationarySinceNanos)
}
