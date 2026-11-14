package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func referenceFixture(t *testing.T) Reference {
	t.Helper()
	root := t.TempDir()
	put(t, root, "pack/manifest.json", "{}")
	for _, p := range []string{"split.json", "a.db", "b.db"} {
		put(t, root, p, "fixture")
	}
	return Reference{Pack: filepath.Join(root, "pack"), Manifest: filepath.Join(root, "split.json"), Split: "tuning", ADB: filepath.Join(root, "a.db"), BDB: filepath.Join(root, "b.db")}
}

func TestScoringReferencePreflight(t *testing.T) {
	ref := referenceFixture(t)
	h := Host{References: map[string]Reference{"host0001/score": ref}}
	if _, err := h.scoringReference("other", "score", "a0"); err == nil {
		t.Fatal("missing mapping accepted")
	}
	got, err := h.scoringReference("host0001", "score", "a0")
	if err != nil || got != ref {
		t.Fatalf("fallback: %+v %v", got, err)
	}
	override := ref
	override.Split = "heldout"
	h.References["host0001/score/a0"] = override
	got, err = h.scoringReference("host0001", "score", "a0")
	if err != nil || got != override {
		t.Fatalf("override: %+v %v", got, err)
	}
	if err := ref.checkScoringInputs(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*Reference)
	}{
		{"unnamed split", func(r *Reference) { r.Split = " " }},
		{"relative pack", func(r *Reference) { r.Pack = "pack" }},
		{"missing manifest", func(r *Reference) { r.Manifest = filepath.Join(t.TempDir(), "missing") }},
		{"directory database", func(r *Reference) { r.ADB = r.Pack }},
		{"file pack", func(r *Reference) { r.Pack = r.Manifest }},
		{"empty database", func(r *Reference) {
			r.BDB = filepath.Join(t.TempDir(), "empty")
			put(t, filepath.Dir(r.BDB), "empty", "")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := ref
			tc.change(&bad)
			if bad.checkScoringInputs() == nil {
				t.Fatal("invalid reference accepted")
			}
		})
	}
}

func TestMissingScoringReferenceSkipsBeforeBuildAndReplay(t *testing.T) {
	o, _ := fixture(t)
	path := filepath.Join(o.Root, "data/experiments/host/host0001.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "steps:\n", "steps:\n - id: score\n   kind: ground_truth_eval\n   over: [arms]\n   estimate: {each: 1s, worker: arrow}\n", 1))
	if err = os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	gitTest(t, o.Root, "add", ".")
	gitTest(t, o.Root, "commit", "-m", "scoring step")
	o.buildTools = func(context.Context, string, string, string) (string, string, error) {
		t.Fatal("built despite missing scoring inputs")
		return "", "", nil
	}
	for _, dry := range []bool{true, false} {
		o.DryRun = dry
		st, err := Tick(context.Background(), o)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(st.Skipped["host0001"], "no frozen reference mapping") || len(st.Runs) != 0 {
			t.Fatalf("%+v", st)
		}
	}
}
