package jobs

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JobRequest is what an operator, the dashboard or the CLI submits. The hub
// validates it, derives its run identity and queues it. It carries
// parameters, never a command line, a host path outside a named root, or a
// container option: the worker turns a kind into an invocation itself.
type JobRequest struct {
	// Kind names an entry in the registry; see Kinds.
	Kind string `json:"kind"`
	// CaptureManifest is the input, by content. The hub stores the manifest
	// and keys the job by its digest.
	CaptureManifest CaptureManifest `json:"capture_manifest"`
	// Tuning is the whole resolved tuning document the job runs with, so the
	// job does not depend on any host's config directory. Digested whole.
	Tuning json.RawMessage `json:"tuning"`
	// Experiments are the named experiments switched on, sorted; part of what
	// the pipeline does and so of the identity.
	Experiments []string       `json:"experiments,omitempty"`
	Code        CodeIdentity   `json:"code"`
	Replay      ReplayContract `json:"replay"`
	// Params are the kind's own parameters, validated by the kind.
	Params json.RawMessage `json:"params,omitempty"`
	// Worker names the worker this job must run on, or "" for any compatible
	// one. Not part of the identity: where a job ran does not change what it
	// computed.
	Worker string `json:"worker,omitempty"`
	// Note is free text for the operator's benefit, not part of the identity.
	Note string `json:"note,omitempty"`
}

// CodeIdentity fixes which code a job runs. A worker builds or fetches the
// kind's tool from exactly this, so a result can be traced to source.
type CodeIdentity struct {
	// GitSHA is the commit the tool is built from. Required: a job that ran
	// "whatever was checked out" cannot be repeated.
	GitSHA string `json:"git_sha"`
	// SourceTreeDigest is set instead of, or as well as, GitSHA when the tree
	// carries uncommitted changes: a digest of an archived snapshot of it.
	SourceTreeDigest Digest `json:"source_tree_digest,omitempty"`
	// BuildTags are the Go build tags, sorted; "pcap" is the one that matters.
	BuildTags []string `json:"build_tags,omitempty"`
}

// ReplayContract is how the captures are replayed and scored. Two runs of the
// same bytes with different windows are not comparable.
type ReplayContract struct {
	SensorID string `json:"sensor_id"`
	// StartSeconds and DurationSeconds bound the replay in capture time.
	// DurationSeconds zero means the whole capture.
	StartSeconds    float64 `json:"start_seconds"`
	DurationSeconds float64 `json:"duration_seconds"`
	// WarmupSeconds is how long the background settles before scoring.
	WarmupSeconds float64 `json:"warmup_seconds"`
	// MeasurementMode is the pipeline's measurement mode, or "" for default.
	MeasurementMode string `json:"measurement_mode,omitempty"`
}

// RunIdentity is the canonical digest of a request's comparable parts:
// input bytes and order, tuning, experiments, code, replay contract, kind and
// its parameters, and the output schema. Not the worker, not the time, not
// the note. Two accepted results with one identity computed the same thing.
type RunIdentity struct {
	Kind                  string         `json:"kind"`
	CaptureManifestDigest Digest         `json:"capture_manifest_digest"`
	TuningDigest          Digest         `json:"tuning_digest"`
	Experiments           []string       `json:"experiments"`
	Code                  CodeIdentity   `json:"code"`
	Replay                ReplayContract `json:"replay"`
	ParamsDigest          Digest         `json:"params_digest"`
	OutputSchema          int            `json:"output_schema"`
}

// OutputSchemaVersion is the result bundle layout this build writes and
// reads. A future reader rejects a bundle from another.
const OutputSchemaVersion = 1

// Validate checks the request against the contract and its kind.
func (r JobRequest) Validate() error {
	kind, ok := Kinds[r.Kind]
	if !ok {
		return fmt.Errorf("unknown job kind %q", r.Kind)
	}
	if err := r.CaptureManifest.Validate(); err != nil {
		return fmt.Errorf("capture manifest: %w", err)
	}
	if len(r.Tuning) == 0 {
		return fmt.Errorf("tuning document is required: a job carries the whole config it runs with")
	}
	if !json.Valid(r.Tuning) {
		return fmt.Errorf("tuning document is not valid JSON")
	}
	if err := r.Code.Validate(); err != nil {
		return err
	}
	if err := r.Replay.Validate(); err != nil {
		return err
	}
	if r.Replay.SensorID != r.CaptureManifest.SensorID {
		return fmt.Errorf("replay sensor %q is not the manifest's sensor %q", r.Replay.SensorID, r.CaptureManifest.SensorID)
	}
	for i := 1; i < len(r.Experiments); i++ {
		if r.Experiments[i] <= r.Experiments[i-1] {
			return fmt.Errorf("experiments must be sorted and unique")
		}
	}
	for _, e := range r.Experiments {
		if strings.TrimSpace(e) == "" {
			return fmt.Errorf("an experiment name is empty")
		}
	}
	if err := kind.ValidateParams(r.Params); err != nil {
		return fmt.Errorf("%s params: %w", r.Kind, err)
	}
	return nil
}

// Validate checks a code identity names something a worker can fetch.
func (c CodeIdentity) Validate() error {
	sha := strings.ToLower(strings.TrimSpace(c.GitSHA))
	if len(sha) != 40 {
		return fmt.Errorf("code git_sha must be a full 40-character commit, got %q", c.GitSHA)
	}
	for _, ch := range sha {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return fmt.Errorf("code git_sha %q is not hex", c.GitSHA)
		}
	}
	if c.SourceTreeDigest != "" && !c.SourceTreeDigest.Valid() {
		return fmt.Errorf("code source_tree_digest is not a valid digest")
	}
	for i := 1; i < len(c.BuildTags); i++ {
		if c.BuildTags[i] <= c.BuildTags[i-1] {
			return fmt.Errorf("build_tags must be sorted and unique")
		}
	}
	return nil
}

// Validate checks the replay contract.
func (rc ReplayContract) Validate() error {
	if strings.TrimSpace(rc.SensorID) == "" {
		return fmt.Errorf("replay sensor_id is required")
	}
	if rc.StartSeconds < 0 || rc.DurationSeconds < 0 || rc.WarmupSeconds < 0 {
		return fmt.Errorf("replay seconds must not be negative")
	}
	if rc.DurationSeconds > 0 && rc.WarmupSeconds >= rc.DurationSeconds {
		return fmt.Errorf("replay warm-up of %.0fs leaves nothing of a %.0fs window to score", rc.WarmupSeconds, rc.DurationSeconds)
	}
	return nil
}

// Identity derives the run identity. It validates first: an identity of an
// invalid request would be a digest of nothing in particular.
func (r JobRequest) Identity() (RunIdentity, Digest, error) {
	if err := r.Validate(); err != nil {
		return RunIdentity{}, "", err
	}
	manifestDigest, err := r.CaptureManifest.Digest()
	if err != nil {
		return RunIdentity{}, "", err
	}
	var tuning any
	if err := json.Unmarshal(r.Tuning, &tuning); err != nil {
		return RunIdentity{}, "", fmt.Errorf("tuning: %w", err)
	}
	tuningDigest, err := DigestCanonical(tuning)
	if err != nil {
		return RunIdentity{}, "", err
	}
	// Absent params and empty params are the same thing.
	var params any = map[string]any{}
	if len(r.Params) > 0 {
		if err := json.Unmarshal(r.Params, &params); err != nil {
			return RunIdentity{}, "", fmt.Errorf("params: %w", err)
		}
	}
	paramsDigest, err := DigestCanonical(params)
	if err != nil {
		return RunIdentity{}, "", err
	}
	experiments := r.Experiments
	if experiments == nil {
		experiments = []string{}
	}
	code := r.Code
	code.GitSHA = strings.ToLower(strings.TrimSpace(code.GitSHA))
	if code.BuildTags == nil {
		code.BuildTags = []string{}
	}
	id := RunIdentity{
		Kind: r.Kind, CaptureManifestDigest: manifestDigest, TuningDigest: tuningDigest,
		Experiments: experiments, Code: code, Replay: r.Replay, ParamsDigest: paramsDigest,
		OutputSchema: OutputSchemaVersion,
	}
	digest, err := DigestCanonical(id)
	if err != nil {
		return RunIdentity{}, "", err
	}
	return id, digest, nil
}
