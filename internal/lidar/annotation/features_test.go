package annotation

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"google.golang.org/protobuf/proto"
)

func featureFixture(t *testing.T) (*Pack, *pb.FeatureEdit) {
	t.Helper()
	return featureFixtureForPack(t, physPack(t))
}

func featureFixtureForPack(t *testing.T, p *Pack) (*Pack, *pb.FeatureEdit) {
	t.Helper()
	s := physSidecar(t, p)
	state, err := LoadFeatures(p)
	if err != nil {
		t.Fatal(err)
	}
	o := &pb.FeatureObservation{SampleId: 0, TimestampNs: physTime(0), SourceOrdinal: 0,
		PointIndices: []uint32{0, 1}, Sphere: &pb.FeatureSphere{XM: 7.75, YM: -0.9, ZM: 0.9, RadiusM: 0.7},
		Decision: pb.FeatureDecision_FEATURE_DECISION_ACCEPTED_PROPOSAL, Method: "manual_sphere", Origin: "human_proposal", Author: "operator",
		MembershipRevision: uint64(s.Revision), MembershipDigest: s.Digest()}
	state.Document.Author = "operator"
	state.Document.Features = []*pb.FeatureCandidate{{FeatureId: "mirror", ObjectId: "car-1", Name: "Mirror?",
		Geometry: pb.FeatureGeometry_FEATURE_GEOMETRY_PROTRUSION, PartId: "body", PartRelation: "unknown", Observations: []*pb.FeatureObservation{o}}}
	return p, &pb.FeatureEdit{Document: state.Document, MembershipDigest: state.MembershipDigest}
}

func TestFeatureRoundTripHistoryAndMaskIsolation(t *testing.T) {
	p, edit := featureFixture(t)
	maskBefore, _ := os.ReadFile(filepath.Join(p.Dir, "annotations.json"))
	state, err := SaveFeatures(p, edit)
	if err != nil {
		t.Fatal(err)
	}
	if state.Document.Revision != 1 || state.Digest == "" {
		t.Fatal(state)
	}
	first, _ := os.ReadFile(filepath.Join(p.Dir, featureFile))
	loaded, err := LoadFeatures(p)
	if err != nil || !proto.Equal(state, loaded) {
		t.Fatalf("load: %v %+v", err, loaded)
	}
	// Overlapping feature support is allowed, unlike whole-object membership.
	next := proto.Clone(state.Document).(*pb.FeatureAnnotations)
	second := proto.Clone(next.Features[0]).(*pb.FeatureCandidate)
	second.FeatureId = "edge"
	next.Features = append(next.Features, second)
	newState, err := SaveFeatures(p, &pb.FeatureEdit{Document: next, BaseDigest: state.Digest, MembershipDigest: state.MembershipDigest})
	if err != nil || newState.Document.Revision != 2 {
		t.Fatalf("overlap: %v", err)
	}
	archived, _ := os.ReadFile(filepath.Join(p.Dir, featureRevisionName(1)))
	if !bytes.Equal(first, archived) {
		t.Fatal("history changed")
	}
	retained, err := LoadFeatureRevision(p, 1)
	if err != nil || retained.Digest != state.Digest || !proto.Equal(retained.Document, state.Document) {
		t.Fatalf("retained evidence changed: %v %+v", err, retained)
	}
	head, err := LoadFeatureRevision(p, 2)
	if err != nil || !proto.Equal(head, newState) {
		t.Fatalf("exact head revision: %v", err)
	}
	// A corrupt later head must not erase a valid retained revision.
	latest, _ := os.ReadFile(filepath.Join(p.Dir, featureFile))
	os.WriteFile(filepath.Join(p.Dir, featureFile), []byte{0xff}, 0600)
	if r, err := LoadFeatureRevision(p, 1); err != nil || r.Digest != state.Digest {
		t.Fatalf("later head damage erased history: %v", err)
	}
	os.WriteFile(filepath.Join(p.Dir, featureFile), latest, 0600)
	maskAfter, _ := os.ReadFile(filepath.Join(p.Dir, "annotations.json"))
	if !bytes.Equal(maskBefore, maskAfter) {
		t.Fatal("feature save changed membership")
	}
	// Current membership changes do not erase the interpretation of revision 1.
	s, _ := LoadSidecar(p)
	s.Masks[0].PointIndices = []int{2, 3}
	if err = SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadFeatures(p); err != nil {
		t.Fatalf("historical evidence: %v", err)
	}
	if historical, err := LoadFeatureRevision(p, 1); err != nil || historical.Digest != retained.Digest {
		t.Fatalf("membership edit erased retained revision: %v", err)
	}
	if _, err = SaveFeatures(p, &pb.FeatureEdit{Document: newState.Document, BaseDigest: newState.Digest, MembershipDigest: state.MembershipDigest}); !errors.Is(err, ErrMembershipChanged) {
		t.Fatal(err)
	}
}

func TestLoadFeatureRevisionRefusals(t *testing.T) {
	for _, name := range []string{"zero", "overflow", "root", "membership", "missing", "corrupt", "identity", "schema", "symlink"} {
		t.Run(name, func(t *testing.T) {
			p, edit := featureFixture(t)
			s, err := SaveFeatures(p, edit)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = SaveFeatures(p, &pb.FeatureEdit{Document: s.Document, BaseDigest: s.Digest, MembershipDigest: s.MembershipDigest}); err != nil {
				t.Fatal(err)
			}
			revision := uint64(1)
			archive := filepath.Join(p.Dir, featureRevisionName(1))
			switch name {
			case "zero":
				revision = 0
			case "overflow":
				revision = math.MaxInt32 + 1
			case "root":
				p.Dir = filepath.Join(p.Dir, "missing")
			case "membership":
				os.WriteFile(filepath.Join(p.Dir, sidecarFile), []byte("corrupt"), 0600)
			case "missing":
				os.Remove(archive)
			case "corrupt":
				os.WriteFile(archive, []byte{0xff}, 0600)
			case "identity", "schema":
				d := proto.Clone(s.Document).(*pb.FeatureAnnotations)
				if name == "identity" {
					d.Revision = 9
				} else {
					d.SchemaVersion = 2
				}
				b, err := proto.Marshal(d)
				if err != nil {
					t.Fatal(err)
				}
				os.WriteFile(archive, b, 0600)
			case "symlink":
				os.Remove(archive)
				outside := filepath.Join(t.TempDir(), "outside.pb")
				b, _ := proto.Marshal(s.Document)
				os.WriteFile(outside, b, 0600)
				if err := os.Symlink(outside, archive); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := LoadFeatureRevision(p, revision); err == nil {
				t.Fatal("accepted invalid retained revision")
			}
		})
	}
}

func TestFeatureConflictAndIdentity(t *testing.T) {
	p, e := featureFixture(t)
	s, err := SaveFeatures(p, e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = SaveFeatures(p, e); !errors.Is(err, ErrSidecarConflict) {
		t.Fatal(err)
	}
	e.Document = s.Document
	e.BaseDigest = s.Digest
	e.Document.Features[0].PartId = "different"
	if _, err = SaveFeatures(p, e); err == nil {
		t.Fatal("reassigned feature part")
	}
	if _, err = SaveFeatures(p, nil); err == nil {
		t.Fatal("nil edit")
	}
	e.Document.Features[0].PartId = "body"
	e.Document.Author = " "
	if _, err = SaveFeatures(p, e); err == nil {
		t.Fatal("unnamed save")
	}
}

func TestFeatureValidationRefusals(t *testing.T) {
	p, original := featureFixture(t)
	cases := []struct {
		name   string
		change func(*pb.FeatureAnnotations)
	}{
		{"schema", func(d *pb.FeatureAnnotations) { d.SchemaVersion++ }},
		{"source", func(d *pb.FeatureAnnotations) { d.PackDigest = "wrong" }},
		{"unknown document", func(d *pb.FeatureAnnotations) { d.ProtoReflect().SetUnknown([]byte{0x78, 1}) }},
		{"duplicate feature", func(d *pb.FeatureAnnotations) { d.Features = append(d.Features, d.Features[0]) }},
		{"nil feature", func(d *pb.FeatureAnnotations) { d.Features = append(d.Features, nil) }},
		{"geometry", func(d *pb.FeatureAnnotations) { d.Features[0].Geometry = 99 }},
		{"anchor", func(d *pb.FeatureAnnotations) { d.Features[0].Anchor = &pb.FeatureAnchor{} }},
		{"duplicate sample", func(d *pb.FeatureAnnotations) {
			f := d.Features[0]
			f.Observations = append(f.Observations, f.Observations[0])
		}},
		{"sample", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].SampleId = 99 }},
		{"time", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].TimestampNs++ }},
		{"author", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].Author = "" }},
		{"decision", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].Decision = 0 }},
		{"source sample", func(d *pb.FeatureAnnotations) { v := uint32(0); d.Features[0].Observations[0].ProposedFromSample = &v }},
		{"membership revision", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].MembershipRevision = 0 }},
		{"missing revision", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].MembershipRevision = 99 }},
		{"membership digest", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].MembershipDigest = "wrong" }},
		{"object", func(d *pb.FeatureAnnotations) { d.Features[0].ObjectId = "ghost" }},
		{"rejected points", func(d *pb.FeatureAnnotations) {
			d.Features[0].Observations[0].Decision = pb.FeatureDecision_FEATURE_DECISION_REJECTED
		}},
		{"sphere", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].Sphere.RadiusM = math.NaN() }},
		{"finite", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].Sphere.XM = math.Inf(1) }},
		{"empty", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].PointIndices = nil }},
		{"wrong object point", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].PointIndices = []uint32{8} }},
		{"duplicate point", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].PointIndices = []uint32{0, 0} }},
		{"outside sphere", func(d *pb.FeatureAnnotations) { d.Features[0].Observations[0].Sphere.RadiusM = 0.01 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := proto.Clone(original.Document).(*pb.FeatureAnnotations)
			tc.change(d)
			if err := ValidateFeatures(p, d); err == nil {
				t.Fatal("accepted invalid document")
			}
		})
	}
	for _, decision := range []pb.FeatureDecision{pb.FeatureDecision_FEATURE_DECISION_REJECTED, pb.FeatureDecision_FEATURE_DECISION_MISSING, pb.FeatureDecision_FEATURE_DECISION_OCCLUDED} {
		d := proto.Clone(original.Document).(*pb.FeatureAnnotations)
		o := d.Features[0].Observations[0]
		o.PointIndices = nil
		o.Decision = decision
		if err := ValidateFeatures(p, d); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFeatureDamagedHeadAndArchive(t *testing.T) {
	p, e := featureFixture(t)
	s, err := SaveFeatures(p, e)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(p.Dir, featureFile), []byte{0xff}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadFeatures(p); err == nil {
		t.Fatal("corrupt head accepted")
	}
	b, _ := proto.Marshal(s.Document)
	os.WriteFile(filepath.Join(p.Dir, featureFile), b, 0600)
	os.MkdirAll(filepath.Join(p.Dir, featureHistory), 0700)
	os.WriteFile(filepath.Join(p.Dir, featureRevisionName(1)), []byte("wrong"), 0600)
	if _, err = SaveFeatures(p, &pb.FeatureEdit{Document: s.Document, BaseDigest: s.Digest, MembershipDigest: s.MembershipDigest}); err == nil {
		t.Fatal("history overwritten")
	}
	os.Remove(filepath.Join(p.Dir, featureFile))
	if _, err = LoadFeatures(p); err == nil {
		t.Fatal("lost head became new")
	}
}

func TestFeatureFilesystemAndValidationFailures(t *testing.T) {
	t.Run("missing root", func(t *testing.T) {
		p, e := featureFixture(t)
		p.Dir = filepath.Join(p.Dir, "absent")
		if _, err := LoadFeatures(p); err == nil {
			t.Fatal("missing root loaded")
		}
		if _, err := SaveFeatures(p, e); err == nil {
			t.Fatal("missing root saved")
		}
	})
	t.Run("damaged membership", func(t *testing.T) {
		p, e := featureFixture(t)
		os.WriteFile(filepath.Join(p.Dir, sidecarFile), []byte("corrupt"), 0600)
		if _, err := LoadFeatures(p); err == nil {
			t.Fatal("bad membership loaded")
		}
		if _, err := SaveFeatures(p, e); err == nil {
			t.Fatal("bad membership saved")
		}
	})
	t.Run("bad feature file", func(t *testing.T) {
		p, e := featureFixture(t)
		os.Mkdir(filepath.Join(p.Dir, featureFile), 0700)
		if _, err := LoadFeatures(p); err == nil {
			t.Fatal("directory loaded")
		}
		if _, err := SaveFeatures(p, e); err == nil {
			t.Fatal("directory overwritten")
		}
		os.Remove(filepath.Join(p.Dir, featureFile))
		e.Document.SchemaVersion++
		b, _ := proto.Marshal(e.Document)
		os.WriteFile(filepath.Join(p.Dir, featureFile), b, 0600)
		if _, err := LoadFeatures(p); err == nil {
			t.Fatal("future document loaded")
		}
	})
	t.Run("busy", func(t *testing.T) {
		p, e := featureFixture(t)
		r, err := os.OpenRoot(p.Dir)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		lock, err := lockAnnotations(r)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		if _, err := SaveFeatures(p, e); !errors.Is(err, ErrSidecarBusy) {
			t.Fatal(err)
		}
	})
	t.Run("exhausted revision", func(t *testing.T) {
		p, e := featureFixture(t)
		e.Document.Revision = math.MaxInt32
		b, _ := proto.Marshal(e.Document)
		os.WriteFile(filepath.Join(p.Dir, featureFile), b, 0600)
		e.BaseDigest = sha256Hex(b)
		if _, err := SaveFeatures(p, e); err == nil {
			t.Fatal("exhausted revision saved")
		}
	})
	t.Run("invalid edit", func(t *testing.T) {
		p, e := featureFixture(t)
		e.Document.Features[0].Observations[0].Origin = "independent"
		if _, err := SaveFeatures(p, e); err == nil {
			t.Fatal("unsupported origin saved")
		}
	})
	t.Run("marshal and size", func(t *testing.T) {
		p, e := featureFixture(t)
		e.Document.Features[0].Name = string([]byte{0xff})
		if _, err := SaveFeatures(p, e); err == nil {
			t.Fatal("invalid UTF-8 saved")
		}
		e.Document.Features[0].Name = strings.Repeat("x", MaxSidecarBytes)
		if _, err := SaveFeatures(p, e); err == nil {
			t.Fatal("oversize document saved")
		}
	})
	t.Run("unwritable head", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root bypasses directory permissions")
		}
		p, e := featureFixture(t)
		if _, err := SaveFeatures(p, e); err != nil {
			t.Fatal(err)
		}
		os.Remove(filepath.Join(p.Dir, featureFile))
		os.Chmod(p.Dir, 0500)
		defer os.Chmod(p.Dir, 0700)
		if _, err := SaveFeatures(p, e); err == nil {
			t.Fatal("unwritable head saved")
		}
	})
	t.Run("nonfinite source support", func(t *testing.T) {
		p, e := featureFixture(t)
		binary.LittleEndian.PutUint32(p.raw, math.Float32bits(float32(math.NaN())))
		if err := ValidateFeatures(p, e.Document); err == nil {
			t.Fatal("nonfinite support accepted")
		}
	})
	t.Run("uncertain object support", func(t *testing.T) {
		p, e := featureFixture(t)
		s, _ := LoadSidecar(p)
		s.Masks[0].PointIndices = []int{0, 2, 3, 4, 5, 6, 7}
		s.Masks[0].UncertainIndices = []int{1}
		if err := SaveSidecar(p, s); err != nil {
			t.Fatal(err)
		}
		o := e.Document.Features[0].Observations[0]
		o.MembershipRevision = uint64(s.Revision)
		o.MembershipDigest = s.Digest()
		if err := ValidateFeatures(p, e.Document); err == nil {
			t.Fatal("uncertain object return became definite feature")
		}
	})
	if err := ValidateFeatures(nil, nil); err == nil {
		t.Fatal("nil document accepted")
	}
}

func TestFeatureHistoricalEvidenceCannotBeChangedAgainstOldMembership(t *testing.T) {
	p, e := featureFixture(t)
	first, err := SaveFeatures(p, e)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := LoadSidecar(p)
	s.Change.Operation = "updated evidence"
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	next := &pb.FeatureEdit{Document: proto.Clone(first.Document).(*pb.FeatureAnnotations), BaseDigest: first.Digest, MembershipDigest: s.Digest()}
	// Metadata-only edits retain the original immutable support pin.
	next.Document.Features[0].Name = "renamed feature"
	second, err := SaveFeatures(p, next)
	if err != nil {
		t.Fatal(err)
	}
	next.Document = second.Document
	next.BaseDigest = second.Digest
	next.Document.Features[0].Observations[0].Note = "changed evidence interpretation"
	if _, err := SaveFeatures(p, next); !errors.Is(err, ErrMembershipChanged) {
		t.Fatalf("old support edit: %v", err)
	}
}

func TestConcurrentFeatureEditorsDoNotLoseAnAcceptedRevision(t *testing.T) {
	p, e := featureFixture(t)
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := SaveFeatures(p, e); results <- err }()
	}
	accepted := 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			accepted++
		} else if !errors.Is(err, ErrSidecarBusy) && !errors.Is(err, ErrSidecarConflict) {
			t.Fatal(err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d concurrent edits", accepted)
	}
	s, err := LoadFeatures(p)
	if err != nil || s.Document.Revision != 1 {
		t.Fatalf("lost revision: %+v %v", s, err)
	}
}

func TestFeatureActiveFacetLimitRetainsRetiredEvidence(t *testing.T) {
	p, edit := featureFixture(t)
	first := edit.Document.Features[0]
	for i := 1; i < 5; i++ {
		f := proto.Clone(first).(*pb.FeatureCandidate)
		f.FeatureId = fmt.Sprintf("facet-%d", i)
		edit.Document.Features = append(edit.Document.Features, f)
	}
	if _, err := SaveFeatures(p, edit); err == nil || !strings.Contains(err.Error(), "four active facets") {
		t.Fatalf("fifth active facet accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.Dir, featureFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("refused save wrote a file: %v", err)
	}
	edit.Document.Features[0].Inactive = true
	state, err := SaveFeatures(p, edit)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Document.Features) != 5 || len(state.Document.Features[0].Observations) != 1 {
		t.Fatal("retiring erased evidence")
	}
	next := proto.Clone(state.Document).(*pb.FeatureAnnotations)
	next.Features[0].Inactive = false
	if _, err := SaveFeatures(p, &pb.FeatureEdit{Document: next, BaseDigest: state.Digest, MembershipDigest: state.MembershipDigest}); err == nil {
		t.Fatal("reactivation bypassed active cap")
	}
	for _, f := range next.Features {
		f.Inactive = true
	}
	if _, err := SaveFeatures(p, &pb.FeatureEdit{Document: next, BaseDigest: state.Digest, MembershipDigest: state.MembershipDigest}); err != nil {
		t.Fatalf("zero active facets must support abstention: %v", err)
	}
}

// A document saved before the cap existed is not frozen out of editing: it can
// edit another object, and come down one retirement at a time, but never grow.
func TestFeatureActiveFacetLimitLetsLegacyDocumentsComeDown(t *testing.T) {
	p, edit := featureFixture(t)
	first := edit.Document.Features[0]
	for i := 1; i < 6; i++ {
		f := proto.Clone(first).(*pb.FeatureCandidate)
		f.FeatureId = fmt.Sprintf("legacy-%d", i)
		edit.Document.Features = append(edit.Document.Features, f)
	}
	// Written as the head directly, as authoring before the cap could save it.
	edit.Document.Revision = 1
	b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(edit.Document)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(p.Dir, featureFile), b, 0600); err != nil {
		t.Fatal(err)
	}
	state, err := LoadFeatures(p)
	if err != nil || activeFacets(state.Document)["car-1"] != 6 {
		t.Fatalf("legacy head: %v", err)
	}
	save := func(doc *pb.FeatureAnnotations) (*pb.FeatureState, error) {
		return SaveFeatures(p, &pb.FeatureEdit{Document: doc, BaseDigest: state.Digest, MembershipDigest: state.MembershipDigest})
	}

	// Another object gains a facet; car-1 stays at six, unchanged.
	points, err := p.PointsAt(0)
	if err != nil {
		t.Fatal(err)
	}
	other := proto.Clone(first).(*pb.FeatureCandidate)
	other.FeatureId, other.ObjectId = "car-2-mirror", "car-2"
	o := other.Observations[0]
	o.PointIndices = []uint32{8, 9}
	o.Sphere = &pb.FeatureSphere{XM: float64(points.X[8]), YM: float64(points.Y[8]), ZM: 0.9, RadiusM: 0.7}
	next := proto.Clone(state.Document).(*pb.FeatureAnnotations)
	next.Features = append(next.Features, other)
	if state, err = save(next); err != nil {
		t.Fatalf("legacy object blocked an edit to another object: %v", err)
	}

	// Six may come down to five.
	next = proto.Clone(state.Document).(*pb.FeatureAnnotations)
	next.Features[0].Inactive = true
	if state, err = save(next); err != nil {
		t.Fatalf("retiring a legacy facet refused: %v", err)
	}
	if activeFacets(state.Document)["car-1"] != 5 {
		t.Fatal(activeFacets(state.Document))
	}

	// Five may not go back up, by reactivation or by a new facet.
	next = proto.Clone(state.Document).(*pb.FeatureAnnotations)
	next.Features[0].Inactive = false
	if _, err = save(next); err == nil || !strings.Contains(err.Error(), "four active facets") {
		t.Fatalf("reactivation grew a legacy object: %v", err)
	}
	next = proto.Clone(state.Document).(*pb.FeatureAnnotations)
	added := proto.Clone(first).(*pb.FeatureCandidate)
	added.FeatureId = "new"
	next.Features = append(next.Features, added)
	if _, err = save(next); err == nil || !strings.Contains(err.Error(), "four active facets") {
		t.Fatalf("new facet grew a legacy object: %v", err)
	}
}

func registeredFeatureFixture(t *testing.T) (*Pack, *pb.FeatureEdit) {
	t.Helper()
	p, e := featureFixture(t)
	return registeredFeatureFixtureForPack(t, p, e)
}

func registeredFeatureFixtureForPack(t *testing.T, p *Pack, e *pb.FeatureEdit) (*Pack, *pb.FeatureEdit) {
	t.Helper()
	r := validPhysical(p)
	pin := &MembershipPin{Revision: int(e.Document.Features[0].Observations[0].MembershipRevision), Digest: e.MembershipDigest}
	for i := range r.Objects {
		if r.Objects[i].Body != nil {
			r.Objects[i].Body.Review.ReviewedAgainst = pin
		}
		for j := range r.Objects[i].Keyframes {
			if r.Objects[i].Keyframes[j].Review.Status == StatusReviewed {
				r.Objects[i].Keyframes[j].Review.ReviewedAgainst = pin
			}
		}
	}
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	f := e.Document.Features[0]
	second := proto.Clone(f.Observations[0]).(*pb.FeatureObservation)
	second.SampleId, second.TimestampNs, second.SourceOrdinal = 1, physTime(1), 1
	second.Sphere.XM++
	f.Observations = append(f.Observations, second)
	point := uint32(0)
	distance := math.Hypot(2.25, float64(float32(0.9)))
	f.PartRelation = "rigid_proposal"
	f.Anchor = &pb.FeatureAnchor{PartFrameId: "body-car-1_xy", PartFrameRevision: 1, XM: -2.25, YM: float64(float32(-0.9)),
		PhysicalRevision: uint64(r.Revision), PhysicalDigest: r.Digest(), BodyId: "body-car-1", KeyframeId: "kf-car-1-s0",
		CoordinateDomain: "body_xy", SourceSample: 0, SourcePointIndex: &point, ReturnBoundM: 0.05,
		BoundM: 0.2 + distance*2*math.Sin(0.025) + 0.05, Origin: "reference_seeded_proposal", Method: "manual_named_return_v1", IdentityNote: "outer rigid mirror tip, checked in frames 0 and 1"}
	return p, e
}

func TestFeatureBodyRegistrationKeepsMetricOffsetAndPhysicalPin(t *testing.T) {
	p, e := registeredFeatureFixture(t)
	before, _ := os.ReadFile(filepath.Join(p.Dir, "physical-references.json"))
	s, err := SaveFeatures(p, e)
	if err != nil {
		t.Fatal(err)
	}
	if s.Document.Features[0].Anchor.SourcePointIndex == nil || *s.Document.Features[0].Anchor.SourcePointIndex != 0 {
		t.Fatal("zero source index was confused with absence")
	}
	after, _ := os.ReadFile(filepath.Join(p.Dir, "physical-references.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("registration changed the physical reference")
	}
	r, _ := LoadPhysicalReferences(p)
	r.Objects[0].Body.BodyID = "new-body-frame"
	r.Objects[0].Body.Width.ValueM = fp(1.85)
	if err := SavePhysicalReferences(p, r); err != nil {
		t.Fatal(err)
	}
	oldAnchor := proto.Clone(s.Document.Features[0].Anchor).(*pb.FeatureAnchor)
	loaded, err := LoadFeatures(p)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(oldAnchor, loaded.Document.Features[0].Anchor) {
		t.Fatal("dimension edit rescaled the metric anchor")
	}
	if _, err := SaveFeatures(p, &pb.FeatureEdit{Document: loaded.Document, BaseDigest: loaded.Digest, MembershipDigest: loaded.MembershipDigest}); err != nil {
		t.Fatalf("retained physical pin could not be read: %v", err)
	}
}

func TestFeatureBodyRegistrationRefusesInventedOrUnpinnedConstraints(t *testing.T) {
	p, original := registeredFeatureFixture(t)
	cases := []struct {
		name   string
		change func(*pb.FeatureCandidate)
	}{
		{"coordinate", func(f *pb.FeatureCandidate) { f.Anchor.XM++ }},
		{"bound", func(f *pb.FeatureCandidate) { f.Anchor.BoundM = 0 }},
		{"return bound", func(f *pb.FeatureCandidate) { f.Anchor.ReturnBoundM = 0 }},
		{"nonfinite", func(f *pb.FeatureCandidate) { f.Anchor.YM = math.NaN() }},
		{"z unresolved", func(f *pb.FeatureCandidate) { f.Anchor.ZM = 1 }},
		{"source absent", func(f *pb.FeatureCandidate) { f.Anchor.SourcePointIndex = nil }},
		{"outside support", func(f *pb.FeatureCandidate) { v := uint32(7); f.Anchor.SourcePointIndex = &v }},
		{"source frame", func(f *pb.FeatureCandidate) { f.Anchor.SourceSample = 2 }},
		{"revision", func(f *pb.FeatureCandidate) { f.Anchor.PhysicalRevision = 99 }},
		{"digest", func(f *pb.FeatureCandidate) { f.Anchor.PhysicalDigest = "wrong" }},
		{"body", func(f *pb.FeatureCandidate) { f.Anchor.BodyId = "wrong"; f.Anchor.PartFrameId = "wrong_xy" }},
		{"pose", func(f *pb.FeatureCandidate) { f.Anchor.KeyframeId = "kf-car-1-s3" }},
		{"point on patch", func(f *pb.FeatureCandidate) { f.Geometry = pb.FeatureGeometry_FEATURE_GEOMETRY_PATCH }},
		{"one frame", func(f *pb.FeatureCandidate) { f.Observations = f.Observations[:1] }},
		{"note", func(f *pb.FeatureCandidate) { f.Anchor.IdentityNote = " " }},
		{"truth origin", func(f *pb.FeatureCandidate) { f.Anchor.Origin = "independent" }},
		{"frame id", func(f *pb.FeatureCandidate) { f.Anchor.PartFrameRevision = 2 }},
		{"relation", func(f *pb.FeatureCandidate) { f.PartRelation = "unknown" }},
		{"unknown anchor", func(f *pb.FeatureCandidate) { f.Anchor.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 1}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := proto.Clone(original.Document).(*pb.FeatureAnnotations)
			tc.change(doc.Features[0])
			if err := ValidateFeatures(p, doc); err == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}
}

func TestFeatureBodyRegistrationRetainsAssistedOriginFromEitherBodyOrPose(t *testing.T) {
	for _, component := range []string{"body", "pose", "both"} {
		t.Run(component, func(t *testing.T) {
			p, e := registeredFeatureFixture(t)
			r, err := LoadPhysicalReferences(p)
			if err != nil {
				t.Fatal(err)
			}
			o := &r.Objects[0]
			if component != "pose" {
				o.Body.BodyID = "assisted-body"
				o.Body.Review.Origin, o.Body.Review.TrackerSource = OriginTrackerAssisted, "online seed"
			}
			if component != "body" {
				o.Keyframes[0].KeyframeID = "assisted-pose"
				o.Keyframes[0].Review.Origin, o.Keyframes[0].Review.TrackerSource = OriginTrackerAssisted, "online seed"
			}
			if err := SavePhysicalReferences(p, r); err != nil {
				t.Fatal(err)
			}
			a := e.Document.Features[0].Anchor
			a.PhysicalRevision, a.PhysicalDigest = uint64(r.Revision), r.Digest()
			a.BodyId, a.PartFrameId, a.KeyframeId = o.Body.BodyID, o.Body.BodyID+"_xy", o.Keyframes[0].KeyframeID
			// A reviewed assisted pose must not masquerade as an independent seed.
			if err := ValidateFeatures(p, e.Document); err == nil {
				t.Fatal("assistance was erased")
			}
			a.Origin = "tracker_seeded_proposal"
			if _, err := SaveFeatures(p, e); err != nil {
				t.Fatal(err)
			}
			a.Origin = "independent"
			if err := ValidateFeatures(p, e.Document); err == nil {
				t.Fatal("proposal became reference truth")
			}
		})
	}
	p, e := registeredFeatureFixture(t)
	e.Document.Features[0].Anchor.Origin = "tracker_seeded_proposal"
	if err := ValidateFeatures(p, e.Document); err == nil {
		t.Fatal("source origin was invented")
	}
}

func TestNewFeatureRegistrationRefusesStaleMembershipButRetainsHistory(t *testing.T) {
	p, e := registeredFeatureFixture(t)
	s, err := SaveFeatures(p, e)
	if err != nil {
		t.Fatal(err)
	}
	membership, _ := LoadSidecar(p)
	membership.Objects[0].Subtype = "updated label"
	if err := SaveSidecar(p, membership); err != nil {
		t.Fatal(err)
	}
	retained, err := SaveFeatures(p, &pb.FeatureEdit{Document: s.Document, BaseDigest: s.Digest, MembershipDigest: membership.Digest()})
	if err != nil {
		t.Fatalf("retained mapping refused: %v", err)
	}
	edit := proto.Clone(retained.Document).(*pb.FeatureAnnotations)
	edit.Features[0].Anchor.IdentityNote += "; revised"
	if _, err := SaveFeatures(p, &pb.FeatureEdit{Document: edit, BaseDigest: retained.Digest, MembershipDigest: membership.Digest()}); !errors.Is(err, ErrMembershipChanged) {
		t.Fatalf("new mapping accepted stale feature support: %v", err)
	}
}
