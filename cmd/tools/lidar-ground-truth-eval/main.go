// Command lidar-ground-truth-eval scores one candidate analysis run against
// one labelled reference analysis run using adapters.EvaluateGroundTruth.
//
// GroundTruthEvaluator (internal/lidar/adapters/ground_truth.go) is fully
// implemented and tested, but the only thing that calls it today is the
// live, human-in-the-loop HINT sweep (POST /api/lidar/sweep/hint). This tool
// exposes the same scoring standalone, against runs that already exist in an
// AnalysisRunStore, so an L5 parameter sweep can be judged by real detection
// rate / false-positive rate / quality metrics against labelled tracks
// instead of a metric the tracker can win by smoothing its own output (see
// data/experiments/try/l5-tracking-noise-parameter-sweep.md's "preliminary
// pass" section for why that distinction matters here).
//
// The reference run and the candidate run don't have to live in the same
// database: a candidate produced by a throwaway, isolated replay server
// (its own fresh --db-path, so it never touches whatever database holds the
// real labelled reference run) is a normal use case, so -reference-db and
// -candidate-db are independent and both fall back to -db. Both are opened
// read-only (sqlite.OpenReadOnly): this tool only ever reads, never writes,
// so it's safe to run concurrently against a database another process has
// open.
//
// That default mode matches whole tracks on temporal overlap, which is gap
// M5: it cannot see state estimates and scores a fragmented track as an
// undetected one. The perframe subcommand (perframe.go) is the per-frame
// replacement: it scores two arms against the reviewed, held-out episodes of
// an annotation pack and reports MOTA, MOTP, identity switches, fragmentation,
// HOTA and IDF1 paired per episode.
//
//	lidar-ground-truth-eval perframe -pack DIR -split-manifest FILE -split NAME -a-db DB -b-db DB ...
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/adapters"
	_ "modernc.org/sqlite"

	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	stdout, stderr := os.Stdout, os.Stderr
	if len(args) > 0 && args[0] == "perframe" {
		return runPerFrame(args[1:], stdout, stderr)
	}
	fs := flag.NewFlagSet("lidar-ground-truth-eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", "sensor_data.db", "default AnalysisRunStore SQLite database, used when -reference-db/-candidate-db are omitted")
	referenceDBPath := fs.String("reference-db", "", "database holding the reference run (default: -db)")
	candidateDBPath := fs.String("candidate-db", "", "database holding the candidate run (default: -db) -- may be a different file, e.g. a throwaway isolated replay server's own database")
	referenceRunID := fs.String("reference-run-id", "", "run ID with human-labelled reference tracks (required)")
	candidateRunID := fs.String("candidate-run-id", "", "run ID to score against the reference (required)")
	weightsPath := fs.String("weights", "", "optional JSON file of adapters.GroundTruthWeights; default: adapters.DefaultGroundTruthWeights()")
	outPath := fs.String("output", "", "output JSON path (default: stdout)")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lidar-ground-truth-eval -reference-run-id ID -candidate-run-id ID [flags]\n")
		fmt.Fprintf(stderr, "       lidar-ground-truth-eval perframe [flags]   (per-frame scoring against annotations; perframe -h)\n\n")
		fmt.Fprintf(stderr, "Scores one candidate analysis run against one labelled reference run\n")
		fmt.Fprintf(stderr, "using adapters.EvaluateGroundTruth, and prints the GroundTruthScore as JSON.\n\nOptions:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	resolvedRefDB := resolveDBPath(*referenceDBPath, *dbPath)
	resolvedCandDB := resolveDBPath(*candidateDBPath, *dbPath)
	if err := validateArgs(*referenceRunID, *candidateRunID, resolvedRefDB, resolvedCandDB); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		fs.Usage()
		return 2
	}

	weights := adapters.DefaultGroundTruthWeights()
	if *weightsPath != "" {
		data, err := os.ReadFile(*weightsPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: read weights file: %v\n", err)
			return 1
		}
		var loaded adapters.GroundTruthWeights
		if err := json.Unmarshal(data, &loaded); err != nil {
			fmt.Fprintf(stderr, "error: parse weights file: %v\n", err)
			return 1
		}
		weights = loaded
	}

	refTracks, err := loadRunTracks(resolvedRefDB, *referenceRunID)
	if err != nil {
		fmt.Fprintf(stderr, "error: load reference run: %v\n", err)
		return 1
	}
	candTracks, err := loadRunTracks(resolvedCandDB, *candidateRunID)
	if err != nil {
		fmt.Fprintf(stderr, "error: load candidate run: %v\n", err)
		return 1
	}

	score := adapters.EvaluateGroundTruth(refTracks, candTracks, weights)
	if score.ReferenceCount == 0 {
		fmt.Fprintf(stderr, "warning: reference run %s has zero positively-labelled tracks; score is meaningless\n", *referenceRunID)
	}

	out, err := json.MarshalIndent(score, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "error: marshal score: %v\n", err)
		return 1
	}
	if *outPath == "" {
		fmt.Fprintln(stdout, string(out))
		return 0
	}
	if err := os.WriteFile(*outPath, append(out, '\n'), 0o644); err != nil {
		fmt.Fprintf(stderr, "error: write output: %v\n", err)
		return 1
	}
	return 0
}

// resolveDBPath returns specific if set, otherwise the shared fallback.
// Separated out so it's directly unit-testable.
func resolveDBPath(specific, fallback string) string {
	if specific != "" {
		return specific
	}
	return fallback
}

// validateArgs is separated from run so it's directly unit-testable without
// touching a database or flag.FlagSet.
func validateArgs(referenceRunID, candidateRunID, referenceDB, candidateDB string) error {
	if referenceRunID == "" {
		return fmt.Errorf("-reference-run-id is required")
	}
	if candidateRunID == "" {
		return fmt.Errorf("-candidate-run-id is required")
	}
	if referenceRunID == candidateRunID && referenceDB == candidateDB {
		return fmt.Errorf("-reference-run-id and -candidate-run-id must differ when reading the same database (got %q for both)", referenceRunID)
	}
	return nil
}

func loadRunTracks(dbPath, runID string) ([]*sqlite.RunTrack, error) {
	db, err := sqlite.OpenReadOnly(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", dbPath, err)
	}
	defer db.Close()
	store := sqlite.NewAnalysisRunStore(db)
	tracks, err := store.GetRunTracks(runID)
	if err != nil {
		return nil, fmt.Errorf("query run %s in %s: %w", runID, dbPath, err)
	}
	return tracks, nil
}
