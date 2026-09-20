package l8analytics

import (
	"math"
	"sort"
)

// Label-free track scorecard.
//
// The campaign's ground-truth evaluator matches whole tracks on temporal
// overlap, so it cannot see state estimates and scores a fragmented track as an
// undetected one (gap analysis M5), and labels exist for two captures. This
// scorecard asks questions that need no labels and that the immutable evidence
// store can answer for any replay: how long tracks live, how they end, how far
// a constant-velocity coast drifts, and how the innovation varies with range
// and support. It is meant for comparing a default-off option on against off
// at the same site, not for grading a tracker in absolute terms.
//
// Determinism is a requirement, not a nicety: two independent replays of the
// same input must produce byte-identical scorecards. So the inputs are sorted
// here into a total order whatever order the caller supplied, tracks are keyed
// by creation sequence (the track ID is a random UUID), histograms are integer
// counts, float sums accumulate in that fixed order, and every slice in the
// output is built by ordered loops, never by ranging over a map.

// ScorecardEstimate is one accepted association: the posterior state and the
// innovation that produced it.
type ScorecardEstimate struct {
	CreationSequence           int64
	FrameUnixNanos             int64
	ObservationID              string
	X, Y, VX, VY               float64
	MeasurementX, MeasurementY float64
	InnovationX, InnovationY   float64
	NIS                        float64
}

// ScorecardCluster is one L4 cluster, associated or not.
type ScorecardCluster struct {
	ObservationID  string
	FrameUnixNanos int64
	X, Y           float64
	PointsCount    int
}

// ScorecardOptions bounds what is scored.
type ScorecardOptions struct {
	// ScoringStartNanos excludes the warm-up: estimates before it are not
	// scored, and tracks born before it are not counted in the population.
	ScoringStartNanos int64
}

// Fixed analysis parameters. They are constants rather than options so that
// two scorecards are always comparable.
const (
	scorecardLifetimeBinSeconds = 0.1
	scorecardLifetimeBins       = 600 // 60 s, last bin is overflow
	scorecardCoastBinMetres     = 0.05
	scorecardCoastBins          = 200 // 10 m, last bin is overflow
	scorecardHorizonTolerance   = 0.05
	scorecardDecelLookback      = 0.5
	scorecardDecelThresholdMps  = 0.5
	scorecardNearMetres         = 1.0
	scorecardFarMetres          = 2.0
	scorecardLookaheadFrames    = 3.5
	scorecardCensorSeconds      = 1.5
	scorecardNIS95              = 5.991 // chi-squared, 2 degrees of freedom
	scorecardMovingMps          = 2.0
)

var (
	scorecardHorizons     = []float64{0.5, 1.0, 1.5}
	scorecardSpeedFloors  = []float64{0, 2, 5, 10, 15}
	scorecardRangeFloors  = []float64{0, 10, 20, 30, 50}
	scorecardPointsFloors = []int{0, 10, 30, 100}
	scorecardLifeFloors   = []float64{0, 1, 5}
	// ScorecardTerminationClasses is the closed set, in report order.
	ScorecardTerminationClasses = []string{"contested", "replaced", "unassigned_nearby", "vanished", "other"}
)

// TrackScorecard is the whole result.
type TrackScorecard struct {
	SchemaVersion    int                  `json:"schema_version"`
	FramePeriodNanos int64                `json:"frame_period_nanos"`
	ScoredSeconds    float64              `json:"scored_seconds"`
	Population       ScorecardPopulation  `json:"population"`
	Coast            []ScorecardCoastCell `json:"coast"`
	Termination      ScorecardTermination `json:"termination"`
	Residuals        []ScorecardResidCell `json:"residuals"`
	Parameters       map[string]float64   `json:"parameters"`
}

// ScorecardPopulation describes the tracks born inside the scoring window.
type ScorecardPopulation struct {
	Tracks               int     `json:"tracks"`
	TracksPerMinute      float64 `json:"tracks_per_minute"`
	Estimates            int     `json:"estimates"`
	SingleEstimateTracks int     `json:"single_estimate_tracks"`
	CensoredAtEnd        int     `json:"censored_at_end"`
	LifetimeMedianSecs   float64 `json:"lifetime_median_seconds"`
	LifetimeP90Secs      float64 `json:"lifetime_p90_seconds"`
	ShareUnderOneSecond  float64 `json:"share_under_one_second"`
	// AssociationDensity is accepted estimates over the frames a track spanned:
	// 1.0 means it was associated on every frame of its life.
	AssociationDensity float64 `json:"association_density"`
	LifetimeHistogram  []int   `json:"lifetime_histogram_100ms"`
	Clusters           int     `json:"clusters"`
	ClustersUnassigned int     `json:"clusters_unassigned"`
}

// ScorecardCoastCell is the shadow coast error in one stratum: the
// constant-velocity prediction made from a track's state h seconds earlier,
// scored against the measurement the track later accepted. Nothing is withheld
// from the filter, so any motion model can be scored against the same
// reference.
type ScorecardCoastCell struct {
	HorizonSeconds float64 `json:"horizon_seconds"`
	SpeedFloorMps  float64 `json:"speed_floor_mps"`
	// Deceleration is "decelerating", "steady" or "unknown" (no state 0.5 s
	// before the start of the coast to compare with).
	Deceleration string  `json:"deceleration"`
	Count        int     `json:"count"`
	MeanMetres   float64 `json:"mean_metres"`
	RMSMetres    float64 `json:"rms_metres"`
	P50Metres    float64 `json:"p50_metres"`
	P95Metres    float64 `json:"p95_metres"`
	// AlongBiasMetres is the signed mean error along the direction of travel
	// at the start of the coast. Positive means the prediction overshot, which
	// is what a constant-velocity coast does to a braking vehicle.
	AlongBiasMetres float64 `json:"along_bias_metres"`
	CrossRMSMetres  float64 `json:"cross_rms_metres"`
}

// ScorecardTermination classifies how each track that ended inside the window
// ended, from what the clusters near its predicted position did next.
type ScorecardTermination struct {
	Ended int `json:"ended"`
	// ByClass and ByClassAndLifetime follow ScorecardTerminationClasses order.
	ByClass            []ScorecardClassCount    `json:"by_class"`
	ByClassAndLifetime []ScorecardClassLifeCell `json:"by_class_and_lifetime"`
}

type ScorecardClassCount struct {
	Class string  `json:"class"`
	Count int     `json:"count"`
	Share float64 `json:"share"`
}

type ScorecardClassLifeCell struct {
	Class             string  `json:"class"`
	LifetimeFloorSecs float64 `json:"lifetime_floor_seconds"`
	Count             int     `json:"count"`
}

// ScorecardResidCell is the innovation in the sensor frame for one stratum.
// The sensor is at the origin of the evidence frame (site calibration is a
// rotation), so radial is along the line of sight to the measurement.
type ScorecardResidCell struct {
	RangeFloorMetres float64 `json:"range_floor_metres"`
	PointsFloor      int     `json:"points_floor"`
	Moving           bool    `json:"moving"`
	Count            int     `json:"count"`
	RadialRMS        float64 `json:"radial_rms_metres"`
	TangentialRMS    float64 `json:"tangential_rms_metres"`
	RadialBias       float64 `json:"radial_bias_metres"`
	TangentialBias   float64 `json:"tangential_bias_metres"`
	MeanNIS          float64 `json:"mean_nis"`
	NISExceedance    float64 `json:"nis_exceedance_ratio"`
}

func bandIndex(floors []float64, v float64) int {
	idx := 0
	for i, f := range floors {
		if v >= f {
			idx = i
		}
	}
	return idx
}

func intBandIndex(floors []int, v int) int {
	idx := 0
	for i, f := range floors {
		if v >= f {
			idx = i
		}
	}
	return idx
}

// histogramQuantile returns the upper edge of the bin holding the q-quantile.
func histogramQuantile(hist []int, total int, q, binWidth float64) float64 {
	if total == 0 {
		return 0
	}
	target := int(math.Ceil(q * float64(total)))
	if target < 1 {
		target = 1
	}
	seen := 0
	for i, n := range hist {
		seen += n
		if seen >= target {
			return float64(i+1) * binWidth
		}
	}
	return float64(len(hist)) * binWidth
}

type coastAcc struct {
	n                             int
	sum, sumSq, alongSum, crossSq float64
	hist                          [scorecardCoastBins]int
}

type residAcc struct {
	n, exceed                        int
	radSum, radSq, tanSum, tanSq, ns float64
}

// ComputeTrackScorecard builds the scorecard. Inputs may arrive in any order.
func ComputeTrackScorecard(estimates []ScorecardEstimate, clusters []ScorecardCluster, opts ScorecardOptions) TrackScorecard {
	est := append([]ScorecardEstimate(nil), estimates...)
	sort.SliceStable(est, func(i, j int) bool {
		a, b := est[i], est[j]
		if a.CreationSequence != b.CreationSequence {
			return a.CreationSequence < b.CreationSequence
		}
		if a.FrameUnixNanos != b.FrameUnixNanos {
			return a.FrameUnixNanos < b.FrameUnixNanos
		}
		return a.ObservationID < b.ObservationID
	})
	cl := append([]ScorecardCluster(nil), clusters...)
	sort.SliceStable(cl, func(i, j int) bool {
		if cl[i].FrameUnixNanos != cl[j].FrameUnixNanos {
			return cl[i].FrameUnixNanos < cl[j].FrameUnixNanos
		}
		return cl[i].ObservationID < cl[j].ObservationID
	})

	out := TrackScorecard{SchemaVersion: 1, Parameters: map[string]float64{
		"near_metres": scorecardNearMetres, "far_metres": scorecardFarMetres,
		"lookahead_frames": scorecardLookaheadFrames, "censor_seconds": scorecardCensorSeconds,
		"decel_lookback_seconds": scorecardDecelLookback, "decel_threshold_mps": scorecardDecelThresholdMps,
		"horizon_tolerance_seconds": scorecardHorizonTolerance, "moving_mps": scorecardMovingMps,
		"nis_95": scorecardNIS95,
	}}
	// Every section exists from the start, so a scorecard with nothing in it
	// has one canonical encoding: empty arrays, never null.
	out.Coast = []ScorecardCoastCell{}
	out.Residuals = []ScorecardResidCell{}
	out.Population.LifetimeHistogram = make([]int, scorecardLifetimeBins)
	classCounts := make([]int, len(ScorecardTerminationClasses))
	classLife := make([]int, len(ScorecardTerminationClasses)*len(scorecardLifeFloors))
	finishTermination := func() {
		out.Termination.ByClass = []ScorecardClassCount{}
		out.Termination.ByClassAndLifetime = []ScorecardClassLifeCell{}
		for ci, name := range ScorecardTerminationClasses {
			share := 0.0
			if out.Termination.Ended > 0 {
				share = float64(classCounts[ci]) / float64(out.Termination.Ended)
			}
			out.Termination.ByClass = append(out.Termination.ByClass,
				ScorecardClassCount{Class: name, Count: classCounts[ci], Share: share})
			for li, lf := range scorecardLifeFloors {
				out.Termination.ByClassAndLifetime = append(out.Termination.ByClassAndLifetime,
					ScorecardClassLifeCell{Class: name, LifetimeFloorSecs: lf, Count: classLife[ci*len(scorecardLifeFloors)+li]})
			}
		}
	}

	// Frame timeline, from every frame that produced a cluster or an estimate.
	frameSet := map[int64]bool{}
	for _, c := range cl {
		frameSet[c.FrameUnixNanos] = true
	}
	for _, e := range est {
		frameSet[e.FrameUnixNanos] = true
	}
	frames := make([]int64, 0, len(frameSet))
	for f := range frameSet {
		frames = append(frames, f)
	}
	sort.Slice(frames, func(i, j int) bool { return frames[i] < frames[j] })
	if len(frames) < 2 {
		finishTermination()
		return out
	}
	diffs := make([]int64, 0, len(frames)-1)
	for i := 1; i < len(frames); i++ {
		diffs = append(diffs, frames[i]-frames[i-1])
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i] < diffs[j] })
	period := diffs[len(diffs)/2]
	out.FramePeriodNanos = period
	lastFrame := frames[len(frames)-1]
	scoringStart := opts.ScoringStartNanos
	if scoringStart < frames[0] {
		scoringStart = frames[0]
	}
	out.ScoredSeconds = float64(lastFrame-scoringStart) / 1e9

	// Lookups. Maps are only ever indexed, never ranged over.
	clustersByFrame := map[int64][]ScorecardCluster{}
	pointsByObservation := map[string]int{}
	for _, c := range cl {
		clustersByFrame[c.FrameUnixNanos] = append(clustersByFrame[c.FrameUnixNanos], c)
		pointsByObservation[c.ObservationID] = c.PointsCount
	}
	ownerByObservation := map[string]int64{}
	for _, e := range est {
		ownerByObservation[e.ObservationID] = e.CreationSequence
	}

	// Tracks: contiguous runs of est, which is sorted by creation sequence.
	type span struct{ lo, hi int }
	var tracks []span
	for i := 0; i < len(est); {
		j := i
		for j < len(est) && est[j].CreationSequence == est[i].CreationSequence {
			j++
		}
		tracks = append(tracks, span{i, j})
		i = j
	}
	firstSeen := map[int64]int64{}
	for _, t := range tracks {
		firstSeen[est[t.lo].CreationSequence] = est[t.lo].FrameUnixNanos
	}

	// --- Population ---
	pop := &out.Population
	for _, c := range cl {
		if c.FrameUnixNanos < scoringStart {
			continue
		}
		pop.Clusters++
		if _, owned := ownerByObservation[c.ObservationID]; !owned {
			pop.ClustersUnassigned++
		}
	}
	var lifetimes []int64
	var spannedFrames, spannedEstimates int64
	for _, t := range tracks {
		first, last := est[t.lo], est[t.hi-1]
		for i := t.lo; i < t.hi; i++ {
			if est[i].FrameUnixNanos >= scoringStart {
				pop.Estimates++
			}
		}
		if first.FrameUnixNanos < scoringStart {
			continue
		}
		pop.Tracks++
		life := last.FrameUnixNanos - first.FrameUnixNanos
		lifetimes = append(lifetimes, life)
		if t.hi-t.lo == 1 {
			pop.SingleEstimateTracks++
		}
		if float64(lastFrame-last.FrameUnixNanos)/1e9 < scorecardCensorSeconds {
			pop.CensoredAtEnd++
		}
		bin := int(float64(life) / 1e9 / scorecardLifetimeBinSeconds)
		if bin >= scorecardLifetimeBins {
			bin = scorecardLifetimeBins - 1
		}
		pop.LifetimeHistogram[bin]++
		spannedFrames += (life+period/2)/period + 1
		spannedEstimates += int64(t.hi - t.lo)
	}
	if len(lifetimes) > 0 {
		sort.Slice(lifetimes, func(i, j int) bool { return lifetimes[i] < lifetimes[j] })
		pop.LifetimeMedianSecs = float64(lifetimes[(len(lifetimes)-1)/2]) / 1e9
		pop.LifetimeP90Secs = float64(lifetimes[(len(lifetimes)-1)*9/10]) / 1e9
		under := 0
		for _, l := range lifetimes {
			if l < 1e9 {
				under++
			}
		}
		pop.ShareUnderOneSecond = float64(under) / float64(len(lifetimes))
	}
	if out.ScoredSeconds > 0 {
		pop.TracksPerMinute = float64(pop.Tracks) / out.ScoredSeconds * 60
	}
	if spannedFrames > 0 {
		pop.AssociationDensity = float64(spannedEstimates) / float64(spannedFrames)
	}

	// --- Shadow coast and stratified residuals ---
	nDecel := 3 // decelerating, steady, unknown
	coast := make([]coastAcc, len(scorecardHorizons)*len(scorecardSpeedFloors)*nDecel)
	coastIdx := func(h, s, d int) int { return (h*len(scorecardSpeedFloors)+s)*nDecel + d }
	resid := make([]residAcc, len(scorecardRangeFloors)*len(scorecardPointsFloors)*2)
	residIdx := func(r, p, m int) int { return (r*len(scorecardPointsFloors)+p)*2 + m }
	tolerance := int64(scorecardHorizonTolerance * 1e9)

	// stateAt finds the estimate of the track in [lo,hi) nearest to target,
	// within tolerance. Estimates in a track are in frame order.
	stateAt := func(lo, hi int, target int64) (int, bool) {
		k := lo + sort.Search(hi-lo, func(i int) bool { return est[lo+i].FrameUnixNanos >= target })
		best, bestDiff := -1, tolerance+1
		for _, c := range []int{k - 1, k} {
			if c < lo || c >= hi {
				continue
			}
			d := est[c].FrameUnixNanos - target
			if d < 0 {
				d = -d
			}
			if d < bestDiff {
				best, bestDiff = c, d
			}
		}
		return best, best >= 0
	}

	for _, t := range tracks {
		for j := t.lo; j < t.hi; j++ {
			e := est[j]
			if e.FrameUnixNanos < scoringStart {
				continue
			}
			// Stratified residual for this accepted association.
			rng := math.Hypot(e.MeasurementX, e.MeasurementY)
			if rng > 0 {
				ux, uy := e.MeasurementX/rng, e.MeasurementY/rng
				radial := e.InnovationX*ux + e.InnovationY*uy
				tangential := -e.InnovationX*uy + e.InnovationY*ux
				moving := 0
				if math.Hypot(e.VX, e.VY) >= scorecardMovingMps {
					moving = 1
				}
				a := &resid[residIdx(bandIndex(scorecardRangeFloors, rng),
					intBandIndex(scorecardPointsFloors, pointsByObservation[e.ObservationID]), moving)]
				a.n++
				a.radSum += radial
				a.radSq += radial * radial
				a.tanSum += tangential
				a.tanSq += tangential * tangential
				a.ns += e.NIS
				if e.NIS > scorecardNIS95 {
					a.exceed++
				}
			}

			// Shadow coast: predictions made from this track's earlier states.
			for hi, h := range scorecardHorizons {
				i, ok := stateAt(t.lo, j, e.FrameUnixNanos-int64(h*1e9))
				if !ok {
					continue
				}
				s := est[i]
				dt := float64(e.FrameUnixNanos-s.FrameUnixNanos) / 1e9
				ex := s.X + s.VX*dt - e.MeasurementX
				ey := s.Y + s.VY*dt - e.MeasurementY
				errMetres := math.Hypot(ex, ey)
				speed := math.Hypot(s.VX, s.VY)
				decel := 2 // unknown
				if p, ok := stateAt(t.lo, i, s.FrameUnixNanos-int64(scorecardDecelLookback*1e9)); ok {
					decel = 1
					if math.Hypot(est[p].VX, est[p].VY)-speed > scorecardDecelThresholdMps {
						decel = 0
					}
				}
				a := &coast[coastIdx(hi, bandIndex(scorecardSpeedFloors, speed), decel)]
				a.n++
				a.sum += errMetres
				a.sumSq += errMetres * errMetres
				if speed > 0 {
					hx, hy := s.VX/speed, s.VY/speed
					a.alongSum += ex*hx + ey*hy
					cross := -ex*hy + ey*hx
					a.crossSq += cross * cross
				}
				bin := int(errMetres / scorecardCoastBinMetres)
				if bin >= scorecardCoastBins {
					bin = scorecardCoastBins - 1
				}
				a.hist[bin]++
			}
		}
	}
	decelNames := []string{"decelerating", "steady", "unknown"}
	for hi, h := range scorecardHorizons {
		for si, sf := range scorecardSpeedFloors {
			for di := 0; di < nDecel; di++ {
				a := coast[coastIdx(hi, si, di)]
				if a.n == 0 {
					continue
				}
				n := float64(a.n)
				out.Coast = append(out.Coast, ScorecardCoastCell{
					HorizonSeconds: h, SpeedFloorMps: sf, Deceleration: decelNames[di], Count: a.n,
					MeanMetres: a.sum / n, RMSMetres: math.Sqrt(a.sumSq / n),
					P50Metres:       histogramQuantile(a.hist[:], a.n, 0.5, scorecardCoastBinMetres),
					P95Metres:       histogramQuantile(a.hist[:], a.n, 0.95, scorecardCoastBinMetres),
					AlongBiasMetres: a.alongSum / n, CrossRMSMetres: math.Sqrt(a.crossSq / n),
				})
			}
		}
	}
	for ri, rf := range scorecardRangeFloors {
		for pi, pf := range scorecardPointsFloors {
			for m := 0; m < 2; m++ {
				a := resid[residIdx(ri, pi, m)]
				if a.n == 0 {
					continue
				}
				n := float64(a.n)
				out.Residuals = append(out.Residuals, ScorecardResidCell{
					RangeFloorMetres: rf, PointsFloor: pf, Moving: m == 1, Count: a.n,
					RadialRMS: math.Sqrt(a.radSq / n), TangentialRMS: math.Sqrt(a.tanSq / n),
					RadialBias: a.radSum / n, TangentialBias: a.tanSum / n,
					MeanNIS: a.ns / n, NISExceedance: float64(a.exceed) / n,
				})
			}
		}
	}

	// --- Termination ---
	lookahead := int64(scorecardLookaheadFrames * float64(period))
	for _, t := range tracks {
		last := est[t.hi-1]
		if last.FrameUnixNanos < scoringStart {
			continue
		}
		if float64(lastFrame-last.FrameUnixNanos)/1e9 < scorecardCensorSeconds {
			continue // still alive, or too close to the end to tell
		}
		contested, replaced, unassignedNear, anyFar := false, false, false, false
		k := sort.Search(len(frames), func(i int) bool { return frames[i] > last.FrameUnixNanos })
		for ; k < len(frames) && frames[k]-last.FrameUnixNanos <= lookahead; k++ {
			dt := float64(frames[k]-last.FrameUnixNanos) / 1e9
			px, py := last.X+last.VX*dt, last.Y+last.VY*dt
			for _, c := range clustersByFrame[frames[k]] {
				d := math.Hypot(c.X-px, c.Y-py)
				if d <= scorecardFarMetres {
					anyFar = true
				}
				if d > scorecardNearMetres {
					continue
				}
				owner, owned := ownerByObservation[c.ObservationID]
				switch {
				case !owned:
					unassignedNear = true
				case firstSeen[owner] > last.FrameUnixNanos:
					replaced = true
				default:
					contested = true
				}
			}
		}
		class := 4 // other
		switch {
		case contested:
			class = 0
		case replaced:
			class = 1
		case unassignedNear:
			class = 2
		case !anyFar:
			class = 3
		}
		classCounts[class]++
		life := float64(last.FrameUnixNanos-est[t.lo].FrameUnixNanos) / 1e9
		classLife[class*len(scorecardLifeFloors)+bandIndex(scorecardLifeFloors, life)]++
		out.Termination.Ended++
	}
	finishTermination()
	return out
}
