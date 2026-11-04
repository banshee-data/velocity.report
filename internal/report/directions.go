package report

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/stats"
	"github.com/banshee-data/velocity.report/internal/units"
)

// Speed by direction (docs/plans/platform-speed-by-direction-reporting-plan.md,
// Items 2 and 3): the report's transits grouped by the radar's two
// directions, each against the limit signed for it and in force when each
// was measured. The headline population and its numbers are not touched:
// these are the same transits, grouped.

// Masking (D-18's temporal strata rule): a direction with fewer than
// DirectionMinCount transits is not reported apart (its transits stay in the
// headline); p98 and max print as "--" below DirectionTailMinCount.
const (
	DirectionMinCount     = 5
	DirectionTailMinCount = 50
)

// DirectionDB is the query a database answers to report by direction. The
// report reads it when the database has it and the source is transits.
type DirectionDB interface {
	TransitDirections(startUnix, endUnix int64, siteID int, modelVersion string, minSpeed float64) ([]db.DirectionalTransit, error)
}

// DirectionSegment is one direction over the part of the period one limit
// was in force.
type DirectionSegment struct {
	From, To time.Time
	Limit    *db.DirectionLimit
	Count    int
	// In the report's display units; nil when masked.
	P50, P85, P98, Max *float64
	// Histogram is display-unit bucket start → count.
	Histogram map[float64]int64
	// Shares above the limit, and by 5, 10 and 15 units of the sign, as
	// percentages; nil without a limit, or when compliance is suppressed.
	OverLimit, Over5, Over10, Over15 *float64
	// Suppressed gives why compliance is not reported.
	Suppressed string
}

// DirectionSummary is one direction of a report.
type DirectionSummary struct {
	Key        string
	Name       string
	BearingDeg *float64
	Count      int
	Segments   []DirectionSegment
}

// DirectionSet is a report's directions, and the transits counted in none.
type DirectionSet struct {
	Directions []DirectionSummary
	// Mixed transits (signs disagreeing) and unknown ones are in the
	// headline but in no direction; Masked counts those of directions under
	// DirectionMinCount.
	Mixed, Unknown, Masked int
}

// BuildDirections groups transits by their deployment direction and splits
// each at the effective dates of its limits inside the period. Speeds are
// m/s in; figures come out in displayUnits, histogram buckets of
// bucketSize display units merged above histMax (0: no cap).
func BuildDirections(transits []db.DirectionalTransit, start, end time.Time, displayUnits string, bucketSize, histMax float64) DirectionSet {
	var set DirectionSet
	byDir := map[string][]db.DirectionalTransit{}
	for _, t := range transits {
		switch t.Direction {
		case db.TransitInbound, db.TransitOutbound:
			if t.Directions == nil || t.Directions.Config(t.Direction) == nil {
				set.Unknown++
				continue
			}
			byDir[t.Direction] = append(byDir[t.Direction], t)
		case db.TransitMixed:
			set.Mixed++
		default:
			set.Unknown++
		}
	}
	for _, dirKey := range []string{db.TransitInbound, db.TransitOutbound} {
		list := byDir[dirKey]
		if len(list) == 0 {
			continue
		}
		if len(list) < DirectionMinCount {
			set.Masked += len(list)
			continue
		}
		// The name and bearing of the config in force at the period's end.
		cfg := list[len(list)-1].Directions.Config(dirKey)
		summary := DirectionSummary{Key: dirKey, Name: cfg.Name, BearingDeg: cfg.BearingDeg, Count: len(list)}
		// Segment by the limit in force at each transit.
		segments := map[string]*DirectionSegment{}
		var order []string
		speeds := map[string][]float64{}
		for _, t := range list {
			at := time.Unix(0, int64(t.StartUnix*1e9)).UTC()
			c := t.Directions.Config(dirKey)
			limit := LimitAt(c.Limits, at)
			k := limitKey(limit)
			seg := segments[k]
			if seg == nil {
				seg = &DirectionSegment{From: at, To: at, Limit: limit}
				segments[k] = seg
				order = append(order, k)
				if t.Directions.FootprintSpansLimitChange {
					seg.Suppressed = "the beam's footprint spans a change of limit"
				}
			}
			if at.Before(seg.From) {
				seg.From = at
			}
			if at.After(seg.To) {
				seg.To = at
			}
			speeds[k] = append(speeds[k], t.SpeedMPS)
		}
		// Segments run from the period's start or the limit's effective
		// date to the next segment's start or the period's end.
		sort.SliceStable(order, func(i, j int) bool { return segments[order[i]].From.Before(segments[order[j]].From) })
		for i, k := range order {
			seg := segments[k]
			seg.From = maxTime(start, limitStart(seg.Limit))
			if i == 0 {
				seg.From = start
			}
			if i+1 < len(order) {
				seg.To = maxTime(start, limitStart(segments[order[i+1]].Limit))
			} else {
				seg.To = end
			}
			summariseSegment(seg, speeds[k], displayUnits, bucketSize, histMax)
			summary.Segments = append(summary.Segments, *seg)
		}
		set.Directions = append(set.Directions, summary)
	}
	return set
}

// LimitAt is the limit in force at t: the one whose effective date is the
// latest not after t and that has not ended; an undated limit holds from
// the start of time.
func LimitAt(limits []db.DirectionLimit, t time.Time) *db.DirectionLimit {
	var best *db.DirectionLimit
	var bestFrom time.Time
	for i := range limits {
		l := &limits[i]
		from := day(l.EffectiveFrom)
		if !from.IsZero() && from.After(t) {
			continue
		}
		if to := day(l.EffectiveTo); !to.IsZero() && !t.Before(to.AddDate(0, 0, 1)) {
			continue
		}
		if best == nil || from.After(bestFrom) {
			best, bestFrom = l, from
		}
	}
	return best
}

// LimitLabel prints a limit as signed with its source: "25 mph (sign)",
// "25 mph (default, no sign recorded)", "25 mph (operator-supplied)".
func LimitLabel(l *db.DirectionLimit) string {
	if l == nil {
		return "no limit recorded"
	}
	switch l.Source {
	case "statutory":
		return fmt.Sprintf("%d %s (default, no sign recorded)", l.Value, l.Unit)
	case "operator":
		return fmt.Sprintf("%d %s (operator-supplied)", l.Value, l.Unit)
	default:
		return fmt.Sprintf("%d %s (%s)", l.Value, l.Unit, l.Source)
	}
}

func summariseSegment(seg *DirectionSegment, speedsMPS []float64, displayUnits string, bucketSize, histMax float64) {
	sorted := append([]float64(nil), speedsMPS...)
	sort.Float64s(sorted)
	seg.Count = len(sorted)
	display := func(p float64) *float64 {
		v := units.ConvertSpeed(stats.NearestRank(sorted, p), displayUnits)
		return &v
	}
	if seg.Count >= DirectionMinCount {
		seg.P50, seg.P85 = display(0.50), display(0.85)
	}
	if seg.Count >= DirectionTailMinCount {
		seg.P98, seg.Max = display(0.98), display(1)
	}
	if bucketSize > 0 {
		seg.Histogram = map[float64]int64{}
		for _, v := range sorted {
			d := units.ConvertSpeed(v, displayUnits)
			b := math.Floor(d/bucketSize) * bucketSize
			if histMax > 0 && b >= histMax {
				b = histMax
			}
			seg.Histogram[b]++
		}
	}
	if seg.Limit == nil || seg.Suppressed != "" || seg.Count < DirectionMinCount {
		return
	}
	// Shares in the sign's own unit.
	signUnit := "mph"
	if seg.Limit.Unit == "km/h" {
		signUnit = "kph"
	}
	share := func(over float64) *float64 {
		n := 0
		for _, v := range sorted {
			if units.ConvertSpeed(v, signUnit) > float64(seg.Limit.Value)+over {
				n++
			}
		}
		p := 100 * float64(n) / float64(len(sorted))
		return &p
	}
	seg.OverLimit, seg.Over5, seg.Over10, seg.Over15 = share(0), share(5), share(10), share(15)
}

func limitKey(l *db.DirectionLimit) string {
	if l == nil {
		return "none"
	}
	return fmt.Sprintf("%d|%s|%s|%s", l.Value, l.Unit, l.Source, l.EffectiveFrom)
}

func limitStart(l *db.DirectionLimit) time.Time {
	if l == nil {
		return time.Time{}
	}
	return day(l.EffectiveFrom)
}

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
