package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
)

func main() {
	var dbPath string
	var threshold int
	var modelVer string
	var dryRun bool

	flag.StringVar(&dbPath, "db", "sensor_data.db", "path to sqlite db")
	flag.IntVar(&threshold, "gap", 1, "session gap in seconds")
	flag.StringVar(&modelVer, "model", "scan-backfill", "model version string for transits")
	flag.BoolVar(&dryRun, "dry-run", false, "only report gaps, don't backfill")
	flag.Parse()

	dbConn, err := db.NewDB(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer dbConn.Close()

	fmt.Println("Scanning for hourly periods with radar_data but missing transit records...")

	// Find hourly periods with radar_data records but no corresponding transits
	gaps, err := findTransitGaps(dbConn)
	if err != nil {
		log.Fatalf("failed to find gaps: %v", err)
	}

	if len(gaps) == 0 {
		fmt.Println("✓ No gaps found - all hourly periods with radar_data have transit records")
		return
	}

	fmt.Printf("\nFound %d hourly periods with radar_data but missing transit records:\n", len(gaps))
	for _, gap := range gaps {
		fmt.Printf("  %s - %s (%d radar_data records)\n",
			gap.Start.Format("2006-01-02 15:04"),
			gap.End.Format("2006-01-02 15:04"),
			gap.RecordCount)
	}

	if dryRun {
		fmt.Println("\nDry run mode - no backfill performed")
		return
	}

	fmt.Println("\nStarting backfill for missing periods...")
	w := db.NewTransitWorker(dbConn, threshold, modelVer)

	for i, gap := range gaps {
		fmt.Printf("\n[%d/%d] Processing %s - %s...\n",
			i+1, len(gaps),
			gap.Start.Format("2006-01-02 15:04"),
			gap.End.Format("2006-01-02 15:04"))

		if err := w.RunRange(context.TODO(), float64(gap.Start.Unix()), float64(gap.End.Unix())); err != nil {
			log.Printf("  ✗ Failed: %v", err)
			continue
		}
		fmt.Printf("  ✓ Completed\n")
	}

	fmt.Println("\n✓ Backfill complete")
}

type transitGap struct {
	Start       time.Time
	End         time.Time
	RecordCount int
}

// findTransitGaps finds hourly periods where radar_data exists but no transits have been computed
func findTransitGaps(dbConn *db.DB) ([]transitGap, error) {
	// Query to find hourly periods with radar_data but no transits
	// We check for each hour bucket if there are radar_data records but no transit records
	query := `
	WITH hourly_data AS (
		-- Get all hourly buckets that have radar_data
		SELECT
			CAST(write_timestamp / 3600 AS INTEGER) * 3600 as hour_start,
			COUNT(*) as data_count
		FROM radar_data
		WHERE speed IS NOT NULL OR magnitude IS NOT NULL
		GROUP BY hour_start
	),
	hourly_transits AS (
		-- Get all hourly buckets that have transits
		SELECT
			CAST(transit_start_unix / 3600 AS INTEGER) * 3600 as hour_start,
			COUNT(*) as transit_count
		FROM radar_data_transits
		GROUP BY hour_start
	)
	SELECT
		hd.hour_start,
		hd.data_count
	FROM hourly_data hd
	WHERE NOT EXISTS (
		SELECT 1 FROM hourly_transits ht
		WHERE ht.hour_start = hd.hour_start
	)
	ORDER BY hd.hour_start
	`

	rows, err := dbConn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var gaps []transitGap
	for rows.Next() {
		var hourStartUnix int64
		var recordCount int64
		if err := rows.Scan(&hourStartUnix, &recordCount); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		start := time.Unix(hourStartUnix, 0).UTC()
		end := start.Add(1 * time.Hour)

		gaps = append(gaps, transitGap{
			Start:       start,
			End:         end,
			RecordCount: int(recordCount),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}

	return gaps, nil
}
