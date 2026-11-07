package l8analytics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"
)

// The lane field of docs/plans/lidar-lane-proposals-from-tracks-plan.md §1:
// one run's track observations summed into a 0.25 m raster over the sensor's
// horizontal plane, per class group, counting what distinct tracks did in each
// cell. It is the only track-derived thing that leaves a site (Gerardus
// ADR-0011): counts per cell, never a track id, a trail or a per-track speed,
// and no cell under k distinct tracks.
//
// The wire contract is Gerardus's LaneFieldHeader and LaneFieldCells
// (gerardus.align.v1alpha1, LANE-PROPOSALS §3), and the JSON form here is the
// one `gerardus upload-lane-field --json` reads. Nothing here imports Gerardus
// code (ADR-0006 decision 4) or any layer above L8.

// Lane field defaults (plan §1).
const (
	// LaneFieldCellM is the raster's cell size; Gerardus refuses any other.
	LaneFieldCellM = 0.25
	// LaneFieldK is the fewest distinct tracks a cell may carry (ADR-0011).
	LaneFieldK = 5
	// LaneFieldMinInstants is the fewest observed instants a track needs.
	LaneFieldMinInstants = 10
	// LaneFieldVehicleMinMaxSpeedMps is the lifetime maximum speed a vehicle
	// needs, so a car parked through the whole run paints no lane.
	LaneFieldVehicleMinMaxSpeedMps = 2.0
	// LaneFieldHeadingMinSpeedMps is the speed an instant needs before its
	// heading counts: below it the heading is noise and a stopped car names
	// no direction. It is LateralFitLocalMinSpeedMps.
	LaneFieldHeadingMinSpeedMps = LateralFitLocalMinSpeedMps
	// LaneFieldMaxBridgeS is the longest step between two consecutive
	// observed instants that the centre's path is drawn across. Instants come
	// about 0.1 s apart, so at street speed the centre moves 0.8 to 1 m
	// between them: four cells. Counting only the cells the centre was
	// sampled in left a 0.25 m line across a lane seeing about a quarter of
	// the tracks that drove over it. A longer step is a gap in observation
	// (occlusion, coasting) and is not bridged, so a guess never paints a
	// lane.
	LaneFieldMaxBridgeS = 0.5
	// LaneFieldHeadingBins is the number of 45° heading bins per cell,
	// clockwise from the sensor's +Y; bin b covers [45b, 45b + 45) degrees.
	LaneFieldHeadingBins = 8
)

// The class groups, in the order a field lists them.
const (
	LaneFieldVehicle    = "vehicle"
	LaneFieldCyclist    = "cyclist"
	LaneFieldPedestrian = "pedestrian"
)

// LaneFieldGroups are the class groups a field carries, in output order.
var LaneFieldGroups = []string{LaneFieldVehicle, LaneFieldCyclist, LaneFieldPedestrian}

// LaneFieldClassGroup maps an L6 label to its class group: vehicle for car,
// truck, bus and motorcyclist, and cyclist and pedestrian for themselves. Any
// other label, and an unclassified track, has no group and is left out.
func LaneFieldClassGroup(label string) (string, bool) {
	switch label {
	case "car", "truck", "bus", "motorcyclist":
		return LaneFieldVehicle, true
	case "cyclist":
		return LaneFieldCyclist, true
	case "pedestrian":
		return LaneFieldPedestrian, true
	}
	return "", false
}

// LaneFieldPart is the part of a run a field covers.
type LaneFieldPart string

// The three parts of a run: the whole, and its two halves by time, so
// Gerardus's two-window agreement test has independent evidence.
const (
	LaneFieldPartAll        LaneFieldPart = "all"
	LaneFieldPartFirstHalf  LaneFieldPart = "first_half"
	LaneFieldPartSecondHalf LaneFieldPart = "second_half"
)

// LaneFieldParts are the parts in the order a run produces them.
var LaneFieldParts = []LaneFieldPart{LaneFieldPartAll, LaneFieldPartFirstHalf, LaneFieldPartSecondHalf}

// ParseLaneFieldPart reads a part name; empty reads as all.
func ParseLaneFieldPart(s string) (LaneFieldPart, error) {
	switch LaneFieldPart(s) {
	case "", LaneFieldPartAll:
		return LaneFieldPartAll, nil
	case LaneFieldPartFirstHalf, LaneFieldPartSecondHalf:
		return LaneFieldPart(s), nil
	}
	return "", fmt.Errorf("part %q is not one of all, first_half, second_half", s)
}

// LaneFieldInstant is one observed instant of a track, in the sensor frame.
// Coasted and predicted instants must not be passed: the store writes only
// associated instants, and a caller reading anything else drops them first.
type LaneFieldInstant struct {
	TimeNs   int64
	X, Y     float64
	VX, VY   float64
	SpeedMps float64
	// The box at that instant: its centre, the heading of its length axis
	// (radians counter-clockwise from +X) and its extents. A zero length or
	// width means no converged box, and the instant counts in tracks only.
	BoxX, BoxY    float64
	BoxHeadingRad float64
	BoxLength     float64
	BoxWidth      float64
}

// LaneFieldTrack is one confirmed track's observed instants with its L6
// label. TrackID is used only to tell tracks apart; it never reaches the
// field.
type LaneFieldTrack struct {
	TrackID  string
	Class    string
	Instants []LaneFieldInstant
}

// LaneFieldOptions are the field's thresholds. The zero value is not valid;
// start from DefaultLaneFieldOptions.
type LaneFieldOptions struct {
	CellM                 float64
	K                     int
	MinInstants           int
	VehicleMinMaxSpeedMps float64
	HeadingMinSpeedMps    float64
	// MaxBridgeS is the longest step between consecutive instants whose path
	// counts; zero counts the sampled cells only.
	MaxBridgeS float64
}

// DefaultLaneFieldOptions are the plan's thresholds.
func DefaultLaneFieldOptions() LaneFieldOptions {
	return LaneFieldOptions{
		CellM:                 LaneFieldCellM,
		K:                     LaneFieldK,
		MinInstants:           LaneFieldMinInstants,
		VehicleMinMaxSpeedMps: LaneFieldVehicleMinMaxSpeedMps,
		HeadingMinSpeedMps:    LaneFieldHeadingMinSpeedMps,
		MaxBridgeS:            LaneFieldMaxBridgeS,
	}
}

// LaneFieldMeta is what a field's header says about where it came from. The
// window is the run's; zero ends are taken from the instants.
type LaneFieldMeta struct {
	L16Token          string
	SensorID          string
	RecordID          string
	CaptureRef        string
	WindowStartNs     int64
	WindowEndNs       int64
	PipelineBuild     string
	TuningFingerprint string
}

// LaneFieldHeader is Gerardus's LaneFieldHeader in the JSON form `gerardus
// upload-lane-field` reads, fields in its order. Origin is the south-west
// cell's centre in the sensor frame, [x, y]; cell (ix, iy) is centred at
// origin + (ix, iy) × cell_m. The window is RFC 3339, or empty when unknown.
type LaneFieldHeader struct {
	L16Token          string     `json:"l16_token"`
	SensorID          string     `json:"sensor_id"`
	RecordID          string     `json:"record_id"`
	CaptureRef        string     `json:"capture_ref"`
	WindowStart       string     `json:"window_start"`
	WindowEnd         string     `json:"window_end"`
	WindowPart        string     `json:"window_part"`
	CellM             float64    `json:"cell_m"`
	Origin            [2]float64 `json:"origin"`
	NX                uint32     `json:"nx"`
	NY                uint32     `json:"ny"`
	K                 uint32     `json:"k"`
	PipelineBuild     string     `json:"pipeline_build"`
	TuningFingerprint string     `json:"tuning_fingerprint"`
	TrackCount        uint32     `json:"track_count"`
}

// LaneFieldGroup is one class group's cells, sparse, as parallel arrays:
// Gerardus's LaneFieldCells. Heading has eight entries a cell.
type LaneFieldGroup struct {
	ClassGroup  string    `json:"class_group"`
	IX          []uint32  `json:"ix"`
	IY          []uint32  `json:"iy"`
	Tracks      []uint32  `json:"tracks"`
	BoxTracks   []uint32  `json:"box_tracks"`
	Heading     []uint32  `json:"heading"`
	SpeedSumMps []float32 `json:"speed_sum_mps"`
}

// LaneField is one part of one run's field.
type LaneField struct {
	Header LaneFieldHeader  `json:"header"`
	Groups []LaneFieldGroup `json:"groups"`
}

// JSON is the field as the route serves it and the CLI writes it: compact,
// fields in the contract's order, then a newline. The same field always
// gives the same bytes.
func (f LaneField) JSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(f); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CellCount is the number of cells the field carries in all groups.
func (f LaneField) CellCount() int {
	n := 0
	for _, g := range f.Groups {
		n += len(g.IX)
	}
	return n
}

type laneCellKey struct{ ix, iy int64 }

// laneCell accumulates one cell of one class group.
type laneCell struct {
	tracks, boxTracks uint32
	heading           [LaneFieldHeadingBins]uint32
	speedSum          float64
}

// laneTrackCell is one track's instants in one cell.
type laneTrackCell struct {
	n          int
	speedSum   float64
	sinSum     float64
	cosSum     float64
	movingSeen bool
}

// BuildLaneField sums the tracks into one part of a run's lane field. It is
// pure: the same tracks in any order give the same field. A track counts when
// it has at least MinInstants observed instants and a class group, and, for a
// vehicle, a lifetime maximum speed of at least VehicleMinMaxSpeedMps. A
// track's centre counts in the cell of each observed instant and in every
// cell its straight path crosses between two consecutive instants at most
// MaxBridgeS apart, with the step's mean speed and its direction of travel.
// Each counted track counts once per cell per channel however many instants
// it spent there. A cell is emitted only when its tracks or its box_tracks
// reaches K; the grid is the bounding box of the emitted cells, so nothing
// of a thinner cell reaches the header either.
func BuildLaneField(tracks []LaneFieldTrack, meta LaneFieldMeta, part LaneFieldPart, opts LaneFieldOptions) (LaneField, error) {
	if opts.CellM <= 0 || opts.K < 1 || opts.MinInstants < 1 {
		return LaneField{}, fmt.Errorf("lane field options need a cell size, k and a minimum instant count")
	}
	// An empty part is all, as ParseLaneFieldPart reads it.
	part, err := ParseLaneFieldPart(string(part))
	if err != nil {
		return LaneField{}, fmt.Errorf("lane field %w", err)
	}

	// Tracks in id order, each with its instants in time order, so float
	// sums are taken in one order whatever order the caller passed.
	byID := map[string]*LaneFieldTrack{}
	ids := []string{}
	for i := range tracks {
		t := tracks[i]
		if have, ok := byID[t.TrackID]; ok {
			have.Instants = append(have.Instants, t.Instants...)
			continue
		}
		cp := LaneFieldTrack{TrackID: t.TrackID, Class: t.Class, Instants: append([]LaneFieldInstant(nil), t.Instants...)}
		byID[t.TrackID] = &cp
		ids = append(ids, t.TrackID)
	}
	sort.Strings(ids)

	startNs, endNs := meta.WindowStartNs, meta.WindowEndNs
	if startNs == 0 || endNs == 0 {
		lo, hi := int64(math.MaxInt64), int64(math.MinInt64)
		for _, id := range ids {
			for _, in := range byID[id].Instants {
				lo = min(lo, in.TimeNs)
				hi = max(hi, in.TimeNs)
			}
		}
		if lo <= hi {
			if startNs == 0 {
				startNs = lo
			}
			if endNs == 0 {
				endNs = hi
			}
		}
	}
	midNs := startNs + (endNs-startNs)/2
	partStart, partEnd := startNs, endNs
	switch part {
	case LaneFieldPartFirstHalf:
		partEnd = midNs
	case LaneFieldPartSecondHalf:
		partStart = midNs
	}
	inPart := func(t int64) bool {
		switch part {
		case LaneFieldPartFirstHalf:
			return t < midNs
		case LaneFieldPartSecondHalf:
			return t >= midNs
		}
		return true
	}

	groups := map[string]map[laneCellKey]*laneCell{}
	var contributing uint32
	for _, id := range ids {
		t := byID[id]
		group, ok := LaneFieldClassGroup(t.Class)
		if !ok || len(t.Instants) < opts.MinInstants {
			continue
		}
		sort.SliceStable(t.Instants, func(i, j int) bool { return t.Instants[i].TimeNs < t.Instants[j].TimeNs })
		if group == LaneFieldVehicle {
			maxSpeed := 0.0
			for _, in := range t.Instants {
				maxSpeed = max(maxSpeed, in.SpeedMps)
			}
			if maxSpeed < opts.VehicleMinMaxSpeedMps {
				continue
			}
		}

		centre := map[laneCellKey]*laneTrackCell{}
		boxed := map[laneCellKey]struct{}{}
		seen := false
		// sample adds one speed and, when moving, one heading (vx, vy, any
		// scale) to a cell of this track.
		sample := func(k laneCellKey, speed, vx, vy float64) {
			c := centre[k]
			if c == nil {
				c = &laneTrackCell{}
				centre[k] = c
			}
			c.n++
			if finite(speed) {
				c.speedSum += speed
			}
			if speed >= opts.HeadingMinSpeedMps && finite(vx, vy) && (vx != 0 || vy != 0) {
				// Bearing clockwise from the sensor's +Y.
				b := math.Atan2(vx, vy)
				c.sinSum += math.Sin(b)
				c.cosSum += math.Cos(b)
				c.movingSeen = true
			}
		}
		var prev *LaneFieldInstant
		for i := range t.Instants {
			in := t.Instants[i]
			if !inPart(in.TimeNs) || !finite(in.X, in.Y) {
				continue
			}
			seen = true
			k := laneCellOf(in.X, in.Y, opts.CellM)
			// The cells the centre crossed since the previous instant,
			// between the two instants' own cells.
			if prev != nil && opts.MaxBridgeS > 0 && float64(in.TimeNs-prev.TimeNs) <= opts.MaxBridgeS*1e9 {
				path := laneSegmentCells(prev.X, prev.Y, in.X, in.Y, opts.CellM)
				if len(path) > 2 {
					speed := (prev.SpeedMps + in.SpeedMps) / 2
					dx, dy := in.X-prev.X, in.Y-prev.Y
					for _, pk := range path[1 : len(path)-1] {
						sample(pk, speed, dx, dy)
					}
				}
			}
			sample(k, in.SpeedMps, in.VX, in.VY)
			prev = &t.Instants[i]
			if in.BoxLength > 0 && in.BoxWidth > 0 && finite(in.BoxX, in.BoxY, in.BoxHeadingRad, in.BoxLength, in.BoxWidth) {
				for _, bk := range laneBoxCells(in, opts.CellM) {
					boxed[bk] = struct{}{}
				}
			}
		}
		if !seen {
			continue
		}
		contributing++

		cells := groups[group]
		if cells == nil {
			cells = map[laneCellKey]*laneCell{}
			groups[group] = cells
		}
		for _, k := range sortedLaneKeys(centre) {
			tc := centre[k]
			c := laneCellAt(cells, k)
			c.tracks++
			c.speedSum += tc.speedSum / float64(tc.n)
			if tc.movingSeen && (tc.sinSum != 0 || tc.cosSum != 0) {
				c.heading[laneHeadingBin(math.Atan2(tc.sinSum, tc.cosSum))]++
			}
		}
		for k := range boxed {
			laneCellAt(cells, k).boxTracks++
		}
	}

	// Keep the cells at k or more, and the grid that bounds them.
	k := uint32(opts.K)
	kept := map[string][]laneCellKey{}
	minX, minY := int64(math.MaxInt64), int64(math.MaxInt64)
	maxX, maxY := int64(math.MinInt64), int64(math.MinInt64)
	for g, cells := range groups {
		for key, c := range cells {
			if c.tracks < k && c.boxTracks < k {
				continue
			}
			kept[g] = append(kept[g], key)
			minX, minY = min(minX, key.ix), min(minY, key.iy)
			maxX, maxY = max(maxX, key.ix), max(maxY, key.iy)
		}
	}

	f := LaneField{
		Header: LaneFieldHeader{
			L16Token:          meta.L16Token,
			SensorID:          meta.SensorID,
			RecordID:          meta.RecordID,
			CaptureRef:        meta.CaptureRef,
			WindowStart:       laneFieldTime(partStart),
			WindowEnd:         laneFieldTime(partEnd),
			WindowPart:        string(part),
			CellM:             opts.CellM,
			K:                 k,
			PipelineBuild:     meta.PipelineBuild,
			TuningFingerprint: meta.TuningFingerprint,
			TrackCount:        contributing,
		},
		Groups: []LaneFieldGroup{},
	}
	if minX > maxX {
		return f, nil
	}
	f.Header.Origin = [2]float64{(float64(minX) + 0.5) * opts.CellM, (float64(minY) + 0.5) * opts.CellM}
	f.Header.NX = uint32(maxX - minX + 1)
	f.Header.NY = uint32(maxY - minY + 1)
	for _, g := range LaneFieldGroups {
		keys := kept[g]
		if len(keys) == 0 {
			continue
		}
		sortLaneKeys(keys)
		out := LaneFieldGroup{
			ClassGroup:  g,
			IX:          make([]uint32, 0, len(keys)),
			IY:          make([]uint32, 0, len(keys)),
			Tracks:      make([]uint32, 0, len(keys)),
			BoxTracks:   make([]uint32, 0, len(keys)),
			Heading:     make([]uint32, 0, LaneFieldHeadingBins*len(keys)),
			SpeedSumMps: make([]float32, 0, len(keys)),
		}
		for _, key := range keys {
			c := groups[g][key]
			out.IX = append(out.IX, uint32(key.ix-minX))
			out.IY = append(out.IY, uint32(key.iy-minY))
			out.Tracks = append(out.Tracks, c.tracks)
			out.BoxTracks = append(out.BoxTracks, c.boxTracks)
			out.Heading = append(out.Heading, c.heading[:]...)
			out.SpeedSumMps = append(out.SpeedSumMps, float32(c.speedSum))
		}
		f.Groups = append(f.Groups, out)
	}
	return f, nil
}

// BuildLaneFields builds the three parts of a run's field, in LaneFieldParts
// order.
func BuildLaneFields(tracks []LaneFieldTrack, meta LaneFieldMeta, opts LaneFieldOptions) ([]LaneField, error) {
	out := make([]LaneField, 0, len(LaneFieldParts))
	for _, p := range LaneFieldParts {
		f, err := BuildLaneField(tracks, meta, p, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func laneFieldTime(ns int64) string {
	if ns == 0 {
		return ""
	}
	return time.Unix(0, ns).UTC().Format(time.RFC3339Nano)
}

func laneCellOf(x, y, cellM float64) laneCellKey {
	return laneCellKey{int64(math.Floor(x / cellM)), int64(math.Floor(y / cellM))}
}

func laneCellAt(cells map[laneCellKey]*laneCell, k laneCellKey) *laneCell {
	c := cells[k]
	if c == nil {
		c = &laneCell{}
		cells[k] = c
	}
	return c
}

// laneSegmentCells are the cells a straight segment from (x0, y0) to (x1,
// y1) passes through, in order from the first point's cell to the last's
// (Amanatides and Woo's traversal). Where it passes exactly through a cell
// corner it steps along x first, so the path stays 4-connected.
func laneSegmentCells(x0, y0, x1, y1, cellM float64) []laneCellKey {
	a := laneCellOf(x0, y0, cellM)
	b := laneCellOf(x1, y1, cellM)
	out := []laneCellKey{a}
	if a == b {
		return out
	}
	dx, dy := x1-x0, y1-y0
	stepX, stepY := int64(0), int64(0)
	tMaxX, tMaxY := math.Inf(1), math.Inf(1)
	tDeltaX, tDeltaY := math.Inf(1), math.Inf(1)
	if dx > 0 {
		stepX = 1
		tMaxX = ((float64(a.ix)+1)*cellM - x0) / dx
		tDeltaX = cellM / dx
	} else if dx < 0 {
		stepX = -1
		tMaxX = (float64(a.ix)*cellM - x0) / dx
		tDeltaX = -cellM / dx
	}
	if dy > 0 {
		stepY = 1
		tMaxY = ((float64(a.iy)+1)*cellM - y0) / dy
		tDeltaY = cellM / dy
	} else if dy < 0 {
		stepY = -1
		tMaxY = (float64(a.iy)*cellM - y0) / dy
		tDeltaY = -cellM / dy
	}
	cur := a
	// The cells between differ from a to b by at most |Δix| + |Δiy| steps.
	steps := absInt64(b.ix-a.ix) + absInt64(b.iy-a.iy)
	for n := int64(0); n < steps && cur != b; n++ {
		if tMaxX <= tMaxY {
			cur.ix += stepX
			tMaxX += tDeltaX
		} else {
			cur.iy += stepY
			tMaxY += tDeltaY
		}
		out = append(out, cur)
	}
	if out[len(out)-1] != b {
		// Rounding at a cell edge: end on the last point's own cell.
		out = append(out, b)
	}
	return out
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// laneHeadingBin is the 45° bin of a bearing in radians clockwise from +Y:
// bin b covers [45b, 45b + 45) degrees, as Gerardus's bin centres say.
func laneHeadingBin(bearingRad float64) int {
	deg := math.Mod(bearingRad*180/math.Pi, 360)
	if deg < 0 {
		deg += 360
	}
	b := int(deg / 45)
	if b >= LaneFieldHeadingBins {
		b = 0
	}
	return b
}

// laneBoxCells are the cells whose centres the instant's box covers: the
// rectangle of its length along its heading and its width across.
func laneBoxCells(in LaneFieldInstant, cellM float64) []laneCellKey {
	ux, uy := math.Cos(in.BoxHeadingRad), math.Sin(in.BoxHeadingRad)
	hl, hw := in.BoxLength/2, in.BoxWidth/2
	// Corners, for the index range to test.
	minX, maxX := math.Inf(1), math.Inf(-1)
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, s := range [4][2]float64{{1, 1}, {1, -1}, {-1, -1}, {-1, 1}} {
		x := in.BoxX + s[0]*hl*ux - s[1]*hw*uy
		y := in.BoxY + s[0]*hl*uy + s[1]*hw*ux
		minX, maxX = math.Min(minX, x), math.Max(maxX, x)
		minY, maxY = math.Min(minY, y), math.Max(maxY, y)
	}
	const eps = 1e-9
	var out []laneCellKey
	for ix := int64(math.Floor(minX / cellM)); ix <= int64(math.Floor(maxX/cellM)); ix++ {
		for iy := int64(math.Floor(minY / cellM)); iy <= int64(math.Floor(maxY/cellM)); iy++ {
			dx := (float64(ix)+0.5)*cellM - in.BoxX
			dy := (float64(iy)+0.5)*cellM - in.BoxY
			along := dx*ux + dy*uy
			across := -dx*uy + dy*ux
			if math.Abs(along) <= hl+eps && math.Abs(across) <= hw+eps {
				out = append(out, laneCellKey{ix, iy})
			}
		}
	}
	return out
}

func sortedLaneKeys(m map[laneCellKey]*laneTrackCell) []laneCellKey {
	keys := make([]laneCellKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortLaneKeys(keys)
	return keys
}

// sortLaneKeys orders cells by ix, then iy.
func sortLaneKeys(keys []laneCellKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ix != keys[j].ix {
			return keys[i].ix < keys[j].ix
		}
		return keys[i].iy < keys[j].iy
	})
}

func finite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}
