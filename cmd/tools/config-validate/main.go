package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("config-validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	path := fs.String("in", "", "Nested tuning config JSON path to validate")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *path == "" {
		fmt.Fprintln(stderr, "error: --in is required")
		return 2
	}

	cfg, err := cfgpkg.LoadTuningConfig(*path)
	if err != nil {
		fmt.Fprintf(stderr, "invalid config: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "valid config: %s (version=%d l3=%s l4=%s l5=%s)\n",
		*path, cfg.Version, cfg.L3.Engine, cfg.L4.Engine, cfg.L5.Engine)
	return 0
}
