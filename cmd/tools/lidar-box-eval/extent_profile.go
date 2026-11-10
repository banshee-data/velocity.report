package main

import (
	"math"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// The solid body and its extent profile (geom0019) on each row: the solid
// body's pose and extents, the profile's (l5tracks.ExtentProfileReading)
// with its end marks and counters, and on a withheld frame the P4 score,
// against the box (geom0019) and against the solid body's own extent at its
// own pose (geom0020's restated P4), both grown by profileGrowthMetres. The
// anchored profile (solid_body_extent_profile_anchored) fills the same
// columns.
// Without -experiment solid_body the solid-body columns are blank, and
// without solid_body_extent_profile the profile's are.

// profileGrowthMetres is how far the profile's extent and the box are both
// grown before counting a withheld frame's returns inside them (geom0019 P4).
const profileGrowthMetres = 0.25

var solidBodyRowHeader = []string{"sb_x", "sb_y", "sb_psi_deg", "sb_length", "sb_width",
	"prof_known", "prof_length", "prof_width", "prof_along_lo", "prof_along_hi", "prof_across_lo", "prof_across_hi",
	"prof_x", "prof_y", "prof_psi_deg", "prof_ends", "prof_admitted", "prof_withheld",
	"prof_admitted_frames", "prof_refused_frames", "prof_heading_refused_frames", "prof_locked_ns",
	"wh_prof_share", "wh_box_share", "wh_area_ratio", "wh_points", "wh_sb_share", "wh_sb_area_ratio"}

// solidBodyRow is the row's solid-body and profile columns for a track on
// the frame at now, with the box it is scored against.
func solidBodyRow(t *l5tracks.TrackedObject, b box, cluster *l5tracks.WorldCluster, now int64) []string {
	out := make([]string, len(solidBodyRowHeader))
	r, ok := t.SolidBody()
	if !ok {
		return out
	}
	e := r.Estimate
	copy(out, []string{f3(float64(e.X)), f3(float64(e.Y)), f3(float64(e.Orientation.PsiRad) * 180 / math.Pi),
		f3(float64(e.Length.Metres)), f3(float64(e.Width.Metres))})
	p := r.Profile
	if p.FrameUnixNanos != now {
		// No profile, or one from an earlier frame: the track coasted.
		return out
	}
	copy(out[5:], []string{strconv.FormatBool(p.Known), f3(float64(p.LengthMetres)), f3(float64(p.WidthMetres)),
		f3(float64(p.AlongLo)), f3(float64(p.AlongHi)), f3(float64(p.AcrossLo)), f3(float64(p.AcrossHi)),
		f3(float64(p.X)), f3(float64(p.Y)), f3(float64(p.PsiRad) * 180 / math.Pi), profileEnds(p),
		strconv.FormatBool(p.Admitted), strconv.FormatBool(p.Withheld),
		strconv.Itoa(p.AdmittedFrames), strconv.Itoa(p.RefusedFrames), strconv.Itoa(p.HeadingRefusedFrames),
		strconv.FormatInt(p.LockedUnixNanos, 10)})
	if !p.Known {
		out[6], out[7], out[8], out[9], out[10], out[11] = "", "", "", "", "", ""
	}
	if p.Withheld && p.Known && cluster != nil && b.length > 0 {
		points := clusterPoints(cluster)
		if prof, boxShare, area, n := withheldScore(p, b, points); n > 0 {
			copy(out[22:], []string{f3(prof), f3(boxShare), f3(area), strconv.Itoa(n)})
			// geom0020's P4 restated: the solid body's own extent at its own
			// pose, grown the same, on the same returns.
			body := box{x: float64(e.X), y: float64(e.Y), psi: float64(e.Orientation.PsiRad),
				length: float64(e.Length.Metres), width: float64(e.Width.Metres)}
			if _, sbShare, sbArea, m := withheldScore(p, body, points); m > 0 {
				copy(out[26:], []string{f3(sbShare), f3(sbArea)})
			}
		}
	}
	return out
}

// profileEnds names each end observed (o) or censored (c), in the order
// rear, front, right, left.
func profileEnds(p l5tracks.ExtentProfileReading) string {
	if !p.Known {
		return ""
	}
	mark := func(observed bool) string {
		if observed {
			return "o"
		}
		return "c"
	}
	return mark(p.RearObserved) + mark(p.FrontObserved) + mark(p.RightObserved) + mark(p.LeftObserved)
}

// withheldScore is geom0019's P4 on one withheld frame: the share of the
// frame's returns inside the profile's extent placed at its pose, grown by
// profileGrowthMetres; the share inside the box grown by the same; and the
// profile's area over the box's. n is the number of returns.
func withheldScore(p l5tracks.ExtentProfileReading, b box, points []l4perception.WorldPoint) (profShare, boxShare, areaRatio float64, n int) {
	if len(points) == 0 || b.length <= 0 || b.width <= 0 {
		return 0, 0, 0, 0
	}
	pc, ps := math.Cos(float64(p.PsiRad)), math.Sin(float64(p.PsiRad))
	midU, midV := float64(p.AlongLo+p.AlongHi)/2, float64(p.AcrossLo+p.AcrossHi)/2
	px, py := float64(p.X)+midU*pc-midV*ps, float64(p.Y)+midU*ps+midV*pc
	halfL, halfW := float64(p.LengthMetres)/2+profileGrowthMetres, float64(p.WidthMetres)/2+profileGrowthMetres
	bc, bs := math.Cos(b.psi), math.Sin(b.psi)
	bL, bW := b.length/2+profileGrowthMetres, b.width/2+profileGrowthMetres
	inProf, inBox := 0, 0
	for _, q := range points {
		dx, dy := q.X-px, q.Y-py
		if math.Abs(dx*pc+dy*ps) <= halfL && math.Abs(-dx*ps+dy*pc) <= halfW {
			inProf++
		}
		dx, dy = q.X-b.x, q.Y-b.y
		if math.Abs(dx*bc+dy*bs) <= bL && math.Abs(-dx*bs+dy*bc) <= bW {
			inBox++
		}
	}
	n = len(points)
	return float64(inProf) / float64(n), float64(inBox) / float64(n),
		float64(p.LengthMetres) * float64(p.WidthMetres) / (b.length * b.width), n
}
