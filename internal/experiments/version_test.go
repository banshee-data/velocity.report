package experiments

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecVersionCompatibility(t *testing.T) {
	for _, v := range []string{"", "1.0", "1.8", "1.999"} {
		if e := CheckSpecVersion(v); e != nil {
			t.Errorf("%s: %v", v, e)
		}
	}
	for _, v := range []string{"2.0", "99.0"} {
		if e := CheckSpecVersion(v); !errors.Is(e, ErrSpecMajor) {
			t.Errorf("%s: %v", v, e)
		}
	}
	for _, v := range []string{"1", "v1.1", "0.1", "1.2.3"} {
		if e := CheckSpecVersion(v); e == nil {
			t.Errorf("accepted %s", v)
		}
	}
	root := tree(t, map[string]string{"data/experiments/abcd/abcd0001.md": strings.Replace(validSpec, "id: abcd0001", "spec_version: 1.7\nfuture_description: allowed\nid: abcd0001", 1), "data/experiments/abcd/abcd0002.md": "---\nspec_version: 2.0\nnew_execution_contract: true\n---\n"})
	skipped := map[string]string{}
	set, e := LoadCompatible(root, skipped)
	if e != nil {
		t.Fatal(e)
	}
	if len(set.Specs) != 1 || !strings.Contains(skipped["abcd0002"], "major") {
		t.Fatalf("specs=%d skipped=%v", len(set.Specs), skipped)
	}
	if _, e = Load(root); e == nil {
		t.Fatal("strict validation accepted future major")
	}
	p := filepath.Join(root, "data/experiments/abcd/abcd0001.md")
	b, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(b), "spec_version: 1.7", "spec_version: 1.0", 1)), 0644)
	if _, e = ReadFile(p); e == nil {
		t.Fatal("current schema accepted misspelt/unknown field")
	}
}
