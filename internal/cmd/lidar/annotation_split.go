//go:build pcap
// +build pcap

package lidar

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/version"
)

const annotationSplitUsage = `Usage:
  velocity lidar annotation-split freeze --draft FILE --author NAME --output FILE [flags]
  velocity lidar annotation-split verify --split FILE --pack DIR [--pack DIR ...]

freeze builds a reviewed, object-disjoint split over one or more annotation
packs from a draft, and writes it once. It refuses unless every object it
partitions has completed membership review, and it pins each pack's digest,
manifest, selection record and annotation revision, with who froze it, when
and with which build. A pack with physical references is pinned at the
revision the draft names (physical_revision) or its head, with its review
and coverage summaries; freezing refuses a document that does not hold
against the pinned annotation revision. Geometry review is recorded beside
membership review.

verify re-checks a frozen split's digest and every pin against its packs, as
the per-frame evaluator does before scoring. A facet-proposal pin, which no
evaluator reads, is checked here against the pinned membership.

Run 'velocity lidar annotation-split <freeze|verify> -h' for flags.`

// AnnotationSplitMain freezes and verifies reviewed splits. args is the
// argument slice after the command word. It returns the exit code.
func AnnotationSplitMain(args []string) int {
	return annotationSplitMain(args, os.Stdout, os.Stderr, time.Now)
}

func annotationSplitMain(args []string, stdout, stderr io.Writer, now func() time.Time) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, annotationSplitUsage)
		return 2
	}
	switch args[0] {
	case "freeze":
		return annotationSplitFreeze(args[1:], stdout, stderr, now)
	case "verify":
		return annotationSplitVerify(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprintln(stdout, annotationSplitUsage)
		return 0
	}
	fmt.Fprintf(stderr, "unknown annotation-split command %q\n\n%s\n", args[0], annotationSplitUsage)
	return 2
}

func annotationSplitFreeze(args []string, stdout, stderr io.Writer, now func() time.Time) int {
	fs := flag.NewFlagSet("velocity-lidar-annotation-split-freeze", flag.ContinueOnError)
	fs.SetOutput(stderr)
	draftPath := fs.String("draft", "", "Split draft JSON (required); pack paths in it are relative to it")
	author := fs.String("author", "", "Who is freezing the split (required)")
	output := fs.String("output", "", "Frozen split JSON to write (required; must not exist)")
	supersedes := fs.String("supersedes", "", "Frozen split this revision replaces")
	guard := fs.Float64("guard-seconds", annotation.DefaultSplitGuardSeconds,
		"Packs of one capture closer than this may hold one object, so they must share a partition")
	forConfig := fs.String("for-config-hash", "", "Config hash the split is frozen to judge (recorded)")
	forParams := fs.String("for-params-hash", "", "Parameter hash the split is frozen to judge (recorded)")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: velocity lidar annotation-split freeze --draft FILE --author NAME --output FILE [flags]\n\nOptions:\n")
		fs.PrintDefaults()
	}
	if code, ok := parseSplitFlags(fs, args, stderr); !ok {
		return code
	}
	for _, required := range []struct{ name, value string }{
		{"--draft", *draftPath}, {"--author", *author}, {"--output", *output},
	} {
		if strings.TrimSpace(required.value) == "" {
			fmt.Fprintf(stderr, "error: %s is required\n", required.name)
			fs.Usage()
			return 2
		}
	}

	draft, err := annotation.LoadSplitDraft(*draftPath)
	if err != nil {
		fmt.Fprintf(stderr, "annotation-split freeze: %v\n", err)
		return 1
	}
	var prev *annotation.FrozenSplit
	if *supersedes != "" {
		if prev, err = annotation.LoadFrozenSplit(*supersedes); err != nil {
			fmt.Fprintf(stderr, "annotation-split freeze: supersedes: %v\n", err)
			return 1
		}
	}
	f, err := annotation.FreezeSplit(annotation.FreezeOptions{
		Draft: draft, BaseDir: filepath.Dir(*draftPath), Author: *author, Now: now(),
		BuildVersion: version.Version, BuildGitSHA: version.GitSHA,
		ForConfigHash: *forConfig, ForParamsHash: *forParams, GuardSeconds: *guard, Supersedes: prev,
	})
	if err != nil {
		fmt.Fprintf(stderr, "annotation-split freeze: %v\n", err)
		return 1
	}
	if err := annotation.WriteFrozenSplit(*output, f); err != nil {
		fmt.Fprintf(stderr, "annotation-split freeze: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "frozen split: %s\n", *output)
	writeSplitSummary(stdout, f)
	return 0
}

func annotationSplitVerify(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("velocity-lidar-annotation-split-verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	splitPath := fs.String("split", "", "Frozen split JSON (required)")
	var packs multiFlag
	fs.Var(&packs, "pack", "Annotation pack directory; repeat for every pack the split names")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: velocity lidar annotation-split verify --split FILE --pack DIR [--pack DIR ...]\n\nOptions:\n")
		fs.PrintDefaults()
	}
	if code, ok := parseSplitFlags(fs, args, stderr); !ok {
		return code
	}
	if *splitPath == "" {
		fmt.Fprintln(stderr, "error: --split is required")
		fs.Usage()
		return 2
	}
	f, err := annotation.LoadFrozenSplit(*splitPath)
	if err != nil {
		fmt.Fprintf(stderr, "annotation-split verify: %v\n", err)
		return 1
	}
	writeSplitSummary(stdout, f)
	failed := false
	bound := map[string]bool{}
	for _, dir := range packs {
		p, err := annotation.OpenPack(dir)
		if err != nil {
			fmt.Fprintf(stderr, "pack %s: %v\n", dir, err)
			failed = true
			continue
		}
		view, s, err := f.Bind(p)
		if err != nil {
			fmt.Fprintf(stderr, "pack %s: %v\n", dir, err)
			failed = true
			continue
		}
		// No evaluator scores facet proposals, so none binds their pin; this
		// is where an edited or missing facet revision is caught.
		facets, err := f.BindFeatures(p, s)
		if err != nil {
			fmt.Fprintf(stderr, "pack %s: %v\n", dir, err)
			failed = true
			continue
		}
		bound[p.Manifest.PackDigest] = true
		fmt.Fprintf(stdout, "pack %s: every pin holds at annotation revision %d (%d episode(s))\n",
			p.Manifest.PackDigest, s.Revision, len(view.Episodes))
		if facets != nil {
			fmt.Fprintf(stdout, "  facet revision %d holds against that membership: %d candidate(s), proposals, not truth\n",
				facets.Revision, len(facets.Features))
		}
		// A newer revision is not a failure: the split scores the one it
		// pinned. Saying so keeps the difference from being a surprise.
		if head, err := annotation.LoadSidecar(p); err == nil && head.Revision > s.Revision {
			fmt.Fprintf(stdout, "  annotation revision %d exists; this split scores revision %d until a new split revision is frozen\n",
				head.Revision, s.Revision)
		}
	}
	for _, fp := range f.Packs {
		if !bound[fp.PackDigest] {
			fmt.Fprintf(stderr, "pack %s (dataset %s) was not verified: pass its directory with --pack\n", fp.PackDigest, fp.DatasetID)
			failed = true
		}
	}
	if failed {
		return 1
	}
	fmt.Fprintf(stdout, "verified: %d pack(s), %d case role(s)\n", len(f.Packs), len(f.Cases))
	return 0
}

// parseSplitFlags parses a subcommand's flags. ok is false when the caller
// should return code: 0 for -h, 2 for a usage error.
func parseSplitFlags(fs *flag.FlagSet, args []string, stderr io.Writer) (code int, ok bool) {
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0, false
		}
		return 2, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "error: unexpected arguments: %v\n", fs.Args())
		fs.Usage()
		return 2, false
	}
	return 0, true
}

// writeSplitSummary prints what a frozen split holds: its identity, the
// cases' roles, and per pack the objects in each partition, the episodes,
// the geometry review, which is not membership review, and the physical
// revision pinned, with what it reviews and can score.
func writeSplitSummary(w io.Writer, f *annotation.FrozenSplit) {
	fmt.Fprintf(w, "split %s, revision %d", f.SplitDigest, f.Revision)
	if f.Supersedes != "" {
		fmt.Fprintf(w, ", supersedes %s", f.Supersedes)
	}
	fmt.Fprintf(w, "\nfrozen by %s at %s, build %s (%s)\n", f.Frozen.Author, f.Frozen.FrozenUTC, f.Frozen.BuildVersion, f.Frozen.BuildGitSHA)
	for _, c := range f.Cases {
		names := make([]string, len(c.Captures))
		for i, cp := range c.Captures {
			names[i] = cp.Basename
		}
		fmt.Fprintf(w, "case %s: %s, captures %s\n", c.CaseID, c.Role, strings.Join(names, ", "))
	}
	fmt.Fprintf(w, "tuned in this lineage: %d pack(s), %d case(s)\n", len(f.Tuned.Spans), len(f.Tuned.Cases))
	for _, p := range f.Packs {
		partitions := map[string]int{}
		geometry := map[string]int{}
		for _, o := range p.Objects {
			partitions[o.Partition]++
			geometry[string(o.Geometry.Status)]++
		}
		fmt.Fprintf(w, "pack %s (%s) at annotation revision %d: objects %s; %d episode(s); geometry review %s; %s\n",
			p.PackDigest, p.DatasetID, p.SidecarRevision, counts(partitions), len(p.Episodes), counts(geometry), describePhysicalPin(p.Physical))
	}
}

// describePhysicalPin is one pack's physical pin in a line: the revision and
// its exact-byte digest, the bodies and keyframes reviewed, and per component
// how many reviewed independent keyframes can be scored.
func describePhysicalPin(pin *annotation.FrozenPhysical) string {
	if pin == nil {
		return "no physical references"
	}
	var bodies, keyframes, reviewed int
	for _, o := range pin.Objects {
		if o.Body.Status == annotation.PhysicalBodyReviewed && o.Body.Independent {
			bodies++
		}
		keyframes += o.Keyframes.Total
		reviewed += o.Keyframes.Reviewed
	}
	parts := make([]string, 0, 7)
	for _, c := range pin.Coverage.Components() {
		parts = append(parts, fmt.Sprintf("%s %d/%d", c.Name, c.Coverage.Scorable, c.Coverage.Scorable+c.Coverage.Unavailable))
	}
	return fmt.Sprintf("physical revision %d (%s): %d object(s), %d reviewed independent bod(ies), %d of %d keyframe(s) reviewed; scorable %s",
		pin.Revision, pin.SHA256, len(pin.Objects), bodies, reviewed, keyframes, strings.Join(parts, ", "))
}

func counts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s %d", k, m[k])
	}
	return strings.Join(parts, ", ")
}
