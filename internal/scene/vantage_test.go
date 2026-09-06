package scene

import (
	"encoding/json"
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

// Precedence: a publisher's override beats the recording, which beats defaults.
func TestResolveVantagesPrecedence(t *testing.T) {
	override := []Vantage{{ID: "custom", Label: "Custom", PolarDeg: 60, Zoom: 1}}
	recorded, _ := json.Marshal([]Vantage{{ID: "recorded", Label: "Recorded", PolarDeg: 60, Zoom: 1}})

	if got := resolveVantages(override, recorded); got[0].ID != "custom" {
		t.Errorf("override should win, got %q", got[0].ID)
	}
	if got := resolveVantages(nil, recorded); got[0].ID != "recorded" {
		t.Errorf("the recording should be used when there is no override, got %q", got[0].ID)
	}
	if got := resolveVantages(nil, nil); len(got) != len(DefaultVantages()) {
		t.Errorf("a scene naming none should fall back to compass defaults, got %d", len(got))
	}
	// A corrupt list in the recording must not publish a broken viewer.
	if got := resolveVantages(nil, json.RawMessage(`[{"id":"BAD ID","label":""}]`)); got[0].ID != "overview" {
		t.Errorf("an invalid recorded list should fall back, got %q", got[0].ID)
	}
}

func TestExportCarriesVantages(t *testing.T) {
	src := writeVRLOG(t, evenTimestamps(10))
	out := filepath.Join(t.TempDir(), "out")
	want := []Vantage{
		{ID: "eb-howard", Label: "Eastbound Howard", AzimuthDeg: 270, PolarDeg: 72, Zoom: 0.7, OffsetX: 8},
	}
	res, err := Export(Options{VRLOGPath: src, OutDir: out, Vantages: want})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(res.Header.Vantages) != 1 || res.Header.Vantages[0].Label != "Eastbound Howard" {
		t.Fatalf("export header vantages = %+v", res.Header.Vantages)
	}

	h := readHeader(t, out)
	if len(h.Vantages) != 1 || h.Vantages[0].OffsetX != 8 {
		t.Errorf("vantages did not survive the round trip: %+v", h.Vantages)
	}
}
