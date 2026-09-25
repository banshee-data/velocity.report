package l8analytics

import (
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// The reference contract the per-frame metrics score against, and the
// hypothesis shape they score.
//
// Why a hypothesis type rather than []*l5tracks.TrackedObject: ComputeCLEARMOT
// keys hypotheses by TrackedObject.TrackID, which is a random UUID assigned
// per run, deliberately, so it stays collision-free across tracker resets. The
// metric only ever compares those IDs for equality, so a random label does not
// change MOTA — but the cost matrix for the frame's optimal assignment is built
// in sorted ID order, so the *tie-breaking* between two equidistant hypotheses
// depends on which random UUID sorted first. Two replays of the same input
// could then report different identity switches. Scoring stored evidence keyed
// by creation_sequence, the tracker's deterministic per-run ordinal, removes
// that. ComputeCLEARMOT is kept as a thin wrapper so the imported test pins the
// matching convention unchanged.
//
// Ignore implements MOT16's distractor rule. The benchmark protocol matches
// first and only then discards hypotheses that matched an ignore-class
// annotation, so that a tracker is "neither penalized nor rewarded" for them.
// That is what the annotation sidecar's Visibility: fully_occluded and
// Completeness: partial will map onto: evidence a human could not certify, and
// so must not become a false negative.

// TrackSeries is one object's positions over time, for either side of the
// comparison. IDs are compared for equality and sorted for determinism; they
// carry no other meaning.
type TrackSeries struct {
	ID     string        `json:"id"`
	Points []SeriesPoint `json:"points"`
}

// SeriesPoint is one object's position at one frame.
type SeriesPoint struct {
	TimestampNanos int64   `json:"ts_unix_nanos"`
	X              float32 `json:"x"`
	Y              float32 `json:"y"`
	// Ignore marks a reference point that is present but uncertifiable. A
	// hypothesis matching it is removed from the tally instead of counting as
	// a false positive, and the point itself is not counted as ground truth,
	// so it can neither be missed nor found. Unused on the hypothesis side.
	Ignore bool `json:"ignore,omitempty"`
}

// TruthSource supplies reference tracks for one evidence source. The two
// implementations are the synthetic generator, which needs no labels, and the
// annotation pack reader, which needs reviewed human poses.
type TruthSource interface {
	// Name identifies the adapter in the report, so a number can never be read
	// without knowing what it was scored against.
	Name() string
	// Tracks returns the reference in a deterministic order.
	Tracks() ([]TrackSeries, error)
}

// SeriesFromGroundTruth converts the cherry-picked reference shape.
func SeriesFromGroundTruth(gt []GroundTruthTrack) []TrackSeries {
	out := make([]TrackSeries, 0, len(gt))
	for _, g := range gt {
		points := make([]SeriesPoint, 0, len(g.Points))
		for _, p := range g.Points {
			points = append(points, SeriesPoint{TimestampNanos: p.TimestampNanos, X: p.X, Y: p.Y})
		}
		out = append(out, TrackSeries{ID: g.ID, Points: points})
	}
	return out
}

// SeriesFromTracked converts live tracker objects, preserving the random
// TrackID. Offline callers scoring stored evidence should key by
// creation_sequence instead; see the note above.
func SeriesFromTracked(hyp []*l5tracks.TrackedObject) []TrackSeries {
	out := make([]TrackSeries, 0, len(hyp))
	for _, h := range hyp {
		if h == nil {
			continue
		}
		points := make([]SeriesPoint, 0, len(h.History))
		for _, p := range h.History {
			points = append(points, SeriesPoint{TimestampNanos: p.Timestamp, X: p.X, Y: p.Y})
		}
		out = append(out, TrackSeries{ID: h.TrackID, Points: points})
	}
	return out
}

// frameIndex buckets a side of the comparison by frame timestamp.
type frameIndex map[int64]map[string]seriesEntry

type seriesEntry struct {
	pos    point2
	ignore bool
}

func indexSeries(series []TrackSeries) frameIndex {
	idx := make(frameIndex)
	for _, s := range series {
		for _, p := range s.Points {
			frame := idx[p.TimestampNanos]
			if frame == nil {
				frame = make(map[string]seriesEntry)
				idx[p.TimestampNanos] = frame
			}
			frame[s.ID] = seriesEntry{pos: point2{p.X, p.Y}, ignore: p.Ignore}
		}
	}
	return idx
}

// sortedIDs returns the IDs present in a frame but not already consumed,
// sorted so the cost matrix is built identically on every run.
func sortedIDs(frame map[string]seriesEntry, used map[string]struct{}) []string {
	out := make([]string, 0, len(frame))
	for id := range frame {
		if _, ok := used[id]; !ok {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// collectFrames returns every timestamp either side mentions, ascending. The
// metrics need temporal order for continuity and identity switches.
func collectFrames(sides ...frameIndex) []int64 {
	seen := map[int64]struct{}{}
	for _, side := range sides {
		for ts := range side {
			seen[ts] = struct{}{}
		}
	}
	out := make([]int64, 0, len(seen))
	for ts := range seen {
		out = append(out, ts)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
