package l6objects

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Type aliases re-export classification types from the parent package.
// These aliases enable gradual migration: callers can import from
// l6objects while the implementation remains in internal/lidar.

// ObjectClass enumerates the possible object classifications.
type ObjectClass = lidar.ObjectClass

// ClassificationResult holds the outcome of classifying a tracked object.
type ClassificationResult = lidar.ClassificationResult

// ClassificationFeatures captures the features used for classification.
type ClassificationFeatures = lidar.ClassificationFeatures

// TrackClassifier determines the object class of a tracked object.
type TrackClassifier = lidar.TrackClassifier

// Constructor re-exports.

// NewTrackClassifier creates a TrackClassifier with default settings.
var NewTrackClassifier = lidar.NewTrackClassifier

// NewTrackClassifierWithMinObservations creates a TrackClassifier with
// a custom minimum observation threshold.
var NewTrackClassifierWithMinObservations = lidar.NewTrackClassifierWithMinObservations

// ComputeSpeedPercentiles calculates p50, p85, and p95 from a speed history.
var ComputeSpeedPercentiles = lidar.ComputeSpeedPercentiles
