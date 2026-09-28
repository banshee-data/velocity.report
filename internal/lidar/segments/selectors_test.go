package segments

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// shippedSelectorFile is the repository's selector file, read from this
// package's directory.
var shippedSelectorFile = filepath.Join("..", "..", "..", DefaultSelectorsPath)

func shippedDocument(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(shippedSelectorFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func selectorIn(doc map[string]any, id string) map[string]any {
	for _, s := range doc["selectors"].([]any) {
		if entry := s.(map[string]any); entry["id"] == id {
			return entry
		}
	}
	panic("no selector " + id)
}

func shippedCatalogue(t *testing.T) *Catalogue {
	t.Helper()
	c, err := LoadSelectors(shippedSelectorFile)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func selectorFrom(t *testing.T, c *Catalogue, id string) Selector {
	t.Helper()
	s, ok := c.Selector(id)
	if !ok {
		t.Fatalf("no selector %q", id)
	}
	return s
}

func TestTheShippedSelectorFileParses(t *testing.T) {
	c := shippedCatalogue(t)
	ids := []string{}
	for _, s := range c.Selectors {
		ids = append(ids, s.ID)
	}
	if want := []string{"following", "close_following", "exposure", "random", "leader_changes", "lateral_jump", "split_flags"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("selectors %v, want %v in file order", ids, want)
	}
	if c.Version != SelectorsVersion || c.Source != filepath.Clean(shippedSelectorFile) || !strings.HasPrefix(c.Digest, "sha256:") || len(c.Digest) != 71 {
		t.Fatalf("catalogue provenance: version %d, source %q, digest %q", c.Version, c.Source, c.Digest)
	}
	heldOut := []string{}
	for _, s := range c.ForRole("held_out") {
		heldOut = append(heldOut, s.ID)
	}
	if want := []string{"following", "exposure", "random"}; !reflect.DeepEqual(heldOut, want) {
		t.Fatalf("held-out selectors %v, want %v", heldOut, want)
	}
	if len(c.ForRole("tuning")) != len(c.Selectors) || len(c.ForRole("validation")) != 0 {
		t.Fatal("every selector chooses tuning windows, and no other role has any")
	}
	if _, ok := c.Selector("tailgating"); ok {
		t.Fatal("found a selector the file does not define")
	}
	closer := selectorFrom(t, c, "close_following")
	p := DefaultParams()
	p.MaxGap = 15
	if closer.Parameters != p || closer.Score != (Score{"pair_seconds", "descending"}) || len(closer.Require) != 1 ||
		closer.Require[0].Measure != "pair_seconds" || *closer.Require[0].Min != 2 || closer.Require[0].Max != nil {
		t.Fatalf("close_following as read: %+v", closer)
	}
}

func TestTheSelectorFileIsStrict(t *testing.T) {
	cases := []struct {
		name   string
		change func(doc map[string]any)
		want   string
	}{
		{"not an object", func(doc map[string]any) { doc["version"] = "one" }, "cannot unmarshal"},
		{"an unknown key", func(doc map[string]any) { doc["comment"] = "x" }, "selectors file: unknown keys: comment"},
		{"a missing key", func(doc map[string]any) { delete(doc, "selectors") }, "missing required keys: selectors"},
		{"another version", func(doc map[string]any) { doc["version"] = 2 }, "version must be 1, not 2"},
		{"no selectors", func(doc map[string]any) { doc["selectors"] = []any{} }, "between 1 and 100 selectors, not 0"},
		{"too many selectors", func(doc map[string]any) {
			many := doc["selectors"].([]any)
			for len(many) <= maxSelectors {
				many = append(many, many[0])
			}
			doc["selectors"] = many
		}, "not 101"},
		{"an unknown selector key", func(doc map[string]any) { selectorIn(doc, "close_following")["weight"] = 1 }, "selectors[1]: unknown keys: weight"},
		{"a missing selector key", func(doc map[string]any) { delete(selectorIn(doc, "close_following"), "description") }, "missing required keys: description"},
		{"an unknown finder", func(doc map[string]any) { selectorIn(doc, "close_following")["finder"] = "tailgating" }, `(close_following): unknown finder "tailgating"`},
		{"an id with capitals", func(doc map[string]any) { selectorIn(doc, "close_following")["id"] = "Close_Following" }, "must be lower case letters"},
		{"no id", func(doc map[string]any) { selectorIn(doc, "close_following")["id"] = "" }, `selectors[1]: id "" must be lower case`},
		{"an id used twice", func(doc map[string]any) {
			doc["selectors"] = append(doc["selectors"].([]any), selectorIn(doc, "close_following"))
		}, `id "close_following" is used twice`},
		{"a label used twice", func(doc map[string]any) { selectorIn(doc, "close_following")["label"] = " vehicles FOLLOWING" }, `label " vehicles FOLLOWING" is already following's`},
		{"a blank label", func(doc map[string]any) { selectorIn(doc, "close_following")["label"] = "  " }, "label must be given"},
		{"a long category", func(doc map[string]any) { selectorIn(doc, "close_following")["category"] = strings.Repeat("c", 41) }, "category must be given, in at most 40"},
		{"a parameter missing", func(doc map[string]any) {
			delete(selectorIn(doc, "close_following")["parameters"].(map[string]any), "max_gap")
		}, "parameters: missing required keys: max_gap"},
		{"a parameter the finder ignores", func(doc map[string]any) {
			selectorIn(doc, "close_following")["parameters"].(map[string]any)["jump_threshold"] = 1
		}, "this finder does not read jump_threshold"},
		{"a null parameter", func(doc map[string]any) {
			selectorIn(doc, "close_following")["parameters"].(map[string]any)["min_gap"] = nil
		}, "min_gap is null"},
		{"a parameter of the wrong type", func(doc map[string]any) {
			selectorIn(doc, "close_following")["parameters"].(map[string]any)["min_gap"] = "3"
		}, "cannot unmarshal string"},
		{"parameters out of range", func(doc map[string]any) {
			selectorIn(doc, "close_following")["parameters"].(map[string]any)["max_gap"] = 1
		}, "invalid finder parameters"},
		{"parameters that are not an object", func(doc map[string]any) { selectorIn(doc, "close_following")["parameters"] = []any{} }, "parameters: expected an object"},
		{"an unknown measure", func(doc map[string]any) {
			selectorIn(doc, "close_following")["score"] = map[string]any{"measure": "speed", "order": "descending"}
		}, `score: unknown measure "speed"`},
		{"a measure the finder does not compute", func(doc map[string]any) {
			selectorIn(doc, "close_following")["score"] = map[string]any{"measure": "events", "order": "descending"}
		}, "score: the following finder does not compute events"},
		{"an unknown order", func(doc map[string]any) {
			selectorIn(doc, "close_following")["score"] = map[string]any{"measure": "pairs", "order": "largest"}
		}, `order must be descending or ascending, not "largest"`},
		{"a weight on the score", func(doc map[string]any) {
			selectorIn(doc, "close_following")["score"] = map[string]any{"measure": "pairs", "order": "descending", "weight": 2}
		}, "score: unknown keys: weight"},
		{"no requirement list", func(doc map[string]any) { selectorIn(doc, "close_following")["require"] = nil }, "require: expected a list"},
		{"a requirement without max", func(doc map[string]any) {
			selectorIn(doc, "close_following")["require"] = []any{map[string]any{"measure": "pairs", "min": 1}}
		}, "require[0]: missing required keys: max"},
		{"a requirement with no bound", func(doc map[string]any) {
			selectorIn(doc, "close_following")["require"] = []any{map[string]any{"measure": "pairs", "min": nil, "max": nil}}
		}, "pairs has neither a min nor a max"},
		{"a requirement upside down", func(doc map[string]any) {
			selectorIn(doc, "close_following")["require"] = []any{map[string]any{"measure": "pairs", "min": 3, "max": 2}}
		}, "pairs has min above max"},
		{"a measure bounded twice", func(doc map[string]any) {
			bound := map[string]any{"measure": "pairs", "min": 1, "max": nil}
			selectorIn(doc, "close_following")["require"] = []any{bound, bound}
		}, "pairs is bounded twice"},
		{"a requirement on an unknown measure", func(doc map[string]any) {
			selectorIn(doc, "close_following")["require"] = []any{map[string]any{"measure": "speed", "min": 1, "max": nil}}
		}, `require: unknown measure "speed"`},
		{"a standard selector redefined", func(doc map[string]any) {
			selectorIn(doc, "following")["parameters"].(map[string]any)["max_gap"] = 15
		}, `"following" is a standard selector, so it must keep the standard definition`},
		{"a standard name on another finder", func(doc map[string]any) {
			random := selectorIn(doc, "random")
			exposure := selectorIn(doc, "exposure")
			for _, key := range []string{"finder", "parameters", "score"} {
				random[key] = exposure[key]
			}
		}, `"random" is a standard selector`},
		{"a standard selector missing", func(doc map[string]any) {
			kept := []any{}
			for _, s := range doc["selectors"].([]any) {
				if s.(map[string]any)["id"] != "split_flags" {
					kept = append(kept, s)
				}
			}
			doc["selectors"] = kept
		}, `the standard selector "split_flags" is missing`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := shippedDocument(t)
			tc.change(doc)
			b, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseSelectors(b); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error containing %q", err, tc.want)
			}
		})
	}
	if _, err := ParseSelectors([]byte(`[]`)); err == nil {
		t.Fatal("accepted a file that is not an object")
	}
}

func TestOnlyAStandardDefinitionChoosesHeldOutWindows(t *testing.T) {
	c := shippedCatalogue(t)
	for id, want := range map[string]bool{
		"following": true, "exposure": true, "random": true,
		"close_following": false, "leader_changes": false, "lateral_jump": false, "split_flags": false,
	} {
		if got := selectorFrom(t, c, id).HeldOut(); got != want {
			t.Errorf("%s: HeldOut %t, want %t", id, got, want)
		}
	}
	// Each of these can steer a traffic measure towards tracker failure.
	following := selectorFrom(t, c, "following")
	for name, change := range map[string]func(*Selector){
		"a wider window":           func(s *Selector) { s.Parameters.WindowSeconds = 20 },
		"no minimum gap":           func(s *Selector) { s.Parameters.MinGap = 0 },
		"another seed":             func(s *Selector) { s.Parameters.RandomSeed = 7 },
		"the smallest first":       func(s *Selector) { s.Score.Order = "ascending" },
		"another measure":          func(s *Selector) { s.Score.Measure = "pair_seconds" },
		"a requirement":            func(s *Selector) { s.Require = []Requirement{{Measure: "pairs", Min: new(float64)}} },
		"a failure-seeking finder": func(s *Selector) { s.Finder, s.Score.Measure = "leader_changes", "leader_changes" },
	} {
		s := following
		s.Require = append([]Requirement(nil), following.Require...)
		change(&s)
		if s.HeldOut() {
			t.Errorf("%s: still held-out eligible", name)
		}
	}
}

func TestTheDigestNamesWhatTheSelectorDoes(t *testing.T) {
	c := shippedCatalogue(t)
	base := selectorFrom(t, c, "close_following")
	digest := base.Digest()
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != 71 || base.Digest() != digest {
		t.Fatalf("digest %q", digest)
	}
	renamed := base
	renamed.Label, renamed.Category, renamed.Description = "Tailgating", "Other", "Renamed."
	if renamed.Digest() != digest {
		t.Fatal("a new label moved the digest")
	}
	unrequired := base
	unrequired.Require = nil
	if empty := (Selector{ID: "x", Require: []Requirement{}}); empty.Digest() != (Selector{ID: "x"}).Digest() {
		t.Fatal("no requirements digest differently written as nil and as []")
	}
	two := 2.5
	for name, changed := range map[string]Selector{
		"id":           func() Selector { s := base; s.ID = "closer"; return s }(),
		"finder":       func() Selector { s := base; s.Finder = "leader_changes"; return s }(),
		"parameters":   func() Selector { s := base; s.Parameters.MaxGap = 16; return s }(),
		"order":        func() Selector { s := base; s.Score.Order = "ascending"; return s }(),
		"requirements": unrequired,
		"a bound":      func() Selector { s := base; s.Require = []Requirement{{Measure: "pair_seconds", Min: &two}}; return s }(),
	} {
		if changed.Digest() == digest {
			t.Errorf("changing the %s left the digest as it was", name)
		}
	}
}

func TestSelectorFilesAreFoundAndRead(t *testing.T) {
	shipped, err := os.ReadFile(shippedSelectorFile)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	good := write("selectors.json", shipped)
	bad := write("bad.json", []byte(`{"version": 1}`))
	if err := os.Mkdir(filepath.Join(dir, "folder.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		path, want string
	}{
		"another extension": {filepath.Join(dir, "selectors.yaml"), `.json extension, not ".yaml"`},
		"nothing there":     {filepath.Join(dir, "missing.json"), "no such file"},
		"a directory":       {filepath.Join(dir, "folder.json"), "is a directory"},
		"too large":         {write("large.json", make([]byte, maxSelectorsFileSize+1)), "more than 1048576"},
		"not a catalogue":   {bad, bad + ": selectors file: missing required keys: selectors"},
	} {
		if _, err := LoadSelectors(tc.path); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
	if c, err := LoadSelectors(good); err != nil || c.Source != good {
		t.Fatalf("load: %v, %+v", err, c)
	}

	// The file on disk is preferred; the embedded copy stands in only for a
	// file that is not there at all.
	if c, err := LoadSelectorsOrEmbedded(good, nil); err != nil || c.Source != good {
		t.Fatalf("present file: %v", err)
	}
	if _, err := LoadSelectorsOrEmbedded(bad, shipped); err == nil {
		t.Fatal("an invalid file fell back to the embedded copy")
	}
	if c, err := LoadSelectorsOrEmbedded(filepath.Join(dir, "missing.json"), shipped); err != nil || c.Source != "embedded" || len(c.Selectors) != 7 {
		t.Fatalf("embedded fallback: %v", err)
	}
	if _, err := LoadSelectorsOrEmbedded(filepath.Join(dir, "missing.json"), nil); err == nil || !strings.Contains(err.Error(), "none is embedded") {
		t.Fatalf("no file and nothing embedded: %v", err)
	}
	if _, err := LoadSelectorsOrEmbedded(filepath.Join(good, "child.json"), shipped); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("a path through a file: %v", err)
	}
	if _, err := LoadSelectorsOrEmbedded(filepath.Join(dir, "missing.json"), []byte(`{}`)); err == nil || !strings.Contains(err.Error(), "embedded selectors") {
		t.Fatalf("an invalid embedded copy: %v", err)
	}

	// The default is the repository's file where a relative path reaches it,
	// and the embedded copy anywhere else.
	defer SetEmbeddedSelectors(embeddedSelectors)
	SetEmbeddedSelectors(shipped)
	if c, err := DefaultCatalogue(); err != nil || c.Source != filepath.Clean(shippedSelectorFile) {
		t.Fatalf("default from the repository: %v, %+v", err, c)
	}
	if c, err := catalogueFrom([]string{filepath.Join(dir, "missing.json"), good}, nil); err != nil || c.Source != good {
		t.Fatalf("the second candidate: %v", err)
	}
	if c, err := catalogueFrom([]string{filepath.Join(dir, "missing.json")}, shipped); err != nil || c.Source != "embedded" {
		t.Fatalf("embedded default: %v", err)
	}
}

// Rank validates a selector built in code as the file's parser does, since a
// caller's parameters replace the file's.
func TestSelectorsBuiltInCodeAreValidated(t *testing.T) {
	c := shippedCatalogue(t)
	base := selectorFrom(t, c, "close_following")
	infinite := math.Inf(1)
	for name, tc := range map[string]struct {
		change func(*Selector)
		want   string
	}{
		"an unknown finder":   {func(s *Selector) { s.Finder = "tailgating" }, `unknown finder "tailgating"`},
		"invalid parameters":  {func(s *Selector) { s.Parameters.WindowSeconds = 0 }, "invalid finder parameters"},
		"an infinite bound":   {func(s *Selector) { s.Require = []Requirement{{Measure: "pairs", Max: &infinite}} }, "not a finite number"},
		"an unknown measure":  {func(s *Selector) { s.Score.Measure = "speed" }, `unknown measure "speed"`},
		"no description":      {func(s *Selector) { s.Description = "" }, "description must be given"},
		"a requirement twice": {func(s *Selector) { s.Require = append(s.Require, s.Require...) }, "bounded twice"},
	} {
		s := base
		s.Require = append([]Requirement(nil), base.Require...)
		tc.change(&s)
		if _, err := Rank(nil, s, "source", "tuning", nil); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
	if err := base.validate(); err != nil {
		t.Fatalf("the shipped selector is invalid: %v", err)
	}
}
