// Package fieldrun is the provisional headway run of Section 10.4, step 2,
// of docs/plans/lidar-behaviour-analytics-plan.md: persisted estimator
// output taken through local path pairing, interaction persistence and the
// production report renderer, with every field result labelled provisional.
//
// One run, in order:
//
//  1. Select exactly one version of lidar_track_estimates for a source at a
//     stage (online, fixed_lag or final), refusing to guess between several.
//  2. Build trajectories from its rows (l8behaviour.TrajectoriesFromEstimates),
//     which claim no heading, extent or class that the rows do not carry.
//  3. Analyse them with l8behaviour.AnalyseFollowing under Params.
//  4. Persist every encounter's event, instants and windows through
//     sqlite.InteractionStore. Rows are write-once per version: a re-run of
//     the same version writes nothing, and different content under a stored
//     id is refused. The run then requires the store to hold, at this
//     version, exactly the events it produced, so a method that changed
//     without a version change fails here rather than mixing results.
//  5. Build the report (Report) from the same trajectories and analysis,
//     with status provisional.
//
// The report is rebuilt from persisted estimates on every run, never read
// back from the persisted interactions: the estimates are the canonical
// source (Section 10.3, "behaviour output is derived data and must be
// reproducible from the persisted final estimates"), and the interaction
// rows carry neither the fitted path nor each follower's timeline, which the
// report shows. Step 4's write-once comparison is what ties the two: the
// rows stored are byte for byte the ones this analysis maps to.
//
// The package reads and writes one evidence database; it runs no estimator.
package fieldrun

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/headway"
)

// Params are the bounds a field run analyses with. They are the analytic
// scenarios' bounds, which are uncalibrated fixture values (Phase 6B lists
// their calibration as remaining); the report prints them with their hash,
// and a change to them is a new interaction version. Their record-gap bound
// is 150 ms, a little above a 10 Hz frame period, and Run refuses a capture
// whose frame period would let it hold a missing row as observed time.
func Params() l8behaviour.FollowingAnalysisParams { return l8behaviour.EncounterScenarioParams() }

// Spec selects the estimates a run reads.
type Spec struct {
	// SourceID is the lidar_track_estimates source: the capture identity.
	SourceID string
	// Stage is online, fixed_lag or final.
	Stage l8behaviour.EstimateStage
	// EstimatorID, ObsModelID and ParamHash narrow the selection when the
	// source holds several versions at the stage, as a fixed_lag_rts replay
	// does (one fixed_lag version per horizon). Empty matches any.
	EstimatorID string
	ObsModelID  string
	ParamHash   string
}

// Result is one run: what it read, what it derived and what it stored.
type Result struct {
	Estimates    sqlite.EstimateVersion
	Trajectories []l8behaviour.Trajectory
	Params       l8behaviour.FollowingAnalysisParams
	Analysis     l8behaviour.FollowingAnalysis
	// Version is the interaction version every encounter carries.
	Version      l8behaviour.InteractionVersion
	Interactions []l8behaviour.FollowingInteraction
	// AlreadyStored counts the encounters whose rows were present before
	// this run; the rest were written by it.
	AlreadyStored int
}

// Run performs one provisional run against an evidence database.
func Run(db sqlite.DBClient, spec Spec) (Result, error) {
	if spec.SourceID == "" {
		return Result{}, errors.New("a field run needs a source id")
	}
	if !spec.Stage.Valid() {
		return Result{}, fmt.Errorf("a field run needs an estimate stage (online, fixed_lag or final), got %s", spec.Stage)
	}
	estimates := sqlite.NewStateEstimateStore(db)
	version, err := selectVersion(estimates, spec)
	if err != nil {
		return Result{}, err
	}
	rows, err := estimates.ListVersionEstimates(sqlite.EstimateVersionKey{
		SourceID: version.SourceID, EstimatorID: version.EstimatorID, ObservationModelID: version.ObservationModelID,
		ParamHash: version.ParamHash, Stage: version.Stage,
	})
	if err != nil {
		return Result{}, err
	}
	persisted := make([]l8behaviour.PersistedEstimate, len(rows))
	for i, r := range rows {
		persisted[i] = l8behaviour.PersistedEstimate{
			TrackID: r.TrackID, SensorID: r.SensorID, FrameUnixNanos: r.FrameUnixNanos,
			EstimatorID: r.EstimatorID, ObsModelID: r.ObservationModelID, ParamHash: r.ParamHash, Stage: r.Stage,
			MeasurementSource: r.MeasurementSource, X: r.X, Y: r.Y, VX: r.VX, VY: r.VY, Covariance: r.Covariance,
		}
	}
	trajectories, err := l8behaviour.TrajectoriesFromEstimates(persisted, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		return Result{}, fmt.Errorf("estimates of %s: %w", version.SourceID, err)
	}
	params := Params()
	if err := checkFramePeriod(trajectories, params.Exposure.MaxIntervalNanos); err != nil {
		return Result{}, err
	}
	analysis, err := l8behaviour.AnalyseFollowing(trajectories, params)
	if err != nil {
		return Result{}, err
	}

	res := Result{
		Estimates: version, Trajectories: trajectories, Params: params, Analysis: analysis,
		Version: l8behaviour.InteractionVersion{
			EstimateStage: spec.Stage, EstimatorID: version.EstimatorID, ObsModelID: version.ObservationModelID,
			MethodID: l8behaviour.FollowingEncounterMethodID + "/" + analysis.ParamsHash, ParamHash: version.ParamHash,
		},
	}
	if res.Interactions, err = l8behaviour.FollowingInteractions(version.SourceID, analysis); err != nil {
		return Result{}, err
	}
	for _, fi := range res.Interactions {
		if v := fi.Event.Version.InteractionVersion(); v != res.Version {
			return Result{}, fmt.Errorf("encounter %s has version %+v, not the run's %+v", fi.Event.EventID, v, res.Version)
		}
	}
	if res.AlreadyStored, err = persist(db, version.SourceID, res.Version, res.Interactions); err != nil {
		return Result{}, err
	}
	return res, nil
}

// selectVersion finds the one estimate version the spec names.
func selectVersion(store *sqlite.StateEstimateStore, spec Spec) (sqlite.EstimateVersion, error) {
	versions, err := store.ListEstimateVersions()
	if err != nil {
		return sqlite.EstimateVersion{}, err
	}
	var atSource, matched []sqlite.EstimateVersion
	for _, v := range versions {
		if v.SourceID != spec.SourceID {
			continue
		}
		atSource = append(atSource, v)
		if v.Stage == spec.Stage.String() &&
			(spec.EstimatorID == "" || v.EstimatorID == spec.EstimatorID) &&
			(spec.ObsModelID == "" || v.ObservationModelID == spec.ObsModelID) &&
			(spec.ParamHash == "" || v.ParamHash == spec.ParamHash) {
			matched = append(matched, v)
		}
	}
	switch {
	case len(matched) == 1:
		return matched[0], nil
	case len(atSource) == 0:
		return sqlite.EstimateVersion{}, fmt.Errorf("the database holds no estimates for source %s", spec.SourceID)
	case len(matched) == 0:
		return sqlite.EstimateVersion{}, fmt.Errorf("source %s holds no %s estimates matching estimator=%q "+
			"observation_model=%q param_hash=%q; it holds:\n%s",
			spec.SourceID, spec.Stage, spec.EstimatorID, spec.ObsModelID, spec.ParamHash, listVersions(atSource))
	default:
		return sqlite.EstimateVersion{}, fmt.Errorf("source %s holds %d %s estimate versions; name the estimator, "+
			"observation model and parameter hash of one:\n%s", spec.SourceID, len(matched), spec.Stage, listVersions(matched))
	}
}

func listVersions(versions []sqlite.EstimateVersion) string {
	var b strings.Builder
	for _, v := range versions {
		fmt.Fprintf(&b, "  stage=%s estimator=%s observation_model=%s param_hash=%s (%d estimates, %d tracks)\n",
			v.Stage, v.EstimatorID, v.ObservationModelID, v.ParamHash, v.Estimates, v.Tracks)
	}
	return strings.TrimRight(b.String(), "\n")
}

// checkFramePeriod refuses parameters under which a single missing row would
// be held as the previous sample's time rather than counted as a record gap.
// Rows exist only at observed frames, so a held missing row would be coasted
// time counted as observed. The frame period is the median interval between
// a track's consecutive rows.
func checkFramePeriod(trajectories []l8behaviour.Trajectory, maxIntervalNanos int64) error {
	var intervals []int64
	for _, t := range trajectories {
		for i := 1; i < len(t.Samples); i++ {
			intervals = append(intervals, t.Samples[i].CaptureUnixNanos-t.Samples[i-1].CaptureUnixNanos)
		}
	}
	if len(intervals) == 0 {
		return nil
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i] < intervals[j] })
	period := intervals[len(intervals)/2]
	if maxIntervalNanos >= 2*period {
		return fmt.Errorf("the record-gap bound %d ms would hold a missing row as observed time at this capture's "+
			"%d ms frame period; the field parameters assume 10 Hz", maxIntervalNanos/1e6, period/1e6)
	}
	return nil
}

// persist writes the run's interactions and requires the store to hold, at
// the run's version, exactly the events the run produced. It returns how
// many of them were stored before.
func persist(db sqlite.DBClient, sourceID string, v l8behaviour.InteractionVersion,
	interactions []l8behaviour.FollowingInteraction) (int, error) {
	store := sqlite.NewInteractionStore(db)
	before, err := store.ListEvents(sourceID, v)
	if err != nil {
		return 0, err
	}
	produced := make(map[string]bool, len(interactions))
	for _, fi := range interactions {
		produced[fi.Event.EventID] = true
	}
	already := 0
	for _, ev := range before {
		if produced[ev.EventID] {
			already++
		}
	}
	if len(interactions) > 0 {
		if err := store.Insert(interactions...); err != nil {
			return 0, fmt.Errorf("persist interactions: %w", err)
		}
	}
	after, err := store.ListEvents(sourceID, v)
	if err != nil {
		return 0, err
	}
	var foreign []string
	for _, ev := range after {
		if !produced[ev.EventID] {
			foreign = append(foreign, ev.EventID)
		}
	}
	if len(foreign) > 0 || len(after) != len(interactions) {
		return 0, fmt.Errorf("the store holds %d events at version %+v and this run produced %d; %d are not this run's "+
			"(%s). The method changed without a version change: remove the version with "+
			"InteractionStore.DeleteVersion and run again", len(after), v, len(interactions), len(foreign),
			strings.Join(foreign, ", "))
	}
	return already, nil
}

// Capture is the run as the report's one capture.
func (r Result) Capture() headway.CaptureInput {
	e := r.Estimates
	source := strings.Join([]string{"lidar_track_estimates", e.SourceID, e.EstimatorID, e.ObservationModelID,
		e.ParamHash, e.Stage}, "/")
	return headway.CaptureInput{
		ID: "source_" + shortDigest(e.SourceID), Description: r.describe(), Source: source,
		Trajectories: r.Trajectories, Params: r.Params, Analysis: r.Analysis,
	}
}

// shortDigest is the first twelve characters of the source id's last path
// element: enough to tell captures apart in a table.
func shortDigest(sourceID string) string {
	last := sourceID[strings.LastIndex(sourceID, "/")+1:]
	return last[:min(12, len(last))]
}

// describe states what the rows could and could not support, counted from
// the samples rather than asserted. It names no id: the capture's source
// already prints the source, estimator, observation model, parameter hash
// and stage, with break points, where a long hash here would overrun its
// cell.
func (r Result) describe() string {
	var samples, bodyCentre, heading, extents, converging int
	vehicles := 0
	for _, t := range r.Trajectories {
		if t.Passage.MotionClass == l8behaviour.MotionRigidVehicle {
			vehicles++
		}
		for _, s := range t.Samples {
			samples++
			if s.Reference == l8behaviour.ReferenceBodyCentre {
				bodyCentre++
			}
			if s.Heading.Resolved() {
				heading++
			}
			if s.Length.Present() && s.Width.Present() {
				extents++
			}
			if s.Estimation == l8behaviour.EstimationGeometryConverging || s.Estimation == l8behaviour.EstimationEstablished {
				converging++
			}
		}
	}
	return fmt.Sprintf("Persisted %s estimates: %d tracks, %d samples. Samples on a body-centre reference: %d; "+
		"with a pose the estimator stands behind: %d; with a resolved heading: %d; with length and width beliefs: %d. "+
		"Tracks classed as rigid vehicles: %d. The analysis parameters are the analytic scenarios' bounds, not calibrated.",
		r.Estimates.Stage, len(r.Trajectories), samples, bodyCentre, converging, heading, extents, vehicles)
}

// Report builds the provisional report from the run. Field data is never a
// synthetic oracle, and nothing is promoted: headway.Build refuses both.
func Report(r Result) (headway.Report, error) {
	return headway.Build(headway.Input{Status: headway.StatusProvisional, Captures: []headway.CaptureInput{r.Capture()}})
}

// Summary is the run in figures, for the command line and for review.
type Summary struct {
	Tracks, Samples int
	// PathsFitted counts followers whose local path was fitted; PathRefusals
	// counts the rest by their first condition.
	PathsFitted  int
	PathRefusals map[l8behaviour.PathCondition]int
	Encounters   int
	// ValidNanos and AccountedNanos sum the encounters' valid following time
	// and accounted time; Suppressed is the rest by each instant's reason.
	ValidNanos     int64
	AccountedNanos int64
	Suppressed     map[l8behaviour.SuppressionReason]l8behaviour.SuppressionCount
	// EvaluatedInstants counts the encounter instants whose pair was
	// evaluated, and InstantReasons every reason that applied at them, not
	// only the first: what else would have to change for a value to appear.
	EvaluatedInstants int
	InstantReasons    map[l8behaviour.SuppressionReason]int
	Written           int
	AlreadyStored     int
}

// Summary tallies the run.
func (r Result) Summary() Summary {
	s := Summary{
		Tracks: len(r.Trajectories), PathRefusals: map[l8behaviour.PathCondition]int{},
		Encounters: len(r.Analysis.Encounters), Suppressed: map[l8behaviour.SuppressionReason]l8behaviour.SuppressionCount{},
		InstantReasons: map[l8behaviour.SuppressionReason]int{},
		Written:        len(r.Interactions) - r.AlreadyStored, AlreadyStored: r.AlreadyStored,
	}
	for _, t := range r.Trajectories {
		s.Samples += len(t.Samples)
	}
	for _, p := range r.Analysis.Paths {
		if p.Path != nil {
			s.PathsFitted++
		} else if len(p.Conditions) > 0 {
			s.PathRefusals[p.Conditions[0]]++
		}
	}
	for _, e := range r.Analysis.Encounters {
		s.ValidNanos += e.Accounting.ValidNanos
		s.AccountedNanos += e.Accounting.ValidNanos
		for _, t := range e.Accounting.Suppressions {
			c := s.Suppressed[t.Reason]
			c.Instants += t.Instants
			c.Nanos += t.Nanos
			s.Suppressed[t.Reason] = c
			s.AccountedNanos += t.Nanos
		}
		for _, in := range e.Instants {
			if in.Point == nil {
				continue
			}
			s.EvaluatedInstants++
			for _, reason := range in.Point.Reasons {
				s.InstantReasons[reason]++
			}
		}
	}
	return s
}

// Lines renders the summary for a terminal, reasons in precedence order.
func (s Summary) Lines() []string {
	seconds := func(n int64) string { return fmt.Sprintf("%.1f s", float64(n)/1e9) }
	out := []string{
		fmt.Sprintf("Tracks: %d (%d samples)", s.Tracks, s.Samples),
		fmt.Sprintf("Follower paths fitted: %d of %d", s.PathsFitted, s.Tracks),
	}
	for _, c := range l8behaviour.PathConditions() {
		if n := s.PathRefusals[c]; n > 0 {
			out = append(out, fmt.Sprintf("  refused %s: %d", c, n))
		}
	}
	out = append(out,
		fmt.Sprintf("Encounters: %d (%d written, %d already stored)", s.Encounters, s.Written, s.AlreadyStored),
		fmt.Sprintf("Valid following time: %s of %s accounted", seconds(s.ValidNanos), seconds(s.AccountedNanos)),
	)
	for _, r := range l8behaviour.SuppressionReasons() {
		if c, ok := s.Suppressed[r]; ok {
			out = append(out, fmt.Sprintf("  suppressed %s: %d instants, %s", r, c.Instants, seconds(c.Nanos)))
		}
	}
	out = append(out, fmt.Sprintf("Evaluated pair instants: %d; every reason that applied:", s.EvaluatedInstants))
	for _, r := range l8behaviour.SuppressionReasons() {
		if n := s.InstantReasons[r]; n > 0 {
			out = append(out, fmt.Sprintf("  %s: %d (%.0f%%)", r, n, 100*float64(n)/math.Max(1, float64(s.EvaluatedInstants))))
		}
	}
	return out
}
