package experiments

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

// DependenciesFile is the generated overview at the root of Dir.
const DependenciesFile = Dir + "/DEPENDENCIES.md"

// fanout is how many units a step runs over.
func (spec *Spec) fanout(st Step) int {
	n := 1
	for _, o := range st.Over {
		switch o {
		case "arms":
			if len(spec.Arms) > 0 {
				n *= len(spec.Arms)
			}
		case "cases":
			if spec.Cases != nil && spec.Cases.Count > 0 {
				n *= spec.Cases.Count
			}
		}
	}
	return n
}

// StepMachine is a step's estimated machine time.
func (spec *Spec) StepMachine(st Step) time.Duration {
	if st.Estimate.Total > 0 {
		return time.Duration(st.Estimate.Total)
	}
	return time.Duration(st.Estimate.Each) * time.Duration(spec.fanout(st))
}

// predecessors resolves a step's After list: empty means the step before.
func (spec *Spec) predecessors(i int) []int {
	st := spec.Steps[i]
	if len(st.After) == 0 {
		if i == 0 {
			return nil
		}
		return []int{i - 1}
	}
	idx := map[string]int{}
	for j, s := range spec.Steps {
		idx[s.ID] = j
	}
	var out []int
	for _, a := range st.After {
		if j, ok := idx[a]; ok && j < i {
			out = append(out, j)
		}
	}
	return out
}

// Estimate is the experiment's machine time along its longest chain of
// steps, and the person time of its manual steps, summed.
func (spec *Spec) Estimate() (machine, human time.Duration) {
	finish := make([]time.Duration, len(spec.Steps))
	for i, st := range spec.Steps {
		var start time.Duration
		for _, j := range spec.predecessors(i) {
			if finish[j] > start {
				start = finish[j]
			}
		}
		finish[i] = start + spec.StepMachine(st)
		if finish[i] > machine {
			machine = finish[i]
		}
		human += time.Duration(st.Estimate.Human)
	}
	return machine, human
}

// kindsOf lists the experiment's step kinds in one class, without repeats,
// in step order, with any manual: prefix trimmed.
func (spec *Spec) kindsOf(class KindClass) []string {
	seen := map[string]bool{}
	var out []string
	for _, st := range spec.Steps {
		if ClassifyKind(st.Kind) == class && !seen[st.Kind] {
			seen[st.Kind] = true
			out = append(out, strings.TrimPrefix(st.Kind, "manual:"))
		}
	}
	return out
}

// Readiness is what stands between an approved experiment and a worker.
type Readiness string

const (
	// ReadyForWorker: dependencies answered, first step a job kind.
	ReadyForWorker Readiness = "ready for a worker"
	// ReadyForPerson: dependencies answered, first step manual.
	ReadyForPerson Readiness = "ready, a person goes first"
	// NeedsTooling: dependencies answered, but a step's kind has no executor.
	NeedsTooling Readiness = "ready, but a step needs tooling"
	// Waiting: a dependency is not answered.
	Waiting Readiness = "waiting on a dependency"
	// NotApproved: proposed, so not schedulable.
	NotApproved Readiness = "proposed, awaiting approval"
)

// Readiness reports what an open experiment waits for. Answered and
// retired experiments return "".
func (s *Set) Readiness(spec *Spec) Readiness {
	switch spec.Status {
	case StatusAnswered, StatusRetired:
		return ""
	}
	for _, d := range spec.DependsOn {
		if dep := s.Get(d); dep == nil || dep.Status != StatusAnswered {
			return Waiting
		}
	}
	if spec.Status == StatusProposed {
		return NotApproved
	}
	if len(spec.kindsOf(KindPlanned)) > 0 {
		return NeedsTooling
	}
	if len(spec.Steps) > 0 && ClassifyKind(spec.Steps[0].Kind) == KindManual {
		return ReadyForPerson
	}
	return ReadyForWorker
}

// CriticalPath is the longest chain of open (proposed or approved)
// experiments by estimated elapsed time (machine plus person: a manual step
// holds the chain as surely as a replay does), following depends_on, and
// its total.
func (s *Set) CriticalPath() ([]string, time.Duration) {
	open := func(sp *Spec) bool { return sp.Status == StatusProposed || sp.Status == StatusApproved }
	memo := map[string]time.Duration{}
	next := map[string]string{}
	var longest func(sp *Spec) time.Duration
	// Longest chain ending at sp, walking back through open dependencies.
	longest = func(sp *Spec) time.Duration {
		if v, ok := memo[sp.ID]; ok {
			return v
		}
		memo[sp.ID] = 0 // cycles are reported by Validate; stop here
		machine, human := sp.Estimate()
		own := machine + human
		var best time.Duration
		for _, d := range sp.DependsOn {
			dep := s.Get(d)
			if dep == nil || !open(dep) {
				continue
			}
			if v := longest(dep); v > best || (v == best && next[sp.ID] > d) {
				best = v
				next[sp.ID] = d
			}
		}
		memo[sp.ID] = own + best
		return memo[sp.ID]
	}
	var end string
	var total time.Duration
	for _, sp := range s.Specs {
		if !open(sp) {
			continue
		}
		if v := longest(sp); v > total {
			total, end = v, sp.ID
		}
	}
	var chain []string
	for id := end; id != ""; id = next[id] {
		chain = append([]string{id}, chain...)
	}
	return chain, total
}

// Mermaid renders the dependency graph: one node per experiment grouped by
// area, coloured by status, with an edge for each depends_on. The questions
// are listed under the graph rather than drawn in it: thirteen of them in one
// row made the graph too wide to read.
func (s *Set) Mermaid() string {
	var b strings.Builder
	b.WriteString("flowchart TB\n")
	b.WriteString("  classDef proposed fill:#f4f4f5,stroke:#71717a,stroke-dasharray:4 3,color:#18181b\n")
	b.WriteString("  classDef approved fill:#dbeafe,stroke:#2563eb,color:#172554\n")
	b.WriteString("  classDef answered fill:#dcfce7,stroke:#16a34a,color:#052e16\n")
	b.WriteString("  classDef retired fill:#fafafa,stroke:#d4d4d8,color:#71717a\n")

	areas := make([]string, 0, len(s.Areas))
	for a := range s.Areas {
		areas = append(areas, a)
	}
	sort.Strings(areas)
	for _, a := range areas {
		fmt.Fprintf(&b, "  subgraph area_%s [\"%s: %s\"]\n", a, a, mermaidText(s.Areas[a].Title))
		for _, sp := range s.Specs {
			if AreaOf(sp.ID) != a {
				continue
			}
			fmt.Fprintf(&b, "    %s[\"%s\"]:::%s\n", sp.ID, s.nodeLabel(sp), sp.Status)
		}
		b.WriteString("  end\n")
	}

	for _, sp := range s.Specs {
		for _, d := range sp.DependsOn {
			if s.Get(d) != nil {
				fmt.Fprintf(&b, "  %s --> %s\n", d, sp.ID)
			}
		}
	}
	return b.String()
}

// CostText describes an estimate: machine time, person time, or both.
func CostText(machine, human time.Duration) string {
	switch {
	case machine > 0 && human > 0:
		return FormatDuration(machine) + " machine + " + FormatDuration(human) + " person"
	case human > 0:
		return FormatDuration(human) + " person"
	default:
		return FormatDuration(machine) + " machine"
	}
}

// asksText links each question an experiment asks, relative to Dir.
func (s *Set) asksText(sp *Spec) string {
	up := strings.Repeat("../", strings.Count(Dir, "/")+1)
	parts := make([]string, 0, len(sp.Asks))
	for _, q := range sp.Asks {
		if decisionPattern.MatchString(q) {
			parts = append(parts, fmt.Sprintf("[%s](%sdocs/DECISIONS.md)", q, up))
			continue
		}
		parts = append(parts, fmt.Sprintf("[%s](%s%s)", questionLabel(q), up, q))
	}
	return strings.Join(parts, ", ")
}

func (s *Set) nodeLabel(sp *Spec) string {
	machine, human := sp.Estimate()
	cost := CostText(machine, human)
	if sp.Status == StatusAnswered && sp.Actual != nil && sp.Actual.Wall > 0 {
		cost += " (took " + sp.Actual.Wall.String() + ")"
	}
	lines := []string{"<b>" + sp.ID + "</b>", mermaidText(shorten(sp.Title, 42)), string(sp.Status) + " · " + cost}
	if m := sp.kindsOf(KindManual); len(m) > 0 {
		lines = append(lines, "manual: "+strings.Join(m, ", "))
	}
	if p := sp.kindsOf(KindPlanned); len(p) > 0 && s.Readiness(sp) != "" {
		lines = append(lines, "needs tooling: "+strings.Join(p, ", "))
	}
	return strings.Join(lines, "<br/>")
}

// questionLabel shortens a question reference: a decision id as it is, a
// path to its file name and anchor.
func questionLabel(q string) string {
	if decisionPattern.MatchString(q) {
		return q
	}
	p, anchor, _ := strings.Cut(q, "#")
	name := strings.TrimSuffix(path.Base(p), ".md")
	if anchor == "" {
		return name
	}
	if r := []rune(anchor); len(r) > 32 {
		anchor = string(r[:31]) + "…"
	}
	return name + " #" + anchor
}

// shorten cuts a title for a graph node at a word boundary; the lists below
// the graph give it whole.
func shorten(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	cut := string(r[:max])
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:") + "…"
}

func mermaidText(s string) string {
	r := strings.NewReplacer(`"`, "#quot;", "<", "#lt;", ">", "#gt;")
	return r.Replace(s)
}

// DependenciesDoc renders the generated overview: the graph, then the
// queue as lists. Lists rather than tables, so the Markdown formatter
// leaves the generated text exactly as written and the file can be checked
// against a fresh render.
func (s *Set) DependenciesDoc() string {
	var b strings.Builder
	b.WriteString("# Experiment dependencies\n\n")
	b.WriteString("<!-- Generated by `make experiments-graph` from the specs under data/experiments. Do not edit: change a spec and regenerate. -->\n\n")
	b.WriteString("Every experiment under this folder, by area. An arrow is `depends_on`: the experiment at its head needs the one at its tail answered first. Colour is the spec's status: grey dashed proposed, blue approved, green answered, pale retired. Each node gives the estimated machine time along its longest chain of steps, plus person time for manual steps, and for answered experiments what the run took. How to write a spec: [README](README.md); the design: [experiment queue plan](../../docs/plans/platform-experiment-queue-plan.md).\n\n")
	b.WriteString("```mermaid\n")
	b.WriteString(s.Mermaid())
	b.WriteString("```\n\n")

	b.WriteString("## Open experiments\n\n")
	listed := false
	for _, sp := range s.Specs {
		r := s.Readiness(sp)
		if r == "" {
			continue
		}
		listed = true
		machine, human := sp.Estimate()
		fmt.Fprintf(&b, "- [%s](%s/%s.md) %s: %s; %s", sp.ID, AreaOf(sp.ID), sp.ID, sp.Title, r, CostText(machine, human))
		if len(sp.DependsOn) > 0 {
			fmt.Fprintf(&b, "; after %s", strings.Join(sp.DependsOn, ", "))
		}
		fmt.Fprintf(&b, "; asks %s.\n", s.asksText(sp))
	}
	if !listed {
		b.WriteString("- None.\n")
	}

	b.WriteString("\n## Answered\n\n")
	listed = false
	for _, sp := range s.Specs {
		if sp.Status != StatusAnswered {
			continue
		}
		listed = true
		rel := strings.Repeat("../", strings.Count(Dir, "/")+1) + sp.Report
		fmt.Fprintf(&b, "- [%s](%s/%s.md) %s: [report](%s)", sp.ID, AreaOf(sp.ID), sp.ID, sp.Title, rel)
		if sp.Actual != nil && sp.Actual.Wall > 0 {
			machine, _ := sp.Estimate()
			fmt.Fprintf(&b, "; estimated %s, took %s", FormatDuration(machine), sp.Actual.Wall)
			if sp.Actual.Worker != "" {
				fmt.Fprintf(&b, " on %s", sp.Actual.Worker)
			}
		}
		fmt.Fprintf(&b, "; asked %s.\n", s.asksText(sp))
	}
	if !listed {
		b.WriteString("- None.\n")
	}

	b.WriteString("\n## Totals\n\n")
	var openMachine, openHuman time.Duration
	var nOpen int
	for _, sp := range s.Specs {
		if sp.Status == StatusProposed || sp.Status == StatusApproved {
			m, h := sp.Estimate()
			openMachine += m
			openHuman += h
			nOpen++
		}
	}
	fmt.Fprintf(&b, "- %d open experiments: %s estimated.\n", nOpen, CostText(openMachine, openHuman))
	if chain, total := s.CriticalPath(); len(chain) > 0 {
		fmt.Fprintf(&b, "- Longest chain of open work, machine and person time together: %s, %s.\n", strings.Join(chain, " → "), FormatDuration(total))
	}
	return b.String()
}
