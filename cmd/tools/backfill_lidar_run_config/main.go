package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	_ "modernc.org/sqlite"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	sqlitepkg "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func run(args []string, stat func(string) (os.FileInfo, error), openDB func(string) (*dbpkg.DB, error), backfill func(sqlitepkg.DBClient, bool) (*sqlitepkg.ImmutableRunConfigBackfillResult, error), logger *log.Logger) error {
	if logger == nil {
		logger = log.Default()
	}

	flagSet := flag.NewFlagSet("backfill-lidar-run-config", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	dbPath := flagSet.String("db", "sensor_data.db", "path to sqlite DB file")
	dryRun := flagSet.Bool("dry-run", false, "inspect rows and report changes without writing")
	if err := flagSet.Parse(args); err != nil {
		return err
	}

	if _, err := stat(*dbPath); err != nil {
		return fmt.Errorf("DB path %s not accessible: %w", *dbPath, err)
	}

	opened, err := openDB(*dbPath)
	if err != nil {
		return fmt.Errorf("open DB: %w", err)
	}
	defer opened.Close()

	result, err := backfill(opened.DB, *dryRun)
	if err != nil {
		return fmt.Errorf("immutable run-config backfill failed: %w", err)
	}

	logger.Printf(
		"done: runs seen=%d updated=%d skipped=%d; replay cases seen=%d updated=%d skipped=%d",
		result.RunsSeen,
		result.RunsUpdated,
		result.RunsSkipped,
		result.ReplayCasesSeen,
		result.ReplayCasesUpdated,
		result.ReplayCasesSkipped,
	)
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stat, dbpkg.OpenDB, sqlitepkg.BackfillImmutableRunConfigReferences, log.Default()); err != nil {
		log.Fatal(err)
	}
}
