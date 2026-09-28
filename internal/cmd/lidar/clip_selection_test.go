//go:build pcap

package lidar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

// selectionReport writes a segments report for two windows ranked by a
// finder for a role at the parameters given.
func selectionReport(t *testing.T, role string, params segments.Params, edit func(report map[string]any, windows []segments.Window)) (string, []segments.Window) {
	t.Helper()
	windows := []segments.Window{
		{ID: segments.Identity("following", "run", role, params, 1), Finder: "following", Version: segments.Version, Source: "run", Role: role, StartNs: 1, EndNs: 1 + int64(params.WindowSeconds*1e9), Capture: "/capture.pcap"},
		{ID: segments.Identity("following", "run", role, params, 10_000_000_001), Finder: "following", Version: segments.Version, Source: "run", Role: role, StartNs: 10_000_000_001, EndNs: 10_000_000_001 + int64(params.WindowSeconds*1e9), Capture: "/capture.pcap"},
	}
	report := map[string]any{"schema": "velocity.report/segments", "source": "run", "finder": "following", "version": segments.Version, "role": role, "parameters": params, "windows": windows}
	if edit != nil {
		edit(report, windows)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "selection.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path, windows
}

func TestReadClipSelectionKeepsFinderParameters(t *testing.T) {
	params := segments.DefaultParams()
	params.MaxGap = 17
	path, windows := selectionReport(t, "tuning", params, nil)
	if _, _, _, err := readClipSelection(path, "", "tuning"); err == nil {
		t.Fatal("accepted ambiguous selection")
	}
	chosen, got, chosenBy, err := readClipSelection(path, windows[1].ID, "tuning")
	if err != nil || chosen.ID != windows[1].ID || got.MaxGap != 17 || chosenBy != nil {
		t.Fatalf("chosen=%+v params=%+v selector=%+v err=%v", chosen, got, chosenBy, err)
	}
	if _, _, _, err := readClipSelection(path, windows[1].ID, "held_out"); err == nil {
		t.Fatal("accepted role change")
	}
	if _, _, _, err := readClipSelection(path, "missing", "tuning"); err == nil {
		t.Fatal("accepted unknown ID")
	}
	if _, _, _, err := readClipSelection(filepath.Join(t.TempDir(), "missing.json"), "", "tuning"); err == nil {
		t.Fatal("missing report was accepted")
	}
	if err := os.WriteFile(path, []byte("bad JSON"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := readClipSelection(path, "", "tuning"); err == nil {
		t.Fatal("malformed report was accepted")
	}
	invalid := params
	invalid.MaxGap = -1
	path, _ = selectionReport(t, "tuning", invalid, nil)
	if _, _, _, err := readClipSelection(path, windows[1].ID, "tuning"); err == nil {
		t.Fatal("invalid finder parameters were accepted")
	}
	path, windows = selectionReport(t, "tuning", params, func(report map[string]any, windows []segments.Window) {
		windows[1].Capture = ""
	})
	if _, _, _, err := readClipSelection(path, windows[1].ID, "tuning"); err == nil {
		t.Fatal("unplaced window was accepted")
	}
}

// A held-out pack is cut only from a window a standard selector chose at its
// own parameters, and the pack names the selector that chose it.
func TestReadClipSelectionHoldsTheHeldOutRule(t *testing.T) {
	catalogue, err := segments.DefaultCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	following, _ := catalogue.Selector("following")
	chosenBy := following.Provenance()
	path, windows := selectionReport(t, "held_out", segments.DefaultParams(), func(report map[string]any, _ []segments.Window) {
		report["selector"] = chosenBy
	})
	if _, _, got, err := readClipSelection(path, windows[0].ID, "held_out"); err != nil || got == nil || got.Digest != following.Digest() {
		t.Fatalf("a standard held-out selection: %+v %v", got, err)
	}
	// A report from before selectors existed names none, and is read as it was.
	path, windows = selectionReport(t, "held_out", segments.DefaultParams(), nil)
	if _, _, got, err := readClipSelection(path, windows[0].ID, "held_out"); err != nil || got != nil {
		t.Fatalf("a report with no selector: %+v %v", got, err)
	}
	wider := segments.DefaultParams()
	wider.MaxGap = 17
	ineligible := chosenBy
	ineligible.HeldOutEligible = false
	mismatched := chosenBy
	mismatched.Parameters.MaxGap = 15
	for name, tc := range map[string]struct {
		params segments.Params
		chosen *segments.SelectorProvenance
		want   string
	}{
		"other parameters":       {wider, nil, "standard selector at its own parameters"},
		"an ineligible selector": {segments.DefaultParams(), &ineligible, "standard selector at its own parameters"},
		"a mismatched selector":  {segments.DefaultParams(), &mismatched, "ran another finder or other parameters"},
	} {
		path, windows := selectionReport(t, "held_out", tc.params, func(report map[string]any, _ []segments.Window) {
			if tc.chosen != nil {
				report["selector"] = tc.chosen
			}
		})
		if _, _, _, err := readClipSelection(path, windows[0].ID, "held_out"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", name, err, tc.want)
		}
	}
}
