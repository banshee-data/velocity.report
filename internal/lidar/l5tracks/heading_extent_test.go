package l5tracks

import "testing"

func observeAll(b *extentBelief, spans ...float32) {
	for _, s := range spans {
		b.Observe(s)
	}
}

// The defect this type exists to fix. A car crossing the field of view is seen
// side-on for part of its transit and obliquely for the rest; a mean of those
// spans describes a car that is not there.
func TestBeliefDoesNotShrinkOnPartialViews(t *testing.T) {
	spans := []float32{4.5, 4.5, 4.5, 1.2, 0.9, 1.4, 1.1, 0.8, 1.3, 1.0}
	var b extentBelief
	observeAll(&b, spans...)

	mean := float32(0)
	for _, s := range spans {
		mean += s
	}
	mean /= float32(len(spans))

	if !nearBelief(b.Estimate(), 4.5) {
		t.Fatalf("estimate %.2f did not recover the car; the mean of these spans is %.2f", b.Estimate(), mean)
	}
	if b.Support != 10 {
		t.Fatalf("support = %d, want 10", b.Support)
	}
}

// The limitation, asserted rather than hoped away. One good view cannot be
// distinguished from one merged cluster, so it does not move the belief on its
// own. Separating those two needs membership evidence, which is D2.2.
func TestBeliefIgnoresASingleUncorroboratedView(t *testing.T) {
	var b extentBelief
	observeAll(&b, 4.5, 1.2, 0.9, 1.4, 1.1, 0.8, 1.3, 1.0, 1.2, 0.9)
	if b.Estimate() > 2 {
		t.Fatalf("estimate %.2f trusted a single uncorroborated span", b.Estimate())
	}
}

// The mirror defect. A raw maximum cannot tell one merged cluster from one
// unusually good view, so a single contaminated frame would resize the object.
func TestBeliefResistsASingleContaminatedView(t *testing.T) {
	var clean, contaminated extentBelief
	spans := []float32{4.5, 4.4, 4.6, 4.5, 4.3, 4.5, 4.6, 4.4, 4.5, 4.5}
	observeAll(&clean, spans...)
	observeAll(&contaminated, append(append([]float32{}, spans...), 11.0)...)

	drift := contaminated.Estimate() - clean.Estimate()
	if drift > extentBeliefBinMetres {
		t.Fatalf("one 11 m cluster moved the estimate by %.2f m; a maximum would have moved it by 6.5", drift)
	}
	if contaminated.Estimate() < 4 {
		t.Fatalf("estimate %.2f collapsed instead of holding", contaminated.Estimate())
	}
}

// A span no road user has is refused rather than clamped, so the disagreement
// stays visible instead of quietly inflating the belief.
func TestBeliefReportsConflictInsteadOfClamping(t *testing.T) {
	var b extentBelief
	observeAll(&b, 4.5, 40.0, 4.4)

	if b.Conflicts != 1 {
		t.Fatalf("conflicts = %d, want 1", b.Conflicts)
	}
	if b.Support != 2 {
		t.Fatalf("support = %d, want 2: the impossible span must not count as evidence", b.Support)
	}
	if b.Estimate() > 5 {
		t.Fatalf("estimate %.2f absorbed the impossible span", b.Estimate())
	}
}

func TestBeliefWithoutSupportEstimatesNothing(t *testing.T) {
	var b extentBelief
	if b.Estimate() != 0 {
		t.Fatalf("empty belief claimed %.2f", b.Estimate())
	}
	// Zero and negative spans are not evidence either.
	observeAll(&b, 0, -1)
	if b.Support != 0 || b.Estimate() != 0 {
		t.Fatal("non-positive span admitted as evidence")
	}
}

func TestBeliefFloorsTinyReturns(t *testing.T) {
	var b extentBelief
	observeAll(&b, 0.01, 0.02, 0.01)
	if got := b.Estimate(); got != extentBeliefMinMetres {
		t.Fatalf("estimate = %.3f, want the %.2f floor", got, extentBeliefMinMetres)
	}
}

// Reseed is the escape from a belief that has itself become the obstacle.
func TestBeliefReseedDiscardsHistory(t *testing.T) {
	var b extentBelief
	observeAll(&b, 1.0, 1.1, 0.9, 1.0)
	b.Reseed(4.5)

	if b.Support != 1 {
		t.Fatalf("support = %d after reseed, want 1", b.Support)
	}
	if !nearBelief(b.Estimate(), 4.5) {
		t.Fatalf("estimate = %.2f after reseed, want about 4.5", b.Estimate())
	}
}

// An accepted observation is lower-bound evidence: it can raise the estimate
// and must never lower it below what has already been seen repeatedly.
func TestBeliefGrowsOnLargerEvidence(t *testing.T) {
	var b extentBelief
	observeAll(&b, 2.0, 2.0, 2.0, 2.0)
	before := b.Estimate()
	observeAll(&b, 4.5, 4.5, 4.5, 4.5, 4.5, 4.5)
	if b.Estimate() <= before {
		t.Fatalf("estimate stayed at %.2f after six larger views", b.Estimate())
	}
	if b.Estimate() > 4.5+extentBeliefBinMetres {
		t.Fatalf("estimate %.2f overshot the largest span observed", b.Estimate())
	}
}

// Estimate guards against a belief whose support and histogram disagree, which
// only a corrupted struct can produce. It reports nothing rather than panicking.
func TestBeliefEstimateGuardsInconsistentState(t *testing.T) {
	b := extentBelief{Support: 3} // support without any recorded spans
	if got := b.Estimate(); got != 0 {
		t.Fatalf("estimate = %v from an empty histogram, want 0", got)
	}
}
