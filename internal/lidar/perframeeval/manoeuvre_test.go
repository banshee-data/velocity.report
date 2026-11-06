package perframeeval

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

func manoeuvreFixture(t *testing.T) (ManoeuvreOptions, *Reference, ManoeuvreManifest) {
	t.Helper()
	f := fixture(t)
	_, frozen := freezeFixture(t, f)
	opts := ManoeuvreOptions{Reference: baseConfig(f).Reference,
		EventsPath: filepath.Join(t.TempDir(), "events.json"), SamplesPath: filepath.Join(t.TempDir(), "samples.jsonl")}
	opts.Reference.SplitManifestPath = frozen
	ref, err := LoadReference(opts.Reference)
	if err != nil {
		t.Fatal(err)
	}
	m := ManoeuvreManifest{SchemaVersion: 1, ReferenceDigest: ref.Identity.Digest, Events: []ManoeuvreEvent{{
		ID: "turn-a", EpisodeID: evalfixture.EpisodeHeldOut, ObjectID: evalfixture.CarA,
		Kind: "turn", StartNS: evalfixture.SampleTime(0), EndNS: evalfixture.SampleTime(9), Status: "reviewed", Reviewer: "fixture-reviewer"}}}
	return opts, ref, m
}

func writeManoeuvreInputs(t *testing.T, opts ManoeuvreOptions, ref *Reference, m ManoeuvreManifest, gated bool) {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.EventsPath, b, 0o600); err != nil {
		t.Fatal(err)
	}
	var rows strings.Builder
	for _, ep := range ref.Episodes {
		for _, series := range ep.Series {
			if series.ID != evalfixture.CarA {
				continue
			}
			for _, p := range series.Points {
				if p.Ignore {
					continue
				}
				b, err := json.Marshal(l5tracks.UncertaintySample{FrameUnixNanos: p.TimestampNanos, X: p.X, Y: p.Y, Rank: 2, Gated: gated})
				if err != nil {
					t.Fatal(err)
				}
				rows.Write(b)
				rows.WriteByte('\n')
			}
		}
	}
	if err := os.WriteFile(opts.SamplesPath, []byte(rows.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestManoeuvreReviewedFramesAndCoverage(t *testing.T) {
	opts, ref, m := manoeuvreFixture(t)
	// A second overlapping event must not inflate the denominator.
	m.Events = append(m.Events, m.Events[0])
	m.Events[1].ID, m.Events[1].Kind = "brake-a", "brake"
	for _, gated := range []bool{false, true} {
		writeManoeuvreInputs(t, opts, ref, m, gated)
		r, err := MeasureManoeuvres(opts)
		if err != nil {
			t.Fatal(err)
		}
		wantStatus, wantGated := "within", 0
		if gated {
			wantStatus, wantGated = "outside", 9
		}
		if r.Labelled != 9 || r.Eligible != 9 || r.Gated != wantGated || r.Status != wantStatus || r.Fraction == nil || !r.Reference.HeldOut || len(r.SamplesSHA256) != 64 || len(r.EventsSHA256) != 64 || r.GateVerdict != "individual_row_only_no_promotion" {
			t.Fatalf("unexpected report: %+v", r)
		}
	}
	// An empty recording and duplicate pairings must never pass as zero errors.
	for _, content := range []string{"", "duplicate", "late", "far"} {
		writeManoeuvreInputs(t, opts, ref, m, false)
		b, _ := os.ReadFile(opts.SamplesPath)
		switch content {
		case "duplicate":
			b = append(b, b...)
		case "late":
			b = []byte(`{"frame_unix_nanos":999,"x":1,"y":0,"rank":2,"gated":false}`)
		case "far":
			b = []byte(`{"frame_unix_nanos":1000000000,"x":999,"y":0,"rank":2,"gated":false}`)
		default:
			b = nil
		}
		if err := os.WriteFile(opts.SamplesPath, b, 0o600); err != nil {
			t.Fatal(err)
		}
		r, err := MeasureManoeuvres(opts)
		if err != nil || r.Status != "insufficient_samples" || r.Eligible != 0 {
			t.Fatalf("%s: %+v, %v", content, r, err)
		}
		if content == "duplicate" && r.Ambiguous != 9 {
			t.Fatalf("duplicate pairings not ambiguous: %+v", r)
		}
	}
	writeManoeuvreInputs(t, opts, ref, ManoeuvreManifest{SchemaVersion: 1, ReferenceDigest: ref.Identity.Digest}, false)
	r, err := MeasureManoeuvres(opts)
	if err != nil || r.Labelled != 0 || r.Fraction != nil || r.Status != "insufficient_samples" {
		t.Fatalf("no event labels: %+v, %v", r, err)
	}
}

func TestManoeuvreManifestRefusals(t *testing.T) {
	opts, ref, original := manoeuvreFixture(t)
	for _, change := range []func(*ManoeuvreManifest){
		func(m *ManoeuvreManifest) { m.SchemaVersion = 2 },
		func(m *ManoeuvreManifest) { m.ReferenceDigest = "stale" },
		func(m *ManoeuvreManifest) { m.Events[0].ID = " " },
		func(m *ManoeuvreManifest) { m.Events[0].EndNS = m.Events[0].StartNS - 1 },
		func(m *ManoeuvreManifest) { m.Events[0].Status = "proposed" },
		func(m *ManoeuvreManifest) { m.Events[0].Reviewer = " " },
		func(m *ManoeuvreManifest) { m.Events[0].Kind = "unknown" },
		func(m *ManoeuvreManifest) { m.Events[0].EpisodeID = "elsewhere" },
		func(m *ManoeuvreManifest) { m.Events[0].ObjectID = evalfixture.Ped },
		func(m *ManoeuvreManifest) { m.Events[0].StartNS, m.Events[0].EndNS = 0, 1 },
		func(m *ManoeuvreManifest) { m.Events[0].EndNS++ },
		func(m *ManoeuvreManifest) { m.Events = append(m.Events, m.Events[0]) },
	} {
		m := original
		m.Events = append([]ManoeuvreEvent(nil), original.Events...)
		change(&m)
		writeManoeuvreInputs(t, opts, ref, m, false)
		if _, err := MeasureManoeuvres(opts); err == nil {
			t.Fatalf("accepted invalid manifest %+v", m)
		}
	}
	writeManoeuvreInputs(t, opts, ref, original, false)
	for _, change := range []func(*ManoeuvreOptions){
		func(o *ManoeuvreOptions) { o.Reference.PackDir = "/missing" },
		func(o *ManoeuvreOptions) { o.Reference.Policy.Status = annotation.ReferenceIncludeProposed },
		func(o *ManoeuvreOptions) { o.EventsPath = "/missing" },
		func(o *ManoeuvreOptions) { o.SamplesPath = "/missing" },
	} {
		o := opts
		change(&o)
		if _, err := MeasureManoeuvres(o); err == nil {
			t.Fatalf("accepted invalid options %+v", o)
		}
	}
	if err := os.WriteFile(opts.EventsPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := MeasureManoeuvres(opts); err == nil {
		t.Fatal("accepted malformed events")
	}
	writeManoeuvreInputs(t, opts, ref, original, false)
	if err := os.WriteFile(opts.SamplesPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := MeasureManoeuvres(opts); err == nil {
		t.Fatal("accepted malformed samples")
	}
}

func TestManoeuvreStrictThresholdAndPartialCoverage(t *testing.T) {
	for _, test := range []struct {
		counts ManoeuvreCounts
		status string
	}{
		{ManoeuvreCounts{}, "insufficient_samples"},
		{ManoeuvreCounts{Eligible: 100, Gated: 1}, "outside"},
		{ManoeuvreCounts{Eligible: 101, Gated: 1}, "within"},
		{ManoeuvreCounts{Eligible: 100, Missing: 1}, "insufficient_samples"},
		{ManoeuvreCounts{Eligible: 100, Ambiguous: 1}, "insufficient_samples"},
	} {
		fraction, status := manoeuvreVerdict(test.counts)
		if status != test.status || (fraction == nil) != (test.counts.Eligible == 0) {
			t.Fatalf("%+v: fraction %v, status %s", test.counts, fraction, status)
		}
	}
}

type manoeuvreReadError struct{}

func (manoeuvreReadError) Read([]byte) (int, error) { return 0, errors.New("read failure") }

func TestManoeuvreSamplesAndSpatialGate(t *testing.T) {
	for _, input := range []string{"null", "{}", `{"frame_unix_nanos":1,"x":0,"y":0,"rank":3,"gated":false}`, `{"frame_unix_nanos":1,"x":0,"y":0,"rank":2}`, strings.Repeat(" ", (1<<20)+1)} {
		if _, err := readManoeuvreSamples(strings.NewReader(input), nil); err == nil {
			t.Fatalf("accepted invalid sample %.80s", input)
		}
	}
	if _, err := readManoeuvreSamples(manoeuvreReadError{}, nil); err == nil {
		t.Fatal("ignored reader failure")
	}
	p := l8analytics.SeriesPoint{FootprintDiagonalMetres: 4}
	if !manoeuvreNear(l5tracks.UncertaintySample{X: 3}, p) || manoeuvreNear(l5tracks.UncertaintySample{X: 3.01}, p) || manoeuvreNear(l5tracks.UncertaintySample{X: float32(math.Inf(1))}, p) {
		t.Fatal("spatial gate changed")
	}
	// Rank one remains eligible; unrelated frames are ignored after validation.
	rows, err := readManoeuvreSamples(strings.NewReader(`{"frame_unix_nanos":1,"x":0,"y":0,"rank":1,"gated":false}`), manoeuvreFrameMap{1: {"a": p}})
	if err != nil || len(rows[1]) != 1 {
		t.Fatalf("rank-one sample lost: %v, %v", rows, err)
	}
	if manoeuvreShared(l5tracks.UncertaintySample{}, "a", map[string]l8analytics.SeriesPoint{"a": p}) ||
		!manoeuvreShared(l5tracks.UncertaintySample{}, "a", map[string]l8analytics.SeriesPoint{"b": {Ignore: true}}) {
		t.Fatal("a shared sample was silently assigned to a labelled object")
	}
}

func TestManoeuvreDevelopmentReferenceIsExplicit(t *testing.T) {
	f := fixture(t)
	opts := ManoeuvreOptions{Reference: baseConfig(f).Reference,
		EventsPath: filepath.Join(t.TempDir(), "events.json"), SamplesPath: filepath.Join(t.TempDir(), "samples.jsonl")}
	opts.Reference.Split = evalfixture.SplitTuning
	opts.Reference.AllowTuningSplit = true
	ref, err := LoadReference(opts.Reference)
	if err != nil {
		t.Fatal(err)
	}
	m := ManoeuvreManifest{SchemaVersion: 1, ReferenceDigest: ref.Identity.Digest}
	writeManoeuvreInputs(t, opts, ref, m, false)
	r, err := MeasureManoeuvres(opts)
	if err != nil || r.Reference.HeldOut || !hasCaveat(r.Caveats, "Development/tuning") || !hasCaveat(r.Caveats, "Unfrozen") {
		t.Fatalf("development evidence lost its caveats: %+v, %v", r, err)
	}
	opts.Reference.AllowTuningSplit = false
	if _, err := MeasureManoeuvres(opts); err == nil {
		t.Fatal("tuning score allowed without explicit opt-in")
	}
}
