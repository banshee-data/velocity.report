// Command lidar-e1-analysis runs Experiment E1 from Section 16.5 of
// docs/plans/lidar-state-estimation-plan.md against a corpus of immutable
// detection observations.
//
// E1 asks whether the medoid's lateral error is a deterministic function of
// aspect angle — the angle between the sensor bearing and the vehicle's body
// axis — rather than noise. That distinction decides whether Phase 2 proceeds
// as designed, because a deterministic geometric bias cannot be filtered out
// however good the estimator is.
//
// This implements E1.3, the cross-measurement disagreement test, which the plan
// nominates to run first: it needs no trajectory fit and no ground truth
// whatsoever, so it cannot be accused of the circularity that the fit-based
// tests have to argue their way out of. Under the null — each candidate
// measuring the same thing with independent noise — the pairwise differences
// between candidates have no structure in aspect angle. Under the hypothesis
// they are structured, because each candidate responds differently to which
// face happens to be visible.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// manifest maps a content-addressed source identity back to the case that
// produced it, so results can be reported per site.
type manifest struct {
	Cases []struct {
		ID       string `json:"id"`
		SourceID string `json:"source_id"`
	} `json:"cases"`
}

// candidate names one of the measurement definitions under comparison.
type candidate int

const (
	candMedoid candidate = iota
	candOBBCentre
	candNearestCorner
	candNearEdge
	candidateCount
)

func (c candidate) String() string {
	switch c {
	case candMedoid:
		return "medoid"
	case candOBBCentre:
		return "obb_centre"
	case candNearestCorner:
		return "nearest_corner"
	default:
		return "near_edge"
	}
}

// sample is one observation reduced to what E1 needs.
type sample struct {
	site          string
	observationID string
	// aspectDeg is folded to [0, 90]: 0 means the line of sight runs along the
	// body axis (end-on) and 90 means it is perpendicular (broadside). The body
	// axis is undirected, so the fold loses nothing.
	aspectDeg float64
	rangeM    float64
	widthM    float64
	lengthM   float64
	// lateral is each candidate's position projected onto the body's lateral
	// axis, which is where the W/2 bias the hypothesis predicts would appear.
	// E1.3 uses it directly; E1.1 needs the site-frame positions instead,
	// because its reference is a fitted path rather than the body's own axis.
	lateral [candidateCount]float64
	posX    [candidateCount]float64
	posY    [candidateCount]float64
	// available marks candidates that could be computed for this observation.
	available [candidateCount]bool
	retained  int
	clipped   bool
}

func main() {
	var (
		dbPath       = flag.String("observations", "", "observations.db written by lidar-state-estimation-baseline (required)")
		manifestPath = flag.String("manifest", "", "source manifest mapping source_id to case id (required)")
		minRetained  = flag.Int("min-retained", 30,
			"skip observations with fewer retained points: a face percentile is not meaningful below this")
		minPoints = flag.Int("min-points", 40,
			"skip clusters smaller than this: below it the object is noise or a distant fragment, not a road user")
		aspectBinDeg = flag.Float64("aspect-bin", 15, "aspect-angle bin width in degrees")
		jsonOut      = flag.String("json", "", "optional path for the machine-readable result")
		minSpeed     = flag.Float64("min-speed", 3.0, "E1.1: minimum mean track speed in m/s; a stationary track has no aspect sweep")
		maxResidual  = flag.Float64("max-residual", 0.5,
			"E1.1: maximum straight-line residual RMS in metres for a track to serve as a reference")
		minEstimates = flag.Int("min-estimates", 20,
			"E1.1: minimum estimates per track, which at 10 Hz is the plan's 2 second window")
		minCell = flag.Int("min-cell", 30, "E1.1: minimum observations before a cell is reported")
	)
	flag.Parse()

	if *dbPath == "" || *manifestPath == "" {
		fmt.Fprintln(os.Stderr, "both -observations and -manifest are required")
		os.Exit(1)
	}

	sites, err := readManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading manifest: %v\n", err)
		os.Exit(1)
	}

	samples, stats, err := loadSamples(*dbPath, sites, *minRetained, *minPoints)
	if err != nil {
		fmt.Fprintf(os.Stderr, "loading observations: %v\n", err)
		os.Exit(1)
	}

	report(samples, stats, *aspectBinDeg, *jsonOut)

	joined, joinStats, err := joinToTrackPaths(*dbPath, sites, samples, conditionalConfig{
		minSpeedMps:    *minSpeed,
		maxResidualRMS: *maxResidual,
		minEstimates:   *minEstimates,
		minCellCount:   *minCell,
		aspectBinDeg:   *aspectBinDeg,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "joining to track paths: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n%s\n", joinStats)
	reportConditionalMeans(joined, conditionalConfig{
		minSpeedMps:    *minSpeed,
		maxResidualRMS: *maxResidual,
		minEstimates:   *minEstimates,
		minCellCount:   *minCell,
		aspectBinDeg:   *aspectBinDeg,
	})
}

func readManifest(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	sites := map[string]string{}
	for _, c := range m.Cases {
		sites[c.SourceID] = c.ID
	}
	if len(sites) == 0 {
		return nil, fmt.Errorf("manifest lists no cases")
	}
	return sites, nil
}

// loadStats records why observations were excluded, so the reported population
// is auditable rather than whatever survived.
type loadStats struct {
	total          int
	noOBB          int
	tooFewPoints   int
	tooFewRetained int
	degenerateOBB  int
	tooClose       int
	groundClipped  int
	kept           int
	nearEdgeFailed map[string]int
	nearEdgeRank   map[int]int
	emptySources   []string
}

func loadSamples(dbPath string, sites map[string]string, minRetained, minPoints int) ([]sample, loadStats, error) {
	// Read through internal/db and the storage layer's own observation store:
	// only those packages may touch database/sql, and the store returns the
	// typed l4bobserve records, so a change to the evidence schema breaks this
	// tool at compile time rather than silently altering an experiment's
	// inputs.
	database, err := db.NewDBWithMigrationCheck(dbPath, false)
	if err != nil {
		return nil, loadStats{}, err
	}
	defer database.Close()
	store := observationsqlite.NewObservationStore(database)

	stats := loadStats{nearEdgeFailed: map[string]int{}, nearEdgeRank: map[int]int{}}
	var samples []sample

	// Ordered by case so the report is deterministic regardless of map order.
	sourceIDs := make([]string, 0, len(sites))
	for sourceID := range sites {
		sourceIDs = append(sourceIDs, sourceID)
	}
	sort.Strings(sourceIDs)

	for _, sourceID := range sourceIDs {
		site := sites[sourceID]
		observations, err := store.ListBySource(sourceID)
		if err != nil {
			return nil, stats, fmt.Errorf("listing observations for %s: %w", site, err)
		}
		if len(observations) == 0 {
			stats.emptySources = append(stats.emptySources, site)
			continue
		}

		for _, observation := range observations {
			rec := observation.Snapshot()
			cl := rec.Cluster
			stats.total++

			if cl.OBB == nil {
				stats.noOBB++
				continue
			}
			if cl.PointsCount < minPoints {
				stats.tooFewPoints++
				continue
			}
			if len(cl.RetainedPoints) < minRetained {
				stats.tooFewRetained++
				continue
			}
			if cl.OBB.Length <= 0 || cl.OBB.Width <= 0 {
				stats.degenerateOBB++
				continue
			}
			// E1's grade mitigation, step 2: a ground-clipped observation is
			// reported as its own stratum rather than mixed into the
			// conditional means.
			if cl.GroundClipped {
				stats.groundClipped++
			}

			s := sample{
				site:          site,
				observationID: rec.ObservationID,
				widthM:        float64(cl.OBB.Width),
				lengthM:       float64(cl.OBB.Length),
				retained:      len(cl.RetainedPoints),
				clipped:       cl.GroundClipped,
			}

			// The sensor sits at the origin of the site frame: the calibration
			// is a yaw rotation about it.
			cx, cy := float64(cl.OBB.CenterX), float64(cl.OBB.CenterY)
			s.rangeM = math.Hypot(cx, cy)
			if s.rangeM < 1 {
				stats.tooClose++
				continue
			}
			bearing := math.Atan2(cy, cx)
			s.aspectDeg = foldAspectDeg(float64(cl.OBB.HeadingRad) - bearing)

			// The body's lateral axis, perpendicular to its heading. Every
			// candidate is projected onto it, so the comparison happens in the
			// frame where the predicted bias lives.
			latX := -math.Sin(float64(cl.OBB.HeadingRad))
			latY := math.Cos(float64(cl.OBB.HeadingRad))
			project := func(x, y float64) float64 { return x*latX + y*latY }

			record := func(c candidate, x, y float64) {
				s.lateral[c] = project(x, y)
				s.posX[c], s.posY[c] = x, y
				s.available[c] = true
			}
			record(candMedoid, float64(cl.CentroidX), float64(cl.CentroidY))
			record(candOBBCentre, cx, cy)

			if nx, ny, ok := nearestCorner(cl.OBB.CenterX, cl.OBB.CenterY,
				cl.OBB.Length, cl.OBB.Width, cl.OBB.HeadingRad); ok {
				record(candNearestCorner, float64(nx), float64(ny))
			}

			// The near-edge candidate, through the production implementation,
			// so this measures the shipped code rather than a restatement.
			set := l5tracks.MeasureNearEdge(l5tracks.NearEdgeInput{
				Cluster: cl,
				Points:  cl.RetainedPoints,
				SensorX: 0, SensorY: 0,
				HeadingRad: cl.OBB.HeadingRad,
				// The observed OBB extents stand in for a dimension belief.
				// This is the weakest part of an observation-only analysis:
				// those extents are themselves wrong when only part of the
				// body is visible, so the near-edge candidate is handicapped
				// here relative to one driven by an accumulated belief. It is
				// the honest choice, because an observation carries no track
				// history by design.
				HalfLength:       cl.OBB.Length / 2,
				HalfWidth:        cl.OBB.Width / 2,
				LengthProvenance: l5tracks.ProvenanceObserved,
				WidthProvenance:  l5tracks.ProvenanceObserved,
				PredictedX:       cl.OBB.CenterX,
				PredictedY:       cl.OBB.CenterY,
			})
			if set.Rank > 0 {
				record(candNearEdge, float64(set.CentreX), float64(set.CentreY))
				stats.nearEdgeRank[set.Rank]++
			} else {
				reason := set.FallbackReason
				if reason == "" {
					reason = "unknown"
				}
				stats.nearEdgeFailed[reason]++
			}

			stats.kept++
			samples = append(samples, s)
		}
	}
	return samples, stats, nil
}

// nearestCorner is the OBB corner closest to the sensor at the origin.
func nearestCorner(cx, cy, length, width, heading float32) (float32, float32, bool) {
	cos := float32(math.Cos(float64(heading)))
	sin := float32(math.Sin(float64(heading)))
	axisX, axisY := cos, sin
	perpX, perpY := -sin, cos

	best := [2]float32{}
	bestDist := math.Inf(1)
	found := false
	for _, sl := range [2]float32{-1, 1} {
		for _, sw := range [2]float32{-1, 1} {
			x := cx + sl*length/2*axisX + sw*width/2*perpX
			y := cy + sl*length/2*axisY + sw*width/2*perpY
			if d := math.Hypot(float64(x), float64(y)); d < bestDist {
				bestDist, best, found = d, [2]float32{x, y}, true
			}
		}
	}
	return best[0], best[1], found
}

// foldAspectDeg folds a signed angle to [0, 90] degrees. A body axis is
// undirected and the two lateral faces are symmetric, so aspect 100 degrees and
// aspect 80 degrees present the same geometry.
func foldAspectDeg(rad float64) float64 {
	deg := math.Abs(math.Mod(math.Abs(rad*180/math.Pi), 180))
	if deg > 90 {
		deg = 180 - deg
	}
	return deg
}

// --- reporting -------------------------------------------------------------

type binResult struct {
	Site        string  `json:"site"`
	AspectLoDeg float64 `json:"aspect_lo_deg"`
	AspectHiDeg float64 `json:"aspect_hi_deg"`
	Count       int     `json:"count"`
	// MeanDisagreement and its spread, per candidate pair, in metres along the
	// body's lateral axis.
	MeanMedoidMinusOBB      float64 `json:"mean_medoid_minus_obb_m"`
	StdMedoidMinusOBB       float64 `json:"std_medoid_minus_obb_m"`
	MeanMedoidMinusNearEdge float64 `json:"mean_medoid_minus_near_edge_m"`
	MeanOBBMinusNearEdge    float64 `json:"mean_obb_minus_near_edge_m"`
	MeanHalfWidth           float64 `json:"mean_half_width_m"`
	// MedoidBiasAsHalfWidthFraction expresses the medoid's disagreement with
	// the near edge as a fraction of the body's own half-width, which is the
	// quantity the hypothesis predicts approaches 1.
	MedoidBiasAsHalfWidthFraction float64 `json:"medoid_bias_as_half_width_fraction"`
	MeanRangeM                    float64 `json:"mean_range_m"`
}

type result struct {
	SchemaVersion int         `json:"schema_version"`
	Test          string      `json:"test"`
	Population    interface{} `json:"population"`
	Bins          []binResult `json:"bins"`
}

func report(samples []sample, stats loadStats, aspectBinDeg float64, jsonOut string) {
	fmt.Printf("Experiment E1.3: cross-measurement disagreement\n")
	fmt.Printf("  observations read        : %d\n", stats.total)
	fmt.Printf("  kept                     : %d\n", stats.kept)
	fmt.Printf("  excluded, no OBB         : %d\n", stats.noOBB)
	fmt.Printf("  excluded, small cluster  : %d\n", stats.tooFewPoints)
	fmt.Printf("  excluded, few retained   : %d\n", stats.tooFewRetained)
	fmt.Printf("  excluded, degenerate OBB : %d\n", stats.degenerateOBB)
	fmt.Printf("  excluded, inside 1 m     : %d\n", stats.tooClose)
	fmt.Printf("  ground-clipped stratum   : %d\n", stats.groundClipped)
	if len(stats.nearEdgeRank) > 0 {
		fmt.Printf("  near-edge rank           :")
		for _, r := range []int{1, 2} {
			if n := stats.nearEdgeRank[r]; n > 0 {
				fmt.Printf(" rank%d=%d", r, n)
			}
		}
		fmt.Println()
	}
	for _, site := range stats.emptySources {
		fmt.Printf("  no observations for       : %s\n", site)
	}
	if len(stats.nearEdgeFailed) > 0 {
		fmt.Printf("  near-edge unavailable    :")
		for reason, n := range stats.nearEdgeFailed {
			fmt.Printf(" %s=%d", reason, n)
		}
		fmt.Println()
	}
	if stats.kept == 0 {
		fmt.Printf("\nNothing to report.\n")
		return
	}

	bySite := map[string][]sample{}
	for _, s := range samples {
		bySite[s.site] = append(bySite[s.site], s)
	}
	siteNames := make([]string, 0, len(bySite))
	for name := range bySite {
		siteNames = append(siteNames, name)
	}
	sort.Strings(siteNames)

	out := result{SchemaVersion: 1, Test: "E1.3"}
	nBins := int(math.Ceil(90 / aspectBinDeg))

	for _, site := range siteNames {
		fmt.Printf("\n%s (%d observations)\n\n", site, len(bySite[site]))
		fmt.Printf("  %-14s %7s %12s %12s %12s %10s %9s\n",
			"aspect", "n", "med-obb", "med-near", "obb-near", "bias/(W/2)", "range")

		for b := 0; b < nBins; b++ {
			lo := float64(b) * aspectBinDeg
			hi := lo + aspectBinDeg

			var medObb, medNear, obbNear, halfWidth, ranges []float64
			for _, s := range bySite[site] {
				if s.clipped || s.aspectDeg < lo || s.aspectDeg >= hi {
					continue
				}
				if s.available[candMedoid] && s.available[candOBBCentre] {
					medObb = append(medObb, s.lateral[candMedoid]-s.lateral[candOBBCentre])
				}
				if s.available[candMedoid] && s.available[candNearEdge] {
					medNear = append(medNear, s.lateral[candMedoid]-s.lateral[candNearEdge])
					halfWidth = append(halfWidth, s.widthM/2)
				}
				if s.available[candOBBCentre] && s.available[candNearEdge] {
					obbNear = append(obbNear, s.lateral[candOBBCentre]-s.lateral[candNearEdge])
				}
				ranges = append(ranges, s.rangeM)
			}
			if len(medObb) == 0 {
				continue
			}

			fraction := math.NaN()
			if m := mean(halfWidth); m > 0 {
				fraction = math.Abs(mean(medNear)) / m
			}
			fmt.Printf("  %5.0f-%-8.0f %7d %12.3f %12.3f %12.3f %10.2f %9.1f\n",
				lo, hi, len(medObb), mean(medObb), mean(medNear), mean(obbNear), fraction, mean(ranges))

			out.Bins = append(out.Bins, binResult{
				Site: site, AspectLoDeg: lo, AspectHiDeg: hi, Count: len(medObb),
				MeanMedoidMinusOBB:            mean(medObb),
				StdMedoidMinusOBB:             stddev(medObb),
				MeanMedoidMinusNearEdge:       mean(medNear),
				MeanOBBMinusNearEdge:          mean(obbNear),
				MeanHalfWidth:                 mean(halfWidth),
				MedoidBiasAsHalfWidthFraction: fraction,
				MeanRangeM:                    mean(ranges),
			})
		}
	}

	reportRangeStratified(bySite, siteNames, aspectBinDeg)

	fmt.Printf("\nRead this as: under the null, each candidate measures the same place with\n")
	fmt.Printf("independent noise, so every pairwise mean is zero in every aspect bin and\n")
	fmt.Printf("the bias fraction is flat. Under Section 3's hypothesis the disagreement is\n")
	fmt.Printf("structured in aspect, and the medoid's departure from the near edge grows\n")
	fmt.Printf("toward the body's own half-width as one face comes to dominate.\n")

	if jsonOut != "" {
		out.Population = map[string]int{
			"read": stats.total, "kept": stats.kept,
			"ground_clipped": stats.groundClipped,
		}
		raw, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "encoding result: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(jsonOut, append(raw, '\n'), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "writing %s: %v\n", jsonOut, err)
			os.Exit(1)
		}
		fmt.Printf("\nwrote %s\n", jsonOut)
	}
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func stddev(v []float64) float64 {
	if len(v) < 2 {
		return math.NaN()
	}
	m := mean(v)
	sum := 0.0
	for _, x := range v {
		sum += (x - m) * (x - m)
	}
	return math.Sqrt(sum / float64(len(v)-1))
}

// rangeStrata are the range bands E1's step-3 mitigation stratifies by.
//
// Range and aspect are correlated in real traffic — a vehicle is nearest when
// it is broadside — so an aspect-conditioned mean computed over all ranges
// cannot distinguish the geometric bias E1 is looking for from a
// range-correlated artefact such as the P11 grade defect. The hypothesis
// predicts a bias that persists *within* a range stratum; a confound predicts
// one that vanishes once range is controlled.
var rangeStrata = []struct {
	lo, hi float64
	name   string
}{
	{0, 15, "0-15 m"},
	{15, 25, "15-25 m"},
	{25, 40, "25-40 m"},
	{40, 200, "40+ m"},
}

func reportRangeStratified(bySite map[string][]sample, siteNames []string, aspectBinDeg float64) {
	fmt.Printf("\nStratified by range (E1 step 3). The question is whether the\n")
	fmt.Printf("aspect trend survives inside a single range band, which is what\n")
	fmt.Printf("separates a geometric bias from a range-correlated artefact.\n")
	fmt.Printf("Cells show mean (medoid - near edge) as a fraction of the body's\n")
	fmt.Printf("own half-width, with the observation count beneath.\n")

	nBins := int(math.Ceil(90 / aspectBinDeg))

	for _, site := range siteNames {
		fmt.Printf("\n%s\n\n", site)
		fmt.Printf("  %-10s", "range")
		for b := 0; b < nBins; b++ {
			fmt.Printf(" %11s", fmt.Sprintf("%.0f-%.0f", float64(b)*aspectBinDeg, float64(b+1)*aspectBinDeg))
		}
		fmt.Println()

		for _, stratum := range rangeStrata {
			fmt.Printf("  %-10s", stratum.name)
			counts := make([]int, nBins)
			for b := 0; b < nBins; b++ {
				lo := float64(b) * aspectBinDeg
				hi := lo + aspectBinDeg
				var diffs, halves []float64
				for _, s := range bySite[site] {
					if s.clipped || s.aspectDeg < lo || s.aspectDeg >= hi {
						continue
					}
					if s.rangeM < stratum.lo || s.rangeM >= stratum.hi {
						continue
					}
					if !s.available[candMedoid] || !s.available[candNearEdge] {
						continue
					}
					diffs = append(diffs, s.lateral[candMedoid]-s.lateral[candNearEdge])
					halves = append(halves, s.widthM/2)
				}
				counts[b] = len(diffs)
				// A cell with too few observations reports nothing rather than
				// a mean that reads as evidence.
				if len(diffs) < 20 {
					fmt.Printf(" %11s", "-")
					continue
				}
				fmt.Printf(" %11.2f", math.Abs(mean(diffs))/mean(halves))
			}
			fmt.Println()
			fmt.Printf("  %-10s", "")
			for b := 0; b < nBins; b++ {
				if counts[b] == 0 {
					fmt.Printf(" %11s", "")
					continue
				}
				fmt.Printf(" %11s", fmt.Sprintf("(n=%d)", counts[b]))
			}
			fmt.Println()
		}
	}
}

// writeFile is a tiny helper the tests use to build manifest fixtures.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
