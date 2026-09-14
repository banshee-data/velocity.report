// Command lidar-state-estimation-baseline runs the committed Phase 0 corpus
// through the offline pipeline. It deliberately resolves captures through the
// checked-in index rather than accepting an arbitrary set of PCAP paths: a
// baseline is useful only when another operator can reproduce its population.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

type corpus struct {
	Cases []corpusCase `json:"cases"`
}

type corpusCase struct {
	ID                   string `json:"id"`
	ExpectedCaptureCount int    `json:"expected_capture_count"`
}

type indexEntry struct {
	ID       string   `json:"id"`
	Captures []string `json:"captures"`
}

type caseSummary struct {
	ID                    string  `json:"id"`
	Captures              int     `json:"captures"`
	DurationSeconds       float64 `json:"duration_seconds"`
	FirstRunFrames        int     `json:"first_run_frames"`
	RepeatRunFrames       int     `json:"repeat_run_frames"`
	BaselineEqual         bool    `json:"baseline_equal"`
	ObservationSourceID   string  `json:"observation_source_id,omitempty"`
	MeasurementSourceMode string  `json:"measurement_source_mode"`
}

func main() {
	var (
		corpusPath                 = flag.String("corpus", "tools/s2-archive/state-estimation-phase01-corpus.json", "committed Phase 0 corpus JSON")
		indexPath                  = flag.String("index", "tools/s2-archive/site-index.json", "capture archive index JSON")
		pcapRoot                   = flag.String("pcap-root", "/Volumes/lidar/lidar", "directory holding archive capture subdirectories")
		pcapSubdir                 = flag.String("pcap-subdir", "s2", "archive capture subdirectory")
		outDir                     = flag.String("out", "", "empty output directory for baseline recordings (required)")
		sourceManifestPath         = flag.String("source-manifest", "", "new immutable JSON manifest of ordered source PCAP hashes")
		existingSourceManifestPath = flag.String("existing-source-manifest", "", "existing immutable source manifest to verify before replay")
		sourceManifestOnly         = flag.Bool("source-manifest-only", false, "write -source-manifest then exit without replaying")
		tuning                     = flag.String("tuning", "", "tuning JSON; empty uses embedded defaults")
		sensorID                   = flag.String("sensor", "hesai-pandar40p", "replay sensor identity")
		duration                   = flag.Float64("duration", 0, "scoring duration in seconds; 0 replays each full case")
		// Marina's first capture reaches the configured L3 convergence threshold
		// at 56.5 seconds. Keep a measured 20% margin so the default preserves
		// the fail-closed scoring-boundary invariant across the Phase 0 corpus.
		warmup          = flag.Float64("warmup", 70, "warm-up seconds before scoring")
		requireSettled  = flag.Bool("require-settled", true, "reject a case whose L3 background is unsettled at the scoring boundary")
		observations    = flag.String("observations-db", "", "optional SQLite database for the first run's immutable observations")
		surfaceGround   = flag.Bool("surface-ground", false, "enable P11 surface-relative ground clipping")
		measurementMode = flag.String("measurement-mode", string(l5tracks.MeasurementOBBCentreV1), "replay position model: obb_centre_v1 candidate or medoid_v0 reference")
	)
	flag.Parse()
	if *sourceManifestOnly && *sourceManifestPath == "" {
		fatal(fmt.Errorf("-source-manifest-only requires -source-manifest"))
	}
	if *sourceManifestPath != "" && *existingSourceManifestPath != "" {
		fatal(fmt.Errorf("-source-manifest and -existing-source-manifest are mutually exclusive"))
	}
	if !*sourceManifestOnly && *outDir == "" {
		fatal(fmt.Errorf("-out is required"))
	}
	if *observations != "" && *sourceManifestPath == "" && *existingSourceManifestPath == "" {
		fatal(fmt.Errorf("-observations-db requires -source-manifest so persisted evidence has immutable source identity"))
	}
	if !*sourceManifestOnly {
		if err := ensureEmptyDir(*outDir); err != nil {
			fatal(err)
		}
	}
	selected, err := readCorpus(*corpusPath)
	if err != nil {
		fatal(err)
	}
	index, err := readIndex(*indexPath)
	if err != nil {
		fatal(err)
	}
	resolvedCases, err := resolveCorpusCases(selected, index, *pcapRoot, *pcapSubdir)
	if err != nil {
		fatal(err)
	}
	var sourceManifestSHA256 string
	if *sourceManifestPath != "" {
		manifest, err := buildSourceManifest(*corpusPath, *indexPath, *pcapRoot, *tuning, *sensorID, resolvedCases)
		if err != nil {
			fatal(err)
		}
		digest, err := writeSourceManifest(*sourceManifestPath, manifest)
		if err != nil {
			fatal(err)
		}
		sourceManifestSHA256 = digest
		fmt.Printf("wrote immutable source manifest %s (%s)\n", *sourceManifestPath, sourceManifestSHA256)
	}
	if *existingSourceManifestPath != "" {
		manifest, err := buildSourceManifest(*corpusPath, *indexPath, *pcapRoot, *tuning, *sensorID, resolvedCases)
		if err != nil {
			fatal(err)
		}
		sourceManifestSHA256, err = verifySourceManifest(*existingSourceManifestPath, manifest)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("verified immutable source manifest %s (%s)\n", *existingSourceManifestPath, sourceManifestSHA256)
	}
	if *sourceManifestOnly {
		return
	}

	summaries := make([]caseSummary, 0, len(selected.Cases))
	for _, resolved := range resolvedCases {
		selectedCase := resolved.corpusCase
		paths := resolved.paths
		caseOut := filepath.Join(*outDir, selectedCase.ID)
		first := replayeval.Config{
			PCAPFiles: paths, OutDir: filepath.Join(caseOut, "first"), TuningFile: *tuning,
			SensorID: *sensorID, UDPPort: 2369, StartSeconds: *warmup, WarmupSeconds: *warmup,
			DurationSeconds: *duration, RequireSettled: *requireSettled, UseSurfaceGround: *surfaceGround,
			MeasurementSourceMode: l5tracks.MeasurementSource(*measurementMode),
		}
		if *observations != "" {
			first.ObservationDBPath = *observations
			first.ReplayCaseID = selectedCase.ID
			first.ObservationCalibration = identityCalibration(*sensorID)
			first.ObservationMaxSamplePoints = 256
		}
		fmt.Printf("%s: first run across %d capture(s)\n", selectedCase.ID, len(paths))
		firstResult, err := replayeval.Run(first)
		if err != nil {
			fatal(fmt.Errorf("first run %s: %w", selectedCase.ID, err))
		}
		repeat := first
		repeat.OutDir = filepath.Join(caseOut, "repeat")
		// Replaying the same evidence into the same immutable database must be
		// rejected. The repeat is intentionally read-only with respect to it.
		repeat.ObservationDBPath = ""
		repeat.ReplayCaseID = ""
		repeat.ObservationCalibration = l4bobserve.Calibration{}
		repeat.ObservationMaxSamplePoints = 0
		fmt.Printf("%s: repeat run\n", selectedCase.ID)
		repeatResult, err := replayeval.Run(repeat)
		if err != nil {
			fatal(fmt.Errorf("repeat run %s: %w", selectedCase.ID, err))
		}
		firstBaseline, err := os.ReadFile(filepath.Join(first.OutDir, "tracking_baseline.json"))
		if err != nil {
			fatal(err)
		}
		repeatBaseline, err := os.ReadFile(filepath.Join(repeat.OutDir, "tracking_baseline.json"))
		if err != nil {
			fatal(err)
		}
		if !bytes.Equal(firstBaseline, repeatBaseline) {
			fatal(fmt.Errorf("baseline differs on repeat for %s", selectedCase.ID))
		}
		summaries = append(summaries, caseSummary{
			ID: selectedCase.ID, Captures: len(paths), DurationSeconds: *duration,
			FirstRunFrames: firstResult.FramesRecorded, RepeatRunFrames: repeatResult.FramesRecorded,
			BaselineEqual: true, ObservationSourceID: firstResult.ObservationSourceID,
			MeasurementSourceMode: string(first.MeasurementSourceMode),
		})
	}
	b, err := json.MarshalIndent(struct {
		SchemaVersion        int           `json:"schema_version"`
		SourceManifestSHA256 string        `json:"source_manifest_sha256,omitempty"`
		Cases                []caseSummary `json:"cases"`
	}{SchemaVersion: 1, SourceManifestSHA256: sourceManifestSHA256, Cases: summaries}, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "phase0-summary.json"), append(b, '\n'), 0644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %d repeat-verified baseline(s) to %s\n", len(summaries), *outDir)
}

func identityCalibration(sensorID string) l4bobserve.Calibration {
	return l4bobserve.Calibration{SensorID: sensorID, FromFrame: "sensor", ToFrame: "site",
		Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
}

func ensureEmptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) != 0 {
		return fmt.Errorf("output directory must be empty: %s", dir)
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect output directory: %w", err)
	}
	return os.MkdirAll(dir, 0755)
}

func readCorpus(name string) (corpus, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return corpus{}, fmt.Errorf("read corpus: %w", err)
	}
	var value corpus
	if err := json.Unmarshal(b, &value); err != nil {
		return corpus{}, fmt.Errorf("decode corpus: %w", err)
	}
	if len(value.Cases) == 0 {
		return corpus{}, fmt.Errorf("corpus has no cases")
	}
	return value, nil
}

func readIndex(name string) (map[string]indexEntry, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	var entries []indexEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}
	result := make(map[string]indexEntry, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.ID) != "" {
			result[entry.ID] = entry
		}
	}
	return result, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "lidar-state-estimation-baseline:", err)
	os.Exit(1)
}
