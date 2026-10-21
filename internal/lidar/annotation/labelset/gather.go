package labelset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// Gathering a local annotation tree into a dataset's labels/.
//
// Annotation work is meant to happen in the dataset clone itself, with the
// server's --lidar-annotation-dir pointed at <clone>/labels, so that what is
// labelled is what is committed and pushed. Gather moves a tree kept
// elsewhere, in either the old layout or the new, into that one:
//
//   - each label set to <into>/<capture>/<set>/, the capture folder named, when
//     a dataset is given, for the dataset capture whose raw file the set is
//     bound to by SHA-256, which is the folder the dataset already uses;
//   - each pack linked to a set into the set's views/, and each pack not in a
//     set into its capture's packs/;
//   - split files and split drafts into <into>, drafts pointed at the packs'
//     new folders.
//
// A set the dataset already holds, at the same head, stays where it is, and
// takes the local copy's history (its journal and journal base). A pack the
// dataset already holds, with the same tracked records, takes the local
// copy's working state (revision archives, proposal layers, facets). The
// local copies of both are moved aside into <from>/.gathered-<time>/, never
// deleted. Anything else already there, or a set at another head, refuses the
// whole run, as Nest does: every move is planned before any is made.
//
// Both trees must be on one file system, since every move is a rename. The
// tree at <into> must already be in the layout (run Nest on it first). Close
// every annotation window and stop the server first.

// GatherOptions configures Gather.
type GatherOptions struct {
	// Dataset is the dataset clone holding manifest.json, whose rows name
	// the capture folders. Empty names folders by capture file, as Nest does.
	Dataset string
	DryRun  bool
}

type gatherPack struct {
	*nestPack
	orig    string // where the pack was, before any move
	final   string // where the pack ends up: its own new folder or the dataset's copy
	adopt   string // the dataset's copy of this pack, when it has one
	entries []string
}

// Gather moves the annotation tree at from into into.
func Gather(from, into string, opts GatherOptions) (*NestReport, error) {
	var err error
	if from, err = filepath.Abs(from); err != nil {
		return nil, err
	}
	if into, err = filepath.Abs(into); err != nil {
		return nil, err
	}
	if from == into || within(into, from) || within(from, into) {
		return nil, fmt.Errorf("gather moves one tree into another: %s and %s overlap (use nest for one tree)", from, into)
	}
	report := &NestReport{Root: into, From: from}
	problem := func(format string, args ...any) {
		report.Problems = append(report.Problems, fmt.Sprintf(format, args...))
	}

	sets, packs, err := nestInventory(from)
	if err != nil {
		return nil, err
	}
	destSets, destPacks, err := nestInventory(into)
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
		s := byDir[filepath.Clean(p.link.LabelSetDir(p.dir))]
		switch {
		case s == nil:
			problem("%s links label set %s, which is not a label set under %s", rel(from, p.dir), p.link.Path, from)
		case s.id != p.link.LabelSetID:
			problem("%s links label set %s, %s holds %s", rel(from, p.dir), p.link.LabelSetID, rel(from, s.dir), s.id)
		default:
			p.set = s
		}
	}

	// The destination must be in the layout already: its packs in a set's
	// views/, a capture's packs/, or a clip job's folder.
	destByID := map[string]*nestSet{}
	for _, s := range destSets {
		destByID[s.id] = s
	}
	destByDigest := map[string][]string{}
	for _, p := range destPacks {
		if _, err := annotation.PackHandle(into, p.dir); err != nil {
			problem("%s is not in the layout: run 'velocity lidar labels nest --root %s' first", rel(into, p.dir), into)
			continue
		}
		destByDigest[p.manifest.PackDigest] = append(destByDigest[p.manifest.PackDigest], p.dir)
	}

	captureOf, err := datasetCaptures(opts.Dataset)
	if err != nil {
		return nil, err
	}
	stems := map[string]bool{}
	for _, stem := range captureOf {
		stems[stem] = true
	}

	taken := map[string]string{}
	claim := func(target, by string) {
		if other, ok := taken[target]; ok && other != by {
			problem("%s and %s would both go to %s", rel(from, other), rel(from, by), rel(into, target))
		}
		taken[target] = by
	}

	// Sets.
	adoptSet := map[*nestSet]*nestSet{}
	for _, s := range sets {
		capture := ""
		if captureOf != nil {
			found := map[string]bool{}
			for _, b := range s.appliesTo {
				if stem, ok := captureOf[b.SHA256]; ok {
					found[stem] = true
				}
			}
			switch len(found) {
			case 0:
				problem("label set %s is bound to no capture of the dataset: bind it to the dataset's raw file first ('velocity lidar labels bind', or stage-label-sets.py --capture-id)", rel(from, s.dir))
				continue
			case 1:
				for stem := range found {
					capture = stem
				}
			default:
				problem("label set %s is bound to %d captures of the dataset: %v", rel(from, s.dir), len(found), sortedKeys(found))
				continue
			}
		} else {
			capture = setCaptureName(from, s, packs)
			if capture == "" {
				problem("label set %s has no linked pack and no bound file to name its capture by; bind it first", rel(from, s.dir))
				continue
			}
		}
		target := filepath.Join(into, capture, filepath.Base(s.dir))
		if d := destByID[s.id]; d != nil {
			// The dataset holds this set already: it keeps its place, and
			// takes this copy's history if both are at one head.
			target = d.dir
			if err := sameHead(s.dir, d.dir); err != nil {
				problem("label set %s: %v", rel(from, s.dir), err)
			}
			for _, local := range []string{JournalFile, JournalBaseDir} {
				if _, err := os.Lstat(filepath.Join(d.dir, local)); err == nil {
					problem("the dataset's %s already has its own %s: it has been edited there; reconcile the two copies by hand", rel(into, d.dir), local)
				}
			}
			adoptSet[s] = d
		} else if _, err := os.Lstat(target); err == nil {
			problem("label set %s would go to %s, which holds something else", rel(from, s.dir), rel(into, target))
		}
		s.target = target
		claim(target, s.dir)
		if s.merging {
			problem("label set %s is in the middle of a Git merge: finish it first ('velocity lidar labels finish-merge')", rel(from, s.dir))
		}
		if _, err := os.Stat(filepath.Join(s.dir, PendingFile)); err == nil {
			problem("label set %s has an interrupted save (%s): open it once to finish the save first", rel(from, s.dir), PendingFile)
		}
	}

	// Packs.
	var gathered []*gatherPack
	for _, np := range packs {
		p := &gatherPack{nestPack: np, orig: np.dir}
		gathered = append(gathered, p)
		if copies := destByDigest[p.manifest.PackDigest]; len(copies) > 0 {
			// The dataset holds this pack already. It takes this copy's
			// working state if their records agree.
			if len(copies) > 1 {
				problem("the dataset holds pack %s %d times: %v", p.manifest.PackDigest, len(copies), copies)
				continue
			}
			p.adopt, p.final = copies[0], copies[0]
			if p.set != nil && adoptSet[p.set] == nil {
				problem("the dataset holds pack %s as %s but not its label set %s", p.manifest.PackDigest, rel(into, p.adopt), rel(from, p.set.dir))
			} else if p.set != nil && p.set.target != "" && !within(p.adopt, p.set.target) && !within(p.adopt, filepath.Dir(p.set.target)) {
				problem("%s is linked to %s, and the dataset's copy %s is not in that set's capture folder", rel(from, p.dir), rel(from, p.set.dir), rel(into, p.adopt))
			}
			for _, name := range annotation.PackRecordFiles() {
				a, aerr := os.ReadFile(filepath.Join(p.dir, name))
				b, berr := os.ReadFile(filepath.Join(p.adopt, name))
				if errors.Is(aerr, os.ErrNotExist) && errors.Is(berr, os.ErrNotExist) {
					continue
				}
				if name == annotation.FeatureProposalsFile && errors.Is(berr, os.ErrNotExist) {
					// Facets are not published: the local file is working state.
					p.entries = append(p.entries, name)
					continue
				}
				if !bytes.Equal(a, b) {
					problem("%s and the dataset's %s hold different %s: reconcile them by hand", rel(from, p.dir), rel(into, p.adopt), name)
				}
			}
			entries, err := os.ReadDir(p.dir)
			if err != nil {
				return nil, err
			}
			for _, e := range entries {
				// Locks are held only while a save runs: nothing to carry.
				if !annotation.IsPackLocalEntry(e.Name()) || e.Name() == ".DS_Store" || e.Name() == ".annotations.lock" || strings.HasSuffix(e.Name(), ".tmp") {
					continue
				}
				if _, err := os.Lstat(filepath.Join(p.adopt, e.Name())); err == nil {
					continue // the dataset's copy has its own; this one is set aside
				}
				p.entries = append(p.entries, e.Name())
			}
			continue
		}
		switch {
		case p.set != nil && p.set.target != "":
			views := filepath.Join(p.set.target, annotation.ViewsDir)
			if filepath.Base(filepath.Dir(p.dir)) == annotation.ViewsDir {
				p.final = filepath.Join(views, filepath.Base(p.dir))
			} else {
				p.final = uniqueDir(views, annotation.ViewName(p.manifest), taken)
			}
			p.move, p.moveTarget = p.dir, p.final
		case p.set != nil:
			continue // its set is refused; the problem is reported
		case annotation.IsClipPackDir(p.dir):
			attempt := filepath.Dir(p.dir)
			p.move, p.moveTarget = attempt, filepath.Join(into, filepath.Base(attempt))
			p.final = filepath.Join(p.moveTarget, filepath.Base(p.dir))
		default:
			capture := annotation.CaptureName(p.manifest)
			if captureOf != nil && !stems[capture] {
				problem("%s is a pack of %s, which the dataset does not hold", rel(from, p.dir), p.manifest.Source.PCAPBasename)
				continue
			}
			p.final = filepath.Join(into, capture, annotation.PacksDir, filepath.Base(p.dir))
			p.move, p.moveTarget = p.dir, p.final
		}
		claim(p.moveTarget, p.move)
		if _, err := os.Lstat(p.moveTarget); err == nil {
			problem("%s would go to %s, which exists", rel(from, p.move), rel(into, p.moveTarget))
		}
	}

	// Split files and drafts.
	splitMoves, err := gatherSplits(from, into, problem)
	if err != nil {
		return nil, err
	}
	draftMoves, err := gatherDrafts(from, into, problem)
	if err != nil {
		return nil, err
	}

	archive := filepath.Join(from, ".gathered-"+time.Now().UTC().Format("20060102-150405"))
	for _, s := range sets {
		if s.target == "" {
			continue
		}
		if d := adoptSet[s]; d != nil {
			report.Adopted = append(report.Adopted, NestMove{"label set history", rel(from, s.dir), rel(into, d.dir)})
		} else {
			report.Moves = append(report.Moves, NestMove{"label set", rel(from, s.dir), rel(into, s.target)})
		}
	}
	for _, p := range gathered {
		switch {
		case p.adopt != "":
			report.Adopted = append(report.Adopted, NestMove{"pack working state " + strings.Join(p.entries, ", "), rel(from, p.dir), rel(into, p.adopt)})
		case p.moveTarget != "":
			what := "pack"
			if p.set != nil {
				what = "view"
			}
			report.Moves = append(report.Moves, NestMove{what, rel(from, p.move), rel(into, p.moveTarget)})
		}
	}
	report.Moves = append(report.Moves, splitMoves...)
	for _, d := range draftMoves {
		report.Drafts = append(report.Drafts, d.name)
	}
	if len(report.Problems) > 0 {
		return report, ErrNestRefused
	}
	report.Archive = rel(from, archive)
	if opts.DryRun {
		return report, nil
	}

	// Sets first, so a view inside a set travels with it.
	for _, s := range sets {
		if s.target == "" {
			continue
		}
		if d := adoptSet[s]; d != nil {
			for _, local := range []string{JournalFile, JournalBaseDir} {
				if _, err := os.Lstat(filepath.Join(s.dir, local)); err == nil {
					if err := renameInto(filepath.Join(s.dir, local), filepath.Join(d.dir, local)); err != nil {
						return report, err
					}
				}
			}
			continue // the rest of the local copy goes aside with the packs it holds
		}
		if err := renameInto(s.dir, s.target); err != nil {
			return report, err
		}
		for _, p := range gathered {
			if p.move != "" && within(p.move, s.dir) {
				p.move = filepath.Join(s.target, strings.TrimPrefix(p.move, s.dir+string(filepath.Separator)))
			}
		}
	}
	for _, p := range gathered {
		if p.adopt != "" {
			for _, name := range p.entries {
				if err := renameInto(filepath.Join(p.dir, name), filepath.Join(p.adopt, name)); err != nil {
					return report, err
				}
			}
			// A view inside a set whose history was adopted goes aside with
			// that set's local copy, below.
			if p.set != nil && adoptSet[p.set] != nil && within(p.dir, p.set.dir) {
				continue
			}
			if err := renameInto(p.dir, filepath.Join(archive, rel(from, p.dir))); err != nil {
				return report, err
			}
			continue
		}
		if p.moveTarget != "" && p.move != p.moveTarget {
			if err := renameInto(p.move, p.moveTarget); err != nil {
				return report, err
			}
		}
	}
	for _, s := range sets {
		if adoptSet[s] != nil {
			if err := renameInto(s.dir, filepath.Join(archive, rel(from, s.dir))); err != nil {
				return report, err
			}
		}
	}
	for _, p := range gathered {
		if p.adopt == "" && p.set != nil && p.final != "" {
			if err := annotation.WriteLabelSetLink(&annotation.Pack{Dir: p.final}, p.set.id, p.set.target); err != nil {
				return report, err
			}
		}
	}
	for _, m := range splitMoves {
		src, dst := filepath.Join(from, filepath.FromSlash(m.From)), filepath.Join(into, filepath.FromSlash(m.To))
		if m.What == "split (already there)" {
			dst = filepath.Join(archive, filepath.FromSlash(m.From))
		}
		if err := renameInto(src, dst); err != nil {
			return report, err
		}
	}
	for _, d := range draftMoves {
		b, err := os.ReadFile(d.from)
		if err != nil {
			return report, err
		}
		for _, p := range gathered {
			if p.final == "" {
				continue
			}
			old, now := rel(from, p.orig), rel(into, p.final)
			b = bytes.ReplaceAll(b, []byte(`"dir": `+strconv.Quote(old)), []byte(`"dir": `+strconv.Quote(now)))
		}
		if err := os.WriteFile(d.to, b, 0o644); err != nil {
			return report, err
		}
		if err := renameInto(d.from, filepath.Join(archive, d.name)); err != nil {
			return report, err
		}
	}
	for _, dir := range []string{filepath.Join(from, "labels"), filepath.Join(from, "splits")} {
		if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
			_ = os.Remove(dir)
		}
	}
	return report, nil
}

// datasetCaptures maps each raw file SHA-256 in a dataset's manifest.json to
// its capture folder name: the raw file's name without its extension. It
// returns nil when no dataset is given.
func datasetCaptures(dataset string) (map[string]string, error) {
	if dataset == "" {
		return nil, nil
	}
	b, err := os.ReadFile(filepath.Join(dataset, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("dataset manifest: %w", err)
	}
	var rows []struct {
		RawPath   string `json:"raw_path"`
		RawSHA256 string `json:"raw_sha256"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, fmt.Errorf("dataset manifest: %w", err)
	}
	out := map[string]string{}
	for _, r := range rows {
		if r.RawSHA256 != "" && r.RawPath != "" {
			out[r.RawSHA256] = annotation.CaptureNameOf(path.Base(r.RawPath))
		}
	}
	return out, nil
}

// setCaptureName is the capture folder Nest would give a set: the one it is
// in, or, for a set in the old labels/ folder, the one its packs or bound
// files name. Empty when nothing names one, or more than one does.
func setCaptureName(root string, s *nestSet, packs []*nestPack) string {
	parent := filepath.Dir(s.dir)
	if filepath.Base(parent) != "labels" || filepath.Dir(parent) != root {
		return filepath.Base(parent)
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
	if len(captures) != 1 {
		return ""
	}
	for c := range captures {
		return c
	}
	return ""
}

// sameHead refuses two copies of a label set at different heads.
func sameHead(a, b string) error {
	read := func(dir string) (Head, error) {
		raw, err := os.ReadFile(filepath.Join(dir, ManifestFile))
		if err != nil {
			return Head{}, err
		}
		var m Manifest
		if err := strictUnmarshal(raw, &m); err != nil {
			return Head{}, err
		}
		return m.Head, nil
	}
	ha, err := read(a)
	if err != nil {
		return err
	}
	hb, err := read(b)
	if err != nil {
		return err
	}
	if ha.Revision != hb.Revision || ha.State != hb.State {
		return fmt.Errorf("the local copy is at revision %d (%s) and the dataset's at %d (%s): publish the local edits (stage-label-sets.py --replace) or reconcile the two by hand", ha.Revision, ha.State, hb.Revision, hb.State)
	}
	return nil
}

// gatherSplits plans moving <from>/splits/* into <into>/splits/. A split
// already there with the same bytes is set aside instead; one with other
// bytes is a problem.
func gatherSplits(from, into string, problem func(string, ...any)) ([]NestMove, error) {
	entries, err := os.ReadDir(filepath.Join(from, "splits"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []NestMove
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		src := filepath.Join(from, "splits", e.Name())
		dst := filepath.Join(into, "splits", e.Name())
		m := NestMove{"split", rel(from, src), rel(into, dst)}
		if have, err := os.ReadFile(dst); err == nil {
			mine, err := os.ReadFile(src)
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(have, mine) {
				problem("split %s differs from the dataset's %s", rel(from, src), rel(into, dst))
				continue
			}
			m.What = "split (already there)"
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out, nil
}

type gatherDraft struct{ name, from, to string }

// gatherDrafts plans moving split drafts (top-level *.json naming packs by
// "dir") from <from> to <into>.
func gatherDrafts(from, into string, problem func(string, ...any)) ([]gatherDraft, error) {
	entries, err := os.ReadDir(from)
	if err != nil {
		return nil, err
	}
	var out []gatherDraft
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		src := filepath.Join(from, e.Name())
		b, err := os.ReadFile(src)
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
		dst := filepath.Join(into, e.Name())
		if _, err := os.Lstat(dst); err == nil {
			problem("split draft %s would go to %s, which exists", e.Name(), rel(into, dst))
			continue
		}
		out = append(out, gatherDraft{name: e.Name(), from: src, to: dst})
	}
	return out, nil
}
