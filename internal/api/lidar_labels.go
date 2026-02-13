package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// maxLabelsPerQuery is the maximum number of labels returned by list queries.
// This prevents excessive memory usage and response sizes for large datasets.
// Clients can use time-based filtering for pagination.
const maxLabelsPerQuery = 1000

// Valid user labels for track classification (what is the object?)
var validUserLabels = map[string]bool{
	"car":   true,
	"ped":   true,
	"noise": true,
}

// Valid quality flags for track quality attributes (multi-select, comma-separated).
// These describe properties of the track rather than what the object is.
var validQualityLabels = map[string]bool{
	"good":            true,
	"noisy":           true,
	"jitter_velocity": true,
	"merge":           true,
	"split":           true,
	"truncated":       true,
	"disconnected":    true,
}

// ValidateUserLabel checks if a user label is valid according to the enum.
// Returns false for empty strings (not in the valid map).
// Note: Empty strings may still be acceptable as optional values in the database,
// but they are not considered valid enum values.
func ValidateUserLabel(label string) bool {
	return validUserLabels[label]
}

// ValidateQualityLabel checks if a quality label string is valid.
// Supports both single labels and comma-separated multi-select flags.
// Returns false for empty strings.
func ValidateQualityLabel(label string) bool {
	if label == "" {
		return false
	}
	// Support comma-separated flags for multi-select
	parts := strings.Split(label, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || !validQualityLabels[part] {
			return false
		}
	}
	return true
}

// LidarLabel represents a manual label applied to a track for training/validation.
type LidarLabel struct {
	LabelID          string   `json:"label_id"`
	TrackID          string   `json:"track_id"`
	ClassLabel       string   `json:"class_label"`
	StartTimestampNs int64    `json:"start_timestamp_ns"`
	EndTimestampNs   *int64   `json:"end_timestamp_ns,omitempty"`
	Confidence       *float32 `json:"confidence,omitempty"`
	CreatedBy        *string  `json:"created_by,omitempty"`
	CreatedAtNs      int64    `json:"created_at_ns"`
	UpdatedAtNs      *int64   `json:"updated_at_ns,omitempty"`
	Notes            *string  `json:"notes,omitempty"`
	SceneID          *string  `json:"scene_id,omitempty"`
	SourceFile       *string  `json:"source_file,omitempty"`
}

// LidarLabelAPI provides HTTP handlers for label management.
type LidarLabelAPI struct {
	db *sql.DB
}

// NewLidarLabelAPI creates a new label API instance.
func NewLidarLabelAPI(db *sql.DB) *LidarLabelAPI {
	return &LidarLabelAPI{db: db}
}

// RegisterRoutes registers label API routes on the provided mux.
func (api *LidarLabelAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/lidar/labels", api.handleLabels)
	mux.HandleFunc("/api/lidar/labels/export", api.handleExport)
	mux.HandleFunc("/api/lidar/labels/", api.handleLabelByID)
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
	trackID := query.Get("track_id")
	classLabel := query.Get("class_label")
	startNs := query.Get("start_ns")
	endNs := query.Get("end_ns")

	// Build query with filters
	sqlQuery := `SELECT label_id, track_id, class_label, start_timestamp_ns,
	               end_timestamp_ns, confidence, created_by, created_at_ns,
	               updated_at_ns, notes, scene_id, source_file
	        FROM lidar_labels WHERE 1=1`
	args := []interface{}{}

	if trackID != "" {
		sqlQuery += " AND track_id = ?"
		args = append(args, trackID)
	}

	if classLabel != "" {
		sqlQuery += " AND class_label = ?"
		args = append(args, classLabel)
	}

	if startNs != "" {
		sqlQuery += " AND start_timestamp_ns >= ?"
		args = append(args, startNs)
	}

	if endNs != "" {
		sqlQuery += " AND (end_timestamp_ns IS NULL OR end_timestamp_ns <= ?)"
		args = append(args, endNs)
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
		if err := rows.Scan(
			&label.LabelID,
			&label.TrackID,
			&label.ClassLabel,
			&label.StartTimestampNs,
			&label.EndTimestampNs,
			&label.Confidence,
			&label.CreatedBy,
			&label.CreatedAtNs,
			&label.UpdatedAtNs,
			&label.Notes,
			&label.SceneID,
			&label.SourceFile,
		); err != nil {
			api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("scan failed: %v", err))
			return
		}
		labels = append(labels, label)
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

	// Validate required fields
	if label.TrackID == "" {
		api.writeJSONError(w, http.StatusBadRequest, "track_id is required")
		return
	}
	if label.ClassLabel == "" {
		api.writeJSONError(w, http.StatusBadRequest, "class_label is required")
		return
	}
	if label.StartTimestampNs == 0 {
		api.writeJSONError(w, http.StatusBadRequest, "start_timestamp_ns is required")
		return
	}

	// Generate label_id if not provided
	if label.LabelID == "" {
		label.LabelID = uuid.New().String()
	}

	// Set created_at_ns if not provided
	if label.CreatedAtNs == 0 {
		label.CreatedAtNs = time.Now().UnixNano()
	}

	// Insert into database
	query := `INSERT INTO lidar_labels (
		label_id, track_id, class_label, start_timestamp_ns, end_timestamp_ns,
		confidence, created_by, created_at_ns, updated_at_ns, notes, scene_id, source_file
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := api.db.Exec(query,
		label.LabelID,
		label.TrackID,
		label.ClassLabel,
		label.StartTimestampNs,
		label.EndTimestampNs,
		label.Confidence,
		label.CreatedBy,
		label.CreatedAtNs,
		label.UpdatedAtNs,
		label.Notes,
		label.SceneID,
		label.SourceFile,
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
	// Extract label ID from URL path
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
	query := `SELECT label_id, track_id, class_label, start_timestamp_ns,
	                 end_timestamp_ns, confidence, created_by, created_at_ns,
	                 updated_at_ns, notes, scene_id, source_file
	          FROM lidar_labels WHERE label_id = ?`

	var label LidarLabel
	err := api.db.QueryRow(query, labelID).Scan(
		&label.LabelID,
		&label.TrackID,
		&label.ClassLabel,
		&label.StartTimestampNs,
		&label.EndTimestampNs,
		&label.Confidence,
		&label.CreatedBy,
		&label.CreatedAtNs,
		&label.UpdatedAtNs,
		&label.Notes,
		&label.SceneID,
		&label.SourceFile,
	)
	if err == sql.ErrNoRows {
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
	var updates LidarLabel
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		api.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	// Check if label exists
	var exists bool
	err := api.db.QueryRow("SELECT 1 FROM lidar_labels WHERE label_id = ?", labelID).Scan(&exists)
	if err == sql.ErrNoRows {
		api.writeJSONError(w, http.StatusNotFound, "label not found")
		return
	}
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("query failed: %v", err))
		return
	}

	// Set updated_at_ns
	nowNs := time.Now().UnixNano()
	updates.UpdatedAtNs = &nowNs

	// Build dynamic UPDATE query based on provided fields
	// Only update fields that are explicitly provided (non-zero values)
	query := "UPDATE lidar_labels SET updated_at_ns = ?"
	args := []interface{}{updates.UpdatedAtNs}

	if updates.ClassLabel != "" {
		query += ", class_label = ?"
		args = append(args, updates.ClassLabel)
	}
	if updates.EndTimestampNs != nil {
		query += ", end_timestamp_ns = ?"
		args = append(args, updates.EndTimestampNs)
	}
	if updates.Confidence != nil {
		query += ", confidence = ?"
		args = append(args, updates.Confidence)
	}
	if updates.Notes != nil {
		query += ", notes = ?"
		args = append(args, updates.Notes)
	}
	if updates.SceneID != nil {
		query += ", scene_id = ?"
		args = append(args, updates.SceneID)
	}
	if updates.SourceFile != nil {
		query += ", source_file = ?"
		args = append(args, updates.SourceFile)
	}

	query += " WHERE label_id = ?"
	args = append(args, labelID)

	_, err = api.db.Exec(query, args...)
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("update failed: %v", err))
		return
	}

	// Fetch and return updated label
	api.handleGetLabel(w, r, labelID)
}

// handleDeleteLabel deletes a label.
func (api *LidarLabelAPI) handleDeleteLabel(w http.ResponseWriter, r *http.Request, labelID string) {
	result, err := api.db.Exec("DELETE FROM lidar_labels WHERE label_id = ?", labelID)
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

	query := `SELECT label_id, track_id, class_label, start_timestamp_ns,
	                 end_timestamp_ns, confidence, created_by, created_at_ns,
	                 updated_at_ns, notes, scene_id, source_file
	          FROM lidar_labels
	          ORDER BY start_timestamp_ns ASC`

	rows, err := api.db.Query(query)
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("query failed: %v", err))
		return
	}
	defer rows.Close()

	labels := []LidarLabel{}
	for rows.Next() {
		var label LidarLabel
		if err := rows.Scan(
			&label.LabelID,
			&label.TrackID,
			&label.ClassLabel,
			&label.StartTimestampNs,
			&label.EndTimestampNs,
			&label.Confidence,
			&label.CreatedBy,
			&label.CreatedAtNs,
			&label.UpdatedAtNs,
			&label.Notes,
			&label.SceneID,
			&label.SourceFile,
		); err != nil {
			api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("scan failed: %v", err))
			return
		}
		labels = append(labels, label)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=lidar_labels_export.json")
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
