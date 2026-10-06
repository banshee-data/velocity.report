package scene

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validVantage() Vantage {
	return Vantage{ID: "eb-howard", Label: "Eastbound Howard", AzimuthDeg: 270, PolarDeg: 72, Zoom: 0.7}
}

func TestVantageValidation(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Vantage)
		problem string // "" means accepted
	}{
		{"accepts a street-named vantage", func(v *Vantage) {}, ""},
		{"accepts offsets", func(v *Vantage) { v.OffsetX, v.OffsetY = 8, -3 }, ""},
		{"rejects an empty id", func(v *Vantage) { v.ID = "" }, "must be lowercase"},
		{"rejects uppercase", func(v *Vantage) { v.ID = "EbHoward" }, "must be lowercase"},
		{"rejects a space in the id", func(v *Vantage) { v.ID = "eb howard" }, "must be lowercase"},
		{"rejects a blank label", func(v *Vantage) { v.Label = "  " }, "needs a label"},
		{"rejects an angle past the ground", func(v *Vantage) { v.PolarDeg = 91 }, "angle must be"},
		{"rejects a negative angle", func(v *Vantage) { v.PolarDeg = -1 }, "angle must be"},
		{"rejects zero zoom", func(v *Vantage) { v.Zoom = 0 }, ""}, // normalised to 1
		{"rejects absurd zoom", func(v *Vantage) { v.Zoom = 9 }, "zoom must be"},
		{"rejects an offset off the map", func(v *Vantage) { v.OffsetX = 5000 }, "within 500 m"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := validVantage()
			c.mutate(&v)
			v.Normalise()
			problem := v.Validate()
			if c.problem == "" && problem != "" {
				t.Fatalf("rejected with %q, want acceptance", problem)
			}
			if c.problem != "" && !strings.Contains(problem, c.problem) {
				t.Errorf("problem %q does not mention %q", problem, c.problem)
			}
		})
	}
}

// A hand-edited bearing of -90 or 450 means what the person meant, not an error.
func TestVantageNormalisesBearing(t *testing.T) {
	for in, want := range map[float64]float64{-90: 270, 450: 90, 360: 0, 45: 45} {
		v := validVantage()
		v.AzimuthDeg = in
		v.Normalise()
		if v.AzimuthDeg != want {
			t.Errorf("bearing %g normalised to %g, want %g", in, v.AzimuthDeg, want)
		}
	}
	v := validVantage()
	v.Zoom = 0
	v.Normalise()
	if v.Zoom != 1 {
		t.Errorf("a missing zoom should default to 1, got %g", v.Zoom)
	}
}

// A duplicate id would make one chip unreachable in the viewer.
func TestVantagesRejectDuplicateIDs(t *testing.T) {
	a, b := validVantage(), validVantage()
	b.Label = "Westbound Howard"
	if problem := ValidateVantages([]Vantage{a, b}); !strings.Contains(problem, "more than once") {
		t.Errorf("duplicate ids accepted: %q", problem)
	}
}

func TestLoadVantagesAcceptsArrayOrWrapper(t *testing.T) {
	dir := t.TempDir()
	list := []Vantage{validVantage()}

	arrayPath := filepath.Join(dir, "array.json")
	raw, _ := json.Marshal(list)
	if err := os.WriteFile(arrayPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadVantages(arrayPath); err != nil || len(got) != 1 {
		t.Errorf("bare array: got %v, err %v", got, err)
	}

	wrapPath := filepath.Join(dir, "wrapped.json")
	wrapped, _ := json.Marshal(map[string]any{"vantages": list})
	if err := os.WriteFile(wrapPath, wrapped, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadVantages(wrapPath); err != nil || len(got) != 1 {
		t.Errorf("wrapped: got %v, err %v", got, err)
	}

	badPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPath, []byte(`{"nope":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadVantages(badPath); err == nil {
		t.Error("an object with no vantages key should not load silently")
	}
}

// Vantages belong in exactly one file, so these guard against a second copy
// coming back. A second copy is one that drifts, and the viewer would then
// have to guess which one the publisher meant — which is precisely the bug
// that made an edited vantages.json look like it did nothing.
func TestExportWritesNoVantagesAnywhere(t *testing.T) {
	src := writeVRLOG(t, evenTimestamps(12))
	out := filepath.Join(t.TempDir(), "out")
	if _, err := Export(Options{VRLOGPath: src, OutDir: out, Site: "s", Title: "T"}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	err := filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		// Chunk files are gzipped frame data; only the plain JSON siblings
		// could plausibly carry configuration.
		if !strings.HasSuffix(path, ".json") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(bytes.ToLower(raw), []byte("vantage")) {
			rel, _ := filepath.Rel(out, path)
			t.Errorf("%s carries vantages; %s at the scene root is the only place they belong",
				rel, VantagesFile)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecordingCarriesNoVantages(t *testing.T) {
	// A VRLOG records what the sensor measured. A viewpoint is a presentation
	// choice about a place, so baking one into the recording would put a
	// second, staler copy inside the file the project calls its source of
	// truth.
	src := writeVRLOG(t, evenTimestamps(6))
	raw, err := os.ReadFile(filepath.Join(src, "header.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bytes.ToLower(raw), []byte("vantage")) {
		t.Errorf("the VRLOG header carries vantages:\n%s", raw)
	}
}

func TestVantagesFileIsNamedOnce(t *testing.T) {
	// The viewer fetches this name and the CLI reports it. Renaming it in one
	// place and not the other is the kind of drift the constant exists to stop.
	if VantagesFile != "vantages.json" {
		t.Errorf("VantagesFile = %q; the published viewer fetches vantages.json", VantagesFile)
	}
}
