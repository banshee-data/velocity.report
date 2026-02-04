// Package visualiser provides gRPC streaming of LiDAR perception data.
package visualiser

import (
	"math"
	"testing"
)

func TestNewSyntheticGenerator(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")

	if gen == nil {
		t.Fatal("expected non-nil SyntheticGenerator")
	}
	if gen.sensorID != "test-sensor" {
		t.Errorf("expected sensorID=test-sensor, got %s", gen.sensorID)
	}
	if gen.PointCount != 10000 {
		t.Errorf("expected PointCount=10000, got %d", gen.PointCount)
	}
	if gen.TrackCount != 10 {
		t.Errorf("expected TrackCount=10, got %d", gen.TrackCount)
	}
	if gen.FrameRate != 10.0 {
		t.Errorf("expected FrameRate=10.0, got %f", gen.FrameRate)
	}
	if gen.AreaRadius != 50.0 {
		t.Errorf("expected AreaRadius=50.0, got %f", gen.AreaRadius)
	}
	if gen.TrackRadius != 20.0 {
		t.Errorf("expected TrackRadius=20.0, got %f", gen.TrackRadius)
	}
	if gen.TrackSpeedMPS != 5.0 {
		t.Errorf("expected TrackSpeedMPS=5.0, got %f", gen.TrackSpeedMPS)
	}
	if gen.rng == nil {
		t.Error("expected non-nil rng")
	}
	if gen.startNs <= 0 {
		t.Error("expected positive startNs")
	}
}

func TestSyntheticGenerator_NextFrame(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")

	frame := gen.NextFrame()

	if frame == nil {
		t.Fatal("expected non-nil frame")
	}
	if frame.FrameID != 1 {
		t.Errorf("expected FrameID=1 for first frame, got %d", frame.FrameID)
	}
	if frame.SensorID != "test-sensor" {
		t.Errorf("expected SensorID=test-sensor, got %s", frame.SensorID)
	}
	if frame.TimestampNanos <= 0 {
		t.Error("expected positive TimestampNanos")
	}
}

func TestSyntheticGenerator_NextFrame_IncrementsFrameID(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")

	frame1 := gen.NextFrame()
	frame2 := gen.NextFrame()
	frame3 := gen.NextFrame()

	if frame1.FrameID != 1 {
		t.Errorf("expected first FrameID=1, got %d", frame1.FrameID)
	}
	if frame2.FrameID != 2 {
		t.Errorf("expected second FrameID=2, got %d", frame2.FrameID)
	}
	if frame3.FrameID != 3 {
		t.Errorf("expected third FrameID=3, got %d", frame3.FrameID)
	}
}

func TestSyntheticGenerator_CoordinateFrame(t *testing.T) {
	gen := NewSyntheticGenerator("hesai-01")

	frame := gen.NextFrame()

	if frame.CoordinateFrame.FrameID != "site/hesai-01" {
		t.Errorf("expected FrameID=site/hesai-01, got %s", frame.CoordinateFrame.FrameID)
	}
	if frame.CoordinateFrame.ReferenceFrame != "ENU" {
		t.Errorf("expected ReferenceFrame=ENU, got %s", frame.CoordinateFrame.ReferenceFrame)
	}
	if frame.CoordinateFrame.OriginLat != 51.5074 {
		t.Errorf("expected OriginLat=51.5074, got %f", frame.CoordinateFrame.OriginLat)
	}
	if frame.CoordinateFrame.OriginLon != -0.1278 {
		t.Errorf("expected OriginLon=-0.1278, got %f", frame.CoordinateFrame.OriginLon)
	}
}

func TestSyntheticGenerator_PointCloud(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")
	gen.PointCount = 1000 // Use smaller count for faster test

	frame := gen.NextFrame()

	if frame.PointCloud == nil {
		t.Fatal("expected non-nil PointCloud")
	}
	pc := frame.PointCloud

	if pc.PointCount != 1000 {
		t.Errorf("expected PointCount=1000, got %d", pc.PointCount)
	}
	if len(pc.X) != 1000 {
		t.Errorf("expected X length=1000, got %d", len(pc.X))
	}
	if len(pc.Y) != 1000 {
		t.Errorf("expected Y length=1000, got %d", len(pc.Y))
	}
	if len(pc.Z) != 1000 {
		t.Errorf("expected Z length=1000, got %d", len(pc.Z))
	}
	if len(pc.Intensity) != 1000 {
		t.Errorf("expected Intensity length=1000, got %d", len(pc.Intensity))
	}
	if len(pc.Classification) != 1000 {
		t.Errorf("expected Classification length=1000, got %d", len(pc.Classification))
	}
	if pc.SensorID != "test-sensor" {
		t.Errorf("expected SensorID=test-sensor, got %s", pc.SensorID)
	}
}

func TestSyntheticGenerator_PointCloud_WithinRadius(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")
	gen.PointCount = 500
	gen.AreaRadius = 50.0

	frame := gen.NextFrame()
	pc := frame.PointCloud

	for i := 0; i < len(pc.X); i++ {
		dist := math.Sqrt(float64(pc.X[i]*pc.X[i] + pc.Y[i]*pc.Y[i]))
		if dist > gen.AreaRadius*1.01 { // Allow small tolerance
			t.Errorf("point %d distance %.2f exceeds AreaRadius %.2f", i, dist, gen.AreaRadius)
		}
	}
}

func TestSyntheticGenerator_PointCloud_Classification(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")
	gen.PointCount = 1000

	frame := gen.NextFrame()
	pc := frame.PointCloud

	foregroundCount := 0
	backgroundCount := 0

	for _, c := range pc.Classification {
		if c == 0 {
			backgroundCount++
		} else if c == 1 {
			foregroundCount++
		} else {
			t.Errorf("unexpected classification value: %d", c)
		}
	}

	// Expect roughly 10% foreground (with some tolerance)
	foregroundRatio := float64(foregroundCount) / float64(gen.PointCount)
	if foregroundRatio < 0.01 || foregroundRatio > 0.25 {
		t.Errorf("unexpected foreground ratio: %.2f (expected ~0.10)", foregroundRatio)
	}
}

func TestSyntheticGenerator_Clusters(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")
	gen.TrackCount = 5

	frame := gen.NextFrame()

	if frame.Clusters == nil {
		t.Fatal("expected non-nil Clusters")
	}
	cs := frame.Clusters

	if len(cs.Clusters) != 5 {
		t.Errorf("expected 5 clusters, got %d", len(cs.Clusters))
	}
	if cs.Method != ClusteringDBSCAN {
		t.Errorf("expected ClusteringDBSCAN, got %d", cs.Method)
	}

	for i, c := range cs.Clusters {
		if c.ClusterID != int64(i+1) {
			t.Errorf("cluster %d: expected ClusterID=%d, got %d", i, i+1, c.ClusterID)
		}
		if c.SensorID != "test-sensor" {
			t.Errorf("cluster %d: expected SensorID=test-sensor, got %s", i, c.SensorID)
		}
		// Check centroid is at track radius
		dist := math.Sqrt(float64(c.CentroidX*c.CentroidX + c.CentroidY*c.CentroidY))
		if math.Abs(dist-gen.TrackRadius) > 0.01 {
			t.Errorf("cluster %d: expected centroid at radius %.2f, got %.2f", i, gen.TrackRadius, dist)
		}
	}
}

func TestSyntheticGenerator_Tracks(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")
	gen.TrackCount = 3

	frame := gen.NextFrame()

	if frame.Tracks == nil {
		t.Fatal("expected non-nil Tracks")
	}
	ts := frame.Tracks

	if len(ts.Tracks) != 3 {
		t.Errorf("expected 3 tracks, got %d", len(ts.Tracks))
	}
	if len(ts.Trails) != 3 {
		t.Errorf("expected 3 trails, got %d", len(ts.Trails))
	}

	for i, track := range ts.Tracks {
		expectedID := "track-001"
		if i == 1 {
			expectedID = "track-002"
		} else if i == 2 {
			expectedID = "track-003"
		}

		if track.TrackID != expectedID {
			t.Errorf("track %d: expected TrackID=%s, got %s", i, expectedID, track.TrackID)
		}
		if track.State != TrackStateConfirmed {
			t.Errorf("track %d: expected TrackStateConfirmed, got %d", i, track.State)
		}
		if track.SensorID != "test-sensor" {
			t.Errorf("track %d: expected SensorID=test-sensor, got %s", i, track.SensorID)
		}
		if track.SpeedMps != float32(gen.TrackSpeedMPS) {
			t.Errorf("track %d: expected SpeedMps=%.2f, got %.2f", i, gen.TrackSpeedMPS, track.SpeedMps)
		}
		if track.Confidence != 0.95 {
			t.Errorf("track %d: expected Confidence=0.95, got %.2f", i, track.Confidence)
		}
		if track.MotionModel != MotionModelCV {
			t.Errorf("track %d: expected MotionModelCV, got %d", i, track.MotionModel)
		}

		// Check track position is at track radius
		dist := math.Sqrt(float64(track.X*track.X + track.Y*track.Y))
		if math.Abs(dist-gen.TrackRadius) > 0.01 {
			t.Errorf("track %d: expected position at radius %.2f, got %.2f", i, gen.TrackRadius, dist)
		}
	}
}

func TestSyntheticGenerator_Tracks_VelocityTangent(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")
	gen.TrackCount = 1

	frame := gen.NextFrame()
	track := frame.Tracks.Trails[0]

	// Velocity should be tangent to the circle (perpendicular to position)
	tr := frame.Tracks.Tracks[0]
	posNorm := math.Sqrt(float64(tr.X*tr.X + tr.Y*tr.Y))
	velNorm := math.Sqrt(float64(tr.VX*tr.VX + tr.VY*tr.VY))

	if velNorm < 0.1 {
		t.Errorf("expected non-zero velocity, got magnitude %.2f", velNorm)
	}

	// Dot product of position and velocity should be ~0 for tangent
	dotProduct := float64(tr.X*tr.VX+tr.Y*tr.VY) / (posNorm * velNorm)
	if math.Abs(dotProduct) > 0.01 {
		t.Errorf("expected velocity tangent to position (dot~0), got %.4f", dotProduct)
	}

	_ = track // Use the track variable
}

func TestSyntheticGenerator_Trails(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")
	gen.TrackCount = 1
	gen.FrameRate = 10.0

	frame := gen.NextFrame()
	trail := frame.Tracks.Trails[0]

	// Trail should have ~20 points (2 seconds * 10 FPS)
	expectedPoints := int(2.0 * gen.FrameRate)
	if len(trail.Points) != expectedPoints {
		t.Errorf("expected %d trail points, got %d", expectedPoints, len(trail.Points))
	}

	if trail.TrackID != "track-001" {
		t.Errorf("expected TrackID=track-001, got %s", trail.TrackID)
	}
}

func TestSyntheticGenerator_PlaybackInfo(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")

	frame := gen.NextFrame()

	if frame.PlaybackInfo == nil {
		t.Fatal("expected non-nil PlaybackInfo")
	}
	if !frame.PlaybackInfo.IsLive {
		t.Error("expected IsLive=true for synthetic data")
	}
	if frame.PlaybackInfo.PlaybackRate != 1.0 {
		t.Errorf("expected PlaybackRate=1.0, got %f", frame.PlaybackInfo.PlaybackRate)
	}
	if frame.PlaybackInfo.Paused {
		t.Error("expected Paused=false")
	}
}

func TestSyntheticGenerator_CustomConfiguration(t *testing.T) {
	gen := NewSyntheticGenerator("custom-sensor")
	gen.PointCount = 500
	gen.TrackCount = 2
	gen.FrameRate = 5.0
	gen.AreaRadius = 25.0
	gen.TrackRadius = 10.0
	gen.TrackSpeedMPS = 2.5

	frame := gen.NextFrame()

	if frame.PointCloud.PointCount != 500 {
		t.Errorf("expected PointCount=500, got %d", frame.PointCloud.PointCount)
	}
	if len(frame.Clusters.Clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(frame.Clusters.Clusters))
	}
	if len(frame.Tracks.Tracks) != 2 {
		t.Errorf("expected 2 tracks, got %d", len(frame.Tracks.Tracks))
	}

	// Check trail length with new frame rate
	expectedTrailPoints := int(2.0 * gen.FrameRate)
	if len(frame.Tracks.Trails[0].Points) != expectedTrailPoints {
		t.Errorf("expected %d trail points, got %d", expectedTrailPoints, len(frame.Tracks.Trails[0].Points))
	}
}

func TestSyntheticGenerator_ClusterBBoxDimensions(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")

	frame := gen.NextFrame()

	for i, c := range frame.Clusters.Clusters {
		// Length should be 2.0-2.5
		if c.AABBLength < 2.0 || c.AABBLength > 2.5 {
			t.Errorf("cluster %d: AABBLength %.2f out of range [2.0, 2.5]", i, c.AABBLength)
		}
		// Width should be 1.8-2.1
		if c.AABBWidth < 1.8 || c.AABBWidth > 2.1 {
			t.Errorf("cluster %d: AABBWidth %.2f out of range [1.8, 2.1]", i, c.AABBWidth)
		}
		// Height should be 1.5-1.7
		if c.AABBHeight < 1.5 || c.AABBHeight > 1.7 {
			t.Errorf("cluster %d: AABBHeight %.2f out of range [1.5, 1.7]", i, c.AABBHeight)
		}
		// CentroidZ should be vehicle centre height (~0.8)
		if c.CentroidZ != 0.8 {
			t.Errorf("cluster %d: expected CentroidZ=0.8, got %.2f", i, c.CentroidZ)
		}
	}
}

func TestSyntheticGenerator_TrackBBoxDimensions(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")

	frame := gen.NextFrame()

	for i, track := range frame.Tracks.Tracks {
		if track.BBoxLengthAvg != 2.2 {
			t.Errorf("track %d: expected BBoxLengthAvg=2.2, got %.2f", i, track.BBoxLengthAvg)
		}
		if track.BBoxWidthAvg != 1.9 {
			t.Errorf("track %d: expected BBoxWidthAvg=1.9, got %.2f", i, track.BBoxWidthAvg)
		}
		if track.BBoxHeightAvg != 1.6 {
			t.Errorf("track %d: expected BBoxHeightAvg=1.6, got %.2f", i, track.BBoxHeightAvg)
		}
		if track.Z != 0.8 {
			t.Errorf("track %d: expected Z=0.8, got %.2f", i, track.Z)
		}
	}
}

func TestSyntheticGenerator_TracksEquidistantOnCircle(t *testing.T) {
	gen := NewSyntheticGenerator("test-sensor")
	gen.TrackCount = 4

	frame := gen.NextFrame()

	// With 4 tracks, they should be 90 degrees apart
	expectedAngleSep := 2 * math.Pi / 4.0

	for i := 0; i < len(frame.Tracks.Tracks)-1; i++ {
		t1 := frame.Tracks.Tracks[i]
		t2 := frame.Tracks.Tracks[i+1]

		angle1 := math.Atan2(float64(t1.Y), float64(t1.X))
		angle2 := math.Atan2(float64(t2.Y), float64(t2.X))

		angleDiff := angle2 - angle1
		if angleDiff < 0 {
			angleDiff += 2 * math.Pi
		}

		if math.Abs(angleDiff-expectedAngleSep) > 0.01 {
			t.Errorf("tracks %d and %d: expected angle separation %.4f, got %.4f",
				i, i+1, expectedAngleSep, angleDiff)
		}
	}
}
