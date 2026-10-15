//go:build pcap
// +build pcap

package lidar

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
	"github.com/banshee-data/velocity.report/internal/version"
)

const labelsUsage = `velocity lidar labels — capture-bound label sets

A label set cites returns of a capture by (packet key, block, channel), so it
applies to every run of the capture and every file holding its packets. Its
history is a journal of deltas; only its current state is published.

Usage:
  velocity lidar labels <command> [flags]

Commands:
  create    Start a label set from a pack and link the pack to it
  attach    Add another pack of the same capture as a domain and link it
  bind      Verify a capture file holds the label set's packets and record it
  verify    Replay the journal and check every revision's digest
  info      Summarise a label set
  project   Print a linked pack's membership in its own point indices
  publish   Copy the current state, without the journal, to a directory
            (--replace refreshes a published copy that is behind)
  publish-view
            Copy a linked pack into a published label set's views/ folder:
            its data and physical references, linked to the set it is in
  check-view
            Check a view and its label set as a fresh copy of their tracked
            files would hold them: run it before committing an edit
  rehead    Make the head files on disk the head after a Git merge: keep this
            copy's labels.json in the conflict, then run it
  relayout  Rewrite the head in the storage layout (from the single-file
            layout, or with another --window-chunk); the state is unchanged
  nest      Move an annotation tree into the layout: sets in their capture
            folder, linked packs in the set's views/, others in packs/
  census    Count packet keys in capture files and report duplicates`

// LabelsMain routes `velocity lidar labels`.
func LabelsMain(args []string) int {
	if len(args) == 0 {
		fmt.Println(labelsUsage)
		return 0
	}
	cmds := map[string]func([]string) error{
		"create": labelsCreate, "attach": labelsAttach, "bind": labelsBind,
		"verify": labelsVerify, "info": labelsInfo, "project": labelsProject, "publish": labelsPublish,
		"publish-view": labelsPublishView, "check-view": labelsCheckView, "rehead": labelsRehead, "relayout": labelsRelayout,
		"nest": labelsNest, "census": labelsCensus,
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Println(labelsUsage)
		return 0
	}
	run, ok := cmds[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown labels command %q\n\n%s\n", args[0], labelsUsage)
		return 2
	}
	if err := run(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		fmt.Fprintf(os.Stderr, "velocity lidar labels %s: %v\n", args[0], err)
		return 1
	}
	return 0
}

func buildIdentity() string {
	return version.Version + "+" + version.GitSHA
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func labelsCreate(args []string) error {
	fs := flag.NewFlagSet("labels create", flag.ContinueOnError)
	pack := fs.String("pack", "", "Version 2 pack directory (required)")
	out := fs.String("labels", "", "Label set directory to create (default: <capture folder>/<name>, beside the pack's packs/ folder)")
	name := fs.String("name", "", "Label set name (required)")
	author := fs.String("author", "", "Who is creating it (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pack == "" || *name == "" || *author == "" {
		return fmt.Errorf("--pack, --name and --author are required")
	}
	p, err := annotation.OpenPack(*pack)
	if err != nil {
		return err
	}
	dir := *out
	if dir == "" {
		dir = filepath.Join(annotation.CaptureDirOf(p.Dir, p.Manifest), *name)
	}
	layout, state, err := labelset.NewFromPack(p)
	if err != nil {
		return err
	}
	ls, err := labelset.Create(dir, labelset.Manifest{Name: *name, Layout: layout}, state, labelset.Change{Author: *author, Operation: "create"})
	if err != nil {
		return err
	}
	if err := annotation.WriteLabelSetLink(p, ls.Manifest.LabelSetID, dir); err != nil {
		return err
	}
	fmt.Printf("created %s (%s) with %d windows from %s\n", dir, ls.Manifest.LabelSetID, len(ls.State.Windows), *pack)
	return nil
}

func labelsAttach(args []string) error {
	fs := flag.NewFlagSet("labels attach", flag.ContinueOnError)
	pack := fs.String("pack", "", "Version 2 pack of the same capture (required)")
	dir := fs.String("labels", "", "Label set directory (required)")
	author := fs.String("author", "", "Who is attaching it (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pack == "" || *dir == "" || *author == "" {
		return fmt.Errorf("--pack, --labels and --author are required")
	}
	p, err := annotation.OpenPack(*pack)
	if err != nil {
		return err
	}
	ls, err := labelset.Open(*dir)
	if err != nil {
		return err
	}
	skipped, err := labelset.AddPackDomain(ls.Manifest.Layout, &ls.State, p)
	if err != nil {
		return err
	}
	if err := ls.Save(labelset.Change{Author: *author, Operation: "attach_pack"}); err != nil {
		return err
	}
	if err := annotation.WriteLabelSetLink(p, ls.Manifest.LabelSetID, *dir); err != nil {
		return err
	}
	fmt.Printf("attached %s as revision %d; %d samples skipped (framed differently)\n", *pack, ls.Revision(), len(skipped))
	return nil
}

func labelsBind(args []string) error {
	fs := flag.NewFlagSet("labels bind", flag.ContinueOnError)
	dir := fs.String("labels", "", "Label set directory (required)")
	pcap := fs.String("pcap", "", "Capture file (required)")
	port := fs.Int("port", 2369, "LiDAR data UDP port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || *pcap == "" {
		return fmt.Errorf("--labels and --pcap are required")
	}
	res, err := labelset.Bind(*dir, *pcap, *port, buildIdentity())
	if res != nil {
		_ = writeJSON(os.Stdout, res)
	}
	return err
}

func labelsVerify(args []string) error {
	fs := flag.NewFlagSet("labels verify", flag.ContinueOnError)
	dir := fs.String("labels", "", "Label set directory (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return fmt.Errorf("--labels is required")
	}
	n, err := labelset.Verify(*dir)
	if err != nil {
		return err
	}
	fmt.Printf("ok: %d journal revisions replay to their recorded digests\n", n)
	return nil
}

func labelsInfo(args []string) error {
	fs := flag.NewFlagSet("labels info", flag.ContinueOnError)
	dir := fs.String("labels", "", "Label set directory (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ls, err := labelset.Open(*dir)
	if err != nil {
		return err
	}
	members := uint64(0)
	for _, m := range ls.State.Masks {
		members += m.Members.Count()
	}
	return writeJSON(os.Stdout, map[string]any{
		"label_set_id": ls.Manifest.LabelSetID, "name": ls.Manifest.Name, "revision": ls.Revision(),
		"state": ls.StateDigest(), "packets": len(ls.State.Packets), "windows": len(ls.State.Windows),
		"domains": len(ls.State.Domains), "objects": len(ls.State.Records.Objects), "masks": len(ls.State.Masks),
		"reviewed_masks": len(ls.State.ReviewedMasks()), "members": members, "applies_to": ls.Manifest.AppliesTo,
		"migrated_from": ls.Manifest.MigratedFrom, "storage": ls.Manifest.Storage,
	})
}

func labelsProject(args []string) error {
	fs := flag.NewFlagSet("labels project", flag.ContinueOnError)
	pack := fs.String("pack", "", "Linked pack (required)")
	revision := fs.Int("revision", 0, "Label set revision (0: head)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := annotation.OpenPack(*pack)
	if err != nil {
		return err
	}
	var sc *annotation.Sidecar
	if *revision > 0 {
		sc, err = annotation.LoadSidecarRevision(p, *revision)
	} else {
		sc, err = annotation.LoadSidecar(p)
	}
	if err != nil {
		return err
	}
	return writeJSON(os.Stdout, sc)
}

func labelsPublish(args []string) error {
	fs := flag.NewFlagSet("labels publish", flag.ContinueOnError)
	dir := fs.String("labels", "", "Label set directory (required)")
	dest := fs.String("dest", "", "Destination directory, empty or absent (required)")
	replace := fs.Bool("replace", false, "Bring an existing published copy of this label set up to the source's state; refused if the copy has been edited, is ahead, or has diverged")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || *dest == "" {
		return fmt.Errorf("--labels and --dest are required")
	}
	publish := labelset.Publish
	if *replace {
		publish = labelset.Republish
	}
	if err := publish(*dir, *dest); err != nil {
		return err
	}
	fmt.Printf("published the current state of %s to %s\n", *dir, *dest)
	return nil
}

func labelsPublishView(args []string) error {
	fs := flag.NewFlagSet("labels publish-view", flag.ContinueOnError)
	pack := fs.String("pack", "", "Linked version 2 pack to publish (required)")
	dir := fs.String("labels", "", "The published label set it becomes the view of (required)")
	dest := fs.String("dest", "", "Destination in the published label set's views/ folder, empty or absent (default: views/<cut time>-<run>)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pack == "" || *dir == "" {
		return fmt.Errorf("--pack and --labels are required")
	}
	if *dest == "" {
		p, err := annotation.OpenPack(*pack)
		if err != nil {
			return err
		}
		*dest = filepath.Join(*dir, annotation.ViewsDir, annotation.ViewName(p.Manifest))
	}
	report, err := labelset.PublishView(*pack, *dir, *dest)
	if report != nil {
		if werr := writeJSON(os.Stdout, report); werr != nil && err == nil {
			err = werr
		}
	}
	return err
}

func labelsCheckView(args []string) error {
	fs := flag.NewFlagSet("labels check-view", flag.ContinueOnError)
	pack := fs.String("pack", "", "View (linked pack) to check (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pack == "" {
		return fmt.Errorf("--pack is required")
	}
	report, err := labelset.CheckView(*pack)
	if report != nil {
		if werr := writeJSON(os.Stdout, report); werr != nil && err == nil {
			err = werr
		}
	}
	return err
}

func labelsRehead(args []string) error {
	fs := flag.NewFlagSet("labels rehead", flag.ContinueOnError)
	dir := fs.String("labels", "", "Label set directory (required)")
	author := fs.String("author", "", "Who resolved the merge; recorded in the journal (required)")
	session := fs.String("session", "", "Optional session identifier")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || *author == "" {
		return fmt.Errorf("--labels and --author are required")
	}
	r, err := labelset.Rehead(*dir, labelset.Change{Author: *author, Session: *session})
	if err != nil {
		return err
	}
	switch {
	case r.Revised:
		fmt.Printf("revision %d is the merged head (%s), after %d\n", r.To.Revision, r.To.State, r.From.Revision)
	case r.Relaid:
		fmt.Printf("revision %d unchanged; its files were rewritten in the label set's layout\n", r.To.Revision)
	default:
		fmt.Printf("revision %d: the files already are the head\n", r.To.Revision)
	}
	return nil
}

func labelsRelayout(args []string) error {
	fs := flag.NewFlagSet("labels relayout", flag.ContinueOnError)
	dir := fs.String("labels", "", "Label set directory (required)")
	chunk := fs.Int("window-chunk", labelset.DefaultWindowChunk, "Windows per masks/ and domain-returns/ file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return fmt.Errorf("--labels is required")
	}
	if err := labelset.Relayout(*dir, *chunk); err != nil {
		return err
	}
	ls, err := labelset.Open(*dir)
	if err != nil {
		return err
	}
	fmt.Printf("%s: revision %d (%s) in the storage layout, %d windows per chunk\n", *dir, ls.Revision(), ls.StateDigest(), *chunk)
	return nil
}

func labelsNest(args []string) error {
	fs := flag.NewFlagSet("labels nest", flag.ContinueOnError)
	root := fs.String("root", "", "Annotation root: --lidar-annotation-dir, or a dataset's labels/ (required)")
	dryRun := fs.Bool("dry-run", false, "Print the moves without making them")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return fmt.Errorf("--root is required")
	}
	report, err := labelset.Nest(*root, *dryRun)
	if report != nil {
		if werr := writeJSON(os.Stdout, report); werr != nil && err == nil {
			err = werr
		}
	}
	return err
}

func labelsCensus(args []string) error {
	fs := flag.NewFlagSet("labels census", flag.ContinueOnError)
	port := fs.Int("port", 2369, "LiDAR data UDP port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("name one or more capture files")
	}
	failed := 0
	for _, path := range fs.Args() {
		c, err := labelset.CensusOf(path, *port)
		if err != nil {
			fmt.Printf("%s\terror\t%v\n", path, err)
			failed++
			continue
		}
		fmt.Printf("%s\t%s\tbytes=%d\tpackets=%d\tunique=%d\tduplicated_keys=%d\textra=%d\tsame_payload=%d\n",
			path, c.SHA256, c.Bytes, c.Packets, c.Unique, c.Duplicated, c.Extra, c.SamePayload)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d files could not be read", failed, fs.NArg())
	}
	return nil
}
