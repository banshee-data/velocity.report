package followingreview

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
)

// ReviewScope preserves the catalogue evidence behind a restricted review.
// Periods are half-open capture-time intervals, not new physical calibration.
type ReviewScope struct {
	Schema     string         `json:"schema"`
	CatalogSHA string         `json:"catalog_sha256"`
	Site       string         `json:"site"`
	Periods    []ReviewPeriod `json:"periods"`
}

type ReviewPeriod struct {
	Start int64 `json:"start_ns,string"`
	End   int64 `json:"end_ns,string"`
}

const ReviewScopeSchema = "following_static_review_v1"

// StaticReviewScope intersects the existing site catalogue's stationary
// periods with this recording. Gaps are retained, never bridged by a new pose.
func StaticReviewScope(data []byte, site string, first, end int64) (*ReviewScope, error) {
	var catalog []struct {
		ID    string `json:"id"`
		Parts []struct {
			Start time.Time `json:"start"`
			End   time.Time `json:"end"`
		} `json:"static_parts"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}
	if first >= end || site == "" {
		return nil, fmt.Errorf("review needs a site and a positive recording interval")
	}
	scope := &ReviewScope{Schema: ReviewScopeSchema, CatalogSHA: digest(data), Site: site}
	found := false
	for _, entry := range catalog {
		if entry.ID != site {
			continue
		}
		if found {
			return nil, fmt.Errorf("duplicate catalogue site %q", site)
		}
		found = true
		for _, p := range entry.Parts {
			a, b := p.Start.UnixNano(), p.End.UnixNano()
			if !p.Start.Equal(time.Unix(0, a)) || !p.End.Equal(time.Unix(0, b)) || a >= b {
				return nil, fmt.Errorf("invalid stationary period for %q", site)
			}
			a, b = max(a, first), min(b, end)
			if a < b {
				scope.Periods = append(scope.Periods, ReviewPeriod{a, b})
			}
		}
	}
	sort.Slice(scope.Periods, func(i, j int) bool { return scope.Periods[i].Start < scope.Periods[j].Start })
	var merged []ReviewPeriod
	for _, p := range scope.Periods {
		if n := len(merged); n > 0 && p.Start <= merged[n-1].End {
			merged[n-1].End = max(merged[n-1].End, p.End)
		} else {
			merged = append(merged, p)
		}
	}
	scope.Periods = merged
	if len(merged) == 0 {
		return nil, fmt.Errorf("no catalogued stationary coverage for %q", site)
	}
	return scope, nil
}

func (s *ReviewScope) period(first, last int64) int {
	for i, p := range s.Periods {
		if first >= p.Start && first <= last && last < p.End {
			return i + 1
		}
	}
	return 0
}

// ReviewFilter admits only intervals and both actors' complete path support
// within one catalogued period. It never trims a path into a different fit.
func (s *Session) ReviewFilter(scope *ReviewScope) (func(fieldrun.EvidenceRow) string, error) {
	if scope == nil {
		return func(fieldrun.EvidenceRow) string { return "" }, nil
	}
	if scope.Schema != ReviewScopeSchema || scope.CatalogSHA == "" || scope.Site != s.Metadata.Binding.SiteID || len(scope.Periods) == 0 {
		return nil, fmt.Errorf("incompatible stationary review scope")
	}
	for i, p := range scope.Periods {
		if p.Start < s.Metadata.Start || p.End > s.Metadata.End || p.Start >= p.End || (i > 0 && p.Start <= scope.Periods[i-1].End) {
			return nil, fmt.Errorf("invalid stationary review periods")
		}
	}
	paths := map[string]int{}
	for _, p := range s.paths {
		if p.Path != nil {
			paths[p.FollowerTrackID] = scope.period(p.First, p.Last)
		}
	}
	return func(row fieldrun.EvidenceRow) string {
		period := scope.period(row.CaptureUnixNanos, row.CaptureUnixNanos)
		// Canonical exposure runs forward from the sample, half-open. The
		// last covered nanosecond may equal the period's final included one.
		if period == 0 || (row.IntervalNanos > 0 && (row.CaptureUnixNanos > (1<<63-1)-row.IntervalNanos || scope.period(row.CaptureUnixNanos, row.CaptureUnixNanos+row.IntervalNanos-1) != period)) {
			return "outside_catalogued_static_period"
		}
		if row.Bucket == "following_eligible" || row.DRAC.Status == "supported" {
			if paths[row.FollowerTrackID] != period || paths[row.Decision.LeaderTrackID] != period {
				return "path_support_crosses_static_period"
			}
		}
		return ""
	}, nil
}
