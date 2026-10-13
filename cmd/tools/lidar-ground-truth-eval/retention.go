package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
)

// The retention subcommand: the geometry plan's W3 measure. For each reviewed
// mask of a split's episodes, frame by frame, the share of the object's
// labelled returns that reached an L4 cluster in a replay, how many clusters
// they fell into, and which clusters held more than one reviewed object.
// Summarised overall, by class, by range band from the sensor, and per object.
//
//	lidar-ground-truth-eval retention -pack DIR -split-manifest FILE -split NAME -db observations.db [-json out.json]
//
// The evidence is the observations.db that lidar-state-estimation-baseline
// writes with -evidence-dir. Replay with -sample-points 1024: the measure
// reads each cluster's retained sample.
//
// Matching, and its limits. The evidence keeps no point index, only each
// cluster's population and a retained sample of its members' coordinates.
// Pack points and the sample are the float32 of the same float64 L4 computed,
// so a sample point is matched to a pack point by exact float32 identity
// within the frame whose time equals the pack sample's: no distance
// tolerance. A cluster whose sample holds every member is matched exactly. A
// cluster larger than the -sample-points cap keeps a simple random sample,
// so the mask's share of it is a Horvitz-Thompson expansion of the returns
// found in the sample (unbiased, not exact), the found returns are reported
// as a confirmed lower bound, and a small fragment of a capped cluster can go
// unseen, which makes fragment counts a lower bound there. The output header
// says Approximate whenever that happened. A return that reached no cluster
// is not attributed to the stage that lost it, and the pack is
// foreground-only, so retention is against the recording run's foreground.
// The full statement is in internal/lidar/perframeeval/retention.go.
//
// There is no estimate stage to choose: clusters are L4 output, before any
// estimator runs. -source picks the evidence source when the database holds
// more than one.

func runRetention(args []string, stdout, stderr io.Writer) int {
	opts, jsonPath, code := parseRetention(args, stderr)
	if opts == nil {
		return code
	}
	report, err := perframeeval.MeasureRetention(*opts)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if jsonPath != "" {
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "error: encode report: %v\n", err)
			return 1
		}
		if err := os.WriteFile(jsonPath, append(payload, '\n'), 0o644); err != nil {
			fmt.Fprintf(stderr, "error: write %s: %v\n", jsonPath, err)
			return 1
		}
	}
	if _, err := io.WriteString(stdout, perframeeval.RenderRetentionMarkdown(*report)); err != nil {
		fmt.Fprintf(stderr, "error: write summary: %v\n", err)
		return 1
	}
	return 0
}

// parseRetention parses a retention command line. Nil options carry the exit
// code: zero after -h, two after a usage error, which it has reported.
func parseRetention(args []string, stderr io.Writer) (*perframeeval.RetentionOptions, string, int) {
	fs := flag.NewFlagSet("lidar-ground-truth-eval retention", flag.ContinueOnError)
	fs.SetOutput(stderr)
	defaults := perframeeval.DefaultRetentionOptions()
	packDir := fs.String("pack", "", "annotation pack directory (required)")
	manifestPath := fs.String("split-manifest", "", "object-disjoint split manifest JSON: a frozen split, or a version 1 manifest (required)")
	split := fs.String("split", "", "split to measure (required)")
	episodes := fs.String("episodes", "", "comma-separated episode IDs (default: every episode of the split)")
	allowTuning := fs.Bool("allow-tuning-split", false, "measure a split whose role is not held_out; the output says it is not held out")
	includeProposed := fs.Bool("include-proposed", false, "also measure proposed (unreviewed) masks; recorded as the reference policy")
	ignorePartial := fs.Bool("ignore-partial-masks", false, "treat masks marked partial as uncertifiable rather than measured")
	position := fs.String("reference-position", string(annotation.PositionFootprintCentre), "where an object's range is taken from: footprint_centre or point_mean")
	dbPath := fs.String("db", "", "the replay's evidence database (observations.db from -evidence-dir), opened read-only (required)")
	source := fs.String("source", "", "evidence source_id (default: the only one in the database)")
	toleranceMs := fs.Float64("frame-tolerance-ms", float64(defaults.FrameToleranceNanos)/1e6, "how far an evidence frame's time may be from a pack sample's")
	bands := fs.String("range-bands", joinFloats(defaults.RangeBandsM), "ascending upper edges of the range bands in metres; the last band is open")
	sensorX := fs.Float64("sensor-x", 0, "sensor x in the pack's frame, metres")
	sensorY := fs.Float64("sensor-y", 0, "sensor y in the pack's frame, metres")
	minDomain := fs.Float64("min-domain-share", defaults.MinDomainShare, "refuse evidence when fewer of its sample points than this share are pack points")
	jsonPath := fs.String("json", "", "report JSON path, with every instant and merge (optional)")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lidar-ground-truth-eval retention -pack DIR -split-manifest FILE -split NAME -db observations.db [flags]\n\n")
		fmt.Fprintf(stderr, "Measures, for each reviewed mask, the share of its returns that reached an L4 cluster in a replay,\n")
		fmt.Fprintf(stderr, "the number of clusters they fell into, and clusters holding more than one reviewed object, by class,\n")
		fmt.Fprintf(stderr, "range band and object. Prints a Markdown summary; -json writes the full report. Replay with\n")
		fmt.Fprintf(stderr, "lidar-state-estimation-baseline -evidence-dir DIR -sample-points 1024.\n\nOptions:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil, "", 0
		}
		return nil, "", 2
	}
	if fs.NArg() > 0 {
		return nil, "", usageError(fs, stderr, fmt.Errorf("unexpected arguments: %v", fs.Args()))
	}
	for _, required := range []struct{ name, value string }{
		{"-pack", *packDir}, {"-split-manifest", *manifestPath}, {"-split", *split}, {"-db", *dbPath},
	} {
		if required.value == "" {
			return nil, "", usageError(fs, stderr, fmt.Errorf("%s is required", required.name))
		}
	}
	edges, err := parseFloats(*bands)
	if err != nil {
		return nil, "", usageError(fs, stderr, fmt.Errorf("-range-bands: %w", err))
	}

	policy := annotation.DefaultReferencePolicy()
	if *includeProposed {
		policy.Status = annotation.ReferenceIncludeProposed
	}
	policy.Position = annotation.ReferencePosition(*position)
	policy.ScorePartialMasks = !*ignorePartial

	var episodeIDs []string
	for _, id := range strings.Split(*episodes, ",") {
		if id = strings.TrimSpace(id); id != "" {
			episodeIDs = append(episodeIDs, id)
		}
	}
	opts := perframeeval.RetentionOptions{
		Reference: perframeeval.ReferenceOptions{
			PackDir: *packDir, SplitManifestPath: *manifestPath, Split: *split, Episodes: episodeIDs,
			AllowTuningSplit: *allowTuning, Policy: policy,
		},
		DBPath: *dbPath, SourceID: *source, FrameToleranceNanos: int64(*toleranceMs * 1e6),
		SensorXM: *sensorX, SensorYM: *sensorY, RangeBandsM: edges, MinDomainShare: *minDomain,
	}
	if err := opts.Validate(); err != nil {
		return nil, "", usageError(fs, stderr, err)
	}
	return &opts, *jsonPath, 0
}

func parseFloats(s string) ([]float64, error) {
	var out []float64
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		v, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", part)
		}
		out = append(out, v)
	}
	return out, nil
}

func joinFloats(v []float64) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = strconv.FormatFloat(x, 'g', -1, 64)
	}
	return strings.Join(parts, ",")
}
