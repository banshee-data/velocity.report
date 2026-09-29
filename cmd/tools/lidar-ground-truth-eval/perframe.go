package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
)

// The perframe subcommand: per-frame acceptance scoring of two arms against a
// reviewed, held-out annotation reference (gap-analysis M5). All of the
// decisions live in internal/lidar/perframeeval; this is flag parsing and
// output. The operations guide is docs/lidar/operations/per-frame-evaluation.md.

type armFlags struct {
	label, db, source, estimator, model, params, stage, runID *string
	declaredBaseline, solidBody                               *bool
}

func registerArm(fs *flag.FlagSet, name, defaultLabel string) armFlags {
	p := "-" + name + "-"
	return armFlags{
		label:            fs.String(name+"-label", defaultLabel, "label for arm "+strings.ToUpper(name)+" in the report"),
		db:               fs.String(name+"-db", "", "database holding arm "+strings.ToUpper(name)+" (required); opened read-only"),
		source:           fs.String(name+"-source", "", "lidar_track_estimates source_id (default: the only one that matches)"),
		estimator:        fs.String(name+"-estimator", "", "estimator_id (default: the only one that matches)"),
		model:            fs.String(name+"-observation-model", "", "observation_model_id (default: the only one that matches)"),
		params:           fs.String(name+"-param-hash", "", "param_hash (default: the only one that matches)"),
		stage:            fs.String(name+"-stage", "", "estimate stage (default final; anything else needs "+p+"declared-baseline)"),
		runID:            fs.String(name+"-run-id", "", "score an analysis run's track positions instead of estimates (needs "+p+"declared-baseline)"),
		declaredBaseline: fs.Bool(name+"-declared-baseline", false, "score a non-final arm as a declared baseline; recorded in the output"),
		solidBody:        fs.Bool(name+"-solid-body", false, "read the version from lidar_track_solid_bodies (replay -experiment solid_body) instead of lidar_track_estimates"),
	}
}

func (a armFlags) spec() perframeeval.ArmSpec {
	return perframeeval.ArmSpec{
		Label: *a.label, DBPath: *a.db, SourceID: *a.source, EstimatorID: *a.estimator,
		ObservationModelID: *a.model, ParamHash: *a.params, Stage: *a.stage, RunID: *a.runID,
		DeclaredBaseline: *a.declaredBaseline, SolidBodies: *a.solidBody,
	}
}

func runPerFrame(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lidar-ground-truth-eval perframe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	packDir := fs.String("pack", "", "annotation pack directory (required)")
	manifestPath := fs.String("split-manifest", "", "object-disjoint split manifest JSON: a frozen split, or a version 1 manifest (required)")
	split := fs.String("split", "", "split to score (required)")
	episodes := fs.String("episodes", "", "comma-separated episode IDs (default: every episode of the split)")
	allowTuning := fs.Bool("allow-tuning-split", false, "score a split whose role is not held_out; the output says it is not held out")
	includeProposed := fs.Bool("include-proposed", false, "also score proposed (unreviewed) masks as truth; recorded as the reference policy")
	position := fs.String("reference-position", string(annotation.PositionFootprintCentre), "reference position: footprint_centre or point_mean")
	ignorePartial := fs.Bool("ignore-partial-masks", false, "treat masks marked partial as uncertifiable (ignored) rather than scored")
	gate := fs.String("gate", string(l8analytics.GateFootprint), "association gate: footprint (gate-metres plus half the object's footprint diagonal) or fixed")
	gateMetres := fs.Float64("gate-metres", 1.0, "fixed gate, or the footprint gate's slack, in metres")
	toleranceMs := fs.Float64("frame-tolerance-ms", 10, "how far a hypothesis point may move in time onto a reference frame")
	maxUnaligned := fs.Float64("max-unaligned-fraction", 0.01, "refuse an arm when more than this share of its points inside an episode lands on no frame")
	physical := fs.Bool("physical-reference", false, "also score each estimate arm against the pack's physical references (centre, yaw, dimensions, bumpers, box, following gap); a held-out split is refused")
	physicalRevision := fs.Int("physical-reference-revision", 0, "physical reference revision to score (default: the current one); recorded either way")
	physicalGate := fs.Float64("physical-gate-metres", perframeeval.DefaultPhysicalGateMetres, "how far a prediction's point may be from a reference body centre (or anchor) and still correspond to it")
	jsonPath := fs.String("json", "", "comparison JSON path (default: stdout)")
	markdownPath := fs.String("markdown", "", "comparison Markdown path (optional)")
	armA := registerArm(fs, "a", "A")
	armB := registerArm(fs, "b", "B")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lidar-ground-truth-eval perframe -pack DIR -split-manifest FILE -split NAME -a-db DB -b-db DB [flags]\n\n")
		fmt.Fprintf(stderr, "Scores two arms per frame (MOTA, MOTP, ID switches, fragmentation, HOTA, IDF1) against the\n")
		fmt.Fprintf(stderr, "reviewed, held-out episodes of an annotation pack, and reports them paired with deltas.\n")
		fmt.Fprintf(stderr, "An arm is one estimate version (final stage by default) or one analysis run.\n\nOptions:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		return usageError(fs, stderr, fmt.Errorf("unexpected arguments: %v", fs.Args()))
	}
	for _, required := range []struct{ name, value string }{
		{"-pack", *packDir}, {"-split-manifest", *manifestPath}, {"-split", *split}, {"-a-db", *armA.db}, {"-b-db", *armB.db},
	} {
		if required.value == "" {
			return usageError(fs, stderr, fmt.Errorf("%s is required", required.name))
		}
	}

	policy := annotation.DefaultReferencePolicy()
	if *includeProposed {
		policy.Status = annotation.ReferenceIncludeProposed
	}
	policy.Position = annotation.ReferencePosition(*position)
	policy.ScorePartialMasks = !*ignorePartial
	if err := policy.Validate(); err != nil {
		return usageError(fs, stderr, err)
	}

	score := perframeeval.DefaultScoreOptions()
	switch l8analytics.GateKind(*gate) {
	case l8analytics.GateFootprint:
		score.Gate = l8analytics.FootprintGate(*gateMetres)
	case l8analytics.GateFixed:
		score.Gate = l8analytics.FixedGate(*gateMetres)
	default:
		return usageError(fs, stderr, fmt.Errorf("-gate %q: want footprint or fixed", *gate))
	}
	score.FrameToleranceNanos = int64(*toleranceMs * 1e6)
	score.MaxUnalignedFraction = *maxUnaligned
	if err := score.Validate(); err != nil {
		return usageError(fs, stderr, err)
	}

	var episodeIDs []string
	for _, id := range strings.Split(*episodes, ",") {
		if id = strings.TrimSpace(id); id != "" {
			episodeIDs = append(episodeIDs, id)
		}
	}

	cfg := perframeeval.Config{
		Reference: perframeeval.ReferenceOptions{
			PackDir: *packDir, SplitManifestPath: *manifestPath, Split: *split, Episodes: episodeIDs,
			AllowTuningSplit: *allowTuning, Policy: policy,
		},
		Score: score,
		A:     armA.spec(),
		B:     armB.spec(),
	}
	if *physical {
		opts := perframeeval.DefaultPhysicalOptions()
		opts.Revision, opts.GateMetres, opts.FrameToleranceNanos = *physicalRevision, *physicalGate, score.FrameToleranceNanos
		if err := opts.Validate(); err != nil {
			return usageError(fs, stderr, err)
		}
		cfg.Physical = &opts
	} else if *physicalRevision != 0 {
		return usageError(fs, stderr, fmt.Errorf("-physical-reference-revision needs -physical-reference"))
	} else if flagSet(fs, "physical-gate-metres") {
		return usageError(fs, stderr, fmt.Errorf("-physical-gate-metres needs -physical-reference"))
	}
	c, err := perframeeval.Run(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	payload, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "error: encode comparison: %v\n", err)
		return 1
	}
	payload = append(payload, '\n')
	if *jsonPath == "" {
		if _, err := stdout.Write(payload); err != nil {
			fmt.Fprintf(stderr, "error: write comparison: %v\n", err)
			return 1
		}
	} else if err := os.WriteFile(*jsonPath, payload, 0o644); err != nil {
		fmt.Fprintf(stderr, "error: write %s: %v\n", *jsonPath, err)
		return 1
	}
	if *markdownPath != "" {
		if err := os.WriteFile(*markdownPath, []byte(perframeeval.RenderMarkdown(*c)), 0o644); err != nil {
			fmt.Fprintf(stderr, "error: write %s: %v\n", *markdownPath, err)
			return 1
		}
	}

	t := c.Total
	fmt.Fprintf(stderr, "reference %s: split %s (held_out=%v), %d episode(s), %d reference points\n",
		c.Reference.Digest, c.Reference.Split, c.Reference.HeldOut, len(c.Paired), t.A.NumGT)
	if c.Reference.SplitDigest != "" {
		fmt.Fprintf(stderr, "frozen split %s, revision %d\n", c.Reference.SplitDigest, c.Reference.SplitRevision)
	}
	for _, arm := range []struct {
		label string
		s     perframeeval.Summary
	}{{c.A.Arm.Label, t.A}, {c.B.Arm.Label, t.B}} {
		fmt.Fprintf(stderr, "%s: MOTA %.4f  IDSW %d  FM %d  HOTA %.4f  IDF1 %.4f\n",
			arm.label, arm.s.MOTA, arm.s.IDSwitches, arm.s.Fragmentations, arm.s.HOTA, arm.s.IDF1)
	}
	if p := c.Physical; p != nil {
		fmt.Fprintf(stderr, "physical references: revision %d, content %s, %d expected instants\n",
			p.Reference.PhysicalRevision, p.Reference.PhysicalContentDigest, p.Reference.ExpectedInstants)
		for _, arm := range []perframeeval.PhysicalResult{p.A, p.B} {
			centre, yaw := arm.Summary.Components[perframeeval.ComponentCentre], arm.Summary.Components[perframeeval.ComponentYaw]
			fmt.Fprintf(stderr, "%s physical: centre %d scored, mean %.3f m  yaw %d scored, mean %.3f rad  gap %d scored, mean %.3f m\n",
				arm.Arm.Label, centre.Scored, centre.MeanAbsError, yaw.Scored, yaw.MeanAbsError,
				arm.Summary.Following.Scored, arm.Summary.Following.MeanAbsError)
		}
	}
	for _, cv := range c.Caveats {
		fmt.Fprintf(stderr, "caveat: %s\n", cv)
	}
	return 0
}

func usageError(fs *flag.FlagSet, stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "error: %v\n", err)
	fs.Usage()
	return 2
}

// flagSet reports whether the command line set a flag, whatever its value.
func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}
