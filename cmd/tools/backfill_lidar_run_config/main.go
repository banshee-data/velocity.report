package main

import (
	"flag"
	"log"
	"os"

	_ "modernc.org/sqlite"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	sqlitepkg "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func main() {
	dbPath := flag.String("db", "sensor_data.db", "path to sqlite DB file")
	dryRun := flag.Bool("dry-run", false, "inspect rows and report changes without writing")
	flag.Parse()

	if _, err := os.Stat(*dbPath); err != nil {
		log.Fatalf("DB path %s not accessible: %v", *dbPath, err)
	}

	opened, err := dbpkg.OpenDB(*dbPath)
	if err != nil {
		log.Fatalf("open DB: %v", err)
	}
	defer opened.Close()

	result, err := sqlitepkg.BackfillImmutableRunConfigReferences(opened.DB, *dryRun)
	if err != nil {
		log.Fatalf("immutable run-config backfill failed: %v", err)
	}

	log.Printf(
		"done: runs seen=%d updated=%d skipped=%d; replay cases seen=%d updated=%d skipped=%d",
		result.RunsSeen,
		result.RunsUpdated,
		result.RunsSkipped,
		result.ReplayCasesSeen,
		result.ReplayCasesUpdated,
		result.ReplayCasesSkipped,
	)
}
