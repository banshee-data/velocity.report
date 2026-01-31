// Package db provides database operations for velocity.report.
package db

import (
	"context"
	"fmt"
	"io"
)

// TransitCLI provides CLI operations for transit data management.
// It wraps TransitWorker and DB methods to provide a testable interface
// for the `velocity-report transits` subcommand.
type TransitCLI struct {
	DB           *DB
	ModelVersion string
	Threshold    int
	Output       io.Writer // where to write output (os.Stdout by default)
}

// NewTransitCLI creates a new TransitCLI instance.
func NewTransitCLI(db *DB, modelVersion string, threshold int, output io.Writer) *TransitCLI {
	return &TransitCLI{
		DB:           db,
		ModelVersion: modelVersion,
		Threshold:    threshold,
		Output:       output,
	}
}

// Analyse shows transit statistics and checks for overlaps.
// Returns the statistics for programmatic use.
func (c *TransitCLI) Analyse(ctx context.Context) (*TransitOverlapStats, error) {
	stats, err := c.DB.AnalyseTransitOverlaps(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to analyse transits: %w", err)
	}

	fmt.Fprintf(c.Output, "Transit Statistics\n")
	fmt.Fprintf(c.Output, "==================\n")
	fmt.Fprintf(c.Output, "Total transits: %d\n\n", stats.TotalTransits)

	fmt.Fprintf(c.Output, "By model version:\n")
	for mv, count := range stats.ModelVersionCounts {
		fmt.Fprintf(c.Output, "  %-20s %d\n", mv, count)
	}

	if len(stats.Overlaps) > 0 {
		fmt.Fprintf(c.Output, "\n⚠️  Overlapping transits detected:\n")
		for _, o := range stats.Overlaps {
			fmt.Fprintf(c.Output, "  %s ↔ %s: %d overlaps\n", o.ModelVersion1, o.ModelVersion2, o.OverlapCount)
		}
		fmt.Fprintf(c.Output, "\nTo fix overlaps, delete one model version:\n")
		fmt.Fprintf(c.Output, "  velocity-report transits delete <model-version>\n")
	} else {
		fmt.Fprintf(c.Output, "\n✅ No overlapping transits found\n")
	}

	return stats, nil
}

// Delete removes all transits for a given model version.
// Returns the number of deleted transits.
func (c *TransitCLI) Delete(ctx context.Context, modelVersion string) (int64, error) {
	worker := NewTransitWorker(c.DB, c.Threshold, c.ModelVersion)
	deleted, err := worker.DeleteAllTransits(ctx, modelVersion)
	if err != nil {
		return 0, fmt.Errorf("failed to delete transits: %w", err)
	}

	fmt.Fprintf(c.Output, "Deleted %d transits with model_version = %q\n", deleted, modelVersion)
	return deleted, nil
}

// Migrate deletes transits with fromVersion and rebuilds with toVersion.
func (c *TransitCLI) Migrate(ctx context.Context, fromVersion, toVersion string) error {
	fmt.Fprintf(c.Output, "Migrating transits from %q to %q\n", fromVersion, toVersion)

	worker := NewTransitWorker(c.DB, c.Threshold, toVersion)
	if err := worker.MigrateModelVersion(ctx, fromVersion); err != nil {
		return fmt.Errorf("failed to migrate transits: %w", err)
	}

	fmt.Fprintf(c.Output, "Migration complete\n")
	return nil
}

// Rebuild deletes all transits for the current model version and rebuilds from full history.
func (c *TransitCLI) Rebuild(ctx context.Context) error {
	fmt.Fprintf(c.Output, "Rebuilding transits with model_version = %q\n", c.ModelVersion)

	worker := NewTransitWorker(c.DB, c.Threshold, c.ModelVersion)

	// Delete existing transits for this model version
	deleted, err := worker.DeleteAllTransits(ctx, c.ModelVersion)
	if err != nil {
		return fmt.Errorf("failed to delete existing transits: %w", err)
	}
	fmt.Fprintf(c.Output, "Deleted %d existing transits\n", deleted)

	// Run full history rebuild
	if err := worker.RunFullHistory(ctx); err != nil {
		return fmt.Errorf("failed to rebuild transits: %w", err)
	}

	fmt.Fprintf(c.Output, "Rebuild complete\n")
	return nil
}

// PrintUsage prints the transits subcommand usage.
func (c *TransitCLI) PrintUsage() {
	fmt.Fprintln(c.Output, "Usage: velocity-report transits <command> [options]")
	fmt.Fprintln(c.Output, "")
	fmt.Fprintln(c.Output, "Commands:")
	fmt.Fprintln(c.Output, "  analyse                      Show transit statistics and check for overlaps")
	fmt.Fprintln(c.Output, "  delete <model-version>       Delete all transits for a model version")
	fmt.Fprintln(c.Output, "  migrate <from> <to>          Migrate transits from one model version to another")
	fmt.Fprintln(c.Output, "  rebuild                      Delete current model version and rebuild from full history")
	fmt.Fprintln(c.Output, "")
}
