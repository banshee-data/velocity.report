package annotation

import (
	"errors"
	"strings"
)

// Where a physical-reference refusal falls. The validator and the link
// checker stop at, or list, records that do not hold; a client that edits
// those records needs to show each refusal beside the control that sets the
// field it names, not at the top of a column of thirty. The location is
// carried on the error chain by wrappers that add no text, so every message
// reads exactly as it did and the rules are unchanged: a location only says
// where a refusal already made belongs.

// physicalRecordFollowing names a following record in a location. Following
// records are not reviewed through PhysicalRecordKind, so it is not one.
const physicalRecordFollowing = "following"

// PhysicalLocation locates a refusal in a physical reference document: the
// object and record it falls in, and the field within the record, in the
// record's own JSON names joined by dots ("length", "position.bound_m",
// "review.uncertainty_assumptions"). Detail is the refusal without the
// location prefixes the message carries, for display beside that field. A
// document-level refusal has no location.
type PhysicalLocation struct {
	ObjectID string `json:"object_id,omitempty"`
	// Record is "body", "keyframe" or "following", empty for a refusal of
	// the object as a whole.
	Record   string `json:"record,omitempty"`
	RecordID string `json:"record_id,omitempty"`
	// SampleID is a keyframe's sample.
	SampleID *int `json:"sample_id,omitempty"`
	// Field is empty for a refusal of the record as a whole, such as its id.
	Field  string `json:"field,omitempty"`
	Detail string `json:"detail"`
}

// locatedError adds one level of location to an error without changing its
// text.
type locatedError struct {
	objectID string
	record   string
	recordID string
	sampleID *int
	field    string
	err      error
}

func (e *locatedError) Error() string { return e.err.Error() }
func (e *locatedError) Unwrap() error { return e.err }

// atField names the field an error is about. Nested calls join outermost
// first: atField("position", atField("bound_m", err)) is "position.bound_m".
func atField(field string, err error) error {
	if err == nil {
		return nil
	}
	return &locatedError{field: field, err: err}
}

// inRecord names the record an error is about.
func inRecord(record, id string, sample *int, err error) error {
	if err == nil {
		return nil
	}
	return &locatedError{record: record, recordID: id, sampleID: sample, err: err}
}

// inObject names the object an error is about.
func inObject(id string, err error) error {
	if err == nil {
		return nil
	}
	return &locatedError{objectID: id, err: err}
}

// LocatePhysicalError reads the location a validation or link refusal
// carries, or nil when it has none: a document-level refusal, or an error
// that did not come from the physical-reference rules.
func LocatePhysicalError(err error) *PhysicalLocation {
	var loc PhysicalLocation
	var fields []string
	found := false
	for e := err; e != nil; e = errors.Unwrap(e) {
		l, ok := e.(*locatedError)
		if !ok {
			continue
		}
		found = true
		if loc.ObjectID == "" {
			loc.ObjectID = l.objectID
		}
		if loc.Record == "" && l.record != "" {
			loc.Record, loc.RecordID = l.record, l.recordID
			if l.sampleID != nil {
				s := *l.sampleID
				loc.SampleID = &s
			}
		}
		if l.field != "" {
			fields = append(fields, l.field)
		}
		// The innermost level wins: below it is only the rule's own text.
		loc.Detail = l.err.Error()
	}
	if !found {
		return nil
	}
	loc.Field = strings.Join(fields, ".")
	return &loc
}
