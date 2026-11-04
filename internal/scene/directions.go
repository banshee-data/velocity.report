package scene

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/lanegeom"
	"github.com/banshee-data/velocity.report/internal/lidar/lanes"
	"github.com/banshee-data/velocity.report/internal/stats"
)

// Directions in the scene export
// (docs/plans/platform-speed-by-direction-reporting-plan.md, Item 4): the
// vehicle speed summary's car tracks grouped by direction of travel, from
// the site's lane snapshot where it has one, and from the heading fallback
// (the two opposed modes of car headings) until it does. Only aggregates
// leave: no track id, trail or per-track speed is written, and a direction
// with fewer than five tracks is folded into the headline.

// Direction sources.
const (
	DirectionSourceLanes   = "lanes"
	DirectionSourceHeading = "heading"
)

// Heading fallback thresholds (speed by direction plan, Direction as the
// first grouping).
const (
	directionMinTracks      = 5
	directionTailMinTracks  = 50
	headingModeShare        = 0.20
	headingModeHalfWidthDeg = 30.0
	headingOtherDeg         = 60.0
	headingMovingMPS        = 2.0
)

// VehicleDirectionSummary is one direction of a scene's car tracks, with the
// limit in force while it was recorded where lanes say so.
type VehicleDirectionSummary struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	BearingDeg *float64             `json:"bearing_deg"`
	Source     string               `json:"source"`
	SpeedLimit *DirectionSpeedLimit `json:"speed_limit"`
	TrackCount int                  `json:"track_count"`
	P50        *float64             `json:"p50"`
	P85        *float64             `json:"p85"`
	// P98 and Max are null under fifty tracks, so a quiet direction does not
	// publish one driver's speed.
	P98       *float64             `json:"p98"`
	Max       *float64             `json:"max"`
	Histogram []VehicleSpeedBucket `json:"histogram"`
}

// DirectionSpeedLimit is a direction's limit as signed.
type DirectionSpeedLimit struct {
	Value         int    `json:"value"`
	Unit          string `json:"unit"`
	Source        string `json:"source"`
	EffectiveFrom string `json:"effective_from,omitempty"`
}

// carTrack is what the accumulator keeps of one car track, in memory only.
type carTrack struct {
	maxMPS   float64
	sin, cos float64
	moving   int
	instants []lanes.Instant
}

// directionSummaries groups the summary's population: each car track over
// the minimum speed counts once, in the direction it is attributed to.
func (a *timelineAccumulator) directionSummaries() []VehicleDirectionSummary {
	type group struct {
		summary VehicleDirectionSummary
		speeds  []float64
	}
	groups := map[string]*group{}
	var order []string
	add := func(id string, speedMPH float64, make func() VehicleDirectionSummary) {
		g := groups[id]
		if g == nil {
			g = &group{summary: make()}
			groups[id] = g
			order = append(order, id)
		}
		g.speeds = append(g.speeds, speedMPH)
	}
	ids := make([]string, 0, len(a.cars))
	for id, c := range a.cars {
		if c.maxMPS*mphPerMPS > vehicleMinimumSpeedMPH && a.isCar[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	if !a.lanes.Empty() {
		mid := time.Unix(0, a.firstNs+(a.lastNs-a.firstNs)/2).UTC()
		dirs := map[string]lanes.Direction{}
		for _, d := range a.lanes.DirectionsAt(mid) {
			dirs[d.ID] = d
		}
		for _, id := range ids {
			c := a.cars[id]
			tr := a.lanes.AttributeTrack(c.instants)
			if tr.DirectionID == "" {
				continue
			}
			d, ok := dirs[tr.DirectionID]
			add(tr.DirectionID, c.maxMPS*mphPerMPS, func() VehicleDirectionSummary {
				s := VehicleDirectionSummary{ID: tr.DirectionID, Name: tr.DirectionName, Source: DirectionSourceLanes}
				if ok {
					b := d.BearingDeg
					s.BearingDeg = &b
					if d.Limit != nil {
						s.SpeedLimit = &DirectionSpeedLimit{Value: d.Limit.Value, Unit: d.Limit.Unit, Source: d.Limit.Source, EffectiveFrom: d.Limit.EffectiveFrom}
					}
				}
				return s
			})
		}
	} else {
		modes := headingModes(a.cars, ids)
		for _, id := range ids {
			c := a.cars[id]
			if c.moving == 0 {
				continue
			}
			h := math.Atan2(c.sin, c.cos)
			for i, m := range modes {
				if lanegeom.HeadingError(h, m) > headingOtherDeg*math.Pi/180 {
					continue
				}
				i, m := i, m
				add(fmt.Sprintf("heading:%d", i), c.maxMPS*mphPerMPS, func() VehicleDirectionSummary {
					s := VehicleDirectionSummary{ID: fmt.Sprintf("heading:%d", i), Source: DirectionSourceHeading, Name: fmt.Sprintf("Direction %c", 'A'+i)}
					if a.sensorYBearing != nil {
						// HeadingToBearing measures clockwise from the
						// sensor's +Y; the pose turns that to true north.
						b := math.Mod(lanegeom.HeadingToBearing(m)+*a.sensorYBearing+360, 360)
						b = math.Round(b*10) / 10
						s.BearingDeg = &b
						s.ID = "heading:" + lowerBound(b)
						s.Name = lanes.BoundWord(lanes.Bound(b))
					}
					return s
				})
				break
			}
		}
	}

	out := []VehicleDirectionSummary{}
	for _, id := range order {
		g := groups[id]
		if len(g.speeds) < directionMinTracks {
			continue
		}
		sort.Float64s(g.speeds)
		s := g.summary
		s.TrackCount = len(g.speeds)
		pct := func(p float64) *float64 {
			v := math.Round(stats.NearestRank(g.speeds, p)*100) / 100
			return &v
		}
		s.P50, s.P85 = pct(0.50), pct(0.85)
		if s.TrackCount >= directionTailMinTracks {
			s.P98, s.Max = pct(0.98), pct(1)
		}
		s.Histogram = speedHistogram(g.speeds)
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) == 1 && out[0].Source == DirectionSourceHeading {
		// One heading mode is not a direction split: the headline holds it.
		return []VehicleDirectionSummary{}
	}
	return out
}

func lowerBound(bearing float64) string {
	b := lanes.Bound(bearing)
	out := []rune(b)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + 'a' - 'A'
		}
	}
	return string(out)
}

// headingModes finds the two largest opposed modes of the tracks' mean
// headings (radians, sensor frame): the busiest 60° window, and the busiest
// window within 30° of its opposite. Each must hold a fifth of the tracks
// and at least five; otherwise there is no split.
func headingModes(cars map[string]*carTrack, ids []string) []float64 {
	var headings []float64
	for _, id := range ids {
		c := cars[id]
		if c.moving > 0 && (c.sin != 0 || c.cos != 0) {
			headings = append(headings, math.Atan2(c.sin, c.cos))
		}
	}
	if len(headings) < 2*directionMinTracks {
		return nil
	}
	count := func(centre float64) int {
		n := 0
		for _, h := range headings {
			if lanegeom.HeadingError(h, centre) <= headingModeHalfWidthDeg*math.Pi/180 {
				n++
			}
		}
		return n
	}
	best, bestN := 0.0, -1
	for d := 0; d < 360; d += 5 {
		c := float64(d) * math.Pi / 180
		if n := count(c); n > bestN {
			best, bestN = c, n
		}
	}
	opp, oppN := 0.0, -1
	for d := -30; d <= 30; d += 5 {
		c := best + math.Pi + float64(d)*math.Pi/180
		if n := count(c); n > oppN {
			opp, oppN = c, n
		}
	}
	need := int(math.Ceil(headingModeShare * float64(len(headings))))
	if bestN < max(need, directionMinTracks) || oppN < max(need, directionMinTracks) {
		return nil
	}
	// Centre each mode on the mean heading inside its window, so a tie
	// between windows cannot leave a mode on its window's edge.
	centre := func(c float64) float64 {
		var sx, sy float64
		for _, h := range headings {
			if lanegeom.HeadingError(h, c) <= headingModeHalfWidthDeg*math.Pi/180 {
				sx += math.Cos(h)
				sy += math.Sin(h)
			}
		}
		return math.Atan2(sy, sx)
	}
	return []float64{centre(best), centre(opp)}
}

// speedHistogram is the summary's 5 mph buckets from 5 mph to the maximum.
func speedHistogram(sortedMPH []float64) []VehicleSpeedBucket {
	if len(sortedMPH) == 0 {
		return []VehicleSpeedBucket{}
	}
	last := int(math.Floor(sortedMPH[len(sortedMPH)-1]/vehicleSpeedBucketMPH)) * vehicleSpeedBucketMPH
	counts := map[int]int{}
	for _, s := range sortedMPH {
		counts[int(math.Floor(s/vehicleSpeedBucketMPH))*vehicleSpeedBucketMPH]++
	}
	out := make([]VehicleSpeedBucket, 0, last/vehicleSpeedBucketMPH+1)
	for start := vehicleMinimumSpeedMPH; start <= last; start += vehicleSpeedBucketMPH {
		out = append(out, VehicleSpeedBucket{StartMPH: start, Count: counts[start]})
	}
	return out
}
