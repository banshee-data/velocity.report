package annotation

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Fitting a physical reference to reviewed points
// (docs/plans/lidar-physical-fit-to-points-plan.md).
//
// Once an object's masks are reviewed, its returns fix where its faces are in
// every frame. What is left to a person is whether an end the returns reach is
// the vehicle's real end or the edge of something in front of it. The fit
// measures the body's size from the frames that show it best, places a pose at
// each of those frames and at any frame asked for, and writes them as
// proposals in an import file, so they meet the same validation and review as
// anything typed by hand.
//
// It reads the reviewed membership and the pack's points, and nothing from a
// tracker: its records are independent proposals. It names itself in the
// review method, never in Provenance.Algorithm, which an independent record
// may not carry.
//
// A fitted box is the plan-view envelope of the reviewed returns, mirrors and
// all. A face sits at the outermost return along its normal, ignoring a few
// stragglers. Every bound is a conservative half-width built from named terms
// (FitBoundTerm): the azimuth step where a face is seen edge-on, the range
// accuracy where it is seen face-on, the stragglers ignored, and the chord a
// heading bound sweeps. Each frame's diagnostics report every term.

// FitMethod is the review method a fitted record carries.
const FitMethod = "fit:mask_v1"

// FitOptions are the fit's thresholds. DefaultFitOptions are the documented
// ones; tests may narrow them.
type FitOptions struct {
	// MinReturns is the fewest reviewed returns a frame needs to be fitted.
	MinReturns int
	// StragglerShare and MaxStragglers set how many outermost returns a face
	// ignores: the share of the object's returns in that frame, rounded down,
	// and never more than the cap. A face seen edge-on is a thin column of
	// returns that a percentage alone would remove on a dense object.
	StragglerShare float64
	MaxStragglers  int
	// AzimuthStepRad is the sensor's horizontal step between returns.
	AzimuthStepRad float64
	// RangeAccuracyM bounds a face seen face-on, placed by measured range.
	RangeAccuracyM float64
	// CourseWindowNs is how far either side of a frame its course is taken
	// from, and MinCourseM how far the object must move in that window for a
	// course to exist.
	CourseWindowNs int64
	MinCourseM     float64
	// JitterM is how far a centroid can move between frames without the
	// object moving, as its visible faces change. It bounds a course heading.
	JitterM float64
	// MinSideSpanM is the least extent along the axis for this frame's own
	// returns to fix the heading; MaxGeometryCourseRad the most the geometric
	// heading may differ from the course before the course is used instead.
	MinSideSpanM         float64
	MaxGeometryCourseRad float64
	// OcclusionRad and OccluderMarginM make the cut test: an end at the edge
	// of the object's silhouette is cut when another return lies within this
	// azimuth beyond it and this much nearer the sensor.
	OcclusionRad    float64
	OccluderMarginM float64
	// EndRegionM is how deep from an end its returns are taken to locate it.
	EndRegionM float64
	// DetachedGapM flags returns at an end separated from the rest along the
	// axis by more than this: usually a mask that holds something else.
	DetachedGapM float64
	// SideOnDeg is the aspect at and above which the side facing the sensor
	// runs end to end; EndOnDeg the aspect at and below which an end face
	// spans the body's width.
	SideOnDeg float64
	EndOnDeg  float64
	// RoofBelowSensorM and RoofRangeM admit a width measured across a roof
	// seen from above: the object's top this far below the sensor, within
	// this range.
	RoofBelowSensorM float64
	RoofRangeM       float64
	// MinSizeFrames is how many frames must measure a dimension end to end for
	// it to be stated as a full span; fewer give a lower bound.
	MinSizeFrames int
	// SizePoses is how many of the frames that measured a dimension get a
	// pose, so the dimension can be checked along its heading.
	SizePoses int
}

// DefaultFitOptions are the thresholds the plan documents.
func DefaultFitOptions() FitOptions {
	return FitOptions{
		MinReturns: 20, StragglerShare: 0.005, MaxStragglers: 5,
		AzimuthStepRad: 0.2 * math.Pi / 180, RangeAccuracyM: 0.03,
		CourseWindowNs: 1_000_000_000, MinCourseM: 2, JitterM: 0.5,
		MinSideSpanM: 2, MaxGeometryCourseRad: 15 * math.Pi / 180,
		OcclusionRad: math.Pi / 180, OccluderMarginM: 0.3, EndRegionM: 0.3, DetachedGapM: 0.5,
		SideOnDeg: 45, EndOnDeg: 30, RoofBelowSensorM: 0.3, RoofRangeM: 20,
		MinSizeFrames: 3, SizePoses: 3,
	}
}

// FitRequest names the object to fit and any frames to place a pose at, beside
// the frames its size is measured at.
type FitRequest struct {
	ObjectID string
	Samples  []int
	Author   string
	Session  string
}

// FitBoundTerm is one named contribution to a bound, in metres.
type FitBoundTerm struct {
	Name string  `json:"name"`
	M    float64 `json:"m"`
}

// Why a face is or is not seen.
const (
	fitReasonFacing   = "faces the sensor"
	fitReasonSideRuns = "the side facing the sensor runs to it"
	fitReasonCut      = "cut: a nearer return lies just beyond it, so this may be an occluder's edge"
	fitReasonAway     = "faces away, and the side facing the sensor does not run to it"
	fitReasonSideAway = "faces away"
)

// FitFace is what one frame shows of one face: whether the returns reach a
// real face, why, where it is along its own normal, and its bound.
type FitFace struct {
	Seen   bool           `json:"seen"`
	Reason string         `json:"reason"`
	AtM    float64        `json:"at_m"`
	BoundM float64        `json:"bound_m"`
	Terms  []FitBoundTerm `json:"terms"`
}

// FitFrame is one frame's diagnostics.
type FitFrame struct {
	SampleID    int      `json:"sample_id"`
	Returns     int      `json:"returns"`
	RangeM      float64  `json:"range_m"`
	AspectDeg   float64  `json:"aspect_deg"`
	YawRad      float64  `json:"yaw_rad"`
	YawBoundRad float64  `json:"yaw_bound_rad"`
	YawSource   string   `json:"yaw_source"`
	Axis        string   `json:"axis"`
	Front       FitFace  `json:"front"`
	Rear        FitFace  `json:"rear"`
	Left        FitFace  `json:"left"`
	Right       FitFace  `json:"right"`
	LengthSpanM float64  `json:"length_span_m"`
	WidthSpanM  float64  `json:"width_span_m"`
	HeightSpanM float64  `json:"height_span_m"`
	WidthHow    string   `json:"width_how,omitempty"`
	Detached    []string `json:"detached,omitempty"`
	UsedFor     []string `json:"used_for,omitempty"`
	Skipped     string   `json:"skipped,omitempty"`
}

// FitResult is a fitted object: the import file of proposals, the membership
// it was fitted from, and what every frame showed.
type FitResult struct {
	ObjectID           string                   `json:"object_id"`
	MembershipRevision int                      `json:"membership_revision"`
	MembershipDigest   string                   `json:"membership_digest"`
	Import             *PhysicalReferenceImport `json:"import"`
	Frames             []FitFrame               `json:"frames"`
	Notes              []string                 `json:"notes,omitempty"`
}

// fitSample is one frame of the object, with everything the fit derives.
type fitSample struct {
	id     int
	ts     int64
	pts    Points
	member []int
	other  []int
	cx, cy float64

	course      float64
	courseOK    bool
	courseBound float64
	courseFrom  []int

	yaw, yawBound float64
	yawSource     string
	axis          AxisState

	// along a, across c (c is positive to the body's left), height z
	aLo, aHi, cLo, cHi, zLo, zHi              float64
	aTrimLo, aTrimHi, cTrimLo, cTrimHi, zTrim float64
	aspectDeg, rangeM                         float64
	front, rear, left, right                  FitFace
	endRegionAcrossLo, endRegionAcrossHi      [2]float64 // rear, front
	endRegionRange                            [2]float64
	used                                      []string

	// widthSpan and widthBound measure the width in this frame when it can be
	// measured side to side; widthHow says how, empty when it cannot.
	widthSpan, widthBound float64
	widthHow              string
	// detached counts the returns at each end (rear, front) separated from
	// the rest along the axis by more than DetachedGapM, and the gap.
	detached    [2]int
	detachedGap [2]float64
}

// FitPhysicalObject fits an object's physical reference to its reviewed
// returns. The object and every one of its masks must be reviewed: a fit never
// rests on proposed membership.
func FitPhysicalObject(p *Pack, s *Sidecar, req FitRequest, opts FitOptions) (*FitResult, error) {
	if strings.TrimSpace(req.Author) == "" {
		return nil, fmt.Errorf("name the author the fitted proposals are recorded under")
	}
	if tv := p.Manifest.Coordinate.TransformVersion; tv != "" {
		return nil, fmt.Errorf("this pack's points carry site transform %q, so the sensor's position in its frame is unknown, and the fit needs it to tell which faces point at the sensor", tv)
	}
	var obj *Object
	for i := range s.Objects {
		if s.Objects[i].ObjectID == req.ObjectID {
			obj = &s.Objects[i]
		}
	}
	if obj == nil {
		return nil, fmt.Errorf("object %q is not in the annotation", req.ObjectID)
	}
	if obj.Status != StatusReviewed {
		return nil, fmt.Errorf("object %q is %s: review it before fitting", req.ObjectID, obj.Status)
	}
	masks := map[int]FrameMask{}
	proposed := 0
	for _, m := range s.Masks {
		if m.ObjectID != req.ObjectID {
			continue
		}
		switch m.Status {
		case StatusProposed:
			proposed++
		case StatusReviewed:
			masks[m.SampleID] = m
		}
	}
	if proposed > 0 {
		return nil, fmt.Errorf("%d of object %q's masks are still proposed: review every frame before fitting", proposed, req.ObjectID)
	}
	for _, id := range req.Samples {
		if m, ok := masks[id]; !ok || len(m.PointIndices) == 0 {
			return nil, fmt.Errorf("object %q has no reviewed returns at sample %d", req.ObjectID, id)
		}
	}

	res := &FitResult{ObjectID: req.ObjectID, MembershipRevision: s.Revision, MembershipDigest: s.Digest()}
	ids := make([]int, 0, len(masks))
	for id := range masks {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var frames []*fitSample
	for _, id := range ids {
		m := masks[id]
		if len(m.PointIndices) < opts.MinReturns {
			if len(m.PointIndices) > 0 {
				res.Frames = append(res.Frames, FitFrame{SampleID: id, Returns: len(m.PointIndices),
					Skipped: fmt.Sprintf("fewer than %d returns", opts.MinReturns)})
			}
			continue
		}
		fs, err := newFitSample(p, m)
		if err != nil {
			return nil, err
		}
		frames = append(frames, fs)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("object %q has no reviewed frame with at least %d returns", req.ObjectID, opts.MinReturns)
	}
	fitCourses(frames, opts)
	for _, f := range frames {
		f.fitHeading(opts)
		f.project(opts)
		f.faces(opts)
	}

	body, lengthFrames, widthFrames, notes := fitBody(frames, opts)
	res.Notes = append(res.Notes, notes...)
	created := time.Now().UTC().Format(time.RFC3339Nano)
	body.BodyID = newPhysicalID("body")
	body.Review = fitReview(req, created, bodyAssumptions(body, opts))

	posed, withPose := map[int]bool{}, map[int]bool{}
	var poseIDs []int
	for _, group := range [][]int{lengthFrames, widthFrames, req.Samples} {
		for _, id := range group {
			if !posed[id] {
				posed[id] = true
				poseIDs = append(poseIDs, id)
			}
		}
	}
	sort.Ints(poseIDs)
	byID := map[int]*fitSample{}
	for _, f := range frames {
		byID[f.id] = f
	}
	var keyframes []PhysicalKeyframe
	for _, id := range poseIDs {
		f := byID[id]
		if f == nil {
			res.Notes = append(res.Notes, fmt.Sprintf("sample %d: fewer than %d returns, no pose", id, opts.MinReturns))
			continue
		}
		k, why := fitPose(p, f, body, lengthFrames, opts)
		if k == nil {
			res.Notes = append(res.Notes, fmt.Sprintf("sample %d: no pose: %s", id, why))
			continue
		}
		withPose[id] = true
		k.KeyframeID = newPhysicalID("keyframe")
		k.Review = fitReview(req, created, k.Review.UncertaintyAssumptions)
		keyframes = append(keyframes, *k)
		f.used = append(f.used, "pose")
	}
	// A dimension checked along a heading needs a pose at each frame it cites.
	for _, d := range []struct {
		name string
		d    *DimensionBound
	}{{"length", &body.Length}, {"width", &body.Width}} {
		if note := trimSupport(d.d, withPose); note != "" {
			res.Notes = append(res.Notes, d.name+": "+note)
		}
	}

	res.Notes = append(res.Notes, detachedSummary(frames)...)
	for _, f := range frames {
		res.Frames = append(res.Frames, f.diagnostics())
	}
	sort.Slice(res.Frames, func(i, j int) bool { return res.Frames[i].SampleID < res.Frames[j].SampleID })
	src := PackPhysicalSource(p)
	res.Import = &PhysicalReferenceImport{
		Schema: PhysicalImportSchema, SchemaVersion: PhysicalImportSchemaVersion,
		PackDigest: p.Manifest.PackDigest, DatasetID: p.Manifest.DatasetID, Source: src,
		Objects: []PhysicalObject{{ObjectID: req.ObjectID, Body: &body, Keyframes: keyframes}},
	}
	return res, nil
}

func newFitSample(p *Pack, m FrameMask) (*fitSample, error) {
	pts, err := p.PointsAt(m.SampleID)
	if err != nil {
		return nil, err
	}
	skip := make([]bool, len(pts.X))
	for _, i := range m.PointIndices {
		skip[i] = true
	}
	for _, i := range m.UncertainIndices {
		skip[i] = true
	}
	f := &fitSample{id: m.SampleID, ts: p.Samples[m.SampleID].TimestampNs, pts: pts, member: m.PointIndices}
	for i := range pts.X {
		if !skip[i] {
			f.other = append(f.other, i)
		}
	}
	for _, i := range f.member {
		f.cx += float64(pts.X[i])
		f.cy += float64(pts.Y[i])
	}
	f.cx /= float64(len(f.member))
	f.cy /= float64(len(f.member))
	return f, nil
}

// fitCourses takes each frame's course from the least-squares velocity of the
// object's centroids within the window either side of it.
func fitCourses(frames []*fitSample, opts FitOptions) {
	for _, f := range frames {
		var n, st, sx, sy, stt, stx, sty float64
		tMin, tMax := math.Inf(1), math.Inf(-1)
		var from []int
		for _, g := range frames {
			if d := g.ts - f.ts; d < -opts.CourseWindowNs || d > opts.CourseWindowNs {
				continue
			}
			t := float64(g.ts-f.ts) / 1e9
			n++
			st, sx, sy = st+t, sx+g.cx, sy+g.cy
			stt, stx, sty = stt+t*t, stx+t*g.cx, sty+t*g.cy
			tMin, tMax = math.Min(tMin, t), math.Max(tMax, t)
			from = append(from, g.id)
		}
		den := n*stt - st*st
		if n < 3 || den <= 0 {
			continue
		}
		vx, vy := (n*stx-st*sx)/den, (n*sty-st*sy)/den
		moved := math.Hypot(vx, vy) * (tMax - tMin)
		if moved < opts.MinCourseM {
			continue
		}
		f.course, f.courseOK, f.courseFrom = math.Atan2(vy, vx), true, from
		f.courseBound = math.Max(2*math.Pi/180, math.Atan(opts.JitterM/moved))
	}
}

// fitHeading sets the frame's heading: the side of the minimum-area rectangle
// around its returns nearest the course, when its own returns run far enough
// along it and agree with the course; the course otherwise. A frame with no
// course takes the rectangle's longer side, with front and rear ambiguous.
func (f *fitSample) fitHeading(opts FitOptions) {
	xs, ys := f.xy()
	theta := minAreaRectAngle(xs, ys)
	if !f.courseOK {
		along, across := extentAt(xs, ys, theta), extentAt(xs, ys, theta+math.Pi/2)
		f.yaw = theta
		if across > along {
			f.yaw = theta + math.Pi/2
			along = across
		}
		f.yawSource, f.axis = "geometry", AxisFrontRearAmbiguous
		f.yawBound = math.Pi / 180
		if along < opts.MinSideSpanM {
			f.yawBound = 5 * math.Pi / 180
		}
		return
	}
	f.axis = AxisResolved
	geom := theta
	for k := 1; k < 4; k++ {
		if c := theta + float64(k)*math.Pi/2; math.Abs(angleDiff(c, f.course)) < math.Abs(angleDiff(geom, f.course)) {
			geom = c
		}
	}
	diff := math.Abs(angleDiff(geom, f.course))
	if diff <= opts.MaxGeometryCourseRad && extentAt(xs, ys, geom) >= opts.MinSideSpanM {
		f.yaw, f.yawSource = normaliseAngle(geom), "geometry"
		f.yawBound = math.Max(0.5*math.Pi/180, diff)
		return
	}
	f.yaw, f.yawSource, f.yawBound = f.course, "course", f.courseBound
}

func (f *fitSample) xy() ([]float64, []float64) {
	xs, ys := make([]float64, len(f.member)), make([]float64, len(f.member))
	for j, i := range f.member {
		xs[j], ys[j] = float64(f.pts.X[i]), float64(f.pts.Y[i])
	}
	return xs, ys
}

// stragglers is how many outermost returns a face ignores.
func stragglers(n int, opts FitOptions) int {
	k := int(math.Floor(opts.StragglerShare * float64(n)))
	if k > opts.MaxStragglers {
		k = opts.MaxStragglers
	}
	return k
}

// trimmed returns a sorted coordinate's extremes less k stragglers each side,
// and how far the outermost return lies beyond each.
func trimmed(v []float64, k int) (lo, hi, trimLo, trimHi float64) {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	n := len(s)
	lo, hi = s[k], s[n-1-k]
	return lo, hi, lo - s[0], s[n-1] - hi
}

// project measures the frame's returns along and across its heading.
func (f *fitSample) project(opts FitOptions) {
	ux, uy := math.Cos(f.yaw), math.Sin(f.yaw)
	n := len(f.member)
	a, c, z := make([]float64, n), make([]float64, n), make([]float64, n)
	for j, i := range f.member {
		x, y := float64(f.pts.X[i]), float64(f.pts.Y[i])
		a[j], c[j], z[j] = x*ux+y*uy, -x*uy+y*ux, float64(f.pts.Z[i])
	}
	k := stragglers(n, opts)
	f.aLo, f.aHi, f.aTrimLo, f.aTrimHi = trimmed(a, k)
	f.findDetached(a, opts)
	f.cLo, f.cHi, f.cTrimLo, f.cTrimHi = trimmed(c, k)
	var zTrimLo, zTrimHi float64
	f.zLo, f.zHi, zTrimLo, zTrimHi = trimmed(z, k)
	f.zTrim = zTrimLo + zTrimHi
	f.rangeM = math.Hypot(f.cx, f.cy)
	if f.rangeM > 0 {
		cosA := math.Abs((f.cx*ux + f.cy*uy) / f.rangeM)
		f.aspectDeg = math.Acos(math.Min(1, cosA)) * 180 / math.Pi
	}
	// The returns within EndRegionM of each end locate it: their extent
	// across, and the nearest of them.
	for e, end := range []float64{f.aLo, f.aHi} {
		lo, hi, near := math.Inf(1), math.Inf(-1), math.Inf(1)
		for j, i := range f.member {
			if math.Abs(a[j]-end) > opts.EndRegionM {
				continue
			}
			lo, hi = math.Min(lo, c[j]), math.Max(hi, c[j])
			near = math.Min(near, math.Hypot(float64(f.pts.X[i]), float64(f.pts.Y[i])))
		}
		f.endRegionAcrossLo[e], f.endRegionAcrossHi[e], f.endRegionRange[e] = lo, hi, near
	}
}

// faces decides, for each end and side, whether the returns reach a real face,
// and bounds where it is.
func (f *fitSample) faces(opts FitOptions) {
	ux, uy := math.Cos(f.yaw), math.Sin(f.yaw)
	vx, vy := -uy, ux
	aMid, cMid := (f.aLo+f.aHi)/2, (f.cLo+f.cHi)/2
	at := func(a, c float64) (float64, float64) { return a*ux + c*vx, a*uy + c*vy }
	// facing: the face's outward normal points toward the sensor at the origin.
	facing := func(px, py, nx, ny float64) bool { return -(px*nx + py*ny) > 0 }
	aspect := f.aspectDeg * math.Pi / 180
	step := opts.AzimuthStepRad
	headingChord := func(halfExtent float64) FitBoundTerm {
		return FitBoundTerm{"heading", halfExtent * math.Tan(f.yawBound)}
	}
	// incidence is the angle between a face's normal and the line of sight.
	endIncidence, sideIncidence := aspect, math.Pi/2-aspect

	for e, face := range []*FitFace{&f.rear, &f.front} {
		sign := float64(2*e - 1) // -1 rear, +1 front
		end := f.aLo
		trim := f.aTrimLo
		if e == 1 {
			end, trim = f.aHi, f.aTrimHi
		}
		px, py := at(end, cMid)
		r := math.Hypot(px, py)
		face.AtM = end
		faces := facing(px, py, sign*ux, sign*uy)
		var terms []FitBoundTerm
		if faces {
			terms = []FitBoundTerm{{"range", opts.RangeAccuracyM}, {"sampling", r * step * math.Sin(endIncidence)}}
		} else {
			terms = []FitBoundTerm{{"sampling", r * step / math.Max(math.Sin(aspect), 0.25)}}
		}
		terms = append(terms, FitBoundTerm{"stragglers", trim}, headingChord((f.cHi-f.cLo)/2))
		face.Terms, face.BoundM = terms, sumTerms(terms)
		switch {
		case f.endCut(e, opts):
			face.Reason = fitReasonCut
		case faces:
			face.Seen, face.Reason = true, fitReasonFacing
		case f.aspectDeg >= opts.SideOnDeg:
			face.Seen, face.Reason = true, fitReasonSideRuns
		default:
			face.Reason = fitReasonAway
		}
	}
	for e, face := range []*FitFace{&f.right, &f.left} {
		sign := float64(2*e - 1) // -1 right, +1 left
		side, trim := f.cLo, f.cTrimLo
		if e == 1 {
			side, trim = f.cHi, f.cTrimHi
		}
		px, py := at(aMid, side)
		r := math.Hypot(px, py)
		face.AtM = side
		terms := []FitBoundTerm{{"range", opts.RangeAccuracyM}, {"sampling", r * step * math.Sin(sideIncidence)},
			{"stragglers", trim}, headingChord((f.aHi - f.aLo) / 2)}
		face.Terms, face.BoundM = terms, sumTerms(terms)
		if facing(px, py, sign*vx, sign*vy) {
			face.Seen, face.Reason = true, fitReasonFacing
		} else {
			face.Reason = fitReasonSideAway
		}
	}
	f.measureWidth(opts)
}

// measureWidth measures the width side to side where a frame can: across an
// end face seen square-on, which spans the body's width, or across a roof
// seen from above. An end face is measured from its own returns alone, so a
// heading error tilts only that thin slice, not the whole body.
func (f *fitSample) measureWidth(opts FitOptions) {
	sampling := 2 * (f.rangeM*opts.AzimuthStepRad + opts.RangeAccuracyM)
	for e, face := range []FitFace{f.rear, f.front} {
		if f.aspectDeg > opts.EndOnDeg || !face.Seen || face.Reason != fitReasonFacing {
			continue
		}
		if span := f.endRegionAcrossHi[e] - f.endRegionAcrossLo[e]; span >= 1 {
			f.widthSpan, f.widthHow = span, "end face"
			f.widthBound = sampling + 2*opts.EndRegionM*math.Tan(f.yawBound)
			return
		}
	}
	if f.zHi <= -opts.RoofBelowSensorM && f.rangeM <= opts.RoofRangeM && f.cHi-f.cLo >= 1 {
		f.widthSpan, f.widthHow = f.cHi-f.cLo, "roof"
		f.widthBound = sampling + f.cTrimLo + f.cTrimHi + (f.aHi-f.aLo)*math.Tan(f.yawBound)
	}
}

// findDetached looks, at each end, for returns separated from the rest along
// the axis by more than DetachedGapM among the outermost few.
func (f *fitSample) findDetached(a []float64, opts FitOptions) {
	s := append([]float64(nil), a...)
	sort.Float64s(s)
	n := len(s)
	limit := n / 50
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}
	for i := 0; i < limit && i+1 < n; i++ {
		if g := s[i+1] - s[i]; g > opts.DetachedGapM && g > f.detachedGap[0] {
			f.detached[0], f.detachedGap[0] = i+1, g
		}
		if g := s[n-1-i] - s[n-2-i]; g > opts.DetachedGapM && g > f.detachedGap[1] {
			f.detached[1], f.detachedGap[1] = i+1, g
		}
	}
}

// detachedSummary reports, for each end, the frames where returns lie apart
// from the body along its axis, in runs, so a mask holding something else is
// found without reading every frame.
func detachedSummary(frames []*fitSample) []string {
	var out []string
	for e, name := range []string{"rear", "front"} {
		var runs []string
		count, most, worst, start, prev := 0, 0.0, -1, -1, -1
		flush := func() {
			if start < 0 {
				return
			}
			if start == prev {
				runs = append(runs, fmt.Sprint(start))
			} else {
				runs = append(runs, fmt.Sprintf("%d-%d", start, prev))
			}
		}
		for _, f := range frames {
			if f.detached[e] == 0 {
				continue
			}
			count++
			if f.detachedGap[e] > most {
				most, worst = f.detachedGap[e], f.id
			}
			if prev >= 0 && f.id == prev+1 {
				prev = f.id
				continue
			}
			flush()
			start, prev = f.id, f.id
		}
		flush()
		if count > 0 {
			out = append(out, fmt.Sprintf("%s: in %d frames (samples %s) a few returns lie apart from the body along its axis, "+
				"up to %.2f m at sample %d; check those masks for returns of something else", name, count, strings.Join(runs, ", "), most, worst))
		}
	}
	return out
}

func (f *fitSample) detachedNotes() []string {
	var out []string
	for e, name := range []string{"rear", "front"} {
		if f.detached[e] > 0 {
			out = append(out, fmt.Sprintf("%d returns at the %s lie %.2f m beyond the rest along the axis: check the mask", f.detached[e], name, f.detachedGap[e]))
		}
	}
	return out
}

// endCut reports whether an end at the edge of the object's silhouette has a
// nearer return just beyond it, so the end the mask reaches may be where an
// occluder begins rather than where the body ends.
func (f *fitSample) endCut(e int, opts FitOptions) bool {
	centre := math.Atan2(f.cy, f.cx)
	rel := func(i int) float64 { return angleDiff(math.Atan2(float64(f.pts.Y[i]), float64(f.pts.X[i])), centre) }
	oLo, oHi := math.Inf(1), math.Inf(-1)
	ux, uy := math.Cos(f.yaw), math.Sin(f.yaw)
	end := f.aLo
	if e == 1 {
		end = f.aHi
	}
	eLo, eHi := math.Inf(1), math.Inf(-1)
	for _, i := range f.member {
		r := rel(i)
		oLo, oHi = math.Min(oLo, r), math.Max(oHi, r)
		if math.Abs(float64(f.pts.X[i])*ux+float64(f.pts.Y[i])*uy-end) <= opts.EndRegionM {
			eLo, eHi = math.Min(eLo, r), math.Max(eHi, r)
		}
	}
	tol := 2 * opts.AzimuthStepRad
	near := f.endRegionRange[e] - opts.OccluderMarginM
	check := func(from, to float64) bool {
		for _, i := range f.other {
			r := rel(i)
			if r > from && r <= to && math.Hypot(float64(f.pts.X[i]), float64(f.pts.Y[i])) < near {
				return true
			}
		}
		return false
	}
	if eHi >= oHi-tol && check(oHi, oHi+opts.OcclusionRad) {
		return true
	}
	return eLo <= oLo+tol && check(oLo-opts.OcclusionRad, oLo)
}

// fitBody measures the size: length from frames that see both ends, width
// from frames that see an end face square-on or a roof from above, and height
// as the vertical span of the returns, a lower bound. It returns the frames
// that measured length and width, which need poses.
func fitBody(frames []*fitSample, opts FitOptions) (BodyGeometry, []int, []int, []string) {
	b := BodyGeometry{AxisConvention: BodyAxisConvention}
	var notes []string
	var lens, wids []sizeMeasure
	for _, f := range frames {
		if f.front.Seen && f.rear.Seen {
			lens = append(lens, sizeMeasure{f, f.aHi - f.aLo, f.front.BoundM + f.rear.BoundM})
		}
		if f.widthHow != "" {
			wids = append(wids, sizeMeasure{f, f.widthSpan, f.widthBound})
		}
	}
	// A lower bound comes from the longest span among frames with a firm
	// heading, so a tilted heading does not lengthen it.
	firm := func(span func(*fitSample) float64) []sizeMeasure {
		var out, all []sizeMeasure
		for _, f := range frames {
			m := sizeMeasure{f, span(f), 0}
			all = append(all, m)
			if f.yawSource == "geometry" && f.yawBound <= 2*math.Pi/180 {
				out = append(out, m)
			}
		}
		if len(out) == 0 {
			return all
		}
		return out
	}
	var lengthFrames, widthFrames []int
	b.Length, lengthFrames = fitDimension("length", lens, firm(func(f *fitSample) float64 { return f.aHi - f.aLo }), opts)
	b.Width, widthFrames = fitDimension("width", wids, firm(func(f *fitSample) float64 { return f.cHi - f.cLo }), opts)
	if b.Length.Span == SpanPartial {
		notes = append(notes, fmt.Sprintf("length: fewer than %d frames see both ends, so it is a lower bound", opts.MinSizeFrames))
	}
	if b.Width.Span == SpanPartial {
		notes = append(notes, fmt.Sprintf("width: fewer than %d frames see an end face square-on or a roof from above, so it is a lower bound", opts.MinSizeFrames))
	}
	// Height: the vertical span of the returns, a lower bound, cited from the
	// frame that shows the most of it. It needs no heading to check.
	best := frames[0]
	for _, f := range frames {
		if f.zHi-f.zLo > best.zHi-best.zLo {
			best = f
		}
	}
	b.Height = DimensionBound{Status: EvidenceObserved, Span: SpanPartial, LowerM: fp64(round3(best.zHi - best.zLo)),
		Support: EvidenceSupport{Frames: []int{best.id}}}
	best.used = append(best.used, "height")
	return b, lengthFrames, widthFrames, notes
}

// sizeMeasure is one frame's measurement of a dimension and its bound.
type sizeMeasure struct {
	f            *fitSample
	span, boundM float64
}

// fitDimension states one dimension from the frames that measure it end to
// end, using the better-bounded half of them. A frame sees at most the whole
// body, so each span is a lower bound on the dimension and the median
// understates it: the value is the 90th percentile of the spans, and its
// half-width the median of those frames' bounds. Fewer than MinSizeFrames give
// a lower bound instead: the longest span among the fallback frames. The
// frames cited are the most tightly bounded of those at or above the median
// span, and get poses.
func fitDimension(name string, full, fallback []sizeMeasure, opts FitOptions) (DimensionBound, []int) {
	if len(full) >= opts.MinSizeFrames {
		sort.SliceStable(full, func(i, j int) bool { return full[i].boundM < full[j].boundM })
		if keep := (len(full) + 1) / 2; keep >= opts.MinSizeFrames {
			full = full[:keep]
		} else {
			full = full[:opts.MinSizeFrames]
		}
		spans, bounds := make([]float64, len(full)), make([]float64, len(full))
		for i, m := range full {
			spans[i], bounds[i] = m.span, m.boundM
			m.f.used = append(m.f.used, name)
		}
		value, mid, hw := quantile(spans, 0.9), quantile(spans, 0.5), quantile(bounds, 0.5)
		var long []*fitSample
		for _, m := range full { // sorted by bound, tightest first
			if m.span >= mid && len(long) < opts.SizePoses {
				long = append(long, m.f)
			}
		}
		cite := make([]int, len(long))
		for i, f := range long {
			cite[i] = f.id
		}
		sort.Ints(cite)
		return DimensionBound{Status: EvidenceObserved, Span: SpanFull,
			LowerM: fp64(round3(math.Max(0, value-hw))), UpperM: fp64(round3(value + hw)), ValueM: fp64(round3(value)),
			Support: EvidenceSupport{Frames: cite}}, cite
	}
	best := fallback[0]
	for _, m := range fallback {
		if m.span > best.span {
			best = m
		}
	}
	best.f.used = append(best.f.used, name+" (lower bound)")
	return DimensionBound{Status: EvidenceObserved, Span: SpanPartial, LowerM: fp64(round3(best.span)),
		Support: EvidenceSupport{Frames: []int{best.f.id}}}, []int{best.f.id}
}

// trimSupport drops cited frames that got no pose: a dimension measured along
// a heading can be checked only at a frame with a keyframe yaw. With none
// left, a full span is stated as inferred from the frames that measured it,
// and a lower bound, which only an observation can state, is withdrawn.
func trimSupport(d *DimensionBound, posed map[int]bool) string {
	var keep []int
	for _, f := range d.Support.Frames {
		if posed[f] {
			keep = append(keep, f)
		}
	}
	switch {
	case len(keep) > 0:
		d.Support.Frames = keep
		return ""
	case d.Span == SpanFull:
		d.Status = EvidenceInferred
		return "no frame that measured it could take a pose, so it is inferred from those frames rather than observed"
	}
	*d = DimensionBound{Status: EvidenceUnknown}
	return "the frame that showed it could take no pose, so it is left unknown"
}

// fitPose places the fitted body on one frame's returns, anchored on what the
// frame shows. It returns nil and the reason when it cannot bound a position.
func fitPose(p *Pack, f *fitSample, body BodyGeometry, lengthFrames []int, opts FitOptions) (*PhysicalKeyframe, string) {
	L, Lhw, Lok := body.Length.Best()
	W, Whw, Wok := body.Width.Best()
	Lok, Wok = Lok && body.Length.Span == SpanFull, Wok && body.Width.Span == SpanFull
	k := &PhysicalKeyframe{SampleID: f.id, TimestampNs: p.Samples[f.id].TimestampNs}
	own := EvidenceSupport{Frames: []int{f.id}}

	k.Yaw = YawBound{Axis: f.axis, YawRad: fp64(round6(normaliseAngle(f.yaw))), BoundRad: fp64(round6(f.yawBound))}
	if f.yawSource == "geometry" {
		k.Yaw.Status, k.Yaw.Support = EvidenceObserved, own
	} else {
		k.Yaw.Status, k.Yaw.Support = EvidenceInferred, EvidenceSupport{Frames: sortedUnique(append(f.courseFrom, f.id))}
	}
	resolved := f.axis == AxisResolved
	frontSeen, rearSeen := resolved && f.front.Seen, resolved && f.rear.Seen
	if f.axis == AxisFrontRearAmbiguous && f.front.Seen && f.rear.Seen {
		frontSeen, rearSeen = true, true // unsigned: both ends seen, neither named
	}

	// Along the axis.
	var along, alongB float64
	var alongTerms []string
	anchor := PhysicalAnchor{Kind: AnchorBodyCentre}
	switch {
	case frontSeen && rearSeen:
		along = (f.aLo + f.aHi) / 2
		alongB = (f.front.BoundM + f.rear.BoundM) / 2
		if Lok {
			alongB = math.Max(alongB, math.Abs((f.aHi-f.aLo)-L)/2)
		}
		alongTerms = append(alongTerms, "midway between the two ends seen")
	case resolved && (frontSeen || rearSeen):
		face, end := f.front, f.aHi
		anchor.Kind = AnchorFrontFace
		if rearSeen {
			face, end, anchor.Kind = f.rear, f.aLo, AnchorRearFace
		}
		along, alongB = end, face.BoundM
		if Lok {
			anchor.OffsetM, anchor.OffsetBoundM = fp64(round3(L/2)), fp64(round3(Lhw/2))
		}
		alongTerms = append(alongTerms, "on the "+map[AnchorKind]string{AnchorFrontFace: "front", AnchorRearFace: "rear"}[anchor.Kind]+" face seen")
	case Lok:
		// No end seen: the body must still cover the returns, so its centre
		// lies between where each end could be.
		lo, hi := f.aHi-L/2, f.aLo+L/2
		if hi < lo {
			lo, hi = hi, lo
		}
		along, alongB = (lo+hi)/2, (hi-lo)/2+Lhw/2
		alongTerms = append(alongTerms, "no end seen: anywhere the fitted length still covers the returns")
	default:
		return nil, "no end is seen and the length is only a lower bound, so the position along the body cannot be bounded"
	}

	// Across the axis.
	var across, acrossB float64
	endOnRegion := -1
	if f.aspectDeg <= opts.EndOnDeg {
		if anchor.Kind == AnchorRearFace || (anchor.Kind == AnchorBodyCentre && rearSeen && f.rear.Reason == fitReasonFacing) {
			endOnRegion = 0
		} else if anchor.Kind == AnchorFrontFace || (anchor.Kind == AnchorBodyCentre && frontSeen && f.front.Reason == fitReasonFacing) {
			endOnRegion = 1
		}
	}
	switch {
	case f.right.Seen && Wok:
		across, acrossB = f.cLo+W/2, f.right.BoundM+Whw/2
	case f.left.Seen && Wok:
		across, acrossB = f.cHi-W/2, f.left.BoundM+Whw/2
	case endOnRegion >= 0 && f.endRegionAcrossHi[endOnRegion]-f.endRegionAcrossLo[endOnRegion] >= 1:
		across = (f.endRegionAcrossLo[endOnRegion] + f.endRegionAcrossHi[endOnRegion]) / 2
		acrossB = f.rangeM*opts.AzimuthStepRad + opts.RangeAccuracyM
	case Wok:
		lo, hi := f.cHi-W/2, f.cLo+W/2
		if hi < lo {
			lo, hi = hi, lo
		}
		across, acrossB = (lo+hi)/2, (hi-lo)/2+Whw/2
	case resolved && (f.right.Seen || f.left.Seen):
		// The width is only a lower bound, so the centre across is unknown:
		// locate the side seen instead, at its middle along the body. A face
		// with no offset locates the face, not the centre.
		side, face := AnchorRightFace, f.right
		across = f.cLo
		if !f.right.Seen {
			side, face, across = AnchorLeftFace, f.left, f.cHi
		}
		acrossB = face.BoundM
		switch {
		case frontSeen && rearSeen:
			// already midway between the two ends seen
		case Lok && frontSeen:
			along, alongB = f.aHi-L/2, f.front.BoundM+Lhw/2
		case Lok && rearSeen:
			along, alongB = f.aLo+L/2, f.rear.BoundM+Lhw/2
		default:
			return nil, "the width is only a lower bound and too little of the length is seen to find the middle of the side"
		}
		anchor = PhysicalAnchor{Kind: side}
		alongTerms = []string{"on the side seen, midway along the body; the width is only a lower bound, so the centre is not located"}
	default:
		return nil, "no side is seen and the width is only a lower bound, so the position across the body cannot be bounded"
	}

	ux, uy := math.Cos(f.yaw), math.Sin(f.yaw)
	x, y := along*ux-across*uy, along*uy+across*ux
	bound := math.Hypot(alongB, acrossB)
	k.Anchor = anchor
	k.Position = PositionBound{XM: fp64(round3(x)), YM: fp64(round3(y)), BoundM: fp64(round3(bound))}
	if anchor.Kind != AnchorBodyCentre {
		k.Position.Status, k.Position.Support = EvidenceObserved, own
	} else {
		k.Position.Status = EvidenceInferred
		k.Position.Support = EvidenceSupport{Frames: sortedUnique(append(append([]int(nil), body.Length.Support.Frames...), f.id))}
	}

	sideNoOffset := (anchor.Kind == AnchorLeftFace || anchor.Kind == AnchorRightFace) && anchor.OffsetM == nil
	endEvidence := func(seen bool) EndpointEvidence {
		switch {
		case !resolved:
			return EndpointEvidence{Status: EvidenceUnknown}
		case seen && !sideNoOffset:
			return EndpointEvidence{Status: EvidenceObserved, Support: own}
		case Lok:
			return EndpointEvidence{Status: EvidenceInferred, Support: EvidenceSupport{Frames: sortedUnique(append(append([]int(nil), lengthFrames...), f.id))}}
		}
		return EndpointEvidence{Status: EvidenceUnknown}
	}
	k.Front, k.Rear = endEvidence(frontSeen), endEvidence(rearSeen)
	k.Review.UncertaintyAssumptions = poseAssumptions(f, anchor, alongB, acrossB, alongTerms)
	return k, ""
}

func fitReview(req FitRequest, created, assumptions string) PhysicalReview {
	return PhysicalReview{
		Status: StatusProposed, Origin: OriginIndependent, Method: FitMethod,
		UncertaintyAssumptions: assumptions,
		Provenance: Provenance{Author: req.Author, Session: req.Session, Operation: "fit_to_points",
			CreatedUTC: created},
	}
}

// fitConvention is what every fitted record states it assumed.
const fitConvention = "Fitted to the reviewed returns (" + FitMethod + "): the box is the plan-view envelope of the object's " +
	"reviewed returns, mirrors included; a face sits at the outermost return less up to five stragglers. " +
	"Bounds are conservative half-widths adding the sensor's azimuth step where a face is seen edge-on, its range accuracy " +
	"where seen face-on, the stragglers ignored, and the chord the heading bound sweeps."

func bodyAssumptions(b BodyGeometry, opts FitOptions) string {
	var parts []string
	for _, d := range []struct {
		name string
		d    DimensionBound
	}{{"length", b.Length}, {"width", b.Width}} {
		if d.d.Span == SpanFull {
			v, hw, _ := d.d.Best()
			parts = append(parts, fmt.Sprintf("%s %.2f ± %.2f m: the 90th percentile of the spans of the frames that measure it end to end "+
				"(each span is a lower bound, as a frame sees at most the whole body), its half-width the median bound of the longer half", d.name, v, hw))
		} else {
			parts = append(parts, fmt.Sprintf("%s at least %.2f m: fewer than %d frames measure it end to end", d.name, *d.d.LowerM, opts.MinSizeFrames))
		}
	}
	parts = append(parts, fmt.Sprintf("height at least %.2f m: the vertical span of the returns", *b.Height.LowerM))
	return fitConvention + " " + strings.Join(parts, "; ") + "."
}

func poseAssumptions(f *fitSample, anchor PhysicalAnchor, alongB, acrossB float64, along []string) string {
	src := "the minimum-area rectangle around this frame's returns, nearest the course"
	if f.yawSource == "course" {
		src = "the course of the object's centroids within a second either side"
	} else if f.axis == AxisFrontRearAmbiguous {
		src = "the longer side of the minimum-area rectangle; the object does not move, so front and rear are not known"
	}
	describe := func(name string, face FitFace) string {
		return fmt.Sprintf("%s %s (±%.2f m: %s)", name, face.Reason, face.BoundM, termsText(face.Terms))
	}
	return fmt.Sprintf("%s Heading from %s, ±%.1f°. %s; %s. Anchor %s, %s; along ±%.2f m, across ±%.2f m, combined as a radius.",
		fitConvention, src, f.yawBound*180/math.Pi, describe("front", f.front), describe("rear", f.rear),
		anchor.Kind, strings.Join(along, ", "), alongB, acrossB)
}

func termsText(ts []FitBoundTerm) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = fmt.Sprintf("%s %.3f", t.Name, t.M)
	}
	return strings.Join(parts, ", ")
}

func (f *fitSample) diagnostics() FitFrame {
	round := func(x FitFace) FitFace {
		x.AtM, x.BoundM = round3(x.AtM), round3(x.BoundM)
		for i := range x.Terms {
			x.Terms[i].M = round3(x.Terms[i].M)
		}
		return x
	}
	return FitFrame{
		SampleID: f.id, Returns: len(f.member), RangeM: round3(f.rangeM), AspectDeg: round3(f.aspectDeg),
		YawRad: round6(normaliseAngle(f.yaw)), YawBoundRad: round6(f.yawBound), YawSource: f.yawSource, Axis: string(f.axis),
		Front: round(f.front), Rear: round(f.rear), Left: round(f.left), Right: round(f.right),
		LengthSpanM: round3(f.aHi - f.aLo), WidthSpanM: round3(f.cHi - f.cLo), HeightSpanM: round3(f.zHi - f.zLo),
		WidthHow: f.widthHow, Detached: f.detachedNotes(), UsedFor: f.used,
	}
}

// minAreaRectAngle is the direction, in [0, pi/2), of a side of the
// minimum-area rectangle enclosing the points: one side of it lies along an
// edge of their convex hull.
func minAreaRectAngle(xs, ys []float64) float64 {
	hull := convexHull(xs, ys)
	if len(hull) < 3 {
		if len(hull) == 2 {
			return math.Mod(math.Atan2(hull[1][1]-hull[0][1], hull[1][0]-hull[0][0])+2*math.Pi, math.Pi/2)
		}
		return 0
	}
	bestArea, best := math.Inf(1), 0.0
	for i := range hull {
		j := (i + 1) % len(hull)
		theta := math.Atan2(hull[j][1]-hull[i][1], hull[j][0]-hull[i][0])
		c, s := math.Cos(theta), math.Sin(theta)
		aLo, aHi, bLo, bHi := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for _, h := range hull {
			a, b := h[0]*c+h[1]*s, -h[0]*s+h[1]*c
			aLo, aHi, bLo, bHi = math.Min(aLo, a), math.Max(aHi, a), math.Min(bLo, b), math.Max(bHi, b)
		}
		if area := (aHi - aLo) * (bHi - bLo); area < bestArea {
			bestArea, best = area, theta
		}
	}
	return math.Mod(best+4*math.Pi, math.Pi/2)
}

// convexHull is Andrew's monotone chain, counter-clockwise.
func convexHull(xs, ys []float64) [][2]float64 {
	pts := make([][2]float64, len(xs))
	for i := range xs {
		pts[i] = [2]float64{xs[i], ys[i]}
	}
	sort.Slice(pts, func(i, j int) bool {
		if pts[i][0] != pts[j][0] {
			return pts[i][0] < pts[j][0]
		}
		return pts[i][1] < pts[j][1]
	})
	cross := func(o, a, b [2]float64) float64 {
		return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
	}
	var hull [][2]float64
	for _, p := range pts {
		for len(hull) >= 2 && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	lower := len(hull) + 1
	for i := len(pts) - 2; i >= 0; i-- {
		for len(hull) >= lower && cross(hull[len(hull)-2], hull[len(hull)-1], pts[i]) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, pts[i])
	}
	if len(hull) > 1 {
		hull = hull[:len(hull)-1]
	}
	return hull
}

// extentAt is the points' extent along a direction.
func extentAt(xs, ys []float64, theta float64) float64 {
	c, s := math.Cos(theta), math.Sin(theta)
	lo, hi := math.Inf(1), math.Inf(-1)
	for i := range xs {
		v := xs[i]*c + ys[i]*s
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	return hi - lo
}

// angleDiff is a-b wrapped into (-pi, pi].
func angleDiff(a, b float64) float64 {
	d := math.Mod(a-b, 2*math.Pi)
	if d > math.Pi {
		d -= 2 * math.Pi
	} else if d <= -math.Pi {
		d += 2 * math.Pi
	}
	return d
}

// normaliseAngle wraps an angle into (-pi, pi].
func normaliseAngle(a float64) float64 { return angleDiff(a, 0) }

func quantile(v []float64, q float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if len(s) == 1 {
		return s[0]
	}
	pos := q * float64(len(s)-1)
	i := int(math.Floor(pos))
	if i >= len(s)-1 {
		return s[len(s)-1]
	}
	return s[i] + (pos-float64(i))*(s[i+1]-s[i])
}

func sumTerms(ts []FitBoundTerm) float64 {
	var s float64
	for _, t := range ts {
		s += t.M
	}
	return s
}

func sortedUnique(v []int) []int {
	s := append([]int(nil), v...)
	sort.Ints(s)
	out := s[:0]
	for i, x := range s {
		if i == 0 || x != s[i-1] {
			out = append(out, x)
		}
	}
	return out
}

func fp64(v float64) *float64 { return &v }

// round3 keeps millimetres; round6 keeps microradians. A fitted record is
// stored, and digits below the sensor's resolution are noise in its digest.
func round3(v float64) float64 { return math.Round(v*1e3) / 1e3 }
func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }
