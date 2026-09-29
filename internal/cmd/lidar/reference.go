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
  validate  Check the stored references (or --file, without saving) and
            print their revision and digests

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
	printReferenceSummary(stdout, "imported", doc, content)
	return 0
}

func referenceValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("velocity-lidar-annotation-reference-validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	packDir := fs.String("pack", "", "Annotation pack directory (required)")
	file := fs.String("file", "", "Validate this import file instead of the stored references; nothing is written")
	revision := fs.Int("revision", 0, "Validate a retained revision instead of the current one")
	if code, ok := parseReferenceFlags(fs, args); !ok {
		return code
	}
	if *packDir == "" {
		fmt.Fprintln(stderr, "error: --pack is required")
		fs.Usage()
		return 2
	}
	if *file != "" && *revision != 0 {
		fmt.Fprintln(stderr, "error: --file and --revision are alternatives")
		fs.Usage()
		return 2
	}
	pack, err := annotation.OpenPack(*packDir)
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
		return 1
	}
	sidecar, err := annotation.LoadSidecar(pack)
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: load annotation: %v\n", err)
		return 1
	}
	var doc *annotation.PhysicalReferenceSet
	switch {
	case *file != "":
		imp, err := annotation.LoadPhysicalImport(*file)
		if err == nil {
			err = imp.Validate(pack, sidecar)
		}
		if err != nil {
			fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
			return 1
		}
		doc = imp.Document()
	case *revision != 0:
		doc, err = annotation.LoadPhysicalReferenceRevision(pack, *revision)
	default:
		doc, err = annotation.LoadPhysicalReferences(pack)
	}
	if err == nil {
		err = doc.ValidateLinks(pack, sidecar)
	}
	var content string
	if err == nil {
		content, err = doc.ContentDigest()
	}
	if err != nil {
		fmt.Fprintf(stderr, "annotation-reference: %v\n", err)
		return 1
	}
	printReferenceSummary(stdout, "valid", doc, content)
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

func printReferenceSummary(stdout io.Writer, verb string, doc *annotation.PhysicalReferenceSet, content string) {
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
	fmt.Fprintf(stdout, "%s: pack %s, revision %d\n", verb, doc.PackDigest, doc.Revision)
	if d := doc.Digest(); d != "" {
		fmt.Fprintf(stdout, "revision digest %s\n", d)
	}
	fmt.Fprintf(stdout, "content digest %s\n", content)
	fmt.Fprintf(stdout, "%d objects, %d bodies, %d keyframes (%d reviewed and independent), %d following references\n",
		len(doc.Objects), bodies, keyframes, truth, len(doc.Following))
}
