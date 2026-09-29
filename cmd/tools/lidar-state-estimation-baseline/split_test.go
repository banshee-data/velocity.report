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

// caseWithCapture declares a case replaying one capture, <id>.pcap, whose
// digest is a digest of the ID.
func caseWithCapture(id string, role annotation.SplitRole) annotation.SplitCase {
	return annotation.SplitCase{CaseID: id, Role: role,
		Captures: []annotation.CaseCapture{{Basename: id + ".pcap", SHA256: sha256Payload([]byte(id))}}}
}

func nearEdgePartition(t *testing.T) (*annotation.FrozenSplit, string) {
	return writeCaseSplit(t,
		caseWithCapture("marina-webster-beach", annotation.SplitRoleTuning),
		caseWithCapture("columbus-broadway", annotation.SplitRoleTuning),
		caseWithCapture("embarcadero-folsom", annotation.SplitRoleHeldOut),
		caseWithCapture("ashbury-downey", annotation.SplitRoleScreen),
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
	use, err := corpusSplit("", false, threeCaseCorpus())
	if use.split != nil || use.record != nil || use.uses != nil || err != nil {
		t.Fatalf("no split: %+v, %v", use, err)
	}
	if err := use.checkCaptures([]resolvedCorpusCase{{corpusCase: corpusCase{ID: "any"}, paths: []string{"anything.pcap"}}}); err != nil {
		t.Fatalf("captures checked without a split: %v", err)
	}
	if _, err := corpusSplit("", true, threeCaseCorpus()); err == nil || !strings.Contains(err.Error(), "-held-out needs -split-manifest") {
		t.Fatalf("held out without a split: %v", err)
	}
	if _, err := corpusSplit(filepath.Join(t.TempDir(), "none.json"), false, threeCaseCorpus()); err == nil {
		t.Fatal("a missing split was accepted")
	}
}

// A case's role binds to its captures: every capture the index resolves for
// a case must be one the split names for it, by content.
func TestCorpusSplitChecksEveryCaseCapture(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	a, b, held := write("a.pcap", "first"), write("b.pcap", "second"), write("held.pcap", "held out")
	_, path := writeCaseSplit(t,
		annotation.SplitCase{CaseID: "site", Role: annotation.SplitRoleTuning, Captures: []annotation.CaseCapture{
			{Basename: "a.pcap", SHA256: sha256Payload([]byte("first"))}, {Basename: "b.pcap", SHA256: sha256Payload([]byte("second"))}}},
		annotation.SplitCase{CaseID: "held", Role: annotation.SplitRoleHeldOut, Captures: []annotation.CaseCapture{
			{Basename: "held.pcap", SHA256: sha256Payload([]byte("held out"))}}},
	)
	use, err := corpusSplit(path, false, corpusOf("site"))
	if err != nil {
		t.Fatal(err)
	}
	if err := use.checkCaptures([]resolvedCorpusCase{{corpusCase: corpusCase{ID: "site"}, paths: []string{a, b}}}); err != nil {
		t.Fatal(err)
	}
	// An index that resolves the held-out capture for the tuning case.
	err = use.checkCaptures([]resolvedCorpusCase{{corpusCase: corpusCase{ID: "site"}, paths: []string{a, held}}})
	if err == nil || !strings.Contains(err.Error(), `is not one of case "site"'s captures`) {
		t.Fatalf("a held-out capture under a tuning case: %v", err)
	}
}

// Tuning and screen cases replay freely; a held-out case only as a held-out
// score, which takes nothing else.
func TestCorpusSplitHoldsCasesToTheirRoles(t *testing.T) {
	f, path := nearEdgePartition(t)

	use, err := corpusSplit(path, false, corpusOf("marina-webster-beach", "columbus-broadway", "ashbury-downey"))
	if err != nil {
		t.Fatal(err)
	}
	record, uses := use.record, use.uses
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

	if _, err := corpusSplit(path, false, threeCaseCorpus()); err == nil || !strings.Contains(err.Error(), `case "embarcadero-folsom" is held out`) {
		t.Fatalf("a held-out case in a tuning run: %v", err)
	}
	use, err = corpusSplit(path, true, corpusOf("embarcadero-folsom"))
	if err != nil || !use.record.HeldOut || !use.uses["embarcadero-folsom"].HeldOut || use.uses["embarcadero-folsom"].Role != "held_out" {
		t.Fatalf("held-out score: %+v, %v", use, err)
	}
	if _, err := corpusSplit(path, true, threeCaseCorpus()); !errors.Is(err, annotation.ErrNotHeldOut) {
		t.Fatalf("tuning cases in a held-out score: %v", err)
	}
	if _, err := corpusSplit(path, false, corpusOf("pierce-haight")); err == nil || !strings.Contains(err.Error(), "has no role") {
		t.Fatalf("an undeclared case: %v", err)
	}
}
