package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func git(ctx context.Context, root string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	c.Env = append(os.Environ(), "GIT_LFS_SKIP_SMUDGE=1", "GIT_TERMINAL_PROMPT=0")
	b, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}

// GitStatus reports local changes and refuses work on uncommitted or non-main
// source. No fetch, checkout, reset, push or commit occurs in this checkout.
func GitStatus(ctx context.Context, root string) (string, error) {
	status, err := git(ctx, root, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return "", err
	}
	if status != "" {
		return "", fmt.Errorf("checkout has uncommitted changes; check in source before running:\n%s", status)
	}
	sha, err := git(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	main, err := git(ctx, root, "rev-parse", "refs/heads/main")
	if err != nil {
		return "", err
	}
	if sha != main {
		return "", fmt.Errorf("HEAD %s is not local main %s", sha, main)
	}
	return sha, nil
}

// build stages immutable source locally, using make and only cached dependencies.
func build(ctx context.Context, root, evidence, sha string) (string, string, error) {
	dir := filepath.Join(evidence, "builds", sha)
	src := filepath.Join(dir, "src")
	bin := filepath.Join(dir, "bin")
	var stamp struct {
		SHA      string
		Binaries map[string]string
	}
	if readJSON(filepath.Join(dir, "build.json"), &stamp) == nil && stamp.SHA == sha {
		good := len(stamp.Binaries) == 3
		for name, want := range stamp.Binaries {
			got, e := hashFile(filepath.Join(bin, name))
			good = good && e == nil && got == want
		}
		if good {
			actual, e := git(ctx, src, "rev-parse", "HEAD")
			if e == nil && actual == sha {
				if _, e = git(ctx, src, "diff", "--quiet", "HEAD", "--"); e != nil {
					return "", "", fmt.Errorf("cached source has tracked changes: %w", e)
				}
				return src, bin, nil
			}
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", "", err
	}
	if _, err := os.Stat(src); os.IsNotExist(err) {
		if _, err = git(ctx, root, "worktree", "add", "--detach", src, sha); err != nil {
			return "", "", err
		}
	}
	actual, err := git(ctx, src, "rev-parse", "HEAD")
	if err != nil || actual != sha {
		return "", "", fmt.Errorf("cached worktree does not match %s", sha)
	}
	if _, e := git(ctx, src, "diff", "--quiet", "HEAD", "--"); e != nil {
		return "", "", fmt.Errorf("cached source has tracked changes: %w", e)
	}
	c := toolCommand(ctx, src, "make", []string{"build-experiment-tools", "EXPERIMENT_TOOLS_DIR=" + bin})
	c.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	log, err := os.Create(filepath.Join(dir, "build.log"))
	if err != nil {
		return "", "", err
	}
	defer log.Close()
	c.Stdout = log
	c.Stderr = log
	if err = c.Run(); err != nil {
		return "", "", fmt.Errorf("offline tool build: %w (see %s)", err, log.Name())
	}
	version, err := buildVersion(src)
	if err != nil {
		return "", "", err
	}
	stamp.Binaries = map[string]string{}
	for _, name := range []string{"lidar-state-estimation-baseline", "lidar-box-eval", "lidar-ground-truth-eval"} {
		path := filepath.Join(bin, name)
		output, e := exec.CommandContext(ctx, path, "-version").CombinedOutput()
		if e != nil || !strings.Contains(string(output), "v"+version) || !strings.Contains(string(output), "git sha:  "+sha) {
			return "", "", fmt.Errorf("%s has a missing or incorrect build stamp", name)
		}
		sum, e := hashFile(path)
		if e != nil {
			return "", "", e
		}
		stamp.Binaries[name] = sum
	}
	stamp.SHA = sha
	if err = atomicJSON(filepath.Join(dir, "build.json"), stamp); err != nil {
		return "", "", err
	}
	return src, bin, nil
}

// syncMain updates only a clean local main branch, using an ordinary
// fast-forward. An unavailable network leaves the local queue usable.
func syncMain(ctx context.Context, root string) error {
	branch, e := git(ctx, root, "symbolic-ref", "--short", "HEAD")
	if e != nil || branch != "main" {
		return fmt.Errorf("connected sync requires the main branch checked out")
	}
	if _, e = git(ctx, root, "fetch", "--no-tags", "origin", "main"); e != nil {
		return e
	}
	_, e = git(ctx, root, "merge", "--ff-only", "--no-edit", "origin/main")
	return e
}

var makefileVersion = regexp.MustCompile(`(?m)^VERSION\s*:?=\s*(\S+)`)

// buildVersion is the version the Makefile stamps into every binary. A VERSION
// file, when a checkout carries one, takes precedence.
func buildVersion(src string) (string, error) {
	if b, err := os.ReadFile(filepath.Join(src, "VERSION")); err == nil {
		return trim(string(b)), nil
	}
	b, err := os.ReadFile(filepath.Join(src, "Makefile"))
	if err != nil {
		return "", fmt.Errorf("no build version in %s: %w", src, err)
	}
	m := makefileVersion.FindSubmatch(b)
	if m == nil {
		return "", fmt.Errorf("no VERSION file or Makefile VERSION in %s", src)
	}
	return string(m[1]), nil
}
