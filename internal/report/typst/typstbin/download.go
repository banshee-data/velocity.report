package typstbin

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Version is the typst release the auto-downloader fetches. Keep in sync with
// the Makefile TYPST_VERSION and scripts/download-typst.sh default.
const Version = "0.13.1"

// EnvNoDownload disables the runtime auto-download fallback when truthy.
const EnvNoDownload = "VELOCITY_TYPST_NO_DOWNLOAD"

// downloadMu serialises auto-downloads within a process so two concurrent
// report generations don't race to fetch the same binary.
var downloadMu sync.Mutex

func downloadDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(osGetenv(EnvNoDownload))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// typstTarget maps the Go runtime to the typst release asset triple and its
// archive extension.
func typstTarget() (target, archive string, err error) {
	return typstTargetFor(runtime.GOOS, runtime.GOARCH)
}

func typstTargetFor(goos, goarch string) (target, archive string, err error) {
	switch goos + "/" + goarch {
	case "linux/arm64":
		return "aarch64-unknown-linux-musl", "tar.xz", nil
	case "linux/amd64":
		return "x86_64-unknown-linux-musl", "tar.xz", nil
	case "darwin/arm64":
		return "aarch64-apple-darwin", "tar.xz", nil
	case "darwin/amd64":
		return "x86_64-apple-darwin", "tar.xz", nil
	case "windows/amd64":
		return "x86_64-pc-windows-msvc", "zip", nil
	default:
		return "", "", fmt.Errorf("no typst release for %s/%s", goos, goarch)
	}
}

// cachedDownload returns the path to a typst binary in the user cache,
// downloading and extracting it on first use and reusing it thereafter. The
// cache key includes the version so upgrades fetch a fresh binary.
func cachedDownload() (string, error) {
	downloadMu.Lock()
	defer downloadMu.Unlock()

	base, err := cacheDirFunc()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, Version)
	if err := osMkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, "typst"+exeSuffix())
	if usableExecutable(dest) {
		return dest, nil
	}

	target, archive, err := typstTargetFunc()
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://github.com/typst/typst/releases/download/v%s/typst-%s.%s", Version, target, archive)
	fmt.Fprintf(os.Stderr, "velocity-report: typst not found; downloading %s\n", url)

	tmp, err := os.MkdirTemp(dir, "dl-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	archivePath := filepath.Join(tmp, "typst."+archive)
	if err := httpDownloadFunc(url, archivePath); err != nil {
		return "", fmt.Errorf("download typst: %w", err)
	}

	binPath, err := extractTypstFunc(archivePath, archive, target, tmp)
	if err != nil {
		return "", err
	}
	if err := osChmod(binPath, 0o755); err != nil {
		return "", err
	}
	if err := osRename(binPath, dest); err != nil {
		// Cross-device rename or a concurrent writer: accept an already-present
		// valid executable, otherwise fall back to a copy.
		if usableExecutable(dest) {
			return dest, nil
		}
		if cpErr := copyExecutableFunc(binPath, dest); cpErr != nil {
			return "", cpErr
		}
	}
	fmt.Fprintf(os.Stderr, "velocity-report: typst %s installed at %s\n", Version, dest)
	return dest, nil
}

func httpDownload(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// extractTypst pulls the typst executable out of the downloaded archive into
// dir and returns its path. tar.xz archives are unpacked via the system tar
// (which handles xz on macOS/Linux); zip archives use archive/zip.
func extractTypst(archivePath, archive, target, dir string) (string, error) {
	if archive == "zip" {
		dest := filepath.Join(dir, "typst.exe")
		return dest, extractZipEntry(archivePath, "typst-"+target+"/typst.exe", dest)
	}
	cmd := exec.Command("tar", "-xJf", archivePath, "-C", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("tar -xJf failed (is `tar` with xz support installed?): %w: %s", err, out)
	}
	return filepath.Join(dir, "typst-"+target, "typst"), nil
}

func extractZipEntry(zipPath, inner, dest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == inner || filepath.Base(f.Name) == filepath.Base(inner) {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			out, err := os.Create(dest)
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, rc)
			return err
		}
	}
	return fmt.Errorf("typst.exe not found in %s", zipPath)
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
