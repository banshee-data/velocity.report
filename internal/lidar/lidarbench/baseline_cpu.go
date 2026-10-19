//go:build pcap

package lidarbench

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

// Hosted runners of one class are given different CPU generations from job to
// job, and kirk0's full profile has run 7.5 s on the fastest and 14.4 s on
// the slowest for identical work. One baseline per host class either fails a
// run for its runner or hides a regression behind it, so a baseline can also
// be kept per CPU model, beside the class one: baseline-kirk0-full-ci.json is
// the class baseline, and baseline-kirk0-full-ci-amd-epyc-7763.json the one
// for that CPU. A comparison uses the per-CPU baseline when one exists and
// measured the same workload, and the class baseline otherwise.

var (
	// cpuSlugNoise is what a CPU model carries besides the part that names
	// it: trademark marks, core counts, "Processor", "CPU", a clock speed
	// and a board revision.
	cpuSlugNoise = regexp.MustCompile(`\((r|tm|c)\)|\b\d+-core\b|\bprocessor\b|\bcpu\b|@.*$|\brev\b.*$`)
	cpuSlugSep   = regexp.MustCompile(`[^a-z0-9]+`)
)

// cpuSlug turns a CPU model into a file-name suffix: "AMD EPYC 7763 64-Core
// Processor" becomes "amd-epyc-7763", and "Raspberry Pi 5 Model B Rev 1.0"
// "raspberry-pi-5-model-b". An unnamed CPU gives "".
func cpuSlug(model string) string {
	s := cpuSlugNoise.ReplaceAllString(strings.ToLower(model), " ")
	return strings.Trim(cpuSlugSep.ReplaceAllString(s, "-"), "-")
}

// perCPUBaselinePath is path with the CPU's slug before its extension, or ""
// when the CPU is unnamed.
func perCPUBaselinePath(path, cpuModel string) string {
	slug := cpuSlug(cpuModel)
	if slug == "" {
		return ""
	}
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + "-" + slug + ext
}

// baselineChoice is the baseline a comparison reads, and why that one.
type baselineChoice struct {
	Path     string
	Baseline *BenchmarkResult
	// Identity is non-nil when Baseline did not measure the same workload as
	// the current run, which refuses the comparison.
	Identity error
	Notice   string
}

// chooseBaseline picks the baseline to compare current against, given the
// class baseline's path: the per-CPU baseline beside it when one exists for
// this run's CPU and measured the same workload, otherwise the class
// baseline. A per-CPU baseline from an earlier tuning falls back rather than
// refusing, since the class baseline may already have been captured again. An
// unreadable baseline is an error.
func chooseBaseline(classPath string, current *BenchmarkResult, tolerance float64) (baselineChoice, error) {
	model := current.SystemInfo.CPUModel
	class := filepath.Base(classPath)
	var notice string
	if cpuPath := perCPUBaselinePath(classPath, model); cpuPath == "" {
		notice = fmt.Sprintf("this runner's CPU is not named; comparing against the class baseline %s", class)
	} else {
		b, err := readBaseline(cpuPath)
		switch {
		case err == nil:
			idErr := checkWorkloadIdentity(b, current, tolerance)
			if idErr == nil {
				return baselineChoice{Path: cpuPath, Baseline: b,
					Notice: fmt.Sprintf("comparing against %s, captured on this runner's CPU (%s)", filepath.Base(cpuPath), model)}, nil
			}
			notice = fmt.Sprintf("%s measured a different workload (%v); comparing against the class baseline %s",
				filepath.Base(cpuPath), idErr, class)
		case errors.Is(err, fs.ErrNotExist):
			notice = fmt.Sprintf("no baseline for this runner's CPU (%s) at %s; comparing against the class baseline %s",
				model, filepath.Base(cpuPath), class)
		default:
			return baselineChoice{}, err
		}
	}
	b, err := readBaseline(classPath)
	if err != nil {
		return baselineChoice{}, err
	}
	return baselineChoice{Path: classPath, Baseline: b,
		Identity: checkWorkloadIdentity(b, current, tolerance), Notice: notice}, nil
}
