package server

import (
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// TrackResponse represents a track in JSON API responses.
type TrackResponse struct {
	TrackID             string               `json:"track_id"`
	SensorID            string               `json:"sensor_id"`
	State               string               `json:"state"`
	Position            Position             `json:"position"`
	Velocity            Velocity             `json:"velocity"`
	SpeedMps            float32              `json:"speed_mps"`
	HeadingRad          float32              `json:"heading_rad"`
	ObjectClass         string               `json:"object_class,omitempty"`
	ObjectConfidence    float32              `json:"object_confidence,omitempty"`
	ClassificationModel string               `json:"classification_model,omitempty"`
	ObservationCount    int                  `json:"observation_count"`
	AgeSeconds          float64              `json:"age_seconds"`
	AvgSpeedMps         float32              `json:"avg_speed_mps"`
	MaxSpeedMps         float32              `json:"max_speed_mps"`
	BoundingBox         BBox                 `json:"bounding_box"`
	OBBHeadingRad       float32              `json:"obb_heading_rad"`
	HeadingSource       int                  `json:"heading_source,omitempty"` // 0=PCA, 1=velocity, 2=displacement, 3=locked
	FirstSeen           string               `json:"first_seen"`
	LastSeen            string               `json:"last_seen"`
	History             []TrackPointResponse `json:"history,omitempty"`
}

// TrackPointResponse represents a point in a track's history.
type TrackPointResponse struct {
	X         float32 `json:"x"`
	Y         float32 `json:"y"`
	Timestamp string  `json:"timestamp"`
}

// Position represents a 3D position in world coordinates.
type Position struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
}

// Velocity represents 2D velocity in world coordinates.
type Velocity struct {
	VX float32 `json:"vx"`
	VY float32 `json:"vy"`
}

// BBox represents bounding box dimensions for rendering.
// These are per-frame cluster dimensions (from DBSCAN OBB), not running averages.
type BBox struct {
	Length float32 `json:"length"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
}

// ClusterResponse represents a cluster in JSON API responses.
type ClusterResponse struct {
	ClusterID   int64    `json:"cluster_id"`
	SensorID    string   `json:"sensor_id"`
	Timestamp   string   `json:"timestamp"`
	Centroid    Position `json:"centroid"`
	BoundingBox struct {
		Length float32 `json:"length"`
		Width  float32 `json:"width"`
		Height float32 `json:"height"`
	} `json:"bounding_box"`
	PointsCount   int     `json:"points_count"`
	HeightP95     float32 `json:"height_p95"`
	IntensityMean float32 `json:"intensity_mean"`
}

// TracksListResponse is the JSON response for listing tracks.
type TracksListResponse struct {
	Tracks    []TrackResponse `json:"tracks"`
	Count     int             `json:"count"`
	Timestamp string          `json:"timestamp"`
}

// ClustersListResponse is the JSON response for listing clusters.
type ClustersListResponse struct {
	Clusters  []ClusterResponse `json:"clusters"`
	Count     int               `json:"count"`
	Timestamp string            `json:"timestamp"`
}

// ObservationsListResponse is the JSON response for observation overlays.
type ObservationsListResponse struct {
	Observations []ObservationResponse `json:"observations"`
	Count        int                   `json:"count"`
	Timestamp    string                `json:"timestamp"`
}

// ObservationResponse represents a raw observation for overlaying foreground objects.
type ObservationResponse struct {
	TrackID     string   `json:"track_id"`
	Timestamp   string   `json:"timestamp"`
	Position    Position `json:"position"`
	Velocity    Velocity `json:"velocity"`
	SpeedMps    float32  `json:"speed_mps"`
	HeadingRad  float32  `json:"heading_rad"`
	BoundingBox struct {
		Length float32 `json:"length"`
		Width  float32 `json:"width"`
		Height float32 `json:"height"`
	} `json:"bounding_box"`
	HeightP95     float32 `json:"height_p95"`
	IntensityMean float32 `json:"intensity_mean"`
}

// TrackSummaryResponse is the JSON response for track summary statistics.
type TrackSummaryResponse struct {
	SensorID  string                  `json:"sensor_id"`
	StartTime string                  `json:"start_time,omitempty"`
	EndTime   string                  `json:"end_time,omitempty"`
	ByClass   map[string]ClassSummary `json:"by_class"`
	ByState   map[string]int          `json:"by_state"`
	Overall   OverallSummary          `json:"overall"`
	Timestamp string                  `json:"timestamp"`
}

// ClassSummary is a type alias for l8analytics.TrackClassSummary.
type ClassSummary = l8analytics.TrackClassSummary

// OverallSummary is a type alias for l8analytics.TrackOverallSummary.
type OverallSummary = l8analytics.TrackOverallSummary

func (api *TrackAPI) writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// toDisplayFrame aligns stored track coordinates (sensor frame with azimuth 0 along +Y)
// to the UI's display frame (azimuth 0 along +X) by swapping X/Y.
func toDisplayFrame(x, y float32) (float32, float32) {
	return y, x
}

func headingFromVelocity(vx, vy float32) float32 {
	return float32(math.Atan2(float64(vy), float64(vx)))
}

func (api *TrackAPI) trackToResponse(track *l5tracks.TrackedObject) TrackResponse {
	first := track.FirstUnixNanos
	last := track.LastUnixNanos
	if last == 0 {
		last = track.FirstUnixNanos
	}
	if last < first {
		last = first
	}

	var spanSeconds float64
	if first > 0 && last > 0 {
		spanSeconds = float64(last-first) / 1e9
	}

	posX, posY := toDisplayFrame(track.X, track.Y)
	velX, velY := toDisplayFrame(track.VX, track.VY)
	speed := float32(math.Sqrt(float64(velX*velX + velY*velY)))
	heading := headingFromVelocity(velX, velY)

	history := make([]TrackPointResponse, 0, len(track.History))
	for _, p := range track.History {
		hx, hy := toDisplayFrame(p.X, p.Y)
		history = append(history, TrackPointResponse{
			X:         hx,
			Y:         hy,
			Timestamp: time.Unix(0, p.Timestamp).UTC().Format(time.RFC3339Nano),
		})
	}

	return TrackResponse{
		TrackID:  track.TrackID,
		SensorID: track.SensorID,
		State:    string(track.State),
		Position: Position{
			X: posX,
			Y: posY,
			// Z is 0 because TrackedObject uses a 2D Kalman filter tracking (x, y, vx, vy).
			// Height information is captured in bounding_box and height_p95 from cluster features.
			Z: 0,
		},
		Velocity: Velocity{
			VX: velX,
			VY: velY,
		},
		SpeedMps:            speed,
		HeadingRad:          heading,
		ObjectClass:         track.ObjectClass,
		ObjectConfidence:    track.ObjectConfidence,
		ClassificationModel: track.ClassificationModel,
		ObservationCount:    track.ObservationCount,
		AgeSeconds:          spanSeconds,
		AvgSpeedMps:         track.AvgSpeedMps,
		MaxSpeedMps:         track.MaxSpeedMps,
		BoundingBox:         bboxFromTrack(track),
		OBBHeadingRad:       track.OBBHeadingRad,
		HeadingSource:       int(track.HeadingSource),
		FirstSeen:           time.Unix(0, first).UTC().Format(time.RFC3339Nano),
		LastSeen:            time.Unix(0, last).UTC().Format(time.RFC3339Nano),
		History:             history,
	}
}

// bboxFromTrack returns a BBox populated from the best available dimensions.
// Per-frame cluster dims (OBBLength/Width/Height) are preferred; when they
// are zero (e.g. tracks loaded from DB where only the historical value is
// stored), BoundingBoxLengthAvg is used as fallback.
func bboxFromTrack(track *l5tracks.TrackedObject) BBox {
	l := track.OBBLength
	if l == 0 {
		l = track.BoundingBoxLengthAvg
	}
	w := track.OBBWidth
	if w == 0 {
		w = track.BoundingBoxWidthAvg
	}
	h := track.OBBHeight
	if h == 0 {
		h = track.BoundingBoxHeightAvg
	}
	return BBox{Length: l, Width: w, Height: h}
}
