package l8analytics

import (
	"fmt"
	"math"
	"sort"
)

// Scripted scenarios with known per-frame truth.
//
// The annotation packs that will eventually supply real reference poses carry
// none yet: FrameMask.Pose is defined and validated but no client writes it,
// and there are zero reviewed masks. Until that changes, the only truth that
// exists is truth we construct. These scenarios are deliberately the cases
// where the right answer is fixed by the geometry — two objects crossing,
// one object occluded — so a metric can be checked against a number rather
// than against a plausible-looking chart.
//
// They also make an association claim testable end to end. Feeding the real
// tracker these object positions as clusters and scoring what comes back is
// the difference between "the likelihood cost should reduce identity switches"
// and a count.

// defaultScenarioBaseNanos is an arbitrary but non-zero wall clock. Zero would
// work for the metrics, which only compare timestamps, but the tracker treats a
// zero LastUpdateNanos as "no previous frame".
const defaultScenarioBaseNanos = int64(1_700_000_000_000_000_000)

// SyntheticScenario is a set of objects with exactly known positions.
type SyntheticScenario struct {
	Name string
	// FramePeriodNanos is the interval between frames.
	FramePeriodNanos int64
	// BaseUnixNanos is the absolute time of frame zero. Truth and whatever
	// drives the tracker have to agree on it, or nothing matches: the metrics
	// key on exact timestamps.
	BaseUnixNanos int64
	// Objects are the true trajectories. Extent is carried so a caller can
	// build plausible clusters from them.
	Objects []SyntheticObject
}

// SyntheticObject is one object's true path.
type SyntheticObject struct {
	ID string
	// LengthMetres and WidthMetres describe the body, for callers building
	// clusters or boxes. They do not affect the truth positions.
	LengthMetres, WidthMetres float64
	// Positions are per frame, starting at FirstFrame. A frame where the
	// object is absent (occluded, out of view) is marked Absent.
	FirstFrame int
	Positions  []SyntheticPosition
}

// SyntheticPosition is one object's true state at one frame.
type SyntheticPosition struct {
	X, Y float64
	// Absent means the object produced no return this frame. It is excluded
	// from the truth entirely rather than marked Ignore: the object genuinely
	// was not observable, so a tracker cannot be faulted for missing it and
	// must not be credited for inventing it.
	Absent bool
}

// Truth converts the scenario to the reference shape the metrics score against.
func (s SyntheticScenario) Truth() []TrackSeries {
	out := make([]TrackSeries, 0, len(s.Objects))
	for _, obj := range s.Objects {
		series := TrackSeries{ID: obj.ID}
		for i, p := range obj.Positions {
			if p.Absent {
				continue
			}
			series.Points = append(series.Points, SeriesPoint{
				TimestampNanos: s.FrameTimestamp(obj.FirstFrame + i),
				X:              float32(p.X), Y: float32(p.Y),
			})
		}
		out = append(out, series)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// FrameTimestamp is the absolute time of a frame index.
func (s SyntheticScenario) FrameTimestamp(frame int) int64 {
	return s.BaseUnixNanos + int64(frame)*s.FramePeriodNanos
}

// FrameCount is the number of frames the scenario spans.
func (s SyntheticScenario) FrameCount() int {
	last := 0
	for _, obj := range s.Objects {
		if end := obj.FirstFrame + len(obj.Positions); end > last {
			last = end
		}
	}
	return last
}

// At returns the objects present at a frame index, in a stable order.
func (s SyntheticScenario) At(frame int) []SyntheticObjectState {
	var out []SyntheticObjectState
	for _, obj := range s.Objects {
		idx := frame - obj.FirstFrame
		if idx < 0 || idx >= len(obj.Positions) || obj.Positions[idx].Absent {
			continue
		}
		out = append(out, SyntheticObjectState{
			ID: obj.ID, X: obj.Positions[idx].X, Y: obj.Positions[idx].Y,
			LengthMetres: obj.LengthMetres, WidthMetres: obj.WidthMetres,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SyntheticObjectState is one object at one frame.
type SyntheticObjectState struct {
	ID                        string
	X, Y                      float64
	LengthMetres, WidthMetres float64
}

// CrossingScenario is two objects on converging straight paths that pass
// within `closestMetres` of each other at the midpoint, then separate. This is
// the case an association cost decides: at the crossing both hypotheses are
// nearly equidistant from both objects, and whichever the cost prefers sets
// whether identities survive.
//
// Speeds are in metres per frame so the geometry stays exact.
func CrossingScenario(frames int, closestMetres, speedPerFrame float64) SyntheticScenario {
	if frames < 2 {
		frames = 2
	}
	half := float64(frames-1) / 2
	a := SyntheticObject{ID: "north", LengthMetres: 4.5, WidthMetres: 1.9}
	b := SyntheticObject{ID: "south", LengthMetres: 4.5, WidthMetres: 1.9}
	for i := 0; i < frames; i++ {
		t := float64(i) - half
		// Both travel along +x; their y converges to ±closest/2 at the middle
		// and separates again, so they are closest exactly at the midpoint.
		spread := closestMetres/2 + math.Abs(t)*0.35
		a.Positions = append(a.Positions, SyntheticPosition{X: t * speedPerFrame, Y: spread})
		b.Positions = append(b.Positions, SyntheticPosition{X: t * speedPerFrame, Y: -spread})
	}
	return SyntheticScenario{
		Name:             fmt.Sprintf("crossing_%dframes_%.2fm", frames, closestMetres),
		FramePeriodNanos: 100_000_000,
		BaseUnixNanos:    defaultScenarioBaseNanos,
		Objects:          []SyntheticObject{a, b},
	}
}

// OcclusionScenario is one object travelling in a straight line that stops
// producing returns for `gapFrames` in the middle. A tracker that coasts and
// re-acquires scores no identity switch; one that gives up and starts again
// scores one switch and one fragmentation.
func OcclusionScenario(frames, gapStart, gapFrames int, speedPerFrame float64) SyntheticScenario {
	obj := SyntheticObject{ID: "occluded", LengthMetres: 4.5, WidthMetres: 1.9}
	for i := 0; i < frames; i++ {
		obj.Positions = append(obj.Positions, SyntheticPosition{
			X:      float64(i) * speedPerFrame,
			Y:      0,
			Absent: i >= gapStart && i < gapStart+gapFrames,
		})
	}
	return SyntheticScenario{
		Name:             fmt.Sprintf("occlusion_%dframes_gap%d", frames, gapFrames),
		FramePeriodNanos: 100_000_000,
		BaseUnixNanos:    defaultScenarioBaseNanos,
		Objects:          []SyntheticObject{obj},
	}
}

// ContestedScenario is the case gap-analysis row S3 describes, built so the
// bias it names can actually fire.
//
// Two objects travel in parallel, `separationMetres` apart. One of them stops
// producing returns for a while, so its track coasts and its covariance
// inflates. While it coasts, the other object's cluster is the only one nearby.
// Under a bare Mahalanobis cost the coasting track pays less for that cluster
// than the track that has been following it all along, so it can take it — and
// taking it is an identity switch on a reference that was never occluded.
//
// A crossing alone does not test this: while both objects keep producing
// returns, no track coasts, so the discount never applies. That is why the
// crossing scenario shows no difference between the two association costs.
func ContestedScenario(frames int, separationMetres float64, gapStart, gapFrames int, speedPerFrame float64) SyntheticScenario {
	steady := SyntheticObject{ID: "steady", LengthMetres: 4.5, WidthMetres: 1.9}
	vanishing := SyntheticObject{ID: "vanishing", LengthMetres: 4.5, WidthMetres: 1.9}
	for i := range frames {
		x := float64(i) * speedPerFrame
		steady.Positions = append(steady.Positions, SyntheticPosition{X: x, Y: 0})
		vanishing.Positions = append(vanishing.Positions, SyntheticPosition{
			X: x, Y: separationMetres,
			Absent: i >= gapStart && i < gapStart+gapFrames,
		})
	}
	return SyntheticScenario{
		Name:             fmt.Sprintf("contested_sep%.1fm_gap%d", separationMetres, gapFrames),
		FramePeriodNanos: 100_000_000,
		BaseUnixNanos:    defaultScenarioBaseNanos,
		Objects:          []SyntheticObject{steady, vanishing},
	}
}

// ConvergingContestedScenario is ContestedScenario with the bias given its best
// chance to fire.
//
// In the parallel version the surviving object's own track sits almost exactly
// on its cluster, so however cheap the coasting track's bid is, the fresh one's
// is cheaper still and no contest occurs. Here the vanishing object is closing
// on the other lane when it disappears, at a rate that carries its coasted
// prediction onto the surviving object by the time the gap ends. Both tracks
// then predict the same place, the surviving object's cluster is the only one
// on offer, and the assignment has to choose between a freshly updated track
// and a coasting one whose covariance has been inflated for gapFrames.
//
// This is the geometry row S3 describes. If a bare Mahalanobis cost never loses
// an identity even here, it does not lose one in this scene class at all.
func ConvergingContestedScenario(frames int, separationMetres float64, gapStart, gapFrames int, speedPerFrame float64) SyntheticScenario {
	steady := SyntheticObject{ID: "steady", LengthMetres: 4.5, WidthMetres: 1.9}
	closing := SyntheticObject{ID: "closing", LengthMetres: 4.5, WidthMetres: 1.9}
	// Close the lateral gap over the occlusion, so the coasted prediction lands
	// on the steady object exactly as it reappears.
	drift := separationMetres / float64(gapFrames)
	for i := range frames {
		x := float64(i) * speedPerFrame
		steady.Positions = append(steady.Positions, SyntheticPosition{X: x, Y: 0})

		y := separationMetres
		if i > gapStart-gapFrames {
			// Begin closing one gap-length before vanishing, so the velocity the
			// filter has learned is the one that carries it across.
			y = separationMetres - drift*float64(i-(gapStart-gapFrames))
		}
		if y < 0 {
			y = 0
		}
		closing.Positions = append(closing.Positions, SyntheticPosition{
			X: x, Y: y, Absent: i >= gapStart && i < gapStart+gapFrames,
		})
	}
	return SyntheticScenario{
		Name:             fmt.Sprintf("converging_sep%.1fm_gap%d", separationMetres, gapFrames),
		FramePeriodNanos: 100_000_000,
		BaseUnixNanos:    defaultScenarioBaseNanos,
		Objects:          []SyntheticObject{steady, closing},
	}
}
