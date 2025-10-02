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
	var startStr string
	var endStr string
	var threshold int
	var modelVer string

	flag.StringVar(&dbPath, "db", "sensor_data.db", "path to sqlite db")
	flag.StringVar(&startStr, "start", "", "start time (RFC3339)")
	flag.StringVar(&endStr, "end", "", "end time (RFC3339)")
	flag.IntVar(&threshold, "gap", 1, "session gap in seconds")
	flag.StringVar(&modelVer, "model", "manual-backfill", "model version string for transits")
	flag.Parse()

	if startStr == "" || endStr == "" {
		log.Fatalf("start and end must be provided")
	}

	startT, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		log.Fatalf("invalid start: %v", err)
	}
	endT, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		log.Fatalf("invalid end: %v", err)
	}

	dbConn, err := db.NewDB(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer dbConn.Close()

	w := db.NewTransitWorker(dbConn, threshold, modelVer)

	// run the backfill in windows of 20 minutes (with no wait) until range covered
	t := startT.UTC()
	for t.Before(endT.UTC()) {
		windowStart := t
		windowEnd := t.Add(w.Window)
		if windowEnd.After(endT.UTC()) {
			windowEnd = endT.UTC()
		}
		fmt.Printf("backfilling window %s -> %s\n", windowStart, windowEnd)
		// run the worker for this specific window
		if err := w.RunRange(context.TODO(), float64(windowStart.Unix()), float64(windowEnd.Unix())); err != nil {
			log.Fatalf("runrange failed: %v", err)
		}
		// advance by window (no overlap) — small overlap could be added if desired
		t = windowEnd
	}

	fmt.Println("backfill complete")
}
