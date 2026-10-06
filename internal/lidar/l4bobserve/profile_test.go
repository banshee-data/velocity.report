package l4bobserve

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

func TestCapabilitySetIsCanonicalAndRequireNamesEveryMiss(t *testing.T) {
	set := NewCapabilitySet(CapabilityPointIntensity, CapabilityClusterSummaries, CapabilityPointIntensity)
	if got := set.List(); !reflect.DeepEqual(got, []Capability{CapabilityClusterSummaries, CapabilityPointIntensity}) {
		t.Fatalf("set = %v, want sorted and unique", got)
	}
	listed := set.List()
	listed[0] = "mutated"
	if !set.Has(CapabilityClusterSummaries) {
		t.Fatal("List aliased the set's storage")
	}
	if err := set.Require(CapabilityPointIntensity); err != nil {
		t.Fatalf("present capability refused: %v", err)
	}
	err := set.Require(CapabilityFrameRecords, CapabilityPointIntensity, CapabilityClusterMembership, CapabilityFrameRecords)
	var missing *MissingCapabilitiesError
	if !errors.As(err, &missing) || !errors.Is(err, ErrMissingCapability) {
		t.Fatalf("error = %v, want a typed missing-capabilities error", err)
	}
	if want := []Capability{CapabilityClusterMembership, CapabilityFrameRecords}; !reflect.DeepEqual(missing.Missing, want) {
		t.Fatalf("missing = %v, want %v", missing.Missing, want)
	}
	if missing.Profile != "" || !strings.Contains(err.Error(), "cluster-membership, frame-records") {
		t.Fatalf("unnamed set error = %q", err)
	}
	if err := (CapabilitySet{}).Require(); err != nil {
		t.Fatalf("empty requirement refused: %v", err)
	}
}

// The legacy JSON record family must report the reduced profile and refuse a
// request for the complete accuracy profile, naming what it lacks.
func TestReducedRecordsCannotSatisfyTheCompleteProfile(t *testing.T) {
	observation, err := New(Record{SchemaVersion: 1, ObservationID: "o", SourceID: "s", CalibrationID: "c",
		Cluster: l4perception.WorldCluster{SensorID: "lidar", FrameID: "site/lidar", PointsCount: 1,
			RetainedPoints: []l4perception.WorldPoint{{X: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	reduced := observation.Profile()
	if reduced.Name != ProfileReducedClusterSample {
		t.Fatalf("legacy record profile = %q", reduced.Name)
	}
	err = reduced.Satisfies(ForegroundComplete())
	var missing *MissingCapabilitiesError
	if !errors.As(err, &missing) || missing.Profile != ProfileReducedClusterSample {
		t.Fatalf("reduced evidence satisfied a full-profile request: %v", err)
	}
	want := []Capability{
		CapabilityClusterMembership, CapabilityCompleteForeground, CapabilityFrameRecords, CapabilityGapRecords,
		CapabilityPointChannel, CapabilityPointSourceOrdinal, CapabilityStageCounts, CapabilityUnassignedPoints,
	}
	if !reflect.DeepEqual(missing.Missing, want) {
		t.Fatalf("missing = %v\nwant %v", missing.Missing, want)
	}
	if !strings.Contains(err.Error(), `"reduced-cluster-sample"`) {
		t.Fatalf("error does not name the profile: %q", err)
	}
	// Requests the reduced family can serve still succeed.
	if err := reduced.Require(CapabilityClusterSummaries, CapabilityPointAcquisitionTime); err != nil {
		t.Fatalf("reduced profile refused its own capabilities: %v", err)
	}
	// Complete evidence supersedes a sample but does not reproduce its draw.
	err = ForegroundComplete().Satisfies(reduced)
	if !errors.As(err, &missing) || !reflect.DeepEqual(missing.Missing, []Capability{CapabilityClusterSample}) {
		t.Fatalf("complete profile vs reduced request = %v", err)
	}
}

func TestLookupProfileRefusesUnknownNames(t *testing.T) {
	for _, name := range []ProfileName{ProfileReducedClusterSample, ProfileForegroundComplete} {
		profile, err := LookupProfile(name)
		if err != nil || profile.Name != name {
			t.Fatalf("lookup %q = %+v, %v", name, profile, err)
		}
	}
	if _, err := LookupProfile("foreground-complete-v2"); err == nil {
		t.Fatal("guessed the meaning of an unknown profile")
	}
}
