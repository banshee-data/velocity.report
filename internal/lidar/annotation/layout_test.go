package annotation

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCaptureNameOf(t *testing.T) {
	for in, want := range map[string]string{
		"kirk0.pcapng": "kirk0",
		"/Volumes/lidar/pcaps/20260902T1320_80858-0f4_columbus-broadway.pcapng": "20260902T1320_80858-0f4_columbus-broadway",
		"capture.pcap.gz":  "capture",
		"two words.pcap":   "two-words",
		"":                 "capture",
		"splits.pcapng":    "capture",
		`C:\caps\x.pcapng`: "x",
		"..":               "capture",
		"site/../a.pcapng": "a",
		"packs.pcap":       "capture",
	} {
		if got := CaptureNameOf(in); got != want {
			t.Errorf("CaptureNameOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestViewName(t *testing.T) {
	m := Manifest{CreatedNs: time.Date(2026, 9, 29, 18, 47, 18, 156, time.UTC).UnixNano()}
	m.Source.VRLOGPath = "ad8b9438-c1d4-40f0-bd0a-f1852c984569"
	if got := ViewName(m); got != "20260929-184718-ad8b9438" {
		t.Fatalf("ViewName = %q", got)
	}
}

func TestCaptureDirOf(t *testing.T) {
	var m Manifest
	m.Source.PCAPBasename = "kirk0.pcapng"
	for dir, want := range map[string]string{
		"/r/kirk0/packs/run-a":              "/r/kirk0",
		"/r/clip-j-1/pack":                  "/r/kirk0",
		"/r/kirk0/kirk0-reference/views/v1": "/r/kirk0",
		"/elsewhere/run-a":                  "/elsewhere/kirk0",
	} {
		if got := CaptureDirOf(dir, m); got != want {
			t.Errorf("CaptureDirOf(%q) = %q, want %q", dir, got, want)
		}
	}
}

func TestValidPackHandle(t *testing.T) {
	for h, want := range map[string]bool{
		"kirk0/packs/run-a":                       true,
		"clip-j-1/pack":                           true,
		"kirk0/packs/clip-j-1/pack":               false,
		"kirk0/kirk0-reference/views/20260929-ad": true,
		"run-a":                      false,
		"other/pack":                 false,
		"kirk0/packs/run-a/other":    false,
		"kirk0/set/other/v":          false,
		"kirk0/packs":                false,
		"../kirk0/packs/run-a":       false,
		"kirk0/packs/../packs/run-a": false,
		"/kirk0/packs/run-a":         false,
		"kirk0/packs/.hidden":        false,
		"splits/packs/x":             false,
		`kirk0\packs\run-a`:          false,
		"kirk0//packs/run-a":         false,
	} {
		if got := ValidPackHandle(h); got != want {
			t.Errorf("ValidPackHandle(%q) = %v, want %v", h, got, want)
		}
	}
}

func TestListPackDirs(t *testing.T) {
	root := t.TempDir()
	put := func(dir string) {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, manifestFile), []byte(`{"pack_digest":"sha256:`+filepath.Base(dir)+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{"kirk0/packs/run-a", "clip-j-1/pack", "kirk0/set/views/v1", "flat-old-pack", "splits/packs/x", "kirk0/.hidden/views/v"} {
		put(d)
	}
	got, err := ListPackDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"clip-j-1/pack", "kirk0/packs/run-a", "kirk0/set/views/v1"}
	if len(got) != len(want) {
		t.Fatalf("listed %v", got)
	}
	for i := range want {
		if h, err := PackHandle(root, got[i]); err != nil || h != want[i] {
			t.Errorf("listed %s (%v), want %s", h, err, want[i])
		}
	}
	if dir, err := FindPackByDigest(root, "sha256:v1"); err != nil || dir != filepath.Join(root, "kirk0/set/views/v1") {
		t.Fatalf("found %q %v", dir, err)
	}
}
