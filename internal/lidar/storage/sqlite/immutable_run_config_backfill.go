package sqlite

import "fmt"

// ImmutableRunConfigBackfillResult holds backfill counters.
type ImmutableRunConfigBackfillResult struct {
	RunsSeen           int
	RunsUpdated        int
	RunsSkipped        int
	ReplayCasesSeen    int
	ReplayCasesUpdated int
	ReplayCasesSkipped int
}

// BackfillImmutableRunConfigReferences is a no-op after migration 000036
// dropped the legacy params_json and optimal_params_json columns that the
// backfill previously read from.
func BackfillImmutableRunConfigReferences(db DBClient, _ bool) (*ImmutableRunConfigBackfillResult, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}
	return &ImmutableRunConfigBackfillResult{}, nil
}
