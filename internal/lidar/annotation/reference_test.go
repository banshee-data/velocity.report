package annotation

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
)

// packOf writes a pack whose samples hold exactly the given horizontal points,
// at 100 ms spacing, so reference positions can be checked by hand.
func packOf(t *testing.T, samples ...[][2]float32) *Pack {
	t.Helper()
	var meta []Sample
	var blocks [][]byte
	for i, pts := range samples {
		p := Points{X: make([]float32, len(pts)), Y: make([]float32, len(pts)), Z: make([]float32, len(pts))}
		for j, xy := range pts {
			p.X[j], p.Y[j], p.Z[j] = xy[0], xy[1], 0.5
		}
		block, err := EncodePoints(p)
		if err != nil {
			t.Fatal(err)
		}
		meta = append(meta, Sample{
			SourceOrdinal: i, SourceFrameID: uint64(i), TimestampNs: int64(1_000_000_000 + i*100_000_000),
			SensorID: "synthetic", PointCount: len(pts),
		})
		blocks = append(blocks, block)
	}
	dir := filepath.Join(t.TempDir(), "pack")
	if err := WritePack(dir, Manifest{Coverage: CoverageForegroundOnly}, meta, blocks); err != nil {
		t.Fatal(err)
	}
	p, err := OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Position and footprint are declared rules. Three coincident returns and one
// outlier pull the mean well away from the centre of the extent, which is the
// difference the two rules exist to expose.
func TestReferencePositionRules(t *testing.T) {
	p := packOf(t, [][2]float32{{0, 0}, {0, 0}, {0, 0}, {4, 2}})
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("car", "car")}
	s.Masks = []FrameMask{mask("car", 0, 0, 1, 2, 3)}

	centre, _, err := BuildReference(p, s, DefaultReferencePolicy())
	if err != nil {
		t.Fatal(err)
	}
	c := centre[0]
	if c.X != 2 || c.Y != 1 || c.FootprintX != 4 || c.FootprintY != 2 || c.Returns != 4 {
		t.Fatalf("footprint centre %+v, want (2, 1) over a 4 × 2 m extent", c)
	}
	if math.Abs(c.FootprintDiagonal-math.Sqrt(20)) > 1e-12 {
		t.Fatalf("diagonal %v, want sqrt(20)", c.FootprintDiagonal)
	}
	if c.TimestampNs != 1_000_000_000 || c.Class != "car" || c.Ignore != "" {
		t.Fatalf("reference %+v lost its sample time, class or scored status", c)
	}

	policy := DefaultReferencePolicy()
	policy.Position = PositionPointMean
	mean, _, err := BuildReference(p, s, policy)
	if err != nil {
		t.Fatal(err)
	}
	if mean[0].X != 1 || mean[0].Y != 0.5 {
		t.Fatalf("point mean (%v, %v), want (1, 0.5)", mean[0].X, mean[0].Y)
	}
}

// Every mapping from the sidecar's vocabulary to scored, ignored or dropped.
func TestReferenceIgnoreMapping(t *testing.T) {
	sample := [][2]float32{{0, 0}, {1, 1}, {10, 0}, {11, 1}}
	withStatus := func(m FrameMask, st ReviewStatus) FrameMask { m.Status = st; return m }
	withVisibility := func(m FrameMask, v Visibility) FrameMask { m.Visibility = v; return m }
	withCompleteness := func(m FrameMask, c MaskCompleteness) FrameMask { m.Completeness = c; return m }
	proposed := func(o Object) Object { o.Status = StatusProposed; return o }
	rejected := func(o Object) Object { o.Status = StatusRejected; return o }
	uncertainOnly := FrameMask{
		ObjectID: "car", SampleID: 0, UncertainIndices: []int{2, 3},
		Completeness: MaskComplete, Visibility: VisiblePresent, Status: StatusReviewed,
	}

	cases := []struct {
		name    string
		object  Object
		mask    FrameMask
		policy  func(*ReferencePolicy)
		want    IgnoreReason
		dropped bool
	}{
		{name: "reviewed car, reviewed complete mask", object: reviewedObject("car", "car"), mask: mask("car", 0, 0, 1)},
		{name: "class compared case-insensitively", object: reviewedObject("car", " Car "), mask: mask("car", 0, 0, 1)},
		{name: "partial mask scored by default", object: reviewedObject("car", "car"),
			mask: withCompleteness(mask("car", 0, 0, 1), MaskPartial)},
		{name: "partial mask ignored when the policy says so", object: reviewedObject("car", "car"),
			mask:   withCompleteness(mask("car", 0, 0, 1), MaskPartial),
			policy: func(p *ReferencePolicy) { p.ScorePartialMasks = false }, want: IgnoreIncompleteMask},
		{name: "completeness never stated", object: reviewedObject("car", "car"),
			mask: withCompleteness(mask("car", 0, 0, 1), MaskUnreviewed), want: IgnoreIncompleteMask},
		{name: "proposed mask of a reviewed object", object: reviewedObject("car", "car"),
			mask: withStatus(mask("car", 0, 0, 1), StatusProposed), want: IgnoreUnreviewed},
		{name: "reviewed mask of a proposed object", object: proposed(reviewedObject("car", "car")),
			mask: mask("car", 0, 0, 1), want: IgnoreUnreviewed},
		{name: "proposed, scored when proposals are included", object: proposed(reviewedObject("car", "car")),
			mask:   withStatus(mask("car", 0, 0, 1), StatusProposed),
			policy: func(p *ReferencePolicy) { p.Status = ReferenceIncludeProposed }},
		{name: "noise is not a road user", object: reviewedObject("car", "noise"), mask: mask("car", 0, 0, 1), want: IgnoreNotRoadUser},
		{name: "ground is not a road user", object: reviewedObject("car", "ground"), mask: mask("car", 0, 0, 1), want: IgnoreNotRoadUser},
		{name: "partly occluded is scored", object: reviewedObject("car", "car"),
			mask: withVisibility(mask("car", 0, 0, 1), VisiblePartlyOccluded)},
		{name: "fully occluded", object: reviewedObject("car", "car"),
			mask: withVisibility(mask("car", 0, 0, 1), VisibleFullyOccluded), want: IgnoreVisibility},
		{name: "outside view", object: reviewedObject("car", "car"),
			mask: withVisibility(mask("car", 0, 0, 1), VisibleOutsideView), want: IgnoreVisibility},
		{name: "visibility unknown", object: reviewedObject("car", "car"),
			mask: withVisibility(mask("car", 0, 0, 1), VisibleUnknown), want: IgnoreVisibility},
		{name: "only uncertain returns", object: reviewedObject("car", "car"), mask: uncertainOnly, want: IgnoreUncertainMembership},
		{name: "rejected mask is dropped", object: reviewedObject("car", "car"),
			mask: withStatus(mask("car", 0, 0, 1), StatusRejected), dropped: true},
		{name: "rejected object is dropped", object: rejected(reviewedObject("car", "car")),
			mask: mask("car", 0, 0, 1), dropped: true},
		{name: "empty mask is dropped", object: reviewedObject("car", "car"), mask: mask("car", 0), dropped: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := packOf(t, sample)
			s := NewSidecar(p)
			s.Objects = []Object{tc.object}
			s.Masks = []FrameMask{tc.mask}
			policy := DefaultReferencePolicy()
			if tc.policy != nil {
				tc.policy(&policy)
			}
			refs, summary, err := BuildReference(p, s, policy)
			if err != nil {
				t.Fatal(err)
			}
			if summary.Masks != 1 {
				t.Fatalf("summary counted %d masks, want 1", summary.Masks)
			}
			if tc.dropped {
				if len(refs) != 0 || summary.DroppedRejected+summary.DroppedEmpty != 1 {
					t.Fatalf("got %+v with summary %+v; want the mask dropped", refs, summary)
				}
				return
			}
			if len(refs) != 1 {
				t.Fatalf("got %d reference points, want 1", len(refs))
			}
			if refs[0].Ignore != tc.want {
				t.Fatalf("ignore = %q, want %q", refs[0].Ignore, tc.want)
			}
			if tc.want == "" && summary.Scored != 1 || tc.want != "" && summary.Ignored[tc.want] != 1 {
				t.Fatalf("summary %+v does not agree with the point", summary)
			}
		})
	}

	// An uncertain-only mask still has a position, so it can absorb a
	// hypothesis: the centre of the uncertain returns.
	p := packOf(t, sample)
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("car", "car")}
	s.Masks = []FrameMask{uncertainOnly}
	refs, _, err := BuildReference(p, s, DefaultReferencePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if refs[0].X != 10.5 || refs[0].Y != 0.5 || refs[0].Returns != 2 {
		t.Fatalf("uncertain-only position %+v, want (10.5, 0.5) from 2 returns", refs[0])
	}
}

func TestReferenceOrderAndRefusals(t *testing.T) {
	p := packOf(t, [][2]float32{{0, 0}, {5, 5}}, [][2]float32{{1, 0}, {6, 5}})
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("b", "car"), reviewedObject("a", "pedestrian")}
	s.Masks = []FrameMask{mask("b", 1, 1), mask("a", 1, 0), mask("b", 0, 1), mask("a", 0, 0)}
	refs, summary, err := BuildReference(p, s, DefaultReferencePolicy())
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, r := range refs {
		order = append(order, r.ObjectID+"@"+string(rune('0'+r.SampleID)))
	}
	if got := strings.Join(order, ","); got != "a@0,b@0,a@1,b@1" {
		t.Fatalf("order %s, want sample then object", got)
	}
	if summary.Scored != 4 {
		t.Fatalf("summary %+v, want 4 scored", summary)
	}

	// A mask citing a return the pack does not have is refused, not guessed.
	s.Masks[0] = mask("b", 1, 9)
	if _, _, err := BuildReference(p, s, DefaultReferencePolicy()); err == nil || !strings.Contains(err.Error(), "outside sample 1") {
		t.Fatalf("an out-of-range mask gave %v, want a refusal naming the sample", err)
	}

	for _, bad := range []ReferencePolicy{
		{Status: "everything", Position: PositionFootprintCentre, RoadUserClasses: RoadUserClasses()},
		{Status: ReferenceReviewedOnly, Position: "pose", RoadUserClasses: RoadUserClasses()},
		{Status: ReferenceReviewedOnly, Position: PositionFootprintCentre},
	} {
		if _, _, err := BuildReference(p, NewSidecar(p), bad); err == nil {
			t.Errorf("policy %+v was accepted", bad)
		}
	}
}
