package annotation

import (
	"math"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"google.golang.org/protobuf/proto"
)

func straightSegment() Points {
	return Points{X: []float32{8, 9, 10}, Y: []float32{-0.9, -0.9, -0.9}, Z: []float32{1, 1, 1}}
}

func TestSegmentRelationLeavesTangentUnresolvedAndPropagatesBounds(t *testing.T) {
	p := straightSegment()
	centre := PlanarBound{XM: 10, BoundM: 0.2}
	yaw := AngleBound{Rad: math.Pi / 2, BoundRad: 0.05}
	line, bound, err := featureSegmentRelation(p, []uint32{0, 1, 2}, 0, 2, centre, yaw, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	angular := 0.05 + math.Asin(0.05)
	expected := 0.25 + (math.Hypot(1, float64(float32(-0.9)))+0.25)*2*math.Sin(angular/2)
	if math.Abs(line.NormalX-1) > 1e-9 || math.Abs(line.NormalY) > 1e-9 || math.Abs(line.OffsetM-float64(float32(-0.9))) > 1e-9 || math.Abs(bound-expected) > 1e-9 || math.Abs(line.NormalBoundRad-angular) > 1e-9 {
		t.Fatal(line, bound, expected)
	}
	reversed, _, err := featureSegmentRelation(p, []uint32{0, 1, 2}, 2, 0, centre, yaw, 0.05)
	if err != nil || math.Abs(reversed.OffsetM-line.OffsetM) > 1e-9 || math.Abs(reversed.NormalX-line.NormalX) > 1e-9 {
		t.Fatal("endpoint order changed line", err)
	}
	// Changing visible support along the same edge cannot create a tangent anchor.
	for i := range p.X {
		p.X[i] += 2
	}
	moved, _, err := featureSegmentRelation(p, []uint32{0, 1, 2}, 0, 2, centre, yaw, 0.05)
	if err != nil || math.Abs(moved.OffsetM-line.OffsetM) > 1e-9 {
		t.Fatal("tangent sampling moved line", err)
	}
}

func TestSegmentRelationRefusesDegenerateUnsupportedAndNonfiniteInputs(t *testing.T) {
	cases := []string{"same", "missing", "sparse", "duplicate", "outside", "nonfinite", "zero bound", "bad centre", "bad yaw", "short", "vertical", "curve", "large return bound", "wide yaw", "overflow"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			p := straightSegment()
			indices := []uint32{0, 1, 2}
			start, end := uint32(0), uint32(2)
			centre := PlanarBound{XM: 10, BoundM: 0.2}
			yaw := AngleBound{BoundRad: 0.05}
			r := 0.05
			switch name {
			case "same":
				end = start
			case "missing":
				indices = []uint32{1, 2}
			case "sparse":
				indices = []uint32{0, 2}
			case "duplicate":
				p.X[1] = p.X[0]
			case "outside":
				indices = append(indices, 99)
			case "nonfinite":
				p.Z[1] = float32(math.NaN())
			case "zero bound":
				r = 0
			case "bad centre":
				centre.BoundM = -1
			case "bad yaw":
				yaw.Rad = math.NaN()
			case "short":
				p.X = []float32{8, 8.02, 8.04}
			case "vertical":
				p.X = []float32{8, 8, 8}
				p.Z = []float32{1, 2, 3}
			case "curve":
				p.Y[1] = 0
			case "large return bound":
				r = 1
			case "wide yaw":
				yaw.BoundRad = math.Pi / 2
			case "overflow":
				centre.BoundM = math.MaxFloat64
			}
			if _, _, err := featureSegmentRelation(p, indices, start, end, centre, yaw, r); err == nil {
				t.Fatal("unsupported segment accepted")
			}
		})
	}
}

func registeredSegmentFixture(t *testing.T) (*Pack, *pb.FeatureEdit) {
	t.Helper()
	old := physPack(t)
	var blocks [][]byte
	for i := range old.Samples {
		pts, err := old.PointsAt(i)
		if err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 3; j++ {
			pts.X[j] = 7.75 + float32(j)*0.5 + float32(i)
			pts.Y[j] = -0.9
			pts.Z[j] = 0.3
		}
		b, err := EncodePoints(pts)
		if err != nil {
			t.Fatal(err)
		}
		blocks = append(blocks, b)
	}
	dir := filepath.Join(t.TempDir(), "segment")
	if err := WritePack(dir, old.Manifest, old.Samples, blocks); err != nil {
		t.Fatal(err)
	}
	p, err := OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, edit := featureFixtureForPack(t, p)
	_, edit = registeredFeatureFixtureForPack(t, p, edit)
	facet := edit.Document.Features[0]
	facet.Geometry = pb.FeatureGeometry_FEATURE_GEOMETRY_EDGE
	for _, o := range facet.Observations {
		o.PointIndices = []uint32{0, 1, 2}
		o.Sphere = &pb.FeatureSphere{XM: 8.25 + float64(o.SampleId), YM: float64(float32(-0.9)), ZM: float64(float32(0.3)), RadiusM: 1.1}
	}
	a := facet.Anchor
	a.Method = "manual_named_segment_v1"
	a.XM = 0
	a.YM = 0
	a.SourcePointIndex = nil
	pts, _ := p.PointsAt(0)
	a.Line, a.BoundM, err = featureSegmentRelation(pts, facet.Observations[0].PointIndices, 0, 2, PlanarBound{XM: 10, BoundM: 0.2}, AngleBound{BoundRad: 0.05}, a.ReturnBoundM)
	if err != nil {
		t.Fatal(err)
	}
	a.IdentityNote = "straight lower sill edge, physical identity checked in two frames"
	return p, edit
}

func TestSegmentProposalSavesReopensAndFreezesWithoutPointCoordinates(t *testing.T) {
	p, e := registeredSegmentFixture(t)
	state, err := SaveFeatures(p, e)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFeatures(p)
	if err != nil || !proto.Equal(loaded, state) {
		t.Fatal(err)
	}
	a := loaded.Document.Features[0].Anchor
	if a.SourcePointIndex != nil || a.XM != 0 || a.YM != 0 || a.Line.SourceStartIndex == nil || *a.Line.SourceStartIndex != 0 {
		t.Fatal("line became a point")
	}
	draft := physDraftPack(p, 0)
	head := 0
	draft.FeatureRevision = &head
	f := mustFreeze(t, freezeOptions(draft))
	_, s, err := f.Bind(p)
	if err != nil {
		t.Fatal(err)
	}
	if doc, err := f.BindFeatures(p, s); err != nil || doc.Features[0].Anchor.Line == nil {
		t.Fatal(err)
	}
}

func TestSegmentRegistrationRefusesChangedMeaningAndUnderstatedBounds(t *testing.T) {
	p, e := registeredSegmentFixture(t)
	cases := []struct {
		name   string
		change func(*pb.FeatureCandidate)
	}{
		{"geometry", func(f *pb.FeatureCandidate) { f.Geometry = pb.FeatureGeometry_FEATURE_GEOMETRY_PATCH }},
		{"missing line", func(f *pb.FeatureCandidate) { f.Anchor.Line = nil }},
		{"point with line", func(f *pb.FeatureCandidate) {
			f.Anchor.Method = "manual_named_return_v1"
			i := uint32(0)
			f.Anchor.SourcePointIndex = &i
		}},
		{"invented point", func(f *pb.FeatureCandidate) { f.Anchor.XM = 1 }},
		{"source point", func(f *pb.FeatureCandidate) { i := uint32(0); f.Anchor.SourcePointIndex = &i }},
		{"missing start", func(f *pb.FeatureCandidate) { f.Anchor.Line.SourceStartIndex = nil }},
		{"unknown line", func(f *pb.FeatureCandidate) { f.Anchor.Line.ProtoReflect().SetUnknown([]byte{0x78, 1}) }},
		{"normal", func(f *pb.FeatureCandidate) { f.Anchor.Line.NormalX = math.NaN() }},
		{"nonunit", func(f *pb.FeatureCandidate) { f.Anchor.Line.NormalY = 2 }},
		{"offset", func(f *pb.FeatureCandidate) { f.Anchor.Line.OffsetM += 1 }},
		{"angle", func(f *pb.FeatureCandidate) { f.Anchor.Line.NormalBoundRad = 0 }},
		{"offset bound", func(f *pb.FeatureCandidate) { f.Anchor.BoundM = 0 }},
		{"wide stated angle", func(f *pb.FeatureCandidate) { f.Anchor.Line.NormalBoundRad = math.Pi / 2 }},
		{"widened angle underbounds offset", func(f *pb.FeatureCandidate) { f.Anchor.Line.NormalBoundRad = 0.5 }},
		{"outside support", func(f *pb.FeatureCandidate) { i := uint32(7); f.Anchor.Line.SourceEndIndex = &i }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := proto.Clone(e.Document).(*pb.FeatureAnnotations)
			tc.change(d.Features[0])
			if err := ValidateFeatures(p, d); err == nil {
				t.Fatal("invalid line proposal accepted")
			}
		})
	}
	d := proto.Clone(e.Document).(*pb.FeatureAnnotations)
	d.Features[0].Anchor.Line.NormalBoundRad = 0.5
	d.Features[0].Anchor.BoundM = 2
	if err := ValidateFeatures(p, d); err != nil {
		t.Fatal("conservative widened bounds refused", err)
	}
	// All common source pins and assisted-origin rules still apply to lines.
	d.Features[0].Anchor.Origin = "tracker_seeded_proposal"
	if err := ValidateFeatures(p, d); err == nil || !strings.Contains(err.Error(), "origin") {
		t.Fatal("line laundered source origin", err)
	}
}
