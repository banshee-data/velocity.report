package followingreview

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

// Analysis is a recording-scoped reduction of verified following evidence.
// One minimum belongs to one software track, not an independently identified
// vehicle. Online estimates remain provisional, including non-closing pairs.
type Analysis struct {
	Schema         string         `json:"schema"`
	Metadata       Metadata       `json:"metadata"`
	Population     string         `json:"population"`
	Rows           int            `json:"rows"`
	Tracks         int            `json:"tracks"`
	WithoutTimeGap int            `json:"without_time_gap"`
	Buckets        map[string]int `json:"row_buckets"`
	Histogram      []MinimumBin   `json:"histogram"`
	Minima         []TrackMinimum `json:"minima"`
	ReviewScope    *ReviewScope   `json:"review_scope,omitempty"`
}

type MinimumBin struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type TrackMinimum struct {
	Follower string  `json:"follower_track_id"`
	Leader   string  `json:"leader_track_id"`
	Capture  string  `json:"capture_unix_nanos"`
	TimeGapS float64 `json:"minimum_time_gap_s"`
	GapM     float64 `json:"gap_m"`
}

// Analyse scans each receipt-verified minute once. It never replays packets,
// changes parameters, copies human labels, or treats record gaps as exposure.
// visit optionally receives exact original samples for a scene sidecar.
func (s *Session) Analyse(ctx context.Context, visit func([]fieldrun.EvidenceRow) error) (Analysis, error) {
	return s.AnalyseScoped(ctx, nil, visit)
}

// AnalyseScoped keeps full row/track accounting, while minima and histogram
// members require support within the declared stationary review periods.
func (s *Session) AnalyseScoped(ctx context.Context, scope *ReviewScope, visit func([]fieldrun.EvidenceRow) error) (Analysis, error) {
	filter, err := s.ReviewFilter(scope)
	if err != nil {
		return Analysis{}, err
	}
	a := Analysis{Schema: "following_analysis_v1", Metadata: s.Metadata,
		Population: "one minimum eligible positive-duration estimated net time gap per follower track per recording; provisional, not physical vehicles",
		Buckets:    map[string]int{}, Histogram: []MinimumBin{{Label: "<0.5"}, {Label: "0.5–1"}, {Label: "1–1.5"}, {Label: "1.5–2"}, {Label: "2+"}}, Minima: []TrackMinimum{}}
	a.ReviewScope = scope
	if scope != nil {
		a.Population = "one minimum eligible estimated net time gap per follower track within catalogued stationary periods, with both paths supported inside one period; denominator all software tracks in the recording, provisional, not physical vehicles"
	}
	tracks := map[string]bool{}
	minima := map[string]TrackMinimum{}
	last := map[string]int64{}
	for _, slice := range s.Metadata.Slices {
		if err := ctx.Err(); err != nil {
			return Analysis{}, err
		}
		_, rows, err := s.Evidence(slice.Start)
		if err != nil {
			return Analysis{}, err
		}
		for _, row := range rows {
			if previous, ok := last[row.FollowerTrackID]; ok && row.CaptureUnixNanos <= previous {
				return Analysis{}, fmt.Errorf("overlapping or unordered follower samples")
			}
			last[row.FollowerTrackID] = row.CaptureUnixNanos
			tracks[row.FollowerTrackID] = true
			a.Rows++
			eligible := row.Bucket == "following_eligible" && row.IntervalNanos > 0 && row.Point != nil && row.Point.TimeGap != nil && row.Point.TimeGap.ValueS != nil && row.Point.Gap != nil && row.Decision.LeaderTrackID != ""
			var timeGap, gap float64
			if eligible {
				timeGap, gap = *row.Point.TimeGap.ValueS, row.Point.Gap.ValueM
				if math.IsNaN(timeGap) || math.IsInf(timeGap, 0) || timeGap < 0 || math.IsNaN(gap) || math.IsInf(gap, 0) || gap <= 0 {
					return Analysis{}, fmt.Errorf("invalid eligible following measurement")
				}
			}
			bucket := row.Bucket
			if reason := filter(row); reason != "" {
				bucket = reason
			}
			a.Buckets[bucket]++
			if bucket != "following_eligible" || !eligible {
				continue
			}
			if old, ok := minima[row.FollowerTrackID]; !ok || timeGap < old.TimeGapS {
				minima[row.FollowerTrackID] = TrackMinimum{row.FollowerTrackID, row.Decision.LeaderTrackID, fmt.Sprint(row.CaptureUnixNanos), timeGap, gap}
			}
		}
		if visit != nil {
			if err := visit(rows); err != nil {
				return Analysis{}, err
			}
		}
	}
	if a.Rows != s.receipt.Rows {
		return Analysis{}, fmt.Errorf("analysis row accounting differs from receipt")
	}
	a.Tracks = len(tracks)
	a.WithoutTimeGap = a.Tracks - len(minima)
	for _, m := range minima {
		a.Minima = append(a.Minima, m)
		// Lower edges are inclusive; equality belongs to the next bin.
		bin := sort.Search(4, func(i int) bool { return m.TimeGapS < []float64{0.5, 1, 1.5, 2}[i] })
		a.Histogram[bin].Count++
	}
	sort.Slice(a.Minima, func(i, j int) bool { return a.Minima[i].Follower < a.Minima[j].Follower })
	return a, nil
}
