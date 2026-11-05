package eligibility

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Purpose is what a request will do with the samples. The server derives it
// from the entry point a request arrives at; a client never chooses it, so a
// client cannot name its way out of a restriction.
type Purpose string

const (
	// PurposeTuning is parameter search of any kind: sweeps, auto-tune,
	// HINT, and replays of an experiment arm's parameter set.
	PurposeTuning Purpose = "tuning"
	// PurposeDevelopmentEvaluation scores a candidate against references.
	// With HeldOutScore it is a held-out score under a frozen split.
	PurposeDevelopmentEvaluation Purpose = "development_evaluation"
	// PurposeDeveloperDiagnostic is any developer-facing read of samples
	// or per-example results: previews, residual tables, oracle scorers.
	PurposeDeveloperDiagnostic Purpose = "developer_diagnostic"
	// PurposeAnnotationReview is a person labelling or reviewing the data.
	PurposeAnnotationReview Purpose = "annotation_review"
	// PurposeQualificationEvaluation is isolated blind scoring of the
	// qualification cohort by an authorised custodian.
	PurposeQualificationEvaluation Purpose = "qualification_evaluation"
	// PurposeProduction is ordinary capture, processing and publication.
	// It is not gated, and nothing is recorded for it.
	PurposeProduction Purpose = "production"
)

// Purposes are every gated purpose, in the order a page lists them.
var Purposes = []Purpose{
	PurposeTuning, PurposeDevelopmentEvaluation, PurposeDeveloperDiagnostic,
	PurposeAnnotationReview, PurposeQualificationEvaluation,
}

func (p Purpose) valid() bool {
	for _, q := range Purposes {
		if p == q {
			return true
		}
	}
	return p == PurposeProduction
}

// developmentRead reports whether the purpose exposes the data to
// development: once read this way, a measure is tuning for that capture.
func (p Purpose) developmentRead(heldOutScore bool) bool {
	switch p {
	case PurposeTuning, PurposeDeveloperDiagnostic:
		return true
	case PurposeDevelopmentEvaluation:
		return !heldOutScore
	}
	return false
}

// Measure is a family of measurements whose exposure is tracked apart. A
// capture read for one family is tuning for that family afterwards and stays
// unread for the others (experiment review 2026-11-05, F21).
type Measure string

const (
	MeasureIdentity Measure = "identity" // identity, noise and counts
	MeasureGeometry Measure = "geometry"
	MeasureGround   Measure = "ground"
	MeasureSettling Measure = "settling"
)

// Measures are every measure family.
var Measures = []Measure{MeasureIdentity, MeasureGeometry, MeasureGround, MeasureSettling}

// ParseMeasures reads a comma-separated list; empty is every family, the
// strictest reading.
func ParseMeasures(s string) ([]Measure, error) {
	if strings.TrimSpace(s) == "" {
		return append([]Measure(nil), Measures...), nil
	}
	var out []Measure
	for _, part := range strings.Split(s, ",") {
		m := Measure(strings.TrimSpace(part))
		ok := false
		for _, k := range Measures {
			ok = ok || k == m
		}
		if !ok {
			return nil, fmt.Errorf("measure %q is not one of identity, geometry, ground, settling", m)
		}
		out = append(out, m)
	}
	return out, nil
}

// Input is one capture or live source a request would read, and the stretch
// of it. A zero StartNs and EndNs is the whole capture. Err is set when the
// input's identity could not be established, which fails closed.
type Input struct {
	Identity Identity
	StartNs  int64
	EndNs    int64
	Err      error
}

// Request is one decision to make.
type Request struct {
	Purpose  Purpose
	Measures []Measure
	Inputs   []Input
	// HeldOutScore marks a development evaluation that scores held-out data
	// as held out under SplitDigest. Held-out stretches allow it; anything
	// tuned on, in any lineage, or read for development for one of the
	// measures, refuses it.
	HeldOutScore bool
	SplitDigest  string
	// Provenance recorded with the decision.
	EntryPoint   string
	Actor        string
	Experiment   string
	RunID        string
	ConfigDigest string
	BuildVersion string
	BuildGitSHA  string
}

// Reason is one thing a decision found. A blocking reason refuses the
// request; the others are recorded and shown.
type Reason struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}

// Reason codes.
const (
	CodeNotConfigured      = "manifest_absent"
	CodeManifestInvalid    = "manifest_invalid"
	CodeUnassigned         = "cohort_unassigned"
	CodeConflict           = "capture_conflict"
	CodeIdentityUnknown    = "identity_unknown"
	CodeValidation         = "cohort_validation"
	CodeQualification      = "cohort_qualification"
	CodeAccessAbsent       = "access_controls_absent"
	CodeNotCustodian       = "not_custodian"
	CodeHeldOut            = "held_out"
	CodeHashMismatch       = "hash_mismatch"
	CodeTunedInLineage     = "tuned_in_lineage"
	CodeExposed            = "exposed_for_measure"
	CodeRegistryIncomplete = "protection_registry_incomplete"
	CodeLedgerUnavailable  = "ledger_unavailable"
	CodeBadRequest         = "bad_request"
	CodeWeakIdentity       = "weak_identity"
)

// InputDecision is the decision for one input.
type InputDecision struct {
	Identity   Identity   `json:"identity"`
	StartNs    int64      `json:"start_ns,string"`
	EndNs      int64      `json:"end_ns,string"`
	Assignment Assignment `json:"assignment"`
	Allowed    bool       `json:"allowed"`
	Reasons    []Reason   `json:"reasons"`
}

// Decision is the outcome of a request.
type Decision struct {
	RequestID string  `json:"request_id"`
	Purpose   Purpose `json:"purpose"`
	// Gated is false for production, which is not checked or recorded.
	Gated        bool            `json:"gated"`
	Allowed      bool            `json:"allowed"`
	Measures     []Measure       `json:"measures"`
	HeldOutScore bool            `json:"held_out_score"`
	SplitDigest  string          `json:"split_digest,omitempty"`
	PolicyDigest string          `json:"policy_digest"`
	Reasons      []Reason        `json:"reasons"`
	Inputs       []InputDecision `json:"inputs"`
	// Recorded is true once the decision is in the ledger.
	Recorded bool `json:"recorded"`
}

// Summary is every blocking reason, one per line.
func (d *Decision) Summary() string {
	var lines []string
	for _, r := range d.Reasons {
		if r.Blocking {
			lines = append(lines, r.Message)
		}
	}
	for _, in := range d.Inputs {
		name := in.Identity.Basename
		if in.Identity.Kind == KindLive {
			name = "live sensor " + in.Identity.SensorID
		}
		for _, r := range in.Reasons {
			if r.Blocking {
				lines = append(lines, name+": "+r.Message)
			}
		}
	}
	return strings.Join(lines, "; ")
}

// ErrNotEligible is returned by Authorize for a refused request.
var ErrNotEligible = errors.New("not eligible")

// AccessControl is what protects qualification data at the storage and
// backend level. This repository provides none: the default refuses every
// qualification read. A manifest's own claim is never enough.
type AccessControl interface {
	QualificationStorageEnforced() bool
}

// Config configures a Service.
type Config struct {
	// ManifestPath is the cohort manifest; empty means none is configured
	// and every capture is unassigned.
	ManifestPath string
	// SplitsDir holds the frozen splits the registry reads.
	SplitsDir string
	// ExtraHashes returns capture hashes verified after a pack was frozen,
	// by pack digest. Optional.
	ExtraHashes func() map[string][]string
	Ledger      Ledger
	Access      AccessControl
	Hasher      *Hasher
	Now         func() time.Time
}

// Service makes and records eligibility decisions.
type Service struct {
	cfg Config
}

// New returns a service.
func New(cfg Config) *Service {
	if cfg.Hasher == nil {
		cfg.Hasher = NewHasher()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg}
}

// Hasher is the service's capture hasher.
func (s *Service) Hasher() *Hasher { return s.cfg.Hasher }

// CaptureInput identifies a capture file as an input.
func (s *Service) CaptureInput(path string, startNs, endNs int64) Input {
	id, err := s.cfg.Hasher.Capture(path)
	if err != nil {
		return Input{Identity: Identity{Kind: KindCapture, Basename: baseOf(path)}, Err: err}
	}
	return Input{Identity: id, StartNs: startNs, EndNs: endNs}
}

func baseOf(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

// policy is everything a decision reads, read fresh for each decision so an
// edit to the manifest or a new split is never decided against a stale copy.
type policy struct {
	manifest    *LoadedManifest
	manifestErr error
	configured  bool
	registry    *Registry
	registryErr error
	digest      string
}

func (s *Service) readPolicy() policy {
	var p policy
	if s.cfg.ManifestPath != "" {
		p.configured = true
		p.manifest, p.manifestErr = LoadManifest(s.cfg.ManifestPath)
		if errors.Is(p.manifestErr, os.ErrNotExist) {
			p.manifestErr = fmt.Errorf("%w: %s does not exist", ErrManifestInvalid, s.cfg.ManifestPath)
		}
	}
	var extra map[string][]string
	if s.cfg.ExtraHashes != nil {
		extra = s.cfg.ExtraHashes()
	}
	if s.cfg.SplitsDir != "" {
		p.registry, p.registryErr = ReadRegistry(s.cfg.SplitsDir, extra)
	} else {
		p.registry = &Registry{HeldOut: []ProtectedSpan{}, Tuned: []ProtectedSpan{}, Digest: digestBytes(nil)}
	}
	h := sha256.New()
	if p.manifest != nil {
		fmt.Fprintf(h, "manifest %s\n", p.manifest.Digest)
	} else {
		fmt.Fprintf(h, "manifest none\n")
	}
	if p.registry != nil {
		fmt.Fprintf(h, "registry %s\n", p.registry.Digest)
	}
	p.digest = "sha256:" + hex.EncodeToString(h.Sum(nil))
	return p
}

// Check decides without recording: what a page shows to say which actions
// are available. It never grants anything; Authorize decides again.
func (s *Service) Check(req Request) Decision {
	return s.decide(req, s.readPolicy())
}

// Authorize decides and records the decision, allowed or refused, before
// the caller loads a sample. It returns ErrNotEligible (wrapped with every
// blocking reason) for a refusal, and refuses when the record cannot be
// written: an unrecorded use is not a permitted one.
func (s *Service) Authorize(req Request) (Decision, error) {
	d := s.decide(req, s.readPolicy())
	if !d.Gated {
		return d, nil
	}
	if s.cfg.Ledger != nil {
		if err := s.cfg.Ledger.Record(usesOf(req, d, s.cfg.Now())); err != nil {
			d.Allowed = false
			d.Reasons = append(d.Reasons, Reason{Code: CodeLedgerUnavailable, Blocking: true,
				Message: fmt.Sprintf("the use could not be recorded: %v", err)})
		} else {
			d.Recorded = true
		}
	}
	if !d.Allowed {
		return d, fmt.Errorf("%w for %s: %s", ErrNotEligible, req.Purpose, d.Summary())
	}
	return d, nil
}

func (s *Service) decide(req Request, p policy) Decision {
	d := Decision{
		RequestID: uuid.NewString(), Purpose: req.Purpose, Gated: req.Purpose != PurposeProduction,
		Measures: req.Measures, HeldOutScore: req.HeldOutScore, SplitDigest: req.SplitDigest,
		PolicyDigest: p.digest, Reasons: []Reason{}, Inputs: []InputDecision{},
	}
	if len(d.Measures) == 0 {
		d.Measures = append([]Measure(nil), Measures...)
	}
	block := func(code, msg string) {
		d.Reasons = append(d.Reasons, Reason{Code: code, Message: msg, Blocking: true})
	}
	if !req.Purpose.valid() {
		block(CodeBadRequest, fmt.Sprintf("purpose %q is not known", req.Purpose))
		d.Gated = true
		return d
	}
	if !d.Gated {
		d.Allowed = true
		return d
	}
	if req.HeldOutScore && (req.Purpose != PurposeDevelopmentEvaluation || req.SplitDigest == "") {
		block(CodeBadRequest, "a held-out score is a development evaluation under a named frozen split")
	}
	if len(req.Inputs) == 0 {
		block(CodeBadRequest, "no inputs: nothing to decide")
	}
	if s.cfg.Ledger == nil {
		block(CodeLedgerUnavailable, "no use ledger: a development use that cannot be recorded is refused")
	}
	// Annotation review is how references are made: it exposes nothing to
	// tuning, so what is unassigned or held out does not refuse it. Only
	// what may be blind qualification data does.
	review := req.Purpose == PurposeAnnotationReview
	soft := func(code, msg string) {
		if review {
			d.Reasons = append(d.Reasons, Reason{Code: code, Message: msg})
		} else {
			block(code, msg)
		}
	}
	switch {
	case !p.configured:
		soft(CodeNotConfigured, "no cohort manifest is configured, so every capture is unassigned: assign sites and captures before development use (assignments are TBD)")
	case p.manifestErr != nil:
		block(CodeManifestInvalid, p.manifestErr.Error())
	}
	if s.cfg.SplitsDir == "" {
		soft(CodeRegistryIncomplete, "no annotation root is configured, so the frozen splits that hold data out cannot be read")
	} else if p.registryErr != nil {
		soft(CodeRegistryIncomplete, "the frozen splits could not be read: "+p.registryErr.Error())
	} else if len(p.registry.Unreadable) > 0 {
		soft(CodeRegistryIncomplete, "frozen splits that could not be read may hold data out: "+strings.Join(p.registry.Unreadable, "; "))
	}
	var exposures map[string][]Use
	if req.HeldOutScore && s.cfg.Ledger != nil {
		exposures = map[string][]Use{}
	}
	for _, in := range req.Inputs {
		d.Inputs = append(d.Inputs, s.decideInput(req, p, in, exposures))
	}
	d.Allowed = true
	for _, r := range d.Reasons {
		d.Allowed = d.Allowed && !r.Blocking
	}
	for _, in := range d.Inputs {
		d.Allowed = d.Allowed && in.Allowed
	}
	return d
}

func (s *Service) decideInput(req Request, p policy, in Input, exposures map[string][]Use) InputDecision {
	out := InputDecision{Identity: in.Identity, StartNs: in.StartNs, EndNs: in.EndNs, Reasons: []Reason{}}
	block := func(code, msg string) {
		out.Reasons = append(out.Reasons, Reason{Code: code, Message: msg, Blocking: true})
	}
	note := func(code, msg string) { out.Reasons = append(out.Reasons, Reason{Code: code, Message: msg}) }
	review := req.Purpose == PurposeAnnotationReview
	// A review of data whose identity is unknown is refused only when the
	// manifest holds qualification captures it could be one of.
	qualificationListed := p.manifest != nil && p.manifest.CohortCounts()[CohortQualification].Captures > 0
	if in.Err != nil {
		msg := fmt.Sprintf("its identity could not be established: %v", in.Err)
		if review && !qualificationListed {
			note(CodeIdentityUnknown, msg)
		} else {
			block(CodeIdentityUnknown, msg)
		}
		out.Assignment = Assignment{Cohort: CohortUnassigned}
		return s.finish(out)
	}
	if in.Identity.Kind == KindCapture && in.Identity.SHA256 == "" {
		block(CodeIdentityUnknown, "no hash of its bytes: a capture is identified by its content")
	}

	// Cohort first: it is the strictest restriction, and nothing below may
	// relax it.
	out.Assignment = Assignment{Cohort: CohortUnassigned}
	if p.manifest != nil {
		if in.Identity.Kind == KindLive {
			out.Assignment = p.manifest.Live(in.Identity.SensorID)
		} else {
			out.Assignment = p.manifest.Capture(in.Identity.SHA256, in.Identity.Basename)
		}
	}
	switch out.Assignment.Cohort {
	case CohortUnassigned:
		switch {
		case out.Assignment.Conflict != "":
			block(CodeConflict, out.Assignment.Conflict)
		case review && !qualificationListed:
			note(CodeUnassigned, "unassigned: its site and cohort are TBD")
		case p.manifest != nil:
			block(CodeUnassigned, "not in the cohort manifest: its site and cohort are TBD")
		default:
			block(CodeUnassigned, "unassigned: no cohort manifest is configured")
		}
	case CohortValidation:
		if req.Purpose != PurposeAnnotationReview {
			block(CodeValidation, fmt.Sprintf("site %s is in the validation cohort: it is not used for %s", out.Assignment.SiteID, req.Purpose))
		}
	case CohortQualification:
		switch req.Purpose {
		case PurposeQualificationEvaluation, PurposeAnnotationReview:
			s.checkQualificationAccess(req, p, block)
		default:
			block(CodeQualification, fmt.Sprintf("site %s is in the blind qualification cohort: no development use", out.Assignment.SiteID))
		}
	case CohortDevelopment:
		if req.Purpose == PurposeQualificationEvaluation {
			block(CodeQualification, "qualification evaluation reads only the qualification cohort")
		}
	}

	if in.Identity.Kind != KindCapture || p.registry == nil {
		return s.finish(out)
	}
	// Frozen splits: what any split holds out, whichever lineage, whether or
	// not the request named one.
	for _, span := range p.registry.HeldOut {
		switch span.match(in.Identity) {
		case matchConflict:
			block(CodeHashMismatch, fmt.Sprintf("its name matches %s, held out by split %s, but its bytes are none of that capture's verified hashes", span.Basename, span.SplitName))
		case matchSame:
			if !span.overlaps(in.StartNs, in.EndNs) {
				continue
			}
			if len(span.SHA256s) == 0 {
				note(CodeWeakIdentity, fmt.Sprintf("matched split %s's held-out capture by name only: bind its label set to the capture so a renamed copy is recognised", span.SplitName))
			}
			if req.HeldOutScore || req.Purpose == PurposeAnnotationReview || req.Purpose == PurposeQualificationEvaluation {
				continue
			}
			what := "pack " + short(span.PackDigest)
			if span.CaseID != "" {
				what = "case " + span.CaseID
			}
			block(CodeHeldOut, fmt.Sprintf("held out by frozen split %s (%s, %s): %s may not read it", span.SplitName, short(span.SplitDigest), what, req.Purpose))
		}
	}
	if !req.HeldOutScore {
		return s.finish(out)
	}
	// A held-out claim: refused for anything tuned on in any lineage, and
	// for any measure the capture was read for in development.
	for _, span := range p.registry.Tuned {
		if span.match(in.Identity) == matchSame && span.overlaps(in.StartNs, in.EndNs) {
			block(CodeTunedInLineage, fmt.Sprintf("tuned on in split %s's lineage (%s): it cannot be scored as held out", span.SplitName, short(span.SplitDigest)))
		}
	}
	if exposures != nil && in.Identity.SHA256 != "" {
		uses, ok := exposures[in.Identity.SHA256]
		if !ok {
			var err error
			uses, err = s.cfg.Ledger.Exposures(in.Identity.SHA256)
			if err != nil {
				block(CodeLedgerUnavailable, fmt.Sprintf("its use history could not be read: %v", err))
				return s.finish(out)
			}
			exposures[in.Identity.SHA256] = uses
		}
		read := map[Measure][]string{}
		for _, u := range uses {
			if !u.overlaps(in.StartNs, in.EndNs) {
				continue
			}
			read[u.Measure] = append(read[u.Measure], u.Purpose+" "+u.RecordedUTC())
		}
		for _, m := range req.Measures {
			if r := read[m]; len(r) > 0 {
				block(CodeExposed, fmt.Sprintf("already read for development for %s (%d uses, first %s): it is tuning for %s now", m, len(r), r[0], m))
			}
		}
	}
	return s.finish(out)
}

func (s *Service) checkQualificationAccess(req Request, p policy, block func(code, msg string)) {
	if s.cfg.Access == nil || !s.cfg.Access.QualificationStorageEnforced() {
		block(CodeAccessAbsent, "qualification data needs storage-level access controls, and this server provides none: the action is refused, not hidden")
		return
	}
	if p.manifest == nil || len(p.manifest.Access.Custodians) == 0 {
		block(CodeNotCustodian, "qualification custodians are TBD")
		return
	}
	for _, c := range p.manifest.Access.Custodians {
		if c == req.Actor {
			return
		}
	}
	block(CodeNotCustodian, fmt.Sprintf("%q is not a qualification custodian", req.Actor))
}

func (s *Service) finish(out InputDecision) InputDecision {
	out.Allowed = true
	for _, r := range out.Reasons {
		out.Allowed = out.Allowed && !r.Blocking
	}
	return out
}

func short(digest string) string {
	d := strings.TrimPrefix(digest, "sha256:")
	if len(d) > 8 {
		return d[:8]
	}
	return d
}

// Status is what the service can say about its own protection: aggregate
// metadata only, never a protected sample, label or result.
type Status struct {
	Manifest   ManifestStatus `json:"manifest"`
	Registry   RegistryStatus `json:"registry"`
	Access     AccessStatus   `json:"access"`
	Ledger     bool           `json:"ledger_available"`
	Complete   bool           `json:"protection_complete"`
	Gaps       []string       `json:"gaps"`
	Purposes   []PurposeState `json:"purposes"`
	PolicyHash string         `json:"policy_digest"`
}

// ManifestStatus is the cohort manifest's state.
type ManifestStatus struct {
	Configured     bool                   `json:"configured"`
	State          string                 `json:"state"` // absent, valid, invalid
	Error          string                 `json:"error,omitempty"`
	Digest         string                 `json:"digest,omitempty"`
	Revision       int                    `json:"revision,omitempty"`
	FrozenUTC      string                 `json:"frozen_utc,omitempty"`
	ProtocolDigest string                 `json:"protocol_digest,omitempty"`
	Cohorts        map[Cohort]CohortCount `json:"cohorts"`
}

// RegistryStatus summarises the frozen-split registry.
type RegistryStatus struct {
	Splits       int      `json:"splits"`
	HeldOutSpans int      `json:"held_out_spans"`
	TunedSpans   int      `json:"tuned_spans"`
	WeakIdentity int      `json:"weak_identity"`
	Unreadable   []string `json:"unreadable"`
	Digest       string   `json:"digest"`
	Error        string   `json:"error,omitempty"`
}

// AccessStatus says what protects qualification data.
type AccessStatus struct {
	QualificationStorageEnforced bool `json:"qualification_storage_enforced"`
	CustodiansTBD                bool `json:"custodians_tbd"`
}

// PurposeState says whether a purpose can be granted at all right now, for
// any input, and why not.
type PurposeState struct {
	Purpose   Purpose `json:"purpose"`
	Available bool    `json:"available"`
	Reason    string  `json:"reason,omitempty"`
}

// Status reports the service's protection.
func (s *Service) Status() Status {
	p := s.readPolicy()
	st := Status{Ledger: s.cfg.Ledger != nil, Gaps: []string{}, PolicyHash: p.digest,
		Manifest: ManifestStatus{Configured: p.configured, State: "absent", Cohorts: map[Cohort]CohortCount{}}}
	switch {
	case p.manifest != nil:
		st.Manifest.State, st.Manifest.Digest, st.Manifest.Revision = "valid", p.manifest.Digest, p.manifest.Revision
		st.Manifest.FrozenUTC, st.Manifest.ProtocolDigest = p.manifest.FrozenUTC, p.manifest.ProtocolDigest
		st.Manifest.Cohorts = p.manifest.CohortCounts()
	case p.manifestErr != nil:
		st.Manifest.State, st.Manifest.Error = "invalid", p.manifestErr.Error()
	}
	if p.registryErr != nil {
		st.Registry.Error = p.registryErr.Error()
	} else {
		st.Registry = RegistryStatus{Splits: p.registry.Splits, HeldOutSpans: len(p.registry.HeldOut), TunedSpans: len(p.registry.Tuned),
			WeakIdentity: p.registry.WeakIdentity, Unreadable: append([]string{}, p.registry.Unreadable...), Digest: p.registry.Digest}
	}
	st.Access.QualificationStorageEnforced = s.cfg.Access != nil && s.cfg.Access.QualificationStorageEnforced()
	st.Access.CustodiansTBD = p.manifest == nil || len(p.manifest.Access.Custodians) == 0

	devBlocked := ""
	switch {
	case !st.Ledger:
		devBlocked = "no use ledger is available"
	case st.Manifest.State == "absent":
		devBlocked = "no cohort manifest: every capture is unassigned (TBD)"
	case st.Manifest.State == "invalid":
		devBlocked = "the cohort manifest is invalid"
	case s.cfg.SplitsDir == "":
		devBlocked = "no annotation root is configured, so frozen splits cannot be read"
	case st.Registry.Error != "" || len(st.Registry.Unreadable) > 0:
		devBlocked = "some frozen splits could not be read"
	}
	if devBlocked != "" {
		st.Gaps = append(st.Gaps, "Development use is refused: "+devBlocked+".")
	}
	if p.manifest != nil && p.manifest.ProtocolDigest == "" {
		st.Gaps = append(st.Gaps, "The qualification protocol is TBD: the manifest names no protocol digest.")
	}
	if !st.Access.QualificationStorageEnforced {
		st.Gaps = append(st.Gaps, "No storage-level access controls protect qualification data: qualification review and evaluation are refused.")
	}
	if st.Access.CustodiansTBD {
		st.Gaps = append(st.Gaps, "Qualification custodians are TBD.")
	}
	if st.Registry.WeakIdentity > 0 {
		st.Gaps = append(st.Gaps, fmt.Sprintf("%d held-out stretches are known by capture name only: a renamed copy would not be recognised until their label sets are bound to the capture.", st.Registry.WeakIdentity))
	}
	st.Gaps = append(st.Gaps, "A capture cut, trimmed or re-encoded from a protected one has new bytes: it is recognised only when the manifest or a label-set binding lists its hash.")
	for _, purpose := range Purposes {
		ps := PurposeState{Purpose: purpose, Available: true}
		switch {
		case devBlocked != "":
			ps.Available, ps.Reason = false, devBlocked
		case purpose == PurposeQualificationEvaluation && !st.Access.QualificationStorageEnforced:
			ps.Available, ps.Reason = false, "no storage-level access controls for qualification data"
		}
		st.Purposes = append(st.Purposes, ps)
	}
	st.Complete = devBlocked == "" && st.Access.QualificationStorageEnforced && !st.Access.CustodiansTBD &&
		st.Registry.WeakIdentity == 0 && p.manifest != nil && p.manifest.ProtocolDigest != ""
	return st
}
