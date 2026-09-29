package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// writeCaseSplit freezes a split of corpus cases alone, the label-free
// partition of the near-edge plan, and returns its path.
func writeCaseSplit(t *testing.T, cases ...annotation.SplitCase) (*annotation.FrozenSplit, string) {
	t.Helper()
	f, err := annotation.FreezeSplit(annotation.FreezeOptions{
		Draft: &annotation.SplitDraft{Schema: annotation.SplitDraftSchema, SchemaVersion: annotation.SplitDraftSchemaVersion,
			Cases: cases},
		Author: "operator", Now: time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC), BuildVersion: "test", BuildGitSHA: "abc",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "split.json")
	if err := annotation.WriteFrozenSplit(path, f); err != nil {
		t.Fatal(err)
	}
	return f, path
}

func nearEdgePartition(t *testing.T) (*annotation.FrozenSplit, string) {
	return writeCaseSplit(t,
		annotation.SplitCase{CaseID: "marina-webster-beach", Role: annotation.SplitRoleTuning},
		annotation.SplitCase{CaseID: "columbus-broadway", Role: annotation.SplitRoleTuning},
		annotation.SplitCase{CaseID: "embarcadero-folsom", Role: annotation.SplitRoleHeldOut},
		annotation.SplitCase{CaseID: "ashbury-downey", Role: annotation.SplitRoleScreen},
	)
}

func corpusOf(ids ...string) corpus {
	var c corpus
	for _, id := range ids {
		c.Cases = append(c.Cases, corpusCase{ID: id})
	}
	return c
}

// Without a split a run is as it always was; a held-out claim needs one.
func TestCorpusSplitIsOptionalButHeldOutIsNot(t *testing.T) {
	record, uses, err := corpusSplit("", false, threeCaseCorpus())
	if record != nil || uses != nil || err != nil {
		t.Fatalf("no split: %+v, %+v, %v", record, uses, err)
	}
	if _, _, err := corpusSplit("", true, threeCaseCorpus()); err == nil || !strings.Contains(err.Error(), "-held-out needs -split-manifest") {
		t.Fatalf("held out without a split: %v", err)
	}
	if _, _, err := corpusSplit(filepath.Join(t.TempDir(), "none.json"), false, threeCaseCorpus()); err == nil {
		t.Fatal("a missing split was accepted")
	}
}

// Tuning and screen cases replay freely; a held-out case only as a held-out
// score, which takes nothing else.
func TestCorpusSplitHoldsCasesToTheirRoles(t *testing.T) {
	f, path := nearEdgePartition(t)

	record, uses, err := corpusSplit(path, false, corpusOf("marina-webster-beach", "columbus-broadway", "ashbury-downey"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if record.Digest != f.SplitDigest || record.Revision != 1 || record.HeldOut || record.FileSHA256 != sha256Payload(b) {
		t.Fatalf("split record %+v", record)
	}
	if u := uses["ashbury-downey"]; u == nil || u.Role != "screen" || u.SplitDigest != f.SplitDigest || u.CaseID != "ashbury-downey" || u.HeldOut {
		t.Fatalf("screen use %+v", u)
	}
	if uses["marina-webster-beach"].Role != "tuning" || len(uses) != 3 {
		t.Fatalf("uses %+v", uses)
	}

	if _, _, err := corpusSplit(path, false, threeCaseCorpus()); err == nil || !strings.Contains(err.Error(), `case "embarcadero-folsom" is held out`) {
		t.Fatalf("a held-out case in a tuning run: %v", err)
	}
	record, uses, err = corpusSplit(path, true, corpusOf("embarcadero-folsom"))
	if err != nil || !record.HeldOut || !uses["embarcadero-folsom"].HeldOut || uses["embarcadero-folsom"].Role != "held_out" {
		t.Fatalf("held-out score: %+v, %+v, %v", record, uses, err)
	}
	if _, _, err := corpusSplit(path, true, threeCaseCorpus()); !errors.Is(err, annotation.ErrNotHeldOut) {
		t.Fatalf("tuning cases in a held-out score: %v", err)
	}
	if _, _, err := corpusSplit(path, false, corpusOf("pierce-haight")); err == nil || !strings.Contains(err.Error(), "has no role") {
		t.Fatalf("an undeclared case: %v", err)
	}
}
