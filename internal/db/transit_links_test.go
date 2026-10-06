package db

import (
	"context"
	"testing"
	"time"
)

// Rebuilding a window under one model version leaves another version's
// transits there with their links: the link refresh used to delete every
// transit's links in the window, whatever its version.
func TestTransitWorker_RebuildKeepsOtherVersionsLinks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	base := float64(time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour).Unix())
	for i := 0; i < 10; i++ {
		insertRadarData(t, db, base+float64(i), 15, 40)
	}
	linksOf := func(version string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM radar_transit_links l
			JOIN radar_data_transits t ON t.transit_id = l.transit_id WHERE t.model_version = ?`, version).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	ctx := context.Background()
	if err := NewTransitWorker(db, 5, "hourly-cron").RunRange(ctx, base-10, base+30); err != nil {
		t.Fatal(err)
	}
	cron := linksOf("hourly-cron")
	if cron == 0 {
		t.Fatal("hourly-cron run linked no rows")
	}
	if err := NewTransitWorker(db, 5, "rebuild-full").RunRange(ctx, base-10, base+30); err != nil {
		t.Fatal(err)
	}
	if got := linksOf("hourly-cron"); got != cron {
		t.Errorf("hourly-cron transits keep %d links after a rebuild-full pass, want %d", got, cron)
	}
	if linksOf("rebuild-full") == 0 {
		t.Error("rebuild-full run linked no rows")
	}
}
