package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const laneSpec = `---
id: abcd0001
title: A lane
status: approved
owner: t
created: 2026-10-13
asks: [docs/q.md]
report: docs/r.md
arms:
  - name: a0
    tuning: config/tuning.defaults.json
  - name: b
    tuning: config/tuning.defaults.json
    replay: [support_filter, cascade]
cases:
  corpus: kirk-labelled
  count: 2
  sites: [kirk0, kirk1]
steps:
  - id: replay
    kind: state_estimation_baseline
    over: [arms, cases]
    estimate: { each: 10m }
  - id: write
    kind: manual:write
    estimate: { human: 2h }
---
`

// repo writes a one-experiment repository and returns its root and a state
// directory.
func repo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	for rel, content := range map[string]string{
		"config/tuning.defaults.json":       "{}",
		"docs/q.md":                         "# Q\n",
		"data/experiments/abcd/README.md":   "---\narea: abcd\ntitle: Test\nlayers: L3\nowner: t\n---\n",
		"data/experiments/abcd/abcd0001.md": laneSpec,
	} {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, filepath.Join(t.TempDir(), "state")
}

// cli runs the command and returns its exit code and standard output.
func cli(t *testing.T, args ...string) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	code := run(args)
	os.Stdout, os.Stderr = stdout, stderr
	w.Close()
	out, _ := io.ReadAll(r)
	return code, string(out)
}

func TestALaneMarksUnitsAndStatusFollows(t *testing.T) {
	root, state := repo(t)
	clock := time.Date(2026, 10, 13, 3, 0, 0, 0, time.UTC)
	now = func() time.Time { return clock }
	t.Cleanup(func() { now = time.Now })
	base := []string{"-root", root, "-state", state}
	do := func(want int, args ...string) string {
		t.Helper()
		code, out := cli(t, append(base, args...)...)
		if code != want {
			t.Fatalf("%v: exit %d, want %d\n%s", args, code, want, out)
		}
		return out
	}

	out := do(0, "units", "abcd0001", "-step", "replay")
	if !strings.Contains(out, "replay/b/kirk1\tb\tkirk1\tconfig/tuning.defaults.json\tsupport_filter,cascade\n") ||
		!strings.Contains(out, "replay/a0/kirk0\ta0\tkirk0\tconfig/tuning.defaults.json\t-\n") {
		t.Errorf("units:\n%s", out)
	}
	if out := do(0, "units", "abcd0001"); !strings.Contains(out, "write\t-\t-\t-\t-\n") {
		t.Errorf("all units:\n%s", out)
	}

	// Flags may follow the positional arguments.
	if out := do(0, "mark", "abcd0001", "replay/a0/kirk0", "start", "-worker", "m1", "-class", "m1pro", "-out", "/ev/x"); !strings.Contains(out, "running (attempt 1 on m1)") {
		t.Errorf("start: %s", out)
	}
	clock = clock.Add(15 * time.Minute)
	if out := do(0, "mark", "abcd0001", "replay/a0/kirk0", "done", "-worker", "m1"); !strings.Contains(out, "done (attempt 1 on m1), took 15m") {
		t.Errorf("done: %s", out)
	}
	if out := do(0, "units", "abcd0001", "-step", "replay", "-todo"); strings.Contains(out, "replay/a0/kirk0") || strings.Count(out, "\n") != 3 {
		t.Errorf("todo after one done:\n%s", out)
	}
	do(0, "mark", "abcd0001", "replay/b/kirk0", "start", "-worker", "m1")
	do(0, "mark", "abcd0001", "replay/b/kirk1", "start", "-worker", "m1")
	do(0, "mark", "abcd0001", "replay/b/kirk1", "failed", "-worker", "m1", "-note", "exit status 1")
	clock = clock.Add(5 * time.Minute)

	out = do(0, "status", "abcd0001")
	for _, want := range []string{
		"1 of 5 units done, 30m machine (~45m at 1.5x) + 2h person left",
		"replay       state_estimation_baseline  running           1/4, 1 running, 1 failed, took 15m",
		"replay/b/kirk0 running on m1 for 5m",
		"replay/b/kirk1 failed on m1: exit status 1",
		"write        manual:write               blocked           0/1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q:\n%s", want, out)
		}
	}
	if out := do(0, "status"); !strings.Contains(out, "abcd0001     approved    1/5   units") {
		t.Errorf("status of all:\n%s", out)
	}

	// Refusals.
	do(1, "mark", "abcd0001", "replay/a0/kirk0", "start", "-worker", "m1")
	do(1, "mark", "abcd0001", "write", "done", "-worker", "m1", "-force")
	do(1, "mark", "abcd0001", "replay/a0/kirk9", "start")
	do(1, "mark", "abcd0001", "replay/a0/kirk1", "start", "-class", "laptop")
	do(1, "mark", "nope0001", "x", "start")
	do(2, "mark", "abcd0001", "replay/a0/kirk1")
	do(2, "mark", "abcd0001", "replay/a0/kirk1", "start", "-bogus")
	do(1, "units", "abcd0001", "-step", "nope")
	do(1, "units", "nope0001")
	do(2, "units")
	do(2, "units", "-bogus")
	do(1, "status", "nope0001")
	do(2, "status", "a", "b")

	// A forced write closes on the report once it exists.
	if err := os.WriteFile(filepath.Join(root, "docs", "r.md"), []byte("# R\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := do(0, "mark", "abcd0001", "write", "done", "-artefact", "docs/r.md", "-force", "-wall", "90m"); !strings.Contains(out, "took 1h30m") {
		t.Errorf("forced write: %s", out)
	}
	if out := do(0, "status", "abcd0001"); !strings.Contains(out, "write closed on docs/r.md") {
		t.Errorf("closed artefact not shown:\n%s", out)
	}

	// A unit file the spec no longer has is reported.
	if err := os.MkdirAll(filepath.Join(state, "abcd0001", "gone"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "abcd0001", "gone", "x.json"), []byte(`{"step":"gone","arm":"x","state":"done"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := do(0, "status", "abcd0001"); !strings.Contains(out, "gone/x has a state file but is not in the spec") {
		t.Errorf("extra unit not shown:\n%s", out)
	}
	// A corrupt unit file stops every progress command.
	if err := os.WriteFile(filepath.Join(state, "abcd0001", "gone", "y.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	do(1, "status", "abcd0001")
	do(1, "status")
	do(1, "units", "abcd0001")
}
