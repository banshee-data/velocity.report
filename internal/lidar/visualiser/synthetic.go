// Package visualiser provides gRPC streaming of LiDAR perception data.
// This file provides synthetic data generation for testing and demos.
package visualiser

import (
	"fmt"
	"math"
	"math/rand"
	"sync/atomic"
	"time"
)

// SyntheticGenerator generates synthetic LiDAR data for testing.
type SyntheticGenerator struct {
	frameID  atomic.Uint64
	sensorID string
	startNs  int64

	// Configuration
	PointCount    int     // points per frame
	TrackCount    int     // number of moving tracks
	FrameRate     float64 // frames per second
	AreaRadius    float64 // metres, radius of point cloud area
	TrackRadius   float64 // metres, radius of track circular paths
	TrackSpeedMPS float64 // metres per second for tracks

	// Internal state
	rng *rand.Rand
}

// NewSyntheticGenerator creates a new synthetic data generator.
func NewSyntheticGenerator(sensorID string) *SyntheticGenerator {
	return &SyntheticGenerator{
		sensorID:      sensorID,
		startNs:       time.Now().UnixNano(),
		PointCount:    10000,
		TrackCount:    10,
		FrameRate:     10.0,
		AreaRadius:    50.0,
		TrackRadius:   20.0,
		TrackSpeedMPS: 5.0,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// NextFrame generates the next synthetic frame.
func (g *SyntheticGenerator) NextFrame() *FrameBundle {
	frameID := g.frameID.Add(1)
	now := time.Now().UnixNano()
	elapsed := float64(now-g.startNs) / 1e9 // seconds

	frame := &FrameBundle{
		FrameID:        frameID,
		TimestampNanos: now,
		SensorID:       g.sensorID,
		CoordinateFrame: CoordinateFrameInfo{
			FrameID:        "site/" + g.sensorID,
			ReferenceFrame: "ENU",
			OriginLat:      51.5074,
			OriginLon:      -0.1278,
		},
		PointCloud: g.generatePointCloud(frameID, now),
		Clusters:   g.generateClusters(frameID, now, elapsed),
		Tracks:     g.generateTracks(frameID, now, elapsed),
		PlaybackInfo: &PlaybackInfo{
			IsLive:       true,
			PlaybackRate: 1.0,
		},
	}

	return frame
}

// generatePointCloud creates a synthetic point cloud.
func (g *SyntheticGenerator) generatePointCloud(frameID uint64, timestampNs int64) *PointCloudFrame {
	pc := &PointCloudFrame{
		FrameID:        frameID,
		TimestampNanos: timestampNs,
		SensorID:       g.sensorID,
		X:              make([]float32, g.PointCount),
		Y:              make([]float32, g.PointCount),
		Z:              make([]float32, g.PointCount),
		Intensity:      make([]uint8, g.PointCount),
		Classification: make([]uint8, g.PointCount),
		PointCount:     g.PointCount,
	}

	// Generate points in a disc pattern with some height variation
	for i := 0; i < g.PointCount; i++ {
		// Random angle and radius (uniform disc distribution)
		angle := g.rng.Float64() * 2 * math.Pi
		r := math.Sqrt(g.rng.Float64()) * g.AreaRadius

		pc.X[i] = float32(r * math.Cos(angle))
		pc.Y[i] = float32(r * math.Sin(angle))

		// Height: mostly ground level with some variation
		if g.rng.Float64() < 0.1 {
			// 10% foreground points (objects)
			pc.Z[i] = float32(g.rng.Float64() * 2.0) // 0-2m height
			pc.Classification[i] = 1                 // foreground
		} else {
			// 90% ground/background
			pc.Z[i] = float32(g.rng.Float64()*0.2 - 0.1) // -0.1 to 0.1m
			pc.Classification[i] = 0                     // background
		}

		// Intensity based on distance (closer = brighter)
		dist := math.Sqrt(float64(pc.X[i]*pc.X[i] + pc.Y[i]*pc.Y[i]))
		intensity := 200 - int(dist*3)
		if intensity < 50 {
			intensity = 50
		}
		pc.Intensity[i] = uint8(intensity + g.rng.Intn(30))
	}

	return pc
}

// generateClusters creates synthetic clusters matching track positions.
func (g *SyntheticGenerator) generateClusters(frameID uint64, timestampNs int64, elapsedSecs float64) *ClusterSet {
	clusters := make([]Cluster, g.TrackCount)

	for i := 0; i < g.TrackCount; i++ {
		// Calculate position on circular path
		baseAngle := float64(i) * 2 * math.Pi / float64(g.TrackCount)
		angularSpeed := g.TrackSpeedMPS / g.TrackRadius
		angle := baseAngle + elapsedSecs*angularSpeed

		x := g.TrackRadius * math.Cos(angle)
		y := g.TrackRadius * math.Sin(angle)

		clusters[i] = Cluster{
			ClusterID:      int64(i + 1),
			SensorID:       g.sensorID,
			TimestampNanos: timestampNs,
			CentroidX:      float32(x),
			CentroidY:      float32(y),
			CentroidZ:      0.8, // typical vehicle centre height
			AABBLength:     2.0 + g.rng.Float32()*0.5,
			AABBWidth:      1.8 + g.rng.Float32()*0.3,
			AABBHeight:     1.5 + g.rng.Float32()*0.2,
		}
	}

	return &ClusterSet{
		FrameID:        frameID,
		TimestampNanos: timestampNs,
		Clusters:       clusters,
		Method:         ClusteringDBSCAN,
	}
}

// generateTracks creates synthetic tracks moving in circles.
func (g *SyntheticGenerator) generateTracks(frameID uint64, timestampNs int64, elapsedSecs float64) *TrackSet {
	tracks := make([]Track, g.TrackCount)
	trails := make([]TrackTrail, g.TrackCount)

	for i := 0; i < g.TrackCount; i++ {
		trackID := fmt.Sprintf("track-%03d", i+1)

		// Calculate position on circular path
		baseAngle := float64(i) * 2 * math.Pi / float64(g.TrackCount)
		angularSpeed := g.TrackSpeedMPS / g.TrackRadius
		angle := baseAngle + elapsedSecs*angularSpeed

		x := g.TrackRadius * math.Cos(angle)
		y := g.TrackRadius * math.Sin(angle)

		// Velocity tangent to circle
		vx := -g.TrackSpeedMPS * math.Sin(angle)
		vy := g.TrackSpeedMPS * math.Cos(angle)

		// Heading from velocity
		heading := math.Atan2(vy, vx)

		tracks[i] = Track{
			TrackID:          trackID,
			SensorID:         g.sensorID,
			State:            TrackStateConfirmed,
			Hits:             int(elapsedSecs * g.FrameRate),
			ObservationCount: int(elapsedSecs * g.FrameRate),
			FirstSeenNanos:   g.startNs,
			LastSeenNanos:    timestampNs,
			X:                float32(x),
			Y:                float32(y),
			Z:                0.8,
			VX:               float32(vx),
			VY:               float32(vy),
			VZ:               0,
			SpeedMps:         float32(g.TrackSpeedMPS),
			HeadingRad:       float32(heading),
			BBoxLengthAvg:    2.2,
			BBoxWidthAvg:     1.9,
			BBoxHeightAvg:    1.6,
			BBoxHeadingRad:   float32(heading),
			Confidence:       0.95,
			MotionModel:      MotionModelCV,
		}

		// Generate trail (last 2 seconds of history)
		trailDuration := 2.0 // seconds
		trailPoints := int(trailDuration * g.FrameRate)
		points := make([]TrackPoint, trailPoints)

		for j := 0; j < trailPoints; j++ {
			t := elapsedSecs - trailDuration + float64(j)/g.FrameRate
			if t < 0 {
				t = 0
			}
			trailAngle := baseAngle + t*angularSpeed
			points[j] = TrackPoint{
				X:              float32(g.TrackRadius * math.Cos(trailAngle)),
				Y:              float32(g.TrackRadius * math.Sin(trailAngle)),
				TimestampNanos: timestampNs - int64((trailDuration-float64(j)/g.FrameRate)*1e9),
			}
		}

		trails[i] = TrackTrail{
			TrackID: trackID,
			Points:  points,
		}
	}

	return &TrackSet{
		FrameID:        frameID,
		TimestampNanos: timestampNs,
		Tracks:         tracks,
		Trails:         trails,
	}
}
