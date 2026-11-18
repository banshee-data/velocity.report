package annotation

import "math"

// jointFeasiblePoint finds a witness in the intersection of bounded point
// disks, normal strips and the declared prior position disk. Checking only
// pairwise residuals against an aggregate radius can accept an empty joint
// intersection. With at most four anchors, all boundary intersections are
// cheap to enumerate. These are hard bounds, not independent Gaussian errors.
func jointFeasiblePoint(prior FacetPosePrior, preferred PlanarBound, disks []PlanarBound, strips []jointNormal, tangent *[2]float64) (float64, float64, bool) {
	originX, originY := *prior.XM, *prior.YM
	const tolerance = 64 * 2.220446049250313e-16 // floating-point roundoff, not a match threshold
	strips = append([]jointNormal(nil), strips...)
	disks = append(append([]PlanarBound(nil), disks...), PlanarBound{XM: originX, YM: originY, BoundM: *prior.PositionBoundM})
	scale := math.Max(1, *prior.PositionBoundM)
	for _, d := range disks {
		if !finiteFeature(d.XM) || !finiteFeature(d.YM) || !finiteFeature(d.BoundM) || d.BoundM < 0 {
			return 0, 0, false
		}
		scale = math.Max(scale, math.Max(d.BoundM, math.Hypot(d.XM-originX, d.YM-originY)))
	}
	for _, s := range strips {
		if !finiteFeature(s.nx) || !finiteFeature(s.ny) || !finiteFeature(s.r) || !finiteFeature(s.bound) || s.bound < 0 || math.Abs(math.Hypot(s.nx, s.ny)-1) > 1e-6 {
			return 0, 0, false
		}
		scale = math.Max(scale, math.Max(s.bound, math.Abs(s.r-s.nx*originX-s.ny*originY)))
	}
	if !finiteFeature(scale) {
		return 0, 0, false
	}
	// Reject dynamic ranges where numerical slack would swallow a real
	// uncertainty bound. A broad prior cannot turn contradictory centimetre
	// constraints into a supported joint fit.
	resolution := math.Max(scale*tolerance, math.Max(jointULP(originX), jointULP(originY)))
	for _, d := range disks {
		if d.BoundM > 0 && d.BoundM < 64*resolution {
			return 0, 0, false
		}
	}
	for _, s := range strips {
		if s.bound > 0 && s.bound < 64*resolution {
			return 0, 0, false
		}
	}
	originalDisks := append([]PlanarBound(nil), disks...)
	originalStrips := append([]jointNormal(nil), strips...)
	originalInside := func(x, y float64) bool {
		if !finiteFeature(x) || !finiteFeature(y) {
			return false
		}
		for _, d := range originalDisks {
			slack := 8 * (jointULP(x) + jointULP(y) + jointULP(d.XM) + jointULP(d.YM) + jointULP(d.BoundM))
			distance := math.Hypot(x-d.XM, y-d.YM)
			if !finiteFeature(distance) || distance > d.BoundM+slack {
				return false
			}
		}
		for _, s := range originalStrips {
			slack := 8 * (jointULP(s.nx*x) + jointULP(s.ny*y) + jointULP(s.r) + jointULP(s.bound))
			distance := math.Abs(s.nx*x + s.ny*y - s.r)
			if !finiteFeature(distance) || distance > s.bound+slack {
				return false
			}
		}
		if tangent != nil {
			slack := 8 * (jointULP(x) + jointULP(y) + jointULP(originX) + jointULP(originY))
			if math.Abs(tangent[0]*(x-originX)+tangent[1]*(y-originY)) > slack {
				return false
			}
		}
		return true
	}
	// A hard zero prior admits only its exact declared centre.
	if *prior.PositionBoundM == 0 {
		return originX, originY, originalInside(originX, originY)
	}
	for i := range disks {
		disks[i] = PlanarBound{XM: (disks[i].XM - originX) / scale, YM: (disks[i].YM - originY) / scale, BoundM: disks[i].BoundM / scale}
	}
	lines := make([]jointNormal, 0, 2*len(strips)+1)
	for i := range strips {
		strips[i].r = (strips[i].r - strips[i].nx*originX - strips[i].ny*originY) / scale
		strips[i].bound /= scale
		lines = append(lines, jointNormal{nx: strips[i].nx, ny: strips[i].ny, r: strips[i].r - strips[i].bound}, jointNormal{nx: strips[i].nx, ny: strips[i].ny, r: strips[i].r + strips[i].bound})
	}
	if tangent != nil {
		lines = append(lines, jointNormal{nx: tangent[0], ny: tangent[1], r: 0})
	}
	inside := func(x, y float64) bool {
		if !finiteFeature(x) || !finiteFeature(y) {
			return false
		}
		for _, d := range disks {
			if math.Hypot(x-d.XM, y-d.YM) > d.BoundM+tolerance {
				return false
			}
		}
		for _, s := range strips {
			if math.Abs(s.nx*x+s.ny*y-s.r) > s.bound+tolerance {
				return false
			}
		}
		return tangent == nil || math.Abs(tangent[0]*x+tangent[1]*y) <= tolerance
	}
	px, py := (preferred.XM-originX)/scale, (preferred.YM-originY)/scale
	if inside(px, py) && originalInside(preferred.XM, preferred.YM) {
		return preferred.XM, preferred.YM, true
	}
	bx, by, best := 0.0, 0.0, math.Inf(1)
	consider := func(x, y float64) {
		if inside(x, y) && originalInside(originX+x*scale, originY+y*scale) {
			if d := math.Hypot(x-px, y-py); d < best {
				bx, by, best = x, y, d
			}
		}
	}
	for _, d := range disks {
		consider(d.XM, d.YM)
	}
	for i, a := range disks {
		for _, b := range disks[i+1:] {
			dx, dy := b.XM-a.XM, b.YM-a.YM
			distance := math.Hypot(dx, dy)
			if distance <= tolerance || distance > a.BoundM+b.BoundM+tolerance || distance < math.Abs(a.BoundM-b.BoundM)-tolerance {
				continue
			}
			along := (a.BoundM*a.BoundM - b.BoundM*b.BoundM + distance*distance) / (2 * distance)
			h2 := a.BoundM*a.BoundM - along*along
			height := math.Sqrt(math.Max(0, h2))
			mx, my := a.XM+along*dx/distance, a.YM+along*dy/distance
			consider(mx-height*dy/distance, my+height*dx/distance)
			consider(mx+height*dy/distance, my-height*dx/distance)
		}
		for _, l := range lines {
			distance := l.nx*a.XM + l.ny*a.YM - l.r
			if math.Abs(distance) > a.BoundM+tolerance {
				continue
			}
			h2 := a.BoundM*a.BoundM - distance*distance
			height := math.Sqrt(math.Max(0, h2))
			mx, my := a.XM-l.nx*distance, a.YM-l.ny*distance
			consider(mx-l.ny*height, my+l.nx*height)
			consider(mx+l.ny*height, my-l.nx*height)
		}
	}
	for i, a := range lines {
		for _, b := range lines[i+1:] {
			det := a.nx*b.ny - a.ny*b.nx
			if math.Abs(det) <= tolerance {
				continue
			}
			consider((a.r*b.ny-a.ny*b.r)/det, (a.nx*b.r-a.r*b.nx)/det)
		}
	}
	if math.IsInf(best, 1) {
		return 0, 0, false
	}
	x, y := originX+bx*scale, originY+by*scale
	return x, y, originalInside(x, y)
}

// Scale-aware ulp for final checks in the original coordinate system.
func jointULP(x float64) float64 { x = math.Abs(x); return math.Nextafter(x, math.Inf(1)) - x }
