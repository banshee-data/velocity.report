package annotation

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Independent import of physical references.
//
// The native editor is the intended local route, not the definition of
// truth: a reference measured elsewhere may be brought in. An import file
// carries the same records the store holds, without what the store owns
// (revision, timestamps, change record, origin ledger). It is turned into a
// document and put through exactly the checks a saved document passes, then
// merged into the current document and saved as a new revision. Unknown
// fields are refused, so a field meant for a later schema fails loudly.

// PhysicalImportSchema names the import file kind.
const PhysicalImportSchema = "velocity.report/physical-reference-import"

// PhysicalImportSchemaVersion is the import layout version.
const PhysicalImportSchemaVersion = 1

// PhysicalReferenceImport is one import file.
type PhysicalReferenceImport struct {
	Schema        string               `json:"schema"`
	SchemaVersion int                  `json:"schema_version"`
	PackDigest    string               `json:"pack_digest"`
	DatasetID     string               `json:"dataset_id"`
	Source        PhysicalSource       `json:"source"`
	Objects       []PhysicalObject     `json:"objects"`
	Following     []FollowingReference `json:"following,omitempty"`
}

// LoadPhysicalImport reads and parses an import file, bounded like a sidecar.
func LoadPhysicalImport(path string) (*PhysicalReferenceImport, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open physical reference import: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxSidecarBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read physical reference import: %w", err)
	}
	if len(b) > MaxSidecarBytes {
		return nil, fmt.Errorf("physical reference import exceeds %d bytes", MaxSidecarBytes)
	}
	return ParsePhysicalImport(b)
}

// ParsePhysicalImport decodes import bytes strictly and checks the schema.
func ParsePhysicalImport(b []byte) (*PhysicalReferenceImport, error) {
	var imp PhysicalReferenceImport
	if err := decodePhysicalJSON(b, &imp, "physical reference import"); err != nil {
		return nil, err
	}
	if imp.Schema != PhysicalImportSchema {
		return nil, fmt.Errorf("import schema %q, want %q", imp.Schema, PhysicalImportSchema)
	}
	if imp.SchemaVersion != PhysicalImportSchemaVersion {
		return nil, fmt.Errorf("import schema version %d, this build reads %d", imp.SchemaVersion, PhysicalImportSchemaVersion)
	}
	return &imp, nil
}

// Document is the import as a stand-alone document at revision 1, with its
// origin ledger taken from its own records. It is for display and digests:
// an import may add a keyframe to a body already stored, so it is validated
// merged with the stored references, as it would be saved
// (PreparePhysicalImport), never on its own.
func (imp *PhysicalReferenceImport) Document() *PhysicalReferenceSet {
	r := &PhysicalReferenceSet{
		Schema: PhysicalReferenceSchema, SchemaVersion: PhysicalReferenceSchemaVersion,
		DatasetID: imp.DatasetID, PackDigest: imp.PackDigest, Revision: 1,
		Source: imp.Source, Objects: clonePhysicalObjects(imp.Objects), Following: cloneFollowing(imp.Following),
	}
	r.RecordOrigins = r.currentOrigins()
	r.canonicalise()
	return r
}

// MergeInto adds the import's records to a loaded document. A body, a
// keyframe at a sample, or a following reference the document already has is
// refused unless replace is set, in which case the imported record replaces
// it. The merged document is not saved and not yet validated as a whole.
func (imp *PhysicalReferenceImport) MergeInto(doc *PhysicalReferenceSet, replace bool) error {
	if imp.PackDigest != doc.PackDigest || imp.DatasetID != doc.DatasetID {
		return fmt.Errorf("import was written against pack %s (dataset %q); these references are for %s (dataset %q)",
			imp.PackDigest, imp.DatasetID, doc.PackDigest, doc.DatasetID)
	}
	// A document that has never held a record takes the import's source; the
	// merged document is then held to the pack like any other. Its
	// calibration identity is the one thing the pack cannot supply.
	if doc.baseDigest == "" && len(doc.Objects) == 0 && len(doc.Following) == 0 {
		doc.Source = imp.Source
	}
	if imp.Source != doc.Source {
		return fmt.Errorf("import source %+v differs from the document's %+v", imp.Source, doc.Source)
	}
	var conflicts []string
	for _, in := range clonePhysicalObjects(imp.Objects) {
		i := indexOfObject(doc.Objects, in.ObjectID)
		if i < 0 {
			doc.Objects = append(doc.Objects, in)
			continue
		}
		have := &doc.Objects[i]
		if in.Body != nil {
			if have.Body != nil && !replace {
				conflicts = append(conflicts, fmt.Sprintf("object %s already has body %s", in.ObjectID, have.Body.BodyID))
			} else {
				have.Body = in.Body
			}
		}
		for _, k := range in.Keyframes {
			j := indexOfKeyframe(have.Keyframes, k.SampleID)
			switch {
			case j < 0:
				have.Keyframes = append(have.Keyframes, k)
			case replace:
				have.Keyframes[j] = k
			default:
				conflicts = append(conflicts, fmt.Sprintf("object %s already has keyframe %s at sample %d",
					in.ObjectID, have.Keyframes[j].KeyframeID, k.SampleID))
			}
		}
	}
	for _, f := range cloneFollowing(imp.Following) {
		j := -1
		for i := range doc.Following {
			if doc.Following[i].FollowingID == f.FollowingID {
				j = i
			}
		}
		switch {
		case j < 0:
			doc.Following = append(doc.Following, f)
		case replace:
			doc.Following[j] = f
		default:
			conflicts = append(conflicts, fmt.Sprintf("following %s already exists", f.FollowingID))
		}
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("import would replace existing records (use replace to allow it): %s", strings.Join(conflicts, "; "))
	}
	return nil
}

// PreparePhysicalImport is an import without the write: it merges the import
// into the current references and puts the merged document through every
// check a save makes against the current state (the whole document against
// the pack, the stored origin ledger, and the links against the current
// sidecar). ImportPhysicalReferences saves what it returns, and the save
// repeats those checks under the lock.
func PreparePhysicalImport(p *Pack, imp *PhysicalReferenceImport, replace bool) (*PhysicalReferenceSet, error) {
	sidecar, err := LoadSidecar(p)
	if err != nil {
		return nil, fmt.Errorf("load annotation: %w", err)
	}
	doc, err := LoadPhysicalReferences(p)
	if err != nil {
		return nil, err
	}
	if err := imp.MergeInto(doc, replace); err != nil {
		return nil, err
	}
	merged := *doc
	merged.Objects, merged.Following = clonePhysicalObjects(doc.Objects), cloneFollowing(doc.Following)
	merged.RecordOrigins = make(map[string]ReferenceOrigin, len(doc.RecordOrigins))
	for key, origin := range doc.RecordOrigins {
		merged.RecordOrigins[key] = origin
	}
	if err := mergeOrigins(merged.RecordOrigins, merged.currentOrigins()); err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	merged.canonicalise()
	if err := merged.Validate(p); err != nil {
		return nil, fmt.Errorf("import merged with the stored references: %w", err)
	}
	if err := merged.ValidateLinks(p, sidecar); err != nil {
		return nil, fmt.Errorf("import merged with the stored references: %w", err)
	}
	return doc, nil
}

// ImportPhysicalReferences prepares an import as PreparePhysicalImport does
// and saves the result as a new revision. change names who imported it and
// from where; each record keeps its own review and provenance.
func ImportPhysicalReferences(p *Pack, imp *PhysicalReferenceImport, change Provenance, replace bool) (*PhysicalReferenceSet, error) {
	if strings.TrimSpace(change.Author) == "" {
		return nil, fmt.Errorf("an import must name who ran it")
	}
	doc, err := PreparePhysicalImport(p, imp, replace)
	if err != nil {
		return nil, err
	}
	if change.Operation == "" {
		change.Operation = "import"
	}
	doc.Change = change
	if err := SavePhysicalReferences(p, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func indexOfObject(objects []PhysicalObject, id string) int {
	for i := range objects {
		if objects[i].ObjectID == id {
			return i
		}
	}
	return -1
}

func indexOfKeyframe(keyframes []PhysicalKeyframe, sampleID int) int {
	for i := range keyframes {
		if keyframes[i].SampleID == sampleID {
			return i
		}
	}
	return -1
}
