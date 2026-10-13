package experiments

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The repository's own specs must validate, and the generated overview must
// be the fresh render, so a spec cannot change without its graph.
func TestRepositoryExperimentsValidate(t *testing.T) {
	root := filepath.Join("..", "..")
	set, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Specs) == 0 {
		t.Fatal("no experiment specs found under " + Dir)
	}
	for _, e := range set.Validate(root) {
		t.Error(e)
	}
}

func TestDependenciesDocIsCurrent(t *testing.T) {
	root := filepath.Join("..", "..")
	set, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadFile(filepath.Join(root, DependenciesFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != set.DependenciesDoc() {
		t.Fatalf("%s is out of date: run make experiments-graph", DependenciesFile)
	}
}

// tree writes a minimal repository: one area, the default tuning file, a
// question document, and the given files.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	base := map[string]string{
		"config/tuning.defaults.json":          "{}",
		"config/experiments/set.json":          "{}",
		"docs/question.md":                     "# Q\n",
		"docs/report.md":                       "# R\n",
		"data/experiments/abcd/README.md":      "---\narea: abcd\ntitle: Test area\nlayers: L3\nowner: t\n---\n",
		"data/experiments/try/legacy.md":       "# not a spec\n",
		"data/experiments/notanarea/README.md": "# no header, skipped\n",
	}
	maps.Copy(base, files)
	for rel, content := range base {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const validSpec = `---
id: abcd0001
title: A test
status: proposed
owner: t
created: 2026-10-13
asks: [docs/question.md#q1]
requires:
  capabilities: [pcap:kirk]
  disk_gb: 2
arms:
  - name: a0
    tuning: config/tuning.defaults.json
  - name: b
    tuning: config/experiments/set.json
    replay: [support_filter]
cases:
  corpus: kirk-labelled
  count: 2
steps:
  - id: replay
    kind: state_estimation_baseline
    over: [arms, cases]
    estimate: { each: 1m }
  - id: label
    kind: manual:label
    estimate: { human: 2h }
    gate: { labelled_fraction: 0.9 }
  - id: write
    kind: manual:write
    estimate: { human: 1h }
---

# body
`

func load(t *testing.T, root string) *Set {
	t.Helper()
	set, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestValidSpecPasses(t *testing.T) {
	root := tree(t, map[string]string{"data/experiments/abcd/abcd0001.md": validSpec})
	set := load(t, root)
	if len(set.Specs) != 1 || len(set.Areas) != 1 {
		t.Fatalf("loaded %d specs in %d areas, want 1 in 1 (try/ and header-less directories skipped)", len(set.Specs), len(set.Areas))
	}
	if errs := set.Validate(root); len(errs) != 0 {
		t.Fatalf("valid spec refused: %v", errs)
	}
	machine, human := set.Specs[0].Estimate()
	if machine != 4*time.Minute || human != 3*time.Hour {
		t.Errorf("estimate %v machine %v person, want 4m (2 arms x 2 cases x 1m) and 3h", machine, human)
	}
}

func TestValidateRefusesEachBrokenRule(t *testing.T) {
	cases := []struct {
		name, from, to, want string
	}{
		{"slug", "id: abcd0001", "id: ab1", "is not a slug"},
		{"title", "title: A test", "title: \"\"", "title is empty"},
		{"date", "created: 2026-10-13", "created: last week", "is not a date"},
		{"status", "status: proposed", "status: running", "want proposed, approved, answered or retired"},
		{"answered without report", "status: proposed", "status: answered", "answered without a report"},
		{"retired without reason", "status: proposed", "status: retired", "retired without a reason"},
		{"asks missing file", "asks: [docs/question.md#q1]", "asks: [docs/nowhere.md]", "want a decision id"},
		{"no asks", "asks: [docs/question.md#q1]", "asks: []", "asks names no question"},
		{"missing dependency", "asks: [docs/question.md#q1]", "asks: [D-28]\ndepends_on: [abcd0099]", "does not exist"},
		{"capability", "capabilities: [pcap:kirk]", "capabilities: [gpu]", "capability \"gpu\""},
		{"arm outside parameter sets", "tuning: config/experiments/set.json", "tuning: scratch/mine.json", "must be config/tuning.defaults.json or a parameter set"},
		{"arm file missing", "tuning: config/experiments/set.json", "tuning: config/experiments/gone.json", "does not exist"},
		{"replay option list", "replay: [support_filter]", "replay: [\"a,b\"]", "must be one -experiment name"},
		{"corpus", "corpus: kirk-labelled", "corpus: everything", "corpus \"everything\""},
		{"count exceeds corpus", "count: 2", "count: 9", "exceeds corpus"},
		{"unknown kind", "kind: manual:write", "kind: shell", "kind \"shell\""},
		{"manual needs human time", "estimate: { human: 1h }", "estimate: { total: 1h }", "needs estimate.human"},
		{"job needs one estimate", "estimate: { each: 1m }", "estimate: { each: 1m, total: 5m }", "exactly one of estimate.each"},
		{"each needs fan-out", "over: [arms, cases]\n    estimate: { each: 1m }", "estimate: { each: 1m }", "must fan out"},
		{"over axis", "over: [arms, cases]", "over: [arms, sites]", "want arms or cases"},
		{"gate on a job", "estimate: { each: 1m }", "estimate: { each: 1m }\n    gate: { x: 1 }", "a gate belongs on a manual step"},
		{"forward after", "  - id: write\n    kind: manual:write", "  - id: write\n    after: [later]\n    kind: manual:write", "no such step"},
		{"reclaim", "requires:", "reclaim: sometimes\nrequires:", "want any, origin or never"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(validSpec, c.from) {
				t.Fatalf("fixture does not contain %q", c.from)
			}
			spec := strings.Replace(validSpec, c.from, c.to, 1)
			root := tree(t, map[string]string{"data/experiments/abcd/abcd0001.md": spec})
			set, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			errs := set.Validate(root)
			for _, e := range errs {
				if strings.Contains(e.Error(), c.want) {
					return
				}
			}
			t.Errorf("want an error containing %q, got %v", c.want, errs)
		})
	}
}

func TestValidateRefusesWrongPlacementAndCycles(t *testing.T) {
	other := strings.Replace(validSpec, "id: abcd0001", "id: abcd0002", 1)
	other = strings.Replace(other, "asks: [docs/question.md#q1]", "asks: [D-1]\ndepends_on: [abcd0001]", 1)
	first := strings.Replace(validSpec, "asks: [docs/question.md#q1]", "asks: [D-1]\ndepends_on: [abcd0002]", 1)
	root := tree(t, map[string]string{
		"data/experiments/abcd/abcd0001.md": first,
		"data/experiments/abcd/abcd0002.md": other,
		"data/experiments/abcd/wrong.md":    strings.Replace(validSpec, "id: abcd0001", "id: abcd0003", 1),
		"data/experiments/abcd/zzzz0001.md": strings.Replace(validSpec, "id: abcd0001", "id: zzzz0001", 1),
	})
	set := load(t, root)
	joined := ""
	for _, e := range set.Validate(root) {
		joined += e.Error() + "\n"
	}
	for _, want := range []string{
		"dependency cycle: abcd0001 -> abcd0002 -> abcd0001",
		"file must be data/experiments/abcd/abcd0003.md",
		`area "zzzz" of zzzz0001 is not a registered area`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
}

func TestReadFileRefusesUnknownFieldsAndMissingHeader(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "a.md")
	os.WriteFile(bad, []byte(strings.Replace(validSpec, "owner: t", "owner: t\nestimated: 3h", 1)), 0o644)
	if _, err := ReadFile(bad); err == nil || !strings.Contains(err.Error(), "estimated") {
		t.Errorf("a misspelt field was accepted: %v", err)
	}
	none := filepath.Join(dir, "b.md")
	os.WriteFile(none, []byte("# just prose\n"), 0o644)
	if _, err := ReadFile(none); err == nil {
		t.Error("a file without a header was accepted")
	}
}

func TestParseDurationHasNoDays(t *testing.T) {
	if v, err := ParseDuration("2h30m"); err != nil || v != 150*time.Minute {
		t.Errorf("2h30m: %v %v", v, err)
	}
	for _, bad := range []string{"1d", "-5m", "soon"} {
		if _, err := ParseDuration(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if got := FormatDuration(26*time.Hour + 2*time.Minute); got != "26h2m" {
		t.Errorf("FormatDuration = %q", got)
	}
}

// Steps run along their After graph: a step with no After follows the one
// before it; "-" starts a parallel branch; the estimate is the longest chain.
func TestEstimateFollowsTheLongestChainOfSteps(t *testing.T) {
	spec := &Spec{
		Arms:  []Arm{{Name: "a"}, {Name: "b"}},
		Cases: &Cases{Count: 3},
		Steps: []Step{
			{ID: "replay", Kind: "state_estimation_baseline", Over: []string{"arms", "cases"}, Estimate: Estimate{Each: Duration(time.Minute)}},
			{ID: "side", Kind: "benchmark", After: []string{"-"}, Estimate: Estimate{Total: Duration(2 * time.Hour)}},
			{ID: "score", Kind: "track_scorecard", After: []string{"replay"}, Estimate: Estimate{Total: Duration(4 * time.Minute)}},
			{ID: "write", Kind: "manual:write", After: []string{"score", "side"}, Estimate: Estimate{Human: Duration(time.Hour)}},
		},
	}
	machine, human := spec.Estimate()
	if machine != 2*time.Hour || human != time.Hour {
		t.Errorf("estimate %v + %v, want the 2h side branch, not 6m+4m in series with it, and 1h person", machine, human)
	}
}

func TestReadinessAndCriticalPath(t *testing.T) {
	mk := func(id string, st Status, deps []string, first string, d time.Duration) *Spec {
		est := Estimate{Total: Duration(d)}
		if strings.HasPrefix(first, "manual:") {
			est = Estimate{Human: Duration(d)}
		}
		return &Spec{ID: id, Status: st, DependsOn: deps, Steps: []Step{{ID: "s", Kind: first, Estimate: est}}}
	}
	specs := []*Spec{
		mk("aaaa0001", StatusAnswered, nil, "benchmark", time.Hour),
		mk("aaaa0002", StatusApproved, []string{"aaaa0001"}, "benchmark", 2*time.Hour),
		mk("aaaa0003", StatusApproved, []string{"aaaa0001"}, "manual:annotate", 3*time.Hour),
		mk("aaaa0004", StatusApproved, []string{"aaaa0001"}, "bench_sweep", time.Hour),
		mk("aaaa0005", StatusApproved, []string{"aaaa0002", "aaaa0003"}, "benchmark", time.Hour),
		mk("aaaa0006", StatusProposed, nil, "benchmark", time.Hour),
		mk("aaaa0007", StatusRetired, nil, "benchmark", time.Hour),
	}
	set := &Set{Specs: specs, byID: map[string]*Spec{}}
	for _, s := range specs {
		set.byID[s.ID] = s
	}
	want := map[string]Readiness{
		"aaaa0001": "", "aaaa0002": ReadyForWorker, "aaaa0003": ReadyForPerson, "aaaa0004": NeedsTooling,
		"aaaa0005": Waiting, "aaaa0006": NotApproved, "aaaa0007": "",
	}
	for id, w := range want {
		if got := set.Readiness(set.Get(id)); got != w {
			t.Errorf("%s: readiness %q, want %q", id, got, w)
		}
	}
	chain, total := set.CriticalPath()
	if strings.Join(chain, ",") != "aaaa0003,aaaa0005" || total != 4*time.Hour {
		t.Errorf("critical path %v %v, want the 3h manual step then the 1h job", chain, total)
	}
}

func TestMermaidDrawsStatusAndDependencies(t *testing.T) {
	root := tree(t, map[string]string{
		"data/experiments/abcd/abcd0001.md": strings.Replace(validSpec, "status: proposed", "status: answered\nreport: docs/report.md", 1),
		"data/experiments/abcd/abcd0002.md": strings.Replace(strings.Replace(validSpec, "id: abcd0001", "id: abcd0002", 1),
			"asks: [docs/question.md#q1]", "asks: [docs/question.md#q1, D-28]\ndepends_on: [abcd0001]", 1),
	})
	set := load(t, root)
	if errs := set.Validate(root); len(errs) != 0 {
		t.Fatal(errs)
	}
	m := set.Mermaid()
	for _, want := range []string{
		`subgraph area_abcd ["abcd: Test area"]`,
		`abcd0001["<b>abcd0001</b><br/>A test<br/>answered`,
		":::answered", ":::proposed",
		"abcd0001 --> abcd0002",
		"manual: label, write",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("mermaid lacks %q:\n%s", want, m)
		}
	}
	doc := set.DependenciesDoc()
	for _, want := range []string{"[report](../../docs/report.md)", "```mermaid\n",
		"asks [question #q1](../../docs/question.md#q1), [D-28](../../docs/DECISIONS.md)."} {
		if !strings.Contains(doc, want) {
			t.Errorf("overview lacks %q:\n%s", want, doc)
		}
	}
}

func TestQuestionLabelShortensLongAnchors(t *testing.T) {
	got := questionLabel("data/maths/proposals/x.md#51-arrival-is-bistable-and-its-clock-is-the-backgrounds-age")
	if got != "x #51-arrival-is-bistable-and-its-…" {
		t.Errorf("questionLabel = %q", got)
	}
	if questionLabel("D-29") != "D-29" {
		t.Error("a decision id was rewritten")
	}
}

func TestKindsComeFromTheJobRegistry(t *testing.T) {
	if ClassifyKind("state_estimation_baseline") != KindJob || ClassifyKind("bench_sweep") != KindPlanned ||
		ClassifyKind("manual:freeze") != KindManual || ClassifyKind("rm -rf") != KindUnknown {
		t.Error("kind classes do not follow the job, planned and manual registries")
	}
	for k := range PlannedKinds {
		if ClassifyKind(k) == KindJob {
			t.Errorf("%s is a job kind now: remove it from PlannedKinds", k)
		}
	}
}
