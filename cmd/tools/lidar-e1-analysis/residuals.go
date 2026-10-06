package main

// E1.2 and E1.4, the two parts of experiment E1 the state-estimation plan left
// open after E1.1 and E1.3 confirmed the aspect-conditioned medoid bias.
//
// E1.4, the stationary noise floor, is the one place real data supplies an
// exact expected value. A body that is not moving has a constant true
// position, so the frame-to-frame scatter of any position candidate measured
// on it is that candidate's measurement noise and nothing else: no motion
// model, no filter, no association error. The shipped filter assumes one
// isotropic R (measurement_noise, 0.05 m^2, so 0.22 m per axis). This
// measures what the scatter is, per candidate and per range band, and prints
// the ratio.
//
// E1.2, the straight-segment residual spectrum, asks whether the lateral
// offset of a candidate from a straight reference path is white noise or
// structured. A deterministic bias that slides with aspect as a vehicle passes
// shows up as strong positive autocorrelation at short lags; measurement noise
// shows up as none. The statistic is the sample autocorrelation at lags 1-10
// and the Ljung-Box Q(10), compared with the chi-squared(10) 95% point,
// 18.307. The filter's own posterior estimate is tested beside the four
// observation candidates, since it is the series downstream code consumes.
//
// Both are descriptive. Tracks are accepted by the same rules E1.1 uses, so
// the populations are auditable, and nothing here fits or tunes anything.

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/banshee-data/velocity.report/internal/db"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

const (
	ljungBoxLags        = 10
	ljungBoxChi2_10_95  = 18.307
	stationaryMaxSpeed  = 0.3 // m/s: below this the track is treated as stationary
	stationaryMinFrames = 50
	whitenessMinFrames  = 30
	seriesMinCoverage   = 0.8 // share of a track's estimates that must carry the candidate
)

var rangeBandEdges = []float64{0, 20, 40, math.Inf(1)}

// candidateOrEstimate indexes the four observation candidates plus the filter
// posterior as a fifth series.
const seriesCount = int(candidateCount) + 1

func seriesName(i int) string {
	if i < int(candidateCount) {
		return candidate(i).String()
	}
	return "estimate"
}

type trackSeries struct {
	site      string
	seq       int64
	estimates []trackEstimate
}

// loadTrackSeries groups every persisted estimate by (source, creation
// sequence) in frame order. Estimates carry the filter posterior; the
// observation candidates are looked up from samples by observation ID.
func loadTrackSeries(dbPath string, sites map[string]string) ([]trackSeries, error) {
	database, err := db.NewDBWithMigrationCheck(dbPath, false)
	if err != nil {
		return nil, err
	}
	defer database.Close()
	store := observationsqlite.NewStateEstimateStore(database)

	sourceIDs := make([]string, 0, len(sites))
	for sourceID := range sites {
		sourceIDs = append(sourceIDs, sourceID)
	}
	sort.Strings(sourceIDs)

	var out []trackSeries
	for _, sourceID := range sourceIDs {
		estimates, err := store.ListBySource(sourceID)
		if err != nil {
			return nil, fmt.Errorf("listing estimates for %s: %w", sites[sourceID], err)
		}
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
		seqs := make([]int64, 0, len(byTrack))
		for seq := range byTrack {
			seqs = append(seqs, seq)
		}
		sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
		for _, seq := range seqs {
			track := byTrack[seq]
			sort.Slice(track, func(i, j int) bool { return track[i].frameUnixNanos < track[j].frameUnixNanos })
			out = append(out, trackSeries{site: sites[sourceID], seq: seq, estimates: track})
		}
	}
	return out, nil
}

// positions returns series i's (x, y) for each estimate that carries it, in
// frame order, and the share of the track's estimates it covers.
func positions(ts trackSeries, byID map[string]sample, i int) (xs, ys []float64, coverage float64) {
	for _, e := range ts.estimates {
		if i == int(candidateCount) {
			xs, ys = append(xs, e.x), append(ys, e.y)
			continue
		}
		s, ok := byID[e.observationID]
		if !ok || s.clipped || !s.available[i] {
			continue
		}
		xs, ys = append(xs, s.posX[i]), append(ys, s.posY[i])
	}
	if len(ts.estimates) == 0 {
		return nil, nil, 0
	}
	return xs, ys, float64(len(xs)) / float64(len(ts.estimates))
}

func meanSpeed(ts trackSeries) float64 {
	if len(ts.estimates) == 0 {
		return 0
	}
	sum := 0.0
	for _, e := range ts.estimates {
		sum += math.Hypot(e.vx, e.vy)
	}
	return sum / float64(len(ts.estimates))
}

func meanRange(ts trackSeries) float64 {
	if len(ts.estimates) == 0 {
		return 0
	}
	sum := 0.0
	for _, e := range ts.estimates {
		sum += math.Hypot(e.x, e.y)
	}
	return sum / float64(len(ts.estimates))
}

func rangeBand(r float64) string {
	for i := 0; i+1 < len(rangeBandEdges); i++ {
		if r >= rangeBandEdges[i] && r < rangeBandEdges[i+1] {
			if math.IsInf(rangeBandEdges[i+1], 1) {
				return fmt.Sprintf("%.0f+", rangeBandEdges[i])
			}
			return fmt.Sprintf("%.0f-%.0f", rangeBandEdges[i], rangeBandEdges[i+1])
		}
	}
	return "?"
}

// scatter2D is the principal standard deviations of a 2-D point set about
// its own mean: the noise floor of a stationary body's position candidate.
func scatter2D(xs, ys []float64) (major, minor float64) {
	n := float64(len(xs))
	mx, my := mean(xs), mean(ys)
	var sxx, syy, sxy float64
	for i := range xs {
		dx, dy := xs[i]-mx, ys[i]-my
		sxx += dx * dx
		syy += dy * dy
		sxy += dx * dy
	}
	sxx, syy, sxy = sxx/(n-1), syy/(n-1), sxy/(n-1)
	tr, det := sxx+syy, sxx*syy-sxy*sxy
	disc := math.Sqrt(math.Max(tr*tr/4-det, 0))
	return math.Sqrt(math.Max(tr/2+disc, 0)), math.Sqrt(math.Max(tr/2-disc, 0))
}

// autocorrelation returns the sample autocorrelation of v at lags 1..maxLag
// and the Ljung-Box Q statistic over those lags.
func autocorrelation(v []float64, maxLag int) (rho []float64, q float64) {
	n := len(v)
	m := mean(v)
	var denom float64
	for _, x := range v {
		denom += (x - m) * (x - m)
	}
	rho = make([]float64, maxLag)
	if denom == 0 || n <= maxLag+1 {
		return rho, 0
	}
	for k := 1; k <= maxLag; k++ {
		var num float64
		for i := k; i < n; i++ {
			num += (v[i] - m) * (v[i-k] - m)
		}
		rho[k-1] = num / denom
		q += rho[k-1] * rho[k-1] / float64(n-k)
	}
	q *= float64(n) * float64(n+2)
	return rho, q
}

type noiseFloorCell struct {
	Tracks           int       `json:"tracks"`
	Observations     int       `json:"observations"`
	MedianSigmaMajor float64   `json:"median_sigma_major_m"`
	MedianSigmaMinor float64   `json:"median_sigma_minor_m"`
	MedianSigmaIso   float64   `json:"median_sigma_isotropic_m"`
	sigmaMajor       []float64 `json:"-"`
	sigmaMinor       []float64 `json:"-"`
	sigmaIso         []float64 `json:"-"`
}

type whitenessCell struct {
	Tracks      int       `json:"tracks"`
	White       int       `json:"white_at_95"`
	WhiteShare  float64   `json:"white_share"`
	MedianRho1  float64   `json:"median_rho1"`
	MedianQ10   float64   `json:"median_q10"`
	MedianSigma float64   `json:"median_residual_sigma_m"`
	rho1        []float64 `json:"-"`
	q10         []float64 `json:"-"`
	sigma       []float64 `json:"-"`
}

type residualReport struct {
	MeasurementNoise    float64                               `json:"measurement_noise_m2"`
	SigmaRConfigured    float64                               `json:"sigma_r_configured_m"`
	StationaryMaxSpeed  float64                               `json:"stationary_max_speed_mps"`
	StationaryMinFrames int                                   `json:"stationary_min_frames"`
	StationaryTracks    int                                   `json:"stationary_tracks"`
	NoiseFloor          map[string]map[string]*noiseFloorCell `json:"e1_4_noise_floor"` // series -> band
	StraightTracks      int                                   `json:"straight_tracks"`
	Whiteness           map[string]*whitenessCell             `json:"e1_2_whiteness"` // series
	LjungBoxLags        int                                   `json:"ljung_box_lags"`
	LjungBoxCritical    float64                               `json:"ljung_box_critical_95"`
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// analyzeResidualStructure runs E1.4 and E1.2 over the loaded tracks.
func analyzeResidualStructure(tracks []trackSeries, samples []sample, cfg conditionalConfig, measurementNoise float64) residualReport {
	byID := make(map[string]sample, len(samples))
	for _, s := range samples {
		byID[s.observationID] = s
	}
	rep := residualReport{
		MeasurementNoise:    measurementNoise,
		SigmaRConfigured:    math.Sqrt(measurementNoise),
		StationaryMaxSpeed:  stationaryMaxSpeed,
		StationaryMinFrames: stationaryMinFrames,
		NoiseFloor:          map[string]map[string]*noiseFloorCell{},
		Whiteness:           map[string]*whitenessCell{},
		LjungBoxLags:        ljungBoxLags,
		LjungBoxCritical:    ljungBoxChi2_10_95,
	}
	for i := 0; i < seriesCount; i++ {
		rep.NoiseFloor[seriesName(i)] = map[string]*noiseFloorCell{}
		rep.Whiteness[seriesName(i)] = &whitenessCell{}
	}

	for _, ts := range tracks {
		n := len(ts.estimates)
		speed := meanSpeed(ts)

		// E1.4: stationary tracks.
		if n >= stationaryMinFrames && speed < stationaryMaxSpeed {
			rep.StationaryTracks++
			band := rangeBand(meanRange(ts))
			for i := 0; i < seriesCount; i++ {
				xs, ys, coverage := positions(ts, byID, i)
				if len(xs) < stationaryMinFrames || coverage < seriesMinCoverage {
					continue
				}
				major, minor := scatter2D(xs, ys)
				cell := rep.NoiseFloor[seriesName(i)][band]
				if cell == nil {
					cell = &noiseFloorCell{}
					rep.NoiseFloor[seriesName(i)][band] = cell
				}
				cell.Tracks++
				cell.Observations += len(xs)
				cell.sigmaMajor = append(cell.sigmaMajor, major)
				cell.sigmaMinor = append(cell.sigmaMinor, minor)
				cell.sigmaIso = append(cell.sigmaIso, math.Sqrt((major*major+minor*minor)/2))
			}
			continue
		}

		// E1.2: straight, moving tracks, accepted as E1.1 accepts them.
		if n < cfg.minEstimates || n < whitenessMinFrames {
			continue
		}
		path, ok := fitTrackPath(ts.estimates)
		if !ok || path.speedMps < cfg.minSpeedMps || path.residualRMS > cfg.maxResidualRMS {
			continue
		}
		rep.StraightTracks++
		for i := 0; i < seriesCount; i++ {
			xs, ys, coverage := positions(ts, byID, i)
			if len(xs) < whitenessMinFrames || coverage < seriesMinCoverage {
				continue
			}
			lateral := make([]float64, len(xs))
			for j := range xs {
				lateral[j] = path.lateralOffset(xs[j], ys[j])
			}
			rho, q := autocorrelation(lateral, ljungBoxLags)
			cell := rep.Whiteness[seriesName(i)]
			cell.Tracks++
			if q < ljungBoxChi2_10_95 {
				cell.White++
			}
			cell.rho1 = append(cell.rho1, rho[0])
			cell.q10 = append(cell.q10, q)
			cell.sigma = append(cell.sigma, stddev(lateral))
		}
	}

	for _, bands := range rep.NoiseFloor {
		for _, cell := range bands {
			cell.MedianSigmaMajor = median(cell.sigmaMajor)
			cell.MedianSigmaMinor = median(cell.sigmaMinor)
			cell.MedianSigmaIso = median(cell.sigmaIso)
		}
	}
	for _, cell := range rep.Whiteness {
		if cell.Tracks > 0 {
			cell.WhiteShare = float64(cell.White) / float64(cell.Tracks)
		}
		cell.MedianRho1 = median(cell.rho1)
		cell.MedianQ10 = median(cell.q10)
		cell.MedianSigma = median(cell.sigma)
	}
	return rep
}

func reportResidualStructure(rep residualReport, jsonOut string) error {
	fmt.Printf("\nExperiment E1.4: stationary noise floor (tracks with mean speed < %.1f m/s over >= %d frames)\n",
		rep.StationaryMaxSpeed, rep.StationaryMinFrames)
	fmt.Printf("  configured sigma_R = sqrt(%.3f) = %.3f m per axis\n", rep.MeasurementNoise, rep.SigmaRConfigured)
	fmt.Printf("  stationary tracks accepted: %d\n", rep.StationaryTracks)
	fmt.Printf("  %-15s %-8s %7s %8s %12s %12s %12s %8s\n", "series", "range m", "tracks", "obs", "sigma_major", "sigma_minor", "sigma_iso", "ratio")
	for i := 0; i < seriesCount; i++ {
		name := seriesName(i)
		bands := make([]string, 0, len(rep.NoiseFloor[name]))
		for b := range rep.NoiseFloor[name] {
			bands = append(bands, b)
		}
		sort.Strings(bands)
		for _, b := range bands {
			c := rep.NoiseFloor[name][b]
			fmt.Printf("  %-15s %-8s %7d %8d %12.3f %12.3f %12.3f %8.2f\n", name, b, c.Tracks, c.Observations,
				c.MedianSigmaMajor, c.MedianSigmaMinor, c.MedianSigmaIso, c.MedianSigmaIso/rep.SigmaRConfigured)
		}
	}

	fmt.Printf("\nExperiment E1.2: lateral residual whiteness on straight segments (Ljung-Box Q(%d) < %.3f)\n",
		rep.LjungBoxLags, rep.LjungBoxCritical)
	fmt.Printf("  straight tracks accepted: %d\n", rep.StraightTracks)
	fmt.Printf("  %-15s %7s %7s %11s %11s %10s %12s\n", "series", "tracks", "white", "white share", "median rho1", "median Q10", "median sigma")
	for i := 0; i < seriesCount; i++ {
		c := rep.Whiteness[seriesName(i)]
		fmt.Printf("  %-15s %7d %7d %11.2f %11.3f %10.1f %12.3f\n", seriesName(i), c.Tracks, c.White, c.WhiteShare,
			c.MedianRho1, c.MedianQ10, c.MedianSigma)
	}

	if jsonOut == "" {
		return nil
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(jsonOut, string(b)+"\n")
}
