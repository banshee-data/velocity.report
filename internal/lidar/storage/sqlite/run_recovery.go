package sqlite

import (
	"fmt"
	"time"
)

// OrphanedRunError is the error a run left "running" by a previous process
// is failed with, the same words a sweep orphaned the same way records.
const OrphanedRunError = "incomplete: server restarted"

// RecoverOrphanedRuns fails every run a previous process left "running" when
// it stopped: the replay that was writing it is gone, so it will never
// finish. Call once during server startup, before any replay begins. It
// returns how many runs it failed.
//
// A restart part-way through a sweep round also leaves the round's finished
// runs complete; a resumed sweep replays the whole round and writes them
// again, so those duplicates stay.
func (s *RunStore) RecoverOrphanedRuns() (int64, error) {
	caps, err := s.runRecordCapabilities()
	if err != nil {
		return 0, err
	}
	query := `UPDATE lidar_runs SET status = 'failed', error_message = ? WHERE status = 'running'`
	args := []any{OrphanedRunError}
	if caps.CompletedAt {
		query = `UPDATE lidar_runs SET status = 'failed', error_message = ?, completed_at = ? WHERE status = 'running'`
		args = append(args, time.Now().UnixNano())
	}
	var affected int64
	err = retryOnBusy(func() error {
		res, err := s.db.Exec(query, args...)
		if err != nil {
			return err
		}
		affected, _ = res.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("recovering orphaned runs: %w", err)
	}
	return affected, nil
}
