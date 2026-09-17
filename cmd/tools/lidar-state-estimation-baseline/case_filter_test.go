package main

import (
	"strings"
	"testing"
)

func threeCaseCorpus() corpus {
	return corpus{Cases: []corpusCase{
		{ID: "marina-webster-beach"},
		{ID: "columbus-broadway"},
		{ID: "embarcadero-folsom"},
	}}
}

func caseIDs(value corpus) []string {
	ids := make([]string, 0, len(value.Cases))
	for _, c := range value.Cases {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestFilterCorpusCasesKeepsOnlyTheNamedCase(t *testing.T) {
	got, err := filterCorpusCases(threeCaseCorpus(), "columbus-broadway")
	if err != nil {
		t.Fatalf("filterCorpusCases: %v", err)
	}
	if ids := caseIDs(got); len(ids) != 1 || ids[0] != "columbus-broadway" {
		t.Errorf("cases = %v, want just columbus-broadway", ids)
	}
}

func TestFilterCorpusCasesPreservesCorpusOrderNotArgumentOrder(t *testing.T) {
	// The corpus file declares replay order, and a run's meaning depends on
	// it. Taking order from the command line would let two runs of the same
	// cases produce different populations.
	got, err := filterCorpusCases(threeCaseCorpus(), "embarcadero-folsom,marina-webster-beach")
	if err != nil {
		t.Fatalf("filterCorpusCases: %v", err)
	}
	want := []string{"marina-webster-beach", "embarcadero-folsom"}
	ids := caseIDs(got)
	if len(ids) != len(want) {
		t.Fatalf("cases = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("cases = %v, want %v (corpus order)", ids, want)
		}
	}
}

func TestFilterCorpusCasesToleratesSpacesAndEmptyEntries(t *testing.T) {
	got, err := filterCorpusCases(threeCaseCorpus(), " columbus-broadway , ,marina-webster-beach ")
	if err != nil {
		t.Fatalf("filterCorpusCases: %v", err)
	}
	if ids := caseIDs(got); len(ids) != 2 {
		t.Errorf("cases = %v, want two", ids)
	}
}

func TestFilterCorpusCasesRejectsAnUnknownID(t *testing.T) {
	// Silently replaying nothing — and writing a source manifest attesting to
	// it — is the worst outcome available here, so an unknown ID is fatal.
	_, err := filterCorpusCases(threeCaseCorpus(), "columbus-broadwya")
	if err == nil {
		t.Fatal("filterCorpusCases accepted an unknown case ID")
	}
	// The message has to name what was available, or the operator is left
	// guessing at the spelling of a site name.
	for _, want := range []string{"columbus-broadwya", "marina-webster-beach", "embarcadero-folsom"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestFilterCorpusCasesRejectsAKnownIDAlongsideAnUnknownOne(t *testing.T) {
	// A partial match must not quietly succeed with the half it recognised.
	if _, err := filterCorpusCases(threeCaseCorpus(), "columbus-broadway,does-not-exist"); err == nil {
		t.Fatal("filterCorpusCases accepted a partially unknown selection")
	}
}

func TestFilterCorpusCasesRejectsASelectionThatNamesNothing(t *testing.T) {
	for _, list := range []string{",", " ", ",,"} {
		if _, err := filterCorpusCases(threeCaseCorpus(), list); err == nil {
			t.Errorf("filterCorpusCases(%q) accepted a selection naming no case", list)
		}
	}
}

func TestFilterCorpusCasesDoesNotMutateTheInput(t *testing.T) {
	// The corpus value is reused to build the source manifest, so a filter
	// that aliased and truncated the caller's slice would silently change
	// what the manifest claims.
	original := threeCaseCorpus()
	if _, err := filterCorpusCases(original, "columbus-broadway"); err != nil {
		t.Fatalf("filterCorpusCases: %v", err)
	}
	if len(original.Cases) != 3 {
		t.Errorf("input corpus now has %d cases, want 3", len(original.Cases))
	}
}
