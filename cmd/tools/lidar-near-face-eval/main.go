// Command lidar-near-face-eval scores where estimated bodies put the faces the
// sensor sees, against the returns a person labelled on those faces.
//
// The per-frame evaluator (lidar-ground-truth-eval perframe) says whether
// tracks keep their identity. Its reference is the centre of the visible
// returns, which sits toward the sensor as the medoid does, so it cannot say
// whether a body estimate is accurate. Physical references would, and a person
// has to author them. This is the part of the answer the labels already give:
// put a reviewed mask's returns in an arm's believed body axes and compare the
// believed face with the surface the sensor measured.
//
//	lidar-near-face-eval -pack PACK -split-manifest SPLIT -split NAME -allow-tuning-split \
//	  -arm control=control/kirk0.db -arm track=track/kirk0.db -json faces.json -markdown faces.md
//
// An arm is one database of solid bodies (replay -experiment solid_body, or
// near_edge_track with it): only a solid body carries the heading and extents a
// face is placed from. Each arm is a database; name the version with the
// -source, -estimator, -observation-model and -param-hash flags when one holds
// several. The labels are bound as the per-frame evaluator binds them, through
// a frozen split's pins or a version 1 manifest, and a split that is not held
// out is scored only with -allow-tuning-split and says so.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// listFlag collects a repeated string flag.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lidar-near-face-eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	def := perframeeval.DefaultNearFaceOptions()
	pack := fs.String("pack", "", "annotation pack directory (required)")
	manifest := fs.String("split-manifest", "", "object-disjoint split manifest: a frozen split or a version 1 manifest (required)")
	split := fs.String("split", "", "split to score (required)")
	episodes := fs.String("episodes", "", "comma-separated episode IDs (default: every episode of the split)")
	allowTuning := fs.Bool("allow-tuning-split", false, "score a split whose role is not held_out; the report says it is not held out")
	includeProposed := fs.Bool("include-proposed", false, "also score proposed (unreviewed) masks as truth")
	ignorePartial := fs.Bool("ignore-partial-masks", false, "treat masks marked partial as uncertifiable (the client saves every mask as partial by default)")
	toleranceMs := fs.Float64("frame-tolerance-ms", float64(def.FrameToleranceNanos)/1e6, "how far a body's frame time may be from a sample's")
	gate := fs.Float64("gate-metres", def.GateSlackMetres, "a body's centre must be within this plus half the mask's footprint diagonal of the mask's footprint centre")
	minReturns := fs.Int("min-returns", def.MinReturns, "labelled returns a mask needs to place a face")
	quantile := fs.Float64("face-quantile", def.FaceQuantile, "share of the outermost returns ignored when taking a face's extreme (a whole count)")
	band := fs.Float64("face-band-metres", def.FaceBandMetres, "how deep from the extreme an end face's returns are taken when measuring its span")
	minSpan := fs.Float64("min-span-coverage", def.MinSpanCoverage, "least share of the believed width an end face's span must cover for the tangent residual")
	maxSpan := fs.Float64("max-span-coverage", def.MaxSpanCoverage, "most share of the believed width an end face's span may cover")
	sensorX := fs.Float64("sensor-x", 0, "sensor x in the pack's frame, metres")
	sensorY := fs.Float64("sensor-y", 0, "sensor y in the pack's frame, metres")
	instants := fs.Bool("instants", false, "keep every scored instant in the JSON")
	stage := fs.String("stage", perframeeval.StageOnline, "estimate stage of every arm; anything but final is scored as a declared baseline")
	source := fs.String("source", "", "lidar_track_solid_bodies source_id (default: the only one that matches)")
	estimator := fs.String("estimator", "", "estimator_id (default: the only one that matches)")
	model := fs.String("observation-model", "", "observation_model_id (default: the only one that matches)")
	params := fs.String("param-hash", "", "param_hash (default: the only one that matches)")
	jsonPath := fs.String("json", "", "report JSON path (default: stdout)")
	markdownPath := fs.String("markdown", "", "reading copy path (optional)")
	var arms listFlag
	fs.Var(&arms, "arm", "an arm as label=database; repeat for each arm, compared with the first (required)")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lidar-near-face-eval -pack DIR -split-manifest FILE -split NAME -arm LABEL=DB [-arm LABEL=DB ...] [flags]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		return usage(fs, stderr, fmt.Errorf("unexpected arguments: %v", fs.Args()))
	}
	for _, r := range []struct{ name, value string }{{"-pack", *pack}, {"-split-manifest", *manifest}, {"-split", *split}} {
		if r.value == "" {
			return usage(fs, stderr, fmt.Errorf("%s is required", r.name))
		}
	}
	if len(arms) == 0 {
		return usage(fs, stderr, fmt.Errorf("-arm is required: name at least one arm as label=database"))
	}
	var specs []perframeeval.ArmSpec
	seen := map[string]bool{}
	for _, a := range arms {
		label, path, ok := strings.Cut(a, "=")
		if !ok || label == "" || path == "" {
			return usage(fs, stderr, fmt.Errorf("-arm %q: want label=database", a))
		}
		if seen[label] {
			return usage(fs, stderr, fmt.Errorf("-arm %q: the label is used twice", label))
		}
		seen[label] = true
		specs = append(specs, perframeeval.ArmSpec{
			Label: label, DBPath: path, SourceID: *source, EstimatorID: *estimator, ObservationModelID: *model,
			ParamHash: *params, Stage: *stage, DeclaredBaseline: *stage != perframeeval.StageFinal,
		})
	}

	opts := def
	opts.PackDir, opts.SplitManifestPath, opts.Split, opts.AllowTuningSplit = *pack, *manifest, *split, *allowTuning
	for _, id := range strings.Split(*episodes, ",") {
		if id = strings.TrimSpace(id); id != "" {
			opts.Episodes = append(opts.Episodes, id)
		}
	}
	opts.Policy = annotation.DefaultReferencePolicy()
	if *includeProposed {
		opts.Policy.Status = annotation.ReferenceIncludeProposed
	}
	opts.Policy.ScorePartialMasks = !*ignorePartial
	opts.FrameToleranceNanos, opts.GateSlackMetres = int64(*toleranceMs*1e6), *gate
	opts.MinReturns, opts.FaceQuantile, opts.FaceBandMetres = *minReturns, *quantile, *band
	opts.MinSpanCoverage, opts.MaxSpanCoverage = *minSpan, *maxSpan
	opts.SensorXM, opts.SensorYM, opts.IncludeInstants = *sensorX, *sensorY, *instants

	report, err := perframeeval.ScoreNearFaces(opts, specs)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "error: encode report: %v\n", err)
		return 1
	}
	payload = append(payload, '\n')
	if *jsonPath == "" {
		if _, err := stdout.Write(payload); err != nil {
			fmt.Fprintf(stderr, "error: write report: %v\n", err)
			return 1
		}
	} else if err := os.WriteFile(*jsonPath, payload, 0o644); err != nil {
		fmt.Fprintf(stderr, "error: write %s: %v\n", *jsonPath, err)
		return 1
	}
	if *markdownPath != "" {
		if err := os.WriteFile(*markdownPath, []byte(perframeeval.RenderNearFaceMarkdown(*report)), 0o644); err != nil {
			fmt.Fprintf(stderr, "error: write %s: %v\n", *markdownPath, err)
			return 1
		}
	}
	summarise(stderr, report)
	return 0
}

// summarise prints the headline of each arm: what was scored, and the end and
// side face residuals over all of it.
func summarise(w io.Writer, r *perframeeval.NearFaceReport) {
	fmt.Fprintf(w, "labels: pack %s revision %d, split %s (held out: %v), episodes %s\n",
		r.Reference.PackDigest, r.Reference.SidecarRevision, r.Reference.Split, r.Reference.HeldOut, strings.Join(r.Reference.Episodes, ","))
	for _, a := range r.Arms {
		acc := a.Accounting
		line := fmt.Sprintf("%s: %d labelled, %d matched, %d scored", a.Arm.Label, acc.Masks, acc.Matched, acc.Scored)
		for _, s := range a.Strata {
			if s.Name != "all" {
				continue
			}
			line += fmt.Sprintf("; end face mean %+.3f m (n=%d, p95 abs %.3f), side face mean %+.3f m (n=%d), end tangent mean abs %.3f m (n=%d)",
				s.EndNormal.Mean, s.EndNormal.N, s.EndNormal.P95Abs, s.SideNormal.Mean, s.SideNormal.N, s.EndTangent.MeanAbs, s.EndTangent.N)
		}
		fmt.Fprintln(w, line)
	}
	for _, c := range r.Caveats {
		fmt.Fprintf(w, "caveat: %s\n", c)
	}
}

func usage(fs *flag.FlagSet, w io.Writer, err error) int {
	fmt.Fprintf(w, "error: %v\n", err)
	fs.Usage()
	return 2
}
