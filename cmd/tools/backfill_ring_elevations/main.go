package main

import (
	"flag"
	"log"
	"os"

	_ "modernc.org/sqlite"

	parsepkg "github.com/banshee-data/velocity.report/internal/lidar/parse"
)

func main() {
	dbPath := flag.String("db", "sensor_data.db", "path to sqlite DB file")
	dry := flag.Bool("dry-run", false, "don't write changes; just report")
	flag.Parse()

	if _, err := os.Stat(*dbPath); err != nil {
		log.Fatalf("DB path %s not accessible: %v", *dbPath, err)
	}

	cfg, err := parsepkg.LoadEmbeddedPandar40PConfig()
	if err != nil {
		log.Printf("warning: failed to load embedded parser config: %v", err)
	}
	var embeddedElevs []float64
	if cfg != nil {
		embeddedElevs = parsepkg.ElevationsFromConfig(cfg)
	}

	total, updated, skipped, err := RunBackfill(dbPath, embeddedElevs, *dry)
	if err != nil {
		log.Fatalf("backfill failed: %v", err)
	}
	log.Printf("done: total=%d updated=%d skipped=%d", total, updated, skipped)
}
