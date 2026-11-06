package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
)

func runManoeuvres(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("manoeuvres", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opts perframeeval.ManoeuvreOptions
	opts.Reference.Policy = annotation.DefaultReferencePolicy()
	fs.StringVar(&opts.Reference.PackDir, "pack", "", "annotation pack directory")
	fs.StringVar(&opts.Reference.SplitManifestPath, "split-manifest", "", "split manifest (prefer frozen)")
	fs.StringVar(&opts.Reference.Split, "split", "", "partition to score")
	fs.BoolVar(&opts.Reference.AllowTuningSplit, "allow-tuning-split", false, "explicit development score, never qualification")
	fs.StringVar(&opts.EventsPath, "events", "", "reviewed manoeuvre manifest pinned to the reference digest")
	fs.StringVar(&opts.SamplesPath, "samples", "", "uncertainty_samples.jsonl from the candidate replay")
	ignore := fs.String("ignore-objects", "", "comma-separated object IDs excluded by the reference policy")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *ignore != "" {
		opts.Reference.Policy.IgnoreObjects = strings.Split(*ignore, ",")
	}
	if fs.NArg() != 0 || opts.Reference.PackDir == "" || opts.Reference.SplitManifestPath == "" || opts.Reference.Split == "" || opts.EventsPath == "" || opts.SamplesPath == "" {
		fmt.Fprintln(stderr, "manoeuvres requires -pack, -split-manifest, -split, -events and -samples; no positional arguments")
		return 2
	}
	report, err := perframeeval.MeasureManoeuvres(opts)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(stderr, "error: write manoeuvre report: %v\n", err)
		return 1
	}
	return 0
}
