package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("config-validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	path := fs.String("in", "", "Nested tuning config JSON path to validate")
	selectors := fs.String("selectors", "", "Segment selector file to validate")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *path == "" && *selectors == "" {
		fmt.Fprintln(stderr, "error: --in or --selectors is required")
		return 2
	}

	if *path != "" {
		cfg, err := cfgpkg.LoadTuningConfig(*path)
		if err != nil {
			fmt.Fprintf(stderr, "invalid config: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "valid config: %s (version=%d l3=%s l4=%s l5=%s)\n",
			*path, cfg.Version, cfg.L3.Engine, cfg.L4.Engine, cfg.L5.Engine)
	}

	if *selectors != "" {
		catalogue, err := segments.LoadSelectors(*selectors)
		if err != nil {
			fmt.Fprintf(stderr, "invalid selectors: %v\n", err)
			return 1
		}
		heldOut := len(catalogue.ForRole("held_out"))
		fmt.Fprintf(stdout, "valid selectors: %s (version=%d selectors=%d held_out=%d digest=%s)\n",
			*selectors, catalogue.Version, len(catalogue.Selectors), heldOut, catalogue.Digest)
	}
	return 0
}
