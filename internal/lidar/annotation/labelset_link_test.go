package annotation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLabelSetCapturesBindCasesByContent(t *testing.T) {
	base := t.TempDir()
	packDir := filepath.Join(base, "pack")
	setDir := filepath.Join(base, "labels", "kirk0-reference")
	os.MkdirAll(packDir, 0o755)
	os.MkdirAll(setDir, 0o755)
	if got, err := LabelSetCaptures(packDir); err != nil || got != nil {
		t.Fatalf("unlinked pack: %v %v", got, err)
	}
	os.WriteFile(filepath.Join(packDir, LabelSetLinkFile), []byte(`{"label_set_id":"ls-1","path":"../labels/kirk0-reference"}`), 0o644)
	os.WriteFile(filepath.Join(setDir, "labels.json"), []byte(`{"label_set_id":"ls-1","applies_to":[
		{"name":"kirk0.pcapng","sha256":"sha256:aa"},{"name":"kirk0-clip.pcapng","sha256":"sha256:bb"}]}`), 0o644)
	captures, err := LabelSetCaptures(packDir)
	if err != nil || len(captures) != 2 {
		t.Fatalf("captures %v %v", captures, err)
	}

	f := &FrozenSplit{
		Cases: []SplitCase{{CaseID: "kirk0", Captures: []CaseCapture{{Basename: "kirk0.pcapng"}}}},
		Packs: []FrozenPack{{CaseID: "kirk0", Source: FrozenSource{PCAPBasename: "kirk0.pcapng"}, Captures: captures}},
	}
	f.deriveCaseCaptures()
	got := f.Cases[0].Captures
	if len(got) != 2 || got[0].SHA256 == "" || got[1].SHA256 == "" {
		t.Fatalf("case captures %+v", got)
	}

	os.WriteFile(filepath.Join(setDir, "labels.json"), []byte(`{"label_set_id":"ls-2","applies_to":[]}`), 0o644)
	if _, err := LabelSetCaptures(packDir); err == nil {
		t.Fatal("a link to the wrong label set was followed")
	}
}
