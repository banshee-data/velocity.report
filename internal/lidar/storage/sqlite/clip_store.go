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

// Clip represents a LiDAR evaluation clip tying a PCAP to a sensor and parameters.
// A clip is a specific environment captured in a PCAP with optional reference ground truth.
type Clip struct {
	ClipID                   string          `json:"clip_id"`
	SensorID                 string          `json:"sensor_id"`
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

	// Files is the ordered capture association, loaded by the store.
	// Paths live only on these capture rows; the clip has no path projection.
	Files []ClipCapture `json:"files,omitempty"`
	// FileCount is the number of associated captures.
	FileCount int `json:"file_count"`
	// SessionID and SourcePeriodID record where the case was cut from, when it
	// came from an indexed session. Both advisory.
	SessionID      string `json:"session_id,omitempty"`
	SourcePeriodID string `json:"source_period_id,omitempty"`

	// Location is where the capture was taken, when a position has been
	// accepted. Populated by the handlers that load it; the family displays it
	// carries are derived on read and never stored.
	Location *CaseLocation `json:"location,omitempty"`
}

// ClipStore provides persistence for LiDAR evaluation clips.
type ClipStore struct {
	db DBClient
}

type clipRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

// NewClipStore creates a new ClipStore.
func NewClipStore(db DBClient) *ClipStore {
	return &ClipStore{db: db}
}

func (s *ClipStore) clipSelectColumns() []string {
	return []string{
		"clip_id",
		"sensor_id",
		"pcap_start_secs",
		"pcap_duration_secs",
		"description",
		"reference_run_id",
		"created_at_ns",
		"updated_at_ns",
		"recommended_param_set_id",
		// Paths and counts come only from the capture association.
		`(SELECT COUNT(*) FROM lidar_clip_captures WHERE clip_id = lidar_clips.clip_id)`,
	}
}

func scanClip(scanner interface{ Scan(dest ...any) error }) (*Clip, error) {
	var clip Clip
	var pcapStartSecs, pcapDurationSecs sql.NullFloat64
	var description, referenceRunID sql.NullString
	var updatedAtNs sql.NullInt64
	var recommendedParamSetID sql.NullString

	dests := []any{
		&clip.ClipID,
		&clip.SensorID,
		&pcapStartSecs,
		&pcapDurationSecs,
		&description,
		&referenceRunID,
		&clip.CreatedAtNs,
		&updatedAtNs,
		&recommendedParamSetID,
		&clip.FileCount,
	}

	if err := scanner.Scan(dests...); err != nil {
		return nil, err
	}

	if pcapStartSecs.Valid {
		v := pcapStartSecs.Float64
		clip.PCAPStartSecs = &v
	}
	if pcapDurationSecs.Valid {
		v := pcapDurationSecs.Float64
		clip.PCAPDurationSecs = &v
	}
	if description.Valid {
		clip.Description = description.String
	}
	if referenceRunID.Valid {
		clip.ReferenceRunID = referenceRunID.String
	}
	if updatedAtNs.Valid {
		v := updatedAtNs.Int64
		clip.UpdatedAtNs = &v
	}
	if recommendedParamSetID.Valid {
		clip.RecommendedParamSetID = recommendedParamSetID.String
	}

	return &clip, nil
}

func (s *ClipStore) normalizeRecommendedParamSet(clip *Clip) error {
	if clip == nil {
		return fmt.Errorf("clip is required")
	}

	if len(clip.OptimalParamsJSON) == 0 || strings.TrimSpace(string(clip.OptimalParamsJSON)) == "" {
		clip.RecommendedParamSetID = ""
		return nil
	}

	configStore := configasset.NewStore(s.db)
	paramSet, err := configasset.MakeRequestedParamSet(clip.OptimalParamsJSON)
	if err != nil {
		return fmt.Errorf("canonicalize recommended params: %w", err)
	}
	storedParamSet, err := configStore.EnsureParamSet(paramSet)
	if err != nil {
		return fmt.Errorf("store recommended params: %w", err)
	}

	clip.RecommendedParamSetID = storedParamSet.ParamSetID
	return nil
}

func (s *ClipStore) hydrateRecommendedParamSet(clip *Clip) {
	if clip == nil || strings.TrimSpace(clip.RecommendedParamSetID) == "" {
		return
	}

	configStore := configasset.NewStore(s.db)
	paramSet, err := configStore.GetParamSet(clip.RecommendedParamSetID)
	if err != nil {
		if err == sql.ErrNoRows {
			return
		}
		log.Printf("hydrate recommended param set for clip %s: %v", clip.ClipID, err)
		return
	}

	clip.RecommendedParamsHash = paramSet.ParamsHash
	clip.RecommendedSchemaVersion = paramSet.SchemaVersion
	clip.RecommendedParamSetType = paramSet.ParamSetType

	paramsPayload, err := configasset.ExtractParamsPayload(paramSet.ParamsJSON)
	if err == nil {
		clip.RecommendedParams = paramsPayload
	}
}

// InsertClip creates a new clip in the database.
// If clip.ClipID is empty, a new UUID is generated.
func (s *ClipStore) InsertClip(clip *Clip) error {
	if clip.ClipID == "" {
		clip.ClipID = uuid.New().String()
	}
	if clip.CreatedAtNs == 0 {
		clip.CreatedAtNs = time.Now().UnixNano()
	}
	if err := s.normalizeRecommendedParamSet(clip); err != nil {
		return err
	}

	columns := []string{
		"clip_id",
		"sensor_id",
		"pcap_start_secs",
		"pcap_duration_secs",
		"description",
		"reference_run_id",
		"created_at_ns",
		"updated_at_ns",
		"recommended_param_set_id",
	}
	args := []any{
		clip.ClipID,
		clip.SensorID,
		nullFloat64(clip.PCAPStartSecs),
		nullFloat64(clip.PCAPDurationSecs),
		nullString(clip.Description),
		nullString(clip.ReferenceRunID),
		clip.CreatedAtNs,
		nullInt64(clip.UpdatedAtNs),
		nullString(clip.RecommendedParamSetID),
	}

	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`
		INSERT INTO lidar_clips (
			%s
		) VALUES (%s)
	`, strings.Join(columns, ", "), strings.Join(placeholders, ", "))

	if _, err := s.db.Exec(query, args...); err != nil {
		return fmt.Errorf("insert clip: %w", err)
	}
	if len(clip.Files) > 0 {
		if err := s.replaceClipFiles(clip.ClipID, clip.Files, nullInt64(clip.UpdatedAtNs)); err != nil {
			_, _ = s.db.Exec(`DELETE FROM lidar_clips WHERE clip_id = ?`, clip.ClipID)
			return err
		}
	}

	clip.FileCount = len(clip.Files)

	s.hydrateRecommendedParamSet(clip)
	return nil
}

// GetClip retrieves a clip by ID.
func (s *ClipStore) GetClip(clipID string) (*Clip, error) {
	columns := s.clipSelectColumns()

	query := fmt.Sprintf(`
		SELECT %s
		FROM lidar_clips
		WHERE clip_id = ?
	`, strings.Join(columns, ", "))

	clip, err := scanClip(s.db.QueryRow(query, clipID))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("clip not found: %s", clipID)
	}
	if err != nil {
		return nil, fmt.Errorf("get clip: %w", err)
	}

	s.hydrateRecommendedParamSet(clip)
	clip.Files, err = s.ClipFiles(clipID)
	if err != nil {
		return nil, err
	}
	return clip, nil
}

// ListClips retrieves all clips, optionally filtered by sensor_id.
func (s *ClipStore) ListClips(sensorID string) ([]*Clip, error) {
	columns := s.clipSelectColumns()

	var (
		query string
		args  []interface{}
	)

	if sensorID != "" {
		query = fmt.Sprintf(`
			SELECT %s
			FROM lidar_clips
			WHERE sensor_id = ?
			ORDER BY created_at_ns DESC
		`, strings.Join(columns, ", "))
		args = append(args, sensorID)
	} else {
		query = fmt.Sprintf(`
			SELECT %s
			FROM lidar_clips
			ORDER BY created_at_ns DESC
		`, strings.Join(columns, ", "))
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list clips: %w", err)
	}
	defer rows.Close()

	clips, err := collectClips(rows, s.hydrateRecommendedParamSet)
	if err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, clip := range clips {
		clip.Files, err = s.ClipFiles(clip.ClipID)
		if err != nil {
			return nil, err
		}
	}
	return clips, nil
}

func collectClips(rows clipRows, hydrate func(*Clip)) ([]*Clip, error) {
	var clips []*Clip
	for rows.Next() {
		clip, err := scanClip(rows)
		if err != nil {
			return nil, fmt.Errorf("scan clip row: %w", err)
		}
		hydrate(clip)
		clips = append(clips, clip)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list clips rows: %w", err)
	}

	return clips, nil
}

// UpdateClip updates an existing clip's mutable fields for the given clip ID.
// Updates description, reference_run_id, pcap_start_secs, and pcap_duration_secs;
// empty strings are stored as NULL, which clears those fields.
func (s *ClipStore) UpdateClip(clip *Clip) error {
	clip.UpdatedAtNs = new(int64)
	*clip.UpdatedAtNs = time.Now().UnixNano()
	if err := s.normalizeRecommendedParamSet(clip); err != nil {
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
		nullString(clip.Description),
		nullString(clip.ReferenceRunID),
		nullFloat64(clip.PCAPStartSecs),
		nullFloat64(clip.PCAPDurationSecs),
		clip.UpdatedAtNs,
		nullString(clip.RecommendedParamSetID),
	}
	args = append(args, clip.ClipID)

	query := fmt.Sprintf(`
		UPDATE lidar_clips
		SET %s
		WHERE clip_id = ?
	`, strings.Join(setClauses, ",\n\t\t    "))

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update clip: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("clip not found: %s", clip.ClipID)
	}

	s.hydrateRecommendedParamSet(clip)
	return nil
}

// DeleteClip deletes a clip by ID.
func (s *ClipStore) DeleteClip(clipID string) error {
	query := `DELETE FROM lidar_clips WHERE clip_id = ?`

	result, err := s.db.Exec(query, clipID)
	if err != nil {
		return fmt.Errorf("delete clip: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check delete result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("clip not found: %s", clipID)
	}

	return nil
}

// SetReferenceRun sets the reference run ID for a clip.
func (s *ClipStore) SetReferenceRun(clipID, runID string) error {
	query := `
		UPDATE lidar_clips
		SET reference_run_id = ?,
		    updated_at_ns = ?
		WHERE clip_id = ?
	`

	updatedAtNs := time.Now().UnixNano()
	result, err := s.db.Exec(query, nullString(runID), updatedAtNs, clipID)
	if err != nil {
		return fmt.Errorf("set reference run: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("clip not found: %s", clipID)
	}

	return nil
}

// SetOptimalParams sets the optimal parameters JSON for a clip.
func (s *ClipStore) SetOptimalParams(clipID string, paramsJSON json.RawMessage) error {
	clip, err := s.GetClip(clipID)
	if err != nil {
		return err
	}

	clip.OptimalParamsJSON = paramsJSON
	return s.UpdateClip(clip)
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
