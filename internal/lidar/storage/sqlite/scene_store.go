package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/storage/configasset"
	"github.com/google/uuid"
)

// ReplayCase represents a LiDAR evaluation replay case tying a PCAP to a sensor and parameters.
// A replay case is a specific environment captured in a PCAP with optional reference ground truth.
type ReplayCase struct {
	ReplayCaseID             string          `json:"replay_case_id"`
	SensorID                 string          `json:"sensor_id"`
	PCAPFile                 string          `json:"pcap_file"`
	PCAPStartSecs            *float64        `json:"pcap_start_secs,omitempty"`
	PCAPDurationSecs         *float64        `json:"pcap_duration_secs,omitempty"`
	Description              string          `json:"description,omitempty"`
	ReferenceRunID           string          `json:"reference_run_id,omitempty"`
	OptimalParamsJSON        json.RawMessage `json:"optimal_params_json,omitempty"`
	RecommendedParamSetID    string          `json:"recommended_param_set_id,omitempty"`
	RecommendedParamsHash    string          `json:"recommended_params_hash,omitempty"`
	RecommendedSchemaVersion string          `json:"recommended_schema_version,omitempty"`
	RecommendedParamSetType  string          `json:"recommended_param_set_type,omitempty"`
	RecommendedParams        json.RawMessage `json:"recommended_params,omitempty"`
	CreatedAtNs              int64           `json:"created_at_ns"`
	UpdatedAtNs              *int64          `json:"updated_at_ns,omitempty"`
}

// ReplayCaseStore provides persistence for LiDAR evaluation replay cases.
type ReplayCaseStore struct {
	db DBClient
}

type replayCaseRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// NewReplayCaseStore creates a new ReplayCaseStore.
func NewReplayCaseStore(db DBClient) *ReplayCaseStore {
	return &ReplayCaseStore{db: db}
}

func (s *ReplayCaseStore) replayCaseSelectColumns() []string {
	return []string{
		"replay_case_id",
		"sensor_id",
		"pcap_file",
		"pcap_start_secs",
		"pcap_duration_secs",
		"description",
		"reference_run_id",
		"created_at_ns",
		"updated_at_ns",
		"recommended_param_set_id",
	}
}

func scanReplayCase(scanner interface{ Scan(dest ...any) error }) (*ReplayCase, error) {
	var scene ReplayCase
	var pcapStartSecs, pcapDurationSecs sql.NullFloat64
	var description, referenceRunID sql.NullString
	var updatedAtNs sql.NullInt64
	var recommendedParamSetID sql.NullString

	dests := []any{
		&scene.ReplayCaseID,
		&scene.SensorID,
		&scene.PCAPFile,
		&pcapStartSecs,
		&pcapDurationSecs,
		&description,
		&referenceRunID,
		&scene.CreatedAtNs,
		&updatedAtNs,
		&recommendedParamSetID,
	}

	if err := scanner.Scan(dests...); err != nil {
		return nil, err
	}

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
	if updatedAtNs.Valid {
		v := updatedAtNs.Int64
		scene.UpdatedAtNs = &v
	}
	if recommendedParamSetID.Valid {
		scene.RecommendedParamSetID = recommendedParamSetID.String
	}

	return &scene, nil
}

func (s *ReplayCaseStore) normalizeRecommendedParamSet(scene *ReplayCase) error {
	if scene == nil {
		return fmt.Errorf("scene is required")
	}

	if len(scene.OptimalParamsJSON) == 0 || strings.TrimSpace(string(scene.OptimalParamsJSON)) == "" {
		scene.RecommendedParamSetID = ""
		return nil
	}

	configStore := configasset.NewStore(s.db)
	paramSet, err := configasset.MakeRequestedParamSet(scene.OptimalParamsJSON)
	if err != nil {
		return fmt.Errorf("canonicalize recommended params: %w", err)
	}
	storedParamSet, err := configStore.EnsureParamSet(paramSet)
	if err != nil {
		return fmt.Errorf("store recommended params: %w", err)
	}

	scene.RecommendedParamSetID = storedParamSet.ParamSetID
	return nil
}

func (s *ReplayCaseStore) hydrateRecommendedParamSet(scene *ReplayCase) {
	if scene == nil || strings.TrimSpace(scene.RecommendedParamSetID) == "" {
		return
	}

	configStore := configasset.NewStore(s.db)
	paramSet, err := configStore.GetParamSet(scene.RecommendedParamSetID)
	if err != nil {
		if err == sql.ErrNoRows {
			return
		}
		log.Printf("hydrate recommended param set for scene %s: %v", scene.ReplayCaseID, err)
		return
	}

	scene.RecommendedParamsHash = paramSet.ParamsHash
	scene.RecommendedSchemaVersion = paramSet.SchemaVersion
	scene.RecommendedParamSetType = paramSet.ParamSetType

	paramsPayload, err := configasset.ExtractParamsPayload(paramSet.ParamsJSON)
	if err == nil {
		scene.RecommendedParams = paramsPayload
	}
}

// InsertScene creates a new replay case in the database.
// If scene.ReplayCaseID is empty, a new UUID is generated.
func (s *ReplayCaseStore) InsertScene(scene *ReplayCase) error {
	if scene.ReplayCaseID == "" {
		scene.ReplayCaseID = uuid.New().String()
	}
	if scene.CreatedAtNs == 0 {
		scene.CreatedAtNs = time.Now().UnixNano()
	}
	if err := s.normalizeRecommendedParamSet(scene); err != nil {
		return err
	}

	columns := []string{
		"replay_case_id",
		"sensor_id",
		"pcap_file",
		"pcap_start_secs",
		"pcap_duration_secs",
		"description",
		"reference_run_id",
		"created_at_ns",
		"updated_at_ns",
		"recommended_param_set_id",
	}
	args := []any{
		scene.ReplayCaseID,
		scene.SensorID,
		scene.PCAPFile,
		nullFloat64(scene.PCAPStartSecs),
		nullFloat64(scene.PCAPDurationSecs),
		nullString(scene.Description),
		nullString(scene.ReferenceRunID),
		scene.CreatedAtNs,
		nullInt64(scene.UpdatedAtNs),
		nullString(scene.RecommendedParamSetID),
	}

	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`
		INSERT INTO lidar_replay_cases (
			%s
		) VALUES (%s)
	`, strings.Join(columns, ", "), strings.Join(placeholders, ", "))

	if _, err := s.db.Exec(query, args...); err != nil {
		return fmt.Errorf("insert scene: %w", err)
	}

	s.hydrateRecommendedParamSet(scene)
	return nil
}

// GetScene retrieves a replay case by ID.
func (s *ReplayCaseStore) GetScene(sceneID string) (*ReplayCase, error) {
	columns := s.replayCaseSelectColumns()

	query := fmt.Sprintf(`
		SELECT %s
		FROM lidar_replay_cases
		WHERE replay_case_id = ?
	`, strings.Join(columns, ", "))

	scene, err := scanReplayCase(s.db.QueryRow(query, sceneID))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("replay case not found: %s", sceneID)
	}
	if err != nil {
		return nil, fmt.Errorf("get scene: %w", err)
	}

	s.hydrateRecommendedParamSet(scene)
	return scene, nil
}

// ListScenes retrieves all replay cases, optionally filtered by sensor_id.
func (s *ReplayCaseStore) ListScenes(sensorID string) ([]*ReplayCase, error) {
	columns := s.replayCaseSelectColumns()

	var (
		query string
		args  []interface{}
	)

	if sensorID != "" {
		query = fmt.Sprintf(`
			SELECT %s
			FROM lidar_replay_cases
			WHERE sensor_id = ?
			ORDER BY created_at_ns DESC
		`, strings.Join(columns, ", "))
		args = append(args, sensorID)
	} else {
		query = fmt.Sprintf(`
			SELECT %s
			FROM lidar_replay_cases
			ORDER BY created_at_ns DESC
		`, strings.Join(columns, ", "))
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list scenes: %w", err)
	}
	defer rows.Close()

	return collectReplayCases(rows, s.hydrateRecommendedParamSet)
}

func collectReplayCases(rows replayCaseRows, hydrate func(*ReplayCase)) ([]*ReplayCase, error) {
	var scenes []*ReplayCase
	for rows.Next() {
		scene, err := scanReplayCase(rows)
		if err != nil {
			return nil, fmt.Errorf("scan scene row: %w", err)
		}
		hydrate(scene)
		scenes = append(scenes, scene)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list scenes rows: %w", err)
	}

	return scenes, nil
}

// UpdateScene updates an existing replay case's mutable fields for the given scene ID.
// Updates description, reference_run_id, pcap_start_secs, and pcap_duration_secs;
// empty strings are stored as NULL, which clears those fields.
func (s *ReplayCaseStore) UpdateScene(scene *ReplayCase) error {
	scene.UpdatedAtNs = new(int64)
	*scene.UpdatedAtNs = time.Now().UnixNano()
	if err := s.normalizeRecommendedParamSet(scene); err != nil {
		return err
	}

	setClauses := []string{
		"description = ?",
		"reference_run_id = ?",
		"pcap_start_secs = ?",
		"pcap_duration_secs = ?",
		"updated_at_ns = ?",
		"recommended_param_set_id = ?",
	}
	args := []any{
		nullString(scene.Description),
		nullString(scene.ReferenceRunID),
		nullFloat64(scene.PCAPStartSecs),
		nullFloat64(scene.PCAPDurationSecs),
		scene.UpdatedAtNs,
		nullString(scene.RecommendedParamSetID),
	}
	args = append(args, scene.ReplayCaseID)

	query := fmt.Sprintf(`
		UPDATE lidar_replay_cases
		SET %s
		WHERE replay_case_id = ?
	`, strings.Join(setClauses, ",\n\t\t    "))

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update scene: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("replay case not found: %s", scene.ReplayCaseID)
	}

	s.hydrateRecommendedParamSet(scene)
	return nil
}

// DeleteScene deletes a replay case by ID.
func (s *ReplayCaseStore) DeleteScene(sceneID string) error {
	query := `DELETE FROM lidar_replay_cases WHERE replay_case_id = ?`

	result, err := s.db.Exec(query, sceneID)
	if err != nil {
		return fmt.Errorf("delete scene: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check delete result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("replay case not found: %s", sceneID)
	}

	return nil
}

// SetReferenceRun sets the reference run ID for a replay case.
func (s *ReplayCaseStore) SetReferenceRun(sceneID, runID string) error {
	query := `
		UPDATE lidar_replay_cases
		SET reference_run_id = ?,
		    updated_at_ns = ?
		WHERE replay_case_id = ?
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
		return fmt.Errorf("replay case not found: %s", sceneID)
	}

	return nil
}

// SetOptimalParams sets the optimal parameters JSON for a replay case.
func (s *ReplayCaseStore) SetOptimalParams(sceneID string, paramsJSON json.RawMessage) error {
	scene, err := s.GetScene(sceneID)
	if err != nil {
		return err
	}

	scene.OptimalParamsJSON = paramsJSON
	return s.UpdateScene(scene)
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
