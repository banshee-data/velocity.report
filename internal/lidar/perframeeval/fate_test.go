package perframeeval

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
)

// The fate measure against evalfixture.WriteFate, whose package comment
// lays out every case.

func fateFixture(t *testing.T) *evalfixture.FateFixture {
	t.Helper()
	f, err := evalfixture.WriteFate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func fateOptions(f *evalfixture.FateFixture) FateOptions {
	o := DefaultFateOptions()
	o.Reference.PackDir, o.Reference.SplitManifestPath, o.Reference.Split = f.PackDir, f.SplitManifestPath, evalfixture.SplitHeldOut
	o.ObservationsDir, o.Port = f.ObservationsDir, evalfixture.FatePort
	return o
}

func measureFate(t *testing.T, o FateOptions) *FateReport {
	t.Helper()
	r, err := MeasureFate(o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func instantAt(t *testing.T, r *FateReport, sample int) FateInstant {
	t.Helper()
	for _, in := range r.Instants {
		if in.SampleID == sample {
			return in
		}
	}
	t.Fatalf("no measured instant at sample %d", sample)
	return FateInstant{}
}

func wantFates(t *testing.T, what string, got map[string]int, want map[string]int) {
	t.Helper()
	for _, name := range FateNames() {
		if got[name] != want[name] {
			t.Errorf("%s: %s = %d, want %d (all: %v)", what, name, got[name], want[name], got)
		}
	}
}

// Every fate class, the identity join and its cross-checks, the gaps of a
// fragmented instant, and the not-measured frames, case by case.
func TestFateClassifiesEveryReviewedReturn(t *testing.T) {
	f := fateFixture(t)
	r := measureFate(t, fateOptions(f))

	// Measured: samples 1, 2, 3, 7 and 8, four returns each.
	var samples []int
	for _, in := range r.Instants {
		if in.ObjectID != evalfixture.CarA {
			t.Fatalf("instant of %s measured; only %s is scored", in.ObjectID, evalfixture.CarA)
		}
		samples = append(samples, in.SampleID)
	}
	if !slices.Equal(samples, []int{1, 2, 3, 7, 8}) {
		t.Fatalf("measured samples %v, want [1 2 3 7 8]", samples)
	}

	s1 := instantAt(t, r, 1)
	wantFates(t, "sample 1", s1.Fates, map[string]int{FateClustered: 3, FateL3Background: 1})
	if len(s1.Clusters) != 2 || s1.Clusters[0] != (FateCluster{ClusterID: 1, Points: 2, ClusterPoints: 3}) ||
		s1.Clusters[1] != (FateCluster{ClusterID: 2, Points: 1, ClusterPoints: 1}) {
		// Cluster 1 holds returns 0 and 1 and the decoy at return 3's
		// coordinates; a coordinate join would have counted return 3 there.
		t.Errorf("sample 1 clusters = %+v", s1.Clusters)
	}
	wantFates(t, "sample 2", instantAt(t, r, 2).Fates, map[string]int{FateHeightBand: 1, FateSurfaceBelowFloor: 1,
		FateSurfaceAboveCeiling: 1, FateVoxelReduced: 1})
	wantFates(t, "sample 3", instantAt(t, r, 3).Fates, map[string]int{FateClusterInputCap: 1, FateClusterNoise: 1,
		FateClusterShape: 1, FateRejectionUnknown: 1})
	s7 := instantAt(t, r, 7)
	wantFates(t, "sample 7", s7.Fates, map[string]int{FateClustered: 2, FateOtherFrame: 1, FateUnresolved: 1})
	if len(s7.Clusters) != 1 || s7.Clusters[0].Points != 2 {
		t.Errorf("sample 7 clusters = %+v", s7.Clusters)
	}
	s8 := instantAt(t, r, 8)
	wantFates(t, "sample 8", s8.Fates, map[string]int{FateUnresolved: 4})
	if len(s8.Clusters) != 0 {
		t.Errorf("unresolved returns were given clusters: %+v", s8.Clusters)
	}

	j := r.Join
	if j.MaskReturns != 20 || j.Matched != 13 || j.ProvenAbsent != 1 || j.OtherFrame != 1 || j.Unresolved != 5 ||
		j.UnresolvedBy[UnresolvedNotInCapture] != 2 || j.UnresolvedBy[UnresolvedAmbiguousSequence] != 2 ||
		j.UnresolvedBy[UnresolvedAmbiguousDomain] != 1 || j.MatchRate != 0.7 {
		t.Errorf("join = %+v", j)
	}
	if j.CoordinatesAgree != 12 {
		t.Errorf("coordinates agree on %d of %d matched returns, want 12 (sample 7's return 3 was moved)", j.CoordinatesAgree, j.Matched)
	}
	// Whole samples: 16 pack returns each; samples 1 and 7 lack one, sample
	// 7 repeats one, and none of sample 8's resolves. Domains: 16 each, with
	// sample 1's decoy, sample 7's twin and sample 8's stray return.
	if j.PackReturns != 80 || j.PackReturnsInDomain != 61 || j.DomainReturns != 81 {
		t.Errorf("foreground agreement = %d of %d pack returns, %d domain returns", j.PackReturnsInDomain, j.PackReturns, j.DomainReturns)
	}

	tot := r.Total
	wantFates(t, "total", tot.Fates, map[string]int{FateClustered: 5, FateL3Background: 1, FateHeightBand: 1,
		FateSurfaceBelowFloor: 1, FateSurfaceAboveCeiling: 1, FateVoxelReduced: 1, FateClusterInputCap: 1,
		FateClusterNoise: 1, FateClusterShape: 1, FateRejectionUnknown: 1, FateOtherFrame: 1, FateUnresolved: 5})
	if tot.Instants != 5 || tot.Returns != 20 || tot.Missed != 3 || tot.Detected != 2 || tot.Fragmented != 1 ||
		tot.MaxClusters != 2 || tot.Shares[FateClustered] != 0.25 {
		t.Errorf("total = %+v", tot)
	}
	// The gap of the one fragmented instant is its absent return.
	wantFates(t, "gaps", tot.GapFates, map[string]int{FateL3Background: 1})
	if tot.GapReturns != 1 || tot.GapShares[FateL3Background] != 1 {
		t.Errorf("gaps = %d returns, shares %v", tot.GapReturns, tot.GapShares)
	}
	// Sample 2's frame has no accepted cluster: retention would not see it.
	if tot.ComparableInstants != 4 || tot.ComparableReturns != 16 || tot.ComparableClustered != 5 || tot.ComparableRetention != 5.0/16 {
		t.Errorf("comparable = %d instants, %d of %d clustered", tot.ComparableInstants, tot.ComparableClustered, tot.ComparableReturns)
	}
	if len(r.ByClass) != 1 || r.ByClass[0].Name != "car" || len(r.ByObject) != 1 || r.ByObject[0].Class != "car" ||
		r.ByRange[0].Name != "range_0_20m" || r.ByRange[0].Instants != 5 || len(r.ByRange) != 4 {
		t.Errorf("strata: class %+v, object %+v, range %+v", r.ByClass, r.ByObject, r.ByRange)
	}

	notMeasured := map[string]FateNotMeasured{}
	for _, nm := range r.NotMeasured {
		notMeasured[nm.Reason] = nm
	}
	for reason, want := range map[string]FateNotMeasured{
		NotMeasuredWarmUp:    {Reason: NotMeasuredWarmUp, Samples: 1, Instants: 1, Returns: 4},
		NotMeasuredUnsettled: {Reason: NotMeasuredUnsettled, Samples: 1, Instants: 1, Returns: 4},
		NotMeasuredGap:       {Reason: NotMeasuredGap, Samples: 1, Instants: 1, Returns: 4},
		NotMeasuredFailed:    {Reason: NotMeasuredFailed, Samples: 1, Instants: 1, Returns: 4},
	} {
		if notMeasured[reason] != want {
			t.Errorf("not measured %s = %+v, want %+v", reason, notMeasured[reason], want)
		}
	}
	if len(r.NotMeasured) != 4 {
		t.Errorf("not measured = %+v", r.NotMeasured)
	}
	ev := r.Evidence
	if ev.Frames != 9 || ev.Gaps != 1 || ev.MeasuredSamples != 5 || ev.FramesByDisposition["unsettled"] != 1 ||
		ev.FramesByDisposition["failed"] != 1 || ev.CapturePackets != 20 || ev.AmbiguousSequences != 1 ||
		ev.ScoringStartNs != evalfixture.SampleTime(0)+1_000_000+evalfixture.FateWindowStartNs {
		t.Errorf("evidence = %+v", ev)
	}
	if r.Reference.Split != evalfixture.SplitHeldOut || !r.Reference.HeldOut || r.Schema != FateSchema {
		t.Errorf("reference = %+v, schema %s", r.Reference, r.Schema)
	}

	md := RenderFateMarkdown(*r)
	for _, want := range []string{"# Fate of every reviewed return", "match rate 0.7000", "| all | 5 | 20 | 0.250 | 0.050 |",
		"## What the gaps between fragments are made of", "| obj_car_a | car | 1 | 1 | 1.000 |",
		"| all | 4 | 16 | 5 | 0.312 |", "warm_up 1 instants at 1 samples"} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown lacks %q:\n%s", want, md)
		}
	}
}

// writeVariant writes the default scenario changed by edit beside the
// fixture's container and returns options reading it.
func writeVariant(t *testing.T, f *evalfixture.FateFixture, edit func(*evalfixture.FateScenario)) FateOptions {
	t.Helper()
	s, err := evalfixture.DefaultFateScenario(f.PackDir)
	if err != nil {
		t.Fatal(err)
	}
	edit(&s)
	o := fateOptions(f)
	o.ObservationsDir = filepath.Join(t.TempDir(), "variant.vrlog")
	if err := evalfixture.WriteFateContainer(o.ObservationsDir, f.CapturePath, s); err != nil {
		t.Fatal(err)
	}
	return o
}

// A missing frame with no gap record, a suppressed frame, and samples past
// the last frame are not measured either, each under its own reason.
func TestFateNotMeasuredReasons(t *testing.T) {
	f := fateFixture(t)
	o := writeVariant(t, f, func(s *evalfixture.FateScenario) {
		var kept []evalfixture.FateFrame
		for _, fr := range s.Frames {
			switch fr.Sample {
			case 4:
				fr.Disposition = l4bobserve.DispositionSuppressed
			case 7:
				fr.GapBefore = false
			case 8, 9:
				continue
			}
			kept = append(kept, fr)
		}
		s.Frames = kept
	})
	r := measureFate(t, o)
	got := map[string]int{}
	for _, nm := range r.NotMeasured {
		got[nm.Reason] = nm.Instants
	}
	want := map[string]int{NotMeasuredWarmUp: 1, NotMeasuredSuppressed: 1, NotMeasuredNoFrame: 1, NotMeasuredOutsideContainer: 2}
	if len(got) != len(want) {
		t.Errorf("not measured = %v, want %v", got, want)
	}
	for reason, n := range want {
		if got[reason] != n {
			t.Errorf("not measured %s = %d, want %d (all %v)", reason, got[reason], n, got)
		}
	}
	// Sample 7's return 1 was in sample 8's frame, which is gone: with no
	// neighbour holding it, it is now proven absent.
	wantFates(t, "sample 7", instantAt(t, r, 7).Fates, map[string]int{FateClustered: 2, FateL3Background: 1, FateUnresolved: 1})
}

// A frame within the tolerance on either side aligns with its sample; with
// a tolerance tighter than the frames' offset none does, and the measure is
// refused rather than reported as nothing measured.
func TestFateFrameAlignment(t *testing.T) {
	f := fateFixture(t)
	o := fateOptions(f)
	o.FrameToleranceNanos = 150_000 // the frames are 200 µs early
	if _, err := MeasureFate(o); err == nil || !strings.Contains(err.Error(), "no measured sample lies within") {
		t.Fatalf("misaligned frames: %v", err)
	}
	o.FrameToleranceNanos = 250_000
	if r := measureFate(t, o); r.Evidence.MeasuredSamples != 5 {
		t.Fatalf("aligned at 250 µs: %d measured samples, want 5", r.Evidence.MeasuredSamples)
	}
}

// Evidence without the foreground-complete capabilities is refused with the
// typed error naming every one it lacks, and a path that is not a container
// at all is refused before anything is read.
func TestFateRefusesEvidenceLackingCapabilities(t *testing.T) {
	err := RequireFateEvidence(l4bobserve.ReducedClusterSample())
	var missing *l4bobserve.MissingCapabilitiesError
	if !errors.As(err, &missing) || !errors.Is(err, l4bobserve.ErrMissingCapability) {
		t.Fatalf("reduced evidence: %v", err)
	}
	for _, want := range []l4bobserve.Capability{l4bobserve.CapabilityCompleteForeground, l4bobserve.CapabilityClusterMembership,
		l4bobserve.CapabilityUnassignedPoints, l4bobserve.CapabilityFrameRecords, l4bobserve.CapabilityPointChannel} {
		if !slices.Contains(missing.Missing, want) {
			t.Errorf("missing %v does not name %s", missing.Missing, want)
		}
	}
	if missing.Profile != l4bobserve.ProfileReducedClusterSample {
		t.Errorf("profile %q not named", missing.Profile)
	}
	if err := RequireFateEvidence(l4bobserve.ForegroundComplete()); err != nil {
		t.Fatalf("foreground-complete refused: %v", err)
	}

	f := fateFixture(t)
	o := fateOptions(f)
	o.ObservationsDir = filepath.Join(t.TempDir(), "observations.db")
	if err := os.WriteFile(o.ObservationsDir, []byte("SQLite format 3\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := MeasureFate(o); err == nil || !errors.Is(err, vrlog.ErrNotContainer) {
		t.Fatalf("an evidence database read as a container: %v", err)
	}
}

// The capture is the bridge between the two identities, so a capture other
// than the container's source, or one whose packets carry no sequence, is
// refused.
func TestFateRefusesTheWrongCapture(t *testing.T) {
	f := fateFixture(t)
	o := fateOptions(f)
	o.CapturePath = filepath.Join(t.TempDir(), "other.pcapng")
	if err := evalfixture.WriteFateCapture(o.CapturePath, [][]byte{make([]byte, parse.PACKET_SIZE_SEQUENCE)}); err != nil {
		t.Fatal(err)
	}
	if _, err := MeasureFate(o); err == nil || !strings.Contains(err.Error(), "not the container's source") {
		t.Fatalf("foreign capture: %v", err)
	}
	if err := os.Remove(o.CapturePath); err != nil {
		t.Fatal(err)
	}
	if err := evalfixture.WriteFateCapture(o.CapturePath, [][]byte{make([]byte, parse.PACKET_SIZE_STANDARD)}); err != nil {
		t.Fatal(err)
	}
	if _, err := MeasureFate(o); err == nil || !strings.Contains(err.Error(), "carries a sequence number") {
		t.Fatalf("unnumbered capture: %v", err)
	}
	o = fateOptions(f)
	o.Port = 2370
	if _, err := MeasureFate(o); err == nil || !strings.Contains(err.Error(), "no packets on UDP port 2370") {
		t.Fatalf("wrong port: %v", err)
	}
}

func TestFateOptionsValidate(t *testing.T) {
	ok := DefaultFateOptions()
	ok.ObservationsDir = "obs"
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*FateOptions){
		"no container":   func(o *FateOptions) { o.ObservationsDir = "" },
		"port":           func(o *FateOptions) { o.Port = 0 },
		"tolerance":      func(o *FateOptions) { o.FrameToleranceNanos = -1 },
		"bands":          func(o *FateOptions) { o.RangeBandsM = []float64{35, 20} },
		"no bands":       func(o *FateOptions) { o.RangeBandsM = nil },
		"reference pol.": func(o *FateOptions) { o.Reference.Policy.RoadUserClasses = nil },
	} {
		o := ok
		edit(&o)
		if o.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
