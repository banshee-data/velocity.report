package labelset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// Nest moves an annotation tree into the layout of annotation's layout.go:
// label sets into their capture folder, packs linked to a set into the set's
// views/ folder, and packs not in a set into their capture's packs/ folder.
// It takes the two trees the tools wrote before the layout, a root holding
// packs beside labels/<set>/ and a dataset's labels/<capture>/ holding a set
// beside its view, and leaves a tree already in the layout as it is.
//
// Every move is planned before any is made, and a problem anywhere refuses
// the whole run. Moves are renames within root, so nothing is copied, and
// the links of every pack whose set or own folder moved are rewritten. Split
// drafts in root (*.json naming packs by "dir") are pointed at the new
// folders; frozen splits pin packs by digest and need nothing. Close every
// annotation window and stop the server first: nothing here can tell that a
// window has a pack open. A clip job's pack not yet in a set stays in the
// job's folder; once linked it moves to views/ like any other, and the
// server finds it again by its digest.

// NestMove is one rename.
type NestMove struct {
	What string `json:"what"`
	From string `json:"from"`
	To   string `json:"to"`
}

// NestReport is what Nest did, or would do.
type NestReport struct {
	Root string `json:"root"`
	// From, Adopted and Archive are Gather's: the tree gathered, what the
	// destination's copies took from it, and where the local copies went.
	From     string     `json:"from,omitempty"`
	Moves    []NestMove `json:"moves,omitempty"`
	Adopted  []NestMove `json:"adopted,omitempty"`
	Archive  string     `json:"archive,omitempty"`
	Relinked []string   `json:"relinked,omitempty"`
	Drafts   []string   `json:"drafts,omitempty"`
	Problems []string   `json:"problems,omitempty"`
}

// ErrNestRefused means the tree has a problem; nothing was moved.
var ErrNestRefused = errors.New("nest refused; nothing was moved")

type nestSet struct {
	dir, target string
	id, name    string
	appliesTo   []Binding
	merging     bool
}

type nestPack struct {
	dir, target string // target is where the pack directory itself ends up
	move        string // the directory renamed: dir, or a clip attempt holding it
	moveTarget  string
	manifest    annotation.Manifest
	link        *annotation.LabelSetLink
	set         *nestSet
}

// Nest plans the moves and, unless dryRun, makes them.
func Nest(root string, dryRun bool) (*NestReport, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	report := &NestReport{Root: root}
	problem := func(format string, args ...any) {
		report.Problems = append(report.Problems, fmt.Sprintf(format, args...))
	}
	sets, packs, err := nestInventory(root)
	if err != nil {
		return nil, err
	}
	byDir := map[string]*nestSet{}
	for _, s := range sets {
		byDir[s.dir] = s
	}
	for _, p := range packs {
		if p.link == nil {
			continue
		}
		dir := filepath.Clean(p.link.LabelSetDir(p.dir))
		s := byDir[dir]
		switch {
		case s == nil:
			problem("%s links label set %s, which is not a label set under %s", rel(root, p.dir), p.link.Path, root)
		case s.id != p.link.LabelSetID:
			problem("%s links label set %s, %s holds %s", rel(root, p.dir), p.link.LabelSetID, rel(root, s.dir), s.id)
		default:
			p.set = s
		}
	}

	// A set already in a capture folder stays in it: in a dataset the folder
	// is the dataset capture's name, which the pack's own file name need not
	// be. A set in the old labels/ folder goes to the capture its packs, or
	// failing them its bound files, name.
	for _, s := range sets {
		parent := filepath.Dir(s.dir)
		if filepath.Base(parent) != "labels" || filepath.Dir(parent) != root {
			s.target = s.dir
			continue
		}
		captures := map[string]bool{}
		for _, p := range packs {
			if p.set == s {
				captures[annotation.CaptureName(p.manifest)] = true
			}
		}
		if len(captures) == 0 {
			for _, b := range s.appliesTo {
				captures[annotation.CaptureNameOf(b.Name)] = true
			}
		}
		switch len(captures) {
		case 0:
			problem("label set %s has no linked pack and no bound file to name its capture by; bind it first", rel(root, s.dir))
		case 1:
			for c := range captures {
				s.target = filepath.Join(root, c, filepath.Base(s.dir))
			}
		default:
			problem("label set %s is linked to packs of %d captures: %v", rel(root, s.dir), len(captures), sortedKeys(captures))
		}
	}

	taken := map[string]string{}
	claim := func(target, by string) {
		if other, ok := taken[target]; ok && other != by {
			problem("%s and %s would both move to %s", rel(root, other), rel(root, by), rel(root, target))
		}
		taken[target] = by
	}
	for _, s := range sets {
		if s.target != "" {
			claim(s.target, s.dir)
		}
	}
	for _, p := range packs {
		switch {
		case p.set != nil && p.set.target != "":
			views := filepath.Join(p.set.target, annotation.ViewsDir)
			if filepath.Base(filepath.Dir(p.dir)) == annotation.ViewsDir && filepath.Dir(p.dir) == filepath.Join(p.set.dir, annotation.ViewsDir) {
				p.target = filepath.Join(views, filepath.Base(p.dir))
			} else {
				p.target = uniqueDir(views, annotation.ViewName(p.manifest), taken)
			}
			p.move, p.moveTarget = p.dir, p.target
		case p.set != nil:
			// Its set cannot move; the problem is already reported.
		case annotation.IsClipPackDir(p.dir):
			// A clip job's folder is named by its job row: it stays.
			p.target = p.dir
		default:
			packsDir := filepath.Join(root, annotation.CaptureName(p.manifest), annotation.PacksDir)
			if filepath.Dir(p.dir) == packsDir {
				p.target = p.dir
			} else {
				p.target = filepath.Join(packsDir, filepath.Base(p.dir))
			}
			p.move, p.moveTarget = p.dir, p.target
		}
		if p.moveTarget != "" {
			claim(p.moveTarget, p.move)
		}
	}
	for target, by := range taken {
		if target == by {
			continue
		}
		if _, err := os.Lstat(target); err == nil {
			problem("%s would move to %s, which exists", rel(root, by), rel(root, target))
		}
	}
	for _, s := range sets {
		if s.merging {
			problem("label set %s is in the middle of a Git merge: finish it first ('velocity lidar labels finish-merge')", rel(root, s.dir))
		}
		if _, err := os.Stat(filepath.Join(s.dir, PendingFile)); err == nil {
			problem("label set %s has an interrupted save (%s): open it once to finish the save first", rel(root, s.dir), PendingFile)
		}
	}

	for _, s := range sets {
		if s.target != "" && s.target != s.dir {
			report.Moves = append(report.Moves, NestMove{"label set", rel(root, s.dir), rel(root, s.target)})
		}
	}
	for _, p := range packs {
		if p.moveTarget != "" && p.moveTarget != p.move {
			what := "pack"
			if p.set != nil {
				what = "view"
			}
			report.Moves = append(report.Moves, NestMove{what, rel(root, p.move), rel(root, p.moveTarget)})
		}
		if p.set != nil && p.target != "" && relLink(p.target, p.set.target) != p.link.Path {
			report.Relinked = append(report.Relinked, rel(root, p.target))
		}
	}
	drafts, err := nestDrafts(root, packs, dryRun)
	if err != nil {
		return nil, err
	}
	report.Drafts = drafts
	if len(report.Problems) > 0 {
		return report, ErrNestRefused
	}
	if dryRun {
		return report, nil
	}

	// Sets first, so a view's move lands in its set's new folder. A view
	// still inside a set that moves travels with it, and moves from there.
	for _, s := range sets {
		if s.target == s.dir {
			continue
		}
		if err := renameInto(s.dir, s.target); err != nil {
			return report, err
		}
		for _, p := range packs {
			if within(p.move, s.dir) {
				p.move = filepath.Join(s.target, strings.TrimPrefix(p.move, s.dir+string(filepath.Separator)))
			}
		}
	}
	for _, p := range packs {
		if p.moveTarget == "" || p.move == p.moveTarget {
			continue
		}
		if err := renameInto(p.move, p.moveTarget); err != nil {
			return report, err
		}
	}
	for _, p := range packs {
		if p.set == nil || p.target == "" {
			continue
		}
		if err := annotation.WriteLabelSetLink(&annotation.Pack{Dir: p.target}, p.set.id, p.set.target); err != nil {
			return report, err
		}
	}
	if _, err := nestDrafts(root, packs, false); err != nil {
		return report, err
	}
	labels := filepath.Join(root, "labels")
	if entries, err := os.ReadDir(labels); err == nil && len(entries) == 0 {
		_ = os.Remove(labels)
	}
	return report, nil
}

// nestInventory finds every label set and pack under root, at the depths
// either old layout or the new one puts them, without descending into a
// set's head or a pack's own folders.
func nestInventory(root string) ([]*nestSet, []*nestPack, error) {
	var sets []*nestSet
	var packs []*nestPack
	skip := map[string]bool{JournalBaseDir: true, "masks": true, "domain-returns": true, "splits": true,
		"proposals": true, "physical-reference-revisions": true, "feature-proposal-revisions": true}
	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		if dir != root {
			if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err == nil {
				// The manifest alone: a set mid-merge is still a set here.
				b, err := os.ReadFile(filepath.Join(dir, ManifestFile))
				if err != nil {
					return fmt.Errorf("label set %s: %w", dir, err)
				}
				var m Manifest
				if err := strictUnmarshal(b, &m); err != nil {
					return fmt.Errorf("label set %s: %s: %w", dir, ManifestFile, err)
				}
				sets = append(sets, &nestSet{dir: dir, id: m.LabelSetID, name: m.Name, appliesTo: m.AppliesTo, merging: m.Merge != nil})
			} else if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err == nil {
				p, err := annotation.OpenPack(dir)
				if err != nil {
					return fmt.Errorf("pack %s: %w", dir, err)
				}
				link, err := annotation.ReadLabelSetLink(dir)
				if err != nil {
					return fmt.Errorf("pack %s: %w", dir, err)
				}
				packs = append(packs, &nestPack{dir: dir, manifest: p.Manifest, link: link})
				return nil
			}
		}
		if depth == 4 {
			return nil
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || skip[e.Name()] {
				continue
			}
			if err := walk(filepath.Join(dir, e.Name()), depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, 0); err != nil {
		return nil, nil, err
	}
	return sets, packs, nil
}

// nestDrafts points split drafts in root at the packs' new folders. A draft
// names a pack by "dir", relative to the draft. With dryRun it only reports
// which drafts would change.
func nestDrafts(root string, packs []*nestPack, dryRun bool) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(root, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var draft struct {
			Packs []struct {
				Dir string `json:"dir"`
			} `json:"packs"`
		}
		if json.Unmarshal(b, &draft) != nil || len(draft.Packs) == 0 {
			continue
		}
		next := b
		for _, p := range packs {
			if p.target == "" || p.target == p.dir {
				continue
			}
			from, to := rel(root, p.dir), rel(root, p.target)
			next = bytes.ReplaceAll(next, []byte(`"dir": `+strconv.Quote(from)), []byte(`"dir": `+strconv.Quote(to)))
		}
		if bytes.Equal(next, b) {
			continue
		}
		changed = append(changed, e.Name())
		if !dryRun {
			if err := os.WriteFile(path, next, 0o644); err != nil {
				return nil, err
			}
		}
	}
	return changed, nil
}

// uniqueDir is dir/name, or dir/name-2 and on when another move or a folder
// on disk already has it.
func uniqueDir(dir, name string, taken map[string]string) string {
	for n := 1; ; n++ {
		candidate := filepath.Join(dir, name)
		if n > 1 {
			candidate = filepath.Join(dir, fmt.Sprintf("%s-%d", name, n))
		}
		if _, ok := taken[candidate]; ok {
			continue
		}
		if _, err := os.Lstat(candidate); err == nil {
			continue
		}
		return candidate
	}
}

func renameInto(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("move %s to %s: %w", from, to, err)
	}
	return nil
}

func within(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

func rel(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(r)
}

func relLink(packDir, setDir string) string {
	r, err := filepath.Rel(packDir, setDir)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(r)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
