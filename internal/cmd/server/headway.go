package server

import (
	"flag"
	"fmt"
	"io"

	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/report/headway"
)

// runHeadway implements "velocity report headway": the headway report of
// observed following exposure (behaviour plan Section 10.4). Only the
// synthetic oracle exists yet, rendered from the analytic encounter
// scenarios; the provisional report from persisted encounters is the next
// slice, and the flag says so rather than rendering anything else.
//
//	velocity report headway --oracle [--output ./reports] [--paper letter|a4]
func runHeadway(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("velocity report headway", flag.ContinueOnError)
	fs.SetOutput(stderr)
	oracle := fs.Bool("oracle", false,
		"Render the synthetic oracle from the analytic l8behaviour encounter scenarios (required)")
	outputDir := fs.String("output", "", "Output directory (default: the current directory)")
	paper := fs.String("paper", string(chart.PaperLetter), "Paper size: letter or a4")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "error: unexpected argument %q\n", fs.Arg(0))
		fs.Usage()
		return 2
	}
	if !*oracle {
		fmt.Fprintln(stderr, "error: --oracle is required: the provisional headway report from "+
			"persisted encounters is not built yet (sprint 0.5.2.4)")
		fs.Usage()
		return 2
	}
	size := chart.PaperSize(*paper)
	if size != chart.PaperLetter && size != chart.PaperA4 {
		fmt.Fprintf(stderr, "error: --paper must be %s or %s, got %q\n", chart.PaperLetter, chart.PaperA4, *paper)
		return 2
	}
	dir, err := normalizePDFOutputDir(*outputDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to resolve output directory: %v\n", err)
		return 1
	}

	r, err := headway.Oracle()
	if err != nil {
		fmt.Fprintf(stderr, "error: build the oracle report: %v\n", err)
		return 1
	}
	res, err := headway.Generate(r, headway.Options{Paper: size, OutputDir: dir})
	if err != nil {
		fmt.Fprintf(stderr, "error: report generation failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Status: %s\n", r.StatusLabel)
	fmt.Fprintf(stdout, "PDF: %s\n", res.PDFPath)
	fmt.Fprintf(stdout, "ZIP: %s\n", res.ZIPPath)
	return 0
}
