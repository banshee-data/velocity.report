package l3grid

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
)

// Gap HW1 in data/maths/paper-implementation-gap-analysis.md: does the relative
// noise term in the closeness threshold agree with the Pandar40P's specified
// range accuracy? It does not, and these tests measure by how much.

// specRangeAccuracyMetres is the Pandar40P's specified range accuracy: ±2 cm
// out to 30 m, ±3 cm from 30 m to 200 m. Flat within each band — the sensor's
// accuracy does not degrade proportionally with range the way the closeness
// model assumes.
func specRangeAccuracyMetres(rangeMetres float64) float64 {
	if rangeMetres <= 30 {
		return 0.02
	}
	return 0.03
}

// shippedClosenessParams reads the three tuning values the threshold is built
// from, so these tests measure the shipped configuration rather than a guess at
// it. The gap analysis row assumed noise_relative = 0.01; it is 0.02.
func shippedClosenessParams(t *testing.T) (multiplier, noiseRelative, safety float64) {
	t.Helper()
	cfg := config.MustLoadDefaultConfig()
	l3 := cfg.L3.EmaBaselineV1
	if l3 == nil {
		t.Fatal("no L3 ema_baseline_v1 block in the default tuning config")
	}
	return l3.ClosenessMultiplier, l3.NoiseRelative, l3.SafetyMarginMetres
}

func TestClosenessNoiseModelAgainstPandar40PSpec(t *testing.T) {
	multiplier, noiseRelative, safety := shippedClosenessParams(t)

	// A settled cell watching a static surface should show a spread no larger
	// than the sensor's own accuracy, so the spec value is the fair stand-in
	// for the measured term and isolates the modelled one.
	for _, rangeMetres := range []float64{5, 20, 50, 100} {
		spec := specRangeAccuracyMetres(rangeMetres)

		got := ClosenessThresholdMetres(multiplier, spec, noiseRelative, rangeMetres, safety)
		// The same threshold with the noise term taken from the sensor spec
		// instead of a fraction of range.
		wantIfSpecFaithful := ClosenessThresholdMetres(multiplier, spec, 0, rangeMetres, safety) +
			multiplier*spec

		modelledNoise := noiseRelative * rangeMetres
		t.Logf("range %5.0f m: threshold %.3f m (spec-faithful %.3f m, inflation %4.1fx); noise term %.3f m against spec %.3f m (%2.0fx)",
			rangeMetres, got, wantIfSpecFaithful, got/wantIfSpecFaithful,
			modelledNoise, spec, modelledNoise/spec)

		// The modelled noise must never be *under* the sensor spec: that would
		// make the threshold optimistic and reject real background as
		// foreground. Over-modelling is the failure mode here, not under.
		if modelledNoise < spec {
			t.Errorf("range %v m: modelled noise %v is below the sensor spec %v, so the threshold is optimistic",
				rangeMetres, modelledNoise, spec)
		}
	}
}

func TestClosenessOverConservatismGrowsWithRange(t *testing.T) {
	// The finding, and the reason this is not simply "conservative is safe":
	// the excess over a spec-faithful threshold is not a constant safety
	// factor. It grows linearly with range, because a relative model is being
	// used where the sensor's accuracy is flat. A fixed multiplier could be
	// argued for; this cannot, because nothing chose its value at 100 m.
	multiplier, noiseRelative, safety := shippedClosenessParams(t)

	inflation := func(rangeMetres float64) float64 {
		spec := specRangeAccuracyMetres(rangeMetres)
		got := ClosenessThresholdMetres(multiplier, spec, noiseRelative, rangeMetres, safety)
		specFaithful := ClosenessThresholdMetres(multiplier, spec, 0, rangeMetres, safety) + multiplier*spec
		return got / specFaithful
	}

	near := inflation(5)
	far := inflation(100)
	if far <= near {
		t.Errorf("expected the over-conservatism to grow with range: 5 m = %.2fx, 100 m = %.2fx", near, far)
	}
	t.Logf("threshold inflation over a spec-faithful model: %.1fx at 5 m, %.1fx at 100 m", near, far)
}

func TestLongRangeClosenessThresholdExceedsAVehicleLength(t *testing.T) {
	// What the growth above costs. classification accepts an observation as
	// background when it sits within the threshold of the cell's learned
	// range, so the threshold is the minimum range separation a foreground
	// object needs to register at all. At 100 m that separation is larger than
	// a car, which means the noise model — not the sensor, and not the
	// clustering that follows — sets the long-range detection floor.
	//
	// Asserted rather than logged so that a tuning change which brings the
	// long-range threshold under a vehicle length shows up as a failure here
	// and gets this note revisited.
	const typicalVehicleLengthMetres = 4.5
	multiplier, noiseRelative, safety := shippedClosenessParams(t)

	spec := specRangeAccuracyMetres(100)
	got := ClosenessThresholdMetres(multiplier, spec, noiseRelative, 100, safety)

	if got <= typicalVehicleLengthMetres {
		t.Errorf("the 100 m closeness threshold is now %.2f m, within a vehicle length (%.1f m): HW1's long-range cost has changed and the note in the gap analysis needs revisiting",
			got, typicalVehicleLengthMetres)
	}
	t.Logf("100 m closeness threshold %.2f m against a %.1f m vehicle: a foreground return must differ from the learned background range by more than %.1f vehicle lengths",
		got, typicalVehicleLengthMetres, got/typicalVehicleLengthMetres)
}

func TestClosenessThresholdIsMonotonicAndFloored(t *testing.T) {
	// Properties the classification loop depends on, independent of the noise
	// question above.
	multiplier, noiseRelative, safety := shippedClosenessParams(t)

	// A cell whose spread has collapsed to zero still gets a usable window,
	// or float noise alone would mark a static surface as foreground.
	zeroSpread := ClosenessThresholdMetres(multiplier, 0, noiseRelative, 10, safety)
	if zeroSpread <= 0 {
		t.Errorf("zero-spread threshold is %v, which would reject every observation", zeroSpread)
	}
	if zeroSpread < multiplier*closenessFloorMetres {
		t.Errorf("zero-spread threshold %v dropped below the floor contribution %v", zeroSpread, multiplier*closenessFloorMetres)
	}

	// Monotonic in every term: a noisier cell, a further observation or a
	// larger operator margin must never narrow the window.
	base := ClosenessThresholdMetres(multiplier, 0.02, noiseRelative, 20, safety)
	for _, tc := range []struct {
		name string
		got  float64
	}{
		{"more spread", ClosenessThresholdMetres(multiplier, 0.05, noiseRelative, 20, safety)},
		{"further range", ClosenessThresholdMetres(multiplier, 0.02, noiseRelative, 40, safety)},
		{"larger safety margin", ClosenessThresholdMetres(multiplier, 0.02, noiseRelative, 20, safety+0.1)},
		{"larger multiplier", ClosenessThresholdMetres(multiplier+1, 0.02, noiseRelative, 20, safety)},
	} {
		if tc.got <= base {
			t.Errorf("%s did not widen the threshold: %v against base %v", tc.name, tc.got, base)
		}
	}

	// The neighbour-confirmation path passes safety=0 and must differ from the
	// classification path by exactly that margin, since the two formulas were
	// one expression before being extracted.
	withSafety := ClosenessThresholdMetres(multiplier, 0.02, noiseRelative, 20, safety)
	withoutSafety := ClosenessThresholdMetres(multiplier, 0.02, noiseRelative, 20, 0)
	if math.Abs((withSafety-withoutSafety)-safety) > 1e-12 {
		t.Errorf("safety margin is not additive: %v against %v with margin %v", withSafety, withoutSafety, safety)
	}
}
