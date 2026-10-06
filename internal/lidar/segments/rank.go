package segments

import (
	"fmt"
	"sort"
)

// measure is a number that a finder's pass computes for each window, and the
// finders whose pass computes it.
type measure struct {
	finders []string
	read    func(Window) float64
}

var followingPass = []string{"following", "leader_changes"}

// measures is the closed catalogue a selector file may name. Adding one is a
// change to the code: the file composes what the finders compute, and cannot
// compute anything new.
var measures = map[string]measure{
	"pair_frames":    {followingPass, func(w Window) float64 { return float64(w.PairFrames) }},
	"pair_seconds":   {followingPass, func(w Window) float64 { return w.PairSeconds }},
	"pairs":          {followingPass, func(w Window) float64 { return float64(w.Pairs) }},
	"followers":      {followingPass, func(w Window) float64 { return float64(w.Followers) }},
	"leaders":        {followingPass, func(w Window) float64 { return float64(w.Leaders) }},
	"leader_changes": {followingPass, func(w Window) float64 { return float64(w.LeaderChanges) }},
	"closest_gap_m":  {followingPass, func(w Window) float64 { return w.ClosestGapM }},
	"tracks": {[]string{"following", "leader_changes", "lateral_jump", "split_flags", "exposure"},
		func(w Window) float64 { return float64(len(w.TrackIDs)) }},
	"events": {[]string{"lateral_jump", "split_flags", "exposure"}, func(w Window) float64 { return float64(w.Events) }},
	// A lateral jump window's own score is its largest residual, and a random
	// window's is its draw. Only those finders compute them.
	"max_residual_m": {[]string{"lateral_jump"}, func(w Window) float64 { return w.Score }},
	"draw":           {[]string{"random"}, func(w Window) float64 { return w.Score }},
}

// nativeMeasure is the measure each finder ranks by on its own.
var nativeMeasure = map[string]string{
	"following":      "pair_frames",
	"leader_changes": "leader_changes",
	"lateral_jump":   "max_residual_m",
	"split_flags":    "events",
	"exposure":       "events",
	"random":         "draw",
}

// Rank ranks a source's windows as a selector defines: its finder at its
// parameters, then the windows that meet its requirements, ordered by its
// measure and, among equals, by time. The peak frame and the windows a
// finder leaves out are the finder's own. A standard selector returns
// exactly what Find returns.
func Rank(points []Point, s Selector, source, role string, captures []Capture) ([]Window, error) {
	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("selector %q: %w", s.ID, err)
	}
	if role == "held_out" && !s.HeldOut() {
		return nil, fmt.Errorf("selector %q cannot choose a held_out window: only a standard traffic or random selector, at its default parameters, can", s.ID)
	}
	windows, err := Find(points, s.Finder, source, role, s.Parameters, captures)
	if err != nil {
		return nil, err
	}
	read := measures[s.Score.Measure].read
	kept := windows[:0]
	for _, w := range windows {
		// The bounds read the window as the finder scored it, before its
		// score becomes the selector's.
		if !s.admits(w) {
			continue
		}
		w.Score = read(w)
		kept = append(kept, w)
	}
	descending := s.Score.Order == "descending"
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].Score != kept[j].Score {
			return (kept[i].Score > kept[j].Score) == descending
		}
		return kept[i].StartNs < kept[j].StartNs
	})
	return kept, nil
}

func (s Selector) admits(w Window) bool {
	for _, r := range s.Require {
		v := measures[r.Measure].read(w)
		if (r.Min != nil && v < *r.Min) || (r.Max != nil && v > *r.Max) {
			return false
		}
	}
	return true
}
