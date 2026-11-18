//go:build pcap

package lidar

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestFollowingCorpusRefusesDuplicateIDsAndExistingOutputs(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	if err := analyseCorpus("missing", []string{"same", "same"}, out, false); err == nil {
		t.Fatal("duplicate runs accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("invalid corpus created output")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := analyseCorpus("missing", []string{"one"}, out, false); err == nil {
		t.Fatal("existing output accepted")
	}
	if code := FollowingMain([]string{}); code != 2 {
		t.Fatal(code)
	}
	if code := FollowingMain([]string{"--help"}); code != 0 {
		t.Fatal(code)
	}
}

func TestFollowingNamespaceAndCompletionEncoding(t *testing.T) {
	if code := Main([]string{"following", "--help"}); code != 0 {
		t.Fatal(code)
	}
	out := t.TempDir()
	if err := writeFollowingCorpusJSON(out, map[string]float64{"invalid": math.NaN()}, os.WriteFile); err == nil {
		t.Fatal("unsupported completion JSON accepted")
	}
	if _, err := os.Stat(filepath.Join(out, "complete.json")); !os.IsNotExist(err) {
		t.Fatal("invalid success marker published")
	}
}
