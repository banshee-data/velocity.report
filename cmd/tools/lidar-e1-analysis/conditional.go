package main

// E1.1, the conditional-mean-by-aspect test from Section 16.5 — the one the
// plan calls decisive.
//
// The logic is that random error has a conditional mean of zero in every bin
// and a geometric bias does not. So for every observation of a confirmed moving
// track we measure each candidate's signed lateral offset from a robust path
// and bin those offsets by aspect angle. Under the null, every candidate's
// conditional mean is zero everywhere. Under Section 3's hypothesis, the
// medoid's approaches ±W/2 in bins where one face dominates and passes through
// zero where two faces are equally weighted.
//
// **On circularity.** The reference path is fitted to the tracker's own
// estimates, and those estimates were produced from one of the candidates under
// test, so the reference is not independent of the thing being measured. The
// plan's answer, which this implementation relies on, is that a conditional
// mean taken over a covariate the fit never sees cannot be manufactured by the
// fit: nothing in a straight-line fit knows the aspect angle. Two further
// guards are applied here. The fit spans the whole straight segment, so an
// aspect-dependent bias that reverses across a pass partly averages out of the
// reference rather than being tracked by it. And the fit is only accepted where
// the segment really is straight, because a line fitted to a curve produces
// residuals that vary along the track for reasons that have nothing to do with
// aspect.

import (
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/db"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// trackEstimate is one persisted filter output, joined to its observation.
type trackEstimate struct {
	observationID  string
	frameUnixNanos int64
	x, y           float64
	vx, vy         float64
}

// trackPath is a straight reference fitted to one track's estimates.
//
// The line is stored as a point on it plus a unit direction, which is the form
// the perpendicular offset needs and which has no trouble with vertical lines
// the way a slope-intercept form would.
type trackPath struct {
	originX, originY float64
	dirX, dirY       float64
	// residualRMS is how well a straight line describes this track. A segment
	// that is not straight is not usable as a reference.
	residualRMS float64
	speedMps    float64
	estimates   int
}

// fitTrackPath fits a total-least-squares line through a track's estimates.
//
// Ordinary least squares would be wrong here: it minimises error in one
// coordinate, so its answer depends on whether the track happens to run along
// X or Y. The principal axis of the point set does not.
func fitTrackPath(estimates []trackEstimate) (trackPath, bool) {
	if len(estimates) < 3 {
		return trackPath{}, false
	}

	var sumX, sumY, sumSpeed float64
	for _, e := range estimates {
		sumX += e.x
		sumY += e.y
		sumSpeed += math.Hypot(e.vx, e.vy)
	}
	n := float64(len(estimates))
	meanX, meanY := sumX/n, sumY/n

	var cxx, cxy, cyy float64
	for _, e := range estimates {
		dx, dy := e.x-meanX, e.y-meanY
		cxx += dx * dx
		cxy += dx * dy
		cyy += dy * dy
	}
	cxx, cxy, cyy = cxx/n, cxy/n, cyy/n

	// Principal axis of the 2x2 covariance, by the same non-cancelling
	// discriminant form the OBB estimator uses.
	spread := cxx - cyy
	disc := math.Sqrt(spread*spread + 4*cxy*cxy)
	lambda := (cxx + cyy + disc) / 2

	dirX, dirY := cxy, lambda-cxx
	if mag := math.Hypot(dirX, dirY); mag > 1e-9 {
		dirX, dirY = dirX/mag, dirY/mag
	} else if cxx >= cyy {
		dirX, dirY = 1, 0
	} else {
		dirX, dirY = 0, 1
	}

	path := trackPath{
		originX: meanX, originY: meanY,
		dirX: dirX, dirY: dirY,
		speedMps:  sumSpeed / n,
		estimates: len(estimates),
	}

	var sumSq float64
	for _, e := range estimates {
		off := path.lateralOffset(e.x, e.y)
		sumSq += off * off
	}
	path.residualRMS = math.Sqrt(sumSq / n)
	return path, true
}

// lateralOffset is the signed perpendicular distance from the path, positive on
// the side the path's left normal points to.
func (p trackPath) lateralOffset(x, y float64) float64 {
	dx, dy := x-p.originX, y-p.originY
	// The left normal of (dirX, dirY).
	return dx*(-p.dirY) + dy*p.dirX
}

// conditionalCell accumulates one aspect-by-range cell for one candidate.
type conditionalCell struct {
	offsets   []float64
	halfWidth []float64
}

func (c *conditionalCell) add(offset, halfWidth float64) {
	c.offsets = append(c.offsets, offset)
	c.halfWidth = append(c.halfWidth, halfWidth)
}

// conditionalConfig holds the thresholds the test is run at.
type conditionalConfig struct {
	minSpeedMps        float64
	maxResidualRMS     float64
	minEstimates       int
	minCellCount       int
	aspectBinDeg       float64
	sensorSideAgnostic bool
}

// reportConditionalMeans runs E1.1 over samples already joined to their tracks.
//
// signedByFace flips the sign of every offset so that positive always means
// "toward the sensor". Without it the two lateral faces cancel: a vehicle
// passing on the left is biased one way in the site frame and one passing on
// the right the other, and averaging them would hide exactly the bias being
// looked for.
func reportConditionalMeans(joined []joinedSample, cfg conditionalConfig) {
	fmt.Printf("\nExperiment E1.1: conditional mean by aspect, signed toward the sensor\n")
	fmt.Printf("  tracks accepted require: speed >= %.1f m/s, straight-line residual RMS <= %.2f m, >= %d estimates\n",
		cfg.minSpeedMps, cfg.maxResidualRMS, cfg.minEstimates)
	fmt.Printf("  cells with fewer than %d observations are not reported\n", cfg.minCellCount)

	bySite := map[string][]joinedSample{}
	for _, j := range joined {
		bySite[j.site] = append(bySite[j.site], j)
	}
	siteNames := make([]string, 0, len(bySite))
	for name := range bySite {
		siteNames = append(siteNames, name)
	}
	sort.Strings(siteNames)

	nBins := int(math.Ceil(90 / cfg.aspectBinDeg))

	for _, site := range siteNames {
		fmt.Printf("\n%s (%d observations on accepted tracks)\n", site, len(bySite[site]))

		for _, cand := range []candidate{candMedoid, candOBBCentre, candNearEdge} {
			cells := make([]conditionalCell, nBins)
			for _, j := range bySite[site] {
				if !j.available[cand] {
					continue
				}
				b := int(j.aspectDeg / cfg.aspectBinDeg)
				if b >= nBins {
					b = nBins - 1
				}
				cells[b].add(j.offsetTowardSensor[cand], j.halfWidthM)
			}

			fmt.Printf("\n  %-14s", cand.String())
			for b := 0; b < nBins; b++ {
				fmt.Printf(" %11s", fmt.Sprintf("%.0f-%.0f", float64(b)*cfg.aspectBinDeg, float64(b+1)*cfg.aspectBinDeg))
			}
			fmt.Println()

			fmt.Printf("  %-14s", "mean (m)")
			for b := 0; b < nBins; b++ {
				if len(cells[b].offsets) < cfg.minCellCount {
					fmt.Printf(" %11s", "-")
					continue
				}
				fmt.Printf(" %11.3f", mean(cells[b].offsets))
			}
			fmt.Println()

			fmt.Printf("  %-14s", "as W/2")
			for b := 0; b < nBins; b++ {
				if len(cells[b].offsets) < cfg.minCellCount {
					fmt.Printf(" %11s", "-")
					continue
				}
				hw := mean(cells[b].halfWidth)
				if hw <= 0 {
					fmt.Printf(" %11s", "-")
					continue
				}
				fmt.Printf(" %11.2f", mean(cells[b].offsets)/hw)
			}
			fmt.Println()

			fmt.Printf("  %-14s", "std (m)")
			for b := 0; b < nBins; b++ {
				if len(cells[b].offsets) < cfg.minCellCount {
					fmt.Printf(" %11s", "-")
					continue
				}
				fmt.Printf(" %11.3f", stddev(cells[b].offsets))
			}
			fmt.Println()

			fmt.Printf("  %-14s", "n")
			for b := 0; b < nBins; b++ {
				fmt.Printf(" %11d", len(cells[b].offsets))
			}
			fmt.Println()
		}
	}

	fmt.Printf("\nThe decisive comparison is the \"as W/2\" row. Random error gives zero in\n")
	fmt.Printf("every bin for every candidate. Section 3's hypothesis predicts the medoid\n")
	fmt.Printf("trending toward 1.0 as aspect approaches broadside, where one face comes to\n")
	fmt.Printf("dominate its point set, while a measurement that models the visible surface\n")
	fmt.Printf("stays flat.\n")
}

// joinedSample is an observation paired with its track's reference path.
type joinedSample struct {
	site       string
	aspectDeg  float64
	rangeM     float64
	halfWidthM float64
	// offsetTowardSensor is each candidate's signed lateral offset from the
	// reference path, with the sign normalised so positive is toward the
	// sensor. Averaging raw site-frame offsets would cancel the two lateral
	// faces against each other.
	offsetTowardSensor [candidateCount]float64
	available          [candidateCount]bool
}

// joinStats records how the track population narrowed, so an accepted set is
// auditable rather than whatever happened to pass.
type joinStats struct {
	estimates      int
	tracks         int
	rejectedSlow   int
	rejectedCurved int
	rejectedShort  int
	acceptedTracks int
	joined         int
	unmatched      int
}

func (s joinStats) String() string {
	return fmt.Sprintf(
		"E1.1 track population: %d estimates over %d tracks; rejected %d as slow, %d as not straight, %d as too short; "+
			"accepted %d tracks carrying %d observations (%d observations had no estimate)",
		s.estimates, s.tracks, s.rejectedSlow, s.rejectedCurved, s.rejectedShort,
		s.acceptedTracks, s.joined, s.unmatched)
}

// joinToTrackPaths fits a reference path per track and attaches every
// observation on an accepted track to it.
func joinToTrackPaths(dbPath string, sites map[string]string, samples []sample, cfg conditionalConfig) ([]joinedSample, joinStats, error) {
	database, err := db.NewDBWithMigrationCheck(dbPath, false)
	if err != nil {
		return nil, joinStats{}, err
	}
	defer database.Close()
	store := observationsqlite.NewStateEstimateStore(database)

	// Observations indexed by identity, so the estimate rows can find them.
	byID := make(map[string]sample, len(samples))
	for _, s := range samples {
		byID[s.observationID] = s
	}

	var stats joinStats
	var joined []joinedSample

	sourceIDs := make([]string, 0, len(sites))
	for sourceID := range sites {
		sourceIDs = append(sourceIDs, sourceID)
	}
	sort.Strings(sourceIDs)

	for _, sourceID := range sourceIDs {
		estimates, err := store.ListBySource(sourceID)
		if err != nil {
			return nil, stats, fmt.Errorf("listing estimates for %s: %w", sites[sourceID], err)
		}
		stats.estimates += len(estimates)

		// Group by the deterministic creation sequence rather than the random
		// track UUID, so the grouping is reproducible across replays.
		byTrack := map[int64][]trackEstimate{}
		for _, e := range estimates {
			byTrack[e.CreationSequence] = append(byTrack[e.CreationSequence], trackEstimate{
				observationID:  e.ObservationID,
				frameUnixNanos: e.FrameUnixNanos,
				x:              float64(e.X),
				y:              float64(e.Y),
				vx:             float64(e.VX),
				vy:             float64(e.VY),
			})
		}

		sequences := make([]int64, 0, len(byTrack))
		for seq := range byTrack {
			sequences = append(sequences, seq)
		}
		sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })

		for _, seq := range sequences {
			track := byTrack[seq]
			stats.tracks++

			if len(track) < cfg.minEstimates {
				stats.rejectedShort++
				continue
			}
			path, ok := fitTrackPath(track)
			if !ok {
				stats.rejectedShort++
				continue
			}
			if path.speedMps < cfg.minSpeedMps {
				stats.rejectedSlow++
				continue
			}
			// A line fitted to a curve produces residuals that vary along the
			// track for reasons unrelated to aspect, which would masquerade as
			// the effect being measured.
			if path.residualRMS > cfg.maxResidualRMS {
				stats.rejectedCurved++
				continue
			}
			stats.acceptedTracks++

			// Which way along the path's normal lies the sensor? Offsets are
			// signed toward it so that vehicles passing on either side do not
			// cancel each other out.
			towardSensor := 1.0
			if path.lateralOffset(0, 0) < 0 {
				towardSensor = -1
			}

			for _, e := range track {
				s, ok := byID[e.observationID]
				if !ok {
					stats.unmatched++
					continue
				}
				if s.clipped {
					continue
				}
				j := joinedSample{
					site:       s.site,
					aspectDeg:  s.aspectDeg,
					rangeM:     s.rangeM,
					halfWidthM: s.widthM / 2,
					available:  s.available,
				}
				for c := candidate(0); c < candidateCount; c++ {
					if !s.available[c] {
						continue
					}
					j.offsetTowardSensor[c] = towardSensor * path.lateralOffset(s.posX[c], s.posY[c])
				}
				joined = append(joined, j)
				stats.joined++
			}
		}
	}
	return joined, stats, nil
}
