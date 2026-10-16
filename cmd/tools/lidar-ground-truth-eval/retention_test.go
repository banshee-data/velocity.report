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

// The retention subcommand end to end, against the synthetic fixture and the
// replay evidence evalfixture.WriteRetentionDB describes case by case.

func retentionArgs(t *testing.T, extra ...string) ([]string, string) {
	t.Helper()
	f := perFrameFixture(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "observations.db")
	if err := evalfixture.WriteRetentionDB(dbPath, f.PackDir); err != nil {
		t.Fatal(err)
	}
	return append([]string{
		"-pack", f.PackDir, "-split-manifest", f.SplitManifestPath, "-split", evalfixture.SplitHeldOut, "-db", dbPath,
	}, extra...), dir
}

func TestRetentionWritesMarkdownAndJSON(t *testing.T) {
	args, dir := retentionArgs(t)
	jsonPath := filepath.Join(dir, "retention.json")
	// The top-level command dispatches to the subcommand: through the
	// default mode, -h after "retention" would be a usage error, not zero.
	if code := run([]string{"retention", "-h"}); code != 0 {
		t.Fatalf("retention -h through run: exit %d, want 0", code)
	}
	var stdout, stderr bytes.Buffer
	if code := runRetention(append(args, "-json", jsonPath), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	md := stdout.String()
	for _, want := range []string{"# Retention and fragmentation per reviewed mask", "**Approximate.**",
		"| all | 7 |", "| range_0_20m | 7 |", "| range_50m_plus | 0 |", "| obj_car_a | car | 7 |"} {
		if !strings.Contains(md, want) {
			t.Fatalf("summary lacks %q:\n%s", want, md)
		}
	}
	b, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var r perframeeval.RetentionReport
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if r.Schema != perframeeval.RetentionSchema || len(r.Instants) != 7 || r.Total.Fragmented != 1 || r.Total.Missed != 3 ||
		!r.Approximate || r.Evidence.SourceID != evalfixture.RetentionSourceID {
		t.Fatalf("report = schema %s, %d instants, total %+v, approximate %v", r.Schema, len(r.Instants), r.Total, r.Approximate)
	}
}

// Range bands are the operator's: moving the first edge below the car's
// range moves every instant out of the first band.
func TestRetentionRangeBandsFlag(t *testing.T) {
	args, _ := retentionArgs(t, "-range-bands", "0.5, 100")
	var stdout, stderr bytes.Buffer
	if code := runRetention(args, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if md := stdout.String(); !strings.Contains(md, "| range_0_0.5m | 1 |") || !strings.Contains(md, "| range_0.5_100m | 6 |") {
		t.Fatalf("bands not applied:\n%s", md)
	}
}

func TestRetentionUsageErrors(t *testing.T) {
	args, _ := retentionArgs(t)
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"no db":          {args[:len(args)-2], "-db is required"},
		"no split":       {[]string{"-pack", "p", "-split-manifest", "m", "-db", "d"}, "-split is required"},
		"bad bands":      {append(args, "-range-bands", "20,x"), "is not a number"},
		"falling bands":  {append(args, "-range-bands", "35,20"), "ascending"},
		"bad position":   {append(args, "-reference-position", "roof"), "reference position"},
		"stray argument": {append(args, "extra"), "unexpected arguments"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runRetention(tc.args, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), tc.want) {
			t.Errorf("%s: exit %d, stderr %q; want 2 and %q", name, code, stderr.String(), tc.want)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := runRetention([]string{"-h"}, &stdout, &stderr); code != 0 || !strings.Contains(stderr.String(), "Usage: lidar-ground-truth-eval retention") {
		t.Fatalf("-h: exit %d, %q", code, stderr.String())
	}
	// A refusal from the measure is exit 1, not a usage error.
	if code := runRetention(append(args, "-source", "source/v1/elsewhere"), &stdout, &stderr); code != 1 {
		t.Fatalf("unknown source: exit %d, want 1", code)
	}
}
