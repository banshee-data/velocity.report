package annotation

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"google.golang.org/protobuf/proto"
)

const featureFile = "feature-proposals.pb"
const featureHistory = "feature-proposal-revisions"
const featureSchema = "velocity.report/feature-proposals"

// LoadFeatures reads proposals independently of the membership writer. Each
// observation pins the membership it was authored against; editing membership
// later does not silently rewrite that historical evidence.
func LoadFeatures(p *Pack) (*pb.FeatureState, error) {
	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	s, err := readSidecar(p, root)
	if err != nil {
		return nil, err
	}
	doc, digest, err := readFeatures(p, root)
	if err != nil {
		return nil, err
	}
	return &pb.FeatureState{Document: doc, Digest: digest, MembershipDigest: s.baseDigest, PackDirectory: p.Dir}, nil
}

func readFeatures(p *Pack, root *os.Root) (*pb.FeatureAnnotations, string, error) {
	b, err := readAnnotationFile(root, featureFile)
	if errors.Is(err, os.ErrNotExist) {
		if _, e := root.Stat(featureHistory); !errors.Is(e, os.ErrNotExist) {
			return nil, "", fmt.Errorf("feature head missing but history exists or is unreadable")
		}
		return &pb.FeatureAnnotations{Schema: featureSchema, SchemaVersion: 1,
			PackDigest: p.Manifest.PackDigest, DatasetId: p.Manifest.DatasetID, PointDomain: "legacy_pack"}, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	doc := new(pb.FeatureAnnotations)
	if err = proto.Unmarshal(b, doc); err != nil {
		return nil, "", err
	}
	if err = ValidateFeatures(p, doc); err != nil {
		return nil, "", err
	}
	return doc, sha256Hex(b), nil
}

// SaveFeatures rejects a changed feature file or membership under the shared
// annotation lock. No mask or physical-reference file is written. Accepted
// proposals are human association judgements, never reviewed physical truth.
func SaveFeatures(p *Pack, edit *pb.FeatureEdit) (*pb.FeatureState, error) {
	if edit == nil || edit.Document == nil {
		return nil, fmt.Errorf("missing feature document")
	}
	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	lock, err := lockAnnotations(root)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	s, err := readSidecar(p, root)
	if err != nil {
		return nil, err
	}
	if s.baseDigest != edit.MembershipDigest {
		return nil, ErrMembershipChanged
	}
	old, digest, err := readFeatures(p, root)
	if err != nil {
		return nil, err
	}
	if digest != edit.BaseDigest || old.Revision != edit.Document.Revision {
		return nil, ErrSidecarConflict
	}
	next := proto.Clone(edit.Document).(*pb.FeatureAnnotations)
	if old.Revision >= math.MaxInt32 {
		return nil, fmt.Errorf("feature revision exhausted")
	}
	next.Revision++
	next.UpdatedUtc = time.Now().UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(next.Author) == "" {
		return nil, fmt.Errorf("feature save needs an author")
	}
	if err = ValidateFeatures(p, next); err != nil {
		return nil, err
	}
	active := map[string]int{}
	for _, f := range next.Features {
		if !f.Inactive {
			active[f.ObjectId]++
			if active[f.ObjectId] > 4 {
				return nil, fmt.Errorf("object %s has more than four active facets; retire one before adding another", f.ObjectId)
			}
		}
	}
	// Feature identity cannot silently move to a different object or part.
	for _, before := range old.Features {
		for _, after := range next.Features {
			if before.FeatureId == after.FeatureId && (before.ObjectId != after.ObjectId || before.PartId != after.PartId) {
				return nil, fmt.Errorf("feature %s changed object or part identity", before.FeatureId)
			}
		}
	}
	// Old observations retain their original evidence pin. Every new or edited
	// observation must be checked against the membership held by this transaction.
	for _, f := range next.Features {
		for _, o := range f.Observations {
			unchanged := false
			for _, before := range old.Features {
				if before.FeatureId != f.FeatureId {
					continue
				}
				for _, previous := range before.Observations {
					if proto.Equal(previous, o) {
						unchanged = true
						break
					}
				}
			}
			if !unchanged && (o.MembershipDigest != s.baseDigest || o.MembershipRevision != uint64(s.Revision)) {
				return nil, ErrMembershipChanged
			}
		}
	}
	b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(next)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxSidecarBytes {
		return nil, fmt.Errorf("feature document too large")
	}
	if digest != "" {
		previous, err := readAnnotationFile(root, featureFile)
		if err != nil {
			return nil, err
		}
		if err = archiveRevision(root, featureHistory, featureRevisionName(old.Revision), previous, int(old.Revision)); err != nil {
			return nil, err
		}
	}
	if err = writeAnnotationFile(root, featureFile, b); err != nil {
		return nil, err
	}
	return &pb.FeatureState{Document: next, Digest: sha256Hex(b), MembershipDigest: s.baseDigest, PackDirectory: p.Dir}, nil
}

func featureRevisionName(rev uint64) string { return fmt.Sprintf("%s/%010d.pb", featureHistory, rev) }

// ValidateFeatures validates the shared protobuf against exact historical mask
// revisions. Unknown protobuf fields are refused on this editing path rather
// than discarded by a client that cannot understand their effect.
func ValidateFeatures(p *Pack, doc *pb.FeatureAnnotations) error {
	if doc == nil || doc.Schema != featureSchema || doc.SchemaVersion != 1 || doc.PointDomain != "legacy_pack" {
		return fmt.Errorf("unsupported feature schema or point domain")
	}
	if doc.PackDigest != p.Manifest.PackDigest || doc.DatasetId != p.Manifest.DatasetID {
		return fmt.Errorf("feature pack identity mismatch")
	}
	if len(doc.ProtoReflect().GetUnknown()) != 0 {
		return fmt.Errorf("unknown feature document fields")
	}
	ids := map[string]bool{}
	for _, f := range doc.Features {
		if f == nil || f.FeatureId == "" || ids[f.FeatureId] || f.ObjectId == "" || f.PartId == "" {
			return fmt.Errorf("missing or duplicate feature identity")
		}
		ids[f.FeatureId] = true
		if f.Geometry < 0 || f.Geometry > pb.FeatureGeometry_FEATURE_GEOMETRY_PROTRUSION || f.PartRelation != "unknown" || f.Anchor != nil || len(f.ProtoReflect().GetUnknown()) != 0 {
			return fmt.Errorf("unsupported feature geometry, part relation, anchor or fields")
		}
		seen := map[uint32]bool{}
		for _, o := range f.Observations {
			if o == nil || seen[o.SampleId] {
				return fmt.Errorf("duplicate or missing feature observation")
			}
			seen[o.SampleId] = true
			if err := validateFeatureObservation(p, f.ObjectId, o); err != nil {
				return fmt.Errorf("feature %s: %w", f.FeatureId, err)
			}
		}
	}
	return nil
}

func validateFeatureObservation(p *Pack, object string, o *pb.FeatureObservation) error {
	if int(o.SampleId) >= len(p.Samples) {
		return fmt.Errorf("sample out of range")
	}
	sample := p.Samples[o.SampleId]
	if sample.SampleID != int(o.SampleId) || sample.TimestampNs != o.TimestampNs || uint32(sample.SourceOrdinal) != o.SourceOrdinal {
		return fmt.Errorf("sample identity mismatch")
	}
	if len(o.ProtoReflect().GetUnknown()) != 0 || strings.TrimSpace(o.Author) == "" || strings.TrimSpace(o.Method) == "" {
		return fmt.Errorf("unknown fields or missing observation provenance")
	}
	if o.Origin != "human_proposal" && o.Origin != "assisted_proposal" {
		return fmt.Errorf("unsupported proposal origin")
	}
	if o.Decision < pb.FeatureDecision_FEATURE_DECISION_ACCEPTED_PROPOSAL || o.Decision > pb.FeatureDecision_FEATURE_DECISION_OCCLUDED {
		return fmt.Errorf("unsupported observation decision")
	}
	if o.ProposedFromSample != nil && (*o.ProposedFromSample >= uint32(len(p.Samples)) || *o.ProposedFromSample == o.SampleId) {
		return fmt.Errorf("invalid proposal source")
	}
	if o.MembershipRevision == 0 || o.MembershipRevision > math.MaxInt32 {
		return fmt.Errorf("missing membership revision")
	}
	s, err := LoadSidecarRevision(p, int(o.MembershipRevision))
	if err != nil {
		return err
	}
	if s.baseDigest != o.MembershipDigest {
		return fmt.Errorf("membership digest mismatch")
	}
	validObject := false
	for _, obj := range s.Objects {
		if obj.ObjectID == object && obj.Status != StatusRejected {
			validObject = true
		}
	}
	if !validObject {
		return fmt.Errorf("missing feature object")
	}
	allowed := map[uint32]bool{}
	for _, m := range s.Masks {
		if m.ObjectID == object && m.SampleID == int(o.SampleId) && m.Status != StatusRejected {
			for _, i := range m.PointIndices {
				allowed[uint32(i)] = true
			}
			for _, i := range m.UncertainIndices {
				delete(allowed, uint32(i))
			}
		}
	}
	if o.Decision != pb.FeatureDecision_FEATURE_DECISION_ACCEPTED_PROPOSAL {
		if len(o.PointIndices)+len(o.UncertainIndices) != 0 {
			return fmt.Errorf("unobserved/rejected feature carries points")
		}
		return nil
	}
	sp := o.Sphere
	if sp == nil || !(sp.RadiusM > 0 && sp.RadiusM <= 5) || !finiteFeature(sp.XM) || !finiteFeature(sp.YM) || !finiteFeature(sp.ZM) || len(sp.ProtoReflect().GetUnknown()) != 0 {
		return fmt.Errorf("invalid feature sphere")
	}
	if len(o.PointIndices) == 0 {
		return fmt.Errorf("accepted feature has no support")
	}
	points, err := p.PointsAt(int(o.SampleId))
	if err != nil {
		return err
	}
	used := map[uint32]bool{}
	for _, group := range [][]uint32{o.PointIndices, o.UncertainIndices} {
		for j, i := range group {
			if used[i] || !allowed[i] || int(i) >= len(points.X) || (j > 0 && i <= group[j-1]) {
				return fmt.Errorf("invalid feature member index")
			}
			used[i] = true
			dx, dy, dz := float64(points.X[i])-sp.XM, float64(points.Y[i])-sp.YM, float64(points.Z[i])-sp.ZM
			if !finiteFeature(dx) || !finiteFeature(dy) || !finiteFeature(dz) || dx*dx+dy*dy+dz*dz > sp.RadiusM*sp.RadiusM+1e-6 {
				return fmt.Errorf("feature point outside sphere")
			}
		}
	}
	return nil
}

func finiteFeature(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
