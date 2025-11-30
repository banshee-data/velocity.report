package lidar

import (
	"fmt"
	"math"
)

// PoseQuality represents the assessed quality of a pose calibration.
type PoseQuality string

const (
	// PoseQualityExcellent indicates RMSE < 0.05m - excellent calibration
	PoseQualityExcellent PoseQuality = "excellent"
	// PoseQualityGood indicates RMSE 0.05-0.15m - good for tracking and training
	PoseQualityGood PoseQuality = "good"
	// PoseQualityFair indicates RMSE 0.15-0.30m - usable but consider recalibration
	PoseQualityFair PoseQuality = "fair"
	// PoseQualityPoor indicates RMSE > 0.30m - requires recalibration
	PoseQualityPoor PoseQuality = "poor"
	// PoseQualityUnknown indicates RMSE not computed
	PoseQualityUnknown PoseQuality = "unknown"
)

// Pose quality RMSE thresholds (meters)
const (
	RMSEThresholdExcellent = 0.05
	RMSEThresholdGood      = 0.15
	RMSEThresholdFair      = 0.30
	// MatrixValidationTolerance is the tolerance for checking rotation matrix validity
	MatrixValidationTolerance = 0.01
)

// PoseValidationResult contains the result of pose validation.
type PoseValidationResult struct {
	Valid   bool
	Quality PoseQuality
	Issues  []string
}

// ValidatePose checks if a pose is valid and returns its quality assessment.
// A pose is valid if it has a proper rigid transform matrix.
// Quality is assessed based on the RMSE (Root Mean Square Error) of the calibration.
func ValidatePose(pose *Pose) PoseValidationResult {
	result := PoseValidationResult{
		Valid:   false,
		Quality: PoseQualityUnknown,
		Issues:  make([]string, 0),
	}

	// Check for nil pose
	if pose == nil {
		result.Issues = append(result.Issues, "pose is nil")
		return result
	}

	// Check transform matrix validity
	if !IsValidTransformMatrix(pose.T) {
		result.Issues = append(result.Issues, "invalid transform matrix (not proper rigid transform)")
		result.Quality = PoseQualityPoor
		return result
	}

	// Assess quality based on RMSE
	rmse := pose.RootMeanSquareErrorMeters
	switch {
	case rmse == 0:
		result.Quality = PoseQualityUnknown
		result.Issues = append(result.Issues, "RMSE not computed - quality unknown")
	case rmse < RMSEThresholdExcellent:
		result.Quality = PoseQualityExcellent
	case rmse < RMSEThresholdGood:
		result.Quality = PoseQualityGood
	case rmse < RMSEThresholdFair:
		result.Quality = PoseQualityFair
		result.Issues = append(result.Issues, "pose quality is fair - consider recalibration")
	default:
		result.Quality = PoseQualityPoor
		result.Issues = append(result.Issues, "pose quality is poor - recalibration required")
	}

	// Valid if no blocking issues (poor quality is still technically valid for some uses)
	result.Valid = result.Quality != PoseQualityPoor || len(result.Issues) == 0

	return result
}

// IsValidTransformMatrix checks if a 4x4 matrix is a valid rigid transform.
// A valid rigid transform has:
// 1. Orthonormal rotation submatrix (det ≈ 1)
// 2. Last row is [0 0 0 1]
func IsValidTransformMatrix(T [16]float64) bool {
	// Extract 3x3 rotation submatrix (row-major layout)
	r00, r01, r02 := T[0], T[1], T[2]
	r10, r11, r12 := T[4], T[5], T[6]
	r20, r21, r22 := T[8], T[9], T[10]

	// Check determinant ≈ 1 (proper rotation, not reflection)
	det := r00*(r11*r22-r12*r21) - r01*(r10*r22-r12*r20) + r02*(r10*r21-r11*r20)
	if math.Abs(det-1.0) > MatrixValidationTolerance {
		return false
	}

	// Check last row is [0 0 0 1]
	if T[12] != 0 || T[13] != 0 || T[14] != 0 || math.Abs(T[15]-1.0) > 0.001 {
		return false
	}

	return true
}

// IsPoseUsableForTracking returns true if the pose quality is sufficient for tracking.
// Tracking can work with Fair quality poses but not Poor.
func IsPoseUsableForTracking(result PoseValidationResult) bool {
	return result.Valid && (result.Quality == PoseQualityExcellent ||
		result.Quality == PoseQualityGood ||
		result.Quality == PoseQualityFair ||
		result.Quality == PoseQualityUnknown) // Unknown is allowed with caution
}

// IsPoseUsableForTraining returns true if the pose quality is sufficient for ML training data.
// Only Excellent and Good quality poses should be used for training to ensure data quality.
func IsPoseUsableForTraining(result PoseValidationResult) bool {
	return result.Valid && (result.Quality == PoseQualityExcellent ||
		result.Quality == PoseQualityGood)
}

// String returns a human-readable description of the pose quality.
func (q PoseQuality) String() string {
	switch q {
	case PoseQualityExcellent:
		return "excellent (RMSE < 0.05m)"
	case PoseQualityGood:
		return "good (RMSE 0.05-0.15m)"
	case PoseQualityFair:
		return "fair (RMSE 0.15-0.30m)"
	case PoseQualityPoor:
		return "poor (RMSE > 0.30m)"
	case PoseQualityUnknown:
		return "unknown (RMSE not computed)"
	default:
		return string(q)
	}
}

// TransformToWorldWithValidation converts polar points to world frame with pose validation.
// Returns an error if the pose is invalid for the intended use.
// Set requireTrainingQuality to true for ML training data (stricter requirements).
func TransformToWorldWithValidation(polarPoints []PointPolar, pose *Pose, sensorID string, requireTrainingQuality bool) ([]WorldPoint, error) {
	validation := ValidatePose(pose)

	if requireTrainingQuality {
		if !IsPoseUsableForTraining(validation) {
			return nil, fmt.Errorf("pose quality %s insufficient for training data: %v",
				validation.Quality, validation.Issues)
		}
	} else {
		if !IsPoseUsableForTracking(validation) {
			return nil, fmt.Errorf("pose quality %s insufficient for tracking: %v",
				validation.Quality, validation.Issues)
		}
	}

	return TransformToWorld(polarPoints, pose, sensorID), nil
}
