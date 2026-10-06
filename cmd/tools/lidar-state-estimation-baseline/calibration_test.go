package main

import (
	"math"
	"testing"
)

// rotationBlock extracts the 3x3 rotation from a row-major 4x4 homogeneous
// Transform.
func rotationBlock(transform [16]float64) [3][3]float64 {
	var r [3][3]float64
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			r[row][col] = transform[row*4+col]
		}
	}
	return r
}

func determinant3x3(m [3][3]float64) float64 {
	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}

// applyRotation returns R * v for a row-major 3x3 rotation.
func applyRotation(r [3][3]float64, v [3]float64) [3]float64 {
	var out [3]float64
	for row := 0; row < 3; row++ {
		out[row] = r[row][0]*v[0] + r[row][1]*v[1] + r[row][2]*v[2]
	}
	return out
}

func almostEqualVec(a, b [3]float64, tol float64) bool {
	return math.Abs(a[0]-b[0]) <= tol && math.Abs(a[1]-b[1]) <= tol && math.Abs(a[2]-b[2]) <= tol
}

func TestSiteCalibrationIsAProperRotationAtEveryBearing(t *testing.T) {
	for _, bearing := range []float64{0, 37, 90, 131, 180, 231, 270, 359} {
		cal := siteCalibration("hesai-pandar40p", bearing)
		r := rotationBlock(cal.Transform)

		// Orthonormal: R * R^T = I.
		for row := 0; row < 3; row++ {
			for col := 0; col < 3; col++ {
				var dot float64
				for k := 0; k < 3; k++ {
					dot += r[row][k] * r[col][k]
				}
				want := 0.0
				if row == col {
					want = 1.0
				}
				if math.Abs(dot-want) > 1e-9 {
					t.Fatalf("bearing=%.0f: R*R^T[%d][%d] = %f, want %f (not orthonormal)", bearing, row, col, dot, want)
				}
			}
		}
		// Proper rotation, not a reflection.
		if det := determinant3x3(r); math.Abs(det-1) > 1e-9 {
			t.Fatalf("bearing=%.0f: determinant = %f, want 1", bearing, det)
		}
		if cal.SensorID != "hesai-pandar40p" || cal.FromFrame != "sensor" || cal.ToFrame != "site" {
			t.Fatalf("bearing=%.0f: calibration = %+v, want sensor/site frame labels preserved", bearing, cal)
		}
	}
}

func TestSiteCalibrationMapsSensorForwardToTheStatedCompassDirection(t *testing.T) {
	// The documented convention: site frame is East-North-Up, and
	// northAzimuthDeg is the sensor's own +X (forward) axis as a compass
	// bearing. Verify the four cardinal cases directly against that claim
	// rather than against the trig derivation, so a sign error here would
	// actually fail.
	sensorForward := [3]float64{1, 0, 0}
	for _, tc := range []struct {
		bearing float64
		want    [3]float64 // expected site-frame direction
		label   string
	}{
		{0, [3]float64{0, 1, 0}, "north"},
		{90, [3]float64{1, 0, 0}, "east"},
		{180, [3]float64{0, -1, 0}, "south"},
		{270, [3]float64{-1, 0, 0}, "west"},
	} {
		cal := siteCalibration("hesai-pandar40p", tc.bearing)
		got := applyRotation(rotationBlock(cal.Transform), sensorForward)
		if !almostEqualVec(got, tc.want, 1e-9) {
			t.Errorf("bearing=%.0f (%s): sensor forward maps to %v, want %v", tc.bearing, tc.label, got, tc.want)
		}
	}
}

func TestSiteCalibrationVariesByBearingUnlikeTheIdentityPlaceholder(t *testing.T) {
	marina := siteCalibration("hesai-pandar40p", 231)
	columbus := siteCalibration("hesai-pandar40p", 35)
	if marina.Transform == columbus.Transform {
		t.Fatal("two sites with different measured bearings produced the same transform")
	}
	if marina.Transform == identityCalibration("hesai-pandar40p").Transform {
		t.Fatal("a measured, non-zero bearing produced the identity transform")
	}
}
