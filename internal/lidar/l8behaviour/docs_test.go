package l8behaviour

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The registries are documents, and these tests are what keep them honest: a
// token or metric added in code without its registry row fails here, as does
// a registry row that code does not recognise.

const (
	metricsRegistryDoc = "docs/platform/architecture/metrics-registry.md"
	labelVocabularyDoc = "docs/lidar/architecture/label-vocabulary.md"
)

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	for _, prefix := range []string{"", "../../../"} {
		raw, err := os.ReadFile(filepath.Join(prefix, rel))
		if err == nil {
			return string(raw)
		}
	}
	t.Fatalf("could not read %s from the package directory", rel)
	return ""
}

// tableCells splits a Markdown table row into trimmed cells.
func tableCells(row string) []string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(row), "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func TestMetricsRegistryDocumentsEveryFollowingMetric(t *testing.T) {
	doc := readRepoFile(t, metricsRegistryDoc)
	rows := map[string][]string{}
	for _, line := range strings.Split(doc, "\n") {
		cells := tableCells(line)
		if strings.HasPrefix(line, "| `interaction.") && len(cells) == 9 {
			rows[strings.Trim(cells[0], "`")] = cells
		}
	}
	for _, d := range FollowingMetrics() {
		cells, ok := rows[string(d.ID)]
		if !ok {
			t.Errorf("%s has no row in %s", d.ID, metricsRegistryDoc)
			continue
		}
		benchmark := "none"
		if d.Benchmark != BenchmarkUnspecified {
			benchmark = "`" + d.Benchmark.String() + "`"
		}
		want := []string{
			"`" + string(d.ID) + "`", "`" + d.Family + "`", "`" + d.Level + "`", "`" + d.Estimator + "`",
			"`" + d.Unit + "`", "`" + d.Visibility.String() + "`", "`provisional`", benchmark,
		}
		for i, w := range want {
			if cells[i] != w {
				t.Errorf("%s column %d is %s in the registry, %s in code", d.ID, i+1, cells[i], w)
			}
		}
		for _, a := range d.Aliases {
			if !strings.Contains(cells[8], "`"+a+"`") {
				t.Errorf("%s alias %s is not documented", d.ID, a)
			}
		}
		delete(rows, string(d.ID))
	}
	for id := range rows {
		t.Errorf("registry row %s has no definition in code", id)
	}
	for _, v := range Visibilities() {
		if !strings.Contains(doc, "`"+v.String()+"`") {
			t.Errorf("visibility %s is not in %s", v, metricsRegistryDoc)
		}
	}
}

func TestLabelVocabularyRegistersEveryToken(t *testing.T) {
	doc := readRepoFile(t, labelVocabularyDoc)
	start := strings.Index(doc, "## Behaviour analytics vocabularies")
	if start < 0 {
		t.Fatalf("%s has no behaviour vocabulary section", labelVocabularyDoc)
	}
	section := doc[start:]
	var tokens []fmt.Stringer
	for _, r := range SuppressionReasons() {
		tokens = append(tokens, r)
	}
	for _, s := range SupportStates() {
		tokens = append(tokens, s)
	}
	for _, s := range EstimateStages() {
		tokens = append(tokens, s)
	}
	for _, s := range EstimationStates() {
		tokens = append(tokens, s)
	}
	for _, s := range EndpointSources() {
		tokens = append(tokens, s)
	}
	for _, m := range MotionClasses() {
		tokens = append(tokens, m)
	}
	for _, r := range ReferencePoints() {
		tokens = append(tokens, r)
	}
	for _, p := range BeliefProvenances() {
		tokens = append(tokens, p)
	}
	for _, e := range PathExtremities() {
		tokens = append(tokens, e)
	}
	for _, k := range UncertaintyKinds() {
		tokens = append(tokens, k)
	}
	for _, m := range PropagationMethods() {
		tokens = append(tokens, m)
	}
	for _, k := range BenchmarkKinds() {
		tokens = append(tokens, k)
	}
	for _, c := range PathConditions() {
		tokens = append(tokens, c)
	}
	for _, d := range CandidateDispositions() {
		tokens = append(tokens, d)
	}
	for _, tok := range tokens {
		if !strings.Contains(section, "`"+tok.String()+"`") {
			t.Errorf("%T token %s is not registered in %s", tok, tok, labelVocabularyDoc)
		}
	}

	// Suppression reasons and path conditions are documented in precedence
	// order, one row each.
	var ordered []fmt.Stringer
	for _, r := range SuppressionReasons() {
		ordered = append(ordered, r)
	}
	for _, c := range PathConditions() {
		ordered = append(ordered, c)
	}
	last := -1
	for _, tok := range ordered {
		i := strings.Index(section, "| `"+tok.String()+"`")
		if i < 0 {
			t.Errorf("%T %s has no table row", tok, tok)
			continue
		}
		if i < last {
			t.Errorf("%T %s is out of precedence order in %s", tok, tok, labelVocabularyDoc)
		}
		last = i
	}
}
