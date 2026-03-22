package l5tracks

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// update applies the Kalman update step with a matched cluster measurement.
func (t *Tracker) update(track *TrackedObject, cluster WorldCluster, nowNanos int64) {
	// Measurement: z = [cluster.CentroidX, cluster.CentroidY]
	zX := cluster.CentroidX
	zY := cluster.CentroidY

	// Innovation
	yX := zX - track.X
	yY := zY - track.Y

	// Record innovation for debug visualisation
	if t.DebugCollector != nil && t.DebugCollector.IsEnabled() {
		residualMag := float32(math.Sqrt(float64(yX*yX + yY*yY)))
		t.DebugCollector.RecordInnovation(track.TrackID, track.X, track.Y, zX, zY, residualMag)
	}

	// Innovation covariance S = H * P * H^T + R
	S00 := track.P[0*4+0] + t.Config.MeasurementNoise
	S01 := track.P[0*4+1]
	S10 := track.P[1*4+0]
	S11 := track.P[1*4+1] + t.Config.MeasurementNoise

	// Compute S inverse
	det := S00*S11 - S01*S10
	if det < MinDeterminantThreshold {
		return // Cannot update with singular covariance
	}

	invS00 := S11 / det
	invS01 := -S01 / det
	invS10 := -S10 / det
	invS11 := S00 / det

	// Kalman gain K = P * H^T * S^-1
	// K is 4x2 matrix
	// K[i,0] = P[i,0]*invS00 + P[i,1]*invS10
	// K[i,1] = P[i,0]*invS01 + P[i,1]*invS11
	var K [8]float32
	for i := 0; i < 4; i++ {
		K[i*2+0] = track.P[i*4+0]*invS00 + track.P[i*4+1]*invS10
		K[i*2+1] = track.P[i*4+0]*invS01 + track.P[i*4+1]*invS11
	}

	// Update state: x' = x + K * y
	track.X += K[0*2+0]*yX + K[0*2+1]*yY
	track.Y += K[1*2+0]*yX + K[1*2+1]*yY
	track.VX += K[2*2+0]*yX + K[2*2+1]*yY
	track.VY += K[3*2+0]*yX + K[3*2+1]*yY

	// Update covariance: P' = (I - K*H) * P
	// K*H is 4x4, where (K*H)[i,j] = K[i,0]*H[0,j] + K[i,1]*H[1,j]
	// H[0,0]=1, H[0,1]=0, H[0,2]=0, H[0,3]=0
	// H[1,0]=0, H[1,1]=1, H[1,2]=0, H[1,3]=0
	// So (K*H)[i,j] = K[i,0] if j==0, K[i,1] if j==1, 0 otherwise
	var IminusKH [16]float32
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			identity := float32(0)
			if i == j {
				identity = 1
			}
			var kh float32
			if j == 0 {
				kh = K[i*2+0]
			} else if j == 1 {
				kh = K[i*2+1]
			}
			IminusKH[i*4+j] = identity - kh
		}
	}

	// P' = IminusKH * P
	var newP [16]float32
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += IminusKH[i*4+k] * track.P[k*4+j]
			}
			newP[i*4+j] = sum
		}
	}
	track.P = newP

	// Guard: reset state if update produced NaN/Inf (task 2.4).
	if !isFiniteState(track) {
		opsf("Update produced non-finite state: track_id=%s deleting track", track.TrackID)
		track.X = 0
		track.Y = 0
		track.VX = 0
		track.VY = 0
		track.P = [16]float32{
			10, 0, 0, 0,
			0, 10, 0, 0,
			0, 0, 1, 0,
			0, 0, 0, 1,
		}
		track.State = TrackDeleted
		return
	}

	// Clamp velocity magnitude after update (task 2.3).
	t.clampVelocity(track)

	// Update timestamp
	track.LastUnixNanos = nowNanos
	if track.LastUnixNanos > track.FirstUnixNanos {
		track.TrackDurationSecs = float32(track.LastUnixNanos-track.FirstUnixNanos) / 1e9
	}

	// Update aggregated features
	track.ObservationCount++

	// Running average for bounding box
	n := float32(track.ObservationCount)
	track.BoundingBoxLengthAvg = ((n-1)*track.BoundingBoxLengthAvg + cluster.BoundingBoxLength) / n
	track.BoundingBoxWidthAvg = ((n-1)*track.BoundingBoxWidthAvg + cluster.BoundingBoxWidth) / n
	track.BoundingBoxHeightAvg = ((n-1)*track.BoundingBoxHeightAvg + cluster.BoundingBoxHeight) / n
	track.IntensityMeanAvg = ((n-1)*track.IntensityMeanAvg + cluster.IntensityMean) / n

	// Max height P95
	if cluster.HeightP95 > track.HeightP95Max {
		track.HeightP95Max = cluster.HeightP95
	}

	// Update speed statistics
	speed := float32(math.Sqrt(float64(track.VX*track.VX + track.VY*track.VY)))
	track.AvgSpeedMps = ((n-1)*track.AvgSpeedMps + speed) / n
	if speed > track.MaxSpeedMps {
		track.MaxSpeedMps = speed
	}

	// Speed jitter: measure frame-to-frame speed change
	if track.ObservationCount > 1 {
		speedDelta := float64(speed - track.PrevSpeedMps)
		track.SpeedJitterSumSq += speedDelta * speedDelta
		track.SpeedJitterCount++
	}
	track.PrevSpeedMps = speed

	// Append to history
	// Skip points too close to origin (noise/self-reflection)
	distFromOrigin := track.X*track.X + track.Y*track.Y
	if distFromOrigin > 0.01 { // > 0.1m squared
		var (
			previousPoint TrackPoint
			hasPrevious   bool
		)
		if len(track.History) > 0 {
			previousPoint = track.History[len(track.History)-1]
			hasPrevious = true
		}
		track.History = append(track.History, TrackPoint{
			X:         track.X,
			Y:         track.Y,
			Timestamp: nowNanos,
		})
		if hasPrevious {
			dx := track.X - previousPoint.X
			dy := track.Y - previousPoint.Y
			track.TrackLengthMeters += float32(math.Sqrt(float64(dx*dx + dy*dy)))
		}
		if len(track.History) > t.Config.MaxTrackHistoryLength {
			track.History = track.History[len(track.History)-t.Config.MaxTrackHistoryLength:]
		}
	}

	// Store speed history for jitter/variance analysis
	track.speedHistory = append(track.speedHistory, speed)
	if len(track.speedHistory) > t.Config.MaxSpeedHistoryLength {
		track.speedHistory = track.speedHistory[1:]
	}

	// Velocity-Trail Alignment: Compare Kalman velocity heading with
	// displacement heading from the last two trail positions.
	// Only compute when the track has sufficient history and speed.
	if len(track.History) >= 2 && speed > 0.5 { // Need ≥2 points and moving
		prev := track.History[len(track.History)-2]
		curr := track.History[len(track.History)-1]

		dx := curr.X - prev.X
		dy := curr.Y - prev.Y
		displacementDist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		if displacementDist > 0.05 { // Minimum displacement to compute heading (5cm)
			displacementHeading := float32(math.Atan2(float64(dy), float64(dx)))
			velocityHeading := float32(math.Atan2(float64(track.VY), float64(track.VX)))

			// Angular difference, normalised to [0, π]
			angDiff := velocityHeading - displacementHeading
			for angDiff > math.Pi {
				angDiff -= 2 * math.Pi
			}
			for angDiff < -math.Pi {
				angDiff += 2 * math.Pi
			}
			absAngDiff := float32(math.Abs(float64(angDiff)))

			track.AlignmentSampleCount++
			track.AlignmentSumRad += absAngDiff
			track.AlignmentMeanRad = track.AlignmentSumRad / float32(track.AlignmentSampleCount)
			if absAngDiff > math.Pi/4 { // > 45° is misaligned
				track.AlignmentMisaligned++
			}
		}
	}

	// Update OBB heading with temporal smoothing.
	// Guards:
	//   1. Skip heading update when cluster has too few points for reliable PCA.
	//   2. Lock heading when aspect ratio ≈ 1:1 (ambiguous principal axis).
	//   3. Reject 90° jumps: if the raw heading delta vs smoothed is near ±90°,
	//      this is likely a PCA axis swap (not real object rotation). Hold heading.
	//   4. EMA α = OBBHeadingSmoothingAlpha (0.08) provides heavy smoothing.
	// Per-frame OBB dimensions use cluster (DBSCAN) values directly when heading
	// is updated. When heading is locked, only height is updated to avoid
	// desynchronising dimensions from the locked heading.
	if cluster.OBB != nil {
		updateHeading := true
		headingSource := HeadingSourcePCA

		// Guard 1: minimum point count for reliable PCA
		if cluster.PointsCount < t.Config.MinPointsForPCA {
			updateHeading = false
			headingSource = HeadingSourceLocked
		}

		// Guard 2: near-square aspect ratio → heading ambiguous
		if updateHeading {
			maxDim := cluster.OBB.Length
			if cluster.OBB.Width > maxDim {
				maxDim = cluster.OBB.Width
			}
			if maxDim > 0 {
				aspectDiff := cluster.OBB.Length - cluster.OBB.Width
				if aspectDiff < 0 {
					aspectDiff = -aspectDiff
				}
				if aspectDiff/maxDim < t.Config.OBBAspectRatioLockThreshold {
					updateHeading = false
					headingSource = HeadingSourceLocked
				}
			}
		}

		if updateHeading {
			newOBBHeading := cluster.OBB.HeadingRad

			// Disambiguate PCA heading using velocity direction.
			// PCA gives the axis of maximum variance but has 180° ambiguity.
			// If the track has sufficient velocity, flip the PCA heading
			// to align with the direction of travel.
			speed := float32(math.Sqrt(float64(track.VX*track.VX + track.VY*track.VY)))
			disambiguated := false
			if speed > 0.5 { // Only disambiguate when moving (>0.5 m/s)
				velHeading := float32(math.Atan2(float64(track.VY), float64(track.VX)))
				// Compute angular difference between PCA heading and velocity heading
				diff := newOBBHeading - velHeading
				// Normalise to [-π, π]
				for diff > math.Pi {
					diff -= 2 * math.Pi
				}
				for diff < -math.Pi {
					diff += 2 * math.Pi
				}
				// If PCA heading opposes velocity (diff > 90°), flip it by π
				if diff > math.Pi/2 || diff < -math.Pi/2 {
					newOBBHeading += math.Pi
					if newOBBHeading > math.Pi {
						newOBBHeading -= 2 * math.Pi
					}
				}
				disambiguated = true
				headingSource = HeadingSourceVelocity
			}

			// Fall back to displacement vector when Kalman velocity
			// is too low for reliable disambiguation. This handles slow-
			// moving objects (e.g. vehicles at junctions, pedestrians) where
			// the Kalman velocity is near zero but real motion is occurring.
			if !disambiguated && len(track.History) >= 2 {
				last := track.History[len(track.History)-1]
				prev := track.History[len(track.History)-2]
				dx := last.X - prev.X
				dy := last.Y - prev.Y
				displacement := float32(math.Sqrt(float64(dx*dx + dy*dy)))
				if displacement > 0.1 { // 10 cm minimum displacement
					refHeading := float32(math.Atan2(float64(dy), float64(dx)))
					diff := newOBBHeading - refHeading
					for diff > math.Pi {
						diff -= 2 * math.Pi
					}
					for diff < -math.Pi {
						diff += 2 * math.Pi
					}
					if diff > math.Pi/2 || diff < -math.Pi/2 {
						newOBBHeading += math.Pi
						if newOBBHeading > math.Pi {
							newOBBHeading -= 2 * math.Pi
						}
					}
					headingSource = HeadingSourceDisplacement
				}
			}

			// Guard 3: Reject 90° jumps. After disambiguation, if the
			// heading delta vs the previous smoothed heading is near ±90°,
			// this is almost certainly a PCA axis swap for a near-square
			// cluster that slipped past Guard 2. Lock heading to prevent
			// the box from spinning by 90° and snapping back.
			if track.ObservationCount > 1 {
				headingDelta := float64(newOBBHeading - track.OBBHeadingRad)
				// Normalise to [-π, π]
				for headingDelta > math.Pi {
					headingDelta -= 2 * math.Pi
				}
				for headingDelta < -math.Pi {
					headingDelta += 2 * math.Pi
				}

				// Track heading jitter before smoothing
				track.HeadingJitterSumSq += headingDelta * headingDelta
				track.HeadingJitterCount++

				// Reject jumps near ±90° (between 60° and 120°). These are
				// characteristic of PCA axis swaps where the principal and
				// perpendicular axes exchange. Real objects do not rotate
				// 90° in a single frame at traffic-monitoring distances.
				absDelta := math.Abs(headingDelta)
				if absDelta > math.Pi/3 && absDelta < 2*math.Pi/3 {
					updateHeading = false
					headingSource = HeadingSourceLocked
				}
			}

			if updateHeading {
				track.OBBHeadingRad = l4perception.SmoothOBBHeading(track.OBBHeadingRad, newOBBHeading, t.Config.OBBHeadingSmoothingAlpha)
			}
		}

		track.HeadingSource = headingSource

		// Use cluster (DBSCAN) dimensions directly for per-frame rendering.
		// The DBSCAN OBB dimensions are aligned with the current frame's PCA
		// heading and capture all cluster points. When the heading was updated
		// this frame, the cluster dimensions are consistent with the new heading.
		// When the heading was locked (Guard 1/2/3), the cluster axes may be
		// rotated relative to the stored heading — in that case only update
		// height (which is independent of horizontal axis orientation) to avoid
		// desynchronising dimensions from the locked heading.
		if updateHeading {
			track.OBBLength = cluster.OBB.Length
			track.OBBWidth = cluster.OBB.Width
			track.OBBHeight = cluster.OBB.Height
		} else {
			// Heading was locked: cluster Length/Width may be swapped relative
			// to the track's heading. Only update height (axis-independent).
			track.OBBHeight = cluster.OBB.Height
		}
		track.LatestZ = cluster.OBB.CenterZ
	}
}
