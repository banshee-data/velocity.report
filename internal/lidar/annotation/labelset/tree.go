package labelset

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// Working over a whole annotation tree: every label set in its capture
// folder and every view in its views/ folder (annotation/layout.go).

// TreeSets lists the label sets of a tree, <root>/<capture>/<set>/, sorted.
func TreeSets(root string) ([]string, error) {
	captures, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range captures {
		if !c.IsDir() || strings.HasPrefix(c.Name(), ".") || c.Name() == "splits" {
			continue
		}
		sets, err := os.ReadDir(filepath.Join(root, c.Name()))
		if err != nil {
			continue
		}
		for _, s := range sets {
			dir := filepath.Join(root, c.Name(), s.Name())
			if s.IsDir() && fileExists(filepath.Join(dir, ManifestFile)) {
				out = append(out, dir)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// SetViews lists the views in a label set's views/ folder, sorted.
func SetViews(setDir string) []string {
	entries, err := os.ReadDir(filepath.Join(setDir, annotation.ViewsDir))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		dir := filepath.Join(setDir, annotation.ViewsDir, e.Name())
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && fileExists(filepath.Join(dir, "manifest.json")) {
			out = append(out, dir)
		}
	}
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// TreeReport is what CheckTree found.
type TreeReport struct {
	Root     string        `json:"root"`
	Sets     []string      `json:"sets"`
	Views    []*ViewReport `json:"views"`
	Problems []string      `json:"problems,omitempty"`
}

// CheckTree checks a tree as a dataset publishes it: every label set opens
// (none is mid-merge), every view holds as a fresh copy (CheckView), and no
// pack sits anywhere but in a set's views/.
func CheckTree(root string) (*TreeReport, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	report := &TreeReport{Root: root}
	sets, err := TreeSets(root)
	if err != nil {
		return nil, err
	}
	inViews := map[string]bool{}
	for _, set := range sets {
		r := rel(root, set)
		report.Sets = append(report.Sets, r)
		if _, err := Open(set); err != nil {
			report.Problems = append(report.Problems, fmt.Sprintf("%s: %v", r, err))
			continue
		}
		for _, view := range SetViews(set) {
			inViews[view] = true
			v, err := CheckView(view)
			if v != nil {
				report.Views = append(report.Views, v)
			}
			if err != nil {
				report.Problems = append(report.Problems, fmt.Sprintf("%s: %v", rel(root, view), err))
				if v != nil {
					for _, p := range v.Problems {
						report.Problems = append(report.Problems, fmt.Sprintf("%s: %s", rel(root, view), p))
					}
				}
			}
		}
	}
	// Every pack, wherever it sits: one beside its set, as datasets held
	// them before the layout, is found too.
	_, packs, err := nestInventory(root)
	if err != nil {
		return nil, err
	}
	for _, p := range packs {
		if !inViews[p.dir] {
			report.Problems = append(report.Problems, fmt.Sprintf("%s is a pack outside any label set's views/: a dataset publishes views only (run 'velocity lidar labels nest')", rel(root, p.dir)))
		}
	}
	if len(report.Problems) > 0 {
		return report, fmt.Errorf("%s: %d problem(s)", root, len(report.Problems))
	}
	return report, nil
}

// FinishedMerge is one set FinishMerges finished.
type FinishedMerge struct {
	Set      string        `json:"set"`
	Result   *ReheadResult `json:"result"`
	Repinned int           `json:"repinned"`
	Drifted  int           `json:"drifted"`
	Views    []*ViewReport `json:"views,omitempty"`
}

// FinishMerges finishes every label set in a tree that records a merge
// (FinishMerge), re-pins its views' reviews where they hold, and checks each
// view as a fresh copy would hold it. It returns the sets it finished and
// the files a commit of the merge must take: each finished set's folder and
// its views' physical references. A view that does not hold is an error,
// after every set has been finished.
func FinishMerges(root string, change Change) ([]FinishedMerge, []string, error) {
	sets, err := TreeSets(root)
	if err != nil {
		return nil, nil, err
	}
	var done []FinishedMerge
	var paths, problems []string
	for _, set := range sets {
		result, err := FinishMerge(set, change)
		if err != nil {
			return done, paths, fmt.Errorf("%s: %w", set, err)
		}
		if result == nil {
			continue
		}
		f := FinishedMerge{Set: set, Result: result}
		paths = append(paths, set)
		for _, view := range SetViews(set) {
			p, err := annotation.OpenPack(view)
			if err != nil {
				return done, paths, fmt.Errorf("%s: %w", view, err)
			}
			n, d, err := annotation.RepinUnchangedReviews(p, change.Author, change.Session)
			if err != nil {
				return done, paths, fmt.Errorf("re-pin %s: %w", view, err)
			}
			f.Repinned += n
			f.Drifted += d
			paths = append(paths, filepath.Join(view, annotation.PhysicalReferencesFile))
			v, err := CheckView(view)
			if v != nil {
				f.Views = append(f.Views, v)
			}
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", view, err))
			}
		}
		done = append(done, f)
	}
	if len(problems) > 0 {
		return done, paths, fmt.Errorf("merged, but %d view(s) do not hold: %s", len(problems), strings.Join(problems, "; "))
	}
	return done, paths, nil
}
