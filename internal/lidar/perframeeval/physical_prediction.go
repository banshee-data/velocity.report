package perframeeval

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// The prediction layer of physical scoring.
//
// An arm's rows are read through the behaviour adapters
// (l8behaviour.TrajectoriesFromEstimates and TrajectoriesFromSolidBodies),
// which claim no reference point, heading or extent a row does not carry: a
// point estimate names the medoid or the visible box centre and has no body,
// and a solid body names the body centre only after a near-edge fix. The
// scorer compares what each prediction says, and records what that was.

// PredictedExtent is one body dimension as an estimate believed it.
type PredictedExtent struct {
	Metres      float64 `json:"metres"`
	SigmaMetres float64 `json:"sigma_metres"`
	Provenance  string  `json:"provenance"`
	// Evidence is false for a class prior: a belief about the class, not
	// about this object.
	Evidence bool `json:"evidence"`
}

// PredictedHeading is an estimate's body orientation belief.
type PredictedHeading struct {
	Rad      float64 `json:"rad"`
	SigmaRad float64 `json:"sigma_rad"`
	Resolved bool    `json:"resolved"`
}

// PredictedBody is one estimate version's claim about one track at one
// instant: the prediction layer, kept apart from the reference it is scored
// against.
type PredictedBody struct {
	TrackKey    string `json:"track_key"`
	TimestampNs int64  `json:"timestamp_ns"`
	// Reference names the point X and Y are: body_centre, cluster_medoid or
	// visible_obb_centre. Only a physical reference has a centre.
	Reference string  `json:"reference"`
	XM        float64 `json:"x_m"`
	YM        float64 `json:"y_m"`
	// PositionSigmaM is the root of the position covariance's trace.
	PositionSigmaM float64           `json:"position_sigma_m"`
	Physical       bool              `json:"physical"`
	CentreXM       float64           `json:"centre_x_m,omitempty"`
	CentreYM       float64           `json:"centre_y_m,omitempty"`
	Heading        *PredictedHeading `json:"heading,omitempty"`
	Length         *PredictedExtent  `json:"length,omitempty"`
	Width          *PredictedExtent  `json:"width,omitempty"`
	Height         *PredictedExtent  `json:"height,omitempty"`

	sample l8behaviour.TrajectorySample
}

// PhysicalArm is one estimate version's predictions, with the source
// identities its rows carry, which the reference's source must match.
type PhysicalArm struct {
	Identity ArmIdentity
	Bodies   []PredictedBody
	// SensorIDs and CalibrationIDs are every distinct value the rows name.
	SensorIDs      []string
	CalibrationIDs []string
}

// LoadPhysicalArm reads one estimate version for physical scoring. An
// analysis run names no reference point for its positions, so it is refused;
// the stage rules are LoadArm's.
func LoadPhysicalArm(spec ArmSpec) (PhysicalArm, error) {
	if spec.RunID != "" {
		return PhysicalArm{}, fmt.Errorf("arm %s: physical scoring needs an estimate version; an analysis run's positions name no point on the body", spec.Label)
	}
	if spec.Label == "" {
		return PhysicalArm{}, fmt.Errorf("an arm needs a label")
	}
	if spec.DBPath == "" {
		return PhysicalArm{}, fmt.Errorf("arm %s: no database", spec.Label)
	}
	if spec.Stage == "" {
		spec.Stage = StageFinal
	}
	if spec.Stage != StageFinal && !spec.DeclaredBaseline {
		return PhysicalArm{}, fmt.Errorf("arm %s: estimates are stage %q, and acceptance scores stage %q; "+
			"declare the arm a baseline to score it anyway", spec.Label, spec.Stage, StageFinal)
	}
	database, err := sqlite.OpenReadOnly(spec.DBPath)
	if err != nil {
		return PhysicalArm{}, fmt.Errorf("arm %s: open %s: %w", spec.Label, spec.DBPath, err)
	}
	defer database.Close()
	return loadPhysicalArm(sqlite.NewStateEstimateStore(database), spec)
}

func loadPhysicalArm(store *sqlite.StateEstimateStore, spec ArmSpec) (PhysicalArm, error) {
	listVersions, table := store.ListEstimateVersions, ""
	if spec.SolidBodies {
		listVersions, table = store.ListSolidBodyVersions, solidBodyTable
	}
	versions, err := listVersions()
	if err != nil {
		return PhysicalArm{}, fmt.Errorf("arm %s: %w", spec.Label, err)
	}
	version, err := selectVersion(versions, spec)
	if err != nil {
		return PhysicalArm{}, err
	}
	key := sqlite.EstimateVersionKey{SourceID: version.SourceID, EstimatorID: version.EstimatorID,
		ObservationModelID: version.ObservationModelID, ParamHash: version.ParamHash, Stage: version.Stage}

	sequence := map[string]int64{}
	heights := map[string]PredictedExtent{}
	sensors, calibrations := map[string]bool{}, map[string]bool{}
	var trajectories []l8behaviour.Trajectory
	bounds := l5tracks.DefaultConvergenceBounds()
	if spec.SolidBodies {
		rows, err := store.ListVersionSolidBodies(key)
		if err != nil {
			return PhysicalArm{}, fmt.Errorf("arm %s: %w", spec.Label, err)
		}
		persisted := make([]l8behaviour.PersistedSolidBody, len(rows))
		for i, r := range rows {
			persisted[i] = l8behaviour.PersistedSolidBody{
				TrackID: r.TrackID, SensorID: r.SensorID, FrameUnixNanos: r.FrameUnixNanos,
				EstimatorID: r.EstimatorID, ObsModelID: r.ObservationModelID, ParamHash: r.ParamHash, Stage: r.Stage,
				Reading: r.Reading,
			}
			sequence[r.TrackID] = r.CreationSequence
			sensors[r.SensorID], calibrations[r.CalibrationID] = true, true
			if h := r.Reading.Estimate.Height; h.Provenance != l5tracks.ProvenanceNone && h.Metres > 0 {
				heights[frameKey(r.TrackID, r.FrameUnixNanos)] = PredictedExtent{
					Metres: float64(h.Metres), SigmaMetres: float64(h.SigmaMetres),
					Provenance: h.Provenance.String(), Evidence: h.Provenance.IsEvidence(),
				}
			}
		}
		trajectories, err = l8behaviour.TrajectoriesFromSolidBodies(persisted, bounds)
		if err != nil {
			return PhysicalArm{}, fmt.Errorf("arm %s: %w", spec.Label, err)
		}
	} else {
		rows, err := store.ListVersionEstimates(key)
		if err != nil {
			return PhysicalArm{}, fmt.Errorf("arm %s: %w", spec.Label, err)
		}
		persisted := make([]l8behaviour.PersistedEstimate, len(rows))
		for i, r := range rows {
			persisted[i] = l8behaviour.PersistedEstimate{
				TrackID: r.TrackID, SensorID: r.SensorID, FrameUnixNanos: r.FrameUnixNanos,
				EstimatorID: r.EstimatorID, ObsModelID: r.ObservationModelID, ParamHash: r.ParamHash, Stage: r.Stage,
				MeasurementSource: r.MeasurementSource, MeasurementUnixNanos: r.MeasurementUnixNanos,
				X: r.X, Y: r.Y, VX: r.VX, VY: r.VY, Covariance: r.Covariance,
			}
			sequence[r.TrackID] = r.CreationSequence
			sensors[r.SensorID], calibrations[r.CalibrationID] = true, true
		}
		trajectories, err = l8behaviour.TrajectoriesFromEstimates(persisted, bounds)
		if err != nil {
			return PhysicalArm{}, fmt.Errorf("arm %s: %w", spec.Label, err)
		}
	}

	arm := PhysicalArm{SensorIDs: sortedKeys(sensors), CalibrationIDs: sortedKeys(calibrations)}
	tracks := map[string]bool{}
	for _, t := range trajectories {
		// The creation sequence, not the random track ID, keys an arm: two
		// replays of one input must name their tracks alike.
		key := fmt.Sprintf("seq-%06d", sequence[t.Passage.TrackID])
		if tracks[key] {
			return PhysicalArm{}, fmt.Errorf("arm %s: creation_sequence %d names two tracks; this source holds more than one tracker run",
				spec.Label, sequence[t.Passage.TrackID])
		}
		tracks[key] = true
		for _, s := range t.Samples {
			b := predictedBody(key, s)
			if h, ok := heights[frameKey(t.Passage.TrackID, s.CaptureUnixNanos)]; ok {
				b.Height = &h
			}
			arm.Bodies = append(arm.Bodies, b)
		}
	}
	sort.Slice(arm.Bodies, func(i, j int) bool {
		if arm.Bodies[i].TimestampNs != arm.Bodies[j].TimestampNs {
			return arm.Bodies[i].TimestampNs < arm.Bodies[j].TimestampNs
		}
		return arm.Bodies[i].TrackKey < arm.Bodies[j].TrackKey
	})
	arm.Identity = ArmIdentity{
		Label: spec.Label, Kind: ArmEstimates, Database: filepath.Base(spec.DBPath),
		SourceID: version.SourceID, EstimatorID: version.EstimatorID,
		ObservationModelID: version.ObservationModelID, ParamHash: version.ParamHash,
		Table: table, Stage: version.Stage, DeclaredBaseline: spec.DeclaredBaseline,
		TrackKey: "creation_sequence", Tracks: len(trajectories), Points: len(arm.Bodies),
	}
	return arm, nil
}

// predictedBody reads one sample as the prediction layer states it.
func predictedBody(key string, s l8behaviour.TrajectorySample) PredictedBody {
	b := PredictedBody{
		TrackKey: key, TimestampNs: s.CaptureUnixNanos, Reference: s.Reference.String(),
		XM: s.X, YM: s.Y, PositionSigmaM: math.Sqrt(math.Max(s.Covariance[0]+s.Covariance[5], 0)),
		Physical: s.Reference.IsPhysical(), sample: s,
	}
	if s.Heading.Provenance.Valid() {
		b.Heading = &PredictedHeading{Rad: s.Heading.Rad, SigmaRad: math.Sqrt(s.Heading.VarianceRad2), Resolved: s.Heading.Resolved()}
	}
	if b.Physical {
		// The offset is body-frame, so it needs the heading; a reference
		// with an offset always has one.
		b.CentreXM, b.CentreYM = s.X, s.Y
		if off := s.AnchorToCentre; off != (l8behaviour.BodyOffset{}) {
			c, sn := math.Cos(s.Heading.Rad), math.Sin(s.Heading.Rad)
			b.CentreXM += off.LongitudinalM*c - off.LateralM*sn
			b.CentreYM += off.LongitudinalM*sn + off.LateralM*c
		}
	}
	extent := func(e l8behaviour.ExtentBelief) *PredictedExtent {
		if !e.Present() {
			return nil
		}
		return &PredictedExtent{Metres: e.Metres, SigmaMetres: e.SigmaMetres, Provenance: e.Provenance.String(), Evidence: e.Provenance.IsEvidence()}
	}
	b.Length, b.Width = extent(s.Length), extent(s.Width)
	return b
}

func frameKey(trackID string, frame int64) string { return fmt.Sprintf("%s@%d", trackID, frame) }

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
