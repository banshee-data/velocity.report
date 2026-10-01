// Command lidar-annotation-prune thins an annotation pack's revision history.
//
// Every save of the annotation window archives the exact bytes it replaced as a
// full snapshot in annotation-revisions/, and nothing removes them. A pack with
// a few thousand masks writes about 25 MB per save: 1,968 saves made 38 GB. The
// history is the recovery path and a frozen split pins one revision by digest,
// but almost all of it is never read again.
//
// This tool plans a prune first and writes nothing:
//
//	lidar-annotation-prune -pack PACK -keep-last 200 -keep-every 1h -split frozen-split.json
//
// It keeps the newest -keep-last revisions, the oldest, one revision per
// -keep-every interval of what is older, and every revision a -split file or a
// -protect flag names. Adding -apply and -backup FILE.tar.gz does the prune:
//
//  1. every revision to remove, and the current snapshot, are written to a
//     gzip-compressed tar with a SHA-256 manifest;
//  2. the tar is read back and every member's digest is compared with the
//     digest taken from the pack;
//  3. only then are the revisions removed, under the pack's annotation writer
//     lock, so the annotation window can stay open: its saves archive new
//     revisions beyond the ones being removed.
//
// A backup that does not verify, a busy lock or a full volume leaves the pack
// exactly as it was. To restore, extract the backup into the pack directory
// (tar -xzf FILE.tar.gz -C PACK); the current snapshot comes out under head/
// and does not replace the live one.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// listFlag collects a repeated string flag.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lidar-annotation-prune", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pack := fs.String("pack", "", "annotation pack directory (required)")
	keepLast := fs.Int("keep-last", 200, "keep the newest this many retained revisions (at least 1)")
	keepEvery := fs.Duration("keep-every", time.Hour, "thin older revisions to one per interval, by the time each was archived; 0 keeps none of them")
	var splits, protect listFlag
	fs.Var(&splits, "split", "a version 1 or frozen split file whose pinned revision for this pack is kept (repeatable)")
	fs.Var(&protect, "protect", "a revision number to keep (repeatable)")
	apply := fs.Bool("apply", false, "do the prune: needs -backup; without it nothing is written")
	backup := fs.String("backup", "", "with -apply: the .tar.gz that receives every removed revision and the current snapshot first (must not exist)")
	minFree := fs.Float64("min-free-ratio", annotation.DefaultPruneMinFreeRatio, "with -apply: refuse unless the backup volume has this fraction of the removed bytes free")
	jsonPath := fs.String("json", "", "write the plan, and the report with -apply, as JSON to this file")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lidar-annotation-prune -pack DIR [-keep-last N] [-keep-every DUR] [-split FILE] [-protect REV] [-apply -backup FILE.tar.gz]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 || *pack == "" {
		fmt.Fprintln(stderr, "error: -pack is required and takes no other arguments")
		fs.Usage()
		return 2
	}
	if *apply && *backup == "" {
		fmt.Fprintln(stderr, "error: -apply needs -backup: nothing is removed before a backup is verified")
		return 2
	}
	if !*apply && *backup != "" {
		fmt.Fprintln(stderr, "error: -backup is only used with -apply")
		return 2
	}

	protected, err := annotation.ProtectedBySplits(*pack, splits)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	for _, v := range protect {
		rev, err := strconv.Atoi(v)
		if err != nil || rev < 1 {
			fmt.Fprintf(stderr, "error: -protect %q: want a revision number of 1 or more\n", v)
			return 2
		}
		protected[rev] = "named by -protect"
	}
	plan, err := annotation.PlanPrune(*pack, annotation.PrunePolicy{KeepLast: *keepLast, KeepEvery: *keepEvery, Protect: protected})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	printPlan(stdout, plan, protected)

	var report *annotation.PruneReport
	if *apply {
		report, err = plan.Execute(annotation.PruneExecuteOptions{
			BackupPath: *backup, MinFreeRatio: *minFree,
			Progress: func(s string) { fmt.Fprintln(stdout, s) },
		})
		if report != nil {
			printReport(stdout, report)
		}
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
		}
	} else {
		fmt.Fprintln(stdout, "dry run: nothing was written. To prune, add -apply -backup /path/to/backup.tar.gz")
	}
	if *jsonPath != "" {
		payload, jerr := json.MarshalIndent(struct {
			Plan   *annotation.PrunePlan   `json:"plan"`
			Report *annotation.PruneReport `json:"report,omitempty"`
		}{plan, report}, "", "  ")
		if jerr == nil {
			jerr = os.WriteFile(*jsonPath, append(payload, '\n'), 0o644)
		}
		if jerr != nil {
			fmt.Fprintf(stderr, "error: write %s: %v\n", *jsonPath, jerr)
			return 1
		}
	}
	if err != nil {
		return 1
	}
	return 0
}

func printPlan(w io.Writer, p *annotation.PrunePlan, protected map[int]string) {
	total := p.KeptBytes + p.PrunedBytes
	fmt.Fprintf(w, "history of %s\n", p.PackDir)
	fmt.Fprintf(w, "  retained: %d revisions, %s\n", len(p.Entries), humanBytes(total))
	fmt.Fprintf(w, "  keep:     %d revisions, %s (%s)\n", p.Kept, humanBytes(p.KeptBytes), reasons(p.KeptByReason))
	fmt.Fprintf(w, "  prune:    %d revisions, %s\n", p.Pruned, humanBytes(p.PrunedBytes))
	if len(protected) > 0 {
		var revs []int
		for r := range protected {
			revs = append(revs, r)
		}
		sort.Ints(revs)
		for _, r := range revs {
			fmt.Fprintf(w, "  protected revision %d: %s\n", r, protected[r])
		}
	}
	if len(p.Ignored) > 0 {
		fmt.Fprintf(w, "  ignored (not archive files): %s\n", strings.Join(p.Ignored, ", "))
	}
}

func printReport(w io.Writer, r *annotation.PruneReport) {
	if r.BackupPath != "" {
		fmt.Fprintf(w, "backup: %s, %s, sha256 %s\n", r.BackupPath, humanBytes(r.BackupBytes), r.BackupSHA256)
	}
	fmt.Fprintf(w, "removed: %d revisions, %s\n", r.Removed, humanBytes(r.RemovedBytes))
	for _, s := range r.Skipped {
		fmt.Fprintf(w, "  skipped %s\n", s)
	}
	if r.Elapsed > 0 {
		fmt.Fprintf(w, "took %s\n", r.Elapsed.Round(time.Second))
	}
}

func reasons(m map[string]int) string {
	var parts []string
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%d %s", v, k))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}
