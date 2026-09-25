package l8analytics

import (
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// CLEAR MOT (Bernardin & Stiefelhagen, 2008) multi-object tracking metrics.
//
// This computes MOTA (accuracy) and MOTP (precision) by matching tracker output
// (hypotheses) against ground-truth tracks frame by frame. The matching follows
// the MOT16 / py-motmetrics convention:
//
//  1. Continuity: re-use the previous frame's correspondences when both objects
//     are still present and within the distance threshold (minimises identity
//     switches).
//  2. Optimal assignment: match the remaining ground-truth and hypotheses with a
//     minimum-distance Hungarian assignment, gated by the threshold.
//
// Per frame it tallies false negatives (unmatched GT), false positives
// (unmatched hypotheses), and identity switches (a GT matched to a different
// hypothesis than the last one it was matched to). Then:
//
//	MOTA = 1 - (FN + FP + IDSW) / sum(GT_t)      (may be negative)
//	MOTP = sum(matched distance) / matches        (mean Euclidean error, metres)
//
// The exact formulae and edge-case handling follow the widely-reproduced MOT16
// protocol; the primary Bernardin (2008) source is tracked as gap M4 for a
// future re-verification.

// clearMOTForbidden is the sentinel cost for out-of-gate pairs. The l5tracks
// Hungarian solver treats costs >= 1e18 as forbidden; we use the same value and
// additionally re-check the assigned distance against the gate, because the
// solver may assign a forbidden cell to complete the (padded) matching.
const clearMOTForbidden = float32(1e18)

// GroundTruthPoint is a ground-truth object position at one frame.
type GroundTruthPoint struct {
	TimestampNanos int64   `json:"ts_unix_nanos"`
	X              float32 `json:"x"`
	Y              float32 `json:"y"`
}

// GroundTruthTrack is the labelled trajectory of one object across frames.
type GroundTruthTrack struct {
	ID     string             `json:"id"`
	Points []GroundTruthPoint `json:"points"`
}

// CLEARMOTResult holds the computed CLEAR MOT metrics and their constituent
// counts.
type CLEARMOTResult struct {
	MOTA       float64 `json:"mota"`
	MOTP       float64 `json:"motp"`
	NumFrames  int     `json:"num_frames"`
	NumGT      int     `json:"num_gt"` // total GT presences summed over frames
	FN         int     `json:"fn"`
	FP         int     `json:"fp"`
	IDSwitches int     `json:"id_switches"`
	Matches    int     `json:"matches"`
}

type point2 struct {
	x, y float32
}

func euclid(a, b point2) float64 {
	dx := float64(a.x - b.x)
	dy := float64(a.y - b.y)
	return math.Sqrt(dx*dx + dy*dy)
}

// ComputeCLEARMOT evaluates the hypotheses against the ground truth over the
// given evaluation frame timestamps, using maxDistMeters as the association
// gate. Objects are matched to a frame by exact timestamp; ground-truth or
// hypothesis points whose timestamp is not in frameTimestamps are ignored.
func ComputeCLEARMOT(gt []GroundTruthTrack, hyp []*l5tracks.TrackedObject, frameTimestamps []int64, maxDistMeters float64) CLEARMOTResult {
	// Delegates to ComputeTrackMetrics (perframe.go), which implements the same
	// MOT16 rule over the neutral TrackSeries shape and adds fragmentation and
	// the ignore rule. This file's test is the pin: it asserts the convention
	// documented above, unmodified from where it was written, so the two
	// cannot drift apart silently.
	//
	// One difference is deliberate. This entry point evaluates exactly the
	// frames the caller names; ComputeTrackMetrics evaluates every frame either
	// side mentions. Filtering here preserves the documented behaviour that
	// points outside frameTimestamps are ignored.
	wanted := make(map[int64]struct{}, len(frameTimestamps))
	for _, ts := range frameTimestamps {
		wanted[ts] = struct{}{}
	}
	filter := func(series []TrackSeries) []TrackSeries {
		out := make([]TrackSeries, 0, len(series))
		for _, s := range series {
			points := make([]SeriesPoint, 0, len(s.Points))
			for _, p := range s.Points {
				if _, ok := wanted[p.TimestampNanos]; ok {
					points = append(points, p)
				}
			}
			out = append(out, TrackSeries{ID: s.ID, Points: points})
		}
		return out
	}

	m := ComputeTrackMetrics(filter(SeriesFromGroundTruth(gt)), filter(SeriesFromTracked(hyp)), maxDistMeters)
	return CLEARMOTResult{
		MOTA: m.MOTA, MOTP: m.MOTP,
		// NumFrames is the caller's evaluation window, which includes frames
		// where neither side had anything and the shared matcher never visits.
		NumFrames:  len(frameTimestamps),
		NumGT:      m.NumGT,
		FN:         m.FN,
		FP:         m.FP,
		IDSwitches: m.IDSwitches,
		Matches:    m.Matches,
	}
}

// sortedRemaining returns the IDs present in m but not in used, sorted for
// deterministic cost-matrix construction.
func sortedRemaining(m map[string]point2, used map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for id := range m {
		if _, ok := used[id]; !ok {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
