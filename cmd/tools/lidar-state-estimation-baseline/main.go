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
	"math"
	"os"
	"path/filepath"
	"sort"
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
	// NorthAzimuthDeg is the sensor's operator-measured compass bearing
	// (clockwise from true north, 0-360) from the "Align from above" scene
	// tool: see tools/s2-archive/README.md. It is the only real per-site
	// extrinsic orientation this index carries today.
	NorthAzimuthDeg float64 `json:"north_azimuth_deg"`
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
	// Ground surface fields are populated only when -surface-ground was
	// passed and the background settled in time to fit a P11 ground plane
	// for this case; omitted otherwise.
	GroundSurfaceSupport       int     `json:"ground_surface_support,omitempty"`
	GroundSurfaceGradientMetre float64 `json:"ground_surface_gradient_metre,omitempty"`
	GroundSurfaceRMSEMetres    float64 `json:"ground_surface_rmse_metres,omitempty"`
	GroundSurfaceRegionCount   int     `json:"ground_surface_region_count,omitempty"`
	GroundSurfaceCellMetres    float64 `json:"ground_surface_cell_metres,omitempty"`
}

func main() {
	var (
		corpusPath                 = flag.String("corpus", "tools/s2-archive/state-estimation-phase01-corpus.json", "committed Phase 0 corpus JSON")
		indexPath                  = flag.String("index", "tools/s2-archive/site-index.json", "capture archive index JSON")
		pcapRoot                   = flag.String("pcap-root", "/Volumes/lidar/lidar", "directory holding archive capture subdirectories")
		pcapSubdir                 = flag.String("pcap-subdir", "s2", "archive capture subdirectory")
		outDir                     = flag.String("out", "", "empty output directory for baseline recordings (required)")
		evidenceDir                = flag.String("evidence-dir", "", "empty directory for observations.db; may be on a different volume from -pcap-root and -out")
		sourceManifestPath         = flag.String("source-manifest", "", "new immutable JSON manifest of ordered source PCAP hashes")
		existingSourceManifestPath = flag.String("existing-source-manifest", "", "existing immutable source manifest to verify before replay")
		sourceManifestOnly         = flag.Bool("source-manifest-only", false, "write -source-manifest then exit without replaying")
		tuning                     = flag.String("tuning", "", "tuning JSON; empty uses embedded defaults")
		sensorID                   = flag.String("sensor", "hesai-pandar40p", "replay sensor identity")
		duration                   = flag.Float64("duration", 0, "scoring duration in seconds; 0 replays each full case")
		// Marina's first capture reaches the configured L3 convergence threshold
		// at 56.5 seconds. Keep a measured 20% margin so the default preserves
		// the fail-closed scoring-boundary invariant across the Phase 0 corpus.
		warmup              = flag.Float64("warmup", 70, "warm-up seconds before scoring")
		requireSettled      = flag.Bool("require-settled", true, "reject a case whose L3 background is unsettled at the scoring boundary")
		observations        = flag.String("observations-db", "", "optional SQLite database for the first run's immutable observations")
		evidenceProfile     = flag.Bool("evidence-profile", false, "print accumulated SQLite frame-evidence timings after each first replay")
		surfaceGround       = flag.Bool("surface-ground", false, "enable P11 surface-relative ground clipping")
		surfaceGroundRegion = flag.Float64("surface-ground-region-metres", 0, "P11 ground-plane region cell size in metres; 0 uses l3grid.DefaultRegionSizeMetres")
		measurementMode     = flag.String("measurement-mode", string(l5tracks.MeasurementOBBCentreV1), "replay position model: obb_centre_v1 candidate or medoid_v0 reference")
		caseFilter          = flag.String("case", "", "replay only these corpus case IDs (comma separated); empty replays every case")
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
	var observationDBPath string
	var err error
	if !*sourceManifestOnly {
		observationDBPath, err = resolveObservationDBPath(*observations, *evidenceDir, *outDir)
		if err != nil {
			fatal(err)
		}
	}
	if observationDBPath != "" && *sourceManifestPath == "" && *existingSourceManifestPath == "" {
		fatal(fmt.Errorf("an evidence output requires -source-manifest so persisted evidence has immutable source identity"))
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
	// Case selection narrows the corpus before anything is resolved or hashed,
	// so a single-case run writes a manifest describing exactly that case
	// rather than one that claims the whole corpus.
	if *caseFilter != "" {
		selected, err = filterCorpusCases(selected, *caseFilter)
		if err != nil {
			fatal(err)
		}
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
	var verifiedSourceManifest *sourceManifest
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
		verifiedSourceManifest = &manifest
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
		verifiedSourceManifest = &manifest
		fmt.Printf("verified immutable source manifest %s (%s)\n", *existingSourceManifestPath, sourceManifestSHA256)
	}
	if *sourceManifestOnly {
		return
	}

	summaries := make([]caseSummary, 0, len(selected.Cases))
	for _, resolved := range resolvedCases {
		selectedCase := resolved.corpusCase
		paths := resolved.paths
		sequence, err := replayeval.PrepareCaptureSequence(paths, 2369)
		if err != nil {
			fatal(fmt.Errorf("prepare capture sequence %s: %w", selectedCase.ID, err))
		}
		caseOut := filepath.Join(*outDir, selectedCase.ID)
		first := replayeval.Config{
			PCAPFiles: paths, OutDir: filepath.Join(caseOut, "first"), TuningFile: *tuning,
			SensorID: *sensorID, UDPPort: 2369, StartSeconds: *warmup, WarmupSeconds: *warmup,
			DurationSeconds: *duration, RequireSettled: *requireSettled, UseSurfaceGround: *surfaceGround,
			SurfaceGroundRegionMetres: *surfaceGroundRegion,
			MeasurementSourceMode:     l5tracks.MeasurementSource(*measurementMode), CaptureSequence: sequence,
		}
		if verifiedSourceManifest != nil {
			first.PCAPSHA256s, err = sourceManifestCaseDigests(*verifiedSourceManifest, selectedCase.ID, len(paths))
			if err != nil {
				fatal(err)
			}
		}
		if observationDBPath != "" {
			first.ObservationDBPath = observationDBPath
			first.ReplayCaseID = selectedCase.ID
			first.ObservationCalibration = siteCalibration(*sensorID, index[selectedCase.ID].NorthAzimuthDeg)
			first.ObservationMaxSamplePoints = 256
			first.ProfileEvidence = *evidenceProfile
		}
		fmt.Printf("%s: first run across %d capture(s)\n", selectedCase.ID, len(paths))
		firstResult, err := replayeval.Run(first)
		if err != nil {
			fatal(fmt.Errorf("first run %s: %w", selectedCase.ID, err))
		}
		if firstResult.EvidencePersistence != nil {
			stats := firstResult.EvidencePersistence
			fmt.Printf("%s: evidence profile frames=%d empty_frames=%d observations=%d estimates=%d begin=%s prepare=%s execute=%s commit=%s\n",
				selectedCase.ID, stats.Frames, stats.EmptyFrames, stats.Observations, stats.Estimates,
				stats.Begin, stats.Prepare, stats.Execute, stats.Commit)
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
		summary := caseSummary{
			ID: selectedCase.ID, Captures: len(paths), DurationSeconds: *duration,
			FirstRunFrames: firstResult.FramesRecorded, RepeatRunFrames: repeatResult.FramesRecorded,
			BaselineEqual: true, ObservationSourceID: firstResult.ObservationSourceID,
			MeasurementSourceMode: string(first.MeasurementSourceMode),
		}
		if fit := firstResult.GroundSurfaceFit; fit != nil {
			summary.GroundSurfaceSupport = fit.Global.Support
			summary.GroundSurfaceGradientMetre = fit.Global.GradientMetre
			summary.GroundSurfaceRMSEMetres = fit.Global.RMSEMetres
			summary.GroundSurfaceRegionCount = fit.RegionCount
			summary.GroundSurfaceCellMetres = fit.CellMetres
			fmt.Printf("%s: ground surface support=%d gradient=%.4f rmse=%.3fm regions=%d cell=%.1fm\n",
				selectedCase.ID, fit.Global.Support, fit.Global.GradientMetre, fit.Global.RMSEMetres,
				fit.RegionCount, fit.CellMetres)
		} else if *surfaceGround {
			fmt.Printf("%s: -surface-ground set but no ground plane was fit (background did not settle in time)\n", selectedCase.ID)
		}
		summaries = append(summaries, summary)
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

// siteCalibration is identityCalibration's per-site replacement for the
// evidence a real replay records: a pure yaw rotation built from the
// sensor's operator-measured compass bearing (site-index.json's
// north_azimuth_deg, from the "Align from above" scene tool — see
// tools/s2-archive/README.md), rather than every site sharing one
// placeholder identity transform.
//
// Convention: site frame is local East-North-Up (+X east, +Y north, +Z
// up); the sensor's origin is the frame origin; northAzimuthDeg is the
// sensor's own azimuth-zero direction as a standard compass bearing
// (clockwise from north, degrees). This is an explicit, reasonable
// default — not yet cross-checked against the alignment tool's own axis
// convention — safe today because nothing applies Transform to actual
// points; CalibrationID only hashes it as content, so a wrong sign
// changes an identity string, not a measurement.
//
// Height and mounting tilt are deliberately not encoded here. A per-run
// ground-plane fit (l3grid.RegionalGroundSurface) can estimate them, but
// baking a measurement into an identity value would make calibration_id
// wobble with ordinary measurement noise between otherwise-identical
// replays of the same site; report that estimate as evidence instead (see
// caseSummary's GroundSurface fields), not as part of this identity.
func siteCalibration(sensorID string, northAzimuthDeg float64) l4bobserve.Calibration {
	mathRad := (90 - northAzimuthDeg) * math.Pi / 180
	c, s := math.Cos(mathRad), math.Sin(mathRad)
	return l4bobserve.Calibration{SensorID: sensorID, FromFrame: "sensor", ToFrame: "site",
		Transform: [16]float64{
			c, -s, 0, 0,
			s, c, 0, 0,
			0, 0, 1, 0,
			0, 0, 0, 1,
		}}
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

func resolveObservationDBPath(observationsDB, evidenceDir, outDir string) (string, error) {
	if observationsDB != "" && evidenceDir != "" {
		return "", fmt.Errorf("-observations-db and -evidence-dir are mutually exclusive")
	}
	if evidenceDir == "" {
		return observationsDB, nil
	}
	if samePath(evidenceDir, outDir) {
		return "", fmt.Errorf("-evidence-dir must differ from -out so recorded artefacts and immutable evidence remain separate")
	}
	if err := ensureEmptyDir(evidenceDir); err != nil {
		return "", fmt.Errorf("prepare evidence directory: %w", err)
	}
	return filepath.Join(evidenceDir, "observations.db"), nil
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	left, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	right, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func sourceManifestCaseDigests(manifest sourceManifest, caseID string, captureCount int) ([]string, error) {
	for _, manifestCase := range manifest.Cases {
		if manifestCase.ID != caseID {
			continue
		}
		if len(manifestCase.Captures) != captureCount {
			return nil, fmt.Errorf("source manifest case %q has %d captures, want %d", caseID, len(manifestCase.Captures), captureCount)
		}
		digests := make([]string, captureCount)
		for ordinal, capture := range manifestCase.Captures {
			if capture.Ordinal != ordinal {
				return nil, fmt.Errorf("source manifest case %q has capture ordinal %d at position %d", caseID, capture.Ordinal, ordinal)
			}
			digests[ordinal] = capture.SHA256
		}
		return digests, nil
	}
	return nil, fmt.Errorf("source manifest has no case %q", caseID)
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

// filterCorpusCases keeps only the named cases, preserving the corpus order.
//
// Order is preserved rather than following the argument, because the corpus
// file is what declares replay order and a caller reordering cases on the
// command line would change what the run means. An unknown ID is an error
// rather than an empty selection: silently replaying nothing, and writing a
// manifest to prove it, is the worst available outcome.
func filterCorpusCases(value corpus, list string) (corpus, error) {
	wanted := map[string]bool{}
	for _, raw := range strings.Split(list, ",") {
		if id := strings.TrimSpace(raw); id != "" {
			wanted[id] = true
		}
	}
	if len(wanted) == 0 {
		return corpus{}, fmt.Errorf("-case was given but names no case")
	}

	kept := make([]corpusCase, 0, len(wanted))
	for _, c := range value.Cases {
		if wanted[c.ID] {
			kept = append(kept, c)
			delete(wanted, c.ID)
		}
	}
	if len(wanted) > 0 {
		missing := make([]string, 0, len(wanted))
		for id := range wanted {
			missing = append(missing, id)
		}
		sort.Strings(missing)
		available := make([]string, 0, len(value.Cases))
		for _, c := range value.Cases {
			available = append(available, c.ID)
		}
		return corpus{}, fmt.Errorf("corpus has no case(s) %s; available: %s",
			strings.Join(missing, ", "), strings.Join(available, ", "))
	}

	value.Cases = kept
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
