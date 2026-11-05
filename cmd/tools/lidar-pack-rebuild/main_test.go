package main

import (
	"encoding/binary"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
)

func TestSampleReturnsAddsWindowStarts(t *testing.T) {
	windows := []labelset.Window{{WindowID: 0, Start: 1, End: 100}, {WindowID: 1, Start: 100, End: 250}}
	domains := []labelset.DomainReturns{
		{DomainID: "fg-aaaaaaaaaaaa", WindowID: 1, Returns: labelset.Ranges{{Start: 0, Len: 2}, {Start: 10, Len: 1}}},
		{DomainID: "fg-aaaaaaaaaaaa", WindowID: 0, Returns: labelset.Ranges{{Start: 5, Len: 1}}},
		{DomainID: "fg-other", WindowID: 0, Returns: labelset.Ranges{{Start: 7, Len: 3}}},
	}
	got, err := sampleReturns(windows, domains, "fg-aaaaaaaaaaaa", 2)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]uint64{{6}, {100, 101, 110}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("returns %v, want %v", got, want)
	}
	if _, err := sampleReturns(windows, domains, "fg-aaaaaaaaaaaa", 1); err == nil {
		t.Fatal("a window beyond the samples was accepted")
	}
}

func TestEncodeSampleLaysOutAsThePack(t *testing.T) {
	pts := map[uint64]point{7: {1, 2, 3, 40}, 9: {4, 5, 6, 50}}
	b, missing := encodeSample([]uint64{7, 9}, pts)
	if missing != 0 || len(b) != 2*14 {
		t.Fatalf("%d bytes, %d missing", len(b), missing)
	}
	f := func(i int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:])) }
	if got := []float32{f(0), f(1), f(2), f(3), f(4), f(5)}; !reflect.DeepEqual(got, []float32{1, 4, 2, 5, 3, 6}) {
		t.Fatalf("coordinates %v: want all x, then y, then z", got)
	}
	if b[24] != 40 || b[25] != 50 || b[26] != classForeground || b[27] != classForeground {
		t.Fatalf("intensity and class %v", b[24:])
	}
	if _, missing := encodeSample([]uint64{7, 8}, pts); missing != 1 {
		t.Fatalf("missing %d, want 1", missing)
	}
}

func TestRewriteManifestRecomputesThePackDigest(t *testing.T) {
	m := map[string]any{
		"samples_sha256": "sha256:s", "packets_sha256": "sha256:p", "returns_sha256": "sha256:r",
		"backgrounds_sha256": "x", "background_points_sha256": "y", "background_count": 6.0,
		"coverage_note": "full file",
	}
	rewriteManifest(m, "sha256:points")
	if m["pack_digest"] != sha([]byte("sha256:points\nsha256:s\nsha256:p\nsha256:r")) {
		t.Fatalf("pack digest %v", m["pack_digest"])
	}
	if _, ok := m["backgrounds_sha256"]; ok || m["background_count"] != 0 {
		t.Fatal("backgrounds kept")
	}
	if note := m["coverage_note"].(string); !strings.HasPrefix(note, "full file; RECONSTRUCTED") {
		t.Fatalf("coverage note %q", note)
	}
}
