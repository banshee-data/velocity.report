package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

func runUncertaintyRecovery(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uncertainty-recovery", flag.ContinueOnError)
	fs.SetOutput(stderr)
	path := fs.String("calibration", "", "frozen uncertainty report containing the content-addressed calibration table")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *path == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "uncertainty-recovery requires -calibration FILE; no positional arguments")
		return 2
	}
	data, err := os.ReadFile(*path)
	if err != nil {
		fmt.Fprintf(stderr, "error: read calibration: %v\n", err)
		return 1
	}
	cal, err := l8analytics.ParseNoiseCalibration(data)
	if err != nil {
		fmt.Fprintf(stderr, "error: parse calibration: %v\n", err)
		return 1
	}
	report, err := l8analytics.CheckFrozenRecovery(cal)
	if err != nil {
		fmt.Fprintf(stderr, "error: recovery: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		fmt.Fprintf(stderr, "error: write recovery report: %v\n", err)
		return 1
	}
	return 0
}
