package dataset

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// existingManifest is two entries as stage-huggingface-dataset.mjs writes
// them (JSON.stringify with a two-space indent), including a hand-ordered
// field, an ampersand and a number with trailing precision.
const existingManifest = `[
  {
    "capture_id": "20251205T1417_8085875c_kirkham-9th",
    "timestamp": "2025-12-05T14:17:04.741089-08:00",
    "latitude": 37.7604,
    "site_slug": "kirkham-9th",
    "sensor_type": "lidar",
    "duration_seconds": 83.183383,
    "label_set_paths": [],
    "export_provenance": {
      "static_export_policy": "whole_original_capture",
      "operator_overrides": [],
      "source_files": [
        {
          "name": "kirk0.pcapng",
          "bytes": 200657872
        }
      ]
    }
  },
  {
    "capture_id": "20260901T1225_808580ac_ashbury-downey",
    "site_slug": "ashbury-downey",
    "sensor_type": "lidar",
    "where": "Ashbury & Downey <north>"
  }
]
`

func survey(t *testing.T) Survey {
	t.Helper()
	first := time.Date(2025, 12, 5, 23, 14, 54, 123456000, time.UTC)
	az := 131.0
	return Survey{
		ID: "kirkham-9th-k3", ClipID: "clip-k3", Where: "Kirkham at 9th, east kerb",
		Lat: 37.7604, Lon: -122.4664, L10: "808587", L13: "8085875c",
		First: first, Last: first.Add(1374 * time.Second), NorthAzimuthDeg: &az,
		SourceFiles: []SourceFile{{Name: "kirk2.pcapng", SHA256: "sha256:ab", Bytes: 5634267536}},
		RawPath:     "raw/lidar/20251205T1514_80858-75c_kirkham-9th-k3.pcapng",
		RawSHA256:   "sha256:cd", RawBytes: 3300000000, BuildVersion: "0.5.2", RubricVersion: 1,
	}
}

func TestStemAndCaptureIDUseTheDatasetClock(t *testing.T) {
	first := time.Date(2025, 12, 5, 23, 14, 54, 0, time.UTC)
	stem, err := Stem(first, "8085875c", "kirkham-9th-k3")
	if err != nil || stem != "20251205T1514_80858-75c_kirkham-9th-k3" {
		t.Fatalf("stem %q %v", stem, err)
	}
	id, err := CaptureID(first, "8085875c", "kirkham-9th-k3")
	if err != nil || id != "20251205T1514_8085875c_kirkham-9th-k3" {
		t.Fatalf("capture id %q %v", id, err)
	}
	if _, err := Stem(first, "", "x"); err == nil {
		t.Fatal("a stem without a cell")
	}
	iso, err := ISOTime(time.Date(2026, 9, 1, 19, 25, 33, 19394000, time.UTC))
	if err != nil || iso != "2026-09-01T12:25:33.019394-07:00" {
		t.Fatalf("iso %q %v", iso, err)
	}
}

func TestUpsertManifestKeepsExistingEntriesByteForByte(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(existingManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := survey(t).Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertManifest(path, entry); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	prefix := strings.TrimSuffix(existingManifest, "\n]\n")
	if !strings.HasPrefix(string(got), prefix+",\n  {\n    \"capture_id\": \"20251205T1514_8085875c_kirkham-9th-k3\"") {
		t.Fatalf("existing entries changed or the new one is misplaced:\n%s", got)
	}
	if !strings.HasSuffix(string(got), "\n]\n") {
		t.Fatalf("manifest does not end as the scripts write it:\n%s", got)
	}

	// Staging the same clip again replaces its entry rather than adding one.
	entry.RawBytes = 42
	if err := UpsertManifest(path, entry); err != nil {
		t.Fatal(err)
	}
	var entries []ManifestEntry
	got, _ = os.ReadFile(path)
	if err := json.Unmarshal(got, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[2].RawBytes != 42 || entries[2].ExportProvenance.ClipID != "clip-k3" {
		t.Fatalf("re-staging: %+v", entries)
	}
	if entries[2].DurationSeconds != 1374 || entries[2].FilenameTimestamp != "20251205T1514" {
		t.Fatalf("entry %+v", entries[2])
	}

	// Another clip may not take a published slug.
	clash := entry
	clash.ExportProvenance.ClipID = "clip-other"
	clash.SiteSlug = "kirkham-9th"
	clash.CaptureID = "20251205T1514_8085875c_kirkham-9th"
	if err := UpsertManifest(path, clash); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("slug clash: %v", err)
	}
}

func TestUpsertManifestStartsAMissingManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	entry, err := survey(t).Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertManifest(path, entry); err != nil {
		t.Fatal(err)
	}
	var entries []ManifestEntry
	got, _ := os.ReadFile(path)
	if err := json.Unmarshal(got, &entries); err != nil || len(entries) != 1 {
		t.Fatalf("%v %s", err, got)
	}
}

func TestSiteIndexEntryMatchesTheScenePublishersNeeds(t *testing.T) {
	entry, err := survey(t).SiteIndex()
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID != "kirkham-9th-k3" || entry.PublishedAs != entry.ID || entry.Origin != Origin {
		t.Fatalf("identity %+v", entry)
	}
	if entry.Start != "2025-12-05T15:14:54.123456-08:00" || entry.Day != "2025-12-05" || entry.Clock != "3:14" {
		t.Fatalf("clock fields %+v", entry)
	}
	start, _ := time.Parse(time.RFC3339Nano, entry.Start)
	end, _ := time.Parse(time.RFC3339Nano, entry.End)
	if end.Sub(start) != 1374*time.Second || entry.Minutes != 22.9 {
		t.Fatalf("span %v minutes %v", end.Sub(start), entry.Minutes)
	}
	if len(entry.StaticParts) != 1 || entry.Captures[0] != "kirk2.pcapng" || *entry.NorthAzimuthDeg != 131 {
		t.Fatalf("parts %+v", entry)
	}
	raw, err := EncodeJSON([]SiteIndexEntry{entry})
	if err != nil || !strings.Contains(string(raw), `"map_mark": null`) {
		t.Fatalf("encoded %s %v", raw, err)
	}
}
