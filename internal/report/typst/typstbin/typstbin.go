// Package typstbin resolves the `typst` executable used to compile reports.
//
// Resolution order:
//
//  1. An embedded binary — present only when the program was built with the
//     `typst_embed` build tag and a platform binary was placed at
//     dist/typst before building (see the Makefile install-typst-dist target).
//     The embedded bytes are extracted once to a content-addressed cache file
//     under a per-user cache directory and reused across runs. This is the form
//     shipped on the Raspberry Pi image and in release binaries.
//  2. `typst` on PATH — the developer fallback.
//  3. A pinned release downloaded into the per-user cache — a development-only
//     convenience, disabled by VELOCITY_TYPST_NO_DOWNLOAD and never reached by
//     distributed builds (they embed the binary at step 1).
package typstbin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Resolve returns a path to a usable typst executable. The returned cleanup
// function must always be called by the caller (it is currently a no-op — the
// resolved binaries are retained in the cache for reuse — but is provided so
// callers can use a uniform `defer cleanup()`).
func Resolve() (path string, cleanup func(), err error) {
	cleanup = func() {}

	if data, ok := embeddedTypstFunc(); ok {
		p, exErr := extractCached(data)
		if exErr != nil {
			return "", cleanup, fmt.Errorf("extract embedded typst: %w", exErr)
		}
		return p, cleanup, nil
	}

	if p, lookErr := execLookPath("typst"); lookErr == nil {
		return p, cleanup, nil
	}

	// Development-only last resort: fetch a pinned typst release into the
	// per-user cache and reuse it. Distributed builds embed the binary (step 1)
	// and never reach this. Disable with VELOCITY_TYPST_NO_DOWNLOAD=1.
	if !downloadDisabled() {
		if p, dlErr := cachedDownloadFunc(); dlErr == nil {
			return p, cleanup, nil
		} else {
			return "", cleanup, fmt.Errorf(
				"typst not found and auto-download failed (%w); build with -tags typst_embed or install typst on PATH",
				dlErr)
		}
	}

	return "", cleanup, errors.New(
		"typst executable not found: build with -tags typst_embed (after make install-typst-dist), " +
			"or install typst on PATH")
}

// Embedded reports whether a typst binary was compiled into this build.
func Embedded() bool {
	_, ok := embeddedTypstFunc()
	return ok
}

// cacheDir returns a per-user cache directory for typst binaries, created with
// 0700 so the extracted/downloaded executable is not exposed in a
// world-writable location (defends against symlink / path-swap attacks).
func cacheDir() (string, error) {
	root, err := osUserCacheDir()
	if err != nil || root == "" {
		// No per-user cache dir available: fall back to a private, randomly
		// named 0700 temp dir rather than a predictable os.TempDir path, which
		// would reintroduce the symlink / path-swap exposure this helper exists
		// to avoid.
		return osMkdirTemp("", "velocity-report-typst-")
	}
	dir := filepath.Join(root, "velocity-report", "typst")
	if err := osMkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// isUsableBinary reports whether dest is a regular file of the expected size
// with the execute bit set — used to safely reuse a cached/concurrently-written
// binary instead of trusting size alone.
func isUsableBinary(dest string, size int64) bool {
	fi, err := osStat(dest)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() != size {
		return false
	}
	return isExecutableMode(runtime.GOOS, fi.Mode())
}

// usableExecutable is like isUsableBinary but for cases where the expected size
// is not known ahead of time (a downloaded binary): regular, non-empty, with
// the execute bit set.
func usableExecutable(dest string) bool {
	fi, err := osStat(dest)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 {
		return false
	}
	return isExecutableMode(runtime.GOOS, fi.Mode())
}

// extractCached writes the embedded binary to a content-addressed file in the
// per-user cache and returns its path, reusing the file if a valid one already
// exists. The write is atomic (temp file + rename) so concurrent report
// generations cannot observe a partially written executable.
func extractCached(data []byte) (string, error) {
	dir, err := cacheDirFunc()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:8])
	dest := filepath.Join(dir, "typst-"+hash+exeSuffix())

	if isUsableBinary(dest, int64(len(data))) {
		return dest, nil
	}

	tmp, err := createTempExec(dir, "typst-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := osRename(tmpName, dest); err != nil {
		// A concurrent writer may have created it first; accept it only if it is
		// a valid executable of the expected size.
		if isUsableBinary(dest, int64(len(data))) {
			return dest, nil
		}
		return "", err
	}
	return dest, nil
}

func exeSuffix() string {
	return exeSuffixFor(runtime.GOOS)
}
