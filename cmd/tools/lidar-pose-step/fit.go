package main

import (
	"fmt"
	"math"
)

// fitParams holds every threshold the comparison of two windows uses. They are
// fixed before any real window is read, from the synthetic scenes in the
// tests, and recorded in the output.
type fitParams struct {
	// Plane estimation on the before window's static cells: neighbours within
	// PlaneRingRadius rings and PlaneAzRadius bins, at least PlaneMinPoints of
	// them over at least two rings, fitted with an RMS within
	// PlaneTolBase + PlaneTolFraction*range.
	PlaneRingRadius  int     `json:"plane_ring_radius"`
	PlaneAzRadius    int     `json:"plane_az_radius_bins"`
	PlaneMinPoints   int     `json:"plane_min_points"`
	PlaneTolBase     float64 `json:"plane_tol_base_m"`
	PlaneTolFraction float64 `json:"plane_tol_fraction"`
	// MinCosIncidence drops cells whose ray meets its plane more obliquely
	// than this (|n.d| below it), where a range difference is mostly the
	// ray sliding along the surface.
	MinCosIncidence float64 `json:"min_cos_incidence"`

	// Robust iteration: HuberIters Huber steps, then Tukey biweight steps
	// up to MaxIter, with the scale from the residuals' MAD, never below
	// MinScale. InlierK*scale bounds the final trimmed inlier set.
	MaxIter    int     `json:"max_iter"`
	HuberIters int     `json:"huber_iters"`
	MinScale   float64 `json:"min_scale_m"`
	InlierK    float64 `json:"inlier_k"`

	// ChangedTau is the |delta range| above which a cell counts as changed in
	// the per-sector shares.
	ChangedTau float64 `json:"changed_tau_m"`
	// Sectors is the number of equal azimuth sectors support and residuals
	// are reported over; also the jackknife's blocks.
	Sectors int `json:"sectors"`
	// MinHeldOutCells is the fewest candidates a sector needs to be held out.
	MinHeldOutCells int `json:"min_held_out_cells"`

	// Abstention.
	MinInliers         int     `json:"min_inliers"`
	MinInlierShare     float64 `json:"min_inlier_share"`
	MaxSectorShare     float64 `json:"max_sector_share"`
	MinSectorsOver5Pct int     `json:"min_sectors_over_5pct"`
	MaxNormalDominance float64 `json:"max_normal_dominance"`
	MinConditionRatio  float64 `json:"min_condition_ratio"`
}

func defaultFitParams() fitParams {
	return fitParams{
		PlaneRingRadius:    1,
		PlaneAzRadius:      3,
		PlaneMinPoints:     8,
		PlaneTolBase:       0.02,
		PlaneTolFraction:   0.001,
		MinCosIncidence:    0.05,
		MaxIter:            40,
		HuberIters:         5,
		MinScale:           0.005,
		InlierK:            3,
		ChangedTau:         0.10,
		Sectors:            12,
		MinHeldOutCells:    50,
		MinInliers:         1000,
		MinInlierShare:     0.5,
		MaxSectorShare:     0.5,
		MinSectorsOver5Pct: 3,
		MaxNormalDominance: 0.9,
		MinConditionRatio:  1e-3,
	}
}

// ringBands groups the Pandar40P's rings for reporting: the six sparse upper
// beams, the dense 0.33-degree band near the horizon, and the ten lower beams
// that mostly see the road.
var ringBands = []struct {
	Name     string
	From, To int // rings, inclusive
}{
	{"upper (ch 1-6)", 0, 5},
	{"middle (ch 7-30)", 6, 29},
	{"lower (ch 31-40)", 30, 39},
}

func ringBand(ring int) int {
	for i, b := range ringBands {
		if ring >= b.From && ring <= b.To {
			return i
		}
	}
	return len(ringBands) - 1
}

// plane is a local surface fitted to a cell's static neighbours.
type plane struct {
	OK     bool
	Normal vec3 // unit, facing the sensor
	RMS    float64
}

// estimatePlanes fits a plane to each static cell's neighbourhood.
func estimatePlanes(g *geometry, cells []cellStat, p fitParams) []plane {
	out := make([]plane, numCells)
	pts := make([]vec3, 0, (2*p.PlaneRingRadius+1)*(2*p.PlaneAzRadius+1))
	for c := range cells {
		if !cells[c].Static {
			continue
		}
		ring, bin := c/numAzBins, c%numAzBins
		pts = pts[:0]
		ringsUsed := 0
		for dr := -p.PlaneRingRadius; dr <= p.PlaneRingRadius; dr++ {
			r := ring + dr
			if r < 0 || r >= numRings {
				continue
			}
			used := false
			for da := -p.PlaneAzRadius; da <= p.PlaneAzRadius; da++ {
				b := (bin + da + numAzBins) % numAzBins
				n := r*numAzBins + b
				if cells[n].Static {
					pts = append(pts, g.dirs[n].scale(cells[n].Median))
					used = true
				}
			}
			if used {
				ringsUsed++
			}
		}
		if len(pts) < p.PlaneMinPoints || ringsUsed < 2 {
			continue
		}
		var centroid vec3
		for _, q := range pts {
			centroid = centroid.add(q)
		}
		centroid = centroid.scale(1 / float64(len(pts)))
		cov := [][]float64{{0, 0, 0}, {0, 0, 0}, {0, 0, 0}}
		for _, q := range pts {
			d := q.sub(centroid)
			for i := 0; i < 3; i++ {
				for j := 0; j < 3; j++ {
					cov[i][j] += d[i] * d[j]
				}
			}
		}
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				cov[i][j] /= float64(len(pts))
			}
		}
		vals, vecs := symEigen(cov)
		rms := math.Sqrt(math.Max(vals[0], 0))
		own := g.dirs[c].scale(cells[c].Median)
		tol := p.PlaneTolBase + p.PlaneTolFraction*cells[c].Median
		n := vec3{vecs[0][0], vecs[0][1], vecs[0][2]}
		if rms > tol || math.Abs(n.dot(own.sub(centroid))) > 2*tol {
			continue
		}
		if n.dot(own) > 0 {
			n = n.scale(-1)
		}
		out[c] = plane{OK: true, Normal: n, RMS: rms}
	}
	return out
}

// candidate is one cell static in both windows with a usable before-window
// plane: the before point Q and normal N in the before frame, and the after
// point PB in the after frame, both on the cell's ray D.
type candidate struct {
	Cell   int
	Ring   int
	Sector int
	D      vec3
	Q      vec3
	N      vec3
	PB     vec3
	RA, RB float64
}

func buildCandidates(g *geometry, a, b []cellStat, planes []plane, p fitParams) []candidate {
	var out []candidate
	for c := range a {
		if !a[c].Static || !b[c].Static || !planes[c].OK {
			continue
		}
		d := g.dirs[c]
		n := planes[c].Normal
		if math.Abs(n.dot(d)) < p.MinCosIncidence {
			continue
		}
		bin := c % numAzBins
		out = append(out, candidate{
			Cell: c, Ring: c / numAzBins, Sector: bin * p.Sectors / numAzBins,
			D: d, Q: d.scale(a[c].Median), N: n, PB: d.scale(b[c].Median),
			RA: a[c].Median, RB: b[c].Median,
		})
	}
	return out
}

// rigid maps after-window sensor coordinates into before-window ones:
// x_before = R x_after + T. T is the after sensor origin in the before frame.
type rigid struct {
	R mat3
	T vec3
}

func identityRigid() rigid { return rigid{R: identity3()} }

func (r rigid) apply(x vec3) vec3 { return r.R.mul(x).add(r.T) }

// residual is the point-to-plane distance of the mapped after point from the
// before plane, metres along the normal.
func (r rigid) residual(c candidate) float64 {
	return c.N.dot(r.apply(c.PB).sub(c.Q))
}

// deltaRange is the after-window range minus the range the before plane
// predicts along the same ray once the transform is applied: the plane
// through Q with normal N, carried into the after frame and met by ray D.
// Under the identity it is RB - RA.
func (r rigid) deltaRange(c candidate) float64 {
	nb := r.R.mulT(c.N)
	den := nb.dot(c.D)
	if math.Abs(den) < 1e-6 {
		return math.NaN()
	}
	pred := (c.N.dot(c.Q) - c.N.dot(r.T)) / den
	return c.RB - pred
}

type rigidFit struct {
	Transform  rigid
	Iterations int
	Converged  bool
	Scale      float64
	Inlier     []bool
	NInliers   int
	Info       [6][6]float64 // sum of J J^T over inliers, unit weights
	Cov        [6][6]float64
	CovOK      bool
	Err        string
}

func jacobian(t rigid, c candidate) (float64, [6]float64) {
	pt := t.apply(c.PB)
	e := c.N.dot(pt.sub(c.Q))
	w := pt.cross(c.N)
	return e, [6]float64{w[0], w[1], w[2], c.N[0], c.N[1], c.N[2]}
}

func robustScale(res []float64, floor float64) float64 {
	if len(res) == 0 {
		return floor
	}
	tmp := append([]float64(nil), res...)
	med := median(tmp)
	for i, v := range res {
		tmp[i] = math.Abs(v - med)
	}
	return math.Max(1.4826*median(tmp), floor)
}

// fitRigid fits a small rigid motion by iteratively reweighted, linearised
// point-to-plane least squares: Huber steps from the identity, then Tukey
// steps, each a 6x6 solve composed onto the running transform.
func fitRigid(cands []candidate, p fitParams) rigidFit {
	out := rigidFit{Transform: identityRigid()}
	if len(cands) < 6 {
		out.Err = "fewer than six candidate cells"
		return out
	}
	res := make([]float64, len(cands))
	for it := 0; it < p.MaxIter; it++ {
		for i, c := range cands {
			res[i] = out.Transform.residual(c)
		}
		scale := robustScale(res, p.MinScale)
		var h [6][6]float64
		var g [6]float64
		for _, c := range cands {
			e, j := jacobian(out.Transform, c)
			w := robustWeight(e, scale, it < p.HuberIters)
			if w == 0 {
				continue
			}
			for a := 0; a < 6; a++ {
				g[a] += w * j[a] * e
				for b := 0; b < 6; b++ {
					h[a][b] += w * j[a] * j[b]
				}
			}
		}
		l, err := cholesky(h)
		if err != nil {
			out.Err = "normal equations are singular: the support does not constrain all six axes"
			out.Iterations = it
			return out
		}
		for a := range g {
			g[a] = -g[a]
		}
		dx := choleskySolve(l, g)
		dr := expSO3(vec3{dx[0], dx[1], dx[2]})
		out.Transform = rigid{R: dr.mulMat(out.Transform.R), T: dr.mul(out.Transform.T).add(vec3{dx[3], dx[4], dx[5]})}
		out.Iterations = it + 1
		step := math.Hypot(math.Hypot(dx[0], dx[1]), dx[2])
		move := math.Hypot(math.Hypot(dx[3], dx[4]), dx[5])
		if it >= p.HuberIters && step < 1e-8 && move < 1e-7 {
			out.Converged = true
			break
		}
	}
	for i, c := range cands {
		res[i] = out.Transform.residual(c)
	}
	out.Scale = robustScale(res, p.MinScale)
	out.Inlier = make([]bool, len(cands))
	var sse float64
	for i, c := range cands {
		if math.Abs(res[i]) > p.InlierK*out.Scale {
			continue
		}
		out.Inlier[i] = true
		out.NInliers++
		e, j := jacobian(out.Transform, c)
		sse += e * e
		for a := 0; a < 6; a++ {
			for b := 0; b < 6; b++ {
				out.Info[a][b] += j[a] * j[b]
			}
		}
	}
	if out.NInliers > 6 {
		if l, err := cholesky(out.Info); err == nil {
			inv := choleskyInverse(l)
			s2 := sse / float64(out.NInliers-6)
			for a := 0; a < 6; a++ {
				for b := 0; b < 6; b++ {
					out.Cov[a][b] = s2 * inv[a][b]
				}
			}
			out.CovOK = true
		}
	}
	return out
}

func robustWeight(e, scale float64, huber bool) float64 {
	a := math.Abs(e)
	if huber {
		k := 1.345 * scale
		if a <= k {
			return 1
		}
		return k / a
	}
	c := 4.685 * scale
	if a >= c {
		return 0
	}
	u := a / c
	return (1 - u*u) * (1 - u*u)
}

// residualStats summarises |delta range| over a set of cells.
type residualStats struct {
	Cells     int     `json:"cells"`
	RMS       float64 `json:"rms_m"`
	MedianAbs float64 `json:"median_abs_m"`
	// ChangedShare is the share of cells with |delta range| over ChangedTau.
	ChangedShare float64 `json:"changed_share"`
}

func summarise(deltas []float64, tau float64) residualStats {
	s := residualStats{}
	abs := make([]float64, 0, len(deltas))
	var ss float64
	changed := 0
	for _, d := range deltas {
		if math.IsNaN(d) {
			continue
		}
		abs = append(abs, math.Abs(d))
		ss += d * d
		if math.Abs(d) > tau {
			changed++
		}
	}
	s.Cells = len(abs)
	if s.Cells == 0 {
		s.RMS, s.MedianAbs = math.NaN(), math.NaN()
		return s
	}
	s.RMS = math.Sqrt(ss / float64(s.Cells))
	s.MedianAbs = median(abs)
	s.ChangedShare = float64(changed) / float64(s.Cells)
	return s
}

// transformReport is a fitted transform in reporting units.
type transformReport struct {
	// RotationDeg is the rotation vector about the sensor x, y and z axes
	// (z is the spin axis, up), degrees: for angles this small, the rotations
	// about each axis. RotationDeg[2] > 0 means the head turned anticlockwise
	// seen from above, so static scenery sits that much further clockwise in
	// the after window.
	RotationDeg     [3]float64 `json:"rotation_deg"`
	RotationSEDeg   [3]float64 `json:"rotation_se_deg"`
	RotationMagDeg  float64    `json:"rotation_magnitude_deg"`
	TiltDeg         float64    `json:"tilt_deg"`
	TranslationM    [3]float64 `json:"translation_m"`
	TranslationSEM  [3]float64 `json:"translation_se_m"`
	TranslationMagM float64    `json:"translation_magnitude_m"`
	// Jackknife standard errors over the leave-one-sector-out fits: they
	// carry the spatial correlation the formal errors ignore.
	JackknifeRotationSEDeg  [3]float64 `json:"jackknife_rotation_se_deg"`
	JackknifeTranslationSEM [3]float64 `json:"jackknife_translation_se_m"`
	JackknifeFits           int        `json:"jackknife_fits"`
}

func reportTransform(t rigid) transformReport {
	w := logSO3(t.R)
	r := transformReport{}
	for i := 0; i < 3; i++ {
		r.RotationDeg[i] = w[i] * 180 / math.Pi
		r.TranslationM[i] = t.T[i]
	}
	r.RotationMagDeg = w.norm() * 180 / math.Pi
	r.TiltDeg = math.Hypot(w[0], w[1]) * 180 / math.Pi
	r.TranslationMagM = t.T.norm()
	return r
}

type sectorSupport struct {
	Sector  int     `json:"sector"`
	FromDeg float64 `json:"from_deg"`
	ToDeg   float64 `json:"to_deg"`
	Inliers int     `json:"inliers"`
	Share   float64 `json:"share"`
}

type bandSupport struct {
	Band    string  `json:"band"`
	Inliers int     `json:"inliers"`
	Share   float64 `json:"share"`
}

type support struct {
	Sectors        []sectorSupport `json:"sectors"`
	RingBands      []bandSupport   `json:"ring_bands"`
	MaxSectorShare float64         `json:"max_sector_share"`
	SectorsOver5   int             `json:"sectors_over_5pct"`
	// NormalDominance is the largest eigenvalue of the inliers' mean n n^T:
	// 1 when every normal is parallel (one plane), 1/3 when isotropic.
	NormalDominance float64    `json:"normal_dominance"`
	DominantNormal  [3]float64 `json:"dominant_normal"`
	// ConditionRatio is the smallest over the largest eigenvalue of the
	// inlier information matrix, rotations scaled by ScaleLengthM so the two
	// halves are in metres; WeakestDirection is its eigenvector, as
	// (rx*L, ry*L, rz*L, tx, ty, tz).
	ConditionRatio   float64    `json:"condition_ratio"`
	ScaleLengthM     float64    `json:"scale_length_m"`
	WeakestDirection [6]float64 `json:"weakest_direction"`
}

type sectorResidual struct {
	Sector  int           `json:"sector"`
	FromDeg float64       `json:"from_deg"`
	ToDeg   float64       `json:"to_deg"`
	Before  residualStats `json:"before"`
	After   residualStats `json:"after"`
	// HeldOutAfter uses the fit made without this sector's cells, so a
	// sector the global transform explains only because it was fitted to it
	// shows here.
	HeldOutAfter *residualStats `json:"held_out_after,omitempty"`
}

type ringResidual struct {
	Ring         int           `json:"ring"`
	ElevationDeg float64       `json:"elevation_deg"`
	Before       residualStats `json:"before"`
	After        residualStats `json:"after"`
}

type bandSectorResidual struct {
	Band             string    `json:"band"`
	BeforeMedianAbsM []float64 `json:"before_median_abs_m"`
	AfterMedianAbsM  []float64 `json:"after_median_abs_m"`
}

type cellCounts struct {
	StaticA     int `json:"static_a"`
	StaticB     int `json:"static_b"`
	StaticBoth  int `json:"static_both"`
	StaticAOnly int `json:"static_a_only"`
	StaticBOnly int `json:"static_b_only"`
	PlanesA     int `json:"planes_a"`
	Candidates  int `json:"candidates"`
}

// pairResult is the comparison of one before window with one after window.
type pairResult struct {
	Name       string          `json:"name"`
	Group      string          `json:"group"`
	A          windowRef       `json:"a"`
	B          windowRef       `json:"b"`
	Counts     cellCounts      `json:"cells"`
	Transform  transformReport `json:"transform"`
	Iterations int             `json:"iterations"`
	Converged  bool            `json:"converged"`
	FitError   string          `json:"fit_error,omitempty"`
	ScaleM     float64         `json:"robust_scale_m"`
	Inliers    int             `json:"inliers"`
	InlierShr  float64         `json:"inlier_share"`
	// StaticBothBefore is |RB - RA| over every cell static in both windows,
	// planes or not: the raw change before any modelling.
	StaticBothBefore residualStats `json:"static_both_before"`
	// Before and After are |delta range| over the fit's candidates; the
	// Inlier variants over its final trimmed inliers.
	Before       residualStats `json:"before"`
	After        residualStats `json:"after"`
	InlierBefore residualStats `json:"inlier_before"`
	InlierAfter  residualStats `json:"inlier_after"`
	// HeldOut pools every held-out sector's cells, each under the fit that
	// excluded it.
	HeldOutBefore residualStats        `json:"held_out_before"`
	HeldOutAfter  residualStats        `json:"held_out_after"`
	Support       support              `json:"support"`
	Abstain       bool                 `json:"abstain"`
	Reasons       []string             `json:"abstain_reasons"`
	PerSector     []sectorResidual     `json:"per_sector"`
	PerRing       []ringResidual       `json:"per_ring"`
	BandSector    []bandSectorResidual `json:"band_by_sector"`
}

// windowRef names a window in a pair, by its anchor-relative seconds.
type windowRef struct {
	StartS     float64 `json:"start_s"`
	EndS       float64 `json:"end_s"`
	StartNanos int64   `json:"start_unix_nanos"`
	EndNanos   int64   `json:"end_unix_nanos"`
}

// comparePair is the whole comparison of two windows' cell summaries.
func comparePair(g *geometry, a, b []cellStat, p fitParams) pairResult {
	var res pairResult
	var staticBoth []float64
	for c := range a {
		if a[c].Static {
			res.Counts.StaticA++
		}
		if b[c].Static {
			res.Counts.StaticB++
		}
		switch {
		case a[c].Static && b[c].Static:
			res.Counts.StaticBoth++
			staticBoth = append(staticBoth, b[c].Median-a[c].Median)
		case a[c].Static:
			res.Counts.StaticAOnly++
		case b[c].Static:
			res.Counts.StaticBOnly++
		}
	}
	res.StaticBothBefore = summarise(staticBoth, p.ChangedTau)
	planes := estimatePlanes(g, a, p)
	for _, pl := range planes {
		if pl.OK {
			res.Counts.PlanesA++
		}
	}
	cands := buildCandidates(g, a, b, planes, p)
	res.Counts.Candidates = len(cands)

	fit := fitRigid(cands, p)
	res.Iterations, res.Converged, res.FitError = fit.Iterations, fit.Converged, fit.Err
	res.ScaleM, res.Inliers = fit.Scale, fit.NInliers
	if len(cands) > 0 {
		res.InlierShr = float64(fit.NInliers) / float64(len(cands))
	}
	res.Transform = reportTransform(fit.Transform)
	if fit.CovOK {
		for i := 0; i < 3; i++ {
			res.Transform.RotationSEDeg[i] = math.Sqrt(fit.Cov[i][i]) * 180 / math.Pi
			res.Transform.TranslationSEM[i] = math.Sqrt(fit.Cov[3+i][3+i])
		}
	}

	before := make([]float64, len(cands))
	after := make([]float64, len(cands))
	var inB, inA []float64
	for i, c := range cands {
		before[i] = c.RB - c.RA
		after[i] = fit.Transform.deltaRange(c)
		if fit.Inlier != nil && fit.Inlier[i] {
			inB = append(inB, before[i])
			inA = append(inA, after[i])
		}
	}
	res.Before = summarise(before, p.ChangedTau)
	res.After = summarise(after, p.ChangedTau)
	res.InlierBefore = summarise(inB, p.ChangedTau)
	res.InlierAfter = summarise(inA, p.ChangedTau)

	res.Support = measureSupport(cands, fit, p)
	res.Abstain, res.Reasons = abstention(res, fit, p)

	// Per sector, with each sector also held out of its own fit.
	sectorDeg := 360 / float64(p.Sectors)
	var jack [][6]float64
	var heldBefore, heldAfter []float64
	for s := 0; s < p.Sectors; s++ {
		sr := sectorResidual{Sector: s, FromDeg: float64(s) * sectorDeg, ToDeg: float64(s+1) * sectorDeg}
		var sb, sa []float64
		var keep []candidate
		var inSector []int
		for i, c := range cands {
			if c.Sector == s {
				sb = append(sb, before[i])
				sa = append(sa, after[i])
				inSector = append(inSector, i)
			} else {
				keep = append(keep, c)
			}
		}
		sr.Before = summarise(sb, p.ChangedTau)
		sr.After = summarise(sa, p.ChangedTau)
		if len(inSector) >= p.MinHeldOutCells && fit.Err == "" {
			hf := fitRigid(keep, p)
			if hf.Err == "" {
				var ha []float64
				for _, i := range inSector {
					d := hf.Transform.deltaRange(cands[i])
					ha = append(ha, d)
					heldBefore = append(heldBefore, before[i])
					heldAfter = append(heldAfter, d)
				}
				st := summarise(ha, p.ChangedTau)
				sr.HeldOutAfter = &st
				tr := reportTransform(hf.Transform)
				jack = append(jack, [6]float64{tr.RotationDeg[0], tr.RotationDeg[1], tr.RotationDeg[2],
					tr.TranslationM[0], tr.TranslationM[1], tr.TranslationM[2]})
			}
		}
		res.PerSector = append(res.PerSector, sr)
	}
	res.HeldOutBefore = summarise(heldBefore, p.ChangedTau)
	res.HeldOutAfter = summarise(heldAfter, p.ChangedTau)
	if m := len(jack); m >= 3 {
		var mean [6]float64
		for _, j := range jack {
			for k := range mean {
				mean[k] += j[k] / float64(m)
			}
		}
		var ss [6]float64
		for _, j := range jack {
			for k := range ss {
				ss[k] += (j[k] - mean[k]) * (j[k] - mean[k])
			}
		}
		for k := 0; k < 3; k++ {
			res.Transform.JackknifeRotationSEDeg[k] = math.Sqrt(float64(m-1) / float64(m) * ss[k])
			res.Transform.JackknifeTranslationSEM[k] = math.Sqrt(float64(m-1) / float64(m) * ss[3+k])
		}
		res.Transform.JackknifeFits = m
	}

	byRingB := make([][]float64, numRings)
	byRingA := make([][]float64, numRings)
	bandB := make([][][]float64, len(ringBands))
	bandA := make([][][]float64, len(ringBands))
	for i := range ringBands {
		bandB[i] = make([][]float64, p.Sectors)
		bandA[i] = make([][]float64, p.Sectors)
	}
	for i, c := range cands {
		byRingB[c.Ring] = append(byRingB[c.Ring], before[i])
		byRingA[c.Ring] = append(byRingA[c.Ring], after[i])
		bnd := ringBand(c.Ring)
		bandB[bnd][c.Sector] = append(bandB[bnd][c.Sector], before[i])
		bandA[bnd][c.Sector] = append(bandA[bnd][c.Sector], after[i])
	}
	for r := 0; r < numRings; r++ {
		res.PerRing = append(res.PerRing, ringResidual{Ring: r, ElevationDeg: g.elevationDeg[r],
			Before: summarise(byRingB[r], p.ChangedTau), After: summarise(byRingA[r], p.ChangedTau)})
	}
	for i, b := range ringBands {
		bs := bandSectorResidual{Band: b.Name}
		for s := 0; s < p.Sectors; s++ {
			bs.BeforeMedianAbsM = append(bs.BeforeMedianAbsM, summarise(bandB[i][s], p.ChangedTau).MedianAbs)
			bs.AfterMedianAbsM = append(bs.AfterMedianAbsM, summarise(bandA[i][s], p.ChangedTau).MedianAbs)
		}
		res.BandSector = append(res.BandSector, bs)
	}
	return res
}

func measureSupport(cands []candidate, fit rigidFit, p fitParams) support {
	s := support{}
	sectorDeg := 360 / float64(p.Sectors)
	counts := make([]int, p.Sectors)
	bands := make([]int, len(ringBands))
	nn := [][]float64{{0, 0, 0}, {0, 0, 0}, {0, 0, 0}}
	var ranges []float64
	n := 0
	for i, c := range cands {
		if fit.Inlier == nil || !fit.Inlier[i] {
			continue
		}
		n++
		counts[c.Sector]++
		bands[ringBand(c.Ring)]++
		for a := 0; a < 3; a++ {
			for b := 0; b < 3; b++ {
				nn[a][b] += c.N[a] * c.N[b]
			}
		}
		ranges = append(ranges, c.RA)
	}
	for k := 0; k < p.Sectors; k++ {
		ss := sectorSupport{Sector: k, FromDeg: float64(k) * sectorDeg, ToDeg: float64(k+1) * sectorDeg, Inliers: counts[k]}
		if n > 0 {
			ss.Share = float64(counts[k]) / float64(n)
		}
		if ss.Share > s.MaxSectorShare {
			s.MaxSectorShare = ss.Share
		}
		if ss.Share >= 0.05 {
			s.SectorsOver5++
		}
		s.Sectors = append(s.Sectors, ss)
	}
	for i, b := range ringBands {
		bs := bandSupport{Band: b.Name, Inliers: bands[i]}
		if n > 0 {
			bs.Share = float64(bands[i]) / float64(n)
		}
		s.RingBands = append(s.RingBands, bs)
	}
	if n == 0 {
		return s
	}
	for a := 0; a < 3; a++ {
		for b := 0; b < 3; b++ {
			nn[a][b] /= float64(n)
		}
	}
	vals, vecs := symEigen(nn)
	s.NormalDominance = vals[2]
	s.DominantNormal = [3]float64{vecs[2][0], vecs[2][1], vecs[2][2]}

	s.ScaleLengthM = median(ranges)
	l := s.ScaleLengthM
	if l <= 0 {
		l = 1
	}
	scaled := make([][]float64, 6)
	for a := 0; a < 6; a++ {
		scaled[a] = make([]float64, 6)
		for b := 0; b < 6; b++ {
			da, db := 1.0, 1.0
			if a < 3 {
				da = 1 / l
			}
			if b < 3 {
				db = 1 / l
			}
			scaled[a][b] = fit.Info[a][b] * da * db
		}
	}
	ev, evec := symEigen(scaled)
	if ev[5] > 0 {
		s.ConditionRatio = ev[0] / ev[5]
	}
	copy(s.WeakestDirection[:], evec[0])
	return s
}

func abstention(r pairResult, fit rigidFit, p fitParams) (bool, []string) {
	reasons := []string{}
	if fit.Err != "" {
		reasons = append(reasons, fit.Err)
	}
	if !fit.Converged && fit.Err == "" {
		reasons = append(reasons, fmt.Sprintf("fit did not converge in %d iterations", p.MaxIter))
	}
	if r.Inliers < p.MinInliers {
		reasons = append(reasons, fmt.Sprintf("%d inliers, under %d", r.Inliers, p.MinInliers))
	}
	if r.InlierShr < p.MinInlierShare {
		reasons = append(reasons, fmt.Sprintf("inlier share %.2f, under %.2f", r.InlierShr, p.MinInlierShare))
	}
	if r.Support.MaxSectorShare > p.MaxSectorShare {
		reasons = append(reasons, fmt.Sprintf("one sector holds %.0f%% of the support", 100*r.Support.MaxSectorShare))
	}
	if r.Support.SectorsOver5 < p.MinSectorsOver5Pct {
		reasons = append(reasons, fmt.Sprintf("only %d sectors hold 5%% of the support", r.Support.SectorsOver5))
	}
	if r.Support.NormalDominance > p.MaxNormalDominance {
		reasons = append(reasons, fmt.Sprintf("one plane dominates the support (normal dominance %.2f)", r.Support.NormalDominance))
	}
	if r.Support.ConditionRatio < p.MinConditionRatio {
		reasons = append(reasons, fmt.Sprintf("an axis is poorly observed (condition ratio %.1e)", r.Support.ConditionRatio))
	}
	return len(reasons) > 0, reasons
}
