package l4perception

import "math"

// RangeAdaptiveEps widens DBSCAN's neighbourhood with range, so that a
// distant object, sampled more sparsely because neighbouring beams diverge,
// is not split by a radius chosen for near returns. It is the replay
// experiment range_adaptive_eps (experiment l4clus0001, decision D-28) and is
// off by default: DBSCANParams.RangeAdaptive is nil unless a replay names it.
//
// The law is Fontalvo et al., "Spatial geometry analysis of roadside LiDAR
// for improved vehicle clustering accuracy", Sensors 26(13):4068 (2026),
// equations (10), (13) and (14) and Algorithm 1. The paper writes a squared
// threshold t = λ·[(Δω·r)² + Δr²], from the spacing between returns of
// adjacent beams at range r, and clusters on d²/τ ≤ 1, where a pair's τ is the
// smaller of its two points' thresholds (Algorithm 1, step 19).
//
// Here, for a point at horizontal range r from the sensor,
//
//	eps(r)² = eps0² + λ·(Δθ·min(r, MaxRange))²
//
// and two points are neighbours when their horizontal distance squared is at
// most the smaller of their two eps², which is eps² at the nearer point's
// range. That pair rule is the paper's, and it keeps the neighbour relation
// symmetric, which DBSCAN needs for its clusters to be well defined.
//
// Three departures from the paper, each for a stated reason:
//
//   - The floor. The paper's constant term λ·Δr² (Δr = 0.4 m, so 0.55 m at
//     λ = 1.9) is its own calibrated near-range tolerance for a VLP-32C. This
//     tree's calibrated near-range tolerance is the tuned eps0, so eps0²
//     takes its place and the sampling term adds to it in quadrature, as the
//     paper's spacing components do in its equation (5). eps(0) is eps0 and
//     eps(r) is never below it. The paper's form with its own floor, capped
//     below at eps0, would not move: with the Pandar40P's beam spacing it
//     stays under the shipped 0.8 m out to about 71 m, beyond every range the
//     W3 gate scores.
//   - One angular step, not k-means groups of beams. The paper assigns each
//     point the spacing of its own beam's group. On the Pandar40P at its
//     ~2.3 m mount, nearly every return on a road user beyond about 20 m
//     comes from the fine band, channels 6 to 30 (+2.356° to −5.733°),
//     spaced 0.337°. The coarser beams below it reach the road before 20 m;
//     above it, only channel 5 (+3.368°, 1.01° from channel 6) still meets
//     the top of a vehicle taller than about 3.5 m, out to about 30 m.
//     Δθ is the fine band's spacing, or the azimuth step if it is larger,
//     as the paper's equation (5) has both. The clustering frame carries no
//     beam index, and recovering one from height would assume the identity
//     pose in a second place.
//   - Continuous range, not radial bins. The bins exist to build the paper's
//     lookup matrix; evaluating the law at each point's range is exact, and
//     makes the paper's per-beam clamp of r to where the beam leaves the
//     region of interest (its equations (8) and (9)) hold of itself for any
//     real return. MaxRange stands for the paper's r_ROI, so a stray return
//     at an absurd range cannot widen the search without bound.
//
// The clustering metric is horizontal (x, y), as DBSCAN's is everywhere in
// this package, whereas the paper's is three-dimensional. On a vertical face
// the gap between rings is vertical and the horizontal metric never sees it;
// the horizontal gap from sampling is the azimuth step, which is smaller.
// The vertical spacing is therefore the generous choice of the two.
type RangeAdaptiveEps struct {
	// Lambda scales the squared sampling spacing (the paper's λ). The paper
	// found error stable for λ in [1.7, 2.4] and reports DBSCAN at 1.7 and
	// 1.9 with equal accuracy.
	Lambda float64
	// AngularStepRad is Δθ, the angular sampling step in radians.
	AngularStepRad float64
	// MaxRange bounds the range at which the law is evaluated, in metres.
	MaxRange float64
	// OriginX and OriginY place the sensor in the clustering frame. The
	// pipeline clusters in the sensor frame (identity pose), so both are 0.
	OriginX, OriginY float64
}

const (
	// rangeAdaptiveLambda is the paper's λ = 1.9: the first of its two
	// DBSCAN settings in Table 2 (1.7 and 1.9 gave the same accuracy) and
	// inside the stable interval [1.7, 2.4] it reports.
	rangeAdaptiveLambda = 1.9
	// pandar40PFineBandSpacingDeg is the mean vertical spacing of the
	// Pandar40P's fine band, channels 6 to 30 of the embedded angle
	// correction table: (2.356 − (−5.733)) / 24 = 0.337°.
	pandar40PFineBandSpacingDeg = 0.337
	// pandar40PAzimuthStepDeg is the Pandar40P's azimuth step at the
	// deployed 10 Hz rotation. At 20 Hz it is 0.4°, which would then be Δθ.
	pandar40PAzimuthStepDeg = 0.2
	// pandar40PMaxRangeMetres is the Pandar40P's rated range, standing for
	// the paper's region-of-interest radius r_ROI.
	pandar40PMaxRangeMetres = 200.0
)

// DefaultRangeAdaptiveEps returns the law at the constants documented on
// RangeAdaptiveEps, for the Pandar40P in the sensor frame.
func DefaultRangeAdaptiveEps() RangeAdaptiveEps {
	stepDeg := math.Max(pandar40PFineBandSpacingDeg, pandar40PAzimuthStepDeg)
	return RangeAdaptiveEps{
		Lambda:         rangeAdaptiveLambda,
		AngularStepRad: stepDeg * math.Pi / 180,
		MaxRange:       pandar40PMaxRangeMetres,
	}
}

// SquaredEpsAt returns eps(r)² for a point at horizontal range r, given the
// configured eps0. It is never below eps0².
func (p RangeAdaptiveEps) SquaredEpsAt(eps0, r float64) float64 {
	if p.MaxRange > 0 && r > p.MaxRange {
		r = p.MaxRange
	}
	spacing := p.AngularStepRad * r
	extra := p.Lambda * spacing * spacing
	if !(extra > 0) { // non-positive λ or step, or NaN: the floor alone
		extra = 0
	}
	return eps0*eps0 + extra
}

// squaredTolerances returns each point's eps² at its range from the origin.
func (p RangeAdaptiveEps) squaredTolerances(points []WorldPoint, eps0 float64) []float64 {
	tol2 := make([]float64, len(points))
	for i, pt := range points {
		tol2[i] = p.SquaredEpsAt(eps0, math.Hypot(pt.X-p.OriginX, pt.Y-p.OriginY))
	}
	return tol2
}

// neighbourhood answers DBSCAN's region queries, either at one eps or, when
// tol2 is set, at each pair's range-adaptive tolerance. With tol2 nil it
// makes exactly the calls DBSCAN made before the option existed, so the
// default clusters are unchanged.
type neighbourhood struct {
	si     *SpatialIndex
	points []WorldPoint
	eps    float64
	tol2   []float64
}

// query returns the indices of points[idx]'s neighbours, written into dst.
func (n *neighbourhood) query(idx int, dst []int) []int {
	if n.tol2 == nil {
		return n.si.regionQueryInto(n.points, idx, n.eps, dst)
	}
	return n.si.regionQueryAdaptiveInto(n.points, idx, n.tol2, dst)
}

// regionQueryAdaptiveInto returns every point whose horizontal distance to
// points[idx] squared is at most min(tol2[idx], tol2[k]).
//
// The index's cells are eps0 wide, and a tolerance here is wider, so the 3×3
// search of regionQueryInto would miss neighbours. Any point within
// sqrt(tol2[idx]) of the query lies within ceil(sqrt(tol2[idx]) / CellSize)
// cells of it on each axis, so searching that many rings of cells finds every
// candidate the pair rule can accept: a pair's tolerance never exceeds the
// query point's own.
func (si *SpatialIndex) regionQueryAdaptiveInto(points []WorldPoint, idx int, tol2 []float64, dst []int) []int {
	p := points[idx]
	own := tol2[idx]
	neighbours := dst[:0]
	reach := int64(math.Ceil(math.Sqrt(own) / si.CellSize))
	cellX := int64(math.Floor(p.X / si.CellSize))
	cellY := int64(math.Floor(p.Y / si.CellSize))
	for dx := -reach; dx <= reach; dx++ {
		for dy := -reach; dy <= reach; dy++ {
			for _, candidateIdx := range si.Grid[szudzikCellID(cellX+dx, cellY+dy)] {
				candidate := points[candidateIdx]
				ex := candidate.X - p.X
				ey := candidate.Y - p.Y
				pair := own
				if t := tol2[candidateIdx]; t < pair {
					pair = t
				}
				if ex*ex+ey*ey <= pair {
					neighbours = append(neighbours, candidateIdx)
				}
			}
		}
	}
	return neighbours
}

// szudzikCellID maps signed cell coordinates to the index's cell key: a
// zigzag to non-negative integers, then Szudzik's pairing function.
func szudzikCellID(cellX, cellY int64) int64 {
	var a, b int64
	if cellX >= 0 {
		a = 2 * cellX
	} else {
		a = -2*cellX - 1
	}
	if cellY >= 0 {
		b = 2 * cellY
	} else {
		b = -2*cellY - 1
	}
	if a >= b {
		return a*a + a + b
	}
	return a + b*b
}
