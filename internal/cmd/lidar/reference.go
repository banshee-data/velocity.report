//go:build pcap
// +build pcap

package lidar

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

const annotationReferenceUsage = `Usage: velocity lidar annotation-reference <import|validate> --pack DIR [flags]

Import or validate the physical references of an annotation pack: body
dimensions, keyframe poses and following gaps, each with bounds, evidence
status and its own review, stored beside the pack's membership sidecar.

  import    Validate an import file against the pack and its annotation,
            merge it into the current references and save a new revision
  validate  Check the stored references against the current annotation, or
            with --file dry-run an import (every import check, no write)

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
	stored, next := "none stored", 1
	if doc.Digest() != "" {
		stored, next = fmt.Sprintf("current revision %d", doc.Revision), doc.Revision+1
	}
	printReferenceSummary(stdout, fmt.Sprintf("import valid: pack %s (%s); importing would save revision %d",
		doc.PackDigest, stored, next), "", content, doc)
	return 0
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
