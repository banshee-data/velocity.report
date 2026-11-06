// Command experiments checks and summarises the experiment specs under
// data/experiments.
//
//	experiments validate              check every spec; exit 1 on any problem
//	experiments graph [-write]        print data/experiments/DEPENDENCIES.md, or write it
//	experiments check-graph           exit 1 if DEPENDENCIES.md is not the fresh render
//	experiments next                  open experiments and what each waits for
//	experiments show <slug>           one spec's steps, fan-out and estimate
//	experiments new <area>            the next free slug in an area
//	experiments pin <slug>... | -open record each arm's tuning fingerprint in open specs
//
// Progress, kept in a state directory outside the checkout (-state, or
// $VELOCITY_EXPERIMENT_STATE, or ~/.local/state/velocity-report/experiments):
//
//	experiments status [slug]                       every experiment with progress, or one in detail
//	experiments units <slug> [-step id] [-todo]     unit, arm, case, tuning, replay options: tab-separated, for a lane
//	experiments mark <slug> <unit> start|done|failed [-out path] [-artefact a] [-note text]
//	                 [-worker host] [-class m1pro] [-reclaim] [-wall 12m] [-force]
//
// A unit is one step on one arm and one case (replay/cmr/lombard-laguna), or
// a whole step that does not fan out (write). A lane marks each unit as it
// starts and finishes it; a person closes a manual unit with its artefact.
//
// Run it from the repository root, or pass -root.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/experiments"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("experiments", flag.ContinueOnError)
	root := fs.String("root", ".", "repository root")
	state := fs.String("state", experiments.DefaultStateDir(), "progress state directory")
	write := fs.Bool("write", false, "graph: write "+experiments.DependenciesFile+" instead of printing it")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: experiments [-root DIR] [-state DIR] validate | graph [-write] | check-graph | next | show <slug> | new <area>\n"+
			"       | pin <slug>... | pin -open\n"+
			"       | status [slug] | units <slug> [-step id] [-todo] | mark <slug> <unit> start|done|failed [flags]")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return 2
	}
	cmd, rest := rest[0], rest[1:]
	if cmd == "graph" {
		// Accept the flag after the subcommand too: experiments graph -write.
		for _, a := range rest {
			if a == "-write" || a == "--write" {
				*write = true
			}
		}
	}

	set, err := experiments.Load(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	switch cmd {
	case "validate":
		errs := set.Validate(*root)
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		if len(errs) > 0 {
			return 1
		}
		fmt.Printf("%d experiments in %d areas: ok\n", len(set.Specs), len(set.Areas))
		return 0
	case "graph":
		doc := set.DependenciesDoc()
		if !*write {
			fmt.Print(doc)
			return 0
		}
		if err := os.WriteFile(filepath.Join(*root, experiments.DependenciesFile), []byte(doc), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("wrote", experiments.DependenciesFile)
		return 0
	case "check-graph":
		have, err := os.ReadFile(filepath.Join(*root, experiments.DependenciesFile))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if string(have) != set.DependenciesDoc() {
			fmt.Fprintf(os.Stderr, "%s is out of date: run make experiments-graph\n", experiments.DependenciesFile)
			return 1
		}
		return 0
	case "next":
		for _, sp := range set.Specs {
			if r := set.Readiness(sp); r != "" {
				m, h := sp.Estimate()
				fmt.Printf("%-12s %-30s %8s machine %8s person  %s\n", sp.ID, r, experiments.FormatDuration(m), experiments.FormatDuration(h), sp.Title)
			}
		}
		return 0
	case "show":
		if len(rest) != 1 {
			fs.Usage()
			return 2
		}
		sp := set.Get(rest[0])
		if sp == nil {
			fmt.Fprintf(os.Stderr, "no experiment %s\n", rest[0])
			return 1
		}
		show(set, sp)
		return 0
	case "new":
		if len(rest) != 1 {
			fs.Usage()
			return 2
		}
		area := rest[0]
		if _, ok := set.Areas[area]; !ok {
			fmt.Fprintf(os.Stderr, "no area %s: create %s/%s/README.md with its header first\n", area, experiments.Dir, area)
			return 1
		}
		max := 0
		for _, sp := range set.Specs {
			if experiments.AreaOf(sp.ID) == area {
				var n int
				fmt.Sscanf(sp.ID[len(area):], "%d", &n)
				if n > max {
					max = n
				}
			}
		}
		next := fmt.Sprintf("%s%04d", area, max+1)
		fmt.Println(next)
		fmt.Fprintf(os.Stderr, "write %s/%s/%s.md, then pin its arms: experiments pin %s\n", experiments.Dir, area, next, next)
		return 0
	case "pin":
		return pin(set, *root, rest)
	case "status":
		return status(set, experiments.OpenLedger(*state), rest)
	case "units":
		return units(set, experiments.OpenLedger(*state), rest)
	case "mark":
		return mark(set, *root, experiments.OpenLedger(*state), rest)
	}
	fs.Usage()
	return 2
}

// pin records each arm's current tuning fingerprint in the named open specs,
// or in every open spec with -open. Pinning is how a spec is written and how
// it is restated after a file changes under it; the commit that re-pins is the
// restatement Git keeps.
func pin(set *experiments.Set, root string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: experiments pin <slug>... | pin -open")
		return 2
	}
	var specs []*experiments.Spec
	if len(args) == 1 && (args[0] == "-open" || args[0] == "--open") {
		for _, sp := range set.Specs {
			if sp.Open() && len(sp.Arms) > 0 {
				specs = append(specs, sp)
			}
		}
	} else {
		for _, id := range args {
			sp := spec(set, id)
			if sp == nil {
				return 1
			}
			specs = append(specs, sp)
		}
	}
	code := 0
	for _, sp := range specs {
		changed, err := experiments.Pin(root, sp)
		switch {
		case err != nil:
			fmt.Fprintln(os.Stderr, err)
			code = 1
		case changed:
			fmt.Printf("%s pinned\n", sp.ID)
		default:
			fmt.Printf("%s already pinned\n", sp.ID)
		}
	}
	return code
}

func show(set *experiments.Set, sp *experiments.Spec) {
	m, h := sp.Estimate()
	fmt.Printf("%s  %s\n  status %s, owner %s, created %s\n", sp.ID, sp.Title, sp.Status, sp.Owner, sp.Created)
	if r := set.Readiness(sp); r != "" {
		fmt.Printf("  %s\n", r)
	}
	fmt.Printf("  asks: %s\n", strings.Join(sp.Asks, "; "))
	if len(sp.DependsOn) > 0 {
		fmt.Printf("  depends on: %s\n", strings.Join(sp.DependsOn, ", "))
	}
	for _, a := range sp.Arms {
		fp := a.Fingerprint
		if fp == "" {
			fp = "unpinned"
		}
		fmt.Printf("  arm %-10s %s @%s %s\n", a.Name, a.Tuning, fp, strings.Join(a.Replay, ","))
	}
	if sp.Cases != nil {
		fmt.Printf("  cases: %d of %s\n", sp.Cases.Count, sp.Cases.Corpus)
	}
	for _, st := range sp.Steps {
		cost := experiments.FormatDuration(sp.StepMachine(st))
		if st.Estimate.Human > 0 {
			cost = experiments.FormatDuration(time.Duration(st.Estimate.Human)) + " person"
		}
		fmt.Printf("  step %-14s %-26s %s\n", st.ID, st.Kind, cost)
	}
	fmt.Printf("  estimate: %s machine, %s person\n", experiments.FormatDuration(m), experiments.FormatDuration(h))
	if sp.Report != "" {
		fmt.Printf("  report: %s\n", sp.Report)
	}
}
