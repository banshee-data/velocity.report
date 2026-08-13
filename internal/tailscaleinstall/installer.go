// Package tailscaleinstall installs the pinned, static Tailscale release used
// by velocity.report. The payload is downloaded only after an operator opts in.
package tailscaleinstall

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// Version is intentionally pinned: updates are shipped by updating this
	// manifest, not by accepting a mutable "latest" download at runtime.
	Version       = "1.102.2"
	rootDir       = "/opt/velocity-report/tailscale"
	maxBinarySize = 64 << 20
)

// Release records a verified static Tailscale tarball.
type Release struct {
	Version      string
	Architecture string
	URL          string
	SHA256       string
}

var releases = map[string]Release{
	"arm64": {
		Version: Version, Architecture: "arm64",
		URL:    "https://pkgs.tailscale.com/stable/tailscale_1.102.2_arm64.tgz",
		SHA256: "2b64e9ade7e73034b5ec9e9bcd537f5ddd14ae3abb435e57e929e7486ae42660",
	},
	"amd64": {
		Version: Version, Architecture: "amd64",
		URL:    "https://pkgs.tailscale.com/stable/tailscale_1.102.2_amd64.tgz",
		SHA256: "ad2cde12f8de95f7b93a1e0401e652291c603d42b9d60a33fb1741eb38ab04d8",
	},
}

// Installer dependencies are fields so extraction, checksums, idempotence,
// and service wiring can be tested without root or network access.
type Installer struct {
	Root         string
	SystemdDir   string
	LinkDir      string
	OSRelease    string
	Architecture func() string
	Fetch        func(context.Context, string) (io.ReadCloser, error)
	Run          func(context.Context, string, ...string) error
}

type metadata struct {
	Version      string `json:"version"`
	Architecture string `json:"architecture"`
	SHA256       string `json:"sha256"`
}

// Install downloads, verifies, extracts and wires the selected release. A
// healthy requested release is left untouched so repeated UI enables are safe.
func (i Installer) Install(ctx context.Context) error {
	if err := i.requireDebianFamily(); err != nil {
		return err
	}
	arch := i.architecture()
	release, ok := releases[arch]
	if !ok {
		return fmt.Errorf("unsupported Tailscale architecture %q (supported: arm64, amd64)", arch)
	}
	if i.healthy(release) {
		return nil
	}
	if err := os.MkdirAll(i.root(), 0755); err != nil {
		return fmt.Errorf("create Tailscale root: %w", err)
	}
	body, err := i.fetch(ctx, release.URL)
	if err != nil {
		return fmt.Errorf("download Tailscale %s: %w", release.Version, err)
	}
	defer body.Close()

	temp, err := os.CreateTemp(i.root(), ".tailscale-*.tgz")
	if err != nil {
		return fmt.Errorf("create Tailscale download: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(temp, hash), io.LimitReader(body, 128<<20)); err != nil {
		temp.Close()
		return fmt.Errorf("write Tailscale download: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close Tailscale download: %w", err)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != release.SHA256 {
		return fmt.Errorf("Tailscale %s checksum mismatch: got %s, want %s", release.Version, got, release.SHA256)
	}
	if err := i.extract(tempName, release); err != nil {
		return err
	}
	if err := i.writeService(release); err != nil {
		return err
	}
	return i.run(ctx, "/usr/bin/systemctl", "daemon-reload")
}

func (i Installer) root() string {
	if i.Root != "" {
		return i.Root
	}
	return rootDir
}
func (i Installer) systemdDir() string {
	if i.SystemdDir != "" {
		return i.SystemdDir
	}
	return "/etc/systemd/system"
}
func (i Installer) linkDir() string {
	if i.LinkDir != "" {
		return i.LinkDir
	}
	return "/usr/local/bin"
}
func (i Installer) architecture() string {
	if i.Architecture != nil {
		return i.Architecture()
	}
	return runtime.GOARCH
}

func (i Installer) fetch(ctx context.Context, url string) (io.ReadCloser, error) {
	if i.Fetch != nil {
		return i.Fetch(ctx, url)
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}
	return resp.Body, nil
}

func (i Installer) run(ctx context.Context, name string, args ...string) error {
	if i.Run != nil {
		return i.Run(ctx, name, args...)
	}
	if output, err := exec.CommandContext(ctx, name, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("run %s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (i Installer) requireDebianFamily() error {
	path := i.OSRelease
	if path == "" {
		path = "/etc/os-release"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read OS release: %w", err)
	}
	values := parseOSRelease(string(b))
	id := values["ID"]
	like := values["ID_LIKE"]
	if id == "debian" || id == "raspbian" || strings.Contains(" "+like+" ", " debian ") {
		return nil
	}
	return fmt.Errorf("unsupported Linux distribution %q; only Debian-family images are supported", id)
}

func parseOSRelease(contents string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(contents, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			result[key] = strings.Trim(value, "\"")
		}
	}
	return result
}

func (i Installer) healthy(release Release) bool {
	b, err := os.ReadFile(filepath.Join(i.root(), "install.json"))
	if err != nil {
		return false
	}
	var got metadata
	if json.Unmarshal(b, &got) != nil || got.Version != release.Version || got.Architecture != release.Architecture || got.SHA256 != release.SHA256 {
		return false
	}
	for _, name := range []string{"tailscale", "tailscaled"} {
		info, err := os.Stat(filepath.Join(i.root(), "current", name))
		if err != nil || info.Mode()&0111 == 0 {
			return false
		}
		link := filepath.Join(i.linkDir(), name)
		linkInfo, err := os.Lstat(link)
		if err != nil || linkInfo.Mode()&os.ModeSymlink == 0 {
			return false
		}
		target, err := os.Readlink(link)
		if err != nil || target != filepath.Join(i.root(), "current", name) {
			return false
		}
	}
	_, err = os.Stat(filepath.Join(i.systemdDir(), "tailscaled.service"))
	return err == nil
}

func (i Installer) extract(tarball string, release Release) error {
	file, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open Tailscale archive: %w", err)
	}
	defer gz.Close()

	staging, err := os.MkdirTemp(i.root(), ".extract-")
	if err != nil {
		return fmt.Errorf("create extraction directory: %w", err)
	}
	defer os.RemoveAll(staging)
	tr := tar.NewReader(gz)
	found := make(map[string]bool)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read Tailscale archive: %w", err)
		}
		name := filepath.ToSlash(filepath.Clean(hdr.Name))
		for _, binary := range []string{"tailscale", "tailscaled"} {
			if name != binary && !strings.HasSuffix(name, "/"+binary) {
				continue
			}
			if hdr.Typeflag != tar.TypeReg {
				return fmt.Errorf("Tailscale archive %s is not a regular file", binary)
			}
			if hdr.Size < 0 || hdr.Size > maxBinarySize {
				return fmt.Errorf("Tailscale archive %s size %d exceeds limit %d", binary, hdr.Size, maxBinarySize)
			}
			out := filepath.Join(staging, binary)
			f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			written, copyErr := io.CopyN(f, tr, hdr.Size)
			closeErr := f.Close()
			if copyErr != nil {
				return fmt.Errorf("extract %s (%d of %d bytes): %w", binary, written, hdr.Size, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close %s: %w", binary, closeErr)
			}
			found[binary] = true
		}
	}
	if !found["tailscale"] || !found["tailscaled"] {
		return fmt.Errorf("Tailscale archive is missing tailscale or tailscaled")
	}
	// os.MkdirTemp creates the staging directory as 0700. The velocity service
	// account must traverse the final version directory to invoke tailscale via
	// the local API socket, so correct that mode before atomically activating it.
	if err := os.Chmod(staging, 0755); err != nil {
		return fmt.Errorf("set Tailscale payload permissions: %w", err)
	}
	versionDir := filepath.Join(i.root(), release.Version)
	if err := os.RemoveAll(versionDir); err != nil {
		return fmt.Errorf("replace Tailscale %s: %w", release.Version, err)
	}
	if err := os.Rename(staging, versionDir); err != nil {
		return fmt.Errorf("activate Tailscale %s: %w", release.Version, err)
	}
	currentTmp := filepath.Join(i.root(), ".current")
	if err := os.Remove(currentTmp); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(release.Version, currentTmp); err != nil {
		return err
	}
	if err := os.Rename(currentTmp, filepath.Join(i.root(), "current")); err != nil {
		return fmt.Errorf("refresh Tailscale current symlink: %w", err)
	}
	if err := i.writeStableLinks(); err != nil {
		return err
	}
	contents, err := json.Marshal(metadata{Version: release.Version, Architecture: release.Architecture, SHA256: release.SHA256})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(i.root(), "install.json"), append(contents, '\n'), 0644); err != nil {
		return fmt.Errorf("record Tailscale install: %w", err)
	}
	return nil
}

func (i Installer) writeStableLinks() error {
	if err := os.MkdirAll(i.linkDir(), 0755); err != nil {
		return fmt.Errorf("create Tailscale link directory: %w", err)
	}
	for _, name := range []string{"tailscale", "tailscaled"} {
		path := filepath.Join(i.linkDir(), name)
		if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refuse to replace non-symlink %s", path)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
		tmp := path + ".velocity-new"
		if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Symlink(filepath.Join(i.root(), "current", name), tmp); err != nil {
			return fmt.Errorf("create %s link: %w", name, err)
		}
		if err := os.Rename(tmp, path); err != nil {
			return fmt.Errorf("refresh %s link: %w", name, err)
		}
	}
	return nil
}

func (i Installer) writeService(release Release) error {
	if err := os.MkdirAll(i.systemdDir(), 0755); err != nil {
		return fmt.Errorf("create systemd directory: %w", err)
	}
	service := `[Unit]
Description=Tailscale node agent
After=network-online.target NetworkManager-wait-online.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=` + filepath.Join(i.root(), "current", "tailscaled") + ` --state=/var/lib/tailscale/tailscaled.state --socket=/run/tailscale/tailscaled.sock
ExecStopPost=` + filepath.Join(i.root(), "current", "tailscaled") + ` --cleanup
Restart=on-failure
RuntimeDirectory=tailscale
StateDirectory=tailscale

[Install]
WantedBy=multi-user.target
`
	if err := os.WriteFile(filepath.Join(i.systemdDir(), "tailscaled.service"), []byte(service), 0644); err != nil {
		return fmt.Errorf("write tailscaled service: %w", err)
	}
	return nil
}
