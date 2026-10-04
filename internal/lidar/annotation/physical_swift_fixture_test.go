package annotation

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/testutil"
)

// The macOS client draws a keyframe's centre, bumpers and box from its own
// port of Geometry, so an operator authoring against the overlay sees what
// the scorer will use. This fixture holds the two to each other: the objects
// and what Go derives from each keyframe are written to testdata, and the
// Swift tests (PhysicalReferenceGeometryTests.swift) read the same file and
// derive the same values. A change to Geometry fails here first; rerun with
// -update-physical-swift-fixture and the Swift tests say whether the port
// still agrees.

var updatePhysicalSwiftFixture = flag.Bool("update-physical-swift-fixture", false,
	"rewrite testdata/physical_geometry_fixture.json from the current Geometry")

const physicalSwiftFixture = "testdata/physical_geometry_fixture.json"

type physicalGeometryFixture struct {
	Objects    []PhysicalObject            `json:"objects"`
	Geometries map[string]PhysicalGeometry `json:"geometries"`
}

// physicalFixtureObjects covers every derivation path: a centre anchor, a
// face anchor with and without an offset, an ambiguous and an unknown axis, a
// partial and a prior-only dimension, a yaw bound past half a turn, a yaw
// across the ±π seam, and bodies that are unreviewed or tracker-assisted.
func physicalFixtureObjects() []PhysicalObject {
	objects := validPhysical(&Pack{}).Objects
	review := independentReview()
	unreviewed := independentReview()
	unreviewed.Status = StatusProposed
	body := func(id string, r PhysicalReview) *BodyGeometry {
		return &BodyGeometry{
			BodyID: id, AxisConvention: BodyAxisConvention,
			Length: DimensionBound{Status: EvidenceObserved, Span: SpanFull, LowerM: fp(4.1), UpperM: fp(4.9), ValueM: fp(4.6), Support: frames(0)},
			Width:  DimensionBound{Status: EvidenceInferred, Span: SpanFull, LowerM: fp(1.7), UpperM: fp(2.0), Support: EvidenceSupport{External: "spec sheet"}},
			Height: DimensionBound{Status: EvidencePriorOnly, Span: SpanFull, LowerM: fp(1.3), UpperM: fp(1.6), Support: EvidenceSupport{External: "class prior"}},
			Review: r,
		}
	}
	kf := func(id string, sample int, anchor PhysicalAnchor, x, y, bound float64, axis AxisState, yaw, yawBound float64, front, rear EvidenceStatus) PhysicalKeyframe {
		k := PhysicalKeyframe{
			KeyframeID: id, SampleID: sample, TimestampNs: physTime(sample), Anchor: anchor,
			Position: PositionBound{Status: EvidenceObserved, XM: fp(x), YM: fp(y), BoundM: fp(bound), Support: frames(sample)},
			Yaw:      YawBound{Status: EvidenceObserved, Axis: axis, YawRad: fp(yaw), BoundRad: fp(yawBound), Support: frames(sample)},
			Front:    EndpointEvidence{Status: front}, Rear: EndpointEvidence{Status: rear}, Review: review,
		}
		if axis == AxisUnknown {
			k.Yaw = YawBound{Status: EvidenceUnknown, Axis: AxisUnknown}
		}
		for _, e := range []*EndpointEvidence{&k.Front, &k.Rear} {
			if e.Status != EvidenceUnknown {
				e.Support = frames(sample)
			}
		}
		return k
	}
	objects = append(objects,
		PhysicalObject{ObjectID: "car-3", Body: body("body-car-3", review), Keyframes: []PhysicalKeyframe{
			kf("kf-left-offset", 0, PhysicalAnchor{Kind: AnchorLeftFace, OffsetM: fp(0.9), OffsetBoundM: fp(0.1)}, 3, 4, 0.2, AxisResolved, 0.7, 0.05, EvidenceObserved, EvidenceInferred),
			kf("kf-right-offset", 1, PhysicalAnchor{Kind: AnchorRightFace, OffsetM: fp(0.9), OffsetBoundM: fp(0.1)}, 3, 4, 0.2, AxisResolved, -2.9, 0.2, EvidenceObserved, EvidenceObserved),
			kf("kf-front-no-offset", 2, PhysicalAnchor{Kind: AnchorFrontFace}, 5, -1, 0.3, AxisResolved, 3.1, 0.1, EvidenceObserved, EvidenceObserved),
			kf("kf-front-offset-seam", 3, PhysicalAnchor{Kind: AnchorFrontFace, OffsetM: fp(2.3), OffsetBoundM: fp(0.2)}, -5, 2, 0.1, AxisResolved, -3.1, 0.1, EvidenceObserved, EvidencePriorOnly),
			kf("kf-ambiguous", 4, PhysicalAnchor{Kind: AnchorBodyCentre}, 1, 1, 0.25, AxisFrontRearAmbiguous, 1.2, 0.3, EvidenceUnknown, EvidenceUnknown),
			kf("kf-axis-unknown", 5, PhysicalAnchor{Kind: AnchorBodyCentre}, 2, 2, 0.4, AxisUnknown, 0, 0, EvidenceUnknown, EvidenceUnknown),
		}},
		PhysicalObject{ObjectID: "car-4", Body: body("body-car-4", unreviewed), Keyframes: []PhysicalKeyframe{
			kf("kf-unreviewed-body", 0, PhysicalAnchor{Kind: AnchorBodyCentre}, 7, 7, 0.2, AxisResolved, 0.5, 4, EvidenceObserved, EvidenceObserved),
		}},
		PhysicalObject{ObjectID: "car-5", Keyframes: []PhysicalKeyframe{
			kf("kf-no-body", 0, PhysicalAnchor{Kind: AnchorRearFace, OffsetM: fp(2), OffsetBoundM: fp(0.1)}, 9, 9, 0.2, AxisResolved, 1, 0.1, EvidenceUnknown, EvidenceObserved),
		}},
	)
	assisted := *body("body-car-6", assistedReview())
	objects = append(objects, PhysicalObject{ObjectID: "car-6", Body: &assisted, Keyframes: []PhysicalKeyframe{
		kf("kf-assisted-body", 0, PhysicalAnchor{Kind: AnchorBodyCentre}, 0, 0, 0.2, AxisResolved, 0, 0.1, EvidenceObserved, EvidenceObserved),
	}})
	return objects
}

func TestPhysicalGeometrySwiftFixture(t *testing.T) {
	objects := physicalFixtureObjects()
	fixture := physicalGeometryFixture{Objects: objects, Geometries: map[string]PhysicalGeometry{}}
	for _, o := range objects {
		for _, k := range o.Keyframes {
			fixture.Geometries[k.KeyframeID] = o.Geometry(k)
		}
	}
	want, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if *updatePhysicalSwiftFixture {
		if err := os.MkdirAll(filepath.Dir(physicalSwiftFixture), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(physicalSwiftFixture, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(physicalSwiftFixture)
	if err != nil {
		t.Fatalf("read %s (run with -update-physical-swift-fixture to create it): %v", physicalSwiftFixture, err)
	}
	// Floats may differ in their last digit between arm64 and amd64 (Go
	// fuses multiply-adds on one and not the other), so a byte mismatch is
	// stale only when a value differs beyond rounding.
	if !bytes.Equal(got, want) {
		if err := testutil.JSONWithin(got, want, 1e-9); err != nil {
			t.Fatalf("%s is stale (%v): rerun with -update-physical-swift-fixture and check the Swift geometry tests", physicalSwiftFixture, err)
		}
	}
}
