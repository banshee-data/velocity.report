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

// The fate subcommand: experiment geom0004's measure. For every return of a
// reviewed road-user mask in a split's episodes, frame by frame, which stage
// of a replay lost it: L3 (not in the frame's foreground), each L4 rejection
// the observation container records (height band, voxel, input cap, DBSCAN
// noise, a rejected cluster's shape), or none (clustered, with the cluster
// identity, so fragments are countable). Summarised overall, by class, by
// range band and per object, with what the gaps between an object's
// fragments are made of.
//
//	lidar-ground-truth-eval fate -pack DIR -split-manifest FILE -split NAME -observations DIR [-pcap FILE] [-json out.json]
//
// The evidence is a foreground-complete observation container, written by
//
//	velocity lidar pcap-replay --pcap FILE --output DIR --config TUNING \
//	    --observations OBS --replay-case-id ID
//
// A container lacking that profile's capabilities is refused. Returns are
// joined by packet identity: the pack's (packet key, block, channel) is
// mapped to the container's (packet sequence, block, channel) through the
// capture file both were made from, which -pcap names (default: the path in
// the container's manifest) and whose SHA-256 must be the manifest's. The
// full statement, with the measure's limits, is in
// internal/lidar/perframeeval/fate.go.

func runFate(args []string, stdout, stderr io.Writer) int {
	opts, jsonPath, code := parseFate(args, stderr)
	if opts == nil {
		return code
	}
	report, err := perframeeval.MeasureFate(*opts)
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
	if _, err := io.WriteString(stdout, perframeeval.RenderFateMarkdown(*report)); err != nil {
		fmt.Fprintf(stderr, "error: write summary: %v\n", err)
		return 1
	}
	return 0
}

// parseFate parses a fate command line. Nil options carry the exit code:
// zero after -h, two after a usage error, which it has reported.
func parseFate(args []string, stderr io.Writer) (*perframeeval.FateOptions, string, int) {
	fs := flag.NewFlagSet("lidar-ground-truth-eval fate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	defaults := perframeeval.DefaultFateOptions()
	packDir := fs.String("pack", "", "annotation pack directory, version 2 (required)")
	manifestPath := fs.String("split-manifest", "", "object-disjoint split manifest JSON: a frozen split, or a version 1 manifest (required)")
	split := fs.String("split", "", "split to measure (required)")
	episodes := fs.String("episodes", "", "comma-separated episode IDs (default: every episode of the split)")
	allowTuning := fs.Bool("allow-tuning-split", false, "measure a split whose role is not held_out; the output says it is not held out")
	includeProposed := fs.Bool("include-proposed", false, "also measure proposed (unreviewed) masks; recorded as the reference policy")
	ignorePartial := fs.Bool("ignore-partial-masks", false, "treat masks marked partial as uncertifiable rather than measured")
	position := fs.String("reference-position", string(annotation.PositionFootprintCentre), "where an object's range is taken from: footprint_centre or point_mean")
	observations := fs.String("observations", "", "foreground-complete observation container (pcap-replay --observations), opened read-only (required)")
	pcap := fs.String("pcap", "", "the capture the container was extracted from, read to map packet keys to sequences (default: the path in its manifest)")
	port := fs.Int("port", defaults.Port, "UDP port of the sensor's packets in the capture")
	toleranceMs := fs.Float64("frame-tolerance-ms", float64(defaults.FrameToleranceNanos)/1e6, "how far a container frame's time may be from a pack sample's")
	bands := fs.String("range-bands", joinFloats(defaults.RangeBandsM), "ascending upper edges of the range bands in metres; the last band is open")
	sensorX := fs.Float64("sensor-x", 0, "sensor x in the pack's frame, metres")
	sensorY := fs.Float64("sensor-y", 0, "sensor y in the pack's frame, metres")
	jsonPath := fs.String("json", "", "report JSON path, with every instant (optional)")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lidar-ground-truth-eval fate -pack DIR -split-manifest FILE -split NAME -observations DIR [flags]\n\n")
		fmt.Fprintf(stderr, "For every return of a reviewed road-user mask, which pipeline stage lost it: L3 background, each L4\n")
		fmt.Fprintf(stderr, "rejection the container records, or none (clustered, with its cluster), by class, range band and object,\n")
		fmt.Fprintf(stderr, "and what the gaps between an object's fragments are made of. Prints a Markdown summary; -json writes\n")
		fmt.Fprintf(stderr, "the full report. Replay with velocity lidar pcap-replay --observations DIR --replay-case-id ID.\n\nOptions:\n")
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
		{"-pack", *packDir}, {"-split-manifest", *manifestPath}, {"-split", *split}, {"-observations", *observations},
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
	opts := perframeeval.FateOptions{
		Reference: perframeeval.ReferenceOptions{
			PackDir: *packDir, SplitManifestPath: *manifestPath, Split: *split, Episodes: episodeIDs,
			AllowTuningSplit: *allowTuning, Policy: policy,
		},
		ObservationsDir: *observations, CapturePath: *pcap, Port: *port, FrameToleranceNanos: int64(*toleranceMs * 1e6),
		SensorXM: *sensorX, SensorYM: *sensorY, RangeBandsM: edges,
	}
	if err := opts.Validate(); err != nil {
		return nil, "", usageError(fs, stderr, err)
	}
	return &opts, *jsonPath, 0
}
