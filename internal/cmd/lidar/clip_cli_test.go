//go:build pcap

package lidar

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

func TestAnnotationClipOneCommandCutsKirk0PackAndRecord(t *testing.T) {
	if testing.Short() {
		t.Skip("real PCAP replay")
	}
	pcap, err := filepath.Abs("../../lidar/perf/pcap/kirk0.pcapng")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pcap); err != nil {
		t.Skip("kirk0 fixture unavailable")
	}
	out := filepath.Join(t.TempDir(), "clip")
	code := silence(t, func() int {
		return AnnotationClipMain([]string{"--pcap", pcap, "--start-seconds", "40", "--duration-seconds", "5", "--warmup-seconds", "35", "--output", out})
	})
	if code != 0 {
		t.Fatalf("one-step clip exited %d", code)
	}
	pack, err := annotation.OpenPack(filepath.Join(out, "pack"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Samples) == 0 || pack.Manifest.Coverage != annotation.CoverageForegroundOnly {
		t.Fatalf("unexpected pack: %d samples, %s coverage", len(pack.Samples), pack.Manifest.Coverage)
	}
	b, err := os.ReadFile(filepath.Join(out, "pack", "segment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record segments.Record
	if err := json.Unmarshal(b, &record); err != nil {
		t.Fatal(err)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	if record.PackDigest != pack.Manifest.PackDigest || record.Role != "tuning" || record.Finder != "manual" {
		t.Fatalf("record does not bind exported pack: %+v", record)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(out, "vrlog", "replay_manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var replay struct {
		ScoringStartNs int64 `json:"scoring_start_ns"`
	}
	if err := json.Unmarshal(manifestBytes, &replay); err != nil {
		t.Fatal(err)
	}
	params := segments.DefaultParams()
	params.WindowSeconds = 5
	params.MaxGap = 17
	selected := segments.Window{Finder: "following", Version: segments.Version, Source: "kirk0", Role: "tuning", StartNs: replay.ScoringStartNs, EndNs: replay.ScoringStartNs + 5_000_000_000, Capture: pcap, OffsetSeconds: 40}
	selected.ID = segments.Identity(selected.Finder, selected.Source, selected.Role, params, selected.StartNs)
	report := map[string]any{"schema": "velocity.report/segments", "source": selected.Source, "finder": selected.Finder, "version": segments.Version, "role": selected.Role, "parameters": params, "windows": []segments.Window{selected}}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(t.TempDir(), "selection.json")
	if err := os.WriteFile(reportPath, reportBytes, 0644); err != nil {
		t.Fatal(err)
	}
	selectedOut := filepath.Join(t.TempDir(), "selected-clip")
	if code := silence(t, func() int {
		return AnnotationClipMain([]string{"--pcap", pcap, "--start-seconds", "40", "--duration-seconds", "5", "--warmup-seconds", "35", "--role", "tuning", "--selection", reportPath, "--observations", "--output", selectedOut})
	}); code != 0 {
		t.Fatalf("selected one-step clip exited %d", code)
	}
	b, err = os.ReadFile(filepath.Join(selectedOut, "pack", "segment.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &record); err != nil {
		t.Fatal(err)
	}
	if record.Segment.ID != selected.ID || record.Parameters.MaxGap != 17 {
		t.Fatalf("selection provenance was lost: %+v", record)
	}
	if _, err := os.Stat(filepath.Join(selectedOut, "observations.vrlog")); err != nil {
		t.Fatalf("optional observation container is missing: %v", err)
	}
	if code := silence(t, func() int {
		return AnnotationClipMain([]string{"--pcap", pcap, "--start-seconds", "40", "--duration-seconds", "5", "--output", out})
	}); code != 2 {
		t.Fatalf("existing output was overwritten: exit %d", code)
	}
}

func TestAnnotationClipRefusesInvalidPreflightInputs(t *testing.T) {
	for name, args := range map[string][]string{
		"missing capture": {"--output", filepath.Join(t.TempDir(), "clip"), "--duration-seconds", "5"},
		"bad flag":        {"--not-a-flag"},
		"negative start":  {"--pcap", "/capture.pcap", "--output", filepath.Join(t.TempDir(), "clip"), "--start-seconds", "-1", "--duration-seconds", "5"},
		"zero duration":   {"--pcap", "/capture.pcap", "--output", filepath.Join(t.TempDir(), "clip"), "--duration-seconds", "0"},
		"bad warmup":      {"--pcap", "/capture.pcap", "--output", filepath.Join(t.TempDir(), "clip"), "--duration-seconds", "5", "--warmup-seconds", "-1"},
		"bad role":        {"--pcap", "/capture.pcap", "--output", filepath.Join(t.TempDir(), "clip"), "--duration-seconds", "5", "--role", "other"},
		"empty path":      {"--pcap", "/capture.pcap,", "--output", filepath.Join(t.TempDir(), "clip"), "--duration-seconds", "5"},
		"held out manual": {"--pcap", "/capture.pcap", "--output", filepath.Join(t.TempDir(), "clip"), "--duration-seconds", "5", "--role", "held_out"},
	} {
		t.Run(name, func(t *testing.T) {
			if code := silence(t, func() int { return AnnotationClipMain(args) }); code != 2 {
				t.Fatalf("invalid inputs exited %d", code)
			}
		})
	}
	if code := silence(t, func() int { return AnnotationClipMain([]string{"--help"}) }); code != 0 {
		t.Fatalf("help exited %d", code)
	}
	missingParent := filepath.Join(t.TempDir(), "missing", "clip")
	samplePCAP, err := filepath.Abs("../../lidar/l1packets/parse/sample_packet.pcapng")
	if err != nil {
		t.Fatal(err)
	}
	if code := silence(t, func() int {
		return AnnotationClipMain([]string{"--pcap", samplePCAP, "--output", missingParent, "--duration-seconds", "5"})
	}); code != 1 {
		t.Fatalf("missing output parent exited %d", code)
	}
	file := filepath.Join(t.TempDir(), "ordinary-file")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if code := silence(t, func() int {
		return AnnotationClipMain([]string{"--pcap", samplePCAP, "--output", filepath.Join(file, "clip"), "--duration-seconds", "5"})
	}); code != 1 {
		t.Fatalf("non-directory output parent exited %d", code)
	}
	if code := silence(t, func() int {
		return AnnotationClipMain([]string{"--pcap", filepath.Join(t.TempDir(), "missing.pcap"), "--output", filepath.Join(t.TempDir(), "clip"), "--duration-seconds", "5"})
	}); code != 1 {
		t.Fatalf("missing capture port detection exited %d", code)
	}
	if code := silence(t, func() int {
		return AnnotationClipMain([]string{"--pcap", samplePCAP + "," + samplePCAP, "--output", filepath.Join(t.TempDir(), "clip"), "--duration-seconds", "5"})
	}); code != 1 {
		t.Fatalf("broken capture sequence exited %d", code)
	}
}

func TestAnnotationClipRejectsSelectionDriftBeforeReplay(t *testing.T) {
	params := segments.DefaultParams()
	window := segments.Window{Finder: "following", Version: segments.Version, Source: "run", Role: "tuning", StartNs: 1, EndNs: 10_000_000_001, Capture: "/capture.pcap", OffsetSeconds: 0}
	window.ID = segments.Identity(window.Finder, window.Source, window.Role, params, window.StartNs)
	report := map[string]any{"schema": "velocity.report/segments", "source": window.Source, "finder": window.Finder, "version": segments.Version, "role": window.Role, "parameters": params, "windows": []segments.Window{window}}
	writeReport := func(t *testing.T, payload any) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "selection.json")
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for _, tc := range []struct {
		name, pcap string
		start      string
	}{
		{"capture", "/other.pcap", "0"},
		{"offset", "/capture.pcap", "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := silence(t, func() int {
				return AnnotationClipMain([]string{"--pcap", tc.pcap, "--start-seconds", tc.start, "--duration-seconds", "10", "--selection", writeReport(t, report), "--output", filepath.Join(t.TempDir(), "clip")})
			})
			if code != 2 {
				t.Fatalf("selection drift exited %d", code)
			}
		})
	}
	if code := silence(t, func() int {
		return AnnotationClipMain([]string{"--pcap", "/capture.pcap", "--start-seconds", "0", "--duration-seconds", "10", "--selection", filepath.Join(t.TempDir(), "missing.json"), "--output", filepath.Join(t.TempDir(), "clip")})
	}); code != 2 {
		t.Fatalf("missing selection exited %d", code)
	}
}

func TestAnnotationClipReportsFailureAtEachPackBoundary(t *testing.T) {
	pcap, err := filepath.Abs("../../lidar/l1packets/parse/sample_packet.pcapng")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"manifest read", "manifest JSON", "manifest bounds", "export", "record"} {
		t.Run(mode, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "clip")
			ops := clipOperations{
				run: func(replayeval.Config) (*replayeval.Result, error) {
					return &replayeval.Result{VRLOGPath: filepath.Join(out, "vrlog")}, nil
				},
				readManifest: func(string) ([]byte, error) {
					switch mode {
					case "manifest read":
						return nil, errors.New("missing manifest")
					case "manifest JSON":
						return []byte("bad JSON"), nil
					case "manifest bounds":
						return []byte(`{"scoring_start_ns":0,"scoring_duration_seconds":0}`), nil
					default:
						return []byte(`{"scoring_start_ns":1000000000,"scoring_duration_seconds":5}`), nil
					}
				},
				export: func(annotation.ExportConfig) (*annotation.Pack, error) {
					if mode == "export" {
						return nil, errors.New("export failed")
					}
					return &annotation.Pack{Dir: filepath.Join(out, "pack"), Manifest: annotation.Manifest{PackDigest: "sha256:stub"}}, nil
				},
				writeRecord: func(string, segments.Record) error {
					if mode == "record" {
						return errors.New("record failed")
					}
					return nil
				},
			}
			code := silence(t, func() int {
				return annotationClipMain([]string{"--pcap", pcap, "--output", out, "--duration-seconds", "5"}, ops)
			})
			if code != 1 {
				t.Fatalf("%s exited %d", mode, code)
			}
		})
	}
}
