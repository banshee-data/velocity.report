package l5tracks

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"time"
)

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

// StationaryAnchor names the point a stationary run's drift is measured from.
type StationaryAnchor string

const (
	// StationaryAnchorFirst measures drift from the track's estimate on the
	// run's first frame: the shipped rule, and the zero value's.
	StationaryAnchorFirst StationaryAnchor = "first_estimate"
	// StationaryAnchorMedian measures drift from the running median of the
	// run's estimates, x and y each, over the run's last
	// StationaryMedianWindowFrames observed frames before this one. A still
	// object whose estimate swings either side of its centre stays within a
	// swing of the median, where from the first estimate, if that sat at
	// one extreme, the other is two swings away. A steady creep drags the
	// median along half a run behind it, so it restarts the run once it has
	// gone about twice the tolerance: 2 m at 1 m, as from the first
	// estimate at 2 m.
	StationaryAnchorMedian StationaryAnchor = "running_median"
)

// StationaryRule is the stationary clock's drift rule. The zero value is the
// shipped rule: StationaryDriftMetres from the run's first estimate. The
// speed under which a track can be in a run is StationarySpeedMps under
// every rule.
type StationaryRule struct {
	// DriftMetres is how far the estimate may lie from the anchor and still
	// be in the run. Zero is StationaryDriftMetres.
	DriftMetres float32
	// Anchor is the point drift is measured from. Empty is
	// StationaryAnchorFirst.
	Anchor StationaryAnchor
}

// Drift is the tolerance the rule runs with.
func (r StationaryRule) Drift() float32 {
	if r.DriftMetres > 0 {
		return r.DriftMetres
	}
	return StationaryDriftMetres
}

// AnchorMode is the anchor the rule runs with.
func (r StationaryRule) AnchorMode() StationaryAnchor {
	if r.Anchor == "" {
		return StationaryAnchorFirst
	}
	return r.Anchor
}

// Validate refuses an anchor the clock does not know and a negative or
// non-finite tolerance, either of which would otherwise run silently as
// something else.
func (r StationaryRule) Validate() error {
	switch r.AnchorMode() {
	case StationaryAnchorFirst, StationaryAnchorMedian:
	default:
		return fmt.Errorf("unknown stationary anchor %q", r.Anchor)
	}
	if r.DriftMetres < 0 || math.IsNaN(float64(r.DriftMetres)) || math.IsInf(float64(r.DriftMetres), 0) {
		return fmt.Errorf("stationary drift %v m is not a non-negative finite distance", r.DriftMetres)
	}
	return nil
}

// observeStationary advances a track's stationary clock on a frame it was
// observed, from the state the update left. A track moving faster than
// StationarySpeedMps is not in a run. One at or under it continues its run
// while it stays within the rule's drift of the run's anchor, and otherwise
// begins a run here. Coasted frames do not touch the clock: there is no
// evidence of motion either way, and the dwell policy acts only on a track
// observed in the frame it acts.
func observeStationary(track *TrackedObject, nowNanos int64, rule StationaryRule) {
	if track.Speed() > StationarySpeedMps {
		track.stationaryRun = false
		return
	}
	median := rule.AnchorMode() == StationaryAnchorMedian
	if track.stationaryRun {
		ax, ay := track.stationaryAnchorX, track.stationaryAnchorY
		if median {
			if track.stationaryWindow == nil {
				// The rule changed during the run: its first estimate is
				// all of the run this track has kept.
				track.stationaryWindow = newStationaryWindow(StationaryMedianWindowFrames)
				track.stationaryWindow.push(ax, ay)
			}
			ax, ay = track.stationaryWindow.median()
		}
		dx, dy := track.X-ax, track.Y-ay
		drift := rule.Drift()
		if dx*dx+dy*dy <= drift*drift {
			if median {
				track.stationaryWindow.push(track.X, track.Y)
			}
			return
		}
	}
	track.stationaryRun = true
	track.stationarySinceNanos = nowNanos
	track.stationaryAnchorX, track.stationaryAnchorY = track.X, track.Y
	if median {
		if track.stationaryWindow == nil {
			track.stationaryWindow = newStationaryWindow(StationaryMedianWindowFrames)
		}
		track.stationaryWindow.reset()
		track.stationaryWindow.push(track.X, track.Y)
	}
}

// stationaryWindow is the last capacity estimates of a stationary run in
// arrival order, with each axis also kept sorted, so the per-axis median is
// read directly and a push costs O(capacity) whatever the run's length. The
// slices grow as the run does, so a short-lived track holds a short window.
type stationaryWindow struct {
	xs, ys   []float32 // arrival order, a ring once full
	next     int       // the slot the next push overwrites, once full
	sortX    []float32
	sortY    []float32
	capacity int
}

func newStationaryWindow(capacity int) *stationaryWindow {
	return &stationaryWindow{capacity: capacity}
}

func (w *stationaryWindow) reset() {
	w.xs, w.ys, w.sortX, w.sortY = w.xs[:0], w.ys[:0], w.sortX[:0], w.sortY[:0]
	w.next = 0
}

// push adds an estimate, dropping the oldest once the window is full.
func (w *stationaryWindow) push(x, y float32) {
	if len(w.xs) < w.capacity {
		w.xs, w.ys = append(w.xs, x), append(w.ys, y)
	} else {
		w.sortX = removeSorted(w.sortX, w.xs[w.next])
		w.sortY = removeSorted(w.sortY, w.ys[w.next])
		w.xs[w.next], w.ys[w.next] = x, y
		w.next = (w.next + 1) % w.capacity
	}
	w.sortX = insertSorted(w.sortX, x)
	w.sortY = insertSorted(w.sortY, y)
}

// median is the per-axis median of the window, the mean of the two middle
// values when it holds an even number. The window is never empty when read:
// a run pushes its first estimate as it begins.
func (w *stationaryWindow) median() (float32, float32) {
	return sortedMedian(w.sortX), sortedMedian(w.sortY)
}

func sortedMedian(s []float32) float32 {
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func insertSorted(s []float32, v float32) []float32 {
	i := sort.Search(len(s), func(i int) bool { return s[i] >= v })
	return slices.Insert(s, i, v)
}

// removeSorted removes one value equal to v. It matches by bits, so it finds
// the value it was given back whatever that value is.
func removeSorted(s []float32, v float32) []float32 {
	bits := math.Float32bits(v)
	i := sort.Search(len(s), func(i int) bool { return s[i] >= v })
	for j := i; j < len(s); j++ {
		if math.Float32bits(s[j]) == bits {
			return slices.Delete(s, j, j+1)
		}
	}
	for j := 0; j < i && j < len(s); j++ {
		if math.Float32bits(s[j]) == bits {
			return slices.Delete(s, j, j+1)
		}
	}
	return s
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
