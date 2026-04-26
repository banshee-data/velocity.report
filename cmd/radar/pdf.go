package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/report"
	"github.com/banshee-data/velocity.report/internal/version"
)

// runPDF implements the "pdf" subcommand: generate a PDF report from the CLI
// without starting the HTTP server.
//
//	velocity-report pdf --config report.json [--output ./out] [--db path/to/db.sqlite]
func runPDF(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("velocity-report pdf", flag.ContinueOnError)
	fs.SetOutput(stderr)

	configPath := fs.String("config", "", "Path to report config JSON file (required)")
	dbPath := fs.String("db", "", "Path to SQLite database file (required)")
	outputDir := fs.String("output", "", "Output directory (overrides config)")
	showVersion := fs.Bool("version", false, "Print version and exit")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		fmt.Fprintf(stdout, "velocity-report pdf %s (%s, %s)\n",
			version.Version, version.GitSHA, version.BuildTime)
		return 0
	}

	if *configPath == "" {
		fmt.Fprintln(stderr, "error: --config is required")
		fs.Usage()
		return 2
	}
	if *dbPath == "" {
		fmt.Fprintln(stderr, "error: --db is required")
		fs.Usage()
		return 2
	}

	// Read and parse config.
	configData, err := os.ReadFile(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to read config file: %v\n", err)
		return 1
	}
	var cfg report.Config
	if err := json.Unmarshal(configData, &cfg); err != nil {
		fmt.Fprintf(stderr, "error: failed to parse config JSON: %v\n", err)
		return 1
	}

	// Override output directory if specified.
	if *outputDir != "" {
		cfg.OutputDir = *outputDir
	}
	cfg.OutputDir, err = normalizePDFOutputDir(cfg.OutputDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to resolve output directory: %v\n", err)
		return 1
	}

	// Open database.
	database, err := db.OpenDB(*dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to open database: %v\n", err)
		return 1
	}
	defer database.Close()

	// Generate report.
	ctx := context.Background()
	result, err := report.Generate(ctx, database, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "error: report generation failed: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "PDF: %s\n", result.PDFPath)
	fmt.Fprintf(stdout, "ZIP: %s\n", result.ZIPPath)
	return 0
}

func normalizePDFOutputDir(outputDir string) (string, error) {
	if outputDir == "" {
		outputDir = "."
	}
	return filepath.Abs(outputDir)
}
