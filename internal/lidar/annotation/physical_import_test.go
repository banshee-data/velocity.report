package annotation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// importOf is the valid document as an import file's records.
func importOf(p *Pack) *PhysicalReferenceImport {
	r := validPhysical(p)
	return &PhysicalReferenceImport{
		Schema: PhysicalImportSchema, SchemaVersion: PhysicalImportSchemaVersion,
		PackDigest: r.PackDigest, DatasetID: r.DatasetID, Source: r.Source,
		Objects: r.Objects, Following: r.Following,
	}
}

func writeImport(t *testing.T, imp *PhysicalReferenceImport) string {
	t.Helper()
	b, err := json.MarshalIndent(imp, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "import.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Completion evidence for P0: an independent import passes the same
// validation, and what it imports round-trips.
func TestPhysicalImportRoundTrip(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	sidecarBefore := readSaved(t, p)
	imp, err := LoadPhysicalImport(writeImport(t, importOf(p)))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ImportPhysicalReferences(p, imp, Provenance{Author: "surveyor", Session: "s1"}, false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if doc.Revision != 1 || doc.Change.Operation != "import" || doc.Change.Author != "surveyor" {
		t.Fatalf("import revision metadata: %d %+v", doc.Revision, doc.Change)
	}
	loaded, err := LoadPhysicalReferences(p)
	if err != nil {
		t.Fatal(err)
	}
	want := validPhysical(p)
	if !reflect.DeepEqual(loaded.Objects, want.Objects) || !reflect.DeepEqual(loaded.Following, want.Following) ||
		loaded.Source.CalibrationID != "calibration/v1/test" {
		t.Fatalf("imported records did not round-trip: %+v", loaded.Objects)
	}
	if want, _ := importOf(p).Document().ContentDigest(); true {
		if got, _ := loaded.ContentDigest(); got != want {
			t.Fatalf("stored content %s differs from the import's %s", got, want)
		}
	}
	if string(sidecarBefore) != string(readSaved(t, p)) {
		t.Fatal("an import changed the membership sidecar")
	}

	// The same records again collide; with replace they are a new revision
	// of the same content.
	if _, err := ImportPhysicalReferences(p, imp, Provenance{Author: "surveyor"}, false); err == nil ||
		!strings.Contains(err.Error(), "would replace existing records") {
		t.Fatalf("a re-import silently replaced records: %v", err)
	}
	again, err := ImportPhysicalReferences(p, imp, Provenance{Author: "surveyor", Operation: "reimport"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := again.ContentDigest(); again.Revision != 2 || again.Change.Operation != "reimport" {
		t.Fatalf("replace: revision %d %q %s", again.Revision, again.Change.Operation, a)
	}

	// New records merge beside existing ones.
	extra := importOf(p)
	extra.Objects = []PhysicalObject{{ObjectID: "car-1", Keyframes: []PhysicalKeyframe{validPhysical(p).Objects[0].Keyframes[0]}}}
	extra.Objects[0].Keyframes[0].KeyframeID = "kf-car-1-s4"
	extra.Objects[0].Keyframes[0].SampleID, extra.Objects[0].Keyframes[0].TimestampNs = 4, physTime(4)
	*extra.Objects[0].Keyframes[0].Position.XM = 14
	extra.Objects[0].Keyframes[0].Position.Support = frames(4)
	extra.Objects[0].Keyframes[0].Yaw.Support = frames(4)
	extra.Objects[0].Keyframes[0].Front.Support = frames(4)
	extra.Objects[0].Keyframes[0].Rear.Support = frames(4)
	extra.Following = nil
	merged, err := ImportPhysicalReferences(p, extra, Provenance{Author: "surveyor"}, false)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if _, ok := merged.Keyframe("car-1", 4); !ok || len(merged.Objects[0].Keyframes) != 4 {
		t.Fatalf("merged keyframes: %+v", merged.Objects[0].Keyframes)
	}
	// A body import for an object that has one is a replacement.
	bodyOnly := importOf(p)
	bodyOnly.Objects = []PhysicalObject{{ObjectID: "car-2", Body: validPhysical(p).Objects[1].Body}}
	bodyOnly.Following = nil
	if _, err := ImportPhysicalReferences(p, bodyOnly, Provenance{Author: "surveyor"}, false); err == nil {
		t.Fatal("a second body replaced the first without replace")
	}
	// A calibration the document does not share is another source.
	other := importOf(p)
	other.Source.CalibrationID = "calibration/v2/test"
	if _, err := ImportPhysicalReferences(p, other, Provenance{Author: "surveyor"}, true); err == nil ||
		!strings.Contains(err.Error(), "import source") {
		t.Fatalf("an import from another calibration merged: %v", err)
	}
}

// The import is refused for exactly the reasons a saved document is, and a
// refused import writes nothing.
func TestPhysicalImportRefusesWhatTheStoreRefuses(t *testing.T) {
	p := physPack(t)
	physSidecar(t, p)
	cases := []struct {
		name   string
		mutate func(*PhysicalReferenceImport)
		want   string
	}{
		{"frame not in the pack", func(i *PhysicalReferenceImport) { i.Objects[0].Keyframes[0].SampleID = 40 }, "sample 40 is not in the pack"},
		{"pack digest mismatch", func(i *PhysicalReferenceImport) { i.PackDigest = "sha256:elsewhere" }, "written against pack"},
		{"source digest mismatch", func(i *PhysicalReferenceImport) { i.Source.VRLOGFramesSHA = "sha256:elsewhere" }, "vrlog_frames_sha256"},
		{"observed without frames", func(i *PhysicalReferenceImport) {
			i.Objects[0].Body.Length.Support = EvidenceSupport{External: "tape"}
		}, "names no supporting frames"},
		{"tracker-assisted as independent", func(i *PhysicalReferenceImport) {
			i.Objects[0].Keyframes[2].Review.Origin = OriginIndependent
		}, "cannot claim independent provenance"},
		{"object not in the annotation", func(i *PhysicalReferenceImport) {
			i.Objects = append(i.Objects, PhysicalObject{ObjectID: "car-7", Keyframes: []PhysicalKeyframe{}})
		}, "declare it there first"},
		{"partial span as full", func(i *PhysicalReferenceImport) { i.Objects[0].Body.Length.Support = frames(3) }, "a partial span supports only a lower bound"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			imp := importOf(p)
			c.mutate(imp)
			parsed, err := LoadPhysicalImport(writeImport(t, imp))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ImportPhysicalReferences(p, parsed, Provenance{Author: "surveyor"}, false); err == nil ||
				!strings.Contains(err.Error(), c.want) {
				t.Fatalf("refused for the wrong reason: %v (want %q)", err, c.want)
			}
			if _, err := os.Stat(filepath.Join(p.Dir, physicalReferenceFile)); !os.IsNotExist(err) {
				t.Fatal("a refused import wrote references")
			}
		})
	}
	if _, err := ImportPhysicalReferences(p, importOf(p), Provenance{}, false); err == nil {
		t.Fatal("an anonymous import ran")
	}
}

func TestPhysicalImportParsing(t *testing.T) {
	p := physPack(t)
	valid, err := json.Marshal(importOf(p))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePhysicalImport(valid); err != nil {
		t.Fatalf("valid import refused: %v", err)
	}
	for name, b := range map[string]string{
		"unknown field":  strings.Replace(string(valid), `"objects"`, `"objcts":[],"objects"`, 1),
		"trailing data":  string(valid) + "{}",
		"wrong schema":   strings.Replace(string(valid), PhysicalImportSchema, "velocity.report/other", 1),
		"future version": strings.Replace(string(valid), `"schema_version":1`, `"schema_version":2`, 1),
		"not json":       "{",
	} {
		if _, err := ParsePhysicalImport([]byte(b)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
	if _, err := LoadPhysicalImport(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing import loaded")
	}
	big := filepath.Join(t.TempDir(), "big.json")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxSidecarBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := LoadPhysicalImport(big); err == nil {
		t.Fatal("oversized import loaded")
	}
	dir := t.TempDir()
	if _, err := LoadPhysicalImport(dir); err == nil {
		t.Fatal("a directory loaded as an import")
	}
}

func TestPhysicalImportDependsOnReadableState(t *testing.T) {
	t.Run("damaged sidecar", func(t *testing.T) {
		p := physPack(t)
		if err := os.WriteFile(filepath.Join(p.Dir, sidecarFile), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ImportPhysicalReferences(p, importOf(p), Provenance{Author: "a"}, false); err == nil {
			t.Fatal("imported against a damaged sidecar")
		}
	})
	t.Run("damaged references", func(t *testing.T) {
		p := physPack(t)
		physSidecar(t, p)
		if err := os.WriteFile(filepath.Join(p.Dir, physicalReferenceFile), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ImportPhysicalReferences(p, importOf(p), Provenance{Author: "a"}, false); err == nil {
			t.Fatal("imported over damaged references")
		}
	})
	t.Run("another pack", func(t *testing.T) {
		p := physPack(t)
		imp := importOf(p)
		doc := NewPhysicalReferenceSet(p)
		imp.DatasetID = "ds_other"
		if err := imp.MergeInto(doc, false); err == nil {
			t.Fatal("merged another dataset's import")
		}
	})
	t.Run("ledger conflict on replace", func(t *testing.T) {
		p := physPack(t)
		physSidecar(t, p)
		if _, err := ImportPhysicalReferences(p, importOf(p), Provenance{Author: "a"}, false); err != nil {
			t.Fatal(err)
		}
		// The tracker-assisted proposal, replaced by an import that calls
		// the same record independent.
		imp := importOf(p)
		k := &imp.Objects[0].Keyframes[2]
		k.Review = independentReview()
		imp.Objects = []PhysicalObject{{ObjectID: "car-1", Keyframes: []PhysicalKeyframe{*k}}}
		imp.Following = nil
		if _, err := ImportPhysicalReferences(p, imp, Provenance{Author: "a"}, true); err == nil ||
			!strings.Contains(err.Error(), "cannot become independent") {
			t.Fatalf("an import laundered a tracker-assisted record: %v", err)
		}
	})
}
