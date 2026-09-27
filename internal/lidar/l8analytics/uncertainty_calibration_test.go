package l8analytics

import (
	"encoding/json"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestChiSquaredHelpers(t *testing.T) {
	for _, c := range []struct {
		dof  int
		p    float64
		want float64
	}{{1, 0.95, 3.841459}, {2, 0.95, 5.991465}, {2, 0.99, 9.210340}, {1, 0.5, 0.454936}} {
		if got := chiSquaredQuantile(c.dof, c.p); !near(got, c.want, 1e-5) {
			t.Errorf("quantile dof=%d p=%v: %v, want %v", c.dof, c.p, got, c.want)
		}
	}
	for _, x := range []float64{0.1, 1, 5.991465, 20, 60} {
		if got := chiSquaredSurvival(x, 2); !near(got, math.Exp(-x/2), 1e-12) {
			t.Errorf("survival chi2(2) at %v: %v, want %v", x, got, math.Exp(-x/2))
		}
	}
	// Tabulated critical values: chi2(9) at p = 0.01 and chi2(1) at p = 0.05.
	if got := chiSquaredSurvival(21.665994, 9); !near(got, 0.01, 1e-6) {
		t.Errorf("chi2(9) survival at 21.666: %v, want 0.01", got)
	}
	if got := chiSquaredSurvival(3.841459, 1); !near(got, 0.05, 1e-6) {
		t.Errorf("chi2(1) survival at 3.841: %v, want 0.05", got)
	}
	if got := chiSquaredSurvival(0, 3); got != 1 {
		t.Errorf("survival at zero: %v", got)
	}
	if got := regularisedGammaQ(1, 3); !near(got, math.Exp(-3), 1e-12) {
		t.Errorf("Q(1, 3) = %v", got)
	}
	if !math.IsNaN(regularisedGammaQ(-1, 1)) {
		t.Error("Q with a non-positive shape is not NaN")
	}
}

// drawSample builds one sample whose innovation is a true draw from
// N(0, P + R) along each axis, with P diagonal in the sensor frame so the joint
// NIS the model would compute is the sum of the axis NIS. modelScale scales
// the R the "filter" believes relative to the truth.
func drawSample(rng *rand.Rand, rangeM float32, support int, aspect float32, predR, predT, trueR, trueT, physR, physT, modelScale float64) l5tracks.UncertaintySample {
	yr := rng.NormFloat64() * math.Sqrt(predR+trueR)
	yt := rng.NormFloat64() * math.Sqrt(predT+trueT)
	modelR, modelT := trueR*modelScale, trueT*modelScale
	nis := yr*yr/(predR+modelR) + yt*yt/(predT+modelT)
	return l5tracks.UncertaintySample{
		Source: l5tracks.MeasurementMedoidV0, Rank: 2, RangeMetres: rangeM, Support: support, AspectRad: aspect,
		InnovRadial: float32(yr), InnovTangential: float32(yt),
		PredRadial: float32(predR), PredTangential: float32(predT),
		NoiseRadial: float32(modelR), NoiseTangential: float32(modelT),
		PhysRadial: float32(physR), PhysTangential: float32(physT),
		NIS: float32(nis), Assigned: nis <= 36, Gated: nis > 36,
	}
}

func cellOf(t *testing.T, fit UncertaintyFit, rangeBin, supportBin, aspectBin int) CalibrationCell {
	t.Helper()
	for _, c := range fit.Calibration.Cells {
		if c.Source == string(l5tracks.MeasurementMedoidV0) && c.RangeBin == rangeBin && c.SupportBin == supportBin && c.AspectBin == aspectBin {
			return c
		}
	}
	t.Fatalf("no cell %d/%d/%d", rangeBin, supportBin, aspectBin)
	return CalibrationCell{}
}

// With innovations drawn from a known P + R, the fit recovers R minus the
// physics term in each stratum, per axis.
func TestFitRecoversKnownNoise(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	var samples []l5tracks.UncertaintySample
	// Near, dense, broadside: tight in both axes.
	for i := 0; i < 4000; i++ {
		samples = append(samples, drawSample(rng, 15, 150, 1.5, 0.02, 0.02, 0.01, 0.04, 0.0005, 0.001, 1))
	}
	// Far, sparse, end-on: bearing much worse than range.
	for i := 0; i < 4000; i++ {
		samples = append(samples, drawSample(rng, 40, 20, 0.1, 0.03, 0.03, 0.02, 0.5, 0.0005, 0.005, 1))
	}
	fit := FitNoiseCoefficients(samples, DefaultUncertaintyFitOptions(0.05))
	nearCell := cellOf(t, fit, 1, 3, 2)
	farCell := cellOf(t, fit, 3, 1, 0)
	for _, c := range []struct {
		name       string
		got        float32
		want       float64
		provenance string
	}{
		{"near radial", nearCell.RadialM2, 0.01 - 0.0005, nearCell.RadialProvenance},
		{"near tangential", nearCell.TangentialM2, 0.04 - 0.001, nearCell.TangentialProvenance},
		{"far radial", farCell.RadialM2, 0.02 - 0.0005, farCell.RadialProvenance},
		{"far tangential", farCell.TangentialM2, 0.5 - 0.005, farCell.TangentialProvenance},
	} {
		if !near(float64(c.got), c.want, 0.1*c.want+0.002) || c.provenance != ProvenanceFitted {
			t.Errorf("%s: %v (%s), want %v fitted", c.name, c.got, c.provenance, c.want)
		}
	}
	// An empty stratum of a fitted source takes the pooled coefficient.
	if empty := cellOf(t, fit, 0, 0, 1); empty.RadialProvenance != ProvenancePooledSource {
		t.Errorf("empty stratum provenance %q", empty.RadialProvenance)
	}
	// A source with no samples at all takes the prior.
	for _, c := range fit.Calibration.Cells {
		if c.Source == string(l5tracks.MeasurementOBBCentreV1) && (c.RadialM2 != 0.05 || c.RadialProvenance != ProvenancePrior) {
			t.Fatalf("unsampled source cell %+v", c)
		}
	}
}

func TestFitBacksOffBelowTheMinimum(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	opts := DefaultUncertaintyFitOptions(0.05)
	var samples []l5tracks.UncertaintySample
	for i := 0; i < opts.MinStratumSamples-1; i++ {
		samples = append(samples, drawSample(rng, 15, 150, 1.5, 0.02, 0.02, 0.3, 0.3, 0, 0, 1))
	}
	fit := FitNoiseCoefficients(samples, opts)
	cell := cellOf(t, fit, 1, 3, 2)
	if cell.RadialProvenance != ProvenancePrior || cell.RadialM2 != 0.05 {
		t.Errorf("too few samples anywhere: %+v, want the prior", cell)
	}
	if len(fit.Strata) != 2 || fit.Strata[0].Provenance != ProvenancePrior || fit.Strata[0].Count != opts.MinStratumSamples-1 {
		t.Errorf("under-sampled stratum not reported as such: %+v", fit.Strata)
	}

	// Enough in total, split so no one stratum has enough: pooled.
	samples = samples[:0]
	for i := 0; i < 40; i++ {
		r := float32(5 + 10*(i%4))
		samples = append(samples, drawSample(rng, r, 150, 1.5, 0.02, 0.02, 0.3, 0.3, 0, 0, 1))
	}
	fit = FitNoiseCoefficients(samples, opts)
	if cell := cellOf(t, fit, 0, 3, 2); cell.TangentialProvenance != ProvenancePooledSource {
		t.Errorf("split samples: %+v, want the pooled source fit", cell)
	}
}

// A stratum whose innovation carries a steady offset gets R from its spread,
// not from the offset, and is flagged: bias is corrected in the measurement
// model, never inflated into R.
func TestFitRefusesToFoldBiasIntoR(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	var samples []l5tracks.UncertaintySample
	for i := 0; i < 2000; i++ {
		s := drawSample(rng, 15, 150, 1.5, 0.02, 0.02, 0.01, 0.01, 0, 0, 1)
		s.InnovRadial += 0.6
		samples = append(samples, s)
	}
	fit := FitNoiseCoefficients(samples, DefaultUncertaintyFitOptions(0.05))
	cell := cellOf(t, fit, 1, 3, 2)
	if !near(float64(cell.RadialM2), 0.01, 0.003) {
		t.Errorf("biased stratum coefficient %v, want about the true 0.01, not 0.36 more", cell.RadialM2)
	}
	var radial FitStratum
	for _, s := range fit.Strata {
		if s.Axis == "radial" && s.RangeBin == 1 {
			radial = s
		}
	}
	if !radial.BiasDominated || !near(radial.MeanInnovation, 0.6, 0.01) {
		t.Errorf("bias not reported: %+v", radial)
	}

	// Too few samples to judge: a stratum of one always has its whole
	// innovation as its centre, and must not be flagged for it. kirk0's first
	// evidence run flagged 50 of 60 strata this way.
	few := FitNoiseCoefficients(samples[:DefaultUncertaintyFitOptions(0.05).MinStratumSamples-1], DefaultUncertaintyFitOptions(0.05))
	for _, s := range few.Strata {
		if s.BiasDominated {
			t.Errorf("under-sampled stratum flagged: %+v", s)
		}
	}
	robust := DefaultUncertaintyFitOptions(0.05)
	robust.Estimator = EstimatorMedian
	for _, s := range FitNoiseCoefficients(samples, robust).Strata {
		if s.Axis == "radial" && !s.BiasDominated {
			t.Errorf("median estimator missed the bias: %+v", s)
		}
	}
}

func TestFitClampsAndRecordsIt(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	var samples []l5tracks.UncertaintySample
	for i := 0; i < 500; i++ {
		// The predicted covariance already exceeds the innovation spread.
		s := drawSample(rng, 15, 150, 1.5, 0.001, 0.001, 0.001, 30, 0, 0, 1)
		s.PredRadial = 0.5
		samples = append(samples, s)
	}
	fit := FitNoiseCoefficients(samples, DefaultUncertaintyFitOptions(0.05))
	cell := cellOf(t, fit, 1, 3, 2)
	if cell.RadialM2 != 0.0025 || cell.TangentialM2 != 4 {
		t.Errorf("clamped cell %+v, want the floor and the ceiling", cell)
	}
	clamped := map[string]string{}
	for _, s := range fit.Strata {
		clamped[s.Axis] = s.Clamped
	}
	if clamped["radial"] != "floor" || clamped["tangential"] != "ceiling" {
		t.Errorf("clamps not recorded: %v", clamped)
	}
}

func consistentSamples(seed int64, n int, modelScale float64) []l5tracks.UncertaintySample {
	rng := rand.New(rand.NewSource(seed))
	out := make([]l5tracks.UncertaintySample, 0, n)
	for i := 0; i < n; i++ {
		r := float32(5 + rng.Float64()*55)
		support := 5 + rng.Intn(300)
		aspect := float32(rng.Float64() * 2 * math.Pi)
		out = append(out, drawSample(rng, r, support, aspect, 0.02, 0.02, 0.03, 0.06, 0.0005, 0.002, modelScale))
	}
	return out
}

func checkStatus(r UncertaintyReport, id string) string {
	for _, c := range r.GateChecks {
		if c.ID == id {
			return c.Status
		}
	}
	return "missing"
}

// A model that is right passes every label-free row; one four times
// overconfident fails the mean, the fit and the coverage.
func TestReportJudgesConsistency(t *testing.T) {
	good, err := BuildUncertaintyReport(UncertaintyInput{Samples: consistentSamples(5, 5000, 1), NoiseModel: "test",
		Options: DefaultUncertaintyFitOptions(0.05)})
	if err != nil {
		t.Fatal(err)
	}
	joint := good.PreGate[0]
	if joint.Dimension != "joint" || joint.DOF != 2 || !near(joint.MeanNISOverDOF, 1, 0.05) || !near(joint.Coverage95, 0.95, 0.01) {
		t.Errorf("consistent joint %+v", joint)
	}
	if joint.GOF == nil || joint.GOF.PValue < 0.01 {
		t.Errorf("consistent model rejected by the fit test: %+v", joint.GOF)
	}
	for _, id := range []string{"nis_mean_joint_m2", "gof_joint_m2", "nis_mean_radial_m1", "nis_mean_tangential_m1",
		"range_deciles", "support_deciles", "aspect_octants", "sample_not_truncated"} {
		if s := checkStatus(good, id); s != CheckWithin {
			t.Errorf("consistent model: %s is %s", id, s)
		}
	}
	if checkStatus(good, "manoeuvres_falsely_gated") != CheckRequiresLabels || !strings.HasPrefix(good.GateVerdict, "not_assessed") {
		t.Error("the report claims more than a label-free replay can")
	}
	if len(good.RangeDeciles) != 10 || len(good.AspectOctants) != 8 {
		t.Errorf("%d range deciles, %d octants", len(good.RangeDeciles), len(good.AspectOctants))
	}

	bad, err := BuildUncertaintyReport(UncertaintyInput{Samples: consistentSamples(5, 5000, 0.25), NoiseModel: "test",
		Options: DefaultUncertaintyFitOptions(0.05)})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"nis_mean_joint_m2", "gof_joint_m2"} {
		if s := checkStatus(bad, id); s != CheckOutside {
			t.Errorf("overconfident model: %s is %s", id, s)
		}
	}
}

// The plan's warning: errors at both ends of the range envelope can cancel in
// the aggregate. The decile check must catch what the mean misses.
func TestRangeDecilesCatchCancellingErrors(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	var samples []l5tracks.UncertaintySample
	for i := 0; i < 6000; i++ {
		r := float32(5 + rng.Float64()*55)
		scale := 3.0 // underconfident near
		if r > 32 {
			scale = 0.45 // overconfident far
		}
		samples = append(samples, drawSample(rng, r, 100, 1.5, 0.02, 0.02, 0.03, 0.06, 0, 0, scale))
	}
	report, err := BuildUncertaintyReport(UncertaintyInput{Samples: samples, Options: DefaultUncertaintyFitOptions(0.05)})
	if err != nil {
		t.Fatal(err)
	}
	if !near(report.PreGate[0].MeanNISOverDOF, 1, 0.4) {
		t.Fatalf("the aggregate does not cancel as intended: %+v", report.PreGate[0])
	}
	if s := checkStatus(report, "range_deciles"); s != CheckOutside {
		t.Errorf("range deciles %s; cancelling errors went unseen", s)
	}
	if s := checkStatus(report, "support_deciles"); s == CheckOutside {
		t.Errorf("support deciles %s, but support was constant", s)
	}
}

// The gate removes the tail, so accepted-only NIS reads low against the
// pre-gate population.
func TestAcceptedOnlyIsCensored(t *testing.T) {
	samples := consistentSamples(7, 3000, 0.5)
	for i := range samples {
		samples[i].Assigned = samples[i].NIS <= 5.991
		samples[i].Gated = !samples[i].Assigned
	}
	report, err := BuildUncertaintyReport(UncertaintyInput{Samples: samples, Options: DefaultUncertaintyFitOptions(0.05)})
	if err != nil {
		t.Fatal(err)
	}
	if !(report.AcceptedOnly[0].MeanNIS < report.PreGate[0].MeanNIS) || report.AcceptedOnly[0].Coverage95 != 1 {
		t.Errorf("accepted-only %+v against pre-gate %+v", report.AcceptedOnly[0], report.PreGate[0])
	}
}

func TestReportIsOrderIndependentAndRejectsInvalid(t *testing.T) {
	samples := consistentSamples(8, 800, 1)
	shuffled := append([]l5tracks.UncertaintySample(nil), samples...)
	rand.New(rand.NewSource(9)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	shuffled = append(shuffled, l5tracks.UncertaintySample{Rank: 3}, l5tracks.UncertaintySample{Rank: 2, NIS: float32(math.NaN())})
	a, err := BuildUncertaintyReport(UncertaintyInput{Samples: samples, Options: DefaultUncertaintyFitOptions(0.05)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildUncertaintyReport(UncertaintyInput{Samples: shuffled, Options: DefaultUncertaintyFitOptions(0.05)})
	if err != nil {
		t.Fatal(err)
	}
	if b.SamplesInvalid != 2 || checkStatus(b, "sample_not_truncated") != CheckOutside {
		t.Errorf("invalid samples not declared: %d", b.SamplesInvalid)
	}
	b.SamplesInvalid = 0
	b.GateChecks, a.GateChecks = nil, nil
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Error("the report depends on sample order")
	}
	if _, err := BuildUncertaintyReport(UncertaintyInput{Options: UncertaintyFitOptions{}}); err == nil {
		t.Error("zero options accepted")
	}
}

func TestCalibrationTableRoundTrip(t *testing.T) {
	report, err := BuildUncertaintyReport(UncertaintyInput{Samples: consistentSamples(10, 2000, 1),
		Options: DefaultUncertaintyFitOptions(0.05)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	cal, err := ParseNoiseCalibration(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cal.ID != report.Fit.Calibration.ID || !strings.HasPrefix(cal.ID, "sha256:") {
		t.Errorf("id %q, table %q", cal.ID, report.Fit.Calibration.ID)
	}
	for _, c := range report.Fit.Calibration.Cells {
		si, _ := l5tracks.NoiseSourceIndex(l5tracks.MeasurementSource(c.Source))
		if got := cal.Coefficients[si][c.RangeBin][c.SupportBin][c.AspectBin]; got != [2]float32{c.RadialM2, c.TangentialM2} {
			t.Fatalf("cell %+v loaded as %v", c, got)
		}
	}

	mutate := func(f func(*CalibrationTable)) error {
		table := report.Fit.Calibration
		table.Cells = append([]CalibrationCell(nil), table.Cells...)
		table.RangeEdgesMetres = append([]float32(nil), table.RangeEdgesMetres...)
		f(&table)
		_, err := table.NoiseCalibration()
		return err
	}
	rehash := func(t *CalibrationTable) { t.ID = t.digest() }
	for name, f := range map[string]func(*CalibrationTable){
		"tampered":      func(t *CalibrationTable) { t.Cells[0].RadialM2 = 9 },
		"edges":         func(t *CalibrationTable) { t.RangeEdgesMetres[1] = 12; rehash(t) },
		"missing cell":  func(t *CalibrationTable) { t.Cells = t.Cells[1:]; rehash(t) },
		"duplicate":     func(t *CalibrationTable) { t.Cells[1] = t.Cells[0]; rehash(t) },
		"unknown cell":  func(t *CalibrationTable) { t.Cells[0].AspectBin = 9; rehash(t) },
		"non-positive":  func(t *CalibrationTable) { t.Cells[0].TangentialM2 = 0; rehash(t) },
		"wrong sources": func(t *CalibrationTable) { t.Sources = t.Sources[:1]; rehash(t) },
	} {
		if err := mutate(f); err == nil {
			t.Errorf("%s table accepted", name)
		}
	}
	if _, err := ParseNoiseCalibration([]byte(`{"schema_version":99}`)); err == nil {
		t.Error("wrong schema accepted")
	}
	if _, err := ParseNoiseCalibration([]byte(`{"schema_version":1}`)); err == nil {
		t.Error("report without a calibration accepted")
	}
}

// End to end through the real tracker: bodies crossing the sensor's view with
// bearing noise far above range noise. Under the shipped isotropic R the
// tangential marginal is overconfident and the radial one underconfident; the
// fit sees the anisotropy, and a replay with the fitted table moves both
// marginals toward 1. The radial marginal cannot get there: these bodies move
// at exactly constant velocity, so the filter's process noise alone predicts
// more radial spread than there is, and no positive R can restore it. The fit
// says so by clamping that coefficient at its floor, which is the report's
// signal to look at Q rather than R.
func TestCalibrationFromTrackerSamplesImprovesConsistency(t *testing.T) {
	type body struct{ x0, y, vx float64 }
	bodies := []body{{-25, 15, 6}, {25, -22, -5}, {-30, 35, 7}}
	const frames = 600
	rng := rand.New(rand.NewSource(21))
	noise := make([][2]float64, frames*len(bodies))
	for i := range noise {
		noise[i] = [2]float64{rng.NormFloat64() * 0.04, rng.NormFloat64() * 0.35}
	}
	run := func(cfg l5tracks.TrackerConfig) UncertaintyReport {
		tk := l5tracks.NewTracker(cfg)
		start := time.Unix(1_700_000_000, 0)
		for f := 0; f < frames; f++ {
			if f == 20 {
				tk.BeginUncertaintyCalibration()
			}
			var clusters []l5tracks.WorldCluster
			for bi, b := range bodies {
				// Each body loops across a 50 m window, so tracks keep coming.
				x := b.x0 + b.vx*math.Mod(float64(f)*0.1, 50/math.Abs(b.vx))
				r := math.Hypot(x, b.y)
				ux, uy := x/r, b.y/r
				n := noise[f*len(bodies)+bi]
				mx, my := x+n[0]*ux-n[1]*uy, b.y+n[0]*uy+n[1]*ux
				clusters = append(clusters, l5tracks.WorldCluster{
					CentroidX: float32(mx), CentroidY: float32(my), PointsCount: 80,
					BoundingBoxLength: 4.5, BoundingBoxWidth: 1.8,
					OBB: &l4perception.OrientedBoundingBox{CenterX: float32(mx), CenterY: float32(my), Length: 4.5, Width: 1.8},
				})
			}
			tk.Update(clusters, start.Add(time.Duration(f)*100*time.Millisecond))
		}
		w := tk.UncertaintyWindow()
		report, err := BuildUncertaintyReport(UncertaintyInput{Samples: w.Samples, SamplesDropped: w.SamplesDropped,
			Options: DefaultUncertaintyFitOptions(float64(cfg.MeasurementNoise))})
		if err != nil {
			t.Fatal(err)
		}
		return report
	}
	marginal := func(r UncertaintyReport, dim string) NISConsistency {
		for _, c := range r.PreGate {
			if c.Dimension == dim {
				return c
			}
		}
		t.Fatalf("no %s marginal", dim)
		return NISConsistency{}
	}

	shipped := l5tracks.DefaultTrackerConfig()
	before := run(shipped)
	if before.Samples < 1000 {
		t.Fatalf("only %d samples", before.Samples)
	}
	var radial, tangential float64
	for _, s := range before.Fit.Pooled {
		if s.Axis == "radial" {
			radial = s.Coefficient
		} else {
			tangential = s.Coefficient
		}
	}
	if !(tangential > 5*radial) {
		t.Errorf("fit missed the anisotropy: radial %v, tangential %v", radial, tangential)
	}

	cal, err := before.Fit.Calibration.NoiseCalibration()
	if err != nil {
		t.Fatal(err)
	}
	calibrated := shipped
	calibrated.AdaptiveMeasurementNoise = true
	calibrated.MeasurementNoiseCalibration = cal
	after := run(calibrated)
	for _, dim := range []string{"radial", "tangential"} {
		b, a := marginal(before, dim), marginal(after, dim)
		if !(math.Abs(a.MeanNISOverDOF-1) < math.Abs(b.MeanNISOverDOF-1)) {
			t.Errorf("%s marginal: mean NIS %v before, %v after calibration", dim, b.MeanNISOverDOF, a.MeanNISOverDOF)
		}
	}
	if tan := marginal(after, "tangential"); !near(tan.MeanNISOverDOF, 1, 0.2) || !near(tan.Coverage95, 0.95, 0.03) {
		t.Errorf("calibrated tangential marginal %+v", tan)
	}
	for _, s := range before.Fit.Pooled {
		if s.Axis == "radial" && (s.Clamped != "floor" || !(s.MeanPredicted > s.InnovationVariance)) {
			t.Errorf("radial pooled fit %+v: the predicted covariance should exceed the spread and clamp the floor", s)
		}
	}
}

// Under the median estimator, gross outliers recorded before the gate must not
// set R for every ordinary observation. Nine per cent of gross outliers move
// the median-matched fit a bounded amount (a contaminated median is a higher
// clean quantile: here about +50%); the mean-matched fit and the moment
// estimate count every one of them and move by more than an order of
// magnitude. Coasting samples move neither fit.
func TestFitEstimatorsAndCoastingExclusion(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	var samples []l5tracks.UncertaintySample
	for i := 0; i < 3000; i++ {
		samples = append(samples, drawSample(rng, 15, 150, 1.5, 0.02, 0.02, 0.03, 0.03, 0, 0, 1))
	}
	for i := 0; i < 300; i++ {
		s := drawSample(rng, 15, 150, 1.5, 0.02, 0.02, 0.03, 0.03, 0, 0, 1)
		s.InnovRadial, s.InnovTangential = float32(4*(1-2*float64(i%2))), 3
		samples = append(samples, s)
	}
	for i := 0; i < 1000; i++ {
		s := drawSample(rng, 15, 150, 1.5, 2, 2, 0.03, 0.03, 0, 0, 1)
		s.Misses = 3
		samples = append(samples, s)
	}
	robust := DefaultUncertaintyFitOptions(0.05)
	robust.Estimator = EstimatorMedian
	report, err := BuildUncertaintyReport(UncertaintyInput{Samples: samples, Options: robust})
	if err != nil {
		t.Fatal(err)
	}
	fit := report.Fit
	if fit.Method != "median_normalised_squared_innovation_matched" {
		t.Errorf("method %q", fit.Method)
	}
	if fit.CoastingExcluded != 1000 {
		t.Errorf("coasting excluded %d, want 1000", fit.CoastingExcluded)
	}
	cell := cellOf(t, fit, 1, 3, 2)
	if !near(float64(cell.RadialM2), 0.03, 0.02) || !near(float64(cell.TangentialM2), 0.03, 0.02) {
		t.Errorf("outliers moved the fit beyond their bounded influence: %+v", cell)
	}
	for _, s := range fit.Pooled {
		if !(s.MomentCoefficient > 0.3) {
			t.Errorf("%s moment estimate %v; the outliers should dominate it", s.Axis, s.MomentCoefficient)
		}
	}
	if len(report.CoastSplit) != 2 || report.CoastSplit[1].Label != "coasting" || report.CoastSplit[1].Count != 1000 ||
		report.CoastSplit[1].Upper != 3 || report.CoastSplit[0].Count != 3300 {
		t.Errorf("coast split %+v", report.CoastSplit)
	}

	mean := FitNoiseCoefficients(samples, DefaultUncertaintyFitOptions(0.05))
	if mean.Method != "mean_normalised_squared_innovation_matched" || mean.CoastingExcluded != 1000 {
		t.Errorf("default fit %q excluded %d", mean.Method, mean.CoastingExcluded)
	}
	if c := cellOf(t, mean, 1, 3, 2); !(c.RadialM2 > 0.3) || !(c.TangentialM2 > 0.3) {
		t.Errorf("mean-matched fit %+v did not count the tail", c)
	}
}

// The mean estimator's defining property: after the fit, each fitted
// stratum's mean normalised squared innovation about its mean is 1, whatever
// the spread of predicted covariances inside it.
func TestMeanEstimatorMatchesTheMean(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	var samples []l5tracks.UncertaintySample
	for i := 0; i < 3000; i++ {
		pred := 0.005 + 0.2*rng.Float64()
		samples = append(samples, drawSample(rng, 15, 150, 1.5, pred, pred, 0.04, 0.2, 0.001, 0.002, 1))
	}
	fit := FitNoiseCoefficients(samples, DefaultUncertaintyFitOptions(0.05))
	cell := cellOf(t, fit, 1, 3, 2)
	for axis, c := range []float32{cell.RadialM2, cell.TangentialM2} {
		var centre, sum float64
		for _, s := range samples {
			y, _, _, _ := axisValues(s, axis)
			centre += y
		}
		centre /= float64(len(samples))
		for _, s := range samples {
			y, pred, _, phys := axisValues(s, axis)
			sum += (y - centre) * (y - centre) / (pred + phys + float64(c))
		}
		if got := sum / float64(len(samples)); !near(got, 1, 1e-3) {
			t.Errorf("axis %d: mean normalised squared innovation %v after the fit, want 1", axis, got)
		}
	}
	if !near(float64(cell.RadialM2), 0.039, 0.012) || !near(float64(cell.TangentialM2), 0.198, 0.025) {
		t.Errorf("mean fit %+v, want about the true 0.04 and 0.2 less physics", cell)
	}
	bad := DefaultUncertaintyFitOptions(0.05)
	bad.Estimator = "mode"
	if _, err := BuildUncertaintyReport(UncertaintyInput{Samples: samples, Options: bad}); err == nil {
		t.Error("unknown estimator accepted")
	}
}

// The fit is one step of a fixed-point iteration: it is converged only when
// the coefficients it returns are the ones the replay ran with.
func TestFitReportsConvergence(t *testing.T) {
	rng := rand.New(rand.NewSource(12))
	var settled, moving []l5tracks.UncertaintySample
	for i := 0; i < 4000; i++ {
		// The replay already used the true 0.03 along both axes.
		settled = append(settled, drawSample(rng, 15, 150, 1.5, 0.02, 0.02, 0.03, 0.03, 0, 0, 1))
		// The replay used five times the truth.
		moving = append(moving, drawSample(rng, 15, 150, 1.5, 0.02, 0.02, 0.03, 0.03, 0, 0, 5))
	}
	opts := DefaultUncertaintyFitOptions(0.05)
	if fit := FitNoiseCoefficients(settled, opts); !fit.Converged || fit.UnconvergedStrata != 0 {
		t.Errorf("fit at its own fixed point reported unconverged: %d strata", fit.UnconvergedStrata)
	}
	fit := FitNoiseCoefficients(moving, opts)
	if fit.Converged || fit.UnconvergedStrata == 0 || fit.ConvergenceTolerance != 0.1 {
		t.Errorf("fit five times off its fixed point reported converged: %+v", fit.Pooled)
	}
}
