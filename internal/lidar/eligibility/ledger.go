package eligibility

import (
	"fmt"
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
