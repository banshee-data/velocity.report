//go:build pcap
// +build pcap

package lidar

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

const annotationReferenceUsage = `Usage: velocity lidar annotation-reference <import|validate|draft-following> --pack DIR [flags]

Import or validate the physical references of an annotation pack: body
dimensions, keyframe poses and following gaps, each with bounds, evidence
status and its own review, stored beside the pack's membership sidecar.

  import           Validate an import file against the pack and its annotation,
                   merge it into the current references and save a new revision
  validate         Check the stored references against the current annotation, or
                   with --file dry-run an import (every import check, no write)
  draft-following  Write an import file recording a follower's leader, no_leader
                   or ambiguous decision over an interval, with gaps derived from
                   the stored keyframes; it is checked as an import before it is
                   written, and nothing is stored

Run 'velocity lidar annotation-reference <command> -h' for flags.`

// AnnotationReferenceMain imports or validates physical references. args is
// the argument slice after the command word. It returns the exit code.
func AnnotationReferenceMain(args []string) int {
	return annotationReferenceMain(args, os.Stdout, os.Stderr)
}

func annotationReferenceMain(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, annotationReferenceUsage)
		return 2
	}
	switch args[0] {
	case "import":
		return referenceImport(args[1:], stdout, stderr)
	case "validate":
		return referenceValidate(args[1:], stdout, stderr)
	case "draft-following":
		return referenceDraftFollowing(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprintln(stdout, annotationReferenceUsage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown annotation-reference command: %q\n\n%s\n", args[0], annotationReferenceUsage)
		return 2
	}
}

func referenceImport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("velocity-lidar-annotation-reference-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	packDir := fs.String("pack", "", "Annotation pack directory (required)")
	file := fs.String("file", "", "Import file, schema "+annotation.PhysicalImportSchema+" (required)")
	author := fs.String("author", "", "Who is running the import; recorded as the revision's author (required)")
	session := fs.String("session", "", "Optional session identifier recorded with the revision")
	replace := fs.Bool("replace", false, "Let imported records replace a body, keyframe or following reference the pack already has")
	if code, ok := parseReferenceFlags(fs, args); !ok {
		return code
	}
	for _, required := range []struct{ name, value string }{{"--pack", *packDir}, {"--file", *file}, {"--author", *author}} {
		if required.value == "" {
			fmt.Fprintf(stderr, "error: %s is required\n", required.name)
			fs.Usage()
			return 2
		}
	}
	pack, err := annotation.OpenPack(*packDir)
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
		return 1
	}
	imp, err := annotation.LoadPhysicalImport(*file)
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
		return 1
	}
	doc, err := annotation.ImportPhysicalReferences(pack, imp,
		annotation.Provenance{Author: *author, Session: *session, Operation: "import"}, *replace)
	var content string
	if err == nil {
		content, err = doc.ContentDigest()
	}
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
		return 1
	}
	printReferenceSummary(stdout, fmt.Sprintf("imported: pack %s, revision %d", doc.PackDigest, doc.Revision), doc.Digest(), content, doc)
	return 0
}

func referenceValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("velocity-lidar-annotation-reference-validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	packDir := fs.String("pack", "", "Annotation pack directory (required)")
	file := fs.String("file", "", "Dry-run this import file: every check an import makes, against the stored references, with nothing written")
	replace := fs.Bool("replace", false, "With --file, let imported records replace stored ones, as import --replace would")
	revision := fs.Int("revision", 0, "Validate a retained revision instead of the current one")
	if code, ok := parseReferenceFlags(fs, args); !ok {
		return code
	}
	usage := func(msg string) int {
		fmt.Fprintln(stderr, "error: "+msg)
		fs.Usage()
		return 2
	}
	switch {
	case *packDir == "":
		return usage("--pack is required")
	case *file != "" && *revision != 0:
		return usage("--file and --revision are alternatives")
	case *replace && *file == "":
		return usage("--replace applies only to --file")
	}
	pack, err := annotation.OpenPack(*packDir)
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
		return 1
	}
	if *file != "" {
		return referenceDryRun(pack, *file, *replace, stdout, stderr)
	}
	var doc *annotation.PhysicalReferenceSet
	if *revision != 0 {
		doc, err = annotation.LoadPhysicalReferenceRevision(pack, *revision)
	} else {
		doc, err = annotation.LoadPhysicalReferences(pack)
	}
	if err == nil && doc.Digest() == "" {
		fmt.Fprintf(stdout, "no physical references stored for pack %s\n", pack.Manifest.PackDigest)
		return 0
	}
	var content string
	if err == nil {
		content, err = doc.ContentDigest()
	}
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
		return 1
	}
	if stale := doc.Stale(); len(stale) > 0 {
		fmt.Fprintf(stderr, "annotation-reference: revision %d has %d record(s) that do not hold against annotation revision %d; "+
			"repair or remove them before the next save:\n", doc.Revision, len(stale), doc.StaleAgainst())
		for _, s := range stale {
			fmt.Fprintf(stderr, "  %s\n", s)
		}
		return 1
	}
	printReferenceSummary(stdout, fmt.Sprintf("valid: pack %s, revision %d", doc.PackDigest, doc.Revision), doc.Digest(), content, doc)
	return 0
}

// referenceDryRun makes every check an import makes, merged with the stored
// references and against the current annotation, and writes nothing.
func referenceDryRun(pack *annotation.Pack, file string, replace bool, stdout, stderr io.Writer) int {
	imp, err := annotation.LoadPhysicalImport(file)
	var doc *annotation.PhysicalReferenceSet
	if err == nil {
		doc, err = annotation.PreparePhysicalImport(pack, imp, replace)
	}
	var content string
	if err == nil {
		content, err = doc.ContentDigest()
	}
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
		return 1
	}
	printReferenceSummary(stdout, importValidHeader(doc), "", content, doc)
	return 0
}

// importValidHeader names the revision a prepared import would save.
func importValidHeader(doc *annotation.PhysicalReferenceSet) string {
	stored, next := "none stored", 1
	if doc.Digest() != "" {
		stored, next = fmt.Sprintf("current revision %d", doc.Revision), doc.Revision+1
	}
	return fmt.Sprintf("import valid: pack %s (%s); importing would save revision %d", doc.PackDigest, stored, next)
}

func referenceDraftFollowing(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("velocity-lidar-annotation-reference-draft-following", flag.ContinueOnError)
	fs.SetOutput(stderr)
	packDir := fs.String("pack", "", "Annotation pack directory (required)")
	follower := fs.String("follower", "", "The follower's object ID (required)")
	decision := fs.String("decision", string(annotation.FollowingLeader), "leader, no_leader or ambiguous")
	leader := fs.String("leader", "", "The leader's object ID: required for a leader decision, refused otherwise")
	first := fs.Int("first", -1, "First sample of the interval the decision covers (required)")
	last := fs.Int("last", -1, "Last sample of the interval, inclusive (required)")
	author := fs.String("author", "", "Who made the decision; recorded as the record's author (required)")
	session := fs.String("session", "", "Optional session identifier recorded with the record")
	id := fs.String("id", "", "Following record ID; default a fresh one. Reuse a stored ID with --replace to revise it")
	reviewed := fs.Bool("reviewed", false, "Record the decision and its gaps as reviewed: a following record has no later review step")
	trackerSource := fs.String("tracker-source", "", "Tracker output the decision was made against; the record is then tracker-assisted")
	replace := fs.Bool("replace", false, "Check the draft as import --replace would: a stored record with this ID is replaced")
	out := fs.String("out", "", "Import file to write; an existing file is not overwritten (required)")
	if code, ok := parseReferenceFlags(fs, args); !ok {
		return code
	}
	usage := func(msg string) int {
		fmt.Fprintln(stderr, "error: "+msg)
		fs.Usage()
		return 2
	}
	for _, required := range []struct{ name, value string }{
		{"--pack", *packDir}, {"--follower", *follower}, {"--author", *author}, {"--out", *out},
	} {
		if required.value == "" {
			return usage(required.name + " is required")
		}
	}
	switch d := annotation.FollowingDecision(*decision); {
	case d != annotation.FollowingLeader && d != annotation.FollowingNoLeader && d != annotation.FollowingAmbiguous:
		return usage(fmt.Sprintf("--decision %q (want leader, no_leader or ambiguous)", *decision))
	case *first < 0 || *last < 0:
		return usage("--first and --last are required")
	case d == annotation.FollowingLeader && *leader == "":
		return usage("a leader decision needs --leader")
	case d != annotation.FollowingLeader && *leader != "":
		return usage("--leader applies only to a leader decision")
	}
	pack, err := annotation.OpenPack(*packDir)
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
		return 1
	}
	doc, err := annotation.LoadPhysicalReferences(pack)
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
		return 1
	}
	imp, skipped, err := annotation.DraftFollowing(doc, annotation.FollowingDraft{
		FollowingID: *id, FollowerObjectID: *follower, Decision: annotation.FollowingDecision(*decision),
		LeaderObjectID: *leader, Interval: annotation.FrameInterval{FirstSample: *first, LastSample: *last},
		Reviewed: *reviewed, TrackerSource: *trackerSource, Author: *author, Session: *session,
	})
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
		return 1
	}
	// The draft is written only if the import would accept it.
	merged, err := annotation.PreparePhysicalImport(pack, imp, *replace)
	var content string
	if err == nil {
		content, err = merged.ContentDigest()
	}
	var b []byte
	if err == nil {
		b, err = json.MarshalIndent(imp, "", "  ")
	}
	if err == nil {
		err = writeNewFile(*out, append(b, '\n'))
	}
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
		return 1
	}
	printDraftedFollowing(stdout, imp.Following[0], skipped)
	printReferenceSummary(stdout, importValidHeader(merged), "", content, merged)
	fmt.Fprintf(stdout, "wrote %s\n", *out)
	return 0
}

// printDraftedFollowing reports the drafted record: its decision, each gap,
// and each sample with a keyframe that has no gap, with the reason.
func printDraftedFollowing(stdout io.Writer, f annotation.FollowingReference, skipped []annotation.SkippedGap) {
	what := string(f.Decision)
	if f.Decision == annotation.FollowingLeader {
		what = "follows " + f.LeaderObjectID
	}
	fmt.Fprintf(stdout, "drafted following %s: %s %s over samples [%d, %d], %s and %s\n", f.FollowingID,
		f.FollowerObjectID, what, f.Interval.FirstSample, f.Interval.LastSample, f.Review.Status, f.Review.Origin)
	for _, g := range f.Gaps {
		fmt.Fprintf(stdout, "  gap at sample %d: %s, %.2f m in [%.2f, %.2f]\n", g.SampleID, g.Status, *g.ValueM, *g.LowerM, *g.UpperM)
	}
	for _, s := range skipped {
		fmt.Fprintf(stdout, "  no gap at sample %d: %s\n", s.SampleID, s.Reason)
	}
	if f.Decision == annotation.FollowingLeader && len(f.Gaps) == 0 {
		fmt.Fprintln(stdout, "  no gaps: the decision is recorded, and its instants count as having no gap reference")
	}
	if f.Review.Status != annotation.StatusReviewed {
		fmt.Fprintln(stdout, "  proposed: scoring counts it as unreviewed; once reviewed, draft it again with "+
			"--reviewed and the same --id, and import it with --replace")
	}
}

// writeNewFile writes a file that must not already exist.
func writeNewFile(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func parseReferenceFlags(fs *flag.FlagSet, args []string) (int, bool) {
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0, false
		}
		return 2, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(fs.Output(), "error: unexpected arguments: %v\n", fs.Args())
		fs.Usage()
		return 2, false
	}
	return 0, true
}

// printReferenceSummary prints a header, the byte digest of the stored
// revision it names (none for a dry run), the content digest and the counts.
func printReferenceSummary(stdout io.Writer, header, revisionDigest, content string, doc *annotation.PhysicalReferenceSet) {
	bodies, keyframes, truth := 0, 0, 0
	for _, o := range doc.Objects {
		if o.Body != nil {
			bodies++
		}
		for _, k := range o.Keyframes {
			keyframes++
			if k.Review.ScoredAsTruth() {
				truth++
			}
		}
	}
	fmt.Fprintln(stdout, header)
	if revisionDigest != "" {
		fmt.Fprintf(stdout, "revision digest %s\n", revisionDigest)
	}
	fmt.Fprintf(stdout, "content digest %s\n", content)
	fmt.Fprintf(stdout, "%d objects, %d bodies, %d keyframes (%d reviewed and independent), %d following references\n",
		len(doc.Objects), bodies, keyframes, truth, len(doc.Following))
}
