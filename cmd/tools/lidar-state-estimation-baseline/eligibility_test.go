package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitycli"
	fx "github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitytest"
)

// developmentGuard is the eligibility flags under which kirk0, by its bytes,
// is development data: a synthetic manifest, an empty splits directory and a
// migrated ledger.
func developmentGuard(t *testing.T, dir, kirk0SHA string) []string {
	t.Helper()
	root := filepath.Join(dir, "annotation")
	if err := os.MkdirAll(filepath.Join(root, "splits"), 0o755); err != nil {
		t.Fatal(err)
	}
	ledger := filepath.Join(dir, "ledger.db")
	d, err := db.NewDB(ledger)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
	manifest := fx.Manifest(t, dir, map[string]any{"sites": []any{fx.Site("kirk", "development")},
		"captures": []any{fx.CaptureEntry("kirk0", "kirk", kirk0SHA, "kirk0.pcapng")}})
	return []string{"-cohort-manifest", manifest, "-annotation-root", root, "-eligibility-db", ledger, "-actor", "tester"}
}

// The corpus run is refused before a capture is read when the guard has no
// manifest, and a held-out score of a case another lineage tuned on is
// refused whichever split the run names.
func TestCorpusRunIsGuardedBeforeCapturesLoad(t *testing.T) {
	dir := t.TempDir()
	path, sha := fx.Capture(t, dir, "kirk0.pcapng", "synthetic kirk0")
	cases := []resolvedCorpusCase{{paths: []string{path}}}
	empty := ""
	none := &eligibilitycli.CLIFlags{Manifest: &empty, AnnotationRoot: &empty, LedgerDB: &empty, Actor: &empty, Measures: &empty}
	if err := (corpusSplitUse{}).authorize(none, cases, "", ""); err == nil {
		t.Fatal("no manifest: refused")
	}
	flags := developmentGuard(t, dir, sha)
	manifest, root, ledger, actor := flags[1], flags[3], flags[5], flags[7]
	guard := &eligibilitycli.CLIFlags{Manifest: &manifest, AnnotationRoot: &root, LedgerDB: &ledger, Actor: &actor, Measures: &empty}
	if err := (corpusSplitUse{}).authorize(guard, cases, "", ""); err != nil {
		t.Fatalf("a development capture: %v", err)
	}
	held := corpusSplitUse{record: &splitRecord{Digest: "sha256:new-lineage", HeldOut: true}}
	// Its tuning read just now is exposure for every measure family.
	if err := held.authorize(guard, cases, "", ""); err == nil || !strings.Contains(err.Error(), "already read for development") {
		t.Fatalf("a held-out score of a capture read for development: %v", err)
	}
}
