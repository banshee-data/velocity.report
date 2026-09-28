//go:build pcap

package lidar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

func TestReadClipSelectionKeepsFinderParameters(t *testing.T) {
	params := segments.DefaultParams()
	params.MaxGap = 17
	windows := []segments.Window{
		{ID: segments.Identity("following", "run", "held_out", params, 1), Finder: "following", Version: segments.Version, Source: "run", Role: "held_out", StartNs: 1, EndNs: 10_000_000_001, Capture: "/capture.pcap"},
		{ID: segments.Identity("following", "run", "held_out", params, 10_000_000_001), Finder: "following", Version: segments.Version, Source: "run", Role: "held_out", StartNs: 10_000_000_001, EndNs: 20_000_000_001, Capture: "/capture.pcap"},
	}
	report := map[string]any{"schema": "velocity.report/segments", "source": "run", "finder": "following", "version": segments.Version, "role": "held_out", "parameters": params, "windows": windows}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "selection.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readClipSelection(path, "", "held_out"); err == nil {
		t.Fatal("accepted ambiguous selection")
	}
	chosen, got, err := readClipSelection(path, windows[1].ID, "held_out")
	if err != nil || chosen.ID != windows[1].ID || got.MaxGap != 17 {
		t.Fatalf("chosen=%+v params=%+v err=%v", chosen, got, err)
	}
	if _, _, err := readClipSelection(path, windows[1].ID, "tuning"); err == nil {
		t.Fatal("accepted role change")
	}
	if _, _, err := readClipSelection(path, "missing", "held_out"); err == nil {
		t.Fatal("accepted unknown ID")
	}
	if _, _, err := readClipSelection(filepath.Join(t.TempDir(), "missing.json"), "", "held_out"); err == nil {
		t.Fatal("missing report was accepted")
	}
	if err := os.WriteFile(path, []byte("bad JSON"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readClipSelection(path, "", "held_out"); err == nil {
		t.Fatal("malformed report was accepted")
	}
	params.MaxGap = -1
	report["parameters"] = params
	data, _ = json.Marshal(report)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readClipSelection(path, windows[1].ID, "held_out"); err == nil {
		t.Fatal("invalid finder parameters were accepted")
	}
	params.MaxGap = 17
	report["parameters"] = params
	windows[1].Capture = ""
	report["windows"] = windows
	data, _ = json.Marshal(report)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readClipSelection(path, windows[1].ID, "held_out"); err == nil {
		t.Fatal("unplaced window was accepted")
	}
}
