package sqlite

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/storage/configasset"
)

type ImmutableRunConfigBackfillResult struct {
	RunsSeen           int
	RunsUpdated        int
	RunsSkipped        int
	ReplayCasesSeen    int
	ReplayCasesUpdated int
	ReplayCasesSkipped int
}

type backfillRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// BackfillImmutableRunConfigReferences repairs legacy LiDAR run and replay-case
// rows by resolving deterministic config assets where the new reference columns
// are still empty.
func BackfillImmutableRunConfigReferences(db DBClient, dryRun bool) (*ImmutableRunConfigBackfillResult, error) {
	if db == nil {
		return nil, fmt.Errorf("database is required")
	}

	result := &ImmutableRunConfigBackfillResult{}
	configStore := configasset.NewStore(db)

	runRows, err := db.Query(`
		SELECT run_id, params_json
		FROM lidar_run_records
		WHERE run_config_id IS NULL OR TRIM(run_config_id) = ''
	`)
	if err != nil {
		return nil, fmt.Errorf("query runs for backfill: %w", err)
	}
	defer runRows.Close()
	if err := backfillRunConfigRows(runRows, result, configStore, db, dryRun); err != nil {
		return nil, err
	}

	sceneStore := NewReplayCaseStore(db)
	sceneCaps, err := sceneStore.replayCaseCapabilities()
	if err != nil {
		return nil, err
	}
	if !sceneCaps.RecommendedParamSetID {
		return result, nil
	}

	sceneRows, err := db.Query(`
		SELECT replay_case_id, optimal_params_json
		FROM lidar_replay_cases
		WHERE (recommended_param_set_id IS NULL OR TRIM(recommended_param_set_id) = '')
		  AND optimal_params_json IS NOT NULL
		  AND TRIM(optimal_params_json) <> ''
	`)
	if err != nil {
		return nil, fmt.Errorf("query replay cases for backfill: %w", err)
	}
	defer sceneRows.Close()
	if err := backfillReplayCaseRows(sceneRows, result, configStore, db, dryRun); err != nil {
		return nil, err
	}

	return result, nil
}

func backfillRunConfigRows(rows backfillRows, result *ImmutableRunConfigBackfillResult, configStore *configasset.Store, db DBClient, dryRun bool) error {
	for rows.Next() {
		var (
			runID      string
			paramsJSON string
		)
		if err := rows.Scan(&runID, &paramsJSON); err != nil {
			return fmt.Errorf("scan run backfill row: %w", err)
		}
		result.RunsSeen++

		if strings.TrimSpace(paramsJSON) == "" {
			result.RunsSkipped++
			continue
		}

		paramSet, err := configasset.MakeLegacyParamSet(json.RawMessage(paramsJSON))
		if err != nil {
			result.RunsSkipped++
			continue
		}
		runConfig, err := configStore.EnsureRunConfig(paramSet, configasset.BuildIdentity{})
		if err != nil {
			return fmt.Errorf("resolve run config for %s: %w", runID, err)
		}
		if !dryRun {
			if _, err := db.Exec(`
				UPDATE lidar_run_records
				SET run_config_id = ?
				WHERE run_id = ?
			`, runConfig.RunConfigID, runID); err != nil {
				return fmt.Errorf("update run_config_id for %s: %w", runID, err)
			}
		}
		result.RunsUpdated++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate runs for backfill: %w", err)
	}
	return nil
}

func backfillReplayCaseRows(rows backfillRows, result *ImmutableRunConfigBackfillResult, configStore *configasset.Store, db DBClient, dryRun bool) error {
	for rows.Next() {
		var (
			replayCaseID string
			optimalJSON  string
		)
		if err := rows.Scan(&replayCaseID, &optimalJSON); err != nil {
			return fmt.Errorf("scan replay-case backfill row: %w", err)
		}
		result.ReplayCasesSeen++

		paramSet, err := configasset.MakeRequestedParamSet(json.RawMessage(optimalJSON))
		if err != nil {
			result.ReplayCasesSkipped++
			continue
		}
		storedParamSet, err := configStore.EnsureParamSet(paramSet)
		if err != nil {
			return fmt.Errorf("resolve recommended params for %s: %w", replayCaseID, err)
		}
		if !dryRun {
			if _, err := db.Exec(`
				UPDATE lidar_replay_cases
				SET recommended_param_set_id = ?
				WHERE replay_case_id = ?
			`, storedParamSet.ParamSetID, replayCaseID); err != nil {
				return fmt.Errorf("update recommended_param_set_id for %s: %w", replayCaseID, err)
			}
		}
		result.ReplayCasesUpdated++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate replay cases for backfill: %w", err)
	}
	return nil
}
