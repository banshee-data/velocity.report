package sqlite

import (
	"fmt"
	"strings"
)

// Read-only lookups the runs and clips lists derive on read, so the Runs page
// can group a run under its clip and fold a sweep's runs under the sweep.
// Nothing here is stored: a run names no sweep and a clip list row names no
// site label, so both are joined in when the list is read.

// listingBatch bounds the placeholders in one IN (...) query; SQLite's limit
// is far higher, but the lists these serve are paged at a few hundred rows.
const listingBatch = 500

// RunSweep names the sweep whose results list a run.
type RunSweep struct {
	SweepID string
	Mode    string
}

// SweepsForRuns maps each run ID to the sweep that ran it, read from the run
// IDs a stored sweep records: each combination's run_id in its results,
// written when it completes or fails, and in its checkpoint results, written
// at each round boundary of a checkpointed sweep and kept when one is
// suspended; and each round's reference_run_id in a HINT sweep's round
// results, since HINT stores no results of its own. The runs of a sweep that
// has stored none of these yet are not matched. When a run appears in several
// sweeps (a replay that failed to start leaves the previous run's ID on the
// next combination) the earliest sweep is the one that ran it.
//
// Every call parses every stored sweep's JSON, so callers ask for it only
// when they will fold runs by sweep. Recording the sweep on the run when the
// sweep runner starts it would replace the scan.
func SweepsForRuns(db DBClient, runIDs []string) (map[string]RunSweep, error) {
	out := make(map[string]RunSweep)
	for start := 0; start < len(runIDs); start += listingBatch {
		end := min(start+listingBatch, len(runIDs))
		batch := runIDs[start:end]
		if len(batch) == 0 {
			continue
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		args := make([]any, len(batch))
		for i, id := range batch {
			args[i] = id
		}
		query := fmt.Sprintf(`
			SELECT run_id, sweep_id, mode FROM (
				SELECT JSON_EXTRACT(r.value, '$.run_id') AS run_id, s.sweep_id, s.mode, s.id AS sid
				  FROM lidar_tuning_sweeps s, JSON_EACH(s.results) r
				 WHERE s.results IS NOT NULL AND JSON_VALID(s.results)
				   AND JSON_TYPE(s.results) = 'array'
				   AND JSON_TYPE(r.value, '$.run_id') = 'text'
				UNION ALL
				SELECT JSON_EXTRACT(r.value, '$.run_id'), s.sweep_id, s.mode, s.id
				  FROM lidar_tuning_sweeps s, JSON_EACH(s.checkpoint_results) r
				 WHERE s.checkpoint_results IS NOT NULL AND JSON_VALID(s.checkpoint_results)
				   AND JSON_TYPE(s.checkpoint_results) = 'array'
				   AND JSON_TYPE(r.value, '$.run_id') = 'text'
				UNION ALL
				SELECT JSON_EXTRACT(r.value, '$.reference_run_id'), s.sweep_id, s.mode, s.id
				  FROM lidar_tuning_sweeps s, JSON_EACH(s.round_results) r
				 WHERE s.round_results IS NOT NULL AND JSON_VALID(s.round_results)
				   AND JSON_TYPE(s.round_results) = 'array'
				   AND JSON_TYPE(r.value, '$.reference_run_id') = 'text'
			)
			 WHERE run_id != '' AND run_id IN (%s)
			 ORDER BY sid`, placeholders)
		rows, err := db.Query(query, args...)
		if err != nil {
			return nil, fmt.Errorf("sweeps for runs: %w", err)
		}
		for rows.Next() {
			var runID, sweepID, mode string
			if err := rows.Scan(&runID, &sweepID, &mode); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan sweep for run: %w", err)
			}
			if _, seen := out[runID]; !seen {
				out[runID] = RunSweep{SweepID: sweepID, Mode: mode}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("sweeps for runs: %w", err)
		}
		rows.Close()
	}
	return out, nil
}

// AttachRunSweeps sets SweepID and SweepMode on each run a sweep ran.
func AttachRunSweeps(db DBClient, runs []*Run) error {
	ids := make([]string, 0, len(runs))
	for _, run := range runs {
		if run != nil {
			ids = append(ids, run.RunID)
		}
	}
	sweeps, err := SweepsForRuns(db, ids)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run == nil {
			continue
		}
		if sweep, ok := sweeps[run.RunID]; ok {
			run.SweepID = sweep.SweepID
			run.SweepMode = sweep.Mode
		}
	}
	return nil
}

// AttachClipSites sets SiteID and SiteLabel on each located clip: the site
// is the L16 cell a clip's accepted position falls in, and its label is the
// operator's name for it when one has been given.
func AttachClipSites(db DBClient, clips []*Clip) error {
	if len(clips) == 0 {
		return nil
	}
	byID := make(map[string]*Clip, len(clips))
	for _, clip := range clips {
		if clip != nil {
			byID[clip.ClipID] = clip
		}
	}
	rows, err := db.Query(`
		SELECT c.clip_id, c.site_id, COALESCE(s.label, '')
		  FROM lidar_clips c
		  LEFT JOIN lidar_sites s ON s.site_id = c.site_id
		 WHERE c.geographic_status = ? AND c.site_id IS NOT NULL AND c.site_id != ''`, GeoLocated)
	if err != nil {
		return fmt.Errorf("clip sites: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var clipID, siteID, label string
		if err := rows.Scan(&clipID, &siteID, &label); err != nil {
			return fmt.Errorf("scan clip site: %w", err)
		}
		if clip, ok := byID[clipID]; ok {
			clip.SiteID = siteID
			clip.SiteLabel = strings.TrimSpace(label)
		}
	}
	return rows.Err()
}
