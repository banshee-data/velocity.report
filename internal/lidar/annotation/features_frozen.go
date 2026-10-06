package annotation

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"google.golang.org/protobuf/proto"
)

// FrozenFeatures pins proposals, not physical truth. Counts include retired
// facets and absence decisions so a favourable subset cannot replace the record.
type FrozenFeatures struct {
	Revision                   uint64 `json:"revision"`
	SHA256                     string `json:"sha256"`
	ContentSHA256              string `json:"content_sha256"`
	Candidates                 int    `json:"candidates"`
	Active                     int    `json:"active"`
	SupportedObservations      int    `json:"supported_observations"`
	AbsenceDecisions           int    `json:"absence_decisions"`
	Registrations              int    `json:"registrations"`
	TrackerSeededRegistrations int    `json:"tracker_seeded_registrations"`
	// PhysicalDivergences records each body registration made against a
	// physical revision other than the one this split pins, or made while the
	// split pins none. A registration keeps the revision it was made against,
	// so this is a recorded fact, not a reason to refuse the freeze.
	PhysicalDivergences []FacetPhysicalDivergence `json:"physical_divergences,omitempty"`
}

// FacetPhysicalDivergence is one registration whose pinned physical revision
// is not the split's physical pin: the facet, its object, and the revision
// and exact-bytes digest the registration names.
type FacetPhysicalDivergence struct {
	FeatureID        string `json:"feature_id"`
	ObjectID         string `json:"object_id"`
	PhysicalRevision uint64 `json:"physical_revision"`
	PhysicalDigest   string `json:"physical_digest"`
}

// facetPhysicalDivergences lists, by feature ID, the registrations whose
// physical revision or digest differs from the split's physical pin. Nil
// when every registration names the pinned revision.
func facetPhysicalDivergences(doc *pb.FeatureAnnotations, physical *FrozenPhysical) []FacetPhysicalDivergence {
	var out []FacetPhysicalDivergence
	for _, facet := range doc.Features {
		a := facet.Anchor
		if a == nil {
			continue
		}
		if physical != nil && a.PhysicalRevision == uint64(physical.Revision) && a.PhysicalDigest == physical.SHA256 {
			continue
		}
		out = append(out, FacetPhysicalDivergence{FeatureID: facet.FeatureId, ObjectID: facet.ObjectId,
			PhysicalRevision: a.PhysicalRevision, PhysicalDigest: a.PhysicalDigest})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FeatureID < out[j].FeatureID })
	return out
}

func (pin *FrozenFeatures) validate() error {
	if pin.Revision == 0 || pin.Revision > math.MaxInt32 {
		return fmt.Errorf("facet revision must name a saved revision")
	}
	for _, digest := range []string{pin.SHA256, pin.ContentSHA256} {
		if !strings.HasPrefix(digest, "sha256:") || len(digest) != 71 {
			return fmt.Errorf("facet pin needs an exact SHA-256 digest")
		}
		for _, c := range strings.TrimPrefix(digest, "sha256:") {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return fmt.Errorf("facet pin digest is not lowercase hexadecimal")
			}
		}
	}
	if pin.Candidates < 0 || pin.Active < 0 || pin.Active > pin.Candidates || pin.SupportedObservations < 0 || pin.AbsenceDecisions < 0 || pin.Registrations < 0 || pin.Registrations > pin.Candidates || pin.TrackerSeededRegistrations < 0 || pin.TrackerSeededRegistrations > pin.Registrations {
		return fmt.Errorf("facet pin counts do not add up")
	}
	if len(pin.PhysicalDivergences) > pin.Registrations {
		return fmt.Errorf("facet pin records more physical divergences than registrations")
	}
	for i, d := range pin.PhysicalDivergences {
		if d.FeatureID == "" || d.ObjectID == "" || (i > 0 && pin.PhysicalDivergences[i-1].FeatureID >= d.FeatureID) {
			return fmt.Errorf("facet pin physical divergences need distinct feature IDs in order")
		}
	}
	return nil
}

func newFrozenFeatures(state *pb.FeatureState) (*FrozenFeatures, error) {
	bytes, err := (proto.MarshalOptions{Deterministic: true}).Marshal(state.Document)
	if err != nil {
		return nil, err
	}
	pin := &FrozenFeatures{Revision: state.Document.Revision, SHA256: state.Digest, ContentSHA256: sha256Hex(bytes), Candidates: len(state.Document.Features)}
	for _, facet := range state.Document.Features {
		if !facet.Inactive {
			pin.Active++
		}
		if facet.Anchor != nil {
			pin.Registrations++
			if facet.Anchor.Origin == "tracker_seeded_proposal" {
				pin.TrackerSeededRegistrations++
			}
		}
		for _, observation := range facet.Observations {
			if observation.Decision == pb.FeatureDecision_FEATURE_DECISION_ACCEPTED_PROPOSAL {
				pin.SupportedObservations++
			} else {
				pin.AbsenceDecisions++
			}
		}
	}
	return pin, pin.validate()
}

// facetMembershipProblems checks the frozen mask as well as each observation's
// own historical pin. A new mask revision is harmless if its definite subset
// still contains the evidence; moving a return to another object is not. A
// rejected mask supports nothing, as validateFeatureObservation holds.
func facetMembershipProblems(s *Sidecar, doc *pb.FeatureAnnotations) []string {
	var problems []string
	for _, facet := range doc.Features {
		validObject := false
		for _, object := range s.Objects {
			if object.ObjectID == facet.ObjectId && object.Status != StatusRejected {
				validObject = true
			}
		}
		if !validObject {
			problems = append(problems, fmt.Sprintf("facet %s: object %s is absent or rejected in the frozen membership", facet.FeatureId, facet.ObjectId))
			continue
		}
		for _, observation := range facet.Observations {
			if observation.Decision != pb.FeatureDecision_FEATURE_DECISION_ACCEPTED_PROPOSAL {
				continue
			}
			definite := map[uint32]bool{}
			for _, mask := range s.Masks {
				if mask.ObjectID == facet.ObjectId && mask.SampleID == int(observation.SampleId) && mask.Status != StatusRejected {
					for _, index := range mask.PointIndices {
						definite[uint32(index)] = true
					}
				}
			}
			for _, index := range observation.PointIndices {
				if !definite[index] {
					problems = append(problems, fmt.Sprintf("facet %s sample %d: return %d is not definite support of object %s in the frozen membership", facet.FeatureId, observation.SampleId, index, facet.ObjectId))
					break
				}
			}
		}
	}
	return problems
}

// freezeFeatures pins the facet proposals a draft opts into: the saved head
// at zero, or an exact retained revision. Asking for the head of a pack that
// has never saved facets is a facet problem, listed with the others so the
// preview still shows every pack; a named revision that cannot be loaded is
// an error, as a named physical revision is.
func freezeFeatures(p *Pack, s *Sidecar, revision *int, physical *FrozenPhysical) (*FrozenFeatures, []string, error) {
	if revision == nil {
		return nil, nil, nil
	}
	if *revision < 0 {
		return nil, nil, fmt.Errorf("invalid facet revision")
	}
	var state *pb.FeatureState
	var err error
	if *revision == 0 {
		state, err = LoadFeatures(p)
	} else {
		state, err = LoadFeatureRevision(p, uint64(*revision))
	}
	if err != nil {
		return nil, nil, fmt.Errorf("load facet proposals: %w", err)
	}
	if state.Digest == "" {
		return nil, []string{fmt.Sprintf("pack %s: feature_revision 0 pins the saved facet head, but the pack has no saved facet proposals; "+
			"save facets first, or omit the facet pin", p.Manifest.PackDigest)}, nil
	}
	pin, err := newFrozenFeatures(state)
	if err != nil {
		return nil, nil, err
	}
	pin.PhysicalDivergences = facetPhysicalDivergences(state.Document, physical)
	problems := facetMembershipProblems(s, state.Document)
	for i := range problems {
		problems[i] = fmt.Sprintf("pack %s facet revision %d: %s", p.Manifest.PackDigest, pin.Revision, problems[i])
	}
	return pin, problems, nil
}

// BindFeatures rechecks exact retained bytes and summaries against the frozen
// membership. It does not turn accepted proposals or assisted anchors into truth.
func (f *FrozenSplit) BindFeatures(p *Pack, s *Sidecar) (*pb.FeatureAnnotations, error) {
	entry, err := f.entry(p)
	if err != nil {
		return nil, err
	}
	if entry.Features == nil {
		return nil, nil
	}
	if s == nil || s.Revision != entry.SidecarRevision || s.baseDigest != entry.SidecarSHA256 {
		return nil, fmt.Errorf("facets require the annotation revision returned by Bind")
	}
	state, err := LoadFeatureRevision(p, entry.Features.Revision)
	if err != nil {
		return nil, fmt.Errorf("load pinned facet revision: %w", err)
	}
	pin, err := newFrozenFeatures(state)
	if err != nil {
		return nil, err
	}
	pin.PhysicalDivergences = facetPhysicalDivergences(state.Document, entry.Physical)
	if !reflect.DeepEqual(pin, entry.Features) {
		return nil, fmt.Errorf("facet bytes or derived summary differ from the frozen pin")
	}
	if problems := facetMembershipProblems(s, state.Document); len(problems) > 0 {
		return nil, fmt.Errorf("facets do not hold against the frozen membership: %s", strings.Join(problems, "; "))
	}
	return state.Document, nil
}
