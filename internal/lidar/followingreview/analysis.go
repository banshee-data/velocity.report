package followingreview

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

// Analysis is a recording-scoped reduction of verified following evidence.
// One minimum belongs to one software track, not an independently identified
// vehicle. Online estimates remain provisional, including non-closing pairs.
type Analysis struct {
	Schema           string         `json:"schema"`
	Metadata         Metadata       `json:"metadata"`
	Population       string         `json:"population"`
	Rows             int            `json:"rows"`
	Tracks           int            `json:"tracks"`
	WithoutTimeGap   int            `json:"without_time_gap"`
	Buckets          map[string]int `json:"row_buckets"`
	Histogram        []MinimumBin   `json:"histogram"`
	Minima           []TrackMinimum `json:"minima"`
	ReviewScope      *ReviewScope   `json:"review_scope,omitempty"`
	ExperimentalDRAC *DRACSummary   `json:"experimental_drac,omitempty"`
}

// DRACSummary accounts for positive-duration follower intervals. Each
// episode is a contiguous sequence of supported intervals for the same
// follower and leader. Bins count episode peaks in [0,1), [1,2), [2,4), [4,∞).
type DRACSummary struct {
	Schema          string                  `json:"schema"`
	Exposure        map[string]DRACExposure `json:"exposure_by_status"`
	TerminalSamples int                     `json:"terminal_samples"`
	EpisodePeakBins [4]int                  `json:"episode_peak_bins"`
}

type DRACExposure struct {
	Samples       int   `json:"samples"`
	IntervalNanos int64 `json:"interval_nanos"`
}

type dracEpisode struct {
	leader  string
	end     int64
	peakBin int
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
	var episodes map[string]dracEpisode
	if scope != nil {
		a.Population = "one minimum eligible estimated net time gap per follower track within catalogued stationary periods, with both paths supported inside one period; denominator all software tracks in the recording, provisional, not physical vehicles"
		a.ExperimentalDRAC = &DRACSummary{Schema: "following_scoped_drac_v1", Exposure: map[string]DRACExposure{}}
		episodes = map[string]dracEpisode{}
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
			reason := filter(row)
			if reason != "" {
				bucket = reason
			}
			if a.ExperimentalDRAC != nil {
				if err := a.ExperimentalDRAC.record(row, reason, episodes); err != nil {
					return Analysis{}, err
				}
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

func (d *DRACSummary) record(row fieldrun.EvidenceRow, scopeReason string, episodes map[string]dracEpisode) error {
	if row.IntervalNanos == 0 {
		d.TerminalSamples++
		return nil
	}
	status := row.DRAC.Status
	switch {
	case row.Bucket == "record_gap":
		status = "record_gap"
	case scopeReason != "":
		status = scopeReason
	case status == "":
		status = "missing_drac_status"
	}
	exposure := d.Exposure[status]
	exposure.Samples++
	exposure.IntervalNanos += row.IntervalNanos
	d.Exposure[status] = exposure

	leader := row.Decision.LeaderTrackID
	if status != string(l8behaviour.DRACSupported) {
		delete(episodes, row.FollowerTrackID)
		return nil
	}
	if row.DRAC.Calculation == nil || row.DRAC.Calculation.ValueMPS2 == nil || leader == "" {
		return fmt.Errorf("supported DRAC lacks a value or leader for sample %d", row.CaptureUnixNanos)
	}
	value := *row.DRAC.Calculation.ValueMPS2
	bin := sort.Search(3, func(i int) bool { return value < [...]float64{1, 2, 4}[i] })
	previous, ok := episodes[row.FollowerTrackID]
	if ok && previous.leader == leader && previous.end == row.CaptureUnixNanos {
		d.EpisodePeakBins[previous.peakBin]--
		bin = max(bin, previous.peakBin)
	}
	d.EpisodePeakBins[bin]++
	episodes[row.FollowerTrackID] = dracEpisode{leader: leader, end: row.CaptureUnixNanos + row.IntervalNanos, peakBin: bin}
	return nil
}
