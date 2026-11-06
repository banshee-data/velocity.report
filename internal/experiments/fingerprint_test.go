package experiments

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/config"
)

// defaultsJSON is the repository's tuning defaults, so fixture arms name a
// file that loads and fingerprints as a real one does.
func defaultsJSON(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "tuning.defaults.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func defaultsFingerprint(t *testing.T) string {
	t.Helper()
	cfg, err := config.ParseTuningConfig([]byte(defaultsJSON(t)))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Fingerprint()
}

// changedJSON is the defaults with the frame budget moved, so the file loads
// and its fingerprint differs.
func changedJSON(t *testing.T) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(defaultsJSON(t)), &doc); err != nil {
		t.Fatal(err)
	}
	pipeline, ok := doc["pipeline"].(map[string]any)
	if !ok {
		t.Fatal("defaults have no pipeline block")
	}
	pipeline["frame_budget_ms"] = 987.5
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := config.ParseTuningConfig(out)
	if err != nil {
		t.Fatalf("changed defaults do not load: %v", err)
	}
	if moved.Fingerprint() == defaultsFingerprint(t) {
		t.Fatal("moving the frame budget did not move the fingerprint")
	}
	return string(out)
}

func validateErrs(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	for _, e := range load(t, root).Validate(root) {
		b.WriteString(e.Error() + "\n")
	}
	return b.String()
}

func TestOpenSpecMustBePinned(t *testing.T) {
	spec := strings.ReplaceAll(validSpec, "    fingerprint: @FP@\n", "")
	root := tree(t, map[string]string{"data/experiments/abcd/abcd0001.md": spec})
	got := validateErrs(t, root)
	if !strings.Contains(got, `arm "a0" is not pinned`) || !strings.Contains(got, `arm "b" is not pinned`) {
		t.Fatalf("unpinned open spec not refused for both arms:\n%s", got)
	}
}

func TestOpenSpecRefusedWhenItsFileDrifts(t *testing.T) {
	root := tree(t, map[string]string{
		"data/experiments/abcd/abcd0001.md": validSpec,
		"config/experiments/set.json":       changedJSON(t),
	})
	got := validateErrs(t, root)
	if !strings.Contains(got, `arm "b": config/experiments/set.json changed under the spec`) {
		t.Fatalf("drifted arm not refused:\n%s", got)
	}
	if strings.Contains(got, `arm "a0"`) {
		t.Fatalf("an arm whose file did not change was refused:\n%s", got)
	}
}

func TestOpenSpecRefusesUnknownAndMalformedFingerprints(t *testing.T) {
	for name, tc := range map[string]struct{ to, want string }{
		"unknown":   {"    fingerprint: unknown\n", "only for answered arms"},
		"malformed": {"    fingerprint: ABC\n", "is not twelve lower-case hex digits"},
	} {
		t.Run(name, func(t *testing.T) {
			spec := strings.Replace(validSpec, "    fingerprint: @FP@\n", tc.to, 1)
			got := validateErrs(t, tree(t, map[string]string{"data/experiments/abcd/abcd0001.md": spec}))
			if !strings.Contains(got, tc.want) {
				t.Fatalf("want %q in:\n%s", tc.want, got)
			}
		})
	}
}

func answered(spec, created string) string {
	spec = strings.Replace(spec, "status: proposed", "status: answered\nreport: docs/report.md", 1)
	return strings.Replace(spec, "created: 2026-10-13", "created: "+created, 1)
}

func TestAnsweredSpecKeepsItsFingerprintAndItsReportCarriesIt(t *testing.T) {
	// The file changed after the run (an adoption): an answered spec is not
	// refused for that, but its report must name what it ran under.
	spec := answered(validSpec, "2026-11-10")
	files := map[string]string{
		"data/experiments/abcd/abcd0001.md": spec,
		"config/experiments/set.json":       changedJSON(t),
	}
	got := validateErrs(t, tree(t, files))
	if !strings.Contains(got, "report docs/report.md does not carry the arm's fingerprint") {
		t.Fatalf("report without the fingerprint accepted:\n%s", got)
	}
	if strings.Contains(got, "changed under the spec") {
		t.Fatalf("an answered spec was refused for a later change to its file:\n%s", got)
	}
	files["docs/report.md"] = "# R\n\nBoth arms ran under @FP@.\n"
	if got := validateErrs(t, tree(t, files)); got != "" {
		t.Fatalf("report carrying the fingerprint refused:\n%s", got)
	}
}

func TestAnsweredSpecsBeforePinningMayOmitFingerprints(t *testing.T) {
	legacy := strings.ReplaceAll(answered(validSpec, "2026-10-13"), "    fingerprint: @FP@\n", "")
	if got := validateErrs(t, tree(t, map[string]string{"data/experiments/abcd/abcd0001.md": legacy})); got != "" {
		t.Fatalf("legacy answered spec refused:\n%s", got)
	}
	unknown := strings.ReplaceAll(answered(validSpec, "2026-11-10"), "@FP@", FingerprintUnknown)
	if got := validateErrs(t, tree(t, map[string]string{"data/experiments/abcd/abcd0001.md": unknown})); got != "" {
		t.Fatalf("answered spec marked unknown refused:\n%s", got)
	}
	recent := strings.ReplaceAll(answered(validSpec, "2026-11-10"), "    fingerprint: @FP@\n", "")
	if got := validateErrs(t, tree(t, map[string]string{"data/experiments/abcd/abcd0001.md": recent})); !strings.Contains(got, "carries the fingerprint it ran under") {
		t.Fatalf("answered spec created after pinning accepted without fingerprints:\n%s", got)
	}
}

func TestPinRecordsAndRestatesFingerprints(t *testing.T) {
	spec := strings.ReplaceAll(validSpec, "    fingerprint: @FP@\n", "")
	root := tree(t, map[string]string{
		"data/experiments/abcd/abcd0001.md": spec,
		"config/experiments/set.json":       changedJSON(t),
	})
	sp := load(t, root).Get("abcd0001")
	changed, err := Pin(root, sp)
	if err != nil || !changed {
		t.Fatalf("Pin = %v, %v; want changed", changed, err)
	}
	if got := validateErrs(t, root); got != "" {
		t.Fatalf("pinned spec refused:\n%s", got)
	}
	raw, _ := os.ReadFile(filepath.Join(root, sp.Path))
	if n := strings.Count(string(raw), "fingerprint:"); n != 2 {
		t.Fatalf("%d fingerprint lines after pinning, want 2:\n%s", n, raw)
	}
	if !strings.Contains(string(raw), "    tuning: config/tuning.defaults.json\n    fingerprint: "+defaultsFingerprint(t)+"\n") {
		t.Fatalf("a0's fingerprint not on the line after its tuning:\n%s", raw)
	}
	if !strings.HasSuffix(string(raw), "# body\n") {
		t.Fatal("pinning changed the body")
	}

	// The file moves back: re-pinning replaces the fingerprint, not adds one.
	if err := os.WriteFile(filepath.Join(root, "config/experiments/set.json"), []byte(defaultsJSON(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := validateErrs(t, root); !strings.Contains(got, "changed under the spec") {
		t.Fatalf("drift after pinning not refused:\n%s", got)
	}
	sp = load(t, root).Get("abcd0001")
	if changed, err := Pin(root, sp); err != nil || !changed {
		t.Fatalf("re-pin = %v, %v", changed, err)
	}
	raw, _ = os.ReadFile(filepath.Join(root, sp.Path))
	if n := strings.Count(string(raw), "fingerprint:"); n != 2 {
		t.Fatalf("%d fingerprint lines after re-pinning, want 2", n)
	}
	if changed, err := Pin(root, load(t, root).Get("abcd0001")); err != nil || changed {
		t.Fatalf("pinning a pinned spec = %v, %v; want unchanged", changed, err)
	}
}

func TestPinRefusesAnsweredSpecs(t *testing.T) {
	root := tree(t, map[string]string{"data/experiments/abcd/abcd0001.md": answered(validSpec, "2026-10-13")})
	if _, err := Pin(root, load(t, root).Get("abcd0001")); err == nil || !strings.Contains(err.Error(), "keeps the fingerprints it ran under") {
		t.Fatalf("Pin on an answered spec = %v", err)
	}
}
