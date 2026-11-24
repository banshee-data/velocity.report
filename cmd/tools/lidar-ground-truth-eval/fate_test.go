package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

// The fate subcommand end to end, against the fixture whose cases
// evalfixture.WriteFate describes.

func fateArgs(t *testing.T, extra ...string) ([]string, *evalfixture.FateFixture) {
	t.Helper()
	f, err := evalfixture.WriteFate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return append([]string{
		"-pack", f.PackDir, "-split-manifest", f.SplitManifestPath, "-split", evalfixture.SplitHeldOut,
		"-observations", f.ObservationsDir,
	}, extra...), f
}

func TestFateWritesMarkdownAndJSON(t *testing.T) {
	args, _ := fateArgs(t)
	jsonPath := filepath.Join(t.TempDir(), "fate.json")
	if code := run([]string{"fate", "-h"}); code != 0 {
		t.Fatalf("fate -h through run: exit %d, want 0", code)
	}
	var stdout, stderr bytes.Buffer
	if code := runFate(append(args, "-json", jsonPath), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	md := stdout.String()
	for _, want := range []string{"# Fate of every reviewed return", "match rate 0.7000", "| all | 5 | 20 |",
		"| obj_car_a | car | 5 | 20 |", "## Comparable with the retention measure", "## Retained membership versus dropped returns", "| all | 20 | 4 | 1 | 1 | 8 | 1 | 5 |"} {
		if !strings.Contains(md, want) {
			t.Fatalf("summary lacks %q:\n%s", want, md)
		}
	}
	// Exercise dispatch with the same synthetic evidence, then verify its JSON.
	if code := run(append([]string{"fate"}, append(args, "-json", jsonPath)...)); code != 0 {
		t.Fatalf("fate through run: exit %d", code)
	}
	b, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var r perframeeval.FateReport
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	wantMembership := perframeeval.FateMembership{Main: 4, Secondary: 1, L3Background: 1, L4Rejected: 8, OtherFrame: 1, Unresolved: 5}
	for _, summary := range []perframeeval.FateSummary{r.Total, r.ByClass[0], r.ByRange[0], r.ByObject[0]} {
		if summary.Membership != wantMembership {
			t.Errorf("%s membership = %+v, want %+v", summary.Name, summary.Membership, wantMembership)
		}
	}
	if r.Instants[0].Membership != (perframeeval.FateMembership{Main: 2, Secondary: 1, L3Background: 1}) {
		t.Errorf("instant membership = %+v", r.Instants[0].Membership)
	}
	if r.ByRange[1].Membership != (perframeeval.FateMembership{}) {
		t.Errorf("empty band membership = %+v", r.ByRange[1].Membership)
	}
	if r.Schema != perframeeval.FateSchema || len(r.Instants) != 5 || r.Total.Fates[perframeeval.FateClustered] != 5 ||
		r.Join.Matched != 13 || r.Total.Fragmented != 1 || r.Total.GapFates[perframeeval.FateL3Background] != 1 {
		t.Fatalf("report = schema %s, %d instants, total %+v, join %+v", r.Schema, len(r.Instants), r.Total, r.Join)
	}
}

func TestFateUsageErrors(t *testing.T) {
	args, f := fateArgs(t)
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"no observations": {args[:len(args)-2], "-observations is required"},
		"no split":        {[]string{"-pack", "p", "-split-manifest", "m", "-observations", "o"}, "-split is required"},
		"bad bands":       {append(args, "-range-bands", "20,x"), "is not a number"},
		"bad port":        {append(args, "-port", "0"), "UDP port"},
		"stray argument":  {append(args, "extra"), "unexpected arguments"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runFate(tc.args, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), tc.want) {
			t.Errorf("%s: exit %d, stderr %q; want 2 and %q", name, code, stderr.String(), tc.want)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := runFate([]string{"-h"}, &stdout, &stderr); code != 0 || !strings.Contains(stderr.String(), "Usage: lidar-ground-truth-eval fate") {
		t.Fatalf("-h: exit %d, %q", code, stderr.String())
	}
	// A refusal from the measure is exit 1: here the pack itself offered as
	// the capture, which is not the container's source.
	stderr.Reset()
	if code := runFate(append(args, "-pcap", filepath.Join(f.PackDir, "points.bin")), &stdout, &stderr); code != 1 {
		t.Fatalf("foreign capture: exit %d, want 1 (%s)", code, stderr.String())
	}
}
