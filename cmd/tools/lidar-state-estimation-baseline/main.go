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
		corpusPath      = flag.String("corpus", "tools/s2-archive/state-estimation-phase01-corpus.json", "committed Phase 0 corpus JSON")
		indexPath       = flag.String("index", "tools/s2-archive/site-index.json", "capture archive index JSON")
		pcapRoot        = flag.String("pcap-root", "/Volumes/lidar/lidar", "directory holding archive capture subdirectories")
		pcapSubdir      = flag.String("pcap-subdir", "s2", "archive capture subdirectory")
		outDir          = flag.String("out", "", "empty output directory for baseline recordings (required)")
		tuning          = flag.String("tuning", "", "tuning JSON; empty uses embedded defaults")
		sensorID        = flag.String("sensor", "hesai-pandar40p", "replay sensor identity")
		duration        = flag.Float64("duration", 0, "scoring duration in seconds; 0 replays each full case")
		warmup          = flag.Float64("warmup", 30, "warm-up seconds before scoring")
		requireSettled  = flag.Bool("require-settled", true, "reject a case whose L3 background is unsettled at the scoring boundary")
		observations    = flag.String("observations-db", "", "optional SQLite database for the first run's immutable observations")
		surfaceGround   = flag.Bool("surface-ground", false, "enable P11 surface-relative ground clipping")
		measurementMode = flag.String("measurement-mode", string(l5tracks.MeasurementOBBCentreV1), "replay position model: obb_centre_v1 candidate or medoid_v0 reference")
	)
	flag.Parse()
	if *outDir == "" {
		fatal(fmt.Errorf("-out is required"))
	}
	if err := ensureEmptyDir(*outDir); err != nil {
		fatal(err)
	}
	selected, err := readCorpus(*corpusPath)
	if err != nil {
		fatal(err)
	}
	index, err := readIndex(*indexPath)
	if err != nil {
		fatal(err)
	}

	summaries := make([]caseSummary, 0, len(selected.Cases))
	for _, selectedCase := range selected.Cases {
		entry, ok := index[selectedCase.ID]
		if !ok {
			fatal(fmt.Errorf("corpus case %q is absent from index", selectedCase.ID))
		}
		if len(entry.Captures) != selectedCase.ExpectedCaptureCount {
			fatal(fmt.Errorf("corpus case %q declares %d captures, index has %d", selectedCase.ID, selectedCase.ExpectedCaptureCount, len(entry.Captures)))
		}
		paths := make([]string, len(entry.Captures))
		for i, capture := range entry.Captures {
			paths[i] = filepath.Join(*pcapRoot, *pcapSubdir, capture)
			if _, err := os.Stat(paths[i]); err != nil {
				fatal(fmt.Errorf("case %s capture %s: %w", selectedCase.ID, paths[i], err))
			}
		}
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
		SchemaVersion int           `json:"schema_version"`
		Cases         []caseSummary `json:"cases"`
	}{SchemaVersion: 1, Cases: summaries}, "", "  ")
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
