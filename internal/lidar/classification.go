package lidar

import "github.com/banshee-data/velocity.report/internal/lidar/l6objects"

// Type aliases for classification types migrated to l6objects.
type ObjectClass = l6objects.ObjectClass
type ClassificationResult = l6objects.ClassificationResult
type ClassificationFeatures = l6objects.ClassificationFeatures
type TrackClassifier = l6objects.TrackClassifier

// Constants re-exported from l6objects.
const (
ClassPedestrian = l6objects.ClassPedestrian
ClassCar        = l6objects.ClassCar
ClassBird       = l6objects.ClassBird
ClassOther      = l6objects.ClassOther
)

// Classification thresholds (configurable for tuning)
const (
// Height thresholds (meters)
BirdHeightMax       = l6objects.BirdHeightMax
PedestrianHeightMin = l6objects.PedestrianHeightMin
PedestrianHeightMax = l6objects.PedestrianHeightMax
VehicleHeightMin    = l6objects.VehicleHeightMin
VehicleLengthMin    = l6objects.VehicleLengthMin
VehicleWidthMin     = l6objects.VehicleWidthMin

// Speed thresholds (m/s)
BirdSpeedMax       = l6objects.BirdSpeedMax
PedestrianSpeedMax = l6objects.PedestrianSpeedMax
VehicleSpeedMin    = l6objects.VehicleSpeedMin
StationarySpeedMax = l6objects.StationarySpeedMax

// Confidence levels
HighConfidence   = l6objects.HighConfidence
MediumConfidence = l6objects.MediumConfidence
LowConfidence    = l6objects.LowConfidence
)

// Function aliases re-exported from l6objects.
var (
NewTrackClassifier                    = l6objects.NewTrackClassifier
NewTrackClassifierWithMinObservations = l6objects.NewTrackClassifierWithMinObservations
ComputeSpeedPercentiles               = l6objects.ComputeSpeedPercentiles
)
