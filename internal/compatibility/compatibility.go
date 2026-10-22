// Package compatibility makes surviving external names explicit and reviewable.
// The policy and removal dates live in docs/platform/operations/compatibility-register.md.
package compatibility

import (
	"flag"
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

// Entry is one row in the compatibility register.
type Entry struct {
	Name     string
	RemoveIn string
}

// ReadRegister reads the register's five-column Markdown table. Other tables are ignored.
func ReadRegister(markdown string) ([]Entry, error) {
	var entries []Entry
	inRegister := false
	for _, line := range strings.Split(markdown, "\n") {
		if line == "## Register" {
			inRegister = true
			continue
		}
		if inRegister && strings.HasPrefix(line, "## ") {
			break
		}
		if !inRegister || !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) != 7 {
			return nil, fmt.Errorf("invalid compatibility row: %s", line)
		}
		name := strings.TrimSpace(cells[1])
		if name == "Surviving name" || strings.HasPrefix(name, "---") {
			continue
		}
		removal := strings.TrimSpace(cells[5])
		if name == "" || removal == "" {
			return nil, fmt.Errorf("incomplete compatibility row: %s", line)
		}
		if strings.HasPrefix(removal, "v") && len(removal) > 1 && removal[1] >= '0' && removal[1] <= '9' && !semver.IsValid(removal) {
			return nil, fmt.Errorf("invalid removal release %q", removal)
		}
		entries = append(entries, Entry{Name: name, RemoveIn: removal})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("compatibility register is empty")
	}
	return entries, nil
}

// CheckExpiry fails at the removal release, including its prereleases: the old
// name must be removed before the release is built, rather than after publishing.
func CheckExpiry(entries []Entry, version string) error {
	version = "v" + strings.TrimPrefix(strings.TrimSpace(version), "v")
	if !semver.IsValid(version) {
		return fmt.Errorf("invalid current release %q", version)
	}
	current := strings.Split(strings.Split(version, "-")[0], "+")[0]
	for _, entry := range entries {
		if semver.IsValid(entry.RemoveIn) && semver.Compare(current, entry.RemoveIn) >= 0 {
			return fmt.Errorf("compatibility name %s expired in %s (current %s)", entry.Name, entry.RemoveIn, version)
		}
	}
	return nil
}

// Registered reports whether an exact code spelling occurs in a surviving-name cell.
func Registered(entries []Entry, name string) bool {
	for _, entry := range entries {
		if strings.Contains(entry.Name, "`"+name+"`") {
			return true
		}
	}
	return false
}

// AliasRoute marks an old HTTP pattern and its canonical replacement. A source
// test checks both the register row and the matching canonical handler.
func AliasRoute(old, canonical string) string { return old }

// TombstoneFlag reserves an old value-taking flag as a refusal. Parsing stops
// with the replacement instruction; it never sets a value or runs the old work.
func TombstoneFlag(fs *flag.FlagSet, old, replacement string) {
	fs.Func(old, "removed; use "+replacement, func(string) error {
		return fmt.Errorf("-%s was removed; use %s", old, replacement)
	})
}
