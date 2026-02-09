package lidar

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Scene represents a LiDAR evaluation scene tying a PCAP to a sensor and parameters.
// A scene is a specific environment captured in a PCAP with optional reference ground truth.
type Scene struct {
	SceneID           string          `json:"scene_id"`
	SensorID          string          `json:"sensor_id"`
	PCAPFile          string          `json:"pcap_file"`
	PCAPStartSecs     *float64        `json:"pcap_start_secs,omitempty"`
	PCAPDurationSecs  *float64        `json:"pcap_duration_secs,omitempty"`
	Description       string          `json:"description,omitempty"`
	ReferenceRunID    string          `json:"reference_run_id,omitempty"`
	OptimalParamsJSON json.RawMessage `json:"optimal_params_json,omitempty"`
	CreatedAtNs       int64           `json:"created_at_ns"`
	UpdatedAtNs       *int64          `json:"updated_at_ns,omitempty"`
}

// SceneStore provides persistence for LiDAR evaluation scenes.
type SceneStore struct {
	db *sql.DB
}

// NewSceneStore creates a new SceneStore.
func NewSceneStore(db *sql.DB) *SceneStore {
	return &SceneStore{db: db}
}

// InsertScene creates a new scene in the database.
// If scene.SceneID is empty, a new UUID is generated.
func (s *SceneStore) InsertScene(scene *Scene) error {
	if scene.SceneID == "" {
		scene.SceneID = uuid.New().String()
	}
	if scene.CreatedAtNs == 0 {
		scene.CreatedAtNs = time.Now().UnixNano()
	}

	query := `
		INSERT INTO lidar_scenes (
			scene_id, sensor_id, pcap_file, pcap_start_secs, pcap_duration_secs,
			description, reference_run_id, optimal_params_json,
			created_at_ns, updated_at_ns
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		scene.SceneID,
		scene.SensorID,
		scene.PCAPFile,
		nullFloat64(scene.PCAPStartSecs),
		nullFloat64(scene.PCAPDurationSecs),
		nullString(scene.Description),
		nullString(scene.ReferenceRunID),
		nullString(string(scene.OptimalParamsJSON)),
		scene.CreatedAtNs,
		nullInt64(scene.UpdatedAtNs),
	)
	if err != nil {
		return fmt.Errorf("insert scene: %w", err)
	}

	return nil
}

// GetScene retrieves a scene by ID.
func (s *SceneStore) GetScene(sceneID string) (*Scene, error) {
	query := `
		SELECT scene_id, sensor_id, pcap_file, pcap_start_secs, pcap_duration_secs,
		       description, reference_run_id, optimal_params_json,
		       created_at_ns, updated_at_ns
		FROM lidar_scenes
		WHERE scene_id = ?
	`

	var scene Scene
	var pcapStartSecs, pcapDurationSecs sql.NullFloat64
	var description, referenceRunID, optimalParamsJSON sql.NullString
	var updatedAtNs sql.NullInt64

	err := s.db.QueryRow(query, sceneID).Scan(
		&scene.SceneID,
		&scene.SensorID,
		&scene.PCAPFile,
		&pcapStartSecs,
		&pcapDurationSecs,
		&description,
		&referenceRunID,
		&optimalParamsJSON,
		&scene.CreatedAtNs,
		&updatedAtNs,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("scene not found: %s", sceneID)
	}
	if err != nil {
		return nil, fmt.Errorf("get scene: %w", err)
	}

	// Map nullable fields
	if pcapStartSecs.Valid {
		v := pcapStartSecs.Float64
		scene.PCAPStartSecs = &v
	}
	if pcapDurationSecs.Valid {
		v := pcapDurationSecs.Float64
		scene.PCAPDurationSecs = &v
	}
	if description.Valid {
		scene.Description = description.String
	}
	if referenceRunID.Valid {
		scene.ReferenceRunID = referenceRunID.String
	}
	if optimalParamsJSON.Valid && optimalParamsJSON.String != "" {
		scene.OptimalParamsJSON = json.RawMessage(optimalParamsJSON.String)
	}
	if updatedAtNs.Valid {
		v := updatedAtNs.Int64
		scene.UpdatedAtNs = &v
	}

	return &scene, nil
}

// ListScenes retrieves all scenes, optionally filtered by sensor_id.
func (s *SceneStore) ListScenes(sensorID string) ([]*Scene, error) {
	var query string
	var args []interface{}

	if sensorID != "" {
		query = `
			SELECT scene_id, sensor_id, pcap_file, pcap_start_secs, pcap_duration_secs,
			       description, reference_run_id, optimal_params_json,
			       created_at_ns, updated_at_ns
			FROM lidar_scenes
			WHERE sensor_id = ?
			ORDER BY created_at_ns DESC
		`
		args = append(args, sensorID)
	} else {
		query = `
			SELECT scene_id, sensor_id, pcap_file, pcap_start_secs, pcap_duration_secs,
			       description, reference_run_id, optimal_params_json,
			       created_at_ns, updated_at_ns
			FROM lidar_scenes
			ORDER BY created_at_ns DESC
		`
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list scenes: %w", err)
	}
	defer rows.Close()

	var scenes []*Scene
	for rows.Next() {
		var scene Scene
		var pcapStartSecs, pcapDurationSecs sql.NullFloat64
		var description, referenceRunID, optimalParamsJSON sql.NullString
		var updatedAtNs sql.NullInt64

		err := rows.Scan(
			&scene.SceneID,
			&scene.SensorID,
			&scene.PCAPFile,
			&pcapStartSecs,
			&pcapDurationSecs,
			&description,
			&referenceRunID,
			&optimalParamsJSON,
			&scene.CreatedAtNs,
			&updatedAtNs,
		)
		if err != nil {
			return nil, fmt.Errorf("scan scene row: %w", err)
		}

		// Map nullable fields
		if pcapStartSecs.Valid {
			v := pcapStartSecs.Float64
			scene.PCAPStartSecs = &v
		}
		if pcapDurationSecs.Valid {
			v := pcapDurationSecs.Float64
			scene.PCAPDurationSecs = &v
		}
		if description.Valid {
			scene.Description = description.String
		}
		if referenceRunID.Valid {
			scene.ReferenceRunID = referenceRunID.String
		}
		if optimalParamsJSON.Valid && optimalParamsJSON.String != "" {
			scene.OptimalParamsJSON = json.RawMessage(optimalParamsJSON.String)
		}
		if updatedAtNs.Valid {
			v := updatedAtNs.Int64
			scene.UpdatedAtNs = &v
		}

		scenes = append(scenes, &scene)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list scenes rows: %w", err)
	}

	return scenes, nil
}

// UpdateScene updates an existing scene's fields.
// Only updates non-empty fields (description, reference_run_id, optimal_params_json).
func (s *SceneStore) UpdateScene(scene *Scene) error {
	scene.UpdatedAtNs = new(int64)
	*scene.UpdatedAtNs = time.Now().UnixNano()

	query := `
		UPDATE lidar_scenes
		SET description = ?,
		    reference_run_id = ?,
		    optimal_params_json = ?,
		    updated_at_ns = ?
		WHERE scene_id = ?
	`

	result, err := s.db.Exec(query,
		nullString(scene.Description),
		nullString(scene.ReferenceRunID),
		nullString(string(scene.OptimalParamsJSON)),
		scene.UpdatedAtNs,
		scene.SceneID,
	)
	if err != nil {
		return fmt.Errorf("update scene: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("scene not found: %s", scene.SceneID)
	}

	return nil
}

// DeleteScene deletes a scene by ID.
func (s *SceneStore) DeleteScene(sceneID string) error {
	query := `DELETE FROM lidar_scenes WHERE scene_id = ?`

	result, err := s.db.Exec(query, sceneID)
	if err != nil {
		return fmt.Errorf("delete scene: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check delete result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("scene not found: %s", sceneID)
	}

	return nil
}

// SetReferenceRun sets the reference run ID for a scene.
func (s *SceneStore) SetReferenceRun(sceneID, runID string) error {
	query := `
		UPDATE lidar_scenes
		SET reference_run_id = ?,
		    updated_at_ns = ?
		WHERE scene_id = ?
	`

	updatedAtNs := time.Now().UnixNano()
	result, err := s.db.Exec(query, nullString(runID), updatedAtNs, sceneID)
	if err != nil {
		return fmt.Errorf("set reference run: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("scene not found: %s", sceneID)
	}

	return nil
}

// SetOptimalParams sets the optimal parameters JSON for a scene.
func (s *SceneStore) SetOptimalParams(sceneID string, paramsJSON json.RawMessage) error {
	query := `
		UPDATE lidar_scenes
		SET optimal_params_json = ?,
		    updated_at_ns = ?
		WHERE scene_id = ?
	`

	updatedAtNs := time.Now().UnixNano()
	result, err := s.db.Exec(query, nullString(string(paramsJSON)), updatedAtNs, sceneID)
	if err != nil {
		return fmt.Errorf("set optimal params: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("scene not found: %s", sceneID)
	}

	return nil
}

// Helper functions for nullable values (reusing existing patterns)

func nullFloat64(f *float64) interface{} {
	if f == nil {
		return nil
	}
	return *f
}

func nullInt64(i *int64) interface{} {
	if i == nil {
		return nil
	}
	return *i
}

// Note: nullString helper is defined in track_store.go within this package.
// It converts empty strings to nil for SQL storage (shared nullable string handling).
