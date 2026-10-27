package main

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// l3bg0024's lower-body block: where a vehicle's lower body goes. For each
// scored frame of a confirmed, moving, vehicle-sized track, with the blended
// ground surface fitted, it finds the rings whose beam meets the box's near
// face between the surface and the track's lowest foreground return, the
// height at which the body should have returned something, and reads what
// each such ring returned in each azimuth slot of the box's span. It reads
// the frame's own returns and the published mask, not the clusters.
const (
	lowerBodyBinMetres     = 10.0
	lowerBodyBins          = 8 // 0 to 80 m; farther frames join the last bin
	lowerBodyMovingMps     = 2.0
	lowerBodyVehicleMetres = 3.0
	lowerBodyFarMetres     = 20.0
	lowerBodyMinRange      = 2.0
	lowerBodyMaxChannels   = 64
	lowerBodyDefaultBins   = 1800
)

// The classes of a ring's azimuth slot that should have met the lower body,
// read from the slot's nearest return.
const (
	// lbAbsorbed is background at the body's range, from the near face to
	// the far side, within the L3 closeness window of it.
	lbAbsorbed = "absorbed"
	// lbOccluded is background short of the body by more than the window:
	// something nearer took the beam.
	lbOccluded = "occluded"
	// lbBehind is background beyond the body's far side by more than the
	// window: the beam passed under or beside the body.
	lbBehind = "behind"
	// lbDropout is no return in the slot.
	lbDropout = "dropout"
	// lbClipped is foreground below L4's floor.
	lbClipped = "clipped"
	// lbForeground is foreground above the floor that the track's lowest
	// return does not reach: noise, or another cluster.
	lbForeground = "foreground"
)

var lowerBodyClasses = []string{lbAbsorbed, lbOccluded, lbBehind, lbDropout, lbClipped, lbForeground}

type lowerBodyAgg struct {
	trackFrames int
	heights     []float64
	slots       int
	counts      map[string]int
}

func newLowerBodyAgg() *lowerBodyAgg { return &lowerBodyAgg{counts: map[string]int{}} }

// lowerBody accumulates the block over a replay's scored frames.
type lowerBody struct {
	bins []*lowerBodyAgg
	far  *lowerBodyAgg
	all  *lowerBodyAgg
	// withoutSurface counts scored vehicle frames before a surface was
	// fitted, or with no reader of it to fit one.
	withoutSurface int
	nearest        []int32
}

func newLowerBody() *lowerBody {
	lb := &lowerBody{far: newLowerBodyAgg(), all: newLowerBodyAgg()}
	for i := 0; i < lowerBodyBins; i++ {
		lb.bins = append(lb.bins, newLowerBodyAgg())
	}
	return lb
}

// lowerBodyVehicle says whether a confirmed track counts this frame: observed
// in it, moving, and drawn vehicle-sized.
func lowerBodyVehicle(t *l5tracks.TrackedObject) bool {
	if t == nil || t.Misses != 0 || float64(t.Speed()) < lowerBodyMovingMps {
		return false
	}
	return float64(t.PublishedBox().Length) >= lowerBodyVehicleMetres
}

func (lb *lowerBody) frame(f replayeval.FrameTapFrame) {
	if !f.Scored || f.Frame == nil || len(f.Mask) != len(f.Frame.Points) {
		return
	}
	var vehicles []*l5tracks.TrackedObject
	for _, t := range f.Tracks {
		if lowerBodyVehicle(t) {
			vehicles = append(vehicles, t)
		}
	}
	if len(vehicles) == 0 {
		return
	}
	if f.Surface == nil {
		lb.withoutSurface += len(vehicles)
		return
	}
	bins := f.AzimuthBins
	if bins <= 0 {
		bins = lowerBodyDefaultBins
	}
	res := 360.0 / float64(bins)
	// The nearest return of each channel in each azimuth slot, and each
	// channel's elevation.
	n := lowerBodyMaxChannels * bins
	if cap(lb.nearest) < n {
		lb.nearest = make([]int32, n)
	}
	nearest := lb.nearest[:n]
	for i := range nearest {
		nearest[i] = -1
	}
	var elev [lowerBodyMaxChannels]float64
	var seen [lowerBodyMaxChannels]bool
	pts := f.Frame.Points
	for i := range pts {
		p := &pts[i]
		c := p.Channel - 1
		if c < 0 || c >= lowerBodyMaxChannels || p.Distance <= 0 {
			continue
		}
		elev[c], seen[c] = p.Elevation, true
		slot := int(math.Floor(p.Azimuth/res)) % bins
		if slot < 0 {
			slot += bins
		}
		k := c*bins + slot
		if j := nearest[k]; j < 0 || p.Distance < pts[j].Distance {
			nearest[k] = int32(i)
		}
	}
	bg := f.Background
	window := func(rng float64) float64 {
		return l3grid.ClosenessThresholdMetres(float64(bg.ClosenessSensitivityMultiplier), 0,
			float64(bg.NoiseRelativeFraction), rng, float64(bg.SafetyMarginMetres))
	}
	for _, t := range vehicles {
		b := t.PublishedBox()
		cx, cy := float64(b.X), float64(b.Y)
		rc := math.Hypot(cx, cy)
		if rc < lowerBodyMinRange {
			continue
		}
		bin := int(rc / lowerBodyBinMetres)
		if bin >= lowerBodyBins {
			bin = lowerBodyBins - 1
		}
		aggs := []*lowerBodyAgg{lb.bins[bin], lb.all}
		if rc >= lowerBodyFarMetres {
			aggs = append(aggs, lb.far)
		}
		zLow := float64(t.LatestZ)
		above := zLow - f.Surface.BlendedHeightAt(cx, cy)
		for _, a := range aggs {
			a.trackFrames++
			a.heights = append(a.heights, above)
		}
		lo, hi, ok := azimuthSpan(b)
		if !ok {
			continue
		}
		for s := int(math.Floor(lo / res)); float64(s)*res <= hi; s++ {
			az := (float64(s) + 0.5) * res
			tin, tout, hit := rayFootprint(b, az)
			if !hit {
				continue
			}
			slot := s % bins
			if slot < 0 {
				slot += bins
			}
			ux, uy := math.Sin(az*math.Pi/180), math.Cos(az*math.Pi/180)
			ground := f.Surface.BlendedHeightAt(tin*ux, tin*uy)
			for c := 0; c < lowerBodyMaxChannels; c++ {
				if !seen[c] {
					continue
				}
				// The beam's height where it meets the near face.
				z := tin * math.Tan(elev[c]*math.Pi/180)
				if z <= ground || z >= zLow {
					continue
				}
				class := lbDropout
				if i := nearest[c*bins+slot]; i >= 0 {
					p := &pts[i]
					switch {
					case f.Mask[i] && p.Z < f.HeightBandFloor:
						class = lbClipped
					case f.Mask[i]:
						class = lbForeground
					default:
						rh, w := math.Hypot(p.X, p.Y), window(p.Distance)
						switch {
						case rh < tin-w:
							class = lbOccluded
						case rh > tout+w:
							class = lbBehind
						default:
							class = lbAbsorbed
						}
					}
				}
				for _, a := range aggs {
					a.slots++
					a.counts[class]++
				}
			}
		}
	}
}

// azimuthSpan is the arc of azimuths (degrees, the frame's convention:
// clockwise from +Y towards +X) the box's footprint covers, as lo <= hi
// around the centre's azimuth; ok is false for a box over the sensor.
func azimuthSpan(b l5tracks.PublishedBox) (lo, hi float64, ok bool) {
	cx, cy := float64(b.X), float64(b.Y)
	centre := math.Atan2(cx, cy) * 180 / math.Pi
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, p := range footprint(b) {
		if math.Hypot(p[0], p[1]) < lowerBodyMinRange/2 {
			return 0, 0, false
		}
		d := math.Atan2(p[0], p[1])*180/math.Pi - centre
		d = math.Mod(d+540, 360) - 180
		lo, hi = math.Min(lo, centre+d), math.Max(hi, centre+d)
	}
	if hi-lo >= 180 {
		return 0, 0, false
	}
	return lo, hi, true
}

// footprint is the box's four corners in the plane.
func footprint(b l5tracks.PublishedBox) [4][2]float64 {
	s, c := math.Sincos(float64(b.HeadingRad))
	hl, hw := float64(b.Length)/2, float64(b.Width)/2
	var out [4][2]float64
	for i, k := range [4][2]float64{{hl, hw}, {hl, -hw}, {-hl, -hw}, {-hl, hw}} {
		out[i] = [2]float64{float64(b.X) + k[0]*c - k[1]*s, float64(b.Y) + k[0]*s + k[1]*c}
	}
	return out
}

// rayFootprint is where the ray from the sensor at azimuth az (degrees)
// enters and leaves the box's footprint, as horizontal ranges.
func rayFootprint(b l5tracks.PublishedBox, az float64) (tin, tout float64, hit bool) {
	ux, uy := math.Sin(az*math.Pi/180), math.Cos(az*math.Pi/180)
	s, c := math.Sincos(float64(b.HeadingRad))
	// The ray in the box's frame: origin and direction along and across.
	ox, oy := -float64(b.X), -float64(b.Y)
	o := [2]float64{ox*c + oy*s, -ox*s + oy*c}
	d := [2]float64{ux*c + uy*s, -ux*s + uy*c}
	half := [2]float64{float64(b.Length) / 2, float64(b.Width) / 2}
	tin, tout = 0, math.Inf(1)
	for k := 0; k < 2; k++ {
		if math.Abs(d[k]) < 1e-12 {
			if math.Abs(o[k]) > half[k] {
				return 0, 0, false
			}
			continue
		}
		t1, t2 := (-half[k]-o[k])/d[k], (half[k]-o[k])/d[k]
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tin, tout = math.Max(tin, t1), math.Min(tout, t2)
		if tin > tout {
			return 0, 0, false
		}
	}
	return tin, tout, tout > 0
}

// LowerBodySummary is the block's output for a case.
type LowerBodySummary struct {
	Definitions map[string]string `json:"definitions"`
	BinMetres   float64           `json:"bin_metres"`
	// WithoutSurface counts scored vehicle frames read before a surface was
	// fitted; with no reader of the surface (box_ground) it is every one.
	WithoutSurface int            `json:"vehicle_frames_without_surface"`
	All            LowerBodyBin   `json:"all"`
	Far            LowerBodyBin   `json:"from_20m"`
	Bins           []LowerBodyBin `json:"bins"`
}

// LowerBodyBin is one range bin, by the box centre's horizontal range.
type LowerBodyBin struct {
	FromMetres  float64 `json:"from_metres"`
	ToMetres    float64 `json:"to_metres,omitempty"`
	TrackFrames int     `json:"track_frames"`
	// LowestAboveSurface is the track's lowest foreground return less the
	// blended surface under the box's centre: p10, p50, p90.
	LowestAboveSurface [3]float64 `json:"lowest_above_surface_p10_p50_p90"`
	// Slots counts ring-slots that should have met the lower body, and
	// Counts and Shares what they returned.
	Slots  int                `json:"ring_slots"`
	Counts map[string]int     `json:"counts"`
	Shares map[string]float64 `json:"shares"`
}

func (a *lowerBodyAgg) bin(from, to float64) LowerBodyBin {
	out := LowerBodyBin{FromMetres: from, ToMetres: to, TrackFrames: a.trackFrames, Slots: a.slots,
		Counts: map[string]int{}, Shares: map[string]float64{}}
	if len(a.heights) > 0 {
		out.LowestAboveSurface = [3]float64{quantile(a.heights, .1), quantile(a.heights, .5), quantile(a.heights, .9)}
	}
	for _, c := range lowerBodyClasses {
		out.Counts[c] = a.counts[c]
		if a.slots > 0 {
			out.Shares[c] = float64(a.counts[c]) / float64(a.slots)
		}
	}
	return out
}

func (lb *lowerBody) summary() *LowerBodySummary {
	s := &LowerBodySummary{BinMetres: lowerBodyBinMetres, WithoutSurface: lb.withoutSurface,
		All: lb.all.bin(0, 0), Far: lb.far.bin(lowerBodyFarMetres, 0), Definitions: map[string]string{
			"track_frame": "a scored frame of a confirmed track observed in it, moving at 2 m/s or more, whose published box is 3 m long or more",
			"ring_slot":   "an azimuth slot of the L3 grid inside the box's footprint span, on a ring whose beam meets the footprint's near face above the blended surface and below the track's lowest foreground return",
			lbAbsorbed:    "the slot's nearest return is background, between the near face less the L3 closeness window (the settled cell's: spread 0) and the far side plus it",
			lbOccluded:    "background, nearer than the near face by more than the window",
			lbBehind:      "background, beyond the far side by more than the window: the beam passed under or beside the body",
			lbDropout:     "no return in the slot",
			lbClipped:     "foreground below L4's height-band floor",
			lbForeground:  "foreground above the floor",
		}}
	for i, a := range lb.bins {
		to := float64(i+1) * lowerBodyBinMetres
		if i == len(lb.bins)-1 {
			to = 0
		}
		s.Bins = append(s.Bins, a.bin(float64(i)*lowerBodyBinMetres, to))
	}
	return s
}
