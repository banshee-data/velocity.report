package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCorpusCasesPreservesDeclaredOrder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "s2"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"second.pcap", "first.pcap"} {
		if err := os.WriteFile(filepath.Join(root, "s2", name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := resolveCorpusCases(corpus{Cases: []corpusCase{{ID: "marina", ExpectedCaptureCount: 2}}}, map[string]indexEntry{
		"marina": {ID: "marina", Captures: []string{"second.pcap", "first.pcap"}},
	}, root, "s2")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 {
		t.Fatalf("resolved cases = %#v, want one", resolved)
	}
	got := resolved[0].paths
	if len(got) != 2 || filepath.Base(got[0]) != "second.pcap" || filepath.Base(got[1]) != "first.pcap" {
		t.Fatalf("resolved paths = %#v, want declared order", resolved)
	}
}

func TestWriteSourceManifestIsImmutableAndDigestible(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "phase01-source.json")
	manifest := sourceManifest{SchemaVersion: 1, Cases: []sourceManifestCase{{ID: "marina", Captures: []sourceManifestCapture{{
		Ordinal: 0, RelativePath: "s2/a.pcap", ByteSize: 3, SHA256: "sha256:" + strings.Repeat("a", 64),
	}}}}}
	digest, err := writeSourceManifest(path, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("digest = %q", digest)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(contents), "\n") || !strings.Contains(string(contents), "\"relative_path\": \"s2/a.pcap\"") {
		t.Fatalf("unexpected manifest content: %s", contents)
	}
	if _, err := writeSourceManifest(path, manifest); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second write error = %v, want os.ErrExist", err)
	}
	verified, err := verifySourceManifest(path, manifest)
	if err != nil || verified != digest {
		t.Fatalf("verifySourceManifest = %q, %v; want %q", verified, err, digest)
	}
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verifySourceManifest(path, manifest); err == nil {
		t.Fatal("changed source manifest verified")
	}
}

func TestBuildSourceManifestUsesRootRelativeCapturePaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	pcapPath := filepath.Join(root, "s2", "a.pcap")
	if err := os.Mkdir(filepath.Dir(pcapPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pcapPath, []byte("pcap"), 0o644); err != nil {
		t.Fatal(err)
	}
	corpusPath := filepath.Join(root, "corpus.json")
	indexPath := filepath.Join(root, "index.json")
	if err := os.WriteFile(corpusPath, []byte(`{"cases":[{"id":"marina","expected_capture_count":1}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, []byte(`[{"id":"marina","captures":["a.pcap"]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := buildSourceManifest(corpusPath, indexPath, root, "", "hesai-pandar40p", []resolvedCorpusCase{{
		corpusCase: corpusCase{ID: "marina", ExpectedCaptureCount: 1}, paths: []string{pcapPath},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.Cases[0].Captures[0].RelativePath; got != "s2/a.pcap" {
		t.Fatalf("relative path = %q", got)
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), root) {
		t.Fatalf("manifest leaked machine-specific root %q: %s", root, payload)
	}
}

func TestSourceManifestRawSHA256(t *testing.T) {
	t.Parallel()
	want := strings.Repeat("a", 64)
	if got, err := sourceManifestRawSHA256("sha256:" + want); err != nil || got != want {
		t.Fatalf("sourceManifestRawSHA256 = %q, %v", got, err)
	}
	if _, err := sourceManifestRawSHA256(want); err == nil {
		t.Fatal("raw digest without prefix succeeded")
	}
}

func TestResolveObservationDBPathSeparatesEvidenceFromRecordingOutput(t *testing.T) {
	root := t.TempDir()
	evidenceDir := filepath.Join(root, "internal-evidence")
	got, err := resolveObservationDBPath("", evidenceDir, filepath.Join(root, "vrlogs"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(evidenceDir, "observations.db"); got != want {
		t.Fatalf("evidence DB = %q, want %q", got, want)
	}
	if _, err := os.Stat(evidenceDir); err != nil {
		t.Fatalf("evidence directory was not created: %v", err)
	}
	if _, err := resolveObservationDBPath(filepath.Join(root, "old.db"), evidenceDir, filepath.Join(root, "vrlogs")); err == nil {
		t.Fatal("accepted both an explicit evidence database and evidence directory")
	}
	if _, err := resolveObservationDBPath("", filepath.Join(root, "vrlogs"), filepath.Join(root, "vrlogs")); err == nil {
		t.Fatal("accepted evidence and recording output in the same directory")
	}
}

func TestSourceManifestCaseDigestsRequireOrderedCompleteCase(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	manifest := sourceManifest{Cases: []sourceManifestCase{{ID: "marina", Captures: []sourceManifestCapture{
		{Ordinal: 0, SHA256: digest}, {Ordinal: 1, SHA256: digest},
	}}}}
	got, err := sourceManifestCaseDigests(manifest, "marina", 2)
	if err != nil || len(got) != 2 || got[0] != digest {
		t.Fatalf("sourceManifestCaseDigests = %#v, %v", got, err)
	}
	if _, err := sourceManifestCaseDigests(manifest, "missing", 1); err == nil {
		t.Fatal("accepted a missing case")
	}
	manifest.Cases[0].Captures[1].Ordinal = 2
	if _, err := sourceManifestCaseDigests(manifest, "marina", 2); err == nil {
		t.Fatal("accepted out-of-order capture ordinals")
	}
}
