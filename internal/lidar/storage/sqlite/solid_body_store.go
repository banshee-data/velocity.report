package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// TrackSolidBody is one persisted solid-body estimate: the versioned identity
// it shares with a point estimate, and the reading itself.
//
// It lives in lidar_track_solid_bodies, beside lidar_track_estimates rather
// than in it. The two answer different questions — where the filter's point
// is, and where the body is — so a reader of one can never be handed the
// other. Each row's state_model names the dynamic layout of its covariance
// blob, and a reader refuses a layout it does not know rather than guessing
// from the blob's length.
type TrackSolidBody struct {
	EstimateID           string
	TrackID              string
	ObservationID        string
	SourceID             string
	CalibrationID        string
	FrameUnixNanos       int64
	MeasurementUnixNanos int64
	EstimatorID          string
	ObservationModelID   string
	ParamHash            string
	// Stage is the version stage, with the lidar_track_estimates vocabulary:
	// "online" for a live estimate, "fixed_lag" for a smoothed one.
	Stage string
	// CreationSequence is the tracker's deterministic per-run track ordinal;
	// see TrackEstimate.
	CreationSequence int64
	Reading          l5tracks.SolidBodyReading
}

// persistedStage maps an in-memory estimate stage to the version stage it is
// stored under. l5tracks has no final stage: a final estimate is only ever
// read from a persisted final row.
func persistedStage(s l5tracks.EstimateStage) string {
	if s == l5tracks.StageSmoothed {
		return "fixed_lag"
	}
	return "online"
}

// knownStateModel reports whether this store can read a covariance blob back
// under the named layout.
func knownStateModel(stateModel string) bool {
	return stateModel == l5tracks.StateModelCVCartesianV1
}

func validateSolidBody(sb TrackSolidBody) error {
	if sb.EstimateID == "" || sb.TrackID == "" || sb.ObservationID == "" || sb.SourceID == "" ||
		sb.CalibrationID == "" || sb.EstimatorID == "" || sb.ObservationModelID == "" || sb.ParamHash == "" || sb.Stage == "" {
		return fmt.Errorf("solid-body estimate requires identity, model and stage fields")
	}
	e := sb.Reading.Estimate
	if !knownStateModel(e.StateModel) {
		return fmt.Errorf("solid-body estimate %s has state model %q, which this store cannot read back", sb.EstimateID, e.StateModel)
	}
	if persistedStage(e.Stage) != sb.Stage {
		return fmt.Errorf("solid-body estimate %s is a %s estimate filed under stage %q", sb.EstimateID, e.Stage, sb.Stage)
	}
	if r := sb.Reading.Measurement.Rank; r < 0 || r > 2 {
		return fmt.Errorf("solid-body estimate %s has measurement rank %d", sb.EstimateID, r)
	}
	block := [4]float32{sb.Reading.Covariance[0], sb.Reading.Covariance[1], sb.Reading.Covariance[4], sb.Reading.Covariance[5]}
	if block != e.PositionCovariance {
		return fmt.Errorf("solid-body estimate %s: covariance position block %v disagrees with the estimate's %v", sb.EstimateID, block, e.PositionCovariance)
	}
	return nil
}

const solidBodyInsertSQL = `INSERT OR REPLACE INTO lidar_track_solid_bodies
		(estimate_id, track_id, creation_sequence, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos,
		 estimator_id, observation_model_id, param_hash, stage, state_model, reference_point, x, y, vx, vy, covariance_json,
		 heading_rad, heading_variance_rad2, heading_ambiguous_weight, heading_provenance,
		 length_m, length_sigma_m, length_frames, length_provenance,
		 width_m, width_sigma_m, width_frames, width_provenance,
		 height_m, height_sigma_m, height_frames, height_provenance,
		 ground_z, ground_surface_model, motion_class, motion_posterior, estimation_state,
		 last_observed_unix_nanos, support_points, coasted_frames,
		 measurement_source, measurement_rank, visible_faces, inferred_extent, aspect_rad, nis, fallback_reason, inserted_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		        ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

func solidBodyInsertArgs(sb TrackSolidBody, covariance []byte, insertedAtNanos int64) []any {
	e := sb.Reading.Estimate
	m := sb.Reading.Measurement
	var aspect any
	if m.AspectKnown {
		aspect = m.AspectRad
	}
	return []any{
		sb.EstimateID, sb.TrackID, sb.CreationSequence, sb.ObservationID, sb.SourceID, sb.CalibrationID,
		sb.FrameUnixNanos, sb.MeasurementUnixNanos, sb.EstimatorID, sb.ObservationModelID, sb.ParamHash, sb.Stage,
		e.StateModel, e.Reference.String(), e.X, e.Y, sb.Reading.VX, sb.Reading.VY, covariance,
		e.Orientation.PsiRad, e.Orientation.VarianceRad2, e.Orientation.AmbiguousModeWeight, e.Orientation.Provenance.String(),
		e.Length.Metres, e.Length.SigmaMetres, e.Length.AdmissibleFrames, e.Length.Provenance.String(),
		e.Width.Metres, e.Width.SigmaMetres, e.Width.AdmissibleFrames, e.Width.Provenance.String(),
		e.Height.Metres, e.Height.SigmaMetres, e.Height.AdmissibleFrames, e.Height.Provenance.String(),
		e.GroundZ, e.GroundSurfaceModel, e.Motion.Class.String(), e.Motion.Posterior, e.Estimation.String(),
		e.LastObservedUnixNanos, e.Support.PointCount, e.Support.CoastedFrames,
		string(m.Source), m.Rank, m.Faces.String(), m.InferredExtent, aspect, m.NIS, m.FallbackReason, insertedAtNanos,
	}
}

// marshalSolidBody validates a solid body and encodes its covariance, so that
// both write paths refuse the same records for the same reasons.
func marshalSolidBody(sb TrackSolidBody) ([]byte, error) {
	if err := validateSolidBody(sb); err != nil {
		return nil, err
	}
	covariance, err := json.Marshal(sb.Reading.Covariance)
	if err != nil {
		return nil, fmt.Errorf("marshal solid-body covariance: %w", err)
	}
	return covariance, nil
}

// InsertSolidBody writes one solid-body estimate through a database or a
// caller-owned transaction. As for a point estimate, replacing a derived row
// is allowed only for its exact versioned key.
func InsertSolidBody(exec Executor, sb TrackSolidBody) error {
	return insertSolidBody(exec, sb, time.Now().UnixNano())
}

func insertSolidBody(exec Executor, sb TrackSolidBody, insertedAtNanos int64) error {
	covariance, err := marshalSolidBody(sb)
	if err != nil {
		return err
	}
	if _, err := exec.Exec(solidBodyInsertSQL, solidBodyInsertArgs(sb, covariance, insertedAtNanos)...); err != nil {
		return fmt.Errorf("insert solid-body estimate %s: %w", sb.EstimateID, err)
	}
	return nil
}

// InsertSolidBody writes one solid-body estimate.
func (s *StateEstimateStore) InsertSolidBody(sb TrackSolidBody) error {
	return InsertSolidBody(s.db, sb)
}

// ListSolidBodiesBySource returns a source's solid-body estimates in the same
// deterministic order as ListBySource: creation_sequence, then frame, then
// estimate_id. A row whose state_model this store does not know is an error,
// never decoded by guessing.
func (s *StateEstimateStore) ListSolidBodiesBySource(sourceID string) ([]TrackSolidBody, error) {
	rows, err := s.db.Query(`
		SELECT estimate_id, track_id, creation_sequence, observation_id, source_id, calibration_id
		     , frame_unix_nanos, measurement_unix_nanos, estimator_id, observation_model_id, param_hash, stage
		     , state_model, reference_point, x, y, vx, vy, covariance_json
		     , heading_rad, heading_variance_rad2, heading_ambiguous_weight, heading_provenance
		     , length_m, length_sigma_m, length_frames, length_provenance
		     , width_m, width_sigma_m, width_frames, width_provenance
		     , height_m, height_sigma_m, height_frames, height_provenance
		     , ground_z, ground_surface_model, motion_class, motion_posterior, estimation_state
		     , last_observed_unix_nanos, support_points, coasted_frames
		     , measurement_source, measurement_rank, visible_faces, inferred_extent, aspect_rad, nis, fallback_reason
		  FROM lidar_track_solid_bodies
		 WHERE source_id = ?
		 ORDER BY creation_sequence, frame_unix_nanos, estimate_id`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list solid-body estimates for source %s: %w", sourceID, err)
	}
	defer rows.Close()

	out := []TrackSolidBody{}
	for rows.Next() {
		sb, err := scanSolidBody(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate solid-body estimates: %w", err)
	}
	return out, nil
}

// scanSolidBody reads one row in ListSolidBodiesBySource's column order.
func scanSolidBody(rows *sql.Rows) (TrackSolidBody, error) {
	var (
		sb                                                         TrackSolidBody
		covariance                                                 []byte
		reference, headingProv, lengthProv, widthProv, heightProv  string
		motionClass, estimation, measurementSource, faces          string
		aspect                                                     sql.NullFloat64
		lengthFrames, widthFrames, heightFrames, supportPoints     int
		coasted, rank                                              int
		inferred                                                   bool
		e                                                          = &sb.Reading.Estimate
		m                                                          = &sb.Reading.Measurement
		x, y, vx, vy, psi, psiVar, ambiguity, groundZ, posterior   float64
		lengthM, lengthSigma, widthM, widthSigma, heightM, heightS float64
		nis                                                        float64
	)
	if err := rows.Scan(
		&sb.EstimateID, &sb.TrackID, &sb.CreationSequence, &sb.ObservationID, &sb.SourceID, &sb.CalibrationID,
		&sb.FrameUnixNanos, &sb.MeasurementUnixNanos, &sb.EstimatorID, &sb.ObservationModelID, &sb.ParamHash, &sb.Stage,
		&e.StateModel, &reference, &x, &y, &vx, &vy, &covariance,
		&psi, &psiVar, &ambiguity, &headingProv,
		&lengthM, &lengthSigma, &lengthFrames, &lengthProv,
		&widthM, &widthSigma, &widthFrames, &widthProv,
		&heightM, &heightS, &heightFrames, &heightProv,
		&groundZ, &e.GroundSurfaceModel, &motionClass, &posterior, &estimation,
		&e.LastObservedUnixNanos, &supportPoints, &coasted,
		&measurementSource, &rank, &faces, &inferred, &aspect, &nis, &m.FallbackReason,
	); err != nil {
		return sb, fmt.Errorf("scan solid-body estimate: %w", err)
	}
	if !knownStateModel(e.StateModel) {
		return sb, fmt.Errorf("solid-body estimate %s has state model %q; refusing to decode its covariance", sb.EstimateID, e.StateModel)
	}
	if err := json.Unmarshal(covariance, &sb.Reading.Covariance); err != nil {
		return sb, fmt.Errorf("unmarshal solid-body covariance %s: %w", sb.EstimateID, err)
	}
	var err error
	fail := func(what string, err error) (TrackSolidBody, error) {
		return sb, fmt.Errorf("solid-body estimate %s %s: %w", sb.EstimateID, what, err)
	}
	if e.Reference, err = l5tracks.ParseReferencePoint(reference); err != nil {
		return fail("reference", err)
	}
	if e.Orientation.Provenance, err = l5tracks.ParseProvenance(headingProv); err != nil {
		return fail("heading provenance", err)
	}
	if e.Length.Provenance, err = l5tracks.ParseProvenance(lengthProv); err != nil {
		return fail("length provenance", err)
	}
	if e.Width.Provenance, err = l5tracks.ParseProvenance(widthProv); err != nil {
		return fail("width provenance", err)
	}
	if e.Height.Provenance, err = l5tracks.ParseProvenance(heightProv); err != nil {
		return fail("height provenance", err)
	}
	if e.Motion.Class, err = l5tracks.ParseMotionClass(motionClass); err != nil {
		return fail("motion class", err)
	}
	if e.Estimation, err = l5tracks.ParseEstimationState(estimation); err != nil {
		return fail("estimation state", err)
	}
	if m.Faces, err = l5tracks.ParseVisibleFaces(faces); err != nil {
		return fail("visible faces", err)
	}
	switch sb.Stage {
	case "online":
		e.Stage = l5tracks.StageLive
	case "fixed_lag":
		e.Stage = l5tracks.StageSmoothed
	default:
		return sb, fmt.Errorf("solid-body estimate %s has stage %q, which has no in-memory estimate stage", sb.EstimateID, sb.Stage)
	}

	e.X, e.Y = float32(x), float32(y)
	sb.Reading.VX, sb.Reading.VY = float32(vx), float32(vy)
	e.PositionCovariance = [4]float32{
		sb.Reading.Covariance[0], sb.Reading.Covariance[1],
		sb.Reading.Covariance[4], sb.Reading.Covariance[5],
	}
	e.Orientation.PsiRad = float32(psi)
	e.Orientation.VarianceRad2 = float32(psiVar)
	e.Orientation.AmbiguousModeWeight = float32(ambiguity)
	e.Length = l5tracks.DimensionBelief{Metres: float32(lengthM), SigmaMetres: float32(lengthSigma), AdmissibleFrames: lengthFrames, Provenance: e.Length.Provenance}
	e.Width = l5tracks.DimensionBelief{Metres: float32(widthM), SigmaMetres: float32(widthSigma), AdmissibleFrames: widthFrames, Provenance: e.Width.Provenance}
	e.Height = l5tracks.DimensionBelief{Metres: float32(heightM), SigmaMetres: float32(heightS), AdmissibleFrames: heightFrames, Provenance: e.Height.Provenance}
	e.GroundZ = float32(groundZ)
	e.Motion.Posterior = float32(posterior)
	e.Support = l5tracks.SupportState{PointCount: supportPoints, CoastedFrames: coasted}
	m.Source = l5tracks.MeasurementSource(measurementSource)
	m.Rank = rank
	m.InferredExtent = inferred
	m.NIS = float32(nis)
	if aspect.Valid {
		m.AspectRad, m.AspectKnown = float32(aspect.Float64), true
	}
	return sb, nil
}
