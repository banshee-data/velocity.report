package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/google/uuid"
)

// maxLabelsPerQuery is the maximum number of labels returned by list queries.
// This prevents excessive memory usage and response sizes for large datasets.
// Clients can use time-based filtering for pagination.
const maxLabelsPerQuery = 1000

// DetectionLabel is a human-assigned classification label (what is the object?).
// Must stay in sync with l6objects.ObjectClass constants, Svelte DetectionLabel,
// and Swift classificationLabels.
type DetectionLabel string

// v0.5.0 ships 7 active detection labels.
// Truck and motorcyclist are reserved for future use (v0.6+).
const (
	LabelCar        DetectionLabel = "car"
	LabelBus        DetectionLabel = "bus"
	LabelPedestrian DetectionLabel = "pedestrian"
	LabelCyclist    DetectionLabel = "cyclist"
	LabelBird       DetectionLabel = "bird"
	LabelNoise      DetectionLabel = "noise"
	LabelDynamic    DetectionLabel = "dynamic"
)

// AllDetectionLabels is the canonical list of valid detection labels.
// Validation maps and doc-generation scripts should derive from this slice.
var AllDetectionLabels = []DetectionLabel{
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

// validUserLabels is derived from AllDetectionLabels for O(1) lookup.
var validUserLabels = func() map[string]bool {
	m := make(map[string]bool, len(AllDetectionLabels))
	for _, l := range AllDetectionLabels {
		m[string(l)] = true
	}
	return m
}()

// validQualityLabels is derived from AllQualityFlags for O(1) lookup.
var validQualityLabels = func() map[string]bool {
	m := make(map[string]bool, len(AllQualityFlags))
	for _, f := range AllQualityFlags {
		m[string(f)] = true
	}
	return m
}()

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

// LidarLabel is the storage-backed label payload exposed by the API.
type LidarLabel = sqlite.LidarLabel

type labelStore interface {
	ListLabels(filter sqlite.LabelFilter) ([]sqlite.LidarLabel, error)
	CreateLabel(label *sqlite.LidarLabel) error
	GetLabel(labelID string) (*sqlite.LidarLabel, error)
	UpdateLabel(labelID string, updates *sqlite.LidarLabel) error
	DeleteLabel(labelID string) error
	ExportLabels() ([]sqlite.LidarLabel, error)
}

// LidarLabelAPI provides HTTP handlers for label management.
type LidarLabelAPI struct {
	store labelStore
}

// NewLidarLabelAPI creates a new label API instance.
func NewLidarLabelAPI(db sqlite.DBClient) *LidarLabelAPI {
	return &LidarLabelAPI{store: sqlite.NewLabelStore(db)}
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
	labels, err := api.store.ListLabels(sqlite.LabelFilter{
		TrackID:          query.Get("track_id"),
		ClassLabel:       query.Get("class_label"),
		StartTimestampNs: query.Get("start_ns"),
		EndTimestampNs:   query.Get("end_ns"),
		Limit:            maxLabelsPerQuery,
	})
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("query failed: %v", err))
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

	if err := api.store.CreateLabel(&label); err != nil {
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
	label, err := api.store.GetLabel(labelID)
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
	var updates LidarLabel
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		api.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	// Set updated_at_ns
	nowNs := time.Now().UnixNano()
	updates.UpdatedAtNs = &nowNs

	err := api.store.UpdateLabel(labelID, &updates)
	if err == sqlite.ErrNotFound {
		api.writeJSONError(w, http.StatusNotFound, "label not found")
		return
	}
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("update failed: %v", err))
		return
	}

	// Fetch and return updated label
	api.handleGetLabel(w, r, labelID)
}

// handleDeleteLabel deletes a label.
func (api *LidarLabelAPI) handleDeleteLabel(w http.ResponseWriter, r *http.Request, labelID string) {
	err := api.store.DeleteLabel(labelID)
	if err == sqlite.ErrNotFound {
		api.writeJSONError(w, http.StatusNotFound, "label not found")
		return
	}
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("delete failed: %v", err))
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

	labels, err := api.store.ExportLabels()
	if err != nil {
		api.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("query failed: %v", err))
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
