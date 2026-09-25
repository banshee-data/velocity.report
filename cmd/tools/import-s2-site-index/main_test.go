package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type phase01Corpus struct {
	Cases []struct {
		ID                   string  `json:"id"`
		ExpectedCaptureCount int     `json:"expected_capture_count"`
		ExpectedMinutes      float64 `json:"expected_minutes"`
	} `json:"cases"`
}

func TestPhase01CorpusSitesRemainMultiFileAndSelectable(t *testing.T) {
	indexPath := filepath.Join("..", "..", "..", "tools", "s2-archive", "site-index.json")
	bytes, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var entries []siteIndexEntry
	if err := json.Unmarshal(bytes, &entries); err != nil {
		t.Fatal(err)
	}
	corpusPath := filepath.Join("..", "..", "..", "tools", "s2-archive", "state-estimation-phase01-corpus.json")
	corpusBytes, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	var corpus phase01Corpus
	if err := json.Unmarshal(corpusBytes, &corpus); err != nil {
		t.Fatal(err)
	}
	wanted := make([]string, 0, len(corpus.Cases))
	want := map[string]int{}
	for _, entry := range corpus.Cases {
		wanted = append(wanted, entry.ID)
		want[entry.ID] = entry.ExpectedCaptureCount
	}
	selected, err := selectEntries(entries, strings.Join(wanted, ","))
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != len(want) {
		t.Fatalf("selected %d corpus sites, want %d", len(selected), len(want))
	}
	// The Phase 0/1 cases exist to exercise PCAP roll-over joins.
	phase01 := map[string]bool{"marina-webster-beach": true, "columbus-broadway": true, "embarcadero-folsom": true}
	totalCaptures := 0
	for _, entry := range selected {
		if got, ok := want[entry.ID]; !ok || len(entry.Captures) != got {
			t.Fatalf("%s has %d captures, want %d", entry.ID, len(entry.Captures), want[entry.ID])
		}
		if phase01[entry.ID] && len(entry.Captures) < 2 {
			t.Fatalf("%s has %d capture, want a multi-file case", entry.ID, len(entry.Captures))
		}
		totalCaptures += len(entry.Captures)
	}
	// The corpus covers all 24 S2 archive sites; the committed phase01 baseline
	// is measured against exactly these 110 captures.
	if len(wanted) != 24 || totalCaptures != 110 {
		t.Fatalf("corpus has %d cases and %d captures, want 24 and 110", len(wanted), totalCaptures)
	}
	if fmt.Sprint(wanted[:3]) != "[marina-webster-beach columbus-broadway embarcadero-folsom]" {
		t.Fatalf("corpus no longer leads with the Phase 0/1 test cases: %v", wanted[:3])
	}
}

func TestSelectEntriesRejectsBadRequests(t *testing.T) {
	entries := []siteIndexEntry{{ID: "a"}, {ID: "b"}}
	if _, err := selectEntries(entries, "a,a"); err == nil {
		t.Fatal("accepted a duplicated request")
	}
	if _, err := selectEntries(entries, "missing"); err == nil {
		t.Fatal("accepted an unknown request")
	}
}
