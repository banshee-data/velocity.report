// Package labelworkdb keeps the labelling workspace's records in the
// server's SQLite database: the eligibility use ledger (lidar_data_uses) and
// the workflow items and their history (lidar_workflow_items,
// lidar_workflow_events), all added by migration 70. It is the only place
// those tables are queried.
package labelworkdb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
)

// UseLedger keeps uses in the lidar_data_uses table (migration 70). The
// table is append-only: triggers refuse updates and deletes.
type UseLedger struct{ DB *sql.DB }

const useColumns = `use_id, request_id, recorded_at_ns, purpose, measure, decision, held_out_score, source_kind,
	capture_sha256, capture_basename, sensor_id, interval_start_ns, interval_end_ns, cohort, site_id, experiment,
	run_id, actor, entry_point, build_version, build_git_sha, config_digest, policy_digest, split_digest, reasons_json`

// Record writes the uses in one transaction.
func (l UseLedger) Record(uses []eligibility.Use) error {
	if len(uses) == 0 {
		return nil
	}
	tx, err := l.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO lidar_data_uses (` + useColumns + `) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, u := range uses {
		reasons, err := json.Marshal(u.Reasons)
		if err != nil {
			return err
		}
		if _, err := stmt.Exec(u.UseID, u.RequestID, u.RecordedAtNs, u.Purpose, string(u.Measure), u.Decision, boolInt(u.HeldOutScore),
			u.SourceKind, u.CaptureSHA256, u.CaptureBasename, u.SensorID, u.IntervalStartNs, u.IntervalEndNs, string(u.Cohort),
			u.SiteID, u.Experiment, u.RunID, u.Actor, u.EntryPoint, u.BuildVersion, u.BuildGitSHA, u.ConfigDigest,
			u.PolicyDigest, u.SplitDigest, string(reasons)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Exposures returns the capture's allowed development reads.
func (l UseLedger) Exposures(sha string) ([]eligibility.Use, error) {
	rows, err := l.DB.Query(`SELECT `+useColumns+` FROM lidar_data_uses
		WHERE capture_sha256 = ? AND decision = 'allowed'
		  AND (purpose IN ('tuning', 'developer_diagnostic') OR (purpose = 'development_evaluation' AND held_out_score = 0))
		ORDER BY recorded_at_ns`, sha)
	if err != nil {
		return nil, err
	}
	return scanUses(rows)
}

// List pages the ledger, newest first, with the total that matches.
func (l UseLedger) List(q eligibility.UseQuery) ([]eligibility.Use, int, error) {
	var where []string
	var args []any
	if q.CaptureSHA256 != "" {
		where, args = append(where, "capture_sha256 = ?"), append(args, q.CaptureSHA256)
	}
	if q.Decision != "" {
		where, args = append(where, "decision = ?"), append(args, q.Decision)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := l.DB.QueryRow(`SELECT COUNT(*) FROM lidar_data_uses`+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := l.DB.Query(`SELECT `+useColumns+` FROM lidar_data_uses`+clause+
		` ORDER BY recorded_at_ns DESC, use_id LIMIT ? OFFSET ?`, append(args, limit, max(q.Offset, 0))...)
	if err != nil {
		return nil, 0, err
	}
	uses, err := scanUses(rows)
	return uses, total, err
}

// Summary counts uses per capture and measure family.
func (l UseLedger) Summary() ([]eligibility.ExposureSummary, error) {
	rows, err := l.DB.Query(`SELECT capture_sha256, MAX(capture_basename), measure,
		SUM(CASE WHEN decision = 'allowed' AND (purpose IN ('tuning', 'developer_diagnostic') OR (purpose = 'development_evaluation' AND held_out_score = 0)) THEN 1 ELSE 0 END),
		SUM(CASE WHEN decision = 'allowed' AND held_out_score = 1 THEN 1 ELSE 0 END),
		SUM(CASE WHEN decision = 'allowed' AND purpose = 'annotation_review' THEN 1 ELSE 0 END),
		SUM(CASE WHEN decision = 'denied' THEN 1 ELSE 0 END),
		MAX(recorded_at_ns)
		FROM lidar_data_uses WHERE source_kind = 'capture'
		GROUP BY capture_sha256, measure ORDER BY MAX(recorded_at_ns) DESC LIMIT 2000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []eligibility.ExposureSummary{}
	for rows.Next() {
		var s eligibility.ExposureSummary
		var measure string
		if err := rows.Scan(&s.CaptureSHA256, &s.CaptureBasename, &measure, &s.DevelopmentUses, &s.HeldOutScores,
			&s.ReviewUses, &s.Denied, &s.LastAtNs); err != nil {
			return nil, err
		}
		s.Measure = eligibility.Measure(measure)
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanUses(rows *sql.Rows) ([]eligibility.Use, error) {
	defer rows.Close()
	out := []eligibility.Use{}
	for rows.Next() {
		var u eligibility.Use
		var measure, cohort, reasons string
		var held int
		if err := rows.Scan(&u.UseID, &u.RequestID, &u.RecordedAtNs, &u.Purpose, &measure, &u.Decision, &held, &u.SourceKind,
			&u.CaptureSHA256, &u.CaptureBasename, &u.SensorID, &u.IntervalStartNs, &u.IntervalEndNs, &cohort, &u.SiteID,
			&u.Experiment, &u.RunID, &u.Actor, &u.EntryPoint, &u.BuildVersion, &u.BuildGitSHA, &u.ConfigDigest,
			&u.PolicyDigest, &u.SplitDigest, &reasons); err != nil {
			return nil, err
		}
		u.Measure, u.Cohort, u.HeldOutScore = eligibility.Measure(measure), eligibility.Cohort(cohort), held == 1
		if err := json.Unmarshal([]byte(reasons), &u.Reasons); err != nil {
			return nil, fmt.Errorf("use %s reasons: %w", u.UseID, err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
