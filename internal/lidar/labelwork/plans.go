package labelwork

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/experiments"
)

// PlanStep is one step of a labelling plan, as its spec states it. It is
// planned work: a spec says what is to be done, never that it was.
type PlanStep struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Human string `json:"human_estimate,omitempty"`
	Note  string `json:"note,omitempty"`
}

// Plan is one labelling experiment spec.
type Plan struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	DependsOn []string   `json:"depends_on"`
	Requires  []string   `json:"requires"`
	Path      string     `json:"path"`
	Steps     []PlanStep `json:"steps"`
	Error     string     `json:"error,omitempty"`
}

// Plans are the labelling specs found, or why none could be read.
type Plans struct {
	Available bool   `json:"available"`
	Dir       string `json:"dir"`
	Error     string `json:"error,omitempty"`
	Plans     []Plan `json:"plans"`
}

// ReadPlans reads data/experiments/label/*.md beneath root.
func ReadPlans(root string) Plans {
	dir := filepath.Join(root, "data", "experiments", "label")
	out := Plans{Dir: filepath.ToSlash(filepath.Join("data", "experiments", "label")), Plans: []Plan{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		out.Error = "the labelling plans could not be read: " + err.Error()
		return out
	}
	out.Available = true
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && e.Name() != "README.md" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		rel := filepath.ToSlash(filepath.Join("data", "experiments", "label", name))
		spec, err := experiments.ReadFile(filepath.Join(dir, name))
		if err != nil {
			out.Plans = append(out.Plans, Plan{ID: strings.TrimSuffix(name, ".md"), Path: rel, Error: err.Error(), Steps: []PlanStep{}})
			continue
		}
		p := Plan{ID: spec.ID, Title: spec.Title, Status: string(spec.Status), DependsOn: append([]string{}, spec.DependsOn...),
			Requires: append([]string{}, spec.Requires.Capabilities...), Path: rel, Steps: []PlanStep{}}
		for _, st := range spec.Steps {
			step := PlanStep{ID: st.ID, Kind: st.Kind, Note: st.Note}
			if st.Estimate.Human > 0 {
				step.Human = st.Estimate.Human.String()
			}
			p.Steps = append(p.Steps, step)
		}
		out.Plans = append(out.Plans, p)
	}
	return out
}
