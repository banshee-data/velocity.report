package main

import (
	"flag"
	"fmt"
	"os"

	cfgpkg "github.com/banshee-data/velocity.report/internal/config"
)

func main() {
	path := flag.String("in", "", "Nested tuning config JSON path to validate")
	flag.Parse()

	if *path == "" {
		fmt.Fprintln(os.Stderr, "error: --in is required")
		os.Exit(2)
	}

	cfg, err := cfgpkg.LoadTuningConfig(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("valid config: %s (version=%d l3=%s l4=%s l5=%s)\n",
		*path, cfg.Version, cfg.L3.Engine, cfg.L4.Engine, cfg.L5.Engine)
}
