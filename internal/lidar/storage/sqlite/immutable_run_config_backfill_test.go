package sqlite

import (
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

func TestBackfillImmutableRunConfigReferences(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	// After migration 000036, the legacy params_json and optimal_params_json
	// columns have been dropped. The backfill should gracefully skip
	// when those columns no longer exist.
	result, err := BackfillImmutableRunConfigReferences(testDB.DB, false)
	if err != nil {
		t.Fatalf("BackfillImmutableRunConfigReferences() error = %v", err)
	}

	if result.RunsUpdated != 0 {
		t.Fatalf("expected 0 run updates (columns dropped), got %+v", result)
	}
	if result.ReplayCasesUpdated != 0 {
		t.Fatalf("expected 0 replay-case updates (columns dropped), got %+v", result)
	}
}
