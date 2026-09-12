// Package l4bobserve owns track-independent detection evidence. It does not
// select object faces, correct positions, or assign detections to tracks.
package l4bobserve

import (
	"fmt"
	"slices"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// Record is the initial evidence boundary, not yet a SQLite storage schema.
// Positions are site-frame metres; capture and frame timestamps are signed UTC
// Unix nanoseconds. Cluster.TSUnixNanos is L4's first member acquisition time,
// not the frame start or a fitted object's effective measurement time.
type Record struct {
	SchemaVersion  int                       `json:"schema_version"`
	ObservationID  string                    `json:"observation_id"`
	SourceID       string                    `json:"source_id"`      // Immutable capture/run identity
	CalibrationID  string                    `json:"calibration_id"` // Immutable site-to-sensor transform identity
	FrameUnixNanos int64                     `json:"frame_unix_nanos"`
	Cluster        l4perception.WorldCluster `json:"raw_cluster"`
	Primitives     Primitives                `json:"primitives"`
}

// Primitives are track-independent candidates fitted from the retained sample.
// They deliberately carry no near/far or leading/trailing labels: those need a
// predicted track pose and belong to a later MeasurementInterpretation.
type Primitives struct {
	Planes []Plane `json:"planes,omitempty"`
	Edges  []Edge  `json:"edges,omitempty"`
}

// Plane is a candidate local plane in the site frame. Normal is unit length
// when the candidate is valid; Support is the retained-point count used to fit it.
type Plane struct {
	Normal  [3]float64 `json:"normal"`
	Offset  float64    `json:"offset"`
	Support int        `json:"support"`
}

// Edge is a track-independent line segment in the site frame.
type Edge struct {
	Start   [3]float64 `json:"start"`
	End     [3]float64 `json:"end"`
	Support int        `json:"support"`
}

// DetectionObservation owns a frozen copy of L4 output. Neither the input nor
// a returned Snapshot aliases its slices or optional geometry. There are no
// track, prediction, interpreted-face, or measurement-covariance fields.
type DetectionObservation struct{ record Record }

// New freezes a detection without changing the current tracker's medoid,
// heading, or frame-start update timing. Identity must be supplied by the
// source owner; a cluster ID alone is only unique within a frame.
func New(r Record) (DetectionObservation, error) {
	if r.SchemaVersion != 1 || r.ObservationID == "" || r.SourceID == "" || r.CalibrationID == "" || r.Cluster.SensorID == "" || r.Cluster.FrameID == "" {
		return DetectionObservation{}, fmt.Errorf("schema version 1 plus observation, source, calibration, sensor, and coordinate-frame identities are required")
	}
	if r.Cluster.PointsCount < len(r.Cluster.RetainedPoints) || len(r.Cluster.RetainedPoints) > 1024 || len(r.Cluster.SamplePoints) > 1024 {
		return DetectionObservation{}, fmt.Errorf("retained evidence exceeds cluster population or the 1024-point cap")
	}
	return DetectionObservation{record: clone(r)}, nil
}

// Snapshot returns an owned copy suitable for a future write-once store or
// replay adapter. Mutating that copy cannot revise the original observation.
func (o DetectionObservation) Snapshot() Record { return clone(o.record) }

func clone(r Record) Record {
	r.Cluster.SamplePoints = slices.Clone(r.Cluster.SamplePoints)
	r.Cluster.RetainedPoints = slices.Clone(r.Cluster.RetainedPoints)
	r.Primitives.Planes = slices.Clone(r.Primitives.Planes)
	r.Primitives.Edges = slices.Clone(r.Primitives.Edges)
	if r.Cluster.OBB != nil {
		v := *r.Cluster.OBB
		r.Cluster.OBB = &v
	}
	if r.Cluster.SensorRingHint != nil {
		v := *r.Cluster.SensorRingHint
		r.Cluster.SensorRingHint = &v
	}
	if r.Cluster.SensorAzDegHint != nil {
		v := *r.Cluster.SensorAzDegHint
		r.Cluster.SensorAzDegHint = &v
	}
	return r
}
