package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/google/uuid"
)

// maxLabelsPerQuery is the maximum number of labels returned by list queries.
// This prevents excessive memory usage and response sizes for large datasets.
// Clients can use time-based filtering for pagination.
const maxLabelsPerQuery = 1000

// ObjectClass is a human-assigned classification label (what is the object?).
// Must stay in sync with l6objects.ObjectClass constants, Svelte ObjectClass,
// and Swift classificationLabels.
type ObjectClass string

// v0.5.0 ships 7 active object classes.
// Truck and motorcyclist are reserved for future use (v0.6+).
const (
	LabelCar        ObjectClass = "car"
	LabelBus        ObjectClass = "bus"
	LabelPedestrian ObjectClass = "pedestrian"
	LabelCyclist    ObjectClass = "cyclist"
	LabelBird       ObjectClass = "bird"
	LabelNoise      ObjectClass = "noise"
	LabelDynamic    ObjectClass = "dynamic"
)

// AllObjectClasses is the canonical list of valid object classes.
// Validation maps and doc-generation scripts should derive from this slice.
var AllObjectClasses = []ObjectClass{
	LabelCar, LabelBus, LabelPedestrian, LabelCyclist,
	LabelBird, LabelNoise, LabelDynamic,
}

// QualityFlag is a track quality attribute (multi-select, comma-separated).
// Describes properties of the track rather than what the object is.
type QualityFlag string

const (
	QualityGood           QualityFlag = "good"
	QualityNoisy          QualityFlag = "noisy"
	QualityJitterVelocity QualityFlag = "jitter_velocity"
	QualityJitterHeading  QualityFlag = "jitter_heading"
	QualityMerge          QualityFlag = "merge"
	QualitySplit          QualityFlag = "split"
	QualityTruncated      QualityFlag = "truncated"
	QualityDisconnected   QualityFlag = "disconnected"
)

// AllQualityFlags is the canonical list of valid quality flags.
var AllQualityFlags = []QualityFlag{
	QualityGood, QualityNoisy, QualityJitterVelocity, QualityJitterHeading,
	QualityMerge, QualitySplit, QualityTruncated, QualityDisconnected,
}

// validObjectClasses is derived from AllObjectClasses for O(1) lookup.
var validObjectClasses = func() map[string]bool {
	m := make(map[string]bool, len(AllObjectClasses))
	for _, l := range AllObjectClasses {
		m[string(l)] = true
	}
	return m
}()

// validQualityFlags is derived from AllQualityFlags for O(1) lookup.
var validQualityFlags = func() map[string]bool {
	m := make(map[string]bool, len(AllQualityFlags))
	for _, f := range AllQualityFlags {
		m[string(f)] = true
	}
	return m
}()

// ValidateObjectClass checks if a user label is valid according to the enum.
// Returns false for empty strings (not in the valid map).
// Note: Empty strings may still be acceptable as optional values in the database,
// but they are not considered valid enum values.
func ValidateObjectClass(label string) bool {
	return validObjectClasses[label]
}

// ValidateQualityFlags checks if a quality flag string is valid.
// Supports both single labels and comma-separated multi-select flags.
// Returns false for empty strings.
func ValidateQualityFlags(label string) bool {
	if label == "" {
		return false
	}
	parts := strings.Split(label, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || !validQualityFlags[part] {
			return false
		}
	}
	return true
}

// LidarLabel is a clip-owned track verdict.
// The label API and annotation-pack API are separate surfaces. Storage
// consolidation remains Phase 2; labels currently live in lidar_replay_annotations.
type LidarLabel struct {
	LabelID          string   `json:"label_id"`
	ClipID           *string  `json:"clip_id,omitempty"`
	RunID            *string  `json:"run_id,omitempty"`
	TrackID          string   `json:"track_id"`
	ClassLabel       string   `json:"class_label"`
	StartTimestampNs int64    `json:"start_timestamp_ns"`
	EndTimestampNs   *int64   `json:"end_timestamp_ns,omitempty"`
	Confidence       *float32 `json:"confidence,omitempty"`
	CreatedBy        *string  `json:"created_by,omitempty"`
	CreatedAtNs      int64    `json:"created_at_ns"`
	UpdatedAtNs      *int64   `json:"updated_at_ns,omitempty"`
	Notes            *string  `json:"notes,omitempty"`
	SourceFile       *string  `json:"source_file,omitempty"`
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

// LidarLabelAPI provides HTTP handlers for label management.
type LidarLabelAPI struct {
	db sqlite.DBClient
}

// NewLidarLabelAPI creates a new label API instance.
func NewLidarLabelAPI(db sqlite.DBClient) *LidarLabelAPI {
	return &LidarLabelAPI{db: db}
}

// RegisterRoutes registers label API routes on the provided mux.
func (api *LidarLabelAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/lidar/labels", api.handleLabels)
	mux.HandleFunc("/api/lidar/labels/export", api.handleExport)
	mux.HandleFunc("/api/lidar/labels/", api.handleLabelByID)
}

func trimOptionalStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func nullableTrimmedString(value *string) interface{} {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullableTrackID(trackID string) interface{} {
	trackID = strings.TrimSpace(trackID)
	if trackID == "" {
		return nil
	}
	return trackID
}

func nullableInt64Ptr(value *int64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func nullableFloat32Ptr(value *float32) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func parseOptionalInt64QueryParam(rawValue, field string) (*int64, error) {
	if strings.TrimSpace(rawValue) == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(strings.TrimSpace(rawValue), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%s must be a valid int64", field)
	}
	return &value, nil
}

func scanLidarLabel(scanner rowScanner, label *LidarLabel) error {
	var (
		clipID       *string
		runID        *string
		trackID      *string
		endTimestamp *int64
		confidence   *float64
		createdBy    *string
		updatedAtNs  *int64
		notes        *string
		sourceFile   *string
	)

	err := scanner.Scan(
		&label.LabelID,
		&clipID,
		&runID,
		&trackID,
		&label.ClassLabel,
		&label.StartTimestampNs,
		&endTimestamp,
		&confidence,
		&createdBy,
		&label.CreatedAtNs,
		&updatedAtNs,
		&notes,
		&sourceFile,
	)
	if err != nil {
		if err == sqlite.ErrNotFound {
			return sqlite.ErrNotFound
		}
		return err
	}

	label.ClipID = clipID
	label.RunID = runID
	label.TrackID = ""
	if trackID != nil {
		label.TrackID = *trackID
	}

	label.EndTimestampNs = endTimestamp
	label.Confidence = nil
	if confidence != nil {
		value := float32(*confidence)
		label.Confidence = &value
	}

	label.CreatedBy = createdBy
	label.UpdatedAtNs = updatedAtNs
	label.Notes = notes
	label.SourceFile = sourceFile

	return nil
}

func (api *LidarLabelAPI) getLabel(labelID string) (*LidarLabel, error) {
	query := `SELECT annotation_id, clip_id, run_id, track_id, class_label,
	                 start_timestamp_ns, end_timestamp_ns, confidence, created_by,
	                 created_at_ns, updated_at_ns, notes, source_file
	          FROM lidar_replay_annotations WHERE annotation_id = ?`

	var label LidarLabel
	if err := scanLidarLabel(api.db.QueryRow(query, labelID), &label); err != nil {
		return nil, err
	}
	return &label, nil
}

func (api *LidarLabelAPI) ensureClipExists(clipID string) error {
	var exists int
	err := api.db.QueryRow("SELECT 1 FROM lidar_clips WHERE clip_id = ?", clipID).Scan(&exists)
	if err == sqlite.ErrNotFound {
		return sqlite.ErrNotFound
	}
	return err
}

func (api *LidarLabelAPI) ensureRunTrackExists(runID, trackID string) error {
	var exists int
	err := api.db.QueryRow(
		"SELECT 1 FROM lidar_run_tracks WHERE run_id = ? AND track_id = ?",
		runID,
		trackID,
	).Scan(&exists)
	if err == sqlite.ErrNotFound {
		return sqlite.ErrNotFound
	}
	return err
}

func validateLabelPayload(label *LidarLabel, requireClip bool, currentStartNs int64) error {
	label.ClassLabel = strings.TrimSpace(label.ClassLabel)
	label.TrackID = strings.TrimSpace(label.TrackID)
	label.RunID = trimOptionalStringPtr(label.RunID)
	label.ClipID = trimOptionalStringPtr(label.ClipID)
	label.CreatedBy = trimOptionalStringPtr(label.CreatedBy)
	label.Notes = trimOptionalStringPtr(label.Notes)
	label.SourceFile = trimOptionalStringPtr(label.SourceFile)

	if requireClip {
		if label.ClipID == nil || *label.ClipID == "" {
			return fmt.Errorf("clip_id is required")
		}
	}

	if label.ClassLabel == "" {
		return fmt.Errorf("class_label is required")
	}
	if !ValidateObjectClass(label.ClassLabel) {
		return fmt.Errorf("invalid class_label: %s", label.ClassLabel)
	}
	if currentStartNs == 0 && label.StartTimestampNs == 0 {
		return fmt.Errorf("start_timestamp_ns is required")
	}
	startNs := label.StartTimestampNs
	if startNs == 0 {
		startNs = currentStartNs
	}
	if label.EndTimestampNs != nil && *label.EndTimestampNs < startNs {
		return fmt.Errorf("end_timestamp_ns must be greater than or equal to start_timestamp_ns")
	}
	if label.Confidence != nil && (*label.Confidence < 0 || *label.Confidence > 1) {
		return fmt.Errorf("confidence must be between 0 and 1")
	}

	hasRunID := label.RunID != nil && *label.RunID != ""
	hasTrackID := label.TrackID != ""
	if hasRunID != hasTrackID {
		return fmt.Errorf("run_id and track_id must be provided together")
	}

	return nil
}

// handleLabels handles list and create operations.
func (api *LidarLabelAPI) handleLabels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		api.handleListLabels(w, r)
	case http.MethodPost:
		api.handleCreateLabel(w, r)
	default:
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleListLabels lists labels with optional filters.
func (api *LidarLabelAPI) handleListLabels(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if strings.TrimSpace(query.Get("session_id")) != "" {
		api.writeJSONError(w, http.StatusBadRequest, "session_id is no longer supported; use clip_id")
		return
	}
	trackID := strings.TrimSpace(query.Get("track_id"))
	runID := strings.TrimSpace(query.Get("run_id"))
	clipID, aliasErr := clipQueryID(query)
	if aliasErr != nil {
		api.writeJSONError(w, http.StatusBadRequest, aliasErr.Error())
		return
	}
	classLabel := strings.TrimSpace(query.Get("class_label"))
	startNs, err := parseOptionalInt64QueryParam(query.Get("start_ns"), "start_ns")
	if err != nil {
		api.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	endNs, err := parseOptionalInt64QueryParam(query.Get("end_ns"), "end_ns")
	if err != nil {
		api.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if trackID != "" && runID == "" {
		api.writeJSONError(w, http.StatusBadRequest, "run_id is required when track_id is provided")
		return
	}

	sqlQuery := `SELECT annotation_id, clip_id, run_id, track_id, class_label,
	               start_timestamp_ns, end_timestamp_ns, confidence, created_by,
	               created_at_ns, updated_at_ns, notes, source_file
	        FROM lidar_replay_annotations WHERE 1=1`
	args := []interface{}{}

	if clipID != "" {
		sqlQuery += " AND clip_id = ?"
		args = append(args, clipID)
	}
	if runID != "" {
		sqlQuery += " AND run_id = ?"
		args = append(args, runID)
	}
	if trackID != "" {
		sqlQuery += " AND track_id = ?"
		args = append(args, trackID)
	}
	if classLabel != "" {
		sqlQuery += " AND class_label = ?"
		args = append(args, classLabel)
	}
	if startNs != nil {
		sqlQuery += " AND start_timestamp_ns >= ?"
		args = append(args, *startNs)
	}
	if endNs != nil {
		sqlQuery += " AND (end_timestamp_ns IS NULL OR end_timestamp_ns <= ?)"
		args = append(args, *endNs)
	}

	sqlQuery += fmt.Sprintf(" ORDER BY start_timestamp_ns DESC LIMIT %d", maxLabelsPerQuery)

	rows, err := api.db.Query(sqlQuery, args...)
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("query failed: %v", err))
		return
	}
	defer rows.Close()

	labels := []LidarLabel{}
	for rows.Next() {
		var label LidarLabel
		if err := scanLidarLabel(rows, &label); err != nil {
			api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("scan failed: %v", err))
			return
		}
		labels = append(labels, label)
	}
	if err := rows.Err(); err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("row iteration failed: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"labels": labels,
		"count":  len(labels),
	})
}

// handleCreateLabel creates a new label.
func (api *LidarLabelAPI) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	var label LidarLabel
	if err := json.NewDecoder(r.Body).Decode(&label); err != nil {
		api.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if err := validateLabelPayload(&label, true, 0); err != nil {
		api.writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := api.ensureClipExists(*label.ClipID); err == sqlite.ErrNotFound {
		api.writeJSONError(w, http.StatusBadRequest, "clip_id not found")
		return
	} else if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("clip lookup failed: %v", err))
		return
	}

	if label.RunID != nil {
		if err := api.ensureRunTrackExists(*label.RunID, label.TrackID); err == sqlite.ErrNotFound {
			api.writeJSONError(w, http.StatusBadRequest, "run_id and track_id must reference an existing run track")
			return
		} else if err != nil {
			api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("run track lookup failed: %v", err))
			return
		}
	}

	if label.LabelID == "" {
		label.LabelID = uuid.New().String()
	}
	if label.CreatedAtNs == 0 {
		label.CreatedAtNs = time.Now().UnixNano()
	}

	query := `INSERT INTO lidar_replay_annotations (
		annotation_id, clip_id, run_id, track_id, class_label,
		start_timestamp_ns, end_timestamp_ns, confidence, created_by,
		created_at_ns, updated_at_ns, notes, source_file
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := api.db.Exec(
		query,
		label.LabelID,
		*label.ClipID,
		nullableTrimmedString(label.RunID),
		nullableTrackID(label.TrackID),
		label.ClassLabel,
		label.StartTimestampNs,
		nullableInt64Ptr(label.EndTimestampNs),
		nullableFloat32Ptr(label.Confidence),
		nullableTrimmedString(label.CreatedBy),
		label.CreatedAtNs,
		nullableInt64Ptr(label.UpdatedAtNs),
		nullableTrimmedString(label.Notes),
		nullableTrimmedString(label.SourceFile),
	)
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("insert failed: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(label)
}

// handleLabelByID handles get, update, and delete operations for a specific label.
func (api *LidarLabelAPI) handleLabelByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/lidar/labels/")
	labelID := strings.TrimSpace(path)

	if labelID == "" || labelID == "export" {
		api.writeJSONError(w, http.StatusBadRequest, "label_id is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		api.handleGetLabel(w, r, labelID)
	case http.MethodPut:
		api.handleUpdateLabel(w, r, labelID)
	case http.MethodDelete:
		api.handleDeleteLabel(w, r, labelID)
	default:
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleGetLabel retrieves a specific label by ID.
func (api *LidarLabelAPI) handleGetLabel(w http.ResponseWriter, r *http.Request, labelID string) {
	label, err := api.getLabel(labelID)
	if err == sqlite.ErrNotFound {
		api.writeJSONError(w, http.StatusNotFound, "label not found")
		return
	}
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("query failed: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(label)
}

// handleUpdateLabel updates an existing label.
func (api *LidarLabelAPI) handleUpdateLabel(w http.ResponseWriter, r *http.Request, labelID string) {
	current, err := api.getLabel(labelID)
	if err == sqlite.ErrNotFound {
		api.writeJSONError(w, http.StatusNotFound, "label not found")
		return
	}
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("query failed: %v", err))
		return
	}
	var updates LidarLabel
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		api.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	updates.ClassLabel = strings.TrimSpace(updates.ClassLabel)
	updates.ClipID = trimOptionalStringPtr(updates.ClipID)
	updates.Notes = trimOptionalStringPtr(updates.Notes)
	updates.SourceFile = trimOptionalStringPtr(updates.SourceFile)

	if updates.ClassLabel != "" && !ValidateObjectClass(updates.ClassLabel) {
		api.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid class_label: %s", updates.ClassLabel))
		return
	}
	if updates.EndTimestampNs != nil && *updates.EndTimestampNs < current.StartTimestampNs {
		api.writeJSONError(w, http.StatusBadRequest, "end_timestamp_ns must be greater than or equal to start_timestamp_ns")
		return
	}
	if updates.Confidence != nil && (*updates.Confidence < 0 || *updates.Confidence > 1) {
		api.writeJSONError(w, http.StatusBadRequest, "confidence must be between 0 and 1")
		return
	}
	if updates.ClipID != nil {
		if *updates.ClipID == "" {
			api.writeJSONError(w, http.StatusBadRequest, "clip_id cannot be empty")
			return
		}
		if err := api.ensureClipExists(*updates.ClipID); err == sqlite.ErrNotFound {
			api.writeJSONError(w, http.StatusBadRequest, "clip_id not found")
			return
		} else if err != nil {
			api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("clip lookup failed: %v", err))
			return
		}
	}

	nowNs := time.Now().UnixNano()
	updates.UpdatedAtNs = &nowNs

	query := "UPDATE lidar_replay_annotations SET updated_at_ns = ?"
	args := []interface{}{nowNs}

	if updates.ClassLabel != "" {
		query += ", class_label = ?"
		args = append(args, updates.ClassLabel)
	}
	if updates.EndTimestampNs != nil {
		query += ", end_timestamp_ns = ?"
		args = append(args, *updates.EndTimestampNs)
	}
	if updates.Confidence != nil {
		query += ", confidence = ?"
		args = append(args, *updates.Confidence)
	}
	if updates.Notes != nil {
		query += ", notes = ?"
		args = append(args, nullableTrimmedString(updates.Notes))
	}
	if updates.ClipID != nil {
		query += ", clip_id = ?"
		args = append(args, *updates.ClipID)
	}
	if updates.SourceFile != nil {
		query += ", source_file = ?"
		args = append(args, nullableTrimmedString(updates.SourceFile))
	}

	query += " WHERE annotation_id = ?"
	args = append(args, labelID)

	if _, err := api.db.Exec(query, args...); err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("update failed: %v", err))
		return
	}

	api.handleGetLabel(w, r, labelID)
}

// handleDeleteLabel deletes a label.
func (api *LidarLabelAPI) handleDeleteLabel(w http.ResponseWriter, r *http.Request, labelID string) {
	result, err := api.db.Exec("DELETE FROM lidar_replay_annotations WHERE annotation_id = ?", labelID)
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("delete failed: %v", err))
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		api.writeJSONError(w, http.StatusNotFound, "label not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "deleted",
		"label_id": labelID,
	})
}

// handleExport exports all labels as a JSON array.
func (api *LidarLabelAPI) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	params := r.URL.Query()
	if strings.TrimSpace(params.Get("session_id")) != "" {
		api.writeJSONError(w, http.StatusBadRequest, "session_id is no longer supported; use clip_id")
		return
	}
	clipID, aliasErr := clipQueryID(params)
	if aliasErr != nil {
		api.writeJSONError(w, http.StatusBadRequest, aliasErr.Error())
		return
	}

	query := `SELECT annotation_id, clip_id, run_id, track_id, class_label,
	                 start_timestamp_ns, end_timestamp_ns, confidence, created_by,
	                 created_at_ns, updated_at_ns, notes, source_file
	          FROM lidar_replay_annotations`
	args := []interface{}{}
	if clipID != "" {
		query += " WHERE clip_id = ?"
		args = append(args, clipID)
	}
	query += " ORDER BY start_timestamp_ns ASC"

	rows, err := api.db.Query(query, args...)
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("query failed: %v", err))
		return
	}
	defer rows.Close()

	labels := []LidarLabel{}
	for rows.Next() {
		var label LidarLabel
		if err := scanLidarLabel(rows, &label); err != nil {
			api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("scan failed: %v", err))
			return
		}
		labels = append(labels, label)
	}
	if err := rows.Err(); err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("row iteration failed: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=lidar_track_annotations_export.json")
	json.NewEncoder(w).Encode(labels)
}

// writeJSONError writes a JSON error response.
func (api *LidarLabelAPI) writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
	})
}

// MarshalJSON preserves the previous DMG's registered label response key.
func (label LidarLabel) MarshalJSON() ([]byte, error) {
	type payload LidarLabel
	return json.Marshal(struct {
		payload
		LegacyClipID *string `json:"replay_case_id,omitempty"`
	}{payload: payload(label), LegacyClipID: label.ClipID})
}

// UnmarshalJSON accepts the previous DMG's registered request key. Two
// conflicting identities are refused, so an alias cannot silently retarget a label.
func (label *LidarLabel) UnmarshalJSON(data []byte) error {
	type payload LidarLabel
	var decoded struct {
		payload
		LegacyClipID *string `json:"replay_case_id"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.ClipID != nil && decoded.LegacyClipID != nil && *decoded.ClipID != *decoded.LegacyClipID {
		return fmt.Errorf("clip_id conflicts with replay_case_id")
	}
	if decoded.ClipID == nil {
		decoded.ClipID = decoded.LegacyClipID
	}
	*label = LidarLabel(decoded.payload)
	return nil
}

func clipQueryID(query url.Values) (string, error) {
	canonical, legacy := strings.TrimSpace(query.Get("clip_id")), strings.TrimSpace(query.Get("replay_case_id"))
	if canonical != "" && legacy != "" && canonical != legacy {
		return "", fmt.Errorf("clip_id conflicts with replay_case_id")
	}
	if canonical == "" {
		return legacy, nil
	}
	return canonical, nil
}
