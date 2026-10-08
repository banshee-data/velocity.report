package annotation

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
)

// fitBox is a box in plan view: centre, length along yaw, width across it.
type fitBox struct{ cx, cy, l, w, yaw float64 }

func (b fitBox) corners() [4][2]float64 {
	ux, uy := math.Cos(b.yaw), math.Sin(b.yaw)
	vx, vy := -uy, ux
	var out [4][2]float64
	for i, s := range [4][2]float64{{1, 1}, {-1, 1}, {-1, -1}, {1, -1}} {
		a, c := s[0]*b.l/2, s[1]*b.w/2
		out[i] = [2]float64{b.cx + a*ux + c*vx, b.cy + a*uy + c*vy}
	}
	return out
}

// rayHit is how far along a ray from the origin in direction (dx, dy) it meets
// the segment from a to b.
func rayHit(dx, dy float64, a, b [2]float64) (float64, bool) {
	ex, ey := b[0]-a[0], b[1]-a[1]
	den := dx*ey - dy*ex
	if math.Abs(den) < 1e-12 {
		return 0, false
	}
	t := (a[0]*ey - a[1]*ex) / den
	u := (a[0]*dy - a[1]*dx) / den
	if t <= 0 || u < 0 || u > 1 {
		return 0, false
	}
	return t, true
}

// castScene samples the boxes the way the sensor does in plan view: one ray
// every 0.2 degrees, each returning from the nearest face it meets, at three
// heights. owner[i] is the box a return came from.
func castScene(boxes []fitBox) (Points, []int) {
	var pts Points
	var owner []int
	step := 0.2 * math.Pi / 180
	for k := 0; k < 1800; k++ {
		phi := -math.Pi + (float64(k)+0.5)*step
		dx, dy := math.Cos(phi), math.Sin(phi)
		best, who := math.Inf(1), -1
		for i, b := range boxes {
			c := b.corners()
			for e := 0; e < 4; e++ {
				if t, ok := rayHit(dx, dy, c[e], c[(e+1)%4]); ok && t < best {
					best, who = t, i
				}
			}
		}
		if who < 0 {
			continue
		}
		for _, z := range []float32{-2.0, -1.4, -0.9} {
			pts.X = append(pts.X, float32(best*dx))
			pts.Y = append(pts.Y, float32(best*dy))
			pts.Z = append(pts.Z, z)
			owner = append(owner, who)
		}
	}
	return pts, owner
}

// fitScenePack writes one sample per scene, 100 ms apart, and reviews box 0
// of each as object "car" and box 1, when there is one, as "post".
func fitScenePack(t *testing.T, scenes [][]fitBox, transform string) (*Pack, *Sidecar) {
	t.Helper()
	var samples []Sample
	var blocks [][]byte
	var owners [][]int
	for i, sc := range scenes {
		pts, owner := castScene(sc)
		block, err := EncodePoints(pts)
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, Sample{SourceOrdinal: i, SourceFrameID: uint64(100 + i),
			TimestampNs: 1_000_000_000 + int64(i)*100_000_000, SensorID: "hesai-test", PointCount: len(pts.X)})
		blocks = append(blocks, block)
		owners = append(owners, owner)
	}
	m := Manifest{
		Coverage: CoverageForegroundOnly,
		Source:   SourceProvenance{SensorID: "hesai-test", VRLOGHeaderSHA: "sha256:header", VRLOGFramesSHA: "sha256:frames"},
		Coordinate: CoordinateContract{Units: "metres", FrameID: "sensor", ReferenceFrame: "sensor",
			Handedness: "right", OriginNote: "sensor origin", TransformVersion: transform},
	}
	dir := filepath.Join(t.TempDir(), "pack")
	if err := WritePack(dir, m, samples, blocks); err != nil {
		t.Fatal(err)
	}
	p, err := OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSidecar(p)
	s.Change = Provenance{Author: "op", Operation: "label"}
	s.Objects = []Object{reviewedObject("car", "car"), reviewedObject("post", "noise")}
	for i, owner := range owners {
		var car, post []int
		for j, o := range owner {
			if o == 0 {
				car = append(car, j)
			} else {
				post = append(post, j)
			}
		}
		if len(car) > 0 {
			s.Masks = append(s.Masks, mask("car", i, car...))
		}
		if len(post) > 0 {
			s.Masks = append(s.Masks, mask("post", i, post...))
		}
	}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	return p, loaded
}

// passingCar is a 4.5 by 1.8 m car driving along +x at 10 m/s, 6 m to the
// sensor's left, from x = -15 to +15.
func passingCar(extra ...fitBox) [][]fitBox {
	var scenes [][]fitBox
	for i := 0; i <= 30; i++ {
		scenes = append(scenes, append([]fitBox{{cx: -15 + float64(i), cy: 6, l: 4.5, w: 1.8}}, extra...))
	}
	return scenes
}

func fitCar(t *testing.T, p *Pack, s *Sidecar, samples ...int) *FitResult {
	t.Helper()
	res, err := FitPhysicalObject(p, s, FitRequest{ObjectID: "car", Samples: samples, Author: "op"}, DefaultFitOptions())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// The fit recovers a box it can see from every side within the bounds it
// states, and the import it writes passes every check an import makes.
func TestFitRecoversAPassingCarWithinItsBounds(t *testing.T) {
	p, s := fitScenePack(t, passingCar(), "")
	res := fitCar(t, p, s, 15)
	if _, err := PreparePhysicalImport(p, res.Import, false); err != nil {
		t.Fatalf("the fitted import is refused: %v", err)
	}
	o := res.Import.Objects[0]
	for _, d := range []struct {
		name  string
		d     DimensionBound
		truth float64
	}{{"length", o.Body.Length, 4.5}, {"width", o.Body.Width, 1.8}} {
		if d.d.Span != SpanFull {
			t.Fatalf("%s: %s span, want full; notes %v", d.name, d.d.Span, res.Notes)
		}
		if d.truth < *d.d.LowerM || d.truth > *d.d.UpperM {
			t.Errorf("%s: true %.2f m outside the fitted [%.3f, %.3f]", d.name, d.truth, *d.d.LowerM, *d.d.UpperM)
		}
		if v, hw, _ := d.d.Best(); hw > 0.5 || math.Abs(v-d.truth) > 0.25 {
			t.Errorf("%s: fitted %.3f ± %.3f m against %.2f: looser than a clean scene should give", d.name, v, hw, d.truth)
		}
	}
	if o.Body.Height.Span != SpanPartial || o.Body.Height.UpperM != nil {
		t.Errorf("height: want a lower bound only, got %+v", o.Body.Height)
	}
	var mid *PhysicalKeyframe
	for i := range o.Keyframes {
		if o.Keyframes[i].SampleID == 15 {
			mid = &o.Keyframes[i]
		}
	}
	if mid == nil {
		t.Fatalf("no pose at the requested sample 15; notes %v", res.Notes)
	}
	if mid.Yaw.Axis != AxisResolved || math.Abs(angleDiff(*mid.Yaw.YawRad, 0)) > *mid.Yaw.BoundRad+1e-9 {
		t.Errorf("heading %.4f ± %.4f rad, axis %s: want 0 within the bound, front resolved", *mid.Yaw.YawRad, *mid.Yaw.BoundRad, mid.Yaw.Axis)
	}
	// At sample 15 the car is broadside at x = 0: both ends seen, so the
	// anchor is the body centre, which must lie within its bound of (0, 6).
	if mid.Anchor.Kind != AnchorBodyCentre {
		t.Fatalf("anchor %s at a broadside frame, want body_centre", mid.Anchor.Kind)
	}
	if d := math.Hypot(*mid.Position.XM-0, *mid.Position.YM-6); d > *mid.Position.BoundM {
		t.Errorf("centre (%.3f, %.3f) is %.3f m from the truth, beyond its bound %.3f", *mid.Position.XM, *mid.Position.YM, d, *mid.Position.BoundM)
	}
	if mid.Front.Status != EvidenceObserved || mid.Rear.Status != EvidenceObserved {
		t.Errorf("ends %s/%s at a broadside frame, want both observed", mid.Front.Status, mid.Rear.Status)
	}
}

// A fitted record is an ordinary independent proposal: never reviewed by the
// fit, never carrying an algorithm (which an independent record may not), and
// always stating its assumptions, so the pane's free-text field is filled.
func TestFitWritesIndependentProposals(t *testing.T) {
	p, s := fitScenePack(t, passingCar(), "")
	o := fitCar(t, p, s).Import.Objects[0]
	reviews := []PhysicalReview{o.Body.Review}
	for _, k := range o.Keyframes {
		reviews = append(reviews, k.Review)
	}
	for _, r := range reviews {
		if r.Status != StatusProposed || r.Origin != OriginIndependent || r.Method != FitMethod {
			t.Errorf("review %+v: want a proposed, independent %s record", r, FitMethod)
		}
		if r.Provenance.Algorithm != "" || r.TrackerSource != "" {
			t.Errorf("review names an algorithm or tracker source: %+v", r)
		}
		if !strings.Contains(r.UncertaintyAssumptions, "envelope") {
			t.Errorf("assumptions do not state the box convention: %q", r.UncertaintyAssumptions)
		}
	}
}

// A post standing just beyond the car's front, nearer the sensor, makes that
// end the edge of an occluder rather than the car's end: it is reported cut,
// and not seen.
func TestFitReportsAnEndCutByANearerReturn(t *testing.T) {
	// At sample 10 the car is at x = -5, its front face between 112 and 118
	// degrees of azimuth. A 0.8 m post at (-1.2, 3) covers about 105 to 119
	// degrees from 3.2 m, so the car's returns stop at the post's edge.
	p, s := fitScenePack(t, passingCar(fitBox{cx: -1.2, cy: 3, l: 0.8, w: 0.8}), "")
	res := fitCar(t, p, s)
	for _, f := range res.Frames {
		if f.SampleID != 10 {
			continue
		}
		if f.Front.Seen || !strings.HasPrefix(f.Front.Reason, "cut") {
			t.Fatalf("front at sample 10: seen %v, %q; want cut", f.Front.Seen, f.Front.Reason)
		}
		return
	}
	t.Fatal("no diagnostics for sample 10")
}

// An object that does not move has no course: its axis is the box's longer
// side with front and rear unknown, so no end is named.
func TestFitLeavesAStationaryObjectsFrontUnknown(t *testing.T) {
	var scenes [][]fitBox
	for i := 0; i < 10; i++ {
		scenes = append(scenes, []fitBox{{cx: 3, cy: 8, l: 4.5, w: 1.8, yaw: 0.3}})
	}
	p, s := fitScenePack(t, scenes, "")
	res := fitCar(t, p, s, 5)
	if _, err := PreparePhysicalImport(p, res.Import, false); err != nil {
		t.Fatalf("the fitted import is refused: %v", err)
	}
	for _, k := range res.Import.Objects[0].Keyframes {
		if k.Yaw.Axis != AxisFrontRearAmbiguous || k.Anchor.Kind != AnchorBodyCentre ||
			k.Front.Status != EvidenceUnknown || k.Rear.Status != EvidenceUnknown {
			t.Errorf("sample %d: axis %s, anchor %s, ends %s/%s; want an ambiguous axis, the body centre and no named end",
				k.SampleID, k.Yaw.Axis, k.Anchor.Kind, k.Front.Status, k.Rear.Status)
		}
	}
}

// The fit refuses what it cannot rest on: proposed membership, an unreviewed
// object, a frame with no returns, and a pack whose sensor position is unknown.
func TestFitRefusesWhatItCannotRestOn(t *testing.T) {
	p, s := fitScenePack(t, passingCar(), "")
	proposed := *s
	proposed.Masks = append([]FrameMask(nil), s.Masks...)
	proposed.Masks[3].Status = StatusProposed
	if _, err := FitPhysicalObject(p, &proposed, FitRequest{ObjectID: "car", Author: "op"}, DefaultFitOptions()); err == nil ||
		!strings.Contains(err.Error(), "still proposed") {
		t.Errorf("a proposed mask: got %v", err)
	}
	unreviewed := *s
	unreviewed.Objects = []Object{{ObjectID: "car", Class: "car", Status: StatusProposed}}
	if _, err := FitPhysicalObject(p, &unreviewed, FitRequest{ObjectID: "car", Author: "op"}, DefaultFitOptions()); err == nil {
		t.Error("an unreviewed object was fitted")
	}
	if _, err := FitPhysicalObject(p, s, FitRequest{ObjectID: "car", Samples: []int{99}, Author: "op"}, DefaultFitOptions()); err == nil {
		t.Error("a sample with no returns was accepted")
	}
	if _, err := FitPhysicalObject(p, s, FitRequest{ObjectID: "car"}, DefaultFitOptions()); err == nil {
		t.Error("a fit with no author was accepted")
	}
	tp, ts := fitScenePack(t, passingCar(), "site-v1")
	if _, err := FitPhysicalObject(tp, ts, FitRequest{ObjectID: "car", Author: "op"}, DefaultFitOptions()); err == nil ||
		!strings.Contains(err.Error(), "site transform") {
		t.Errorf("a transformed pack: got %v", err)
	}
}

// A few returns far ahead of the body are reported, so a mask holding
// something else is found.
func TestFitReportsReturnsDetachedFromTheBody(t *testing.T) {
	p, s := fitScenePack(t, passingCar(fitBox{cx: 30, cy: 6.5, l: 0.2, w: 0.2}), "")
	// Fold the far post into the car's mask at every sample.
	merged := *s
	merged.Masks = nil
	var post = map[int][]int{}
	for _, m := range s.Masks {
		if m.ObjectID == "post" {
			post[m.SampleID] = m.PointIndices
		}
	}
	for _, m := range s.Masks {
		if m.ObjectID == "car" {
			m.PointIndices = CanonicalIndices(append(append([]int(nil), m.PointIndices...), post[m.SampleID]...))
			merged.Masks = append(merged.Masks, m)
		}
	}
	res, err := FitPhysicalObject(p, &merged, FitRequest{ObjectID: "car", Author: "op"}, DefaultFitOptions())
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range res.Notes {
		if strings.Contains(n, "front:") && strings.Contains(n, "apart from the body") {
			return
		}
	}
	t.Errorf("no detached-returns note; notes %v", res.Notes)
}

// With fewer frames than it takes to state a full span, a dimension is a lower
// bound, and the import still passes every check.
func TestFitStatesALowerBoundFromTooFewFrames(t *testing.T) {
	// Three broadside frames see both ends; asking for five makes the length
	// a lower bound. No roof returns, so the width is one too, and the pose
	// falls back to the side it sees.
	scenes := passingCar()[14:17]
	p, s := fitScenePack(t, scenes, "")
	opts := DefaultFitOptions()
	opts.MinSizeFrames = 5
	opts.MinCourseM = 1 // the three frames span 2 m, at the default's edge
	res, err := FitPhysicalObject(p, s, FitRequest{ObjectID: "car", Author: "op"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	l := res.Import.Objects[0].Body.Length
	if l.Span != SpanPartial || l.UpperM != nil || *l.LowerM > 4.5+ObservedSpanSlackM {
		t.Fatalf("length %+v: want a lower bound no longer than the car; notes %v", l, res.Notes)
	}
	if _, err := PreparePhysicalImport(p, res.Import, false); err != nil {
		t.Fatalf("the fitted import is refused: %v", err)
	}
	for _, k := range res.Import.Objects[0].Keyframes {
		if k.Anchor.Kind != AnchorRightFace || k.Anchor.OffsetM != nil || k.Front.Status == EvidenceObserved {
			t.Errorf("sample %d: anchor %+v, front %s; want the right face, no offset, no end claimed observed",
				k.SampleID, k.Anchor, k.Front.Status)
		}
	}
}

// A dimension keeps only the frames that got a pose; with none, a full span is
// stated as inferred and a lower bound is withdrawn, since only an observation
// can state one.
func TestFitTrimsSupportToPosedFrames(t *testing.T) {
	full := DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp64(4), UpperM: fp64(5),
		Support: EvidenceSupport{Frames: []int{1, 2, 3}}}
	if note := trimSupport(&full, map[int]bool{2: true}); note != "" || len(full.Support.Frames) != 1 || full.Support.Frames[0] != 2 {
		t.Errorf("kept %v (%q), want [2]", full.Support.Frames, note)
	}
	if note := trimSupport(&full, map[int]bool{}); note == "" || full.Status != EvidenceInferred {
		t.Errorf("a full span with no posed frame: %+v (%q), want inferred", full, note)
	}
	partial := DimensionBound{Status: EvidenceObserved, Span: SpanPartial, LowerM: fp64(4), Support: EvidenceSupport{Frames: []int{1}}}
	if note := trimSupport(&partial, map[int]bool{}); note == "" || partial.Status != EvidenceUnknown || partial.LowerM != nil {
		t.Errorf("a lower bound with no posed frame: %+v (%q), want unknown", partial, note)
	}
}
