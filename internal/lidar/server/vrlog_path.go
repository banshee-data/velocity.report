package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/security"
)

// defaultVRLogDir is used when no VRLOG directory is configured.
const defaultVRLogDir = "/var/lib/velocity-report"

// vrlogDir is the directory recordings must lie in (--lidar-vrlog-dir).
func (ws *Server) vrlogDir() string {
	if ws.vrlogSafeDir != "" {
		return ws.vrlogSafeDir
	}
	return defaultVRLogDir
}

// vrlogPathError says why a recording cannot be opened, with the HTTP
// status that fits: 400 for a path the caller got wrong, 404 for a recording
// that is not there, 503 for a VRLOG directory the server cannot read, which
// is usually a volume that is not mounted.
type vrlogPathError struct {
	Status int
	Msg    string
}

func (e *vrlogPathError) Error() string { return e.Msg }

// checkVRLogPath resolves a recording's path inside the VRLOG directory,
// following symlinks, and checks it is there. It returns the canonical path
// to open, or a *vrlogPathError naming the path, the directory and the
// cause.
func (ws *Server) checkVRLogPath(path string) (string, error) {
	dir := ws.vrlogDir()
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return "", &vrlogPathError{http.StatusBadRequest, fmt.Sprintf("vrlog_path must be an absolute path, not %q", path)}
	}
	if info, err := os.Stat(dir); err != nil {
		return "", &vrlogPathError{http.StatusServiceUnavailable, fmt.Sprintf(
			"the VRLOG directory %s cannot be read (%v): is the volume it is on mounted? It is set by --lidar-vrlog-dir", dir, err)}
	} else if !info.IsDir() {
		return "", &vrlogPathError{http.StatusServiceUnavailable, fmt.Sprintf("the VRLOG directory %s is not a directory", dir)}
	}
	// ResolvePathWithinDirectory follows symlinks before checking the
	// boundary, unlike a plain string-prefix check: a symlink planted inside
	// the directory that points outside it would pass a prefix check on the
	// literal path while still resolving elsewhere on disk. The canonical
	// path it returns is what gets opened, not the original string.
	canonical, err := security.ResolvePathWithinDirectory(cleaned, dir)
	if err != nil {
		return "", &vrlogPathError{http.StatusBadRequest, fmt.Sprintf(
			"the recording %s is outside the VRLOG directory %s: %v", path, dir, err)}
	}
	info, err := os.Stat(canonical)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", &vrlogPathError{http.StatusNotFound, fmt.Sprintf(
				"the recording %s does not exist (resolved to %s in %s)", path, canonical, dir)}
		}
		return "", &vrlogPathError{http.StatusServiceUnavailable, fmt.Sprintf(
			"the recording %s cannot be read: %v", canonical, err)}
	}
	if !info.IsDir() {
		// A recording is a directory (header.json and its chunks, or an
		// observation container's manifest); a file here is the wrong path.
		return "", &vrlogPathError{http.StatusBadRequest, fmt.Sprintf(
			"the recording %s is a file, not a VRLOG recording directory", canonical)}
	}
	return canonical, nil
}

// warnIfVRLogDirUnreadable logs at startup when the VRLOG directory cannot
// be read, so a volume that is not mounted shows in the ops log before the
// first replay is refused.
func (ws *Server) warnIfVRLogDirUnreadable() {
	dir := ws.vrlogDir()
	info, err := os.Stat(dir)
	switch {
	case err != nil:
		opsf("VRLOG directory %s cannot be read (%v): VRLOG replays and lane fields from recordings will fail until it can be (is its volume mounted?)", dir, err)
	case !info.IsDir():
		opsf("VRLOG directory %s is not a directory: VRLOG replays and lane fields from recordings will fail until --lidar-vrlog-dir names one", dir)
	}
}
