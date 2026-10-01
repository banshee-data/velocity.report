package annotation

import (
	"fmt"
	"math"
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
// still contains the evidence; moving a return to another object is not.
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
				if mask.ObjectID == facet.ObjectId && mask.SampleID == int(observation.SampleId) {
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

func freezeFeatures(p *Pack, s *Sidecar, revision *int) (*FrozenFeatures, []string, error) {
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
	pin, err := newFrozenFeatures(state)
	if err != nil {
		return nil, nil, err
	}
	return pin, facetMembershipProblems(s, state.Document), nil
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
	if *pin != *entry.Features {
		return nil, fmt.Errorf("facet bytes or derived summary differ from the frozen pin")
	}
	if problems := facetMembershipProblems(s, state.Document); len(problems) > 0 {
		return nil, fmt.Errorf("facets do not hold against the frozen membership: %s", strings.Join(problems, "; "))
	}
	return state.Document, nil
}
