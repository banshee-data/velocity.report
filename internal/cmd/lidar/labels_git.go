//go:build pcap
// +build pcap

package lidar

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
)

// Label sets in Git: what a change did, whether a tree holds, and the merge
// drivers and hooks that let Git finish a merge of label sets.

// errConflict is a merge driver's refusal: Git marks the file conflicted.
var errConflict = errors.New("merge conflict")

func labelsDiff(args []string) error {
	fs := flag.NewFlagSet("labels diff", flag.ContinueOnError)
	format := fs.String("format", "text", "text, markdown or json")
	revs := fs.String("git", "", "Compare Git revisions: REV (against the working tree) or REV..REV")
	setPath := fs.String("labels", "", "With --git: the label set's folder")
	quiet := fs.Bool("quiet", false, "Print nothing; exit 1 when a label changed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var from, to *labelset.LabelSet
	switch {
	case *revs != "":
		if *setPath == "" || fs.NArg() != 0 {
			return fmt.Errorf("--git takes --labels SET and no folders")
		}
		repo, rel, err := repoPath(*setPath)
		if err != nil {
			return err
		}
		a, b, _ := strings.Cut(*revs, "..")
		var done func()
		if from, done, err = labelset.OpenRevision(repo, a, rel); err != nil {
			return err
		}
		defer done()
		if b == "" {
			if to, err = labelset.Open(*setPath); err != nil {
				return err
			}
		} else if to, done, err = labelset.OpenRevision(repo, b, rel); err != nil {
			return err
		} else {
			defer done()
		}
	case fs.NArg() == 2:
		var err error
		if from, err = labelset.Open(fs.Arg(0)); err != nil {
			return err
		}
		if to, err = labelset.Open(fs.Arg(1)); err != nil {
			return err
		}
	default:
		return fmt.Errorf("name two label set folders, or --git REV[..REV] --labels SET")
	}
	if from.Manifest.LabelSetID != to.Manifest.LabelSetID {
		return fmt.Errorf("these are two label sets (%s, %s), not two states of one", from.Manifest.LabelSetID, to.Manifest.LabelSetID)
	}
	s := labelset.Summarise(&from.State, &to.State)
	s.Name, s.From, s.To = to.Manifest.Name, from.Manifest.Head, to.Manifest.Head
	if *quiet {
		if s.Empty() {
			return nil
		}
		os.Exit(1)
	}
	switch *format {
	case "text":
		fmt.Print(s.Text())
	case "markdown":
		fmt.Print(s.Markdown())
	case "json":
		return writeJSON(os.Stdout, s)
	default:
		return fmt.Errorf("--format %q: use text, markdown or json", *format)
	}
	return nil
}

// repoPath is the repository holding dir and dir's path in it.
func repoPath(dir string) (repo, rel string, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	out, err := exec.Command("git", "-C", abs, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", "", fmt.Errorf("%s is not in a Git repository: %v", dir, err)
	}
	repo = strings.TrimSpace(string(out))
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	if real, err := filepath.EvalSymlinks(repo); err == nil {
		repo = real
	}
	r, err := filepath.Rel(repo, abs)
	if err != nil {
		return "", "", err
	}
	return repo, filepath.ToSlash(r), nil
}

func labelsCheckTree(args []string) error {
	fs := flag.NewFlagSet("labels check-tree", flag.ContinueOnError)
	root := fs.String("root", "", "Annotation root: a dataset's labels/ (required)")
	base := fs.String("base", "", "Also print, in Markdown, what changed in each set since this Git revision")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return fmt.Errorf("--root is required")
	}
	report, err := labelset.CheckTree(*root)
	if report != nil && *base == "" {
		if werr := writeJSON(os.Stdout, report); werr != nil && err == nil {
			err = werr
		}
	}
	if report != nil && *base != "" {
		for _, set := range report.Sets {
			dir := filepath.Join(report.Root, filepath.FromSlash(set))
			repo, rel, rerr := repoPath(dir)
			if rerr != nil {
				return rerr
			}
			now, oerr := labelset.Open(dir)
			if oerr != nil {
				continue
			}
			then, done, oerr := labelset.OpenRevision(repo, *base, rel)
			if oerr != nil {
				fmt.Printf("### %s\n\nNew since %s.\n\n", set, *base)
				continue
			}
			s := labelset.Summarise(&then.State, &now.State)
			s.Name, s.From, s.To = set, then.Manifest.Head, now.Manifest.Head
			done()
			fmt.Println(s.Markdown())
		}
		for _, p := range report.Problems {
			fmt.Printf("- problem: %s\n", p)
		}
	}
	return err
}

// labelsMergeDriver is what Git runs for labels.json and
// physical-references.json in a merge: merge-driver head|refs %O %A %B %P.
// It writes the result over %A and exits non-zero on a conflict, which Git
// then reports for the file.
func labelsMergeDriver(args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: labels merge-driver head|refs BASE OURS THEIRS [PATH]")
	}
	kind, basePath, oursPath, theirsPath := args[0], args[1], args[2], args[3]
	name := oursPath
	if len(args) > 4 {
		name = args[4]
	}
	base, err := os.ReadFile(basePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ours, err := os.ReadFile(oursPath)
	if err != nil {
		return err
	}
	theirs, err := os.ReadFile(theirsPath)
	if err != nil {
		return err
	}
	var merged []byte
	var conflicts []string
	switch kind {
	case "head":
		var conflict string
		merged, conflict, err = labelset.MergeManifests(base, ours, theirs)
		if conflict != "" {
			conflicts = []string{conflict}
		}
	case "refs":
		merged, conflicts, err = annotation.MergePhysicalReferences(base, ours, theirs)
	default:
		return fmt.Errorf("merge driver %q: use head or refs", kind)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if merged != nil {
		if err := os.WriteFile(oursPath, merged, 0o644); err != nil {
			return err
		}
	}
	if len(conflicts) > 0 {
		if kind == "refs" {
			// The other branch's whole file, to resolve from.
			_ = os.WriteFile(name+".theirs", theirs, 0o644)
		}
		fmt.Fprintf(os.Stderr, "%s: %s\n", name, strings.Join(conflicts, "; "))
		return errConflict
	}
	return nil
}

func labelsFinishMerge(args []string) error {
	fs := flag.NewFlagSet("labels finish-merge", flag.ContinueOnError)
	root := fs.String("root", "labels", "Annotation root holding the merged label sets")
	author := fs.String("author", "", "Who is merging (default: git config user.name)")
	gitAdd := fs.Bool("git-add", false, "Stage what the merge's commit must take")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *author == "" {
		if out, err := exec.Command("git", "config", "user.name").Output(); err == nil {
			*author = strings.TrimSpace(string(out))
		}
	}
	if *author == "" {
		return fmt.Errorf("--author is required (or set git config user.name)")
	}
	done, paths, err := labelset.FinishMerges(*root, labelset.Change{Author: *author, Operation: "merge"})
	for _, f := range done {
		fmt.Fprintf(os.Stderr, "finished the merge of %s: revision %d → %d; %d review(s) re-pinned, %d to review again\n",
			f.Set, f.Result.From.Revision, f.Result.To.Revision, f.Repinned, f.Drifted)
	}
	if *gitAdd && len(paths) > 0 {
		add := exec.Command("git", append([]string{"add", "--"}, paths...)...)
		add.Stdout, add.Stderr = os.Stderr, os.Stderr
		if aerr := add.Run(); aerr != nil && err == nil {
			err = fmt.Errorf("git add: %w", aerr)
		}
	}
	return err
}

// The hooks git-setup installs. Git runs pre-merge-commit when a merge
// commits by itself, and pre-commit when a merge that stopped on a conflict
// is committed by hand; both finish any label set merge first. A rebase runs
// neither: run finish-merge yourself after one.
const finishMergeHook = `#!/bin/sh
# Installed by 'velocity lidar labels git-setup'. Before Git commits a merge,
# make each merged label set's head files its next revision, re-pin the
# reviews that still hold, and check every view as a fresh copy would hold it.
grep -rqs --include=labels.json '"merge":' labels || exit 0
exec %q lidar labels finish-merge --root labels --git-add
`

func labelsGitSetup(args []string) error {
	fs := flag.NewFlagSet("labels git-setup", flag.ContinueOnError)
	repo := fs.String("repo", ".", "The dataset clone")
	if err := fs.Parse(args); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	top, err := exec.Command("git", "-C", *repo, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("%s is not a Git repository: %v", *repo, err)
	}
	root := strings.TrimSpace(string(top))
	hooks := filepath.Join(root, ".githooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		return err
	}
	hook := []byte(fmt.Sprintf(finishMergeHook, exe))
	for _, name := range []string{"pre-merge-commit", "pre-commit"} {
		if err := os.WriteFile(filepath.Join(hooks, name), hook, 0o755); err != nil {
			return err
		}
	}
	driver := func(kind string) string {
		return fmt.Sprintf("%q lidar labels merge-driver %s %%O %%A %%B %%P", exe, kind)
	}
	for _, kv := range [][2]string{
		{"core.hooksPath", ".githooks"},
		{"merge.labelset-head.name", "velocity.report label set head"},
		{"merge.labelset-head.driver", driver("head")},
		{"merge.labelset-refs.name", "velocity.report physical references"},
		{"merge.labelset-refs.driver", driver("refs")},
		{"diff.lfs.textconv", "cat"},
	} {
		if out, err := exec.Command("git", "-C", root, "config", kv[0], kv[1]).CombinedOutput(); err != nil {
			return fmt.Errorf("git config %s: %v: %s", kv[0], err, out)
		}
	}
	attrs, _ := os.ReadFile(filepath.Join(root, ".gitattributes"))
	for _, rule := range []string{"merge=labelset-head", "merge=labelset-refs"} {
		if !bytes.Contains(attrs, []byte(rule)) {
			fmt.Fprintf(os.Stderr, "warning: .gitattributes has no %s rule: stage the dataset with tools/s2-archive/stage-label-sets.py\n", rule)
		}
	}
	report, _ := json.Marshal(map[string]string{"repo": root, "hooks": hooks, "velocity": exe})
	fmt.Println(string(report))
	return nil
}
