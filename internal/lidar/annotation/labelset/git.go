package labelset

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// Reading a label set as a Git revision holds it.

// gitOutput runs git in repo and returns its standard output.
func gitOutput(repo string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// ExportRevision writes the label set at setPath (relative to the repository
// root, forward slashes) as revision rev of the repository at repo holds it,
// into dest, which must not exist. Files go through the repository's filters,
// so an LFS pointer arrives as its content. A set's views/ are left out.
func ExportRevision(repo, rev, setPath, dest string) error {
	setPath = strings.Trim(path.Clean(filepath.ToSlash(setPath)), "/")
	listing, err := gitOutput(repo, "ls-tree", "-r", "-z", "--name-only", rev, "--", setPath+"/")
	if err != nil {
		return err
	}
	var names []string
	for _, name := range strings.Split(string(listing), "\x00") {
		if name == "" {
			continue
		}
		rel := strings.TrimPrefix(name, setPath+"/")
		if rel == name || rel == annotation.ViewsDir || strings.HasPrefix(rel, annotation.ViewsDir+"/") {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return fmt.Errorf("%s holds no label set at %s", setPath, rev)
	}
	if err := os.Mkdir(dest, 0o755); err != nil {
		return err
	}
	for _, name := range names {
		b, err := gitOutput(repo, "cat-file", "--filters", rev+":"+name)
		if err != nil {
			return err
		}
		out := filepath.Join(dest, filepath.FromSlash(strings.TrimPrefix(name, setPath+"/")))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// OpenRevision opens the label set at setPath as revision rev holds it, from
// a copy in a temporary directory, which the returned function removes.
func OpenRevision(repo, rev, setPath string) (*LabelSet, func(), error) {
	tmp, err := os.MkdirTemp("", "labels-at-")
	if err != nil {
		return nil, nil, err
	}
	done := func() { os.RemoveAll(tmp) }
	dir := filepath.Join(tmp, "set")
	if err := ExportRevision(repo, rev, setPath, dir); err != nil {
		done()
		return nil, nil, err
	}
	ls, err := Open(dir)
	if err != nil {
		done()
		return nil, nil, fmt.Errorf("%s at %s: %w", setPath, rev, err)
	}
	return ls, done, nil
}
