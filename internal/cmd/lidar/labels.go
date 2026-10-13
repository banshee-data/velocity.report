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
		"census": labelsCensus,
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
	out := fs.String("labels", "", "Label set directory to create (default: <pack parent>/labels/<name>)")
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
		dir = filepath.Join(filepath.Dir(filepath.Clean(*pack)), "labels", *name)
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
		"migrated_from": ls.Manifest.MigratedFrom,
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
