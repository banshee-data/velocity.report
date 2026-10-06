package sqlite

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"sort"
)

// EvidenceOracle is a compact, portable semantic fingerprint of immutable
// frame evidence. It deliberately excludes inserted_at_ns and SQLite page
// layout: both may differ when transaction mechanics change without changing
// the evidence itself.
type EvidenceOracle struct {
	SchemaVersion        int             `json:"schema_version"`
	SourceManifestSHA256 string          `json:"source_manifest_sha256"`
	Tables               []EvidenceTable `json:"tables"`
	Frames               []EvidenceFrame `json:"frames"`
}

type EvidenceTable struct {
	Name     string `json:"name"`
	RowCount int64  `json:"row_count"`
	SHA256   string `json:"sha256"`
}

type EvidenceFrame struct {
	SourceID       string          `json:"source_id"`
	FrameUnixNanos int64           `json:"frame_unix_nanos"`
	Tables         []EvidenceTable `json:"tables"`
}

// BuildEvidenceOracle reads the evidence tables in canonical
// source/frame/record order: observations, point estimates and residuals
// always, and solid bodies when the run wrote any (oracleOptionalTableNames).
// expectedSourceIDs is the set declared by the source-PCAP manifest; unknown
// or absent sources fail closed.
func BuildEvidenceOracle(db *sql.DB, expectedSourceIDs map[string]struct{}) (EvidenceOracle, error) {
	if len(expectedSourceIDs) == 0 {
		return EvidenceOracle{}, fmt.Errorf("expected source IDs are required")
	}
	result := oracleBuilder{
		tables:          map[string]*oracleDigest{},
		frames:          map[oracleFrameKey]*oracleFrameDigest{},
		seenSources:     map[string]struct{}{},
		expectedSources: expectedSourceIDs,
		observations:    map[string]oracleObservation{},
		estimates:       map[string]oracleEstimate{},
	}
	for _, name := range oracleDigestNames() {
		result.tables[name] = newOracleDigest()
	}
	if err := result.readObservations(db); err != nil {
		return EvidenceOracle{}, err
	}
	if err := result.readEstimates(db); err != nil {
		return EvidenceOracle{}, err
	}
	if err := result.readResiduals(db); err != nil {
		return EvidenceOracle{}, err
	}
	if err := result.readSolidBodies(db); err != nil {
		return EvidenceOracle{}, err
	}
	for sourceID := range expectedSourceIDs {
		if _, ok := result.seenSources[sourceID]; !ok {
			return EvidenceOracle{}, fmt.Errorf("source manifest source %q has no observations", sourceID)
		}
	}
	return result.finish(), nil
}

// oracleTableNames are listed in every oracle, whether or not they hold rows.
var oracleTableNames = []string{"lidar_observations", "lidar_track_estimates", "lidar_track_residuals"}

// oracleOptionalTableNames are listed, after oracleTableNames and in every
// frame, only when the run wrote rows to them. A run without the solid_body
// experiment writes no solid body, so its oracle is the one built before
// lidar_track_solid_bodies was covered, byte for byte, and an oracle file
// recorded then still verifies. A database written before the table existed
// (migration 000052), or whose table is empty, reads as one without solid
// bodies.
var oracleOptionalTableNames = []string{"lidar_track_solid_bodies"}

// oracleDigestNames is every table the builder digests, in report order.
func oracleDigestNames() []string {
	return append(append([]string{}, oracleTableNames...), oracleOptionalTableNames...)
}

type oracleDigest struct {
	count int64
	hash  hash.Hash
}

func newOracleDigest() *oracleDigest { return &oracleDigest{hash: sha256.New()} }

func (d *oracleDigest) add(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := d.hash.Write(payload); err != nil {
		return err
	}
	if _, err := d.hash.Write([]byte{'\n'}); err != nil {
		return err
	}
	d.count++
	return nil
}

func (d *oracleDigest) table(name string) EvidenceTable {
	return EvidenceTable{Name: name, RowCount: d.count, SHA256: "sha256:" + hex.EncodeToString(d.hash.Sum(nil))}
}

type oracleFrameKey struct {
	sourceID string
	frameNS  int64
}

type oracleFrameDigest struct{ tables map[string]*oracleDigest }

type oracleObservation struct {
	sourceID, calibrationID string
	frameNS                 int64
}

type oracleEstimate struct {
	observationID           string
	sourceID, calibrationID string
	frameNS                 int64
}

type oracleBuilder struct {
	tables          map[string]*oracleDigest
	frames          map[oracleFrameKey]*oracleFrameDigest
	seenSources     map[string]struct{}
	expectedSources map[string]struct{}
	observations    map[string]oracleObservation
	estimates       map[string]oracleEstimate
}

func (b *oracleBuilder) frame(sourceID string, frameNS int64) *oracleFrameDigest {
	key := oracleFrameKey{sourceID: sourceID, frameNS: frameNS}
	if frame, ok := b.frames[key]; ok {
		return frame
	}
	frame := &oracleFrameDigest{tables: map[string]*oracleDigest{}}
	for _, name := range oracleDigestNames() {
		frame.tables[name] = newOracleDigest()
	}
	b.frames[key] = frame
	return frame
}

func (b *oracleBuilder) add(table, sourceID string, frameNS int64, value any) error {
	if _, ok := b.expectedSources[sourceID]; !ok {
		return fmt.Errorf("%s contains source %q absent from source manifest", table, sourceID)
	}
	b.seenSources[sourceID] = struct{}{}
	if err := b.tables[table].add(value); err != nil {
		return fmt.Errorf("hash %s row: %w", table, err)
	}
	if err := b.frame(sourceID, frameNS).tables[table].add(value); err != nil {
		return fmt.Errorf("hash %s frame row: %w", table, err)
	}
	return nil
}

func (b *oracleBuilder) readObservations(db *sql.DB) error {
	rows, err := db.Query(`SELECT observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos, cluster_unix_nanos, cluster_id, record_json
		FROM lidar_observations ORDER BY source_id, frame_unix_nanos, cluster_unix_nanos, cluster_id, observation_id`)
	if err != nil {
		return fmt.Errorf("query observations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var row oracleObservationRow
		if err := rows.Scan(&row.ObservationID, &row.SchemaVersion, &row.SourceID, &row.CalibrationID, &row.SensorID, &row.FrameID, &row.FrameUnixNanos, &row.ClusterUnixNanos, &row.ClusterID, &row.RecordJSON); err != nil {
			return fmt.Errorf("scan observation: %w", err)
		}
		if _, exists := b.observations[row.ObservationID]; exists {
			return fmt.Errorf("duplicate observation ID %q", row.ObservationID)
		}
		b.observations[row.ObservationID] = oracleObservation{sourceID: row.SourceID, calibrationID: row.CalibrationID, frameNS: row.FrameUnixNanos}
		if err := b.add("lidar_observations", row.SourceID, row.FrameUnixNanos, row); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (b *oracleBuilder) readEstimates(db *sql.DB) error {
	// Ordered by observation_id, not estimate_id: estimate_id embeds the
	// track's random UUID (l5tracks assigns it that way deliberately, to stay
	// collision-free across tracker resets and restarts), so ordering by it
	// would make accumulation order itself non-reproducible between two
	// replays of the same input. One observation carries one online estimate
	// but may also carry refined ones (fixed_lag, final), so the version key
	// breaks the tie; with online rows alone the order is unchanged.
	rows, err := db.Query(`SELECT estimate_id, track_id, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos, estimator_id, observation_model_id, param_hash, stage, measurement_source, creation_sequence, x, y, vx, vy, covariance_json, reference_point, support_instant
		FROM lidar_track_estimates ORDER BY source_id, frame_unix_nanos, observation_id, estimator_id, observation_model_id, param_hash, stage`)
	if err != nil {
		return fmt.Errorf("query estimates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var estimateID, trackID string
		var row oracleEstimateRow
		if err := rows.Scan(&estimateID, &trackID, &row.ObservationID, &row.SourceID, &row.CalibrationID, &row.FrameUnixNanos, &row.MeasurementUnixNanos, &row.EstimatorID, &row.ObservationModelID, &row.ParamHash, &row.Stage, &row.MeasurementSource, &row.CreationSequence, &row.X, &row.Y, &row.VX, &row.VY, &row.CovarianceJSON, &row.ReferencePoint, &row.SupportInstant); err != nil {
			return fmt.Errorf("scan estimate: %w", err)
		}
		observation, ok := b.observations[row.ObservationID]
		if !ok {
			return fmt.Errorf("estimate %q links missing observation %q", estimateID, row.ObservationID)
		}
		if observation.sourceID != row.SourceID || observation.calibrationID != row.CalibrationID || observation.frameNS != row.FrameUnixNanos {
			return fmt.Errorf("estimate %q identity differs from observation %q", estimateID, row.ObservationID)
		}
		if _, exists := b.estimates[estimateID]; exists {
			return fmt.Errorf("duplicate estimate ID %q", estimateID)
		}
		b.estimates[estimateID] = oracleEstimate{observationID: row.ObservationID, sourceID: row.SourceID, calibrationID: row.CalibrationID, frameNS: row.FrameUnixNanos}
		_ = trackID // identity only; deliberately excluded from the hashed row, see oracleEstimateRow
		if err := b.add("lidar_track_estimates", row.SourceID, row.FrameUnixNanos, row); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (b *oracleBuilder) readResiduals(db *sql.DB) error {
	// Ordered by observation_id for the same reason as readEstimates:
	// estimate_id embeds the estimate's random track UUID. The joined version
	// key breaks ties between an observation's online and refined residuals;
	// the LEFT JOIN keeps an orphaned residual so the check below reports it.
	rows, err := db.Query(`SELECT r.estimate_id, r.observation_id, r.predicted_x, r.predicted_y, r.measurement_x, r.measurement_y, r.innovation_x, r.innovation_y, r.nis, r.geometry_cov_xx, r.geometry_cov_xy, r.geometry_cov_yy, r.disposition, r.reason
		FROM lidar_track_residuals r LEFT JOIN lidar_track_estimates e ON e.estimate_id = r.estimate_id
		ORDER BY r.observation_id, e.estimator_id, e.observation_model_id, e.param_hash, e.stage`)
	if err != nil {
		return fmt.Errorf("query residuals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var estimateID string
		var row oracleResidualRow
		if err := rows.Scan(&estimateID, &row.ObservationID, &row.PredictedX, &row.PredictedY, &row.MeasurementX, &row.MeasurementY, &row.InnovationX, &row.InnovationY, &row.NIS, &row.GeometryCovXX, &row.GeometryCovXY, &row.GeometryCovYY, &row.Disposition, &row.Reason); err != nil {
			return fmt.Errorf("scan residual: %w", err)
		}
		estimate, ok := b.estimates[estimateID]
		if !ok {
			return fmt.Errorf("residual links missing estimate %q", estimateID)
		}
		if estimate.observationID != row.ObservationID {
			return fmt.Errorf("residual %q observation differs from estimate", estimateID)
		}
		if err := b.add("lidar_track_residuals", estimate.sourceID, estimate.frameNS, row); err != nil {
			return err
		}
	}
	return rows.Err()
}

// solidBodyRowCount is how many rows lidar_track_solid_bodies holds: zero
// for a database written before the table existed.
func solidBodyRowCount(db *sql.DB) (int64, error) {
	var n int64
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'lidar_track_solid_bodies'`).Scan(&n)
	if err == nil && n > 0 {
		err = db.QueryRow(`SELECT COUNT(*) FROM lidar_track_solid_bodies`).Scan(&n)
	}
	return n, err
}

// readSolidBodies digests lidar_track_solid_bodies as readEstimates digests
// estimates: ordered by observation and version key, never by the random
// estimate or track identity, with creation_sequence breaking a tie, and each
// row linked to a stored observation of the same source, calibration and
// frame. An absent or empty table has nothing to read, whatever columns its
// schema version gave it; rows in a table without every column hashed here
// (one written before migration 000053) are an error, not a partial digest.
func (b *oracleBuilder) readSolidBodies(db *sql.DB) error {
	count, err := solidBodyRowCount(db)
	if err != nil {
		return fmt.Errorf("count solid bodies: %w", err)
	}
	if count == 0 {
		return nil
	}
	rows, err := db.Query(`SELECT estimate_id, observation_id, source_id, calibration_id, frame_unix_nanos, measurement_unix_nanos
		     , estimator_id, observation_model_id, param_hash, stage, creation_sequence, state_model, reference_point
		     , x, y, vx, vy, covariance_json
		     , heading_rad, heading_variance_rad2, heading_ambiguous_weight, heading_provenance
		     , length_m, length_sigma_m, length_frames, length_provenance
		     , width_m, width_sigma_m, width_frames, width_provenance
		     , height_m, height_sigma_m, height_frames, height_provenance
		     , ground_z, ground_surface_model, motion_class, motion_posterior, estimation_state
		     , last_observed_unix_nanos, support_points, coasted_frames, support_instant, support_fragmented, support_truncated
		     , measurement_source, measurement_rank, visible_faces, inferred_extent, aspect_rad, nis, fallback_reason
		FROM lidar_track_solid_bodies
		ORDER BY source_id, frame_unix_nanos, observation_id, estimator_id, observation_model_id, param_hash, stage, creation_sequence`)
	if err != nil {
		return fmt.Errorf("query solid bodies: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var estimateID string
		var row oracleSolidBodyRow
		if err := rows.Scan(&estimateID, &row.ObservationID, &row.SourceID, &row.CalibrationID, &row.FrameUnixNanos, &row.MeasurementUnixNanos,
			&row.EstimatorID, &row.ObservationModelID, &row.ParamHash, &row.Stage, &row.CreationSequence, &row.StateModel, &row.ReferencePoint,
			&row.X, &row.Y, &row.VX, &row.VY, &row.CovarianceJSON,
			&row.HeadingRad, &row.HeadingVarianceRad2, &row.HeadingAmbiguousWeight, &row.HeadingProvenance,
			&row.LengthM, &row.LengthSigmaM, &row.LengthFrames, &row.LengthProvenance,
			&row.WidthM, &row.WidthSigmaM, &row.WidthFrames, &row.WidthProvenance,
			&row.HeightM, &row.HeightSigmaM, &row.HeightFrames, &row.HeightProvenance,
			&row.GroundZ, &row.GroundSurfaceModel, &row.MotionClass, &row.MotionPosterior, &row.EstimationState,
			&row.LastObservedUnixNanos, &row.SupportPoints, &row.CoastedFrames, &row.SupportInstant, &row.SupportFragmented, &row.SupportTruncated,
			&row.MeasurementSource, &row.MeasurementRank, &row.VisibleFaces, &row.InferredExtent, &row.AspectRad, &row.NIS, &row.FallbackReason); err != nil {
			return fmt.Errorf("scan solid body: %w", err)
		}
		observation, ok := b.observations[row.ObservationID]
		if !ok {
			return fmt.Errorf("solid body %q links missing observation %q", estimateID, row.ObservationID)
		}
		if observation.sourceID != row.SourceID || observation.calibrationID != row.CalibrationID || observation.frameNS != row.FrameUnixNanos {
			return fmt.Errorf("solid body %q identity differs from observation %q", estimateID, row.ObservationID)
		}
		if err := b.add("lidar_track_solid_bodies", row.SourceID, row.FrameUnixNanos, row); err != nil {
			return err
		}
	}
	return rows.Err()
}

// listedTableNames is the oracle's table list for this run: every core table,
// then each optional table that holds rows.
func (b *oracleBuilder) listedTableNames() []string {
	names := append([]string{}, oracleTableNames...)
	for _, name := range oracleOptionalTableNames {
		if b.tables[name].count > 0 {
			names = append(names, name)
		}
	}
	return names
}

func (b *oracleBuilder) finish() EvidenceOracle {
	listed := b.listedTableNames()
	result := EvidenceOracle{SchemaVersion: 1, Tables: make([]EvidenceTable, 0, len(listed))}
	for _, name := range listed {
		result.Tables = append(result.Tables, b.tables[name].table(name))
	}
	keys := make([]oracleFrameKey, 0, len(b.frames))
	for key := range b.frames {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].sourceID == keys[j].sourceID {
			return keys[i].frameNS < keys[j].frameNS
		}
		return keys[i].sourceID < keys[j].sourceID
	})
	result.Frames = make([]EvidenceFrame, 0, len(keys))
	for _, key := range keys {
		frame := EvidenceFrame{SourceID: key.sourceID, FrameUnixNanos: key.frameNS, Tables: make([]EvidenceTable, 0, len(listed))}
		for _, name := range listed {
			frame.Tables = append(frame.Tables, b.frames[key].tables[name].table(name))
		}
		result.Frames = append(result.Frames, frame)
	}
	return result
}

type oracleObservationRow struct {
	ObservationID    string `json:"observation_id"`
	SchemaVersion    int    `json:"schema_version"`
	SourceID         string `json:"source_id"`
	CalibrationID    string `json:"calibration_id"`
	SensorID         string `json:"sensor_id"`
	FrameID          string `json:"frame_id"`
	FrameUnixNanos   int64  `json:"frame_unix_nanos"`
	ClusterUnixNanos int64  `json:"cluster_unix_nanos"`
	ClusterID        int64  `json:"cluster_id"`
	RecordJSON       []byte `json:"record_json"`
}

// oracleEstimateRow is the hashed content of one estimate. estimate_id and
// track_id are deliberately absent: l5tracks assigns track_id a random UUID
// so it stays collision-free across tracker resets and restarts (see
// internal/lidar/l5tracks/tracking.go), which means two replays of identical
// input produce different track IDs even though the physical estimate is
// unchanged. CreationSequence is the tracker's deterministic per-run
// substitute — reproducible across replays — and is what groups an estimate
// to its track here instead.
type oracleEstimateRow struct {
	ObservationID        string  `json:"observation_id"`
	SourceID             string  `json:"source_id"`
	CalibrationID        string  `json:"calibration_id"`
	FrameUnixNanos       int64   `json:"frame_unix_nanos"`
	MeasurementUnixNanos int64   `json:"measurement_unix_nanos"`
	EstimatorID          string  `json:"estimator_id"`
	ObservationModelID   string  `json:"observation_model_id"`
	ParamHash            string  `json:"param_hash"`
	Stage                string  `json:"stage"`
	MeasurementSource    string  `json:"measurement_source"`
	CreationSequence     int64   `json:"creation_sequence"`
	X                    float64 `json:"x"`
	Y                    float64 `json:"y"`
	VX                   float64 `json:"vx"`
	VY                   float64 `json:"vy"`
	CovarianceJSON       []byte  `json:"covariance_json"`
	// ReferencePoint and SupportInstant are hashed as stored, not parsed: the
	// oracle compares what two runs wrote, and a reader's refusal of a value
	// is not the oracle's to make.
	ReferencePoint string `json:"reference_point"`
	SupportInstant string `json:"support_instant"`
}

// oracleResidualRow is the hashed content of one residual. estimate_id is
// absent for the same reason as oracleEstimateRow; observation_id already
// identifies the row deterministically.
type oracleResidualRow struct {
	ObservationID string  `json:"observation_id"`
	PredictedX    float64 `json:"predicted_x"`
	PredictedY    float64 `json:"predicted_y"`
	MeasurementX  float64 `json:"measurement_x"`
	MeasurementY  float64 `json:"measurement_y"`
	InnovationX   float64 `json:"innovation_x"`
	InnovationY   float64 `json:"innovation_y"`
	NIS           float64 `json:"nis"`
	GeometryCovXX float64 `json:"geometry_cov_xx"`
	GeometryCovXY float64 `json:"geometry_cov_xy"`
	GeometryCovYY float64 `json:"geometry_cov_yy"`
	Disposition   string  `json:"disposition"`
	Reason        string  `json:"reason"`
}

// oracleSolidBodyRow is the hashed content of one solid body: every stored
// column but estimate_id and track_id, which carry the tracker's random track
// identity (see oracleEstimateRow), and inserted_at_ns. Values are hashed as
// stored, flags as their integers and an unknown aspect as null, so the
// oracle compares what two runs wrote rather than what a reader would make of
// it.
type oracleSolidBodyRow struct {
	ObservationID          string   `json:"observation_id"`
	SourceID               string   `json:"source_id"`
	CalibrationID          string   `json:"calibration_id"`
	FrameUnixNanos         int64    `json:"frame_unix_nanos"`
	MeasurementUnixNanos   int64    `json:"measurement_unix_nanos"`
	EstimatorID            string   `json:"estimator_id"`
	ObservationModelID     string   `json:"observation_model_id"`
	ParamHash              string   `json:"param_hash"`
	Stage                  string   `json:"stage"`
	CreationSequence       int64    `json:"creation_sequence"`
	StateModel             string   `json:"state_model"`
	ReferencePoint         string   `json:"reference_point"`
	X                      float64  `json:"x"`
	Y                      float64  `json:"y"`
	VX                     float64  `json:"vx"`
	VY                     float64  `json:"vy"`
	CovarianceJSON         []byte   `json:"covariance_json"`
	HeadingRad             float64  `json:"heading_rad"`
	HeadingVarianceRad2    float64  `json:"heading_variance_rad2"`
	HeadingAmbiguousWeight float64  `json:"heading_ambiguous_weight"`
	HeadingProvenance      string   `json:"heading_provenance"`
	LengthM                float64  `json:"length_m"`
	LengthSigmaM           float64  `json:"length_sigma_m"`
	LengthFrames           int64    `json:"length_frames"`
	LengthProvenance       string   `json:"length_provenance"`
	WidthM                 float64  `json:"width_m"`
	WidthSigmaM            float64  `json:"width_sigma_m"`
	WidthFrames            int64    `json:"width_frames"`
	WidthProvenance        string   `json:"width_provenance"`
	HeightM                float64  `json:"height_m"`
	HeightSigmaM           float64  `json:"height_sigma_m"`
	HeightFrames           int64    `json:"height_frames"`
	HeightProvenance       string   `json:"height_provenance"`
	GroundZ                float64  `json:"ground_z"`
	GroundSurfaceModel     string   `json:"ground_surface_model"`
	MotionClass            string   `json:"motion_class"`
	MotionPosterior        float64  `json:"motion_posterior"`
	EstimationState        string   `json:"estimation_state"`
	LastObservedUnixNanos  int64    `json:"last_observed_unix_nanos"`
	SupportPoints          int64    `json:"support_points"`
	CoastedFrames          int64    `json:"coasted_frames"`
	SupportInstant         string   `json:"support_instant"`
	SupportFragmented      int64    `json:"support_fragmented"`
	SupportTruncated       int64    `json:"support_truncated"`
	MeasurementSource      string   `json:"measurement_source"`
	MeasurementRank        int64    `json:"measurement_rank"`
	VisibleFaces           string   `json:"visible_faces"`
	InferredExtent         int64    `json:"inferred_extent"`
	AspectRad              *float64 `json:"aspect_rad"`
	NIS                    float64  `json:"nis"`
	FallbackReason         string   `json:"fallback_reason"`
}
