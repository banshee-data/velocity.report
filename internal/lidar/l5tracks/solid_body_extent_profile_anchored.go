package l5tracks

// ExtentProfileAnchored (SolidBodyOptions.ExtentProfileAnchored): geom0019's
// extent profile with two changes, for geom0020. Its constants, grids,
// evidence, lock-on, end marks and withheld frames are the profile's own
// (solid_body_extent_profile.go), and so is the code that applies them
// (extentProfile.advance). Like the profile it is a shadow: no track, no
// solid-body state and no published box changes.
//
// The grid is anchored to the body. geom0019's grid had its origin at the
// solid body's centre, and that centre moves relative to the body whenever
// the extent state moves it: a half-extent that grows with one face held
// moves the centre by the growth, and the profile, which cannot take its
// locked cells back, wrote the move as length (truck 2 on kirk0, 9.6 m then
// 12.5 m from 77.4 s). Here the origin is the centre plus an offset in the
// body frame, along and across the profile's axis, and each frame the offset
// takes back the part of the centre's change that the extent state made, so
// the origin follows only the body's tracked motion:
//
//   - A change of the half-extents, Δh = h − h_prev, moves the centre by the
//     regression of the position on the half-extents in the joint state,
//     P_ch · P_hh⁻¹ · Δh, read from the frame's posterior covariance. That is
//     exactly the move a span bound's estimate projection makes (it moves the
//     mean by that regression and leaves P alone), and it is the part of a
//     face update's move that goes with the half-extents. The remainder of
//     the centre's change, the prediction's v·dt and the innovation the
//     half-extents do not carry, is the body's motion and is followed.
//   - A change of reference (the body re-referenced between its medoid and
//     its centre) moves the centre by an amount no state accounts for; on
//     that frame the origin follows the solid body's velocity over the
//     interval, v·dt, and the rest of the jump goes to the offset.
//
// The offset starts at zero on the first admitted frame and is carried in
// the body frame, so it turns with the axis. Without ExtentState there are no
// half-extents in the state, and only a change of reference is taken back.
//
// The believed axis is the published box's heading filter (box_model.go), as
// SharedOrientation reads it: its psi and P_psi-psi, on a frame the box has
// updated and once it is labelled, so that psi is a heading and not one of
// four axes. A frame is admitted when that standard deviation is
// axisBelievedSigmaRad (5 degrees, geom0019's bound) or less, with the body
// referenced to its centre and past initialising as before; there is no
// course fallback. The box's heading sits 1.2 degrees from reviewed yaw at
// the median on kirk0 (geom0015), where the solid body's orientation spread
// 33 to 44 degrees. The profile runs after step 3d, where the box has this
// frame's posterior, so it reads the solid body as step 3c left it and the
// box as step 3d did.

import "math"

// profileAnchor is the anchored grid's origin: the offset from the solid
// body's centre, and the centre, half-extents and reference it was last
// computed from.
type profileAnchor struct {
	set bool
	// x, y, h, ext and ref are the solid body's centre, half-extents,
	// whether it carried them, and its reference at the last step; nanos
	// that step's frame.
	x, y  float32
	h     [2]float32
	ext   bool
	ref   ReferencePoint
	nanos int64
	// offA and offN are the origin less the centre, along and across the
	// profile's axis.
	offA, offN float64
}

// profileBoxAxis is the anchored profile's believed axis: the box's heading
// posterior on a frame the box updated, once labelled, with a standard
// deviation of at most axisBelievedSigmaRad. ok is false when the body is
// not referenced to its centre or is initialising, and otherwise when the
// box's heading is not believed; headingOnly then says the heading was the
// only reason.
func profileBoxAxis(track *TrackedObject, nowNanos int64) (psi float32, ok, headingOnly bool) {
	sb := &track.solidBody
	if sb.reference != ReferenceBodyCentre || sb.estimation == EstimationInitialising {
		return 0, false, false
	}
	b := track.box
	if b == nil || !b.seeded || !b.labelled || b.lastNanos != nowNanos {
		return 0, false, true
	}
	if v := b.p[0][0]; !(v >= 0 && v <= axisBelievedSigmaRad*axisBelievedSigmaRad) {
		return 0, false, true
	}
	return float32(b.psi), true, false
}

// stepAnchored admits or refuses one frame along the box's heading, at the
// origin held to the body, and returns the reading.
func (p *extentProfile) stepAnchored(track *TrackedObject, cluster WorldCluster, frame *profileFrame, nowNanos int64) ExtentProfileReading {
	sb := &track.solidBody
	believed, admit, headingOnly := profileBoxAxis(track, nowNanos)
	refusedPsi := sb.orientation.PsiRad
	if b := track.box; b != nil && b.seeded {
		refusedPsi = float32(b.psi)
	}
	// The axis the offset is carried along this frame: the one this frame
	// will be placed along when admitted, and the last admitted otherwise.
	psi := p.axis
	if admit {
		psi = p.axisFor(believed)
	}
	x, y := p.follow(sb, float64(psi), nowNanos)
	return p.advance(profileStep{believed: believed, admit: admit, headingOnly: headingOnly,
		refusedPsi: refusedPsi, x: x, y: y}, cluster, frame, nowNanos)
}

// follow updates the anchor from the solid body's change since the last
// step and returns the grid's origin in the world. Before the first admitted
// frame there is no grid, and the origin is the centre.
func (p *extentProfile) follow(sb *solidBodyTrack, psi float64, nowNanos int64) (float32, float32) {
	a := &p.anchor
	c, s := math.Cos(psi), math.Sin(psi)
	if a.set && p.axisSet {
		var sx, sy float64
		switch {
		case sb.reference != a.ref:
			dt := float64(nowNanos-a.nanos) / 1e9
			sx = float64(sb.state[0]-a.x) - float64(sb.state[2])*dt
			sy = float64(sb.state[1]-a.y) - float64(sb.state[3])*dt
		case sb.ext.on && a.ext:
			sx, sy = extentShift(sb.ext, float64(sb.ext.h[0]-a.h[0]), float64(sb.ext.h[1]-a.h[1]))
		}
		a.offA -= sx*c + sy*s
		a.offN -= -sx*s + sy*c
	}
	a.set, a.x, a.y, a.h, a.ext, a.ref, a.nanos = true, sb.state[0], sb.state[1], sb.ext.h, sb.ext.on, sb.reference, nowNanos
	if !p.axisSet {
		a.offA, a.offN = 0, 0
	}
	return float32(float64(sb.state[0]) + a.offA*c - a.offN*s), float32(float64(sb.state[1]) + a.offA*s + a.offN*c)
}

// extentShift is the move of the centre that goes with a change dh of the
// half-extents in the joint state: P_ch · P_hh⁻¹ · dh. With P_hh singular
// each half-extent is regressed alone where its variance is positive.
func extentShift(e solidBodyExtent, dhL, dhW float64) (float64, float64) {
	if dhL == 0 && dhW == 0 {
		return 0, 0
	}
	a, b, c, d := float64(e.phh[0]), float64(e.phh[1]), float64(e.phh[2]), float64(e.phh[3])
	var kL, kW float64
	if det := a*d - b*c; det > 1e-12 && !math.IsNaN(det) && !math.IsInf(det, 0) {
		kL, kW = (d*dhL-b*dhW)/det, (-c*dhL+a*dhW)/det
	} else {
		if a > 1e-9 {
			kL = dhL / a
		}
		if d > 1e-9 {
			kW = dhW / d
		}
	}
	sx := float64(e.pxh[0])*kL + float64(e.pxh[1])*kW
	sy := float64(e.pxh[2])*kL + float64(e.pxh[3])*kW
	if math.IsNaN(sx) || math.IsNaN(sy) || math.IsInf(sx, 0) || math.IsInf(sy, 0) {
		return 0, 0
	}
	return sx, sy
}
