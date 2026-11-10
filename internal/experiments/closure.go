package experiments

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var closureDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func closureGit(ctx context.Context, root string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	b, e := c.CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("closure git check: %w: %s", e, strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}

var prClosure = regexp.MustCompile(`^#([1-9][0-9]*)$`)
var decisionClosure = regexp.MustCompile(`^D-[0-9]+$`)

// CheckClosure verifies a manual step closure against local main and repository artefacts.
func CheckClosure(ctx context.Context, root string, sp *Spec, st Step) error {
	a := st.Closed
	if m := prClosure.FindStringSubmatch(a); m != nil {
		log, e := closureGit(ctx, root, "log", "refs/heads/main", "--format=%s", "--fixed-strings", "--grep=(#"+m[1]+")")
		if e != nil {
			return e
		}
		if log == "" {
			return fmt.Errorf("%s closure %s absent from local main squash history", st.ID, a)
		}
		return nil
	}
	switch st.Kind {
	case "manual:implement":
		if !regexp.MustCompile(`^[0-9a-f]{7,40}$`).MatchString(a) {
			return fmt.Errorf("%s needs a checked-in commit or #PR closure", st.ID)
		}
		_, e := closureGit(ctx, root, "merge-base", "--is-ancestor", a, "refs/heads/main")
		return e
	case "manual:decide":
		raw, e := os.ReadFile(filepath.Join(root, "docs", "DECISIONS.md"))
		if e != nil {
			return e
		}
		if !decisionClosure.MatchString(a) || !regexp.MustCompile(`\b`+regexp.QuoteMeta(a)+`\b`).Match(raw) {
			return fmt.Errorf("unknown decision closure %s", a)
		}
	case "manual:write":
		return RepoArtefacts(root, sp)(st.Kind, a)
	default:
		if !closureDigest.MatchString(a) {
			return fmt.Errorf("%s needs a sha256 digest closure", st.ID)
		}
	}
	return nil
}
