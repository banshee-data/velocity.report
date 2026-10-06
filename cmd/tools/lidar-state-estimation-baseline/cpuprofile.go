package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
)

// cpuProfilePath is where -cpuprofile-dir puts a case's profile. Only the
// repeat replay is profiled: it writes no evidence database, so the profile
// is the pipeline's own cost rather than SQLite's or the capture digests'.
func cpuProfilePath(dir, caseID string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, caseID+".repeat.cpu.pprof")
}

// profiled runs fn under a pprof CPU profile written to path, or runs it
// bare when path is empty. An existing file is refused, never overwritten,
// as the tool treats its other outputs. fn's error wins over a close error.
func profiled(path string, fn func() error) error {
	if path == "" {
		return fn()
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("cpu profile: %w", err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		_ = f.Close()
		return fmt.Errorf("cpu profile: %w", err)
	}
	runErr := fn()
	pprof.StopCPUProfile()
	if err := f.Close(); err != nil && runErr == nil {
		return fmt.Errorf("cpu profile: %w", err)
	}
	return runErr
}
