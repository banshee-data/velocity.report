// Command lidar-refinement-eval replays one capture with the fixed_lag_rts
// experiment and reports causal tracking against fixed-assignment RTS at every
// comparison horizon (three frames; 0.5, 1 and 2 s; the whole track), all from
// the same replay. See docs/lidar/operations/retrospective-refinement-criteria.md
// for what the figures mean and the criteria a horizon must pass.
//
// With -evidence-db the replay also writes immutable observations, online
// estimates and every refined arm as its own estimate version, and the command
// prints the per-frame evaluator invocation that scores each arm against
// reviewed episodes. Put -out and -evidence-db on a different device from
// -pcap, as for every replay tool.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lidar-refinement-eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pcap := fs.String("pcap", "", "capture to replay (required)")
	outDir := fs.String("out", "", "empty output directory for the VRLOG and reports (required)")
	tuning := fs.String("tuning", "", "tuning JSON; empty uses the embedded defaults")
	sensor := fs.String("sensor", "pcap-replay", "replay sensor identity")
	port := fs.Int("port", 2369, "UDP port of the LiDAR data in the capture")
	start := fs.Float64("start", 20, "scoring start, seconds into the capture")
	warmup := fs.Float64("warmup", 20, "warm-up seconds processed before -start without scoring")
	duration := fs.Float64("duration", 0, "scoring duration in seconds; 0 scores to the end of the capture")
	experiments := fs.String("experiment", "", "further default-off options, comma separated ("+strings.Join(replayeval.KnownExperiments(), ", ")+")")
	evidenceDB := fs.String("evidence-db", "", "new SQLite database for observations, online and refined estimates; enables per-frame scoring")
	caseID := fs.String("case", "", "replay case ID for the evidence source identity (required with -evidence-db)")
	samplePoints := fs.Int("sample-points", 64, "retained points per stored observation (1-1024) with -evidence-db")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lidar-refinement-eval -pcap FILE -out DIR [-evidence-db DB -case ID] [flags]\n\n")
		fmt.Fprintf(stderr, "Replays a capture once and compares the online estimate with fixed-assignment RTS at\n")
		fmt.Fprintf(stderr, "three frames, 0.5 s, 1 s, 2 s and the whole track, on label-free figures.\n\nOptions:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *pcap == "" || *outDir == "" {
		fmt.Fprintln(stderr, "lidar-refinement-eval: -pcap and -out are required")
		fs.Usage()
		return 2
	}
	if (*evidenceDB == "") != (*caseID == "") {
		fmt.Fprintln(stderr, "lidar-refinement-eval: -evidence-db and -case go together")
		return 2
	}
	names, err := replayeval.ParseExperiments(*experiments)
	if err != nil {
		fmt.Fprintf(stderr, "lidar-refinement-eval: %v\n", err)
		return 2
	}
	cfg := replayeval.Config{
		PCAPFile: *pcap, OutDir: *outDir, TuningFile: *tuning, SensorID: *sensor, UDPPort: *port,
		StartSeconds: *start, WarmupSeconds: *warmup, DurationSeconds: *duration,
		Experiments: append(names, replayeval.ExperimentFixedLagRTS),
	}
	if *evidenceDB != "" {
		cfg.ObservationDBPath = *evidenceDB
		cfg.ReplayCaseID = *caseID
		cfg.ObservationCalibration = identityCalibration(*sensor)
		cfg.ObservationMaxSamplePoints = *samplePoints
	}
	result, err := replayeval.Run(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "lidar-refinement-eval: %v\n", err)
		return 1
	}
	report := result.Refinement
	table := l8analytics.RefinementComparisonMarkdown(report.Arms)
	markdownPath := filepath.Join(*outDir, "refinement_report.md")
	if err := os.WriteFile(markdownPath, []byte(table), 0o644); err != nil {
		fmt.Fprintf(stderr, "lidar-refinement-eval: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "replayed %d frames (%d recorded) in %s\n\n%s\n", result.FramesRead, result.FramesRecorded,
		result.Elapsed.Round(1e6), table)
	for _, s := range report.Smoothers {
		st := s.Stats
		fmt.Fprintf(stdout, "%-5s cap %d: %d released (lag %d, chain end %d, cap %d, barrier %d), truncated %d, clamped transitions %d, covariance fallbacks %d, revisions without evidence %d, peak held %d\n",
			s.Lag, s.WindowCap, st.Released, st.ReleasedByLag, st.ReleasedAtChainEnd, st.ReleasedByWindowCap,
			st.ReleasedAtBarrier, st.LookaheadTruncated, st.ClampedTransitions, st.CovarianceFallbacks,
			st.RevisionsWithoutEvidence, st.PeakHeldSteps)
	}
	fmt.Fprintf(stdout, "\nreport: %s\ntable:  %s\n", filepath.Join(*outDir, "refinement_report.json"), markdownPath)
	if report.Persisted {
		fmt.Fprintf(stdout, "\nScore each arm against reviewed episodes (fill in the pack and split):\n\n")
		for _, command := range perFrameCommands(*evidenceDB, report) {
			fmt.Fprintf(stdout, "%s\n\n", command)
		}
	}
	return 0
}

// perFrameCommands returns one lidar-ground-truth-eval perframe invocation per
// refined arm, each paired with the online arm on the same database. Every
// version field is named, so the evaluator never has to guess, and every arm
// here is a declared baseline: acceptance scores the stage the criteria
// document selects, not these comparisons.
func perFrameCommands(dbPath string, report *replayeval.RefinementReport) []string {
	online := report.Arms[0]
	var out []string
	for _, arm := range report.Arms[1:] {
		out = append(out, strings.Join([]string{
			"go run ./cmd/tools/lidar-ground-truth-eval perframe",
			"  -pack PACK_DIR -split-manifest SPLIT_JSON -split held_out",
			fmt.Sprintf("  -a-label online -a-db %s -a-source %s -a-estimator %s -a-observation-model %s -a-param-hash %s -a-stage %s -a-declared-baseline",
				dbPath, report.SourceID, online.EstimatorID, report.ObservationModelID, online.ParamHash, online.Stage),
			fmt.Sprintf("  -b-label %q -b-db %s -b-source %s -b-estimator %s -b-observation-model %s -b-param-hash %s -b-stage %s -b-declared-baseline",
				arm.Label, dbPath, report.SourceID, arm.EstimatorID, report.ObservationModelID, arm.ParamHash, arm.Stage),
			fmt.Sprintf("  -json refinement-perframe-%s.json -markdown refinement-perframe-%s.md", fileLabel(arm.Lag), fileLabel(arm.Lag)),
		}, " \\\n"))
	}
	return out
}

func fileLabel(lag string) string { return strings.ReplaceAll(lag, ".", "p") }

func identityCalibration(sensorID string) l4bobserve.Calibration {
	return l4bobserve.Calibration{SensorID: sensorID, FromFrame: "sensor", ToFrame: "site",
		Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
}
