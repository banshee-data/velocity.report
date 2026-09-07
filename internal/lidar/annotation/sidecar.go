package annotation

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// SidecarSchemaVersion is the annotation record layout version.
const SidecarSchemaVersion = 1

const sidecarFile = "annotations.json"

// ReviewStatus separates what a person has confirmed from what an algorithm
// suggested. The distinction is load-bearing: a proposal that can quietly
// become a reviewed label through a save-and-reload cycle would let the
// tracker grade its own homework.
type ReviewStatus string

const (
	// StatusProposed was produced by an algorithm and has not been confirmed.
	// It may never be exported as reference truth.
	StatusProposed ReviewStatus = "proposed"
	// StatusReviewed was confirmed by a person.
	StatusReviewed ReviewStatus = "reviewed"
	// StatusRejected was examined and found wrong. It is kept rather than
	// deleted so the same proposal is not re-offered as if it were new.
	StatusRejected ReviewStatus = "rejected"
)

// Visibility records what could be seen of the object, which is not the same
// as what was labelled. An object fully occluded in a frame has an empty mask
// for an understandable reason, and that is different from nobody having
// looked at it yet.
type Visibility string

const (
	VisiblePresent        Visibility = "present"
	VisiblePartlyOccluded Visibility = "partly_occluded"
	VisibleFullyOccluded  Visibility = "fully_occluded"
	VisibleOutsideView    Visibility = "outside_view"
	VisibleUnknown        Visibility = "unknown"
)

// Completeness states whether a mask claims to hold every visible return of
// the object in that sample. Even a complete object mask does not establish
// background negatives: that needs an explicitly exhaustively reviewed ROI.
type MaskCompleteness string

const (
	MaskComplete   MaskCompleteness = "complete"
	MaskPartial    MaskCompleteness = "partial"
	MaskUnreviewed MaskCompleteness = "unreviewed"
)

// Provenance records who or what produced a record and from where.
type Provenance struct {
	Author     string `json:"author"`
	Session    string `json:"session,omitempty"`
	CreatedUTC string `json:"created_utc"`
	Operation  string `json:"operation,omitempty"`
	ParentRev  int    `json:"parent_revision,omitempty"`
	Revision   int    `json:"revision"`
	// Algorithm and AlgorithmVersion are set only for proposals, and are what
	// makes a suggestion traceable to the code that made it.
	Algorithm        string `json:"algorithm,omitempty"`
	AlgorithmVersion string `json:"algorithm_version,omitempty"`
}

// Object is a physical road user, identified independently of any predicted
// track. One object may correspond to several predicted tracks, and one
// merged predicted track to several objects; neither is an error.
type Object struct {
	ObjectID   string       `json:"object_id"`
	Class      string       `json:"class"`
	Subtype    string       `json:"subtype,omitempty"`
	Confidence float64      `json:"confidence"`
	Status     ReviewStatus `json:"status"`
	Provenance Provenance   `json:"provenance"`
	// Notes carries anything the operator needs a later reader to know, such
	// as why two objects that look alike are deliberately separate.
	Notes string `json:"notes,omitempty"`
}

// Pose is an optional physical box. It is kept apart from the mask because a
// mask describes returns that were observed while a box asserts extent that
// was mostly not: the minimum and maximum of a partial mask is not a vehicle.
type Pose struct {
	CenterX float64 `json:"center_x"`
	CenterY float64 `json:"center_y"`
	CenterZ float64 `json:"center_z"`
	Length  float64 `json:"length"`
	Width   float64 `json:"width"`
	Height  float64 `json:"height"`
	YawRad  float64 `json:"yaw_rad"`
	// AxisAmbiguous marks a pose whose long axis could not be resolved. It is
	// not the same as a pose nobody supplied.
	AxisAmbiguous bool         `json:"axis_ambiguous"`
	Confidence    float64      `json:"confidence"`
	Status        ReviewStatus `json:"status"`
	Provenance    Provenance   `json:"provenance"`
}

// FrameMask is one object's membership within one sample.
type FrameMask struct {
	ObjectID string `json:"object_id"`
	SampleID int    `json:"sample_id"`
	// PointIndices is the canonical sorted, duplicate-free membership set.
	PointIndices []int `json:"point_indices"`
	// UncertainIndices are returns the operator could not decide about. They
	// are neither positives nor negatives and are excluded from hard scores,
	// rather than being quietly rounded to whichever is convenient.
	UncertainIndices []int            `json:"uncertain_indices,omitempty"`
	Completeness     MaskCompleteness `json:"completeness"`
	Visibility       Visibility       `json:"visibility"`
	Status           ReviewStatus     `json:"status"`
	Pose             *Pose            `json:"pose,omitempty"`
	Provenance       Provenance       `json:"provenance"`
}

// TrackCorrespondence links a reference object to predicted tracks over a time
// range. It is written after the fact for reporting and is never an input to
// the estimator being judged.
type TrackCorrespondence struct {
	ObjectID string   `json:"object_id"`
	RunID    string   `json:"run_id,omitempty"`
	TrackIDs []string `json:"track_ids"`
	StartNs  int64    `json:"start_ns"`
	EndNs    int64    `json:"end_ns"`
	Note     string   `json:"note,omitempty"`
}

// Sidecar is the revisable annotation document for exactly one pack.
type Sidecar struct {
	SchemaVersion int    `json:"schema_version"`
	DatasetID     string `json:"dataset_id"`
	// PackDigest binds these annotations to a point domain. A pack whose
	// points changed gets a different digest, and loading fails rather than
	// silently reinterpreting indices against different points.
	PackDigest string `json:"pack_digest"`
	Revision   int    `json:"revision"`
	UpdatedUTC string `json:"updated_utc"`
	// Change describes this save, separately from the provenance of each mask.
	Change Provenance `json:"change,omitempty"`
	// RestoredFrom identifies undo/redo without rewriting the old revision.
	RestoredFrom int `json:"restored_from,omitempty"`
	// baseDigest is the optimistic concurrency token obtained by loading the
	// exact saved bytes. It also catches edits made outside this writer.
	baseDigest string

	Objects         []Object              `json:"objects"`
	Masks           []FrameMask           `json:"masks"`
	Correspondences []TrackCorrespondence `json:"correspondences,omitempty"`
}

// NewSidecar starts an empty annotation document for a pack.
func NewSidecar(p *Pack) *Sidecar {
	return &Sidecar{
		SchemaVersion: SidecarSchemaVersion,
		DatasetID:     p.Manifest.DatasetID,
		PackDigest:    p.Manifest.PackDigest,
		Revision:      1,
		UpdatedUTC:    time.Now().UTC().Format(time.RFC3339),
	}
}

// Validate checks the document against its pack and its own rules. It is run
// before every write and after every read, because an invalid sidecar that
// loads is worse than one that refuses: it produces plausible numbers.
func (s *Sidecar) Validate(p *Pack) error {
	if s.Revision < 1 || s.Revision == math.MaxInt {
		return fmt.Errorf("invalid sidecar revision %d", s.Revision)
	}
	if s.SchemaVersion != SidecarSchemaVersion {
		return fmt.Errorf("sidecar schema version %d, this build reads %d", s.SchemaVersion, SidecarSchemaVersion)
	}
	if s.PackDigest != p.Manifest.PackDigest {
		return fmt.Errorf("sidecar was written against pack %s, this pack is %s",
			s.PackDigest, p.Manifest.PackDigest)
	}
	if s.DatasetID != p.Manifest.DatasetID {
		return fmt.Errorf("sidecar dataset %q does not match pack dataset %q", s.DatasetID, p.Manifest.DatasetID)
	}

	known := make(map[string]bool, len(s.Objects))
	for _, o := range s.Objects {
		if o.ObjectID == "" {
			return fmt.Errorf("an object has no id")
		}
		if known[o.ObjectID] {
			return fmt.Errorf("object %q is declared twice", o.ObjectID)
		}
		if !validStatus(o.Status) {
			return fmt.Errorf("object %q has status %q", o.ObjectID, o.Status)
		}
		known[o.ObjectID] = true
	}

	// A point may belong to at most one object in a sample. Two objects
	// claiming the same return is a conflict for a person to resolve, not
	// something to average away.
	claimed := make(map[int]map[int]string)
	seenMasks := make(map[struct {
		object string
		sample int
	}]bool)
	for i, m := range s.Masks {
		key := struct {
			object string
			sample int
		}{m.ObjectID, m.SampleID}
		if seenMasks[key] {
			return fmt.Errorf("duplicate mask for object %q sample %d", m.ObjectID, m.SampleID)
		}
		seenMasks[key] = true
		if !known[m.ObjectID] {
			return fmt.Errorf("mask %d references unknown object %q", i, m.ObjectID)
		}
		if !validStatus(m.Status) {
			return fmt.Errorf("mask %d has status %q", i, m.Status)
		}
		if !validCompleteness(m.Completeness) {
			return fmt.Errorf("mask %d has completeness %q", i, m.Completeness)
		}
		if !validVisibility(m.Visibility) {
			return fmt.Errorf("mask %d has visibility %q", i, m.Visibility)
		}
		if err := p.ValidateIndices(m.SampleID, m.PointIndices); err != nil {
			return fmt.Errorf("mask %d (%s): %w", i, m.ObjectID, err)
		}
		if err := p.ValidateIndices(m.SampleID, m.UncertainIndices); err != nil {
			return fmt.Errorf("mask %d (%s) uncertain set: %w", i, m.ObjectID, err)
		}
		// A point cannot be both claimed and doubted by the same mask.
		if overlap := intersects(m.PointIndices, m.UncertainIndices); overlap >= 0 {
			return fmt.Errorf("mask %d (%s): point %d is both a member and uncertain", i, m.ObjectID, overlap)
		}

		if claimed[m.SampleID] == nil {
			claimed[m.SampleID] = make(map[int]string)
		}
		for _, idx := range m.PointIndices {
			if other, taken := claimed[m.SampleID][idx]; taken && other != m.ObjectID {
				return fmt.Errorf("sample %d point %d is claimed by both %q and %q",
					m.SampleID, idx, other, m.ObjectID)
			}
			claimed[m.SampleID][idx] = m.ObjectID
		}
	}

	for i, c := range s.Correspondences {
		if !known[c.ObjectID] {
			return fmt.Errorf("correspondence %d references unknown object %q", i, c.ObjectID)
		}
	}
	return nil
}

func validStatus(s ReviewStatus) bool {
	return s == StatusProposed || s == StatusReviewed || s == StatusRejected
}

func validCompleteness(c MaskCompleteness) bool {
	return c == MaskComplete || c == MaskPartial || c == MaskUnreviewed
}

func validVisibility(v Visibility) bool {
	switch v {
	case VisiblePresent, VisiblePartlyOccluded, VisibleFullyOccluded, VisibleOutsideView, VisibleUnknown:
		return true
	}
	return false
}

// intersects returns the first value present in both sorted sets, or -1.
func intersects(a, b []int) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			return a[i]
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return -1
}

// Canonicalise puts the document into its one permitted encoding, so an
// unchanged annotation set round-trips byte for byte and a diff shows real
// edits rather than map ordering.
func (s *Sidecar) Canonicalise() {
	sort.Slice(s.Objects, func(i, j int) bool { return s.Objects[i].ObjectID < s.Objects[j].ObjectID })
	for i := range s.Masks {
		s.Masks[i].PointIndices = CanonicalIndices(s.Masks[i].PointIndices)
		s.Masks[i].UncertainIndices = CanonicalIndices(s.Masks[i].UncertainIndices)
		if len(s.Masks[i].UncertainIndices) == 0 {
			s.Masks[i].UncertainIndices = nil
		}
	}
	sort.Slice(s.Masks, func(i, j int) bool {
		if s.Masks[i].SampleID != s.Masks[j].SampleID {
			return s.Masks[i].SampleID < s.Masks[j].SampleID
		}
		return s.Masks[i].ObjectID < s.Masks[j].ObjectID
	})
	sort.Slice(s.Correspondences, func(i, j int) bool {
		return s.Correspondences[i].ObjectID < s.Correspondences[j].ObjectID
	})
}

// ReviewedMasks returns only the masks a person confirmed.
//
// This is the accessor an evaluation must use. Proposals are excluded here
// rather than at export, so a reload cannot promote them: the filter lives on
// the read path that scoring actually calls.
func (s *Sidecar) ReviewedMasks() []FrameMask {
	out := make([]FrameMask, 0, len(s.Masks))
	byID := make(map[string]ReviewStatus, len(s.Objects))
	for _, o := range s.Objects {
		byID[o.ObjectID] = o.Status
	}
	for _, m := range s.Masks {
		// Both the mask and its object must be reviewed. A confirmed mask on a
		// speculative object is still speculative.
		if m.Status == StatusReviewed && byID[m.ObjectID] == StatusReviewed {
			out = append(out, m)
		}
	}
	return out
}
