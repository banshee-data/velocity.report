//go:build pcap

package lidar

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

type clipOperations struct {
	run          func(replayeval.Config) (*replayeval.Result, error)
	readManifest func(string) ([]byte, error)
	export       func(annotation.ExportConfig) (*annotation.Pack, error)
	writeRecord  func(string, segments.Record) error
}

func AnnotationClipMain(args []string) int {
	return annotationClipMain(args, clipOperations{replayeval.Run, os.ReadFile, annotation.Export, segments.WriteRecord})
}

func annotationClipMain(args []string, ops clipOperations) int {
	fs := flag.NewFlagSet("velocity-lidar-annotation-clip", flag.ContinueOnError)
	pcap := fs.String("pcap", "", "Capture file, or comma-separated continuous captures")
	output := fs.String("output", "", "New output directory containing vrlog and pack")
	start := fs.Float64("start-seconds", 0, "Offset from start of capture sequence")
	duration := fs.Float64("duration-seconds", 0, "Length of clip in seconds")
	warmup := fs.Float64("warmup-seconds", 35, "Warm-up prefix, clamped to the start offset")
	port := fs.Int("port", 0, "UDP port; zero detects from first capture")
	configPath := fs.String("config", config.DefaultConfigPath, "Pipeline tuning config")
	sensor := fs.String("sensor-id", "pcap-replay", "Sensor identity")
	role := fs.String("role", "tuning", "tuning or held_out")
	selection := fs.String("selection", "", "JSON report written by velocity lidar segments")
	segmentID := fs.String("segment-id", "", "Window ID in --selection (optional when report has one window)")
	observations := fs.Bool("observations", false, "Also write a VRLOG 1.x observation container")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *pcap == "" || *output == "" || !finiteClip(*start) || *start < 0 || !finiteClip(*duration) || *duration <= 0 || !finiteClip(*warmup) || *warmup < 0 {
		fmt.Fprintln(os.Stderr, "annotation-clip: --pcap, --output, nonnegative --start-seconds and positive --duration-seconds are required")
		return 2
	}
	if *role != "tuning" && *role != "held_out" {
		fmt.Fprintln(os.Stderr, "annotation-clip: role must be tuning or held_out")
		return 2
	}
	files := strings.Split(*pcap, ",")
	for i := range files {
		files[i] = strings.TrimSpace(files[i])
		if files[i] == "" {
			fmt.Fprintln(os.Stderr, "annotation-clip: empty capture path")
			return 2
		}
	}
	var chosen segments.Window
	selectionParams := segments.DefaultParams()
	if *selection != "" {
		var err error
		chosen, selectionParams, err = readClipSelection(*selection, *segmentID, *role)
		if err != nil {
			fmt.Fprintf(os.Stderr, "annotation-clip: selection: %v\n", err)
			return 2
		}
		if chosen.Capture != "" && filepath.Clean(chosen.Capture) != filepath.Clean(files[0]) {
			fmt.Fprintln(os.Stderr, "annotation-clip: selected segment belongs to another capture")
			return 2
		}
		if math.Abs(chosen.OffsetSeconds-*start) > 0.11 || math.Abs(float64(chosen.EndNs-chosen.StartNs)/1e9-*duration) > 0.11 {
			fmt.Fprintln(os.Stderr, "annotation-clip: selected segment does not match replay window")
			return 2
		}
	} else if *role == "held_out" {
		fmt.Fprintln(os.Stderr, "annotation-clip: held-out cuts require a traffic or random --selection")
		return 2
	}
	if _, err := os.Stat(*output); err == nil {
		fmt.Fprintln(os.Stderr, "annotation-clip: output already exists")
		return 2
	} else if !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resolvedPort := resolveUDPPort(*port, files[0])
	if resolvedPort < 0 {
		return 1
	}
	if err := os.Mkdir(*output, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	vrlog := filepath.Join(*output, "vrlog")
	cfg := replayeval.Config{OutDir: vrlog, TuningFile: *configPath, SensorID: *sensor, UDPPort: resolvedPort, StartSeconds: *start, DurationSeconds: *duration, WarmupSeconds: math.Min(*warmup, *start), RequireSettled: true, IncludePoints: true, ProgressEvery: 200}
	if len(files) == 1 {
		cfg.PCAPFile = files[0]
	} else {
		cfg.PCAPFiles = files
	}
	if *observations {
		cfg.ObservationLogDir = filepath.Join(*output, "observations.vrlog")
		cfg.ReplayCaseID = "annotation-clip-" + filepath.Base(*output)
		cfg.ObservationCalibration = replayCalibration(*sensor)
	}
	result, err := ops.run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "annotation-clip: replay: %v (partial output: %s)\n", err, *output)
		return 1
	}
	manifestBytes, err := ops.readManifest(filepath.Join(result.VRLOGPath, "replay_manifest.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var manifest struct {
		ScoringStartNs         int64   `json:"scoring_start_ns"`
		ScoringDurationSeconds float64 `json:"scoring_duration_seconds"`
	}
	if err = json.Unmarshal(manifestBytes, &manifest); err != nil || manifest.ScoringStartNs <= 0 || manifest.ScoringDurationSeconds <= 0 {
		fmt.Fprintln(os.Stderr, "annotation-clip: replay manifest has no valid scoring bounds")
		return 1
	}
	end := manifest.ScoringStartNs + int64(manifest.ScoringDurationSeconds*1e9)
	pack, err := ops.export(annotation.ExportConfig{VRLOGPath: result.VRLOGPath, OutDir: filepath.Join(*output, "pack"), StartNs: manifest.ScoringStartNs, EndNs: end, MaxSamples: 0, Coverage: annotation.CoverageForegroundOnly, CoverageNote: "publisher records foreground points and periodic background snapshots"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "annotation-clip: export: %v (recording: %s)\n", err, result.VRLOGPath)
		return 1
	}
	if *selection == "" {
		chosen = segments.Window{Finder: "manual", Version: 1, Source: strings.Join(files, ","), StartNs: manifest.ScoringStartNs, EndNs: end, PeakNs: manifest.ScoringStartNs, Status: "packed"}
	}
	r := segments.Record{Schema: "velocity.report/annotation-segment", SchemaVersion: 1, PackDigest: pack.Manifest.PackDigest, Role: *role, Finder: chosen.Finder, FinderVersion: chosen.Version, Parameters: selectionParams, Segment: chosen}
	if err = ops.writeRecord(pack.Dir, r); err != nil {
		fmt.Fprintf(os.Stderr, "annotation-clip: segment record: %v\n", err)
		return 1
	}
	fmt.Printf("vrlog: %s\npack: %s\nsegment: %s\n", result.VRLOGPath, pack.Dir, filepath.Join(pack.Dir, "segment.json"))
	return 0
}

func finiteClip(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func readClipSelection(path, id, role string) (segments.Window, segments.Params, error) {
	var report struct {
		Schema     string            `json:"schema"`
		Source     string            `json:"source"`
		Finder     string            `json:"finder"`
		Version    int               `json:"version"`
		Role       string            `json:"role"`
		Parameters segments.Params   `json:"parameters"`
		Windows    []segments.Window `json:"windows"`
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return segments.Window{}, segments.Params{}, err
	}
	if err := json.Unmarshal(b, &report); err != nil {
		return segments.Window{}, segments.Params{}, err
	}
	if report.Schema != "velocity.report/segments" || report.Version != segments.Version || report.Role != role || !segments.Allowed(report.Finder, role) {
		return segments.Window{}, segments.Params{}, fmt.Errorf("report version, finder or role is invalid")
	}
	if err := report.Parameters.Validate(); err != nil {
		return segments.Window{}, segments.Params{}, err
	}
	if id == "" && len(report.Windows) != 1 {
		return segments.Window{}, segments.Params{}, fmt.Errorf("--segment-id is required when the report has %d windows", len(report.Windows))
	}
	for _, window := range report.Windows {
		if id == "" || id == window.ID {
			if window.ID != segments.Identity(report.Finder, report.Source, report.Role, report.Parameters, window.StartNs) || window.Finder != report.Finder || window.Version != report.Version || window.Source != report.Source || window.Role != report.Role || window.EndNs-window.StartNs != int64(report.Parameters.WindowSeconds*1e9) || window.Capture == "" {
				return segments.Window{}, segments.Params{}, fmt.Errorf("selected window does not match report or lacks an indexed capture")
			}
			return window, report.Parameters, nil
		}
	}
	return segments.Window{}, segments.Params{}, fmt.Errorf("segment %q is not in report", id)
}
