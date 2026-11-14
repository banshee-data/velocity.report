package fieldrun

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

const EvidenceSchema = "following_evidence_v1"

// EvidenceContext is an explicit source-to-site binding supplied by the caller,
// never inferred from overlapping capture timestamps. BindingID identifies the
// reviewed site/source record; it is provenance, not a physical validation claim.
type EvidenceContext struct {
	SiteNamespace  string                    `json:"site_namespace"`
	SiteID         string                    `json:"site_id"`
	SourceID       string                    `json:"source_id"`
	BindingID      string                    `json:"binding_id"`
	BuildGitSHA    string                    `json:"analysis_build_git_sha"`
	PolicyID       string                    `json:"policy_id"`
	SourceManifest *l4bobserve.CaptureSource `json:"source_manifest"`
	CaptureFiles   []string                  `json:"capture_files,omitempty"`
}

// EvidenceRow represents one follower sample and the half-open interval to its
// next sample. Terminal samples have zero duration. Record gaps never contribute
// observed exposure. Reasons may overlap; Accounting buckets do not.
type EvidenceRow struct {
	FollowerTrackID  string                      `json:"follower_track_id"`
	CaptureUnixNanos int64                       `json:"capture_unix_nanos"`
	IntervalNanos    int64                       `json:"interval_nanos"`
	Bucket           string                      `json:"bucket"`
	Decision         l8behaviour.PairingDecision `json:"decision"`
	Point            *l8behaviour.FollowingPoint `json:"point,omitempty"`
	FollowerSpeedMPS *float64                    `json:"follower_speed_mps"`
	LeaderSpeedMPS   *float64                    `json:"leader_speed_mps"`
	DRAC             EvidenceDRAC                `json:"experimental_drac"`
}

// EvidenceDRAC separates evidence-admission failures from the calculation's
// physical applicability. A suppressed evidence row has no calculation value.
type EvidenceDRAC struct {
	Status         string                        `json:"status"`
	EvidenceReason l8behaviour.SuppressionReason `json:"evidence_reason,omitempty"`
	Calculation    *l8behaviour.DRACResult       `json:"calculation"`
}

// EvidenceDocument retains full trajectories and fitted paths, including
// followers with no leader, as well as the interactions. It is review evidence;
// its point values carry no qualification or missing-covariance assumption.
type EvidenceDocument struct {
	ReadQuality    EvidenceReadQuality                 `json:"read_quality"`
	Schema         string                              `json:"schema"`
	Context        EvidenceContext                     `json:"context"`
	Version        l8behaviour.InteractionVersion      `json:"analysis_version"`
	Params         l8behaviour.FollowingAnalysisParams `json:"parameters"`
	Trajectories   []l8behaviour.Trajectory            `json:"trajectories"`
	Analysis       l8behaviour.FollowingAnalysis       `json:"analysis"`
	Interactions   []l8behaviour.FollowingInteraction  `json:"interactions"`
	Rows           []EvidenceRow                       `json:"rows"`
	Accounting     map[string]int64                    `json:"follower_interval_nanos"`
	DRACAccounting map[string]int64                    `json:"experimental_drac_interval_nanos"`
}

// EvidenceReadQuality records unreadable source rows separately from sampled
// follower-time. Their duration is unknown and never becomes observed exposure.
type EvidenceReadQuality struct {
	Selected         sqlite.EstimateVersion    `json:"selected_estimates"`
	Boxes            bool                      `json:"boxes"`
	SolidBodies      bool                      `json:"solid_bodies"`
	BoxlessEstimates int                       `json:"boxless_estimates"`
	UnfiledEstimates int                       `json:"unfiled_estimates"`
	SolidBodyGaps    l8behaviour.SolidBodyGaps `json:"solid_body_gaps"`
}

// Evidence regenerates the analysis so mutable or stale cached summaries cannot
// contradict its timelines. Accounting is follower-time, not unique encounter
// time or a population denominator; the same scene time can occur in two rows.
func Evidence(r Result, c EvidenceContext) (EvidenceDocument, error) {
	if err := c.Validate(); err != nil {
		return EvidenceDocument{}, err
	}
	if c.SourceID != r.Estimates.SourceID {
		return EvidenceDocument{}, fmt.Errorf("context source differs from selected estimates")
	}
	if c.SourceManifest != nil {
		id, err := l4bobserve.SourceID(*c.SourceManifest)
		if err != nil {
			return EvidenceDocument{}, err
		}
		if id != c.SourceID {
			return EvidenceDocument{}, fmt.Errorf("source manifest differs from context source")
		}
	}
	if err := r.Version.Validate(); err != nil {
		return EvidenceDocument{}, err
	}
	wantModel := r.Estimates.ObservationModelID
	if r.Boxes {
		wantModel = l8behaviour.BoxObservationModelID(wantModel)
	}
	if r.Estimates.EstimatorID != r.Version.EstimatorID || wantModel != r.Version.ObsModelID || r.Estimates.ParamHash != r.Version.ParamHash || r.Estimates.Stage != r.Version.EstimateStage.String() {
		return EvidenceDocument{}, fmt.Errorf("selected source version differs from analysis version")
	}
	if r.BoxlessEstimates < 0 || r.UnfiledEstimates < 0 {
		return EvidenceDocument{}, fmt.Errorf("negative missing-source counts")
	}
	tracks := append([]l8behaviour.Trajectory(nil), r.Trajectories...)
	if len(tracks) == 0 {
		return EvidenceDocument{}, fmt.Errorf("no readable trajectories for evidence export")
	}
	sort.Slice(tracks, func(i, j int) bool { return tracks[i].Passage.TrackID < tracks[j].Passage.TrackID })
	byID := map[string]l8behaviour.Trajectory{}
	for _, t := range tracks {
		if t.Passage.SiteID != "" && t.Passage.SiteID != c.SiteID {
			return EvidenceDocument{}, fmt.Errorf("track site differs from explicit binding")
		}
		if _, ok := byID[t.Passage.TrackID]; ok {
			return EvidenceDocument{}, fmt.Errorf("duplicate track %s", t.Passage.TrackID)
		}
		if err := t.Validate(); err != nil {
			return EvidenceDocument{}, err
		}
		if t.Estimate.EstimatorID != r.Version.EstimatorID || t.Estimate.ObsModelID != r.Version.ObsModelID || t.Estimate.ParamHash != r.Version.ParamHash {
			return EvidenceDocument{}, fmt.Errorf("trajectory version differs from selected analysis")
		}
		for _, sample := range t.Samples {
			if sample.Stage != r.Version.EstimateStage {
				return EvidenceDocument{}, fmt.Errorf("trajectory stage differs from selected analysis")
			}
		}
		byID[t.Passage.TrackID] = t
	}
	a, err := l8behaviour.AnalyseFollowing(tracks, r.Params)
	if err != nil {
		return EvidenceDocument{}, err
	}
	if r.Version.MethodID != l8behaviour.FollowingEncounterMethodID+"/"+a.ParamsHash {
		return EvidenceDocument{}, fmt.Errorf("analysis method differs from retained parameters")
	}
	interactions, err := l8behaviour.FollowingInteractions(c.SourceID, a)
	if err != nil {
		return EvidenceDocument{}, err
	}
	d := EvidenceDocument{Schema: EvidenceSchema, Context: c, Version: r.Version, Params: r.Params, ReadQuality: EvidenceReadQuality{Selected: r.Estimates, Boxes: r.Boxes, SolidBodies: r.SolidBodies, BoxlessEstimates: r.BoxlessEstimates, UnfiledEstimates: r.UnfiledEstimates, SolidBodyGaps: r.SolidBodyGaps}, Trajectories: tracks, Analysis: a, Interactions: interactions, Rows: []EvidenceRow{}, Accounting: map[string]int64{}, DRACAccounting: map[string]int64{}}
	points := map[string]map[int64]*l8behaviour.FollowingPoint{}
	for _, e := range a.Encounters {
		for _, in := range e.Instants {
			if in.Point != nil {
				key := e.FollowerTrackID + "\x00" + e.LeaderTrackID
				if points[key] == nil {
					points[key] = map[int64]*l8behaviour.FollowingPoint{}
				}
				points[key][in.CaptureUnixNanos] = in.Point
			}
		}
	}
	for _, tl := range a.Timelines {
		t := byID[tl.FollowerTrackID]
		for i, dec := range tl.Decisions {
			row := EvidenceRow{FollowerTrackID: tl.FollowerTrackID, CaptureUnixNanos: dec.CaptureUnixNanos, Decision: dec, Bucket: "no_leader"}
			if i+1 < len(t.Samples) {
				row.IntervalNanos = t.Samples[i+1].CaptureUnixNanos - t.Samples[i].CaptureUnixNanos
			}
			in := l8behaviour.DRACInput{}
			evidenceReason := l8behaviour.ReasonUnspecified
			if dec.Reason != l8behaviour.ReasonUnspecified {
				row.Bucket = "pairing_excluded"
			}
			if dec.LeaderTrackID != "" {
				row.Bucket = "following_excluded"
				row.Point = points[tl.FollowerTrackID+"\x00"+dec.LeaderTrackID][dec.CaptureUnixNanos]
				if p := row.Point; p != nil {
					candidate, found := dec.Candidate(dec.LeaderTrackID)
					in.SamePath = found && candidate.Established && candidate.Synchronised && dec.Reason == l8behaviour.ReasonUnspecified
					in.Observed = p.Leader != nil && p.Follower != nil && p.Leader.Trailing.Support == l8behaviour.SupportObserved && p.Follower.Leading.Support == l8behaviour.SupportObserved
					if p.Leader != nil {
						v := p.Leader.AlongSpeedMps
						row.LeaderSpeedMPS = &v
					}
					if p.Follower != nil {
						v := p.Follower.AlongSpeedMps
						row.FollowerSpeedMPS = &v
					}
					if p.Gap != nil {
						v := p.Gap.ValueM
						in.GapM = &v
					}
					for _, reason := range p.Reasons {
						if reason != l8behaviour.ReasonEstimateNotFinal && reason != l8behaviour.ReasonBelowSpeedFloor && reason != l8behaviour.ReasonNonPositiveGap {
							if evidenceReason == l8behaviour.ReasonUnspecified {
								evidenceReason = reason
							}
						}
					}
					if p.SupportedOpportunity {
						row.Bucket = "following_eligible"
					}
				}
			}
			in.FollowerSpeedMPS = row.FollowerSpeedMPS
			in.LeaderSpeedMPS = row.LeaderSpeedMPS
			if row.IntervalNanos > r.Params.Exposure.MaxIntervalNanos {
				row.Bucket = "record_gap"
				in.Observed = false
			}
			switch {
			case row.Bucket == "record_gap":
				row.DRAC = EvidenceDRAC{Status: "record_gap"}
			case dec.LeaderTrackID == "":
				row.DRAC = EvidenceDRAC{Status: "no_candidate", EvidenceReason: dec.Reason}
			case evidenceReason != l8behaviour.ReasonUnspecified:
				row.DRAC = EvidenceDRAC{Status: "ineligible_evidence", EvidenceReason: evidenceReason}
			default:
				calculation := l8behaviour.EvaluateDRAC(in)
				row.DRAC = EvidenceDRAC{Status: string(calculation.Status), Calculation: &calculation}
			}
			d.Accounting[row.Bucket] += row.IntervalNanos
			d.DRACAccounting[row.DRAC.Status] += row.IntervalNanos
			d.Rows = append(d.Rows, row)
		}
	}
	return d, nil
}

// Validate checks that the declared binding has every required provenance axis.
func (c EvidenceContext) Validate() error {
	if c.SiteNamespace == "" || c.SiteID == "" || c.SourceID == "" || c.BindingID == "" || c.BuildGitSHA == "" || c.PolicyID == "" {
		return fmt.Errorf("evidence requires explicit site namespace, site, source, binding, build and policy")
	}
	return nil
}

// WriteEvidence writes deterministic JSON under a context/version identity.
// A repeat is a no-op; different bytes under the same identity are refused.
// Linking a completed temporary file prevents partial files or writer races.
func WriteEvidence(dir string, d EvidenceDocument) (string, error) {
	return writeEvidence(dir, d, localEvidenceFiles())
}

// evidenceFiles isolates filesystem operations so durability failures can be
// tested without relying on a full disk or corrupting a real output directory.
type evidenceFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
	Name() string
}
type evidenceFiles struct {
	mkdir  func(string, os.FileMode) error
	temp   func(string, string) (evidenceFile, error)
	remove func(string) error
	link   func(string, string) error
	read   func(string) ([]byte, error)
}

func localEvidenceFiles() evidenceFiles {
	return evidenceFiles{mkdir: os.MkdirAll, temp: func(dir, pattern string) (evidenceFile, error) { return os.CreateTemp(dir, pattern) }, remove: os.Remove, link: os.Link, read: os.ReadFile}
}
func writeEvidence(dir string, d EvidenceDocument, files evidenceFiles) (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	b = append(b, '\n')
	key, _ := json.Marshal(struct {
		Schema        string
		SiteNamespace string
		SiteID        string
		SourceID      string
		BuildGitSHA   string
		PolicyID      string
		Version       l8behaviour.InteractionVersion
	}{d.Schema, d.Context.SiteNamespace, d.Context.SiteID, d.Context.SourceID, d.Context.BuildGitSHA, d.Context.PolicyID, d.Version})
	sum := sha256.Sum256(key)
	path := filepath.Join(dir, hex.EncodeToString(sum[:])+".json")
	if err = files.mkdir(dir, 0755); err != nil {
		return "", err
	}
	tmp, err := files.temp(dir, ".following-")
	if err != nil {
		return "", err
	}
	defer files.remove(tmp.Name())
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	if err = files.link(tmp.Name(), path); err != nil {
		if !os.IsExist(err) {
			return "", err
		}
		old, e := files.read(path)
		if e != nil {
			return "", e
		}
		if !bytes.Equal(old, b) {
			return "", fmt.Errorf("conflicting evidence at %s", path)
		}
	}
	return path, nil
}

// Validate re-derives every row and accounting bucket from the retained inputs.
// This prevents a caller from publishing a summary that disagrees with evidence.
func (d EvidenceDocument) Validate() error {
	r := Result{Estimates: d.ReadQuality.Selected, Boxes: d.ReadQuality.Boxes, SolidBodies: d.ReadQuality.SolidBodies, BoxlessEstimates: d.ReadQuality.BoxlessEstimates, UnfiledEstimates: d.ReadQuality.UnfiledEstimates, SolidBodyGaps: d.ReadQuality.SolidBodyGaps, Version: d.Version, Params: d.Params, Trajectories: d.Trajectories}
	want, err := Evidence(r, d.Context)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(d, want) {
		return fmt.Errorf("evidence document differs from its derived content")
	}
	return nil
}
