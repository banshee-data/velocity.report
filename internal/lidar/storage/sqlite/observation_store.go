package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
)

// ErrObservationExists reports an attempted revision of immutable evidence.
// Callers must create a new observation identity when source or calibration
// changes; replacing a row would make replay comparisons meaningless.
var ErrObservationExists = errors.New("immutable observation already exists")

// ObservationStore persists track-independent L4 evidence. It has no Update
// method by design: observations are the fixed inputs to later estimators.
type ObservationStore struct{ db DBClient }

func NewObservationStore(db DBClient) *ObservationStore { return &ObservationStore{db: db} }

// Insert writes one frozen observation. The payload is the owned snapshot, so
// the caller cannot mutate stored evidence by retaining a slice reference.
func (s *ObservationStore) Insert(observation l4bobserve.DetectionObservation) error {
	record := observation.Snapshot()
	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal observation %s: %w", record.ObservationID, err)
	}
	_, err = s.db.Exec(`
		INSERT INTO lidar_observations
			(observation_id, schema_version, source_id, calibration_id, sensor_id,
			 frame_id, frame_unix_nanos, cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ObservationID, record.SchemaVersion, record.SourceID, record.CalibrationID,
		record.Cluster.SensorID, record.Cluster.FrameID, record.FrameUnixNanos,
		record.Cluster.TSUnixNanos, record.Cluster.ClusterID, payload, time.Now().UnixNano())
	if err != nil {
		if isUniqueConstraint(err) {
			return fmt.Errorf("%w: %s", ErrObservationExists, record.ObservationID)
		}
		return fmt.Errorf("insert observation %s: %w", record.ObservationID, err)
	}
	return nil
}

// Get returns a fresh immutable observation reconstructed from its stored
// evidence. The relational identity fields are indexed for queries, while the
// JSON payload remains the authoritative replay record.
func (s *ObservationStore) Get(observationID string) (l4bobserve.DetectionObservation, error) {
	var payload []byte
	err := s.db.QueryRow(`SELECT record_json FROM lidar_observations WHERE observation_id = ?`, observationID).Scan(&payload)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return l4bobserve.DetectionObservation{}, ErrNotFound
		}
		return l4bobserve.DetectionObservation{}, fmt.Errorf("get observation %s: %w", observationID, err)
	}
	return decodeObservation(payload)
}

// ListBySource returns a capture sequence in deterministic frame and identity
// order. It deliberately has no mutable pagination token: source evidence is
// replayed as a bounded case, not a live activity feed.
func (s *ObservationStore) ListBySource(sourceID string) ([]l4bobserve.DetectionObservation, error) {
	rows, err := s.db.Query(`
		SELECT record_json FROM lidar_observations
		 WHERE source_id = ?
		 ORDER BY frame_unix_nanos, observation_id`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list observations for source %s: %w", sourceID, err)
	}
	defer rows.Close()
	result := []l4bobserve.DetectionObservation{}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan observation: %w", err)
		}
		observation, err := decodeObservation(payload)
		if err != nil {
			return nil, err
		}
		result = append(result, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate observations: %w", err)
	}
	return result, nil
}

func decodeObservation(payload []byte) (l4bobserve.DetectionObservation, error) {
	var record l4bobserve.Record
	if err := json.Unmarshal(payload, &record); err != nil {
		return l4bobserve.DetectionObservation{}, fmt.Errorf("decode stored observation: %w", err)
	}
	observation, err := l4bobserve.New(record)
	if err != nil {
		return l4bobserve.DetectionObservation{}, fmt.Errorf("validate stored observation: %w", err)
	}
	return observation, nil
}

func isUniqueConstraint(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint") || strings.Contains(strings.ToLower(err.Error()), "primary key")
}
