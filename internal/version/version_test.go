package version

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// was written. Print uses fmt.Printf directly, so this is the only way to
// observe it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = orig })

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing pipe writer: %v", err)
	}
	return <-done
}

// setVars swaps the package-level build stamps for the duration of a test.
func setVars(t *testing.T, version, sha, buildTime string) {
	t.Helper()
	origVersion, origSHA, origBuild := Version, GitSHA, BuildTime
	Version, GitSHA, BuildTime = version, sha, buildTime
	t.Cleanup(func() {
		Version, GitSHA, BuildTime = origVersion, origSHA, origBuild
	})
}

func TestPrintIncludesBinaryNameAndBuildStamps(t *testing.T) {
	setVars(t, "0.5.1", "abc1234", "2026-08-14T00:00:00Z")

	got := captureStdout(t, func() { Print("velocity-report") })

	for _, want := range []string{
		"velocity-report  v0.5.1",
		"git sha:  abc1234",
		"built:  2026-08-14T00:00:00Z",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n---\n%s", want, got)
		}
	}
	// Three labelled lines, one per stamp.
	if lines := strings.Count(got, "\n"); lines != 3 {
		t.Errorf("line count = %d, want 3\n---\n%s", lines, got)
	}
}

func TestPrintUsesDefaultsWhenUnstamped(t *testing.T) {
	// An un-stamped dev build still prints something usable rather than
	// blank fields.
	setVars(t, "dev", "unknown", "unknown")

	got := captureStdout(t, func() { Print("velocity") })

	if !strings.Contains(got, "velocity  vdev") {
		t.Errorf("output = %q, want the dev version", got)
	}
	if strings.Count(got, "unknown") != 2 {
		t.Errorf("output = %q, want both sha and build time as \"unknown\"", got)
	}
}

func TestPrintUsesSuppliedBinaryName(t *testing.T) {
	setVars(t, "1.0.0", "sha", "when")

	got := captureStdout(t, func() { Print("some-other-binary") })

	if !strings.HasPrefix(got, "some-other-binary  v1.0.0") {
		t.Errorf("output = %q, want it to lead with the supplied binary name", got)
	}
}
