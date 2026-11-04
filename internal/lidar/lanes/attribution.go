package lanes

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/lanegeom"
)

// Version is one version of one lane, as one snapshot held it, with the
// window its limit was in force.
type Version struct {
	Lane       Lane      `json:"lane"`
	SnapshotID string    `json:"snapshot_id"`
	SnapshotAt time.Time `json:"snapshot_at"`
	// From is when the version's limit took effect (its effective_from at
	// UTC midnight); zero when the lane carries no date, which reads as in
	// force for all time before the next dated version.
	From time.Time `json:"from,omitempty"`
	// To is when it stopped: the day after effective_to, or the time of the
	// first later snapshot that no longer holds the lane. Zero while open.
	To time.Time `json:"to,omitempty"`
	// EditedAt is Gerardus's edit time of the row, for ties.
	EditedAt time.Time `json:"edited_at,omitempty"`
}

// History is every version of every lane a deployment's snapshots hold.
type History struct {
	Versions  []Version `json:"versions"`
	Snapshots []string  `json:"snapshot_ids"`
	// byFeature indexes Versions by feature id, ordered by From then edit.
	byFeature map[string][]int
	cache     map[string][]Direction
}

// NewHistory builds the history of one deployment's snapshots, in any order.
func NewHistory(snapshots []Snapshot) *History {
	sorted := append([]Snapshot(nil), snapshots...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].FetchedAt.Before(sorted[j].FetchedAt) })
	h := &History{byFeature: map[string][]int{}, cache: map[string][]Direction{}}
	// The first snapshot after a lane's last appearance ends it.
	lastSeen := map[string]int{}
	for si, s := range sorted {
		h.Snapshots = append(h.Snapshots, s.SnapshotID)
		for _, l := range s.Lanes {
			lastSeen[l.FeatureID] = si
		}
	}
	seenRow := map[string]bool{}
	for si, s := range sorted {
		for _, l := range s.Lanes {
			// One version per Gerardus row, kept from the snapshot that first
			// held it.
			if seenRow[l.FeatureID+"\x00"+l.RowID] {
				continue
			}
			seenRow[l.FeatureID+"\x00"+l.RowID] = true
			v := Version{Lane: l, SnapshotID: s.SnapshotID, SnapshotAt: s.FetchedAt}
			if l.Limit != nil {
				v.From = day(l.Limit.EffectiveFrom)
				if to := day(l.Limit.EffectiveTo); !to.IsZero() {
					v.To = to.AddDate(0, 0, 1)
				}
			}
			if t, err := time.Parse(time.RFC3339Nano, l.EditedAt); err == nil {
				v.EditedAt = t
			}
			if last := lastSeen[l.FeatureID]; last+1 < len(sorted) {
				ended := sorted[last+1].FetchedAt
				if v.To.IsZero() || ended.Before(v.To) {
					v.To = ended
				}
			}
			_ = si
			h.Versions = append(h.Versions, v)
		}
	}
	for i, v := range h.Versions {
		h.byFeature[v.Lane.FeatureID] = append(h.byFeature[v.Lane.FeatureID], i)
	}
	for _, idx := range h.byFeature {
		sort.SliceStable(idx, func(a, b int) bool {
			va, vb := h.Versions[idx[a]], h.Versions[idx[b]]
			if !va.From.Equal(vb.From) {
				return va.From.Before(vb.From)
			}
			if !va.EditedAt.Equal(vb.EditedAt) {
				return va.EditedAt.Before(vb.EditedAt)
			}
			return va.SnapshotAt.Before(vb.SnapshotAt)
		})
	}
	return h
}

// Empty reports whether the history holds no lane.
func (h *History) Empty() bool { return h == nil || len(h.Versions) == 0 }

// At returns the version of each lane in force at t: per lane, the version
// whose effective date is the latest not after t, the latest edit breaking a
// tie, and none when that version had ended by t.
func (h *History) At(t time.Time) []Version {
	if h.Empty() {
		return nil
	}
	ids := make([]string, 0, len(h.byFeature))
	for id := range h.byFeature {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []Version
	for _, id := range ids {
		var chosen *Version
		for _, i := range h.byFeature[id] {
			v := h.Versions[i]
			if !v.From.IsZero() && v.From.After(t) {
				continue
			}
			c := v
			chosen = &c
		}
		if chosen == nil || (!chosen.To.IsZero() && !t.Before(chosen.To)) {
			continue
		}
		out = append(out, *chosen)
	}
	return out
}

// DirectionsAt are the directions of the lanes in force at t.
func (h *History) DirectionsAt(t time.Time) []Direction {
	versions := h.At(t)
	return h.directionsOf(versions)
}

func (h *History) directionsOf(versions []Version) []Direction {
	keys := make([]string, len(versions))
	lanes := make([]Lane, len(versions))
	for i, v := range versions {
		keys[i] = v.Lane.FeatureID + "@" + v.Lane.RowID
		lanes[i] = v.Lane
	}
	key := strings.Join(keys, ",")
	if d, ok := h.cache[key]; ok {
		return d
	}
	d := DeriveDirections(lanes)
	h.cache[key] = d
	return d
}

// Reasons attribution gives no lane, or no limit.
const (
	ReasonNoSnapshot = "no_snapshot"
	ReasonNoLane     = "no_lane"
	ReasonWrongWay   = "wrong_way"
	ReasonAmbiguous  = "ambiguous"
	ReasonNoLimit    = "no_limit"
	ReasonStationary = "stationary"
)

// Attribution is where one measurement belongs.
type Attribution struct {
	FeatureID     string  `json:"feature_id,omitempty"`
	RowID         string  `json:"row_id,omitempty"`
	DirectionID   string  `json:"direction_id,omitempty"`
	DirectionName string  `json:"direction_name,omitempty"`
	Limit         *Limit  `json:"limit,omitempty"`
	Reason        string  `json:"reason,omitempty"`
	ArcLengthM    float64 `json:"arc_length_m,omitempty"`
	LateralM      float64 `json:"lateral_m,omitempty"`
	SnapshotID    string  `json:"snapshot_id,omitempty"`
}

// Attribute gives a position and heading (radians, sensor frame) at time t
// the lane, direction and limit in force, or the reason there is none. With
// `previous` the feature id the track held at its last instant, membership
// keeps it while it still fits (hysteresis).
func (h *History) Attribute(p lanegeom.Point, heading float64, t time.Time, previous string) Attribution {
	versions := h.At(t)
	if len(versions) == 0 {
		return Attribution{Reason: ReasonNoSnapshot}
	}
	geo := make([]lanegeom.Lane, len(versions))
	prev := -1
	for i, v := range versions {
		geo[i] = lanegeom.Lane{ID: v.Lane.FeatureID, Points: v.Lane.Sensor, HalfWidth: lanegeom.Corridor(v.Lane.WidthM)}
		if v.Lane.FeatureID == previous {
			prev = i
		}
	}
	m := lanegeom.Member(geo, p, heading, prev)
	if m.Lane < 0 {
		reason := ReasonNoLane
		if m.Reason == lanegeom.ReasonOpposed {
			reason = ReasonWrongWay
		}
		return Attribution{Reason: reason}
	}
	v := versions[m.Lane]
	a := Attribution{
		FeatureID: v.Lane.FeatureID, RowID: v.Lane.RowID, Limit: v.Lane.Limit,
		ArcLengthM: m.Projection.S, LateralM: m.Projection.Lateral, SnapshotID: v.SnapshotID,
	}
	for _, d := range h.directionsOf(versions) {
		for _, id := range d.Lanes {
			if id == v.Lane.FeatureID {
				a.DirectionID, a.DirectionName = d.ID, d.Name
			}
		}
	}
	if a.Limit == nil {
		a.Reason = ReasonNoLimit
	}
	return a
}

// Instant is one observed instant of a track, sensor frame.
type Instant struct {
	TimeNs   int64
	X, Y     float64
	VX, VY   float64
	SpeedMps float64
}

// LaneSpeed is a track's maximum speed within one lane, against the limit in
// force when that maximum was measured.
type LaneSpeed struct {
	FeatureID   string  `json:"feature_id"`
	DirectionID string  `json:"direction_id,omitempty"`
	MaxSpeedMps float64 `json:"max_speed_mps"`
	AtNs        int64   `json:"at_ns"`
	Limit       *Limit  `json:"limit,omitempty"`
}

// TrackAttribution is a track's lane and direction, or the reason it has
// none, and its maxima per lane.
type TrackAttribution struct {
	FeatureID     string  `json:"feature_id,omitempty"`
	DirectionID   string  `json:"direction_id,omitempty"`
	DirectionName string  `json:"direction_name,omitempty"`
	Reason        string  `json:"reason,omitempty"`
	MaxSpeedMps   float64 `json:"max_speed_mps"`
	// Limit is the limit of the lane holding the sample where the track's
	// maximum speed was observed, in force at that sample's time.
	Limit *Limit `json:"limit,omitempty"`
	// PerLane holds the track's maximum within each lane it was in, so a
	// track crossing a split at a sign counts in each lane's population,
	// and once in the site's.
	PerLane []LaneSpeed `json:"per_lane,omitempty"`
}

// Track shares: a track's lane holds this share of its moving instants.
const (
	TrackLaneShare      = 0.6
	TrackMovingSpeedMps = 2.0
)

// AttributeTrack attributes a track (snapshot plan, Attribution; speed by
// direction plan, LiDAR per track): its lane is the lane holding at least
// 60 % of its moving instants, otherwise none with a reason (ambiguous for
// two lanes neither over 60 %, wrong_way when inside lanes but against every
// one, no_lane otherwise). Its speed is its maximum observed speed, compared
// with the limit of the lane holding that sample at that time.
func (h *History) AttributeTrack(instants []Instant) TrackAttribution {
	sorted := append([]Instant(nil), instants...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].TimeNs < sorted[j].TimeNs })
	out := TrackAttribution{}
	type laneMax struct {
		speed float64
		at    int64
		limit *Limit
		dir   string
	}
	counts := map[string]int{}
	reasons := map[string]int{}
	perLane := map[string]*laneMax{}
	moving := 0
	previous := ""
	maxSpeed := -1.0
	// The attribution of the sample where the maximum was observed; empty
	// when that sample was not moving or in no lane.
	var maxAttr Attribution
	for _, in := range sorted {
		newMax := in.SpeedMps > maxSpeed
		if newMax {
			maxSpeed = in.SpeedMps
			maxAttr = Attribution{}
		}
		if in.SpeedMps < TrackMovingSpeedMps || (in.VX == 0 && in.VY == 0) {
			continue
		}
		moving++
		a := h.Attribute(lanegeom.Point{X: in.X, Y: in.Y}, math.Atan2(in.VY, in.VX), time.Unix(0, in.TimeNs).UTC(), previous)
		if a.FeatureID == "" {
			reasons[a.Reason]++
			previous = ""
			continue
		}
		previous = a.FeatureID
		counts[a.FeatureID]++
		lm := perLane[a.FeatureID]
		if lm == nil || in.SpeedMps > lm.speed {
			perLane[a.FeatureID] = &laneMax{speed: in.SpeedMps, at: in.TimeNs, limit: a.Limit, dir: a.DirectionID}
		}
		if newMax {
			maxAttr = a
		}
	}
	out.MaxSpeedMps = math.Max(0, maxSpeed)
	if moving == 0 {
		out.Reason = ReasonStationary
		return out
	}
	ids := make([]string, 0, len(perLane))
	for id := range perLane {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		lm := perLane[id]
		out.PerLane = append(out.PerLane, LaneSpeed{FeatureID: id, DirectionID: lm.dir, MaxSpeedMps: lm.speed, AtNs: lm.at, Limit: lm.limit})
	}
	best, bestN, second := "", 0, 0
	for _, id := range ids {
		n := counts[id]
		if n > bestN {
			second, best, bestN = bestN, id, n
		} else if n > second {
			second = n
		}
	}
	if best != "" && float64(bestN) >= TrackLaneShare*float64(moving) {
		out.FeatureID = best
		out.DirectionID = perLane[best].dir
		for _, d := range h.DirectionsAt(time.Unix(0, perLane[best].at).UTC()) {
			if d.ID == out.DirectionID {
				out.DirectionName = d.Name
			}
		}
		if maxAttr.FeatureID != "" {
			out.Limit = maxAttr.Limit
		}
		if out.Limit == nil {
			out.Reason = ReasonNoLimit
		}
		return out
	}
	switch {
	case len(counts) >= 2 && second > 0:
		out.Reason = ReasonAmbiguous
	case reasons[ReasonWrongWay] > reasons[ReasonNoLane] && reasons[ReasonWrongWay] > 0:
		out.Reason = ReasonWrongWay
	case reasons[ReasonNoSnapshot] == moving:
		out.Reason = ReasonNoSnapshot
	default:
		out.Reason = ReasonNoLane
	}
	return out
}

func day(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
