package jobs

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Kind is one entry in the allowlist of what a worker may be asked to run.
//
// A kind names a tool and reads that tool's parameters from a job; it never
// takes a command line. The worker owns the mapping from kind and parameters
// to an invocation, and a kind that is not here does not run anywhere.
type Kind struct {
	Name string
	// Tool is the program the worker runs: a cmd/tools name built from the
	// job's commit, or "in-process" for work the worker binary does itself.
	Tool string
	// OnMain says whether the tool is on main today. A kind whose tool lives
	// on a branch runs only from a commit on that branch, and the dashboard
	// can say so.
	OnMain bool
	// Summary is the file in the bundle the kind's summary is read from.
	Summary string
	// ValidateParams rejects parameters the kind does not understand or that
	// would take a worker outside its roots. Nil or empty params are valid
	// for every kind: each has usable defaults.
	ValidateParams func(params json.RawMessage) error
}

// Kinds is the registry.
var Kinds = map[string]Kind{
	KindBenchmark: {
		Name: KindBenchmark, Tool: "in-process", OnMain: true, Summary: "benchmark.json",
		ValidateParams: validateBenchmarkParams,
	},
	KindStateEstimationBaseline: {
		Name: KindStateEstimationBaseline, Tool: "lidar-state-estimation-baseline", OnMain: false,
		Summary: "phase0-summary.json", ValidateParams: validateBaselineParams,
	},
	KindTrackScorecard: {
		Name: KindTrackScorecard, Tool: "lidar-track-scorecard", OnMain: false,
		Summary: "scorecard.json", ValidateParams: validateScorecardParams,
	},
	KindVRLOGRecord: {
		Name: KindVRLOGRecord, Tool: "in-process", OnMain: true, Summary: "run.json",
		ValidateParams: validateVRLOGParams,
	},
}

// The kind names. Stable: they appear in stored jobs and bundle manifests.
const (
	KindBenchmark               = "benchmark"
	KindStateEstimationBaseline = "state_estimation_baseline"
	KindTrackScorecard          = "track_scorecard"
	KindVRLOGRecord             = "vrlog_record"
)

// KindNames lists the registry in a fixed order, for the dashboard's form.
func KindNames() []string {
	return []string{KindBenchmark, KindStateEstimationBaseline, KindTrackScorecard, KindVRLOGRecord}
}

// BenchmarkParams are lidar-bench's knobs that change what is measured.
type BenchmarkParams struct {
	// Profile reduces pipeline depth: "", "l3-only", "detect" or "full".
	Profile string `json:"profile,omitempty"`
	// Repeats runs the benchmark N times and reports the median.
	Repeats int `json:"repeats,omitempty"`
}

// BaselineParams are the state-estimation baseline tool's knobs.
type BaselineParams struct {
	// Case is the corpus case id to replay; the manifest's captures are it.
	Case string `json:"case"`
	// SurfaceGround switches the tool's ground surfacing on.
	SurfaceGround bool `json:"surface_ground,omitempty"`
	// KeepEvidence keeps the evidence database in the bundle. Large.
	KeepEvidence bool `json:"keep_evidence,omitempty"`
}

// ScorecardParams are the track scorecard's knobs. It scores a baseline
// job's evidence, so it names that job.
type ScorecardParams struct {
	// EvidenceJob is the accepted baseline job whose evidence is scored.
	EvidenceJob string `json:"evidence_job"`
	// ScoringStartSeconds is where scoring begins; usually the warm-up.
	ScoringStartSeconds float64 `json:"scoring_start_seconds,omitempty"`
}

// VRLOGParams are a recording replay's knobs.
type VRLOGParams struct {
	// IncludePoints records point clouds, which an annotation pack needs.
	IncludePoints bool `json:"include_points,omitempty"`
	// SettleFirst replays the settling period before recording starts.
	SettleFirst bool `json:"settle_first,omitempty"`
}

func decodeParams(params json.RawMessage, into any) error {
	if len(params) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return err
	}
	return nil
}

func validateBenchmarkParams(params json.RawMessage) error {
	var p BenchmarkParams
	if err := decodeParams(params, &p); err != nil {
		return err
	}
	switch p.Profile {
	case "", "l3-only", "detect", "full":
	default:
		return fmt.Errorf("unknown profile %q", p.Profile)
	}
	if p.Repeats < 0 || p.Repeats > 25 {
		return fmt.Errorf("repeats %d is outside 0 to 25", p.Repeats)
	}
	return nil
}

func validateBaselineParams(params json.RawMessage) error {
	var p BaselineParams
	if err := decodeParams(params, &p); err != nil {
		return err
	}
	if strings.TrimSpace(p.Case) == "" {
		return fmt.Errorf("case is required")
	}
	if strings.ContainsAny(p.Case, "/\\ \t\n") {
		return fmt.Errorf("case %q is a corpus case id, not a path", p.Case)
	}
	return nil
}

func validateScorecardParams(params json.RawMessage) error {
	var p ScorecardParams
	if err := decodeParams(params, &p); err != nil {
		return err
	}
	if strings.TrimSpace(p.EvidenceJob) == "" {
		return fmt.Errorf("evidence_job is required: a scorecard scores a baseline job's evidence")
	}
	if p.ScoringStartSeconds < 0 {
		return fmt.Errorf("scoring_start_seconds must not be negative")
	}
	return nil
}

func validateVRLOGParams(params json.RawMessage) error {
	var p VRLOGParams
	return decodeParams(params, &p)
}
