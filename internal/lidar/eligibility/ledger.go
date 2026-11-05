package eligibility

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Use is one recorded decision for one input and one measure family: what
// was asked, by which entry point and actor, against which policy, and the
// outcome. Refused requests are recorded too, so an attempt is visible.
type Use struct {
	UseID           string   `json:"use_id"`
	RequestID       string   `json:"request_id"`
	RecordedAtNs    int64    `json:"recorded_at_ns,string"`
	Purpose         string   `json:"purpose"`
	Measure         Measure  `json:"measure"`
	Decision        string   `json:"decision"` // allowed or denied
	HeldOutScore    bool     `json:"held_out_score"`
	SourceKind      string   `json:"source_kind"`
	CaptureSHA256   string   `json:"capture_sha256,omitempty"`
	CaptureBasename string   `json:"capture_basename,omitempty"`
	SensorID        string   `json:"sensor_id,omitempty"`
	IntervalStartNs int64    `json:"interval_start_ns,string"`
	IntervalEndNs   int64    `json:"interval_end_ns,string"`
	Cohort          Cohort   `json:"cohort"`
	SiteID          string   `json:"site_id,omitempty"`
	Experiment      string   `json:"experiment,omitempty"`
	RunID           string   `json:"run_id,omitempty"`
	Actor           string   `json:"actor"`
	EntryPoint      string   `json:"entry_point"`
	BuildVersion    string   `json:"build_version,omitempty"`
	BuildGitSHA     string   `json:"build_git_sha,omitempty"`
	ConfigDigest    string   `json:"config_digest,omitempty"`
	PolicyDigest    string   `json:"policy_digest"`
	SplitDigest     string   `json:"split_digest,omitempty"`
	Reasons         []Reason `json:"reasons"`
}

// RecordedUTC is the record time in RFC 3339.
func (u Use) RecordedUTC() string {
	return time.Unix(0, u.RecordedAtNs).UTC().Format(time.RFC3339)
}

func (u Use) overlaps(start, end int64) bool {
	whole := func(a, b int64) bool { return a == 0 && b == 0 }
	if whole(start, end) || whole(u.IntervalStartNs, u.IntervalEndNs) {
		return true
	}
	return start <= u.IntervalEndNs && end >= u.IntervalStartNs
}

func usesOf(req Request, d Decision, now time.Time) []Use {
	var out []Use
	for i, in := range d.Inputs {
		decision := "denied"
		if d.Allowed {
			decision = "allowed"
		}
		reasons := append(append([]Reason{}, d.Reasons...), in.Reasons...)
		for _, m := range d.Measures {
			out = append(out, Use{
				UseID: fmt.Sprintf("%s/%d/%s", d.RequestID, i, m), RequestID: d.RequestID, RecordedAtNs: now.UnixNano(),
				Purpose: string(req.Purpose), Measure: m, Decision: decision, HeldOutScore: req.HeldOutScore,
				SourceKind: in.Identity.Kind, CaptureSHA256: in.Identity.SHA256, CaptureBasename: in.Identity.Basename,
				SensorID: in.Identity.SensorID, IntervalStartNs: in.StartNs, IntervalEndNs: in.EndNs,
				Cohort: in.Assignment.Cohort, SiteID: in.Assignment.SiteID, Experiment: req.Experiment, RunID: req.RunID,
				Actor: req.Actor, EntryPoint: req.EntryPoint, BuildVersion: req.BuildVersion, BuildGitSHA: req.BuildGitSHA,
				ConfigDigest: req.ConfigDigest, PolicyDigest: d.PolicyDigest, SplitDigest: req.SplitDigest, Reasons: reasons,
			})
		}
	}
	return out
}

// Ledger keeps the use records.
type Ledger interface {
	Record(uses []Use) error
	// Exposures are the allowed development reads of a capture: tuning,
	// diagnostics and development evaluation that was not a held-out score.
	Exposures(sha string) ([]Use, error)
	List(q UseQuery) ([]Use, int, error)
	Summary() ([]ExposureSummary, error)
}

// UseQuery pages the ledger, newest first.
type UseQuery struct {
	CaptureSHA256 string
	Decision      string
	Limit         int
	Offset        int
}

// ExposureSummary is how often one capture was read for one measure family,
// by decision and purpose: aggregate metadata.
type ExposureSummary struct {
	CaptureSHA256   string  `json:"capture_sha256"`
	CaptureBasename string  `json:"capture_basename"`
	Measure         Measure `json:"measure"`
	DevelopmentUses int     `json:"development_uses"`
	HeldOutScores   int     `json:"held_out_scores"`
	ReviewUses      int     `json:"review_uses"`
	Denied          int     `json:"denied"`
	LastAtNs        int64   `json:"last_at_ns,string"`
}

// SQLLedger keeps uses in the lidar_data_uses table (migration 70). The
// table is append-only: triggers refuse updates and deletes.
type SQLLedger struct{ DB *sql.DB }

const useColumns = `use_id, request_id, recorded_at_ns, purpose, measure, decision, held_out_score, source_kind,
	capture_sha256, capture_basename, sensor_id, interval_start_ns, interval_end_ns, cohort, site_id, experiment,
	run_id, actor, entry_point, build_version, build_git_sha, config_digest, policy_digest, split_digest, reasons_json`

// Record writes the uses in one transaction.
func (l SQLLedger) Record(uses []Use) error {
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
func (l SQLLedger) Exposures(sha string) ([]Use, error) {
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
func (l SQLLedger) List(q UseQuery) ([]Use, int, error) {
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
func (l SQLLedger) Summary() ([]ExposureSummary, error) {
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
	out := []ExposureSummary{}
	for rows.Next() {
		var s ExposureSummary
		var measure string
		if err := rows.Scan(&s.CaptureSHA256, &s.CaptureBasename, &measure, &s.DevelopmentUses, &s.HeldOutScores,
			&s.ReviewUses, &s.Denied, &s.LastAtNs); err != nil {
			return nil, err
		}
		s.Measure = Measure(measure)
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanUses(rows *sql.Rows) ([]Use, error) {
	defer rows.Close()
	out := []Use{}
	for rows.Next() {
		var u Use
		var measure, cohort, reasons string
		var held int
		if err := rows.Scan(&u.UseID, &u.RequestID, &u.RecordedAtNs, &u.Purpose, &measure, &u.Decision, &held, &u.SourceKind,
			&u.CaptureSHA256, &u.CaptureBasename, &u.SensorID, &u.IntervalStartNs, &u.IntervalEndNs, &cohort, &u.SiteID,
			&u.Experiment, &u.RunID, &u.Actor, &u.EntryPoint, &u.BuildVersion, &u.BuildGitSHA, &u.ConfigDigest,
			&u.PolicyDigest, &u.SplitDigest, &reasons); err != nil {
			return nil, err
		}
		u.Measure, u.Cohort, u.HeldOutScore = Measure(measure), Cohort(cohort), held == 1
		if err := json.Unmarshal([]byte(reasons), &u.Reasons); err != nil {
			return nil, fmt.Errorf("use %s reasons: %w", u.UseID, err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
