package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GuardNotes enforces the run-note size, provenance, path and write-once rules.
// base is an optional Git base revision; harnessBranch restricts every changed
// path to a newly added note. It performs no network access.
func GuardNotes(ctx context.Context, root, base string, harnessBranch bool) error {
	files, e := git(ctx, root, "ls-files", "--", "data/experiments/runs/")
	if e != nil {
		return e
	}
	for _, p := range strings.Split(files, "\n") {
		if p == "" {
			continue
		}
		b, e := os.ReadFile(filepath.Join(root, p))
		if e != nil {
			return e
		}
		h, e := CheckNote(b)
		if e != nil {
			return fmt.Errorf("%s: %w", p, e)
		}
		if p != "data/experiments/runs/"+h.Slug+"/"+h.RunID+".md" {
			return fmt.Errorf("run note path disagrees with its header: %s", p)
		}
	}
	if base == "" {
		return nil
	}
	// NUL delimiters prevent unusual Git filenames escaping the path checks.
	c := exec.CommandContext(ctx, "git", "-C", root, "diff", "--name-status", "-z", "--no-renames", base+"...HEAD")
	raw, e := c.Output()
	if e != nil {
		return e
	}
	parts := strings.Split(string(raw), "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		status, p := parts[i], parts[i+1]
		note := strings.HasPrefix(p, "data/experiments/runs/")
		if harnessBranch && (!note || status != "A") {
			return fmt.Errorf("harness branches may add only new run notes: %s %s", status, p)
		}
		if note && status != "A" {
			return fmt.Errorf("run notes are write-once: %s %s", status, p)
		}
	}
	return nil
}
