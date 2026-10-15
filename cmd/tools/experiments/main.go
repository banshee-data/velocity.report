// Command experiments checks and summarises the experiment specs under
// data/experiments.
//
//	experiments validate              check every spec; exit 1 on any problem
//	experiments graph [-write]        print data/experiments/DEPENDENCIES.md, or write it
//	experiments check-graph           exit 1 if DEPENDENCIES.md is not the fresh render
//	experiments next                  open experiments and what each waits for
//	experiments show <slug>           one spec's steps, fan-out and estimate
//	experiments new <area>            the next free slug in an area
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
	write := fs.Bool("write", false, "graph: write "+experiments.DependenciesFile+" instead of printing it")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: experiments [-root DIR] validate | graph [-write] | check-graph | next | show <slug> | new <area>")
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
		fmt.Printf("%s%04d\n", area, max+1)
		return 0
	}
	fs.Usage()
	return 2
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
		fmt.Printf("  arm %-10s %s %s\n", a.Name, a.Tuning, strings.Join(a.Replay, ","))
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
