package l3grid

import (
	"math"
	"time"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// The track guard keeps the objects L5 is tracking out of the background
// (the v0.5.13 backlog item: background/foreground separation leaks moving
// objects). On kirk0 with the guard off, 42 % of the returns inside observed
// confirmed tracks' boxes were published as background, and 13 % of the
// settled background lay inside a box some track had drawn
// (docs/lidar/operations/l3-track-guard-kirk-2026-10.md). Two things put
// them there:
//
//   - a vehicle's return inside its cell's acceptance window is classed
//     background, and the windows on the road reach metres: almost all such
//     returns were in cells running a region override, most windows were 4 m
//     or more, and three quarters of the returns were more than 0.3 m in front
//     of the background the cell had learned;
//   - a return classed background updates the cell, at the boosted
//     re-acquisition rate when the cell saw foreground recently, so the
//     traffic pulls the cell's range towards the vehicles and widens its
//     spread, which widens the window, which admits more of the traffic.
//
// The guard answers both with what L5 knows and L3 cannot: where the tracked
// objects are.
//
//   - Hold: a return inside a confirmed track's box, carried to the frame by
//     the track's velocity and grown by HoldMarginMetres, leaves its cell
//     unchanged. The cell neither learns it as background nor loses
//     confidence to it: an object L5 is tracking says nothing about the
//     surface behind it. The return is still classed by the cell's model, so
//     L4's input is the one the model gives.
//   - Cool-down: a cell that held a return stays unchanged for
//     HoldCooldownNanos after, so the tail of a vehicle the box did not reach
//     and the first returns behind it cannot teach the cell the vehicle.
//   - Edge follow: the published classification (the full frames a site
//     model accumulates, and the foreground streamed) takes the class of the
//     object a return belongs to. A return the cell classed background inside
//     a held box, or inside the box of a cluster associated with a confirmed
//     track or that track's box this frame, grown by EdgeMarginMetres, is
//     published as foreground (FollowBoxes). Only the output moves: neither
//     the model nor L4's input.
//
// No box is grown downwards: what lies under its base is the ground the
// object stands on.

// TrackGuardParams configures the track guard. Nil in BackgroundParams is
// off, as every L3 before it ran.
type TrackGuardParams struct {
	// HoldTracks leaves a cell unchanged by a return inside a held box.
	HoldTracks bool
	// HoldMarginMetres grows each held box on every side in the plane and
	// above its top. It covers the box's lag on a vehicle that moved since
	// the track was updated and the returns of the same surfaces just outside
	// the cluster the box was fitted to.
	HoldMarginMetres float64
	// HoldCooldownNanos is how long a cell stays unchanged after it last held
	// a return. Zero holds only the returns inside a box.
	HoldCooldownNanos int64
	// HoldMinSpeedMps holds only the tracks moving at least this fast (the
	// pipeline filters the boxes it hands SetTrackHolds). Held still, a
	// static object tracked as one would keep itself foreground for good.
	HoldMinSpeedMps float64
	// EdgeFollowCluster publishes as foreground the background returns
	// inside held boxes and inside the boxes of tracked clusters
	// (FollowBoxes, called by the pipeline after tracking).
	EdgeFollowCluster bool
	// EdgeMarginMetres grows the boxes of EdgeFollowCluster.
	EdgeMarginMetres float64
}

// TrackGuardParamsFromTuning is the tuning block's guard; nil without the
// block. The block's cool-down is validated by the config loader, so one that
// fails to parse here reads as none.
func TrackGuardParamsFromTuning(t *config.L3TrackGuard) *TrackGuardParams {
	if t == nil {
		return nil
	}
	cooldown, err := time.ParseDuration(t.HoldCooldown)
	if err != nil {
		cooldown = 0
	}
	return &TrackGuardParams{
		HoldTracks: t.HoldTracks, HoldMarginMetres: t.HoldMarginMetres,
		HoldCooldownNanos: cooldown.Nanoseconds(), HoldMinSpeedMps: t.HoldMinSpeedMps,
		EdgeFollowCluster: t.EdgeFollowCluster, EdgeMarginMetres: t.EdgeMarginMetres,
	}
}

// TrackGuardTuningFromParams is the tuning block for the guard a manager
// runs, for reading the running configuration back; nil when it runs none.
func TrackGuardTuningFromParams(p *TrackGuardParams) *config.L3TrackGuard {
	if p == nil {
		return nil
	}
	return &config.L3TrackGuard{
		HoldTracks: p.HoldTracks, HoldMarginMetres: p.HoldMarginMetres,
		HoldCooldown:      time.Duration(p.HoldCooldownNanos).String(),
		HoldMinSpeedMps:   p.HoldMinSpeedMps,
		EdgeFollowCluster: p.EdgeFollowCluster, EdgeMarginMetres: p.EdgeMarginMetres,
	}
}

func (p *TrackGuardParams) clone() *TrackGuardParams {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

// holdMaxAgeNanos is how long a set of held boxes guards the frames after it
// was given. The pipeline gives a new set after every tracker update; frames
// that end before the tracker (no foreground, no clusters) are guarded by the
// last set, carried on by each box's velocity. Half a second is five frames,
// longer than the gaps a passing vehicle leaves and short enough that a set
// left behind by a replay seek or a paused tracker stops guarding anything.
const holdMaxAgeNanos = int64(500 * time.Millisecond)

// GuardBox is an object's box for the track guard, in the sensor frame: the
// frame L3's polar returns convert to.
type GuardBox struct {
	// X, Y, Z is the box's centre.
	X, Y, Z float64
	// Length is along HeadingRad, measured from +X towards +Y; Width across it.
	Length, Width, Height float64
	HeadingRad            float64
	// VX, VY carry the box to the frame it guards (metres per second).
	VX, VY float64
}

// preparedBox is a GuardBox carried to the frame and grown, with what the
// per-return test needs.
type preparedBox struct {
	x, y, sin, cos     float64
	halfL, halfW       float64
	zMin, zMax         float64
	radius             float64
	binFirst, binCount int
}

func prepareBox(b GuardBox, dt, margin float64, azBins int) (preparedBox, bool) {
	if !(b.Length > 0) || !(b.Width > 0) || !(b.Height > 0) {
		return preparedBox{}, false
	}
	if margin < 0 {
		margin = 0
	}
	s, c := math.Sincos(b.HeadingRad)
	p := preparedBox{
		x: b.X + b.VX*dt, y: b.Y + b.VY*dt, sin: s, cos: c,
		halfL: b.Length/2 + margin, halfW: b.Width/2 + margin,
		zMin: b.Z - b.Height/2, zMax: b.Z + b.Height/2 + margin,
	}
	if math.IsNaN(p.x) || math.IsNaN(p.y) || math.IsInf(p.x, 0) || math.IsInf(p.y, 0) {
		return preparedBox{}, false
	}
	p.radius = math.Hypot(p.halfL, p.halfW)
	// The azimuth bins the box covers, seen from the sensor: the bins of the
	// circle about it. A box over the sensor covers every bin.
	dist := math.Hypot(p.x, p.y)
	if dist <= p.radius || azBins <= 0 {
		p.binFirst, p.binCount = 0, azBins
		return p, true
	}
	centre := math.Atan2(p.x, p.y) * 180 / math.Pi // X = sin(az), Y = cos(az)
	half := math.Asin(p.radius/dist) * 180 / math.Pi
	binDeg := 360.0 / float64(azBins)
	first := int(math.Floor((centre - half) / binDeg))
	last := int(math.Floor((centre + half) / binDeg))
	p.binFirst = ((first % azBins) + azBins) % azBins
	p.binCount = last - first + 1
	if p.binCount > azBins {
		p.binCount = azBins
	}
	return p, true
}

func (p *preparedBox) contains(x, y, z float64) bool {
	if z < p.zMin || z > p.zMax {
		return false
	}
	dx, dy := x-p.x, y-p.y
	if dx*dx+dy*dy > p.radius*p.radius {
		return false
	}
	along := dx*p.cos + dy*p.sin
	across := -dx*p.sin + dy*p.cos
	return math.Abs(along) <= p.halfL && math.Abs(across) <= p.halfW
}

// boxSet is a frame's prepared boxes and the azimuth bins any of them covers.
type boxSet struct {
	boxes []preparedBox
	bins  []bool
}

func (s *boxSet) prepare(boxes []GuardBox, dt, margin float64, azBins int) {
	s.boxes = s.boxes[:0]
	if cap(s.bins) < azBins {
		s.bins = make([]bool, azBins)
	}
	s.bins = s.bins[:azBins]
	clear(s.bins)
	for _, b := range boxes {
		p, ok := prepareBox(b, dt, margin, azBins)
		if !ok {
			continue
		}
		s.boxes = append(s.boxes, p)
		for k := 0; k < p.binCount; k++ {
			s.bins[(p.binFirst+k)%azBins] = true
		}
	}
}

// azBin is the bin of an azimuth in degrees, any value.
func azBin(azimuth float64, azBins int) int {
	az := math.Mod(azimuth, 360.0)
	if az < 0 {
		az += 360.0
	}
	bin := int((az / 360.0) * float64(azBins))
	if bin >= azBins {
		bin = azBins - 1
	}
	return bin
}

// holds reports whether any box holds the return, which lies in azimuth bin
// bin.
func (s *boxSet) holds(p PointPolar, bin int) bool {
	if len(s.boxes) == 0 || !s.bins[bin] {
		return false
	}
	x, y, z := l2frames.SphericalToCartesian(p.Distance, p.Azimuth, p.Elevation)
	for i := range s.boxes {
		if s.boxes[i].contains(x, y, z) {
			return true
		}
	}
	return false
}

// SetTrackHolds gives the track guard the boxes of the confirmed tracks as
// they stood at time at, for the frames that follow; it replaces the set
// given before. The pipeline calls it after every tracker update. Nothing is
// held unless BackgroundParams.TrackGuard has HoldTracks set.
func (bm *BackgroundManager) SetTrackHolds(boxes []GuardBox, at time.Time) {
	if bm == nil || bm.Grid == nil {
		return
	}
	g := bm.Grid
	g.mu.Lock()
	defer g.mu.Unlock()
	bm.trackHolds = append(bm.trackHolds[:0], boxes...)
	bm.trackHoldsAt = at.UnixNano()
}

// TrackHeldMask is the last frame's held returns, indexed as its points:
// true for a return inside a held box. Nil when the frame held nothing. The
// slice is reused by the next frame.
func (bm *BackgroundManager) TrackHeldMask() []bool {
	if bm == nil || bm.Grid == nil {
		return nil
	}
	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()
	if !bm.heldAny {
		return nil
	}
	return bm.heldMask
}

// trackGuardFrame is the guard's state for one frame of
// ProcessFramePolarWithMaskAt.
type trackGuardFrame struct {
	on       bool
	cooldown int64
	set      *boxSet
	held     []bool
	until    []int64
}

// beginTrackGuardLocked prepares the guard for a frame of n returns at
// nowNanos. Caller holds g.mu.
func (bm *BackgroundManager) beginTrackGuardLocked(n int, nowNanos int64) trackGuardFrame {
	g := bm.Grid
	bm.heldAny = false
	guard := g.Params.TrackGuard
	if guard == nil || !guard.HoldTracks {
		return trackGuardFrame{}
	}
	if len(bm.heldUntil) != len(g.Cells) {
		bm.heldUntil = make([]int64, len(g.Cells))
	}
	if cap(bm.heldMask) < n {
		bm.heldMask = make([]bool, n)
	}
	bm.heldMask = bm.heldMask[:n]
	clear(bm.heldMask)
	var boxes []GuardBox
	dt := float64(nowNanos-bm.trackHoldsAt) / 1e9
	if age := nowNanos - bm.trackHoldsAt; age >= 0 && age <= holdMaxAgeNanos {
		boxes = bm.trackHolds
	} else {
		dt = 0
	}
	bm.holdSet.prepare(boxes, dt, guard.HoldMarginMetres, g.AzimuthBins)
	cooldown := guard.HoldCooldownNanos
	if cooldown < 0 {
		cooldown = 0
	}
	return trackGuardFrame{on: true, cooldown: cooldown, set: &bm.holdSet, held: bm.heldMask, until: bm.heldUntil}
}

// readOnly reports whether return i, in cell cellIdx and azimuth bin bin,
// must leave its cell unchanged: it is inside a held box, or the cell held
// one within the cool-down. A held return starts the cell's cool-down.
func (f *trackGuardFrame) readOnly(i int, p PointPolar, cellIdx, bin int, nowNanos int64, bm *BackgroundManager) bool {
	if !f.on {
		return false
	}
	if f.set.holds(p, bin) {
		f.held[i] = true
		bm.heldAny = true
		f.until[cellIdx] = nowNanos + f.cooldown
		return true
	}
	return f.until[cellIdx] > nowNanos
}

// followBins is the azimuth resolution of FollowBoxes' first test: a box
// covers a few of these, so a return in any other is passed at once.
const followBins = 720

// FollowBoxes publishes as foreground the background returns of a frame
// that lie inside any of the boxes grown by margin (and not below a box's
// base): the edge of a tracked object that its cells classed background. It
// writes mask in place, which must be indexed as points, and returns the
// number of returns it changed. It reads no state and changes no cell.
func FollowBoxes(points []PointPolar, mask []bool, boxes []GuardBox, margin float64) int {
	if len(boxes) == 0 || len(points) != len(mask) {
		return 0
	}
	var set boxSet
	set.prepare(boxes, 0, margin, followBins)
	if len(set.boxes) == 0 {
		return 0
	}
	flipped := 0
	for i, p := range points {
		if mask[i] {
			continue
		}
		if set.holds(p, azBin(p.Azimuth, followBins)) {
			mask[i] = true
			flipped++
		}
	}
	return flipped
}

// PublishedMask is the frame's mask as the pipeline publishes it under
// TrackGuard.EdgeFollowCluster: the per-return mask with every held return
// foreground. It copies mask into buf, grown as needed, and returns the copy;
// with the option off it returns mask itself.
func (bm *BackgroundManager) PublishedMask(mask, buf []bool) []bool {
	if bm == nil || bm.Grid == nil || mask == nil {
		return mask
	}
	g := bm.Grid
	g.mu.RLock()
	defer g.mu.RUnlock()
	guard := g.Params.TrackGuard
	if guard == nil || !guard.EdgeFollowCluster {
		return mask
	}
	buf = append(buf[:0], mask...)
	if bm.heldAny && len(bm.heldMask) == len(buf) {
		for i, h := range bm.heldMask {
			if h {
				buf[i] = true
			}
		}
	}
	return buf
}
