package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/experiments"
)

// now is the clock, replaced in tests.
var now = time.Now

// parse reads a subcommand's flags wherever they fall among its arguments,
// so that a lane can write `mark l3bg0008 replay/a0/x done -out DIR`.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func spec(set *experiments.Set, slug string) *experiments.Spec {
	sp := set.Get(slug)
	if sp == nil {
		fmt.Fprintf(os.Stderr, "no experiment %s\n", slug)
	}
	return sp
}

// units prints a step's units, one per line and tab-separated: the unit id,
// its arm, its case, the arm's tuning file and its replay options (comma
// separated), "-" for none. A lane reads the arm's configuration from here
// rather than restating it. With -todo only the units not yet done.
func units(set *experiments.Set, ledger *experiments.Ledger, args []string) int {
	fs := flag.NewFlagSet("units", flag.ContinueOnError)
	step := fs.String("step", "", "only this step's units")
	todo := fs.Bool("todo", false, "only units not done")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: experiments units <slug> [-step id] [-todo]")
		return 2
	}
	sp := spec(set, pos[0])
	if sp == nil {
		return 1
	}
	recs, err := ledger.Records(sp.ID)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	found := *step == ""
	for _, st := range sp.Steps {
		if *step != "" && st.ID != *step {
			continue
		}
		found = true
		for _, u := range sp.Units(st) {
			if *todo && recs[u.ID()].State == experiments.UnitDone {
				continue
			}
			tuning, replay := "", ""
			for _, a := range sp.Arms {
				if a.Name == u.Arm {
					tuning, replay = a.Tuning, strings.Join(a.Replay, ",")
				}
			}
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n", u.ID(), dash(u.Arm), dash(u.Case), dash(tuning), dash(replay))
		}
	}
	if !found {
		fmt.Fprintf(os.Stderr, "%s has no step %q\n", sp.ID, *step)
		return 1
	}
	return 0
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// mark records that a unit started, finished or failed.
func mark(set *experiments.Set, root string, ledger *experiments.Ledger, args []string) int {
	host, _ := os.Hostname()
	host, _, _ = strings.Cut(host, ".")
	fs := flag.NewFlagSet("mark", flag.ContinueOnError)
	worker := fs.String("worker", host, "this worker's id")
	class := fs.String("class", "", "this worker's class (m1pro, swan, pi4, person)")
	reclaim := fs.Bool("reclaim", false, "this worker reclaims orphans")
	force := fs.Bool("force", false, "an operator's override of the queue's refusals")
	out := fs.String("out", "", "where the unit's output is")
	artefact := fs.String("artefact", "", "what a manual unit closes on")
	note := fs.String("note", "", "a note kept with the unit")
	wall := fs.Duration("wall", 0, "wall time of a unit marked done without a start")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 3 {
		fmt.Fprintln(os.Stderr, "usage: experiments mark <slug> <unit> start|done|failed [flags]")
		return 2
	}
	sp := spec(set, pos[0])
	if sp == nil {
		return 1
	}
	if *class != "" {
		if _, ok := experiments.WorkerClasses[*class]; !ok {
			fmt.Fprintf(os.Stderr, "worker class %q is not registered\n", *class)
			return 1
		}
	}
	u, _, err := sp.ParseUnit(pos[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if pos[2] == string(experiments.Start) && !*force && set.Readiness(sp) == experiments.Waiting {
		fmt.Fprintf(os.Stderr, "%s waits for %s to be answered first (-force to start anyway)\n", sp.ID, strings.Join(sp.DependsOn, ", "))
		return 1
	}
	digest, err := experiments.SpecDigest(root, sp)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	rec, err := ledger.Apply(sp, experiments.Mark{
		Unit: u, To: experiments.Transition(pos[2]), Worker: *worker, Class: *class,
		Reclaim: *reclaim, Force: *force, Out: *out, Artefact: *artefact, Note: *note,
		Wall: *wall, SpecDigest: digest, Now: now().UTC(),
	}, experiments.RepoArtefacts(root, sp))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	line := fmt.Sprintf("%s %s %s (attempt %d on %s)", sp.ID, rec.ID(), rec.State, rec.Attempt, rec.Worker)
	if rec.State != experiments.UnitRunning && rec.WallSeconds > 0 {
		line += ", took " + experiments.FormatDuration(rec.Wall().Round(time.Second))
	}
	fmt.Println(line)
	return 0
}

// status prints a line per experiment that is approved or has progress, or
// one experiment's steps in detail.
func status(set *experiments.Set, ledger *experiments.Ledger, args []string) int {
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "usage: experiments status [slug]")
		return 2
	}
	t := now().UTC()
	if len(args) == 1 {
		sp := spec(set, args[0])
		if sp == nil {
			return 1
		}
		recs, err := ledger.Records(sp.ID)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		detail(sp, recs, t)
		return 0
	}
	for _, sp := range set.Specs {
		recs, err := ledger.Records(sp.ID)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if len(recs) == 0 && sp.Status != experiments.StatusApproved {
			continue
		}
		p := sp.Progress(recs, t)
		fmt.Printf("%-12s %-9s %3d/%-3d units  %s  %s\n", sp.ID, sp.Status, p.Done, p.Units, left(p), sp.Title)
	}
	return 0
}

// left says what an experiment has left: machine time, rescaled by how its
// done units ran against their estimate, and person time.
func left(p experiments.Progress) string {
	s := experiments.FormatDuration(p.MachineLeft) + " machine"
	if r := p.Ratio(); r > 0 && p.MachineLeft > 0 {
		s += fmt.Sprintf(" (~%s at %.1fx)", experiments.FormatDuration(time.Duration(float64(p.MachineLeft)*r).Round(time.Minute)), r)
	}
	if p.HumanLeft > 0 {
		s += " + " + experiments.FormatDuration(p.HumanLeft) + " person"
	}
	return s + " left"
}

func detail(sp *experiments.Spec, recs map[string]experiments.UnitRecord, t time.Time) {
	p := sp.Progress(recs, t)
	fmt.Printf("%s  %s\n  %s; %d of %d units done, %s\n", sp.ID, sp.Title, sp.Status, p.Done, p.Units, left(p))
	for _, s := range p.Steps {
		line := fmt.Sprintf("  %-12s %-26s %-17s %d/%d", s.Step.ID, s.Step.Kind, s.State(), s.Done, s.Units)
		if s.Running > 0 {
			line += fmt.Sprintf(", %d running", s.Running)
		}
		if s.Stale > 0 {
			line += fmt.Sprintf(" (%d past 3x estimate)", s.Stale)
		}
		if s.Failed > 0 {
			line += fmt.Sprintf(", %d failed", s.Failed)
		}
		if s.Wall > 0 {
			line += ", took " + experiments.FormatDuration(s.Wall.Round(time.Second))
		}
		fmt.Println(line)
		for _, u := range sp.Units(s.Step) {
			r, ok := recs[u.ID()]
			if !ok || r.State == experiments.UnitDone && len(sp.Units(s.Step)) > 1 {
				continue
			}
			switch r.State {
			case experiments.UnitRunning:
				fmt.Printf("      %s running on %s for %s\n", u.ID(), r.Worker, experiments.FormatDuration(t.Sub(r.Started).Round(time.Second)))
			case experiments.UnitFailed:
				fmt.Printf("      %s failed on %s: %s\n", u.ID(), r.Worker, r.Note)
			case experiments.UnitDone:
				if r.Artefact != "" {
					fmt.Printf("      %s closed on %s\n", u.ID(), r.Artefact)
				}
			}
		}
	}
	for _, id := range p.Extra {
		fmt.Printf("  %s has a state file but is not in the spec\n", id)
	}
}
