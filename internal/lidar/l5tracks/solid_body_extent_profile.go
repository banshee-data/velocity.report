package l5tracks

// ExtentProfile (SolidBodyOptions.ExtentProfile): a body-frame occupancy
// profile of each solid body's extent, as a shadow of the shadow. geom0019
// scores it on kirk0 against the extent state and the published box; the
// form follows Schäfer, Alrifaee and Hashemi 2026, who keep a one-dimensional
// occupancy grid per dimension in the object frame, with lock-on, instead of
// one number per frame.
//
// It changes nothing: no track, no solid-body state, no published box. It
// reads the solid body after its update and the frame's returns, and writes
// only its own grids and the reading's Profile block.
//
// The grids. Two one-dimensional grids of profileCellMetres cells in the body
// frame, origin at the solid body's reference (its position, the body centre
// it believes) and axis its orientation: along the axis from
// -profileAlongHalfCells to +profileAlongHalfCells cells, and across it from
// -profileAcrossHalfCells to +profileAcrossHalfCells. Positive along is the
// orientation's direction (the front), positive across its left. The grid's
// axis keeps its sense from frame to frame: an orientation that flips by pi
// is read as the same axis, so a direction flip does not mirror the profile.
// Each cell holds a binary Bayes log-odds, prior 0, clamped to
// [profileMinLogOdds, profileMaxLogOdds] so that a cell stays revisable.
//
// Occupied. A cell takes profileOccupiedLogOdds on a frame where one or more
// of the cluster's returns in the height band fall in it: along by the
// return's body-frame coordinate along the axis, across by its coordinate
// across, each a marginal of the other. The band is profileBandLowMetres to
// profileBandHighMetres above the cluster's lowest return: the lowest
// decimetres are where ground returns and spray survive foreground
// extraction.
//
// Free. The ray model is two-dimensional, from the sensor origin
// (SolidBodyOptions.SensorX, SensorY), by azimuth column of
// profileColumnDeg, about two of the Pandar40P's 0.2-degree firings at 10 Hz.
// Each column holds the range of the nearest in-band return of any of the
// frame's clusters, so a vehicle in front of this one shadows it. A
// body-frame point is crossed without a return when its range is
// profileShadowMarginMetres or more short of its column's nearest return,
// or when its column holds no foreground return and lies outside the bearings
// of the track's own cluster and of the profile's locked footprint at this
// pose. Inside those bearings a column without a return is unknown, not free:
// glass, dark paint and sparse rings leave gaps in a body, a grazing side's
// returns can be too sparse for DBSCAN to keep, and a gap is not evidence the
// body is absent. So inside its own footprint the profile is freed only by a
// return beyond the point, and in practice it grows or holds; its ends are
// bounded by free cells outside the footprint and between it and the sensor.
// The free test samples each strip inside its ends by
// profileStripMarginMetres, so that an extent a smear has widened does not put
// the sample off the body. The model takes the
// sensor's vertical fan to cross the band at every range the grid covers,
// which holds for the Pandar40P beyond a few metres; it does not see static
// background, so a parked car shadowing an end is read as free, a named
// limitation. A cell along the axis is free when a crossed point lies in it
// with its across coordinate within the across extent (the locked extent, or
// before lock-on the frame's in-band span), since a rigid body present at that
// distance along would occupy every point across it; the across grid
// likewise against the along extent. A cell occupied on the frame is not also
// freed. A ray that passes over a low bonnet does not free the bonnet's cell
// while any return in its column, at any height in the band, lies nearer.
//
// Admission. A frame updates the profile only while the body is referenced to
// its centre, past initialising, and has a believed axis (profileAxis): its
// orientation, or at speed its course, with a standard deviation of at most
// axisBelievedSigmaRad (5 degrees), the orientation only where the course
// does not contradict it. A wrong heading smears occupancy across the axis,
// where it reads as width. Every other frame is refused and counted. Under
// the face-state options the orientation is the tracked heading, whose
// variance is its jitter's; the first build took the orientation alone and
// admitted almost no frame on the placement segments.
//
// The reading. The extent along each axis is the span from the lowest to the
// highest cell at or above profileLockOnLogOdds, and each of its ends is
// observed when a cell within profileEndMarginMetres beyond it reads
// profileEndFreeLogOdds or less, and censored otherwise: a censored end is a
// lower bound on how far the body reaches. The profile is Known once both
// grids have a locked cell.
//
// Withheld frames (geom0019's P4). Every profileWithholdEvery-th admitted
// frame updates nothing and is marked Withheld, so a scorer can test the
// profile against returns it has not seen.
//
// Cost and memory. The grids are fixed-size, (2·150 + 2·50) float32 per
// track, allocated on the track's first admitted frame and only under the
// option. The free test samples only cells within profileFreeReachMetres of
// the extent, at profileCellMetres along each cell's strip.

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// The profile's constants, fixed in geom0019's implement note before any
// replay and not tuned on its results.
const (
	profileCellMetres         = 0.1
	profileAlongHalfCells     = 150 // ±15 m
	profileAcrossHalfCells    = 50  // ±5 m
	profileOccupiedLogOdds    = 0.85
	profileFreeLogOdds        = -0.40
	profileMinLogOdds         = -2.0
	profileMaxLogOdds         = 3.5
	profileLockOnLogOdds      = 1.5
	profileEndFreeLogOdds     = -0.4
	profileEndMarginMetres    = 0.3
	profileBandLowMetres      = 0.2
	profileBandHighMetres     = 4.0
	profileColumnDeg          = 0.4
	profileShadowMarginMetres = 0.3
	profileWithholdEvery      = 5
	// profileStripMarginMetres is how far inside the other grid's extent the
	// free test samples (insideStrip).
	profileStripMarginMetres = 0.3
	// profileFreeReachMetres is how far beyond the extent (or the frame's
	// span before lock-on) the free test looks: the ends are what it bounds.
	profileFreeReachMetres = 2.0
)

// profileColumns is the number of azimuth columns in a full turn.
const profileColumns = int(360 / profileColumnDeg)

// ExtentProfileReading is a track's extent profile at its latest update
// under SolidBodyOptions.ExtentProfile. The zero value is no profile.
type ExtentProfileReading struct {
	// FrameUnixNanos is the frame this reading was made on; a reader holding
	// a later frame's track should read it as stale.
	FrameUnixNanos int64
	// Known says both grids have a locked cell, so the extents below hold.
	Known bool
	// AlongLo, AlongHi, AcrossLo and AcrossHi are the extents in the body
	// frame, metres from the reference at (X, Y) along PsiRad and across it
	// to the left.
	AlongLo, AlongHi, AcrossLo, AcrossHi float32
	// LengthMetres and WidthMetres are AlongHi−AlongLo and AcrossHi−AcrossLo.
	LengthMetres, WidthMetres float32
	// The ends, observed (bounded by free cells) or censored.
	RearObserved, FrontObserved, RightObserved, LeftObserved bool
	// X, Y and PsiRad are the pose the extents are placed at: the solid
	// body's position and the profile's axis on this frame.
	X, Y, PsiRad float32
	// Admitted says this frame passed admission; Withheld that it was
	// admitted and withheld from the update.
	Admitted, Withheld bool
	// AdmittedFrames, RefusedFrames and WithheldFrames count the track's
	// frames so far; HeadingRefusedFrames are the refused frames whose only
	// reason was the orientation's standard deviation.
	AdmittedFrames, RefusedFrames, WithheldFrames, HeadingRefusedFrames int
	// LockedUnixNanos is the frame the profile first became Known; zero
	// before.
	LockedUnixNanos int64
}

// extentProfile is one track's grids and counters.
type extentProfile struct {
	along  [2 * profileAlongHalfCells]float32
	across [2 * profileAcrossHalfCells]float32
	// axis is the direction positive along points, kept within a
	// quarter-turn of the orientation's axis from frame to frame.
	axis    float32
	axisSet bool

	admitted, refused, withheld, headingRefused int
	lockedNanos                                 int64
}

// profileFrame is the frame's free-space table: per azimuth column, the
// nearest in-band foreground return's range, +Inf for none.
type profileFrame struct {
	sensorX, sensorY float64
	nearest          [profileColumns]float32
}

// column is the azimuth column of a bearing in radians.
func profileColumn(bearing float64) int {
	deg := bearing * 180 / math.Pi
	c := int(math.Floor(deg / profileColumnDeg))
	c %= profileColumns
	if c < 0 {
		c += profileColumns
	}
	return c
}

// bandOf is a cluster's height band: from profileBandLowMetres to
// profileBandHighMetres above its lowest return.
func bandOf(points []l4perception.WorldPoint) (lo, hi float64, ok bool) {
	if len(points) == 0 {
		return 0, 0, false
	}
	z := math.Inf(1)
	for _, p := range points {
		z = math.Min(z, p.Z)
	}
	return z + profileBandLowMetres, z + profileBandHighMetres, true
}

// newProfileFrame builds the free-space table from every cluster's in-band
// returns.
func newProfileFrame(clusters []WorldCluster, sensorX, sensorY float32) *profileFrame {
	f := &profileFrame{sensorX: float64(sensorX), sensorY: float64(sensorY)}
	for i := range f.nearest {
		f.nearest[i] = float32(math.Inf(1))
	}
	for i := range clusters {
		points := nearEdgePoints(clusters[i])
		lo, hi, ok := bandOf(points)
		if !ok {
			continue
		}
		for _, p := range points {
			if p.Z < lo || p.Z > hi {
				continue
			}
			dx, dy := p.X-f.sensorX, p.Y-f.sensorY
			c := profileColumn(math.Atan2(dy, dx))
			if r := float32(math.Hypot(dx, dy)); r < f.nearest[c] {
				f.nearest[c] = r
			}
		}
	}
	return f
}

// bearings is the angular extent of a cluster's returns seen from the sensor:
// the bearings relative to centre, from lo to hi, in radians.
type bearings struct {
	centre, lo, hi float64
	ok             bool
}

func bearingsOf(points []l4perception.WorldPoint, sx, sy float64) bearings {
	if len(points) == 0 {
		return bearings{}
	}
	var mx, my float64
	for _, p := range points {
		mx += p.X
		my += p.Y
	}
	n := float64(len(points))
	b := bearings{centre: math.Atan2(my/n-sy, mx/n-sx), lo: math.Inf(1), hi: math.Inf(-1), ok: true}
	for _, p := range points {
		a := wrapAngle(math.Atan2(p.Y-sy, p.X-sx) - b.centre)
		b.lo, b.hi = math.Min(b.lo, a), math.Max(b.hi, a)
	}
	return b
}

// include widens the bearings to hold one more.
func (b *bearings) include(bearing float64) {
	if !b.ok {
		return
	}
	a := wrapAngle(bearing - b.centre)
	b.lo, b.hi = math.Min(b.lo, a), math.Max(b.hi, a)
}

// inside says a bearing lies within the cluster's, widened by half a column.
func (b bearings) inside(bearing float64) bool {
	if !b.ok {
		return false
	}
	half := profileColumnDeg / 2 * math.Pi / 180
	a := wrapAngle(bearing - b.centre)
	return a >= b.lo-half && a <= b.hi+half
}

// crossed says the ray model saw past the world point (x, y) without a
// return there (see the file comment).
func (f *profileFrame) crossed(x, y float64, own bearings) bool {
	dx, dy := x-f.sensorX, y-f.sensorY
	bearing := math.Atan2(dy, dx)
	nearest := f.nearest[profileColumn(bearing)]
	if math.IsInf(float64(nearest), 1) {
		return !own.inside(bearing)
	}
	return math.Hypot(dx, dy) <= float64(nearest)-profileShadowMarginMetres
}

// profileAxis is the believed axis a frame's profile update is taken along,
// geom0018's rule for a believed axis: the solid body's orientation when its
// standard deviation is at most axisBelievedSigmaRad (5 degrees) and, at
// speed, the course does not contradict it (spanAxisBelieved); otherwise the
// course, at CourseAlignmentMinSpeedMps or more, when its own standard
// deviation is within the same bound. ok is false when neither is believed,
// or when the body is not referenced to its centre or is still initialising;
// headingOnly then says the axis was the only reason.
func profileAxis(sb *solidBodyTrack) (psi float32, ok, headingOnly bool) {
	o := sb.orientation
	if sb.reference != ReferenceBodyCentre || sb.estimation == EstimationInitialising || o.Provenance == ProvenanceNone {
		return 0, false, false
	}
	bound := float32(axisBelievedSigmaRad * axisBelievedSigmaRad)
	if o.VarianceRad2 >= 0 && o.VarianceRad2 <= bound && spanAxisBelieved(sb, o.PsiRad, false) {
		return o.PsiRad, true, false
	}
	if course, ok := courseOrientation(sb.state, sb.p); ok && course.VarianceRad2 >= 0 && course.VarianceRad2 <= bound {
		return course.PsiRad, true, false
	}
	return 0, false, true
}

// updateExtentProfiles runs every associated track's profile for the frame,
// after the solid bodies have been updated: step 3c's tail.
func (t *Tracker) updateExtentProfiles(clusters []WorldCluster, associations []string, nowNanos int64) {
	var frame *profileFrame
	for clusterIdx, trackID := range associations {
		if trackID == "" {
			continue
		}
		track := t.Tracks[trackID]
		if track == nil || track.TrackState == TrackDeleted || !track.solidBody.seeded {
			continue
		}
		if frame == nil {
			frame = newProfileFrame(clusters, t.Config.SolidBody.SensorX, t.Config.SolidBody.SensorY)
		}
		sb := &track.solidBody
		if sb.profile == nil {
			sb.profile = &extentProfile{}
		}
		sb.reading.Profile = sb.profile.step(sb, clusters[clusterIdx], frame, nowNanos)
	}
}

// step admits or refuses one frame and returns the reading.
func (p *extentProfile) step(sb *solidBodyTrack, cluster WorldCluster, frame *profileFrame, nowNanos int64) ExtentProfileReading {
	believed, admit, headingOnly := profileAxis(sb)
	if !admit {
		p.refused++
		if headingOnly {
			p.headingRefused++
		}
		r := p.reading(nowNanos)
		r.X, r.Y, r.PsiRad = sb.state[0], sb.state[1], p.axisFor(sb.orientation.PsiRad)
		return r
	}
	psi := p.axisFor(believed)
	p.axis, p.axisSet = psi, true
	p.admitted++
	withheld := p.admitted%profileWithholdEvery == 0
	if withheld {
		p.withheld++
	} else {
		p.observe(cluster, frame, float64(sb.state[0]), float64(sb.state[1]), float64(psi))
	}
	r := p.reading(nowNanos)
	if r.Known && p.lockedNanos == 0 {
		p.lockedNanos = nowNanos
		r.LockedUnixNanos = nowNanos
	}
	r.X, r.Y, r.PsiRad = sb.state[0], sb.state[1], psi
	r.Admitted, r.Withheld = true, withheld
	return r
}

// axisFor is the orientation's axis with the profile's sense: psi or psi+pi,
// whichever is within a quarter-turn of the last admitted axis.
func (p *extentProfile) axisFor(psi float32) float32 {
	if !p.axisSet {
		return psi
	}
	if math.Cos(float64(psi-p.axis)) < 0 {
		return wrapToPi(psi + math.Pi)
	}
	return psi
}

// observe applies one frame's occupied and free evidence.
func (p *extentProfile) observe(cluster WorldCluster, frame *profileFrame, ox, oy, psi float64) {
	points := nearEdgePoints(cluster)
	lo, hi, ok := bandOf(points)
	if !ok {
		return
	}
	c, s := math.Cos(psi), math.Sin(psi)
	var occAlong [2 * profileAlongHalfCells]bool
	var occAcross [2 * profileAcrossHalfCells]bool
	uLo, uHi, vLo, vHi := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	inBand := 0
	for _, pt := range points {
		if pt.Z < lo || pt.Z > hi {
			continue
		}
		dx, dy := pt.X-ox, pt.Y-oy
		u, v := dx*c+dy*s, -dx*s+dy*c
		iu, okU := profileCell(u, profileAlongHalfCells)
		iv, okV := profileCell(v, profileAcrossHalfCells)
		if !okU || !okV {
			continue
		}
		occAlong[iu], occAcross[iv] = true, true
		uLo, uHi, vLo, vHi = math.Min(uLo, u), math.Max(uHi, u), math.Min(vLo, v), math.Max(vHi, v)
		inBand++
	}
	if inBand == 0 {
		return
	}
	// The strips the free test samples: the other grid's locked extent, or
	// the frame's in-band span before lock-on.
	if a, b, ok := lockedSpan(p.along[:], profileAlongHalfCells); ok {
		uLo, uHi = a, b
	}
	if a, b, ok := lockedSpan(p.across[:], profileAcrossHalfCells); ok {
		vLo, vHi = a, b
	}
	// The strips are sampled inside their ends: an extent a smear or a
	// drifting reference has widened would otherwise put the strip's edge off
	// the body, where an oblique ray passes, and free the body's own cells.
	uLo, uHi = insideStrip(uLo, uHi)
	vLo, vHi = insideStrip(vLo, vHi)
	own := bearingsOf(points, frame.sensorX, frame.sensorY)
	world := func(u, v float64) (float64, float64) { return ox + u*c - v*s, oy + u*s + v*c }
	if a0, a1, ok := lockedSpan(p.along[:], profileAlongHalfCells); ok {
		if c0, c1, ok := lockedSpan(p.across[:], profileAcrossHalfCells); ok {
			// The locked footprint's bearings are the body's too: a return
			// DBSCAN dropped as noise, at a grazing side, leaves its column
			// empty while the body is there.
			for _, corner := range [4][2]float64{{a0, c0}, {a0, c1}, {a1, c0}, {a1, c1}} {
				x, y := world(corner[0], corner[1])
				own.include(math.Atan2(y-frame.sensorY, x-frame.sensorX))
			}
		}
	}
	freeAlong := func(i int) bool {
		u := (float64(i-profileAlongHalfCells) + 0.5) * profileCellMetres
		for v := vLo; ; v += profileCellMetres {
			if v > vHi {
				v = vHi
			}
			if x, y := world(u, v); frame.crossed(x, y, own) {
				return true
			}
			if v >= vHi {
				return false
			}
		}
	}
	freeAcross := func(i int) bool {
		v := (float64(i-profileAcrossHalfCells) + 0.5) * profileCellMetres
		for u := uLo; ; u += profileCellMetres {
			if u > uHi {
				u = uHi
			}
			if x, y := world(u, v); frame.crossed(x, y, own) {
				return true
			}
			if u >= uHi {
				return false
			}
		}
	}
	applyProfileEvidence(p.along[:], occAlong[:], profileAlongHalfCells, uLo, uHi, freeAlong)
	applyProfileEvidence(p.across[:], occAcross[:], profileAcrossHalfCells, vLo, vHi, freeAcross)
}

// insideStrip is [lo, hi] less profileStripMarginMetres at each end, or its
// midpoint when it is no wider than twice that.
func insideStrip(lo, hi float64) (float64, float64) {
	if hi-lo <= 2*profileStripMarginMetres {
		m := (lo + hi) / 2
		return m, m
	}
	return lo + profileStripMarginMetres, hi - profileStripMarginMetres
}

// applyProfileEvidence updates one grid: occupied cells up, and the cells
// within profileFreeReachMetres of [lo, hi] that the ray model crossed down.
func applyProfileEvidence(grid []float32, occupied []bool, half int, lo, hi float64, free func(int) bool) {
	first, _ := profileCell(lo-profileFreeReachMetres, half)
	last, _ := profileCell(hi+profileFreeReachMetres, half)
	if lo-profileFreeReachMetres < -float64(half)*profileCellMetres {
		first = 0
	}
	if hi+profileFreeReachMetres >= float64(half)*profileCellMetres {
		last = len(grid) - 1
	}
	for i := range grid {
		switch {
		case occupied[i]:
			grid[i] = clampLogOdds(grid[i] + profileOccupiedLogOdds)
		case i >= first && i <= last && free(i):
			grid[i] = clampLogOdds(grid[i] + profileFreeLogOdds)
		}
	}
}

func clampLogOdds(v float32) float32 {
	return float32(math.Max(profileMinLogOdds, math.Min(profileMaxLogOdds, float64(v))))
}

// profileCell is the cell holding coordinate x on a grid of 2·half cells
// centred on zero, and false outside it.
func profileCell(x float64, half int) (int, bool) {
	i := int(math.Floor(x/profileCellMetres)) + half
	if i < 0 || i >= 2*half {
		return 0, false
	}
	return i, true
}

// lockedSpan is the span from the lowest to the highest locked cell, in
// metres from the origin, and false with none locked.
func lockedSpan(grid []float32, half int) (lo, hi float64, ok bool) {
	first, last := lockedCells(grid)
	if first < 0 {
		return 0, 0, false
	}
	return float64(first-half) * profileCellMetres, float64(last+1-half) * profileCellMetres, true
}

func lockedCells(grid []float32) (first, last int) {
	first, last = -1, -1
	for i, v := range grid {
		if v >= profileLockOnLogOdds {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	return first, last
}

// endsObserved says whether free cells bound the locked span's low and high
// ends within profileEndMarginMetres.
func endsObserved(grid []float32, first, last int) (low, high bool) {
	reach := int(math.Round(profileEndMarginMetres / profileCellMetres))
	for k := 1; k <= reach; k++ {
		if i := first - k; i >= 0 && grid[i] <= profileEndFreeLogOdds {
			low = true
		}
		if i := last + k; i < len(grid) && grid[i] <= profileEndFreeLogOdds {
			high = true
		}
	}
	return low, high
}

// reading summarises the grids.
func (p *extentProfile) reading(nowNanos int64) ExtentProfileReading {
	r := ExtentProfileReading{FrameUnixNanos: nowNanos, AdmittedFrames: p.admitted, RefusedFrames: p.refused,
		WithheldFrames: p.withheld, HeadingRefusedFrames: p.headingRefused, LockedUnixNanos: p.lockedNanos}
	a0, a1 := lockedCells(p.along[:])
	c0, c1 := lockedCells(p.across[:])
	if a0 < 0 || c0 < 0 {
		return r
	}
	r.Known = true
	r.AlongLo = float32(float64(a0-profileAlongHalfCells) * profileCellMetres)
	r.AlongHi = float32(float64(a1+1-profileAlongHalfCells) * profileCellMetres)
	r.AcrossLo = float32(float64(c0-profileAcrossHalfCells) * profileCellMetres)
	r.AcrossHi = float32(float64(c1+1-profileAcrossHalfCells) * profileCellMetres)
	r.LengthMetres, r.WidthMetres = r.AlongHi-r.AlongLo, r.AcrossHi-r.AcrossLo
	r.RearObserved, r.FrontObserved = endsObserved(p.along[:], a0, a1)
	r.RightObserved, r.LeftObserved = endsObserved(p.across[:], c0, c1)
	return r
}
