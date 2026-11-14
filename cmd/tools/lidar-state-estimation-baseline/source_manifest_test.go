package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
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
	}}, l4bobserve.ReplayWindow{})
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
	got, err := resolveObservationDBPath("", evidenceDir, filepath.Join(root, "vrlogs"), false)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(evidenceDir, "observations.db"); got != want {
		t.Fatalf("evidence DB = %q, want %q", got, want)
	}
	if _, err := os.Stat(evidenceDir); err != nil {
		t.Fatalf("evidence directory was not created: %v", err)
	}
	if _, err := resolveObservationDBPath(filepath.Join(root, "old.db"), evidenceDir, filepath.Join(root, "vrlogs"), false); err == nil {
		t.Fatal("accepted both an explicit evidence database and evidence directory")
	}
	if _, err := resolveObservationDBPath("", filepath.Join(root, "vrlogs"), filepath.Join(root, "vrlogs"), false); err == nil {
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

func TestSourceManifestBindsEachSitesActualReplayCalibration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "capture")
	if err := os.WriteFile(path, []byte("capture"), 0600); err != nil {
		t.Fatal(err)
	}
	selected := corpus{Cases: []corpusCase{{ID: "columbus", ExpectedCaptureCount: 1}, {ID: "bryant", ExpectedCaptureCount: 1}}}
	index := map[string]indexEntry{
		"columbus": {Captures: []string{"capture"}, NorthAzimuthDeg: 35},
		"bryant":   {Captures: []string{"capture"}, NorthAzimuthDeg: 353},
	}
	cases, err := resolveCorpusCases(selected, index, root, "")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := buildSourceManifest(path, path, root, "", "sensor", cases, l4bobserve.ReplayWindow{})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 3 || manifest.Cases[0].CalibrationID == manifest.Cases[1].CalibrationID {
		t.Fatalf("manifest: %+v", manifest)
	}
	for _, c := range manifest.Cases {
		calibration := siteCalibration("sensor", index[c.ID].NorthAzimuthDeg)
		want, _ := l4bobserve.CalibrationID(calibration)
		if c.CalibrationID != want {
			t.Fatalf("%s calibration %s want %s", c.ID, c.CalibrationID, want)
		}
		if err := manifest.verifyCalibration(c.ID, calibration); err != nil {
			t.Fatal(err)
		}
		if err := manifest.verifyCalibration(c.ID, identityCalibration("sensor")); err == nil {
			t.Fatal("placeholder accepted")
		}
		if err := manifest.verifyCalibration(c.ID, siteCalibration("other", index[c.ID].NorthAzimuthDeg)); err == nil {
			t.Fatal("sensor drift accepted")
		}
	}
	payload, _ := json.Marshal(manifest)
	var top map[string]json.RawMessage
	if err := json.Unmarshal(payload, &top); err != nil {
		t.Fatal(err)
	}
	if _, exists := top["calibration_id"]; exists {
		t.Fatal("misleading global calibration retained")
	}
	if err := manifest.verifyCalibration("absent", identityCalibration("sensor")); err == nil {
		t.Fatal("absent case accepted")
	}
	if err := manifest.verifyCalibration("columbus", l4bobserve.Calibration{}); err == nil {
		t.Fatal("invalid calibration accepted")
	}
	manifest.SchemaVersion = 1
	if err := manifest.verifyCalibration("columbus", siteCalibration("sensor", 35)); err == nil {
		t.Fatal("legacy placeholder manifest accepted")
	}
}

func TestCorpusReplayChecksDeclaredCalibrationBeforeWritingEvidence(t *testing.T) {
	cal := siteCalibration("sensor", 35)
	id, _ := l4bobserve.CalibrationID(cal)
	for _, matches := range []bool{true, false} {
		t.Run(fmt.Sprint(matches), func(t *testing.T) {
			declared := id
			if !matches {
				declared = "placeholder"
			}
			stop := errors.New("reached replay")
			runner := corpusReplay{base: replayeval.Config{SensorID: "sensor"}, observationDBPath: filepath.Join(t.TempDir(), "e.db"),
				sourceManifest: &sourceManifest{SchemaVersion: 3, Cases: []sourceManifestCase{{ID: "site", CalibrationID: declared, Captures: []sourceManifestCapture{{Ordinal: 0, SHA256: "sha256:" + strings.Repeat("a", 64)}}}}},
				index:          map[string]indexEntry{"site": {NorthAzimuthDeg: 35}},
				prepare:        func([]string, int) (*capseq.Sequence, error) { return nil, nil },
				replay: func(cfg replayeval.Config) (*replayeval.Result, error) {
					if cfg.ObservationCalibration != cal {
						t.Fatal("wrong writer calibration")
					}
					return nil, stop
				},
			}
			_, _, err := runner.run([]resolvedCorpusCase{{corpusCase: corpusCase{ID: "site"}, paths: []string{"capture"}}})
			if matches && !errors.Is(err, stop) {
				t.Fatalf("matching declaration: %v", err)
			}
			if !matches && (err == nil || !strings.Contains(err.Error(), "calibration differs")) {
				t.Fatalf("drift not refused: %v", err)
			}
		})
	}
}

func TestSourceManifestBindsReplayWindowAndRejectsDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture")
	if err := os.WriteFile(path, []byte("capture"), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []resolvedCorpusCase{{corpusCase: corpusCase{ID: "site"}, paths: []string{path}}}
	root := filepath.Dir(path)
	a, err := buildSourceManifest(path, path, root, "", "sensor", cases, l4bobserve.ReplayWindow{StartOffsetNanos: 20, WarmupNanos: 20, DurationNanos: 30})
	if err != nil {
		t.Fatal(err)
	}
	b, err := buildSourceManifest(path, path, root, "", "sensor", cases, l4bobserve.ReplayWindow{StartOffsetNanos: 20, WarmupNanos: 20, DurationNanos: 40})
	if err != nil {
		t.Fatal(err)
	}
	if a.Cases[0].SourceID == b.Cases[0].SourceID {
		t.Fatal("different windows shared source")
	}
	runner := corpusReplay{sourceManifest: &a}
	if _, _, err := runner.run(cases); err == nil || !strings.Contains(err.Error(), "replay window differs") {
		t.Fatalf("accepted replay window drift: %v", err)
	}
}
