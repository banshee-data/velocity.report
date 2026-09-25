package perframeeval

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// StageFinal is the estimate stage acceptance scores: the full-track
// retrospective estimate (state-estimation plan §10.1). Online and fixed-lag
// estimates are what the system believed at the time, and a metric built on
// them quotes a number the system no longer believes.
const StageFinal = "final"

// StageOnline is what an analysis run's persisted track positions are: the
// filtered state at each matched frame.
const StageOnline = "online"

// ArmKind is where an arm's hypotheses come from.
type ArmKind string

const (
	ArmEstimates   ArmKind = "estimates"
	ArmAnalysisRun ArmKind = "analysis_run"
)

// ArmSpec names one arm. Set RunID for an analysis run; otherwise the
// estimate fields select one version of lidar_track_estimates, and any left
// empty must be unambiguous in the database.
type ArmSpec struct {
	Label              string
	DBPath             string
	SourceID           string
	EstimatorID        string
	ObservationModelID string
	ParamHash          string
	// Stage defaults to final.
	Stage string
	RunID string
	// DeclaredBaseline permits a non-final arm. It is recorded, and the
	// comparison lists it among its caveats.
	DeclaredBaseline bool
}

// Kind is the arm's source.
func (s ArmSpec) Kind() ArmKind {
	if s.RunID != "" {
		return ArmAnalysisRun
	}
	return ArmEstimates
}

// ArmIdentity is what an arm's numbers were computed from.
type ArmIdentity struct {
	Label string  `json:"label"`
	Kind  ArmKind `json:"kind"`
	// Database is the file's base name: enough to tell two arms apart in a
	// report without writing a machine's paths into it.
	Database           string `json:"database"`
	SourceID           string `json:"source_id,omitempty"`
	EstimatorID        string `json:"estimator_id,omitempty"`
	ObservationModelID string `json:"observation_model_id,omitempty"`
	ParamHash          string `json:"param_hash,omitempty"`
	RunID              string `json:"run_id,omitempty"`
	Stage              string `json:"stage"`
	DeclaredBaseline   bool   `json:"declared_baseline"`
	// TrackKey says what a hypothesis ID is. Neither is the random track_id:
	// the matcher breaks ties in sorted ID order, so a random label would let
	// two replays of the same input report different identity switches.
	TrackKey string `json:"track_key"`
	Tracks   int    `json:"tracks"`
	Points   int    `json:"points"`
}

// Hypothesis is one arm's tracks.
type Hypothesis struct {
	Identity ArmIdentity
	Series   []l8analytics.TrackSeries
}

// LoadArm reads one arm from its database, read-only.
func LoadArm(spec ArmSpec) (Hypothesis, error) {
	if spec.Label == "" {
		return Hypothesis{}, fmt.Errorf("an arm needs a label")
	}
	if spec.DBPath == "" {
		return Hypothesis{}, fmt.Errorf("arm %s: no database", spec.Label)
	}
	if spec.RunID != "" && (spec.SourceID != "" || spec.EstimatorID != "" || spec.ObservationModelID != "" ||
		spec.ParamHash != "" || spec.Stage != "") {
		return Hypothesis{}, fmt.Errorf("arm %s: name an analysis run or an estimate version, not both", spec.Label)
	}
	if spec.Stage == "" && spec.RunID == "" {
		spec.Stage = StageFinal
	}
	stage := spec.Stage
	if spec.RunID != "" {
		stage = StageOnline
	}
	// Refused before the database is opened: the rule does not depend on
	// what is in it.
	if stage != StageFinal && !spec.DeclaredBaseline {
		return Hypothesis{}, fmt.Errorf("arm %s: %s positions are stage %q, and acceptance scores stage %q; "+
			"declare the arm a baseline to score it anyway", spec.Label, spec.Kind(), stage, StageFinal)
	}

	database, err := sqlite.OpenReadOnly(spec.DBPath)
	if err != nil {
		return Hypothesis{}, fmt.Errorf("arm %s: open %s: %w", spec.Label, spec.DBPath, err)
	}
	defer database.Close()

	if spec.RunID != "" {
		return loadRunArm(database, spec)
	}
	return loadEstimateArm(database, spec)
}

func loadEstimateArm(database sqlite.DBClient, spec ArmSpec) (Hypothesis, error) {
	store := sqlite.NewStateEstimateStore(database)
	versions, err := store.ListEstimateVersions()
	if err != nil {
		return Hypothesis{}, fmt.Errorf("arm %s: %w", spec.Label, err)
	}
	version, err := selectVersion(versions, spec)
	if err != nil {
		return Hypothesis{}, err
	}
	positions, err := store.ListEstimatePositions(version)
	if err != nil {
		return Hypothesis{}, fmt.Errorf("arm %s: %w", spec.Label, err)
	}

	bySequence := map[int64]*l8analytics.TrackSeries{}
	trackOf := map[int64]string{}
	for _, p := range positions {
		if first, seen := trackOf[p.CreationSequence]; seen && first != p.TrackID {
			return Hypothesis{}, fmt.Errorf("arm %s: creation_sequence %d names two tracks (%s and %s). "+
				"The sequence restarts when the tracker resets, so this source holds more than one tracker run "+
				"and its identities cannot be keyed reproducibly", spec.Label, p.CreationSequence, first, p.TrackID)
		}
		trackOf[p.CreationSequence] = p.TrackID
		s := bySequence[p.CreationSequence]
		if s == nil {
			s = &l8analytics.TrackSeries{ID: fmt.Sprintf("seq-%06d", p.CreationSequence)}
			bySequence[p.CreationSequence] = s
		}
		s.Points = append(s.Points, l8analytics.SeriesPoint{TimestampNanos: p.FrameUnixNanos, X: p.X, Y: p.Y})
	}
	sequences := make([]int64, 0, len(bySequence))
	for seq := range bySequence {
		sequences = append(sequences, seq)
	}
	sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })
	series := make([]l8analytics.TrackSeries, 0, len(sequences))
	for _, seq := range sequences {
		series = append(series, *bySequence[seq])
	}

	return Hypothesis{
		Identity: ArmIdentity{
			Label: spec.Label, Kind: ArmEstimates, Database: filepath.Base(spec.DBPath),
			SourceID: version.SourceID, EstimatorID: version.EstimatorID,
			ObservationModelID: version.ObservationModelID, ParamHash: version.ParamHash,
			Stage: version.Stage, DeclaredBaseline: spec.DeclaredBaseline,
			TrackKey: "creation_sequence", Tracks: len(series), Points: len(positions),
		},
		Series: series,
	}, nil
}

// selectVersion finds the one version the spec names. Any field left empty
// matches anything, but the result must be a single version: mixing two
// estimators, parameter sets or sources in one arm is not an arm.
func selectVersion(versions []sqlite.EstimateVersion, spec ArmSpec) (sqlite.EstimateVersion, error) {
	var matched []sqlite.EstimateVersion
	for _, v := range versions {
		if (spec.SourceID == "" || v.SourceID == spec.SourceID) &&
			(spec.EstimatorID == "" || v.EstimatorID == spec.EstimatorID) &&
			(spec.ObservationModelID == "" || v.ObservationModelID == spec.ObservationModelID) &&
			(spec.ParamHash == "" || v.ParamHash == spec.ParamHash) &&
			v.Stage == spec.Stage {
			matched = append(matched, v)
		}
	}
	switch len(matched) {
	case 1:
		return matched[0], nil
	case 0:
		return sqlite.EstimateVersion{}, fmt.Errorf("arm %s: no estimates match source=%q estimator=%q observation_model=%q param_hash=%q stage=%q; the database holds:\n%s",
			spec.Label, spec.SourceID, spec.EstimatorID, spec.ObservationModelID, spec.ParamHash, spec.Stage, listVersions(versions))
	default:
		return sqlite.EstimateVersion{}, fmt.Errorf("arm %s: %d estimate versions match; name the source, estimator, observation model and parameter hash of one:\n%s",
			spec.Label, len(matched), listVersions(matched))
	}
}

func listVersions(versions []sqlite.EstimateVersion) string {
	if len(versions) == 0 {
		return "  (no estimates at all)"
	}
	const limit = 20
	var b strings.Builder
	for i, v := range versions {
		if i == limit {
			fmt.Fprintf(&b, "  ... and %d more\n", len(versions)-limit)
			break
		}
		fmt.Fprintf(&b, "  source=%s estimator=%s observation_model=%s param_hash=%s stage=%s (%d estimates, %d tracks)\n",
			v.SourceID, v.EstimatorID, v.ObservationModelID, v.ParamHash, v.Stage, v.Estimates, v.Tracks)
	}
	return strings.TrimRight(b.String(), "\n")
}

func loadRunArm(database sqlite.DBClient, spec ArmSpec) (Hypothesis, error) {
	runs := sqlite.NewAnalysisRunStore(database)
	n, err := runs.CountRunTracks(spec.RunID)
	if err != nil {
		return Hypothesis{}, fmt.Errorf("arm %s: %w", spec.Label, err)
	}
	if n == 0 {
		return Hypothesis{}, fmt.Errorf("arm %s: run %s has no tracks in lidar_run_tracks: it does not exist here, or recorded none",
			spec.Label, spec.RunID)
	}
	positions, err := runs.ListRunTrackPositions(spec.RunID)
	if err != nil {
		return Hypothesis{}, fmt.Errorf("arm %s: %w", spec.Label, err)
	}
	if len(positions) == 0 {
		return Hypothesis{}, fmt.Errorf("arm %s: run %s has %d tracks but no per-frame positions in lidar_track_observations. "+
			"A replay with track persistence disabled writes none; score its versioned estimates instead", spec.Label, spec.RunID, n)
	}

	byTrack := map[string]*l8analytics.TrackSeries{}
	for _, p := range positions {
		s := byTrack[p.TrackID]
		if s == nil {
			s = &l8analytics.TrackSeries{ID: p.TrackID}
			byTrack[p.TrackID] = s
		}
		s.Points = append(s.Points, l8analytics.SeriesPoint{TimestampNanos: p.FrameUnixNanos, X: p.X, Y: p.Y})
	}
	// A run's track_id is a random UUID and it has no creation sequence, so
	// the tracks are renamed by what they contain: ordered by first frame,
	// then first position, and the UUID only as a last resort. Two replays
	// producing the same tracks get the same names.
	series := make([]l8analytics.TrackSeries, 0, len(byTrack))
	for _, s := range byTrack {
		series = append(series, *s)
	}
	sort.Slice(series, func(i, j int) bool {
		a, b := series[i].Points[0], series[j].Points[0]
		switch {
		case a.TimestampNanos != b.TimestampNanos:
			return a.TimestampNanos < b.TimestampNanos
		case a.X != b.X:
			return a.X < b.X
		case a.Y != b.Y:
			return a.Y < b.Y
		}
		return series[i].ID < series[j].ID
	})
	for i := range series {
		series[i].ID = fmt.Sprintf("run-%06d", i+1)
	}

	return Hypothesis{
		Identity: ArmIdentity{
			Label: spec.Label, Kind: ArmAnalysisRun, Database: filepath.Base(spec.DBPath),
			RunID: spec.RunID, Stage: StageOnline, DeclaredBaseline: spec.DeclaredBaseline,
			TrackKey: "run_track_order", Tracks: len(series), Points: len(positions),
		},
		Series: series,
	}, nil
}
