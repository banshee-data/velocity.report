// Command lidar-annotation-split-draft drafts a split manifest from a reviewed
// annotation pack, so a labelled window can be scored without writing the
// manifest by hand.
//
// The per-frame evaluator and the near-face scorer read a split manifest: which
// objects form a partition and which frames are scored as an episode. For a
// pack that was labelled whole, the useful draft is every object that would
// survive freezing, scored from the sample where a replay's estimates start:
//
//	lidar-annotation-split-draft -pack PACK -from-evidence control/evidence/kirk0.db -output splits/kirk0-tuning.json
//
// It lists the objects it took and the ones it left out, with the reason: not a
// road user, not reviewed, a mask still proposed or of unstated completeness,
// or nothing scored in the window. It pins the annotation revision it read, so
// every result names the labels it used. The manifest it writes is a version 1
// manifest, enough for `lidar-ground-truth-eval perframe` and
// `lidar-near-face-eval`; to freeze it, add -freeze-draft with the corpus case
// and the capture's SHA-256, then run `velocity lidar annotation-split freeze`.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// listFlag collects a repeated string flag.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

var sha256Hex = regexp.MustCompile(`^(sha256:)?[0-9a-f]{64}$`)

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lidar-annotation-split-draft", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pack := fs.String("pack", "", "annotation pack directory (required)")
	output := fs.String("output", "", "where to write the version 1 manifest (required; never overwritten)")
	name := fs.String("split", "tuning", "name of the one partition drafted")
	role := fs.String("role", string(annotation.SplitRoleTuning), "its role: tuning or held_out")
	episode := fs.String("episode", "", "episode id (default: <split>-from-<first sample>)")
	fromSample := fs.Int("from-sample", 0, "first scored sample")
	fromSeconds := fs.Float64("from-seconds", 0, "first scored sample, as seconds after the pack's first; not with -from-sample or -from-evidence")
	fromEvidence := fs.String("from-evidence", "", "first scored sample: the first at or after the first solid-body row of this evidence database, where a replay's estimates start; not with -from-sample or -from-seconds")
	toSample := fs.Int("to-sample", 0, "last scored sample (default: the pack's last)")
	note := fs.String("note", "", "free text kept in the manifest")
	freezeDraft := fs.String("freeze-draft", "", "also write a draft for `velocity lidar annotation-split freeze` here; needs -case")
	caseID := fs.String("case", "", "corpus case id the pack was cut from, for the freeze draft")
	caseRole := fs.String("case-role", "", "the case's role: tuning, held_out or screen (default: -role)")
	var captures listFlag
	fs.Var(&captures, "capture", "a capture of the case as basename=sha256 (repeatable; the freeze draft needs one per capture)")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lidar-annotation-split-draft -pack DIR -output FILE [-from-seconds S | -from-sample N] [flags]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	switch {
	case fs.NArg() > 0:
		return usage(fs, stderr, fmt.Errorf("unexpected arguments: %v", fs.Args()))
	case *pack == "" || *output == "":
		return usage(fs, stderr, fmt.Errorf("-pack and -output are required"))
	case (*fromSample != 0 && *fromSeconds != 0) || (*fromEvidence != "" && (*fromSample != 0 || *fromSeconds != 0)):
		return usage(fs, stderr, fmt.Errorf("-from-sample, -from-seconds and -from-evidence each set the first sample: use one"))
	case *freezeDraft != "" && *caseID == "":
		return usage(fs, stderr, fmt.Errorf("-freeze-draft needs -case: a frozen split binds a role to a corpus case"))
	case *freezeDraft == "" && (*caseID != "" || *caseRole != "" || len(captures) > 0):
		return usage(fs, stderr, fmt.Errorf("-case, -case-role and -capture are for -freeze-draft"))
	}
	if _, err := os.Lstat(*output); err == nil {
		return usage(fs, stderr, fmt.Errorf("%s exists: a manifest is never overwritten", *output))
	}
	var caps []annotation.CaseCapture
	for _, c := range captures {
		base, sum, ok := strings.Cut(c, "=")
		if !ok || base == "" || !sha256Hex.MatchString(sum) {
			return usage(fs, stderr, fmt.Errorf("-capture %q: want basename=sha256 with a 64-digit hex digest", c))
		}
		caps = append(caps, annotation.CaseCapture{Basename: base, SHA256: "sha256:" + strings.TrimPrefix(sum, "sha256:")})
	}

	p, err := annotation.OpenPack(*pack)
	if err != nil {
		fmt.Fprintf(stderr, "error: open pack: %v\n", err)
		return 1
	}
	sc, err := annotation.LoadSidecar(p)
	if err != nil {
		fmt.Fprintf(stderr, "error: load annotation: %v\n", err)
		return 1
	}
	first := *fromSample
	switch {
	case *fromSeconds != 0:
		first, err = annotation.FirstSampleAtOrAfter(p, *fromSeconds)
	case *fromEvidence != "":
		var ns int64
		if ns, err = perframeeval.FirstBodyNs(perframeeval.ArmSpec{Label: "evidence", DBPath: *fromEvidence, Stage: perframeeval.StageOnline, DeclaredBaseline: true}); err == nil {
			first, err = annotation.FirstSampleAtOrAfterNs(p, ns)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	manifest, report, err := annotation.DraftSplitManifest(p, sc, annotation.DraftSplitOptions{
		SplitName: *name, Role: annotation.SplitRole(*role), EpisodeID: *episode,
		FirstSample: first, LastSample: *toSample, Policy: annotation.DefaultReferencePolicy(), Note: *note,
	})
	if report != nil {
		printReport(stdout, p, report)
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if err := writeJSON(*output, manifest); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s (manifest digest pins annotation revision %d)\n", *output, manifest.SidecarRevision)

	if *freezeDraft != "" {
		if _, err := os.Lstat(*freezeDraft); err == nil {
			fmt.Fprintf(stderr, "error: %s exists: a draft is never overwritten\n", *freezeDraft)
			return 1
		}
		absPack, _ := filepath.Abs(*pack)
		absOut, _ := filepath.Abs(*output)
		cr := annotation.SplitRole(*caseRole)
		if cr == "" {
			cr = annotation.SplitRole(*role)
		}
		draft := annotation.SplitDraft{
			Schema: annotation.SplitDraftSchema, SchemaVersion: annotation.SplitDraftSchemaVersion,
			Note:  *note,
			Cases: []annotation.SplitCase{{CaseID: *caseID, Role: cr, Captures: caps}},
			Packs: []annotation.DraftPack{{Dir: absPack, CaseID: *caseID, SplitManifest: absOut}},
		}
		if err := writeJSON(*freezeDraft, draft); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "wrote %s: freeze it with `velocity lidar annotation-split freeze --draft %s --author NAME --output FILE`\n", *freezeDraft, *freezeDraft)
		if len(caps) == 0 {
			fmt.Fprintln(stderr, "warning: no -capture given: freezing refuses a case with no capture, and a role binds to the capture's SHA-256")
		}
	}
	return 0
}

func printReport(w io.Writer, p *annotation.Pack, r *annotation.DraftSplitReport) {
	fmt.Fprintf(w, "annotation revision %d, samples %d to %d of %d\n", r.SidecarRevision, r.FirstSample, r.LastSample, len(p.Samples))
	fmt.Fprintf(w, "included %d objects, %d scored masks:\n", len(r.Included), r.ScoredMasks)
	for _, o := range r.Included {
		fmt.Fprintf(w, "  %s (%s): %d masks, samples %d to %d\n", o.ObjectID, o.Class, o.ScoredMasks, o.FirstSample, o.LastSample)
	}
	if len(r.Skipped) > 0 {
		fmt.Fprintf(w, "left out %d objects:\n", len(r.Skipped))
		for _, s := range r.Skipped {
			fmt.Fprintf(w, "  %s (%s): %s\n", s.ObjectID, s.Class, s.Reason)
		}
	}
}

// writeJSON writes a new file, never replacing one.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	return f.Close()
}

func usage(fs *flag.FlagSet, w io.Writer, err error) int {
	fmt.Fprintf(w, "error: %v\n", err)
	fs.Usage()
	return 2
}
