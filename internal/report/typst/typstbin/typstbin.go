// Package typstbin resolves the `typst` executable used to compile reports.
//
// Resolution order:
//
//  1. VELOCITY_TYPST_PATH — an explicit executable path (ops / test override).
//  2. An embedded binary — present only when the program was built with the
//     `typst_embed` build tag and a platform binary was placed at
//     dist/typst before building (see the Makefile install-typst-dist target).
//     The embedded bytes are extracted once to a content-addressed cache file
//     under the temp dir and reused across runs.
//  3. `typst` on PATH — the developer fallback.
//
// The embedded path is what ships on the Raspberry Pi image: the typst binary
// travels inside the velocity binary, so no separate package install or PATH
// entry is required on the device.
package typstbin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// EnvPath is the environment variable that overrides typst resolution.
const EnvPath = "VELOCITY_TYPST_PATH"

// Resolve returns a path to a usable typst executable. The returned cleanup
// function must always be called by the caller (it is a no-op for the env and
// PATH cases, and removes nothing for the cached embed case since the binary
// is intentionally retained for reuse). It is provided so callers can use a
// uniform `defer cleanup()` regardless of how the binary was resolved.
func Resolve() (path string, cleanup func(), err error) {
	cleanup = func() {}

	if p := os.Getenv(EnvPath); p != "" {
		if _, statErr := os.Stat(p); statErr != nil {
			return "", cleanup, fmt.Errorf("%s=%q: %w", EnvPath, p, statErr)
		}
		return p, cleanup, nil
	}

	if data, ok := embeddedTypst(); ok {
		p, exErr := extractCached(data)
		if exErr != nil {
			return "", cleanup, fmt.Errorf("extract embedded typst: %w", exErr)
		}
		return p, cleanup, nil
	}

	if p, lookErr := exec.LookPath("typst"); lookErr == nil {
		return p, cleanup, nil
	}

	// Last resort: fetch a pinned typst release into the user cache and reuse it
	// on subsequent runs. This makes local development "just work" without a
	// manual install. Disable with VELOCITY_TYPST_NO_DOWNLOAD=1 (the device
	// image embeds the binary, so it never reaches this path).
	if !downloadDisabled() {
		if p, dlErr := cachedDownload(); dlErr == nil {
			return p, cleanup, nil
		} else {
			return "", cleanup, fmt.Errorf(
				"typst not found and auto-download failed (%w); build with -tags typst_embed, set %s, or install typst on PATH",
				dlErr, EnvPath)
		}
	}

	return "", cleanup, errors.New(
		"typst executable not found: build with -tags typst_embed (after make install-typst-dist), " +
			"set " + EnvPath + ", or install typst on PATH")
}

// Embedded reports whether a typst binary was compiled into this build.
func Embedded() bool {
	_, ok := embeddedTypst()
	return ok
}

// extractCached writes the embedded binary to a content-addressed file under
// the temp dir and returns its path, reusing the file if it already exists.
// The write is atomic (temp file + rename) so concurrent report generations
// cannot observe a partially written executable.
func extractCached(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:8])

	dir := filepath.Join(os.TempDir(), "velocity-typst")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, "typst-"+hash+exeSuffix())

	if fi, err := os.Stat(dest); err == nil && fi.Size() == int64(len(data)) && fi.Mode()&0o111 != 0 {
		return dest, nil
	}

	tmp, err := os.CreateTemp(dir, "typst-*.tmp")
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
	if err := os.Rename(tmpName, dest); err != nil {
		// A concurrent writer may have created it first; accept an existing
		// file of the right size.
		if fi, statErr := os.Stat(dest); statErr == nil && fi.Size() == int64(len(data)) {
			return dest, nil
		}
		return "", err
	}
	return dest, nil
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
