// Package lanes is velocity.report's copy of the travel lanes Gerardus holds
// for a deployment, and the attribution of a measurement to a lane, a
// direction and the limit in force when it was made
// (docs/plans/lidar-lane-snapshot-and-attribution-plan.md, Item 1).
//
// A snapshot is a full copy of the street-use features Gerardus returned,
// exactly as returned, with the accepted pose that places them in the sensor
// frame. Snapshots are append-only, so their union is the version history of
// every lane this repository has been told about: a limit that changes over
// time is a new version of the same lane (same feature id) with the date it
// took effect, and attribution reads the version in force at a
// measurement's time, never the one current when the report runs.
//
// The lanes are measured spatial data: the operator's reading of where
// traffic runs, at a stated time. This package imports lanegeom and nothing
// above it, so the report pipeline and the scene export can both use it.
package lanes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/lanegeom"
)

// Writers of a snapshot.
const (
	WriterPainter = "painter"
	WriterPull    = "pull"
)

// The tags a lane's limit is read from (Gerardus LANES-AND-LIMITS §3.1).
const (
	TagSpeedLimit    = "speed_limit"
	TagUnit          = "speed_limit_unit"
	TagSource        = "speed_limit_source"
	TagJurisdiction  = "speed_limit_jurisdiction"
	TagNote          = "speed_limit_note"
	TagEffectiveFrom = "speed_limit_effective_from"
	TagEffectiveTo   = "speed_limit_effective_to"
	TagStreet        = "street"
	TagCNN           = "cnn"
	TagBound         = "bound"
	TagBearing       = "bearing_deg"
	TagDirectionName = "direction_name"
	TagWidth         = "width_m"
	TagLaneType      = "lane_type"
)

// KindTravelLane is the feature kind attribution reads.
const KindTravelLane = "travel_lane"

// Pose places the site frame's features in the sensor frame: the sensor's
// origin in the site frame and its axes as a unit quaternion (w, x, y, z).
type Pose struct {
	Translation [3]float64 `json:"translation_m"`
	Rotation    [4]float64 `json:"rotation_wxyz"`
}

// Feature is one street-use feature as Gerardus returned it: its id and row,
// kind, site-frame vertices (x, y, z interleaved), whether it is closed, its
// tags, author and edit time.
type Feature struct {
	FeatureID string            `json:"feature_id"`
	RowID     string            `json:"row_id"`
	Kind      string            `json:"kind"`
	XYZ       []float64         `json:"xyz"`
	Closed    bool              `json:"closed,omitempty"`
	Tags      map[string]string `json:"tags"`
	Author    string            `json:"author,omitempty"`
	EditedAt  string            `json:"edited_at,omitempty"`
}

// Request is what a writer sends: PUT /api/lidar/sites/{l16}/lanes, or
// `velocity lidar lanes pull`.
type Request struct {
	RequestID       string    `json:"request_id"`
	Writer          string    `json:"writer"`
	SensorID        string    `json:"sensor_id"`
	RecordID        string    `json:"record_id"`
	Pose            Pose      `json:"pose"`
	Author          string    `json:"author"`
	GerardusVersion string    `json:"gerardus_version,omitempty"`
	GerardusAddress string    `json:"gerardus_address,omitempty"`
	Features        []Feature `json:"features"`
}

// Limit is a lane's limit as signed.
type Limit struct {
	Value        int    `json:"value"`
	Unit         string `json:"unit"`
	Source       string `json:"source"`
	Jurisdiction string `json:"jurisdiction,omitempty"`
	Note         string `json:"note,omitempty"`
	// EffectiveFrom and EffectiveTo are YYYY-MM-DD, the second optional.
	EffectiveFrom string `json:"effective_from,omitempty"`
	EffectiveTo   string `json:"effective_to,omitempty"`
}

// Lane is one travel lane of a snapshot, in both frames.
type Lane struct {
	FeatureID string `json:"feature_id"`
	RowID     string `json:"row_id"`
	EditedAt  string `json:"edited_at,omitempty"`
	// Sensor is the centreline in the sensor frame's plane, in the
	// direction of travel, derived from the pose when the snapshot was
	// written.
	Sensor     []lanegeom.Point  `json:"sensor_xy"`
	WidthM     float64           `json:"width_m"`
	Street     string            `json:"street,omitempty"`
	CNN        string            `json:"cnn,omitempty"`
	Bound      string            `json:"bound,omitempty"`
	BearingDeg float64           `json:"bearing_deg"`
	Direction  string            `json:"direction"`
	Limit      *Limit            `json:"limit,omitempty"`
	Tags       map[string]string `json:"tags"`
}

// Direction is the lanes of one street carrying traffic the same way.
type Direction struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Street     string   `json:"street,omitempty"`
	BearingDeg float64  `json:"bearing_deg"`
	Lanes      []string `json:"lanes"`
	// Limit is set when every lane in it carries the same value and unit.
	Limit *Limit `json:"limit,omitempty"`
}

// Snapshot is one write: the request's provenance, the features as returned,
// and the lanes and directions derived from them.
type Snapshot struct {
	SnapshotID      string      `json:"snapshot_id"`
	L16Token        string      `json:"l16_token"`
	SensorID        string      `json:"sensor_id"`
	Writer          string      `json:"writer"`
	RequestID       string      `json:"request_id"`
	RecordID        string      `json:"record_id"`
	Pose            Pose        `json:"pose"`
	Author          string      `json:"author"`
	GerardusVersion string      `json:"gerardus_version,omitempty"`
	GerardusAddress string      `json:"gerardus_address,omitempty"`
	FeaturesDigest  string      `json:"features_digest"`
	FetchedAt       time.Time   `json:"fetched_at"`
	Supersedes      string      `json:"supersedes,omitempty"`
	Features        []Feature   `json:"features"`
	Lanes           []Lane      `json:"lanes"`
	Directions      []Direction `json:"directions"`
}

var (
	jurisdictionRE = regexp.MustCompile(`^[A-Z]{2}-[A-Z0-9]{1,3}$`)
	// ErrInvalid wraps every refusal of a request.
	ErrInvalid = errors.New("invalid lane snapshot")
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Validate checks a request before anything is derived.
func (r Request) Validate() error {
	if strings.TrimSpace(r.RequestID) == "" {
		return invalid("request_id is required")
	}
	if r.Writer != WriterPainter && r.Writer != WriterPull {
		return invalid("writer must be painter or pull")
	}
	if r.SensorID == "" || r.RecordID == "" {
		return invalid("sensor_id and record_id are required: a snapshot without a pose cannot place lanes")
	}
	q := r.Pose.Rotation
	n := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3])
	if math.IsNaN(n) || math.Abs(n-1) > 1e-3 {
		return invalid("pose.rotation_wxyz must be a unit quaternion")
	}
	for _, v := range r.Pose.Translation {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return invalid("pose.translation_m must be finite")
		}
	}
	if strings.TrimSpace(r.Author) == "" {
		return invalid("author is required")
	}
	seen := map[string]bool{}
	for i, f := range r.Features {
		if f.FeatureID == "" || f.RowID == "" {
			return invalid("feature %d: feature_id and row_id are required (features as Gerardus returned them)", i)
		}
		if seen[f.FeatureID] {
			return invalid("feature %s appears twice", f.FeatureID)
		}
		seen[f.FeatureID] = true
		if len(f.XYZ)%3 != 0 {
			return invalid("feature %s: xyz must hold whole vertices", f.FeatureID)
		}
	}
	return nil
}

// Digest is a stable hash of the features, independent of their order.
func Digest(features []Feature) string {
	sorted := append([]Feature(nil), features...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].FeatureID < sorted[j].FeatureID })
	b, _ := json.Marshal(sorted)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Build derives a snapshot from a request: each travel lane's sensor-frame
// centreline through the inverse of the pose, its limit, and the
// directions. Lanes with fewer than two vertices are kept as features but
// carry no lane.
func Build(l16 string, r Request, fetchedAt time.Time, supersedes string) (Snapshot, error) {
	if err := r.Validate(); err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{
		L16Token: l16, SensorID: r.SensorID, Writer: r.Writer, RequestID: r.RequestID,
		RecordID: r.RecordID, Pose: r.Pose, Author: r.Author,
		GerardusVersion: r.GerardusVersion, GerardusAddress: r.GerardusAddress,
		FeaturesDigest: Digest(r.Features), FetchedAt: fetchedAt.UTC(), Supersedes: supersedes,
		Features: r.Features, Lanes: []Lane{}, Directions: []Direction{},
	}
	for _, f := range r.Features {
		if f.Kind != KindTravelLane || len(f.XYZ) < 6 {
			continue
		}
		lane := Lane{FeatureID: f.FeatureID, RowID: f.RowID, EditedAt: f.EditedAt, Tags: f.Tags}
		for i := 0; i+2 < len(f.XYZ); i += 3 {
			lane.Sensor = append(lane.Sensor, r.Pose.toSensor(f.XYZ[i], f.XYZ[i+1], f.XYZ[i+2]))
		}
		lane.WidthM, _ = strconv.ParseFloat(f.Tags[TagWidth], 64)
		lane.Street = f.Tags[TagStreet]
		lane.CNN = f.Tags[TagCNN]
		n := len(f.XYZ)
		bearing := siteBearing(f.XYZ[0], f.XYZ[1], f.XYZ[n-3], f.XYZ[n-2])
		if b, err := strconv.ParseFloat(f.Tags[TagBearing], 64); err == nil {
			bearing = b
		}
		lane.BearingDeg = bearing
		lane.Bound = f.Tags[TagBound]
		if lane.Bound == "" {
			lane.Bound = Bound(bearing)
		}
		lane.Limit = ParseLimit(f.Tags)
		s.Lanes = append(s.Lanes, lane)
	}
	sort.Slice(s.Lanes, func(i, j int) bool { return s.Lanes[i].FeatureID < s.Lanes[j].FeatureID })
	s.Directions = DeriveDirections(s.Lanes)
	for i := range s.Lanes {
		for _, d := range s.Directions {
			for _, id := range d.Lanes {
				if id == s.Lanes[i].FeatureID {
					s.Lanes[i].Direction = d.ID
				}
			}
		}
	}
	s.SnapshotID = snapshotID(s)
	return s, nil
}

func snapshotID(s Snapshot) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%d", s.L16Token, s.SensorID, s.RequestID, s.FeaturesDigest, s.FetchedAt.UnixNano())
	return "lsn_" + hex.EncodeToString(h.Sum(nil))[:24]
}

// toSensor maps a site-frame point into the sensor frame's plane:
// R⁻¹ (p − t).
func (p Pose) toSensor(x, y, z float64) lanegeom.Point {
	d := [3]float64{x - p.Translation[0], y - p.Translation[1], z - p.Translation[2]}
	w, qx, qy, qz := p.Rotation[0], -p.Rotation[1], -p.Rotation[2], -p.Rotation[3]
	r := rotate(w, qx, qy, qz, d)
	return lanegeom.Point{X: r[0], Y: r[1]}
}

// SensorYBearing is the compass bearing of the sensor's +Y in the site frame.
func (p Pose) SensorYBearing() float64 {
	y := rotate(p.Rotation[0], p.Rotation[1], p.Rotation[2], p.Rotation[3], [3]float64{0, 1, 0})
	b := math.Atan2(y[0], y[1]) * 180 / math.Pi
	if b < 0 {
		b += 360
	}
	return b
}

func rotate(w, x, y, z float64, v [3]float64) [3]float64 {
	// v' = q v q*, expanded.
	tx := 2 * (y*v[2] - z*v[1])
	ty := 2 * (z*v[0] - x*v[2])
	tz := 2 * (x*v[1] - y*v[0])
	return [3]float64{
		v[0] + w*tx + (y*tz - z*ty),
		v[1] + w*ty + (z*tx - x*tz),
		v[2] + w*tz + (x*ty - y*tx),
	}
}

func siteBearing(x0, y0, x1, y1 float64) float64 {
	b := math.Atan2(x1-x0, y1-y0) * 180 / math.Pi
	if b < 0 {
		b += 360
	}
	return b
}

// Bound is a bearing's compass octant, as Gerardus's `bound` tag.
func Bound(bearing float64) string {
	names := []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	b := math.Mod(bearing, 360)
	if b < 0 {
		b += 360
	}
	return names[int(math.Floor((b+22.5)/45))%8]
}

// BoundWord is "eastbound" for "E".
func BoundWord(bound string) string {
	switch bound {
	case "N":
		return "northbound"
	case "NE":
		return "northeastbound"
	case "E":
		return "eastbound"
	case "SE":
		return "southeastbound"
	case "S":
		return "southbound"
	case "SW":
		return "southwestbound"
	case "W":
		return "westbound"
	case "NW":
		return "northwestbound"
	}
	return ""
}

// TitleCase is "Columbus Ave" for "COLUMBUS AVE".
func TitleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}

// ParseLimit reads a lane's limit tags; nil when it has no valid limit.
func ParseLimit(tags map[string]string) *Limit {
	v, err := strconv.Atoi(strings.TrimSpace(tags[TagSpeedLimit]))
	unit := tags[TagUnit]
	if err != nil || v <= 0 || (unit != "mph" && unit != "km/h") {
		return nil
	}
	if (unit == "mph" && v > 90) || (unit == "km/h" && v > 150) {
		return nil
	}
	l := &Limit{
		Value: v, Unit: unit, Source: tags[TagSource], Jurisdiction: tags[TagJurisdiction],
		Note: tags[TagNote], EffectiveFrom: tags[TagEffectiveFrom], EffectiveTo: tags[TagEffectiveTo],
	}
	if l.Jurisdiction != "" && !jurisdictionRE.MatchString(l.Jurisdiction) {
		l.Jurisdiction = ""
	}
	return l
}

// DeriveDirections groups travel lanes that share a street and whose
// bearings lie within 45° of their group's circular mean, named from the
// lanes' direction_name when one is given, else the street and the bound.
func DeriveDirections(lanes []Lane) []Direction {
	type group struct {
		street   string
		lanes    []int
		sin, cos float64
	}
	var groups []*group
	order := make([]int, len(lanes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return lanes[order[a]].FeatureID < lanes[order[b]].FeatureID })
	for _, i := range order {
		l := lanes[i]
		street := l.Street
		if street == "" {
			street = l.CNN
		}
		var into *group
		for _, g := range groups {
			mean := math.Atan2(g.sin, g.cos) * 180 / math.Pi
			if g.street == street && angleGap(mean, l.BearingDeg) <= 45 {
				into = g
				break
			}
		}
		if into == nil {
			into = &group{street: street}
			groups = append(groups, into)
		}
		into.lanes = append(into.lanes, i)
		r := l.BearingDeg * math.Pi / 180
		into.sin += math.Sin(r)
		into.cos += math.Cos(r)
	}
	out := make([]Direction, 0, len(groups))
	for _, g := range groups {
		mean := math.Mod(math.Atan2(g.sin, g.cos)*180/math.Pi+360, 360)
		d := Direction{Street: g.street, BearingDeg: math.Round(mean*10) / 10}
		bound := Bound(mean)
		var name string
		var limit *Limit
		agree := true
		for k, i := range g.lanes {
			l := lanes[i]
			d.Lanes = append(d.Lanes, l.FeatureID)
			if name == "" {
				name = l.Tags[TagDirectionName]
			}
			if k == 0 {
				limit = l.Limit
			} else if !sameLimit(limit, l.Limit) {
				agree = false
			}
		}
		sort.Strings(d.Lanes)
		if agree && limit != nil {
			c := *limit
			d.Limit = &c
		}
		if name == "" {
			name = strings.TrimSpace(TitleCase(g.street) + " " + BoundWord(bound))
		}
		d.Name = name
		d.ID = directionID(g.street, bound)
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	// Two groups of one street and bound (a curve past 45°) would share an
	// id: the second and later are numbered.
	seen := map[string]int{}
	for i := range out {
		seen[out[i].ID]++
		if n := seen[out[i].ID]; n > 1 {
			out[i].ID = fmt.Sprintf("%s-%d", out[i].ID, n)
		}
	}
	return out
}

func directionID(street, bound string) string {
	s := strings.ToLower(strings.Join(strings.Fields(street), "-"))
	if s == "" {
		s = "street"
	}
	return s + ":" + strings.ToLower(bound)
}

func sameLimit(a, b *Limit) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Value == b.Value && a.Unit == b.Unit
}

func angleGap(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360)
	if d > 180 {
		d = 360 - d
	}
	return d
}
