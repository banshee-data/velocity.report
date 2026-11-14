package harness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusCaptureRoots(t *testing.T) {
	src := t.TempDir()
	put(t, src, "corpus.json", `{"cases":[{"id":"kirk0","expected_capture_count":1}]}`)
	put(t, src, "index.json", `[{"id":"kirk0","captures":["kirk0.pcapng"]}]`)
	for _, root := range []string{"/mnt/captures/lidar", "/Volumes/captures/lidar", "/different/mount"} {
		t.Run(root, func(t *testing.T) {
			h := Host{CaptureRoot: "/legacy", CaptureSubdir: "s2"}
			c := Corpus{Corpus: "corpus.json", Index: "index.json", CaptureRoot: root}
			paths, err := caseCaptures(src, c, h, "kirk0")
			if err != nil || len(paths) != 1 || paths[0] != filepath.Join(root, "kirk0.pcapng") {
				t.Fatalf("paths %v: %v", paths, err)
			}
			effective := captureHost(h, c)
			if effective.CaptureRoot != root || effective.CaptureSubdir != "" {
				t.Fatalf("executor binding: %+v", effective)
			}
		})
	}
	c := Corpus{Corpus: "corpus.json", Index: "index.json"}
	paths, err := caseCaptures(src, c, Host{CaptureRoot: "/legacy", CaptureSubdir: "s2"}, "kirk0")
	if err != nil || paths[0] != "/legacy/s2/kirk0.pcapng" {
		t.Fatalf("legacy paths %v: %v", paths, err)
	}
	put(t, src, "index.json", `[{"id":"kirk0","captures":["../kirk0.pcapng"]}]`)
	c.CaptureRoot = "/mnt/captures/lidar"
	if _, err := caseCaptures(src, c, Host{}, "kirk0"); err == nil {
		t.Fatal("accepted escaping capture")
	}
}

func TestCorpusRootExecutorAndIdentity(t *testing.T) {
	o, bin := fixture(t)
	mount := filepath.Join(t.TempDir(), "mounted-captures")
	put(t, mount, "kirk0.pcapng", "capture on another mount")
	c := o.Host.Corpora["kirk0"]
	c.CaptureRoot = mount
	o.Host.Corpora["kirk0"] = c
	o.Host.CaptureRoot = "/unavailable-legacy-root"
	o.Host.CaptureSubdir = "s2"
	put(t, bin, "lidar-state-estimation-baseline", `#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in
 -out) shift; out="$1";;
 -pcap-root) shift; root="$1";;
 -pcap-subdir) shift; subdir="$1";;
 esac
 shift
done
[ -z "$subdir" ] || exit 42
[ -r "$root/kirk0.pcapng" ] || exit 43
mkdir -p "$out"
printf '%s\n' '{"cases":[{"first_run_frames":20,"duration_seconds":2,"continuity":{"tracks_confirmed":3}}]}' > "$out/phase0-summary.json"
`)
	if _, err := Tick(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	first := activeRun(t, o)
	if first.State != "complete" {
		t.Fatalf("%+v", first)
	}
	c.CaptureRoot = filepath.Join(t.TempDir(), "second-mount")
	put(t, c.CaptureRoot, "kirk0.pcapng", "capture on another mount")
	o.Host.Corpora["kirk0"] = c
	if _, err := Tick(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	second := activeRun(t, o)
	if second.InputDigest == first.InputDigest || second.ID == first.ID || second.State != "complete" {
		t.Fatalf("changed binding did not supersede: %+v", second)
	}
}

func TestReadHostRejectsRelativeCorpusRoot(t *testing.T) {
	o, _ := fixture(t)
	c := o.Host.Corpora["kirk0"]
	c.CaptureRoot = "relative/captures"
	o.Host.Corpora["kirk0"] = c
	raw, err := json.Marshal(o.Host)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "host.json")
	put(t, filepath.Dir(path), filepath.Base(path), string(raw))
	if _, err := ReadHost(path); err == nil || !strings.Contains(err.Error(), "capture_root must be absolute") {
		t.Fatalf("unexpected validation: %v", err)
	}
}

func TestCorpusRootBoxExecutor(t *testing.T) {
	o, bin := fixture(t)
	path := filepath.Join(o.Root, "data/experiments/host/host0001.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	put(t, o.Root, "data/experiments/host/host0001.md", strings.ReplaceAll(string(raw), "state_estimation_baseline", "box_eval"))
	gitTest(t, o.Root, "add", ".")
	gitTest(t, o.Root, "commit", "-m", "box fixture")
	mount := filepath.Join(t.TempDir(), "box-captures")
	put(t, mount, "kirk0.pcapng", "capture on box mount")
	c := o.Host.Corpora["kirk0"]
	c.CaptureRoot = mount
	o.Host.Corpora["kirk0"] = c
	o.Host.CaptureRoot = "/unavailable"
	o.Host.CaptureSubdir = "s2"
	put(t, bin, "lidar-box-eval", `#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in
 -out) shift; out="$1";;
 -pcap) shift; pcap="$1";;
 esac
 shift
done
[ -r "$pcap" ] || exit 42
printf '%s\n' '{"scored_frames":20,"confirmed_tracks":3,"wall_seconds":1}' > "$out"
`)
	if _, err := Tick(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if r := activeRun(t, o); r.State != "complete" {
		t.Fatalf("%+v", r)
	}
}
