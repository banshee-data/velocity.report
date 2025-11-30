package lidar

import (
	"math"
	"testing"
)

func TestValidatePose_NilPose(t *testing.T) {
	result := ValidatePose(nil)

	if result.Valid {
		t.Error("expected invalid for nil pose")
	}
	if len(result.Issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(result.Issues))
	}
}

func TestValidatePose_ValidIdentity(t *testing.T) {
	pose := &Pose{
		T: [16]float64{
			1, 0, 0, 0,
			0, 1, 0, 0,
			0, 0, 1, 0,
			0, 0, 0, 1,
		},
		RootMeanSquareErrorMeters: 0.03, // Excellent quality
	}

	result := ValidatePose(pose)

	if !result.Valid {
		t.Error("expected valid pose")
	}
	if result.Quality != PoseQualityExcellent {
		t.Errorf("expected excellent quality, got %s", result.Quality)
	}
}

func TestValidatePose_QualityLevels(t *testing.T) {
	tests := []struct {
		rmse     float32
		expected PoseQuality
	}{
		{0.02, PoseQualityExcellent},
		{0.05, PoseQualityGood}, // At threshold, should be Good
		{0.10, PoseQualityGood},
		{0.15, PoseQualityFair}, // At threshold, should be Fair
		{0.25, PoseQualityFair},
		{0.35, PoseQualityPoor},
		{0.00, PoseQualityUnknown},
	}

	for _, tt := range tests {
		pose := &Pose{
			T: [16]float64{
				1, 0, 0, 0,
				0, 1, 0, 0,
				0, 0, 1, 0,
				0, 0, 0, 1,
			},
			RootMeanSquareErrorMeters: tt.rmse,
		}

		result := ValidatePose(pose)

		if result.Quality != tt.expected {
			t.Errorf("RMSE %.2f: expected quality %s, got %s", tt.rmse, tt.expected, result.Quality)
		}
	}
}

func TestValidatePose_InvalidRotation(t *testing.T) {
	// Invalid transform: determinant != 1
	pose := &Pose{
		T: [16]float64{
			2, 0, 0, 0, // Scale factor 2 makes det = 8
			0, 2, 0, 0,
			0, 0, 2, 0,
			0, 0, 0, 1,
		},
		RootMeanSquareErrorMeters: 0.03,
	}

	result := ValidatePose(pose)

	if result.Valid {
		t.Error("expected invalid for non-rigid transform")
	}
	if result.Quality != PoseQualityPoor {
		t.Errorf("expected poor quality for invalid matrix, got %s", result.Quality)
	}
}

func TestIsValidTransformMatrix(t *testing.T) {
	// Valid identity
	identity := [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	if !IsValidTransformMatrix(identity) {
		t.Error("identity should be valid")
	}

	// Valid rotation (90° around Z)
	rotZ90 := [16]float64{0, -1, 0, 0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	if !IsValidTransformMatrix(rotZ90) {
		t.Error("90° Z rotation should be valid")
	}

	// Invalid: bad last row
	badLastRow := [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 1, 0, 0, 1}
	if IsValidTransformMatrix(badLastRow) {
		t.Error("bad last row should be invalid")
	}

	// Invalid: reflection (det = -1)
	reflection := [16]float64{-1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	if IsValidTransformMatrix(reflection) {
		t.Error("reflection should be invalid")
	}
}

func TestIsPoseUsableForTracking(t *testing.T) {
	tests := []struct {
		quality  PoseQuality
		expected bool
	}{
		{PoseQualityExcellent, true},
		{PoseQualityGood, true},
		{PoseQualityFair, true},
		{PoseQualityPoor, false},
		{PoseQualityUnknown, true},
	}

	for _, tt := range tests {
		result := PoseValidationResult{Valid: true, Quality: tt.quality}
		if IsPoseUsableForTracking(result) != tt.expected {
			t.Errorf("quality %s: expected usable=%v", tt.quality, tt.expected)
		}
	}
}

func TestIsPoseUsableForTraining(t *testing.T) {
	tests := []struct {
		quality  PoseQuality
		expected bool
	}{
		{PoseQualityExcellent, true},
		{PoseQualityGood, true},
		{PoseQualityFair, false},
		{PoseQualityPoor, false},
		{PoseQualityUnknown, false},
	}

	for _, tt := range tests {
		result := PoseValidationResult{Valid: true, Quality: tt.quality}
		if IsPoseUsableForTraining(result) != tt.expected {
			t.Errorf("quality %s: expected usable=%v", tt.quality, tt.expected)
		}
	}
}

func TestTransformToWorldWithValidation_GoodPose(t *testing.T) {
	pose := &Pose{
		T: [16]float64{
			1, 0, 0, 0,
			0, 1, 0, 0,
			0, 0, 1, 0,
			0, 0, 0, 1,
		},
		RootMeanSquareErrorMeters: 0.03,
	}

	points := []PointPolar{{Distance: 10, Azimuth: 0, Elevation: 0}}

	// Should work for both tracking and training
	world, err := TransformToWorldWithValidation(points, pose, "test", false)
	if err != nil {
		t.Errorf("unexpected error for tracking: %v", err)
	}
	if len(world) != 1 {
		t.Errorf("expected 1 world point, got %d", len(world))
	}

	world, err = TransformToWorldWithValidation(points, pose, "test", true)
	if err != nil {
		t.Errorf("unexpected error for training: %v", err)
	}
	if len(world) != 1 {
		t.Errorf("expected 1 world point, got %d", len(world))
	}
}

func TestTransformToWorldWithValidation_FairPose(t *testing.T) {
	pose := &Pose{
		T: [16]float64{
			1, 0, 0, 0,
			0, 1, 0, 0,
			0, 0, 1, 0,
			0, 0, 0, 1,
		},
		RootMeanSquareErrorMeters: 0.20, // Fair quality
	}

	points := []PointPolar{{Distance: 10, Azimuth: 0, Elevation: 0}}

	// Should work for tracking
	world, err := TransformToWorldWithValidation(points, pose, "test", false)
	if err != nil {
		t.Errorf("unexpected error for tracking: %v", err)
	}
	if len(world) != 1 {
		t.Errorf("expected 1 world point, got %d", len(world))
	}

	// Should fail for training
	_, err = TransformToWorldWithValidation(points, pose, "test", true)
	if err == nil {
		t.Error("expected error for training with fair quality pose")
	}
}

func TestPoseQuality_String(t *testing.T) {
	tests := []struct {
		quality PoseQuality
		want    string
	}{
		{PoseQualityExcellent, "excellent (RMSE < 0.05m)"},
		{PoseQualityGood, "good (RMSE 0.05-0.15m)"},
		{PoseQualityFair, "fair (RMSE 0.15-0.30m)"},
		{PoseQualityPoor, "poor (RMSE > 0.30m)"},
		{PoseQualityUnknown, "unknown (RMSE not computed)"},
	}

	for _, tt := range tests {
		if got := tt.quality.String(); got != tt.want {
			t.Errorf("PoseQuality.String() = %v, want %v", got, tt.want)
		}
	}
}

func TestIsValidTransformMatrix_WithTranslation(t *testing.T) {
	// Valid rotation with translation
	validWithTranslation := [16]float64{
		1, 0, 0, 10, // Translation X=10
		0, 1, 0, 20, // Translation Y=20
		0, 0, 1, 5, // Translation Z=5
		0, 0, 0, 1,
	}
	if !IsValidTransformMatrix(validWithTranslation) {
		t.Error("valid transform with translation should be valid")
	}
}

func TestIsValidTransformMatrix_RotationAroundY(t *testing.T) {
	// 90° rotation around Y axis
	angle := math.Pi / 2
	cosA := math.Cos(angle)
	sinA := math.Sin(angle)

	rotY90 := [16]float64{
		cosA, 0, sinA, 0,
		0, 1, 0, 0,
		-sinA, 0, cosA, 0,
		0, 0, 0, 1,
	}
	if !IsValidTransformMatrix(rotY90) {
		t.Error("90° Y rotation should be valid")
	}
}
