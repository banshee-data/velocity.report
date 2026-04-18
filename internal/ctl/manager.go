package ctl

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/version"
)

const (
	defaultReleasesMetaURL = "https://velocity.report/releases.json"
	defaultBinaryName      = "velocity-report"
	defaultBinaryPath      = "/usr/local/bin/" + defaultBinaryName
	defaultServiceName     = "velocity-report.service"
	defaultBackupDir       = "/var/lib/velocity-report/backups"
	defaultDBPath          = "/var/lib/velocity-report/sensor_data.db"
)

type Config struct {
	ReleasesMetaURL string
	BinaryName      string
	BinaryPath      string
	ServiceName     string
	BackupDir       string
	DBPath          string
	RequestTimeout  time.Duration
	DownloadTimeout time.Duration
	VerifyDelay     time.Duration
	CurrentVersion  string
	GOOS            string
	GOARCH          string
}

type UpgradeOptions struct {
	IncludePrereleases bool
}

func DefaultConfig() Config {
	return Config{
		ReleasesMetaURL: defaultReleasesMetaURL,
		BinaryName:      defaultBinaryName,
		BinaryPath:      defaultBinaryPath,
		ServiceName:     defaultServiceName,
		BackupDir:       defaultBackupDir,
		DBPath:          defaultDBPath,
		RequestTimeout:  30 * time.Second,
		DownloadTimeout: 5 * time.Minute,
		VerifyDelay:     2 * time.Second,
		CurrentVersion:  version.Version,
		GOOS:            runtime.GOOS,
		GOARCH:          runtime.GOARCH,
	}
}

type HTTPGetter interface {
	Get(url string) (*http.Response, error)
}

type CommandRunner interface {
	Run(name string, args ...string) error
}

type ExecRunner struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (r ExecRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	return cmd.Run()
}

type Manager struct {
	cfg        Config
	httpClient HTTPGetter
	runner     CommandRunner
	out        io.Writer
	err        io.Writer
	now        func() time.Time
	sleep      func(time.Duration)
}

func NewDefaultManager() *Manager {
	cfg := DefaultConfig()
	return &Manager{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.RequestTimeout},
		runner:     ExecRunner{Stdout: os.Stdout, Stderr: os.Stderr},
		out:        os.Stdout,
		err:        os.Stderr,
		now:        time.Now,
		sleep:      time.Sleep,
	}
}

func NewManager(cfg Config, httpClient HTTPGetter, runner CommandRunner, out io.Writer, err io.Writer) *Manager {
	if cfg.ReleasesMetaURL == "" {
		cfg.ReleasesMetaURL = defaultReleasesMetaURL
	}
	if cfg.BinaryName == "" {
		cfg.BinaryName = defaultBinaryName
	}
	if cfg.BinaryPath == "" {
		cfg.BinaryPath = defaultBinaryPath
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = defaultServiceName
	}
	if cfg.BackupDir == "" {
		cfg.BackupDir = defaultBackupDir
	}
	if cfg.DBPath == "" {
		cfg.DBPath = defaultDBPath
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 30 * time.Second
	}
	if cfg.DownloadTimeout == 0 {
		cfg.DownloadTimeout = 5 * time.Minute
	}
	if cfg.VerifyDelay == 0 {
		cfg.VerifyDelay = 2 * time.Second
	}
	if cfg.CurrentVersion == "" {
		cfg.CurrentVersion = version.Version
	}
	if cfg.GOOS == "" {
		cfg.GOOS = runtime.GOOS
	}
	if cfg.GOARCH == "" {
		cfg.GOARCH = runtime.GOARCH
	}

	if out == nil {
		out = io.Discard
	}
	if err == nil {
		err = io.Discard
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.RequestTimeout}
	}
	if runner == nil {
		runner = ExecRunner{Stdout: out, Stderr: err}
	}

	return &Manager{
		cfg:        cfg,
		httpClient: httpClient,
		runner:     runner,
		out:        out,
		err:        err,
		now:        time.Now,
		sleep:      time.Sleep,
	}
}

func (m *Manager) ServiceName() string {
	return m.cfg.ServiceName
}

func (m *Manager) RunUpgrade(checkOnly bool, binaryFile string) error {
	return m.RunUpgradeWithOptions(checkOnly, binaryFile, UpgradeOptions{})
}

func (m *Manager) RunUpgradeWithOptions(checkOnly bool, binaryFile string, opts UpgradeOptions) error {
	if binaryFile != "" {
		return m.applyLocalBinary(binaryFile)
	}

	latest, assetURL, err := m.fetchLatestRelease(opts.IncludePrereleases)
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}

	current := m.cfg.CurrentVersion

	if latest == current {
		fmt.Fprintf(m.out, "Already up to date (v%s).\n", current)
		return nil
	}

	// Semver comparison to prevent downgrades.
	currentSV, currentParsed := parseSemver(current)
	latestSV, latestParsed := parseSemver(latest)
	if currentParsed && latestParsed {
		cmp := compareSemver(latestSV, currentSV)
		if cmp < 0 {
			// Label reflects the fetched version's actual channel, not the
			// flag: --prerelease can fall back to stable when no prerelease
			// is published.
			channelLabel := "stable"
			if latestSV.isPrerelease() {
				channelLabel = "pre-release"
			}
			fmt.Fprintf(m.out, "Current: v%s\n", current)
			fmt.Fprintf(m.out, "Latest %s: v%s\n", channelLabel, latest)
			fmt.Fprintf(m.out, "\nCurrent version is newer than the latest %s release.\n", channelLabel)
			if currentSV.isPrerelease() && !opts.IncludePrereleases {
				fmt.Fprintln(m.out, "You are on a pre-release. Run 'sudo velocity-ctl upgrade --prerelease' to check for newer pre-releases.")
			}
			return nil
		}
		if cmp == 0 {
			fmt.Fprintf(m.out, "Already up to date (v%s).\n", current)
			return nil
		}
	}

	fmt.Fprintf(m.out, "Current: v%s\n", current)
	fmt.Fprintf(m.out, "Latest:  v%s\n", latest)

	if checkOnly {
		fmt.Fprintln(m.out, "\nRun 'sudo velocity-ctl upgrade' to apply.")
		return nil
	}

	fmt.Fprintf(m.out, "Downloading from %s...\n", assetURL)
	tmpFile, err := m.downloadToTemp(assetURL)
	if err != nil {
		return fmt.Errorf("downloading release: %w", err)
	}
	defer os.Remove(tmpFile)

	return m.applyUpgrade(tmpFile)
}

func (m *Manager) RunBackup(outputDir string) (string, error) {
	if outputDir == "" {
		outputDir = m.cfg.BackupDir
	}
	return m.createBackupTo(outputDir)
}

func (m *Manager) RunRollback() error {
	entries, err := os.ReadDir(m.cfg.BackupDir)
	if err != nil {
		return fmt.Errorf("reading backup directory %s: %w", m.cfg.BackupDir, err)
	}

	var backups []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "20") {
			backups = append(backups, e.Name())
		}
	}

	if len(backups) == 0 {
		return fmt.Errorf("no backups found in %s", m.cfg.BackupDir)
	}

	sort.Strings(backups)
	latest := backups[len(backups)-1]
	backupPath := filepath.Join(m.cfg.BackupDir, latest)

	fmt.Fprintf(m.out, "Rolling back to backup: %s\n", latest)
	return m.restoreBackup(backupPath)
}

func (m *Manager) RunStatus() error {
	err := m.runner.Run("systemctl", "status", m.cfg.ServiceName)
	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		fmt.Fprintf(m.err, "\nService is not running (exit code %d).\n", exitErr.ExitCode())
		return nil
	}

	type exitCoder interface {
		ExitCode() int
	}
	var codedErr exitCoder
	if errors.As(err, &codedErr) {
		fmt.Fprintf(m.err, "\nService is not running (exit code %d).\n", codedErr.ExitCode())
		return nil
	}

	return fmt.Errorf("running systemctl: %w", err)
}

type releasesMeta struct {
	Stable     releasesChannel `json:"stable"`
	Prerelease releasesChannel `json:"prerelease"`
}

type releasesChannel struct {
	Version    string       `json:"version"`
	LinuxArm64 releaseAsset `json:"linux_arm64"`
	MacArm64   releaseAsset `json:"mac_arm64"`
	Visualiser releaseAsset `json:"visualiser"`
}

type releaseAsset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

func (m *Manager) applyLocalBinary(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("binary file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("expected a file, got directory: %s", path)
	}

	fmt.Fprintf(m.out, "Applying local binary: %s\n", path)
	return m.applyUpgrade(path)
}

func (m *Manager) applyUpgrade(newBinaryPath string) error {
	fmt.Fprintln(m.out, "Creating backup...")
	backupPath, err := m.createBackupTo(m.cfg.BackupDir)
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}
	fmt.Fprintf(m.out, "Backup saved to %s\n", backupPath)

	fmt.Fprintln(m.out, "Stopping velocity-report...")
	if err := m.systemctl("stop", m.cfg.ServiceName); err != nil {
		return fmt.Errorf("stopping service: %w", err)
	}

	fmt.Fprintln(m.out, "Installing new binary...")
	if err := m.installBinary(newBinaryPath, m.cfg.BinaryPath); err != nil {
		fmt.Fprintf(m.err, "install failed: %v - attempting rollback\n", err)
		_ = m.restoreBackup(backupPath)
		_ = m.systemctl("start", m.cfg.ServiceName)
		return fmt.Errorf("installing binary: %w", err)
	}

	fmt.Fprintln(m.out, "Running database migrations...")
	if err := m.runMigrations(); err != nil {
		fmt.Fprintf(m.err, "warning: migration failed: %v\n", err)
	}

	fmt.Fprintln(m.out, "Starting velocity-report...")
	if err := m.systemctl("start", m.cfg.ServiceName); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}

	fmt.Fprintln(m.out, "Verifying service status...")
	m.sleep(m.cfg.VerifyDelay)
	if err := m.systemctl("is-active", m.cfg.ServiceName); err != nil {
		fmt.Fprintln(m.err, "warning: service may not be running - check 'velocity-ctl status'")
	} else {
		fmt.Fprintln(m.out, "Upgrade complete. Service is active.")
	}

	return nil
}

// fetchLatestRelease fetches velocity.report/releases.json and returns the
// version string and download URL for the appropriate channel and platform.
func (m *Manager) fetchLatestRelease(includePrereleases bool) (version, downloadURL string, err error) {
	resp, err := m.httpClient.Get(m.cfg.ReleasesMetaURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("releases metadata returned %s", resp.Status)
	}

	var meta releasesMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", "", fmt.Errorf("parsing releases.json: %w", err)
	}

	ch := meta.Stable
	if includePrereleases && meta.Prerelease.Version != "" {
		ch = meta.Prerelease
	}
	if ch.Version == "" {
		return "", "", fmt.Errorf("no version found in releases.json")
	}

	asset, err := pickAsset(ch, m.cfg.GOOS, m.cfg.GOARCH)
	if err != nil {
		return "", "", err
	}
	if asset.URL == "" {
		return "", "", fmt.Errorf("release %s has no download URL for %s/%s", ch.Version, m.cfg.GOOS, m.cfg.GOARCH)
	}
	return ch.Version, asset.URL, nil
}

// pickAsset returns the asset for the caller's platform. velocity-ctl
// only ships the server binary; the visualiser is macOS-only and not an
// upgrade target here.
func pickAsset(ch releasesChannel, goos, goarch string) (releaseAsset, error) {
	switch {
	case goos == "linux" && goarch == "arm64":
		return ch.LinuxArm64, nil
	case goos == "darwin" && goarch == "arm64":
		return ch.MacArm64, nil
	default:
		return releaseAsset{}, fmt.Errorf("unsupported platform %s/%s", goos, goarch)
	}
}

func (m *Manager) downloadToTemp(url string) (string, error) {
	client := &http.Client{Timeout: m.cfg.DownloadTimeout}
	resp, err := client.Get(url) //nolint:gosec // URL comes from release metadata
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %s", resp.Status)
	}

	tmp, err := os.CreateTemp("", "velocity-ctl-upgrade-*")
	if err != nil {
		return "", err
	}

	h := sha256.New()
	written, err := io.Copy(tmp, io.TeeReader(resp.Body, h))
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}

	computed := hex.EncodeToString(h.Sum(nil))
	fmt.Fprintf(m.out, "Downloaded %d bytes (SHA-256: %s)\n", written, computed)

	// Derive binary filename and the SHA256SUMS URL from the last path segment.
	// strings.LastIndex preserves the URL scheme and authority unchanged.
	lastSlash := strings.LastIndex(url, "/")
	binaryName := url[lastSlash+1:]
	sumsURL := url[:lastSlash+1] + "SHA256SUMS"

	expected, err := m.fetchExpectedChecksum(client, sumsURL, binaryName)
	if err != nil {
		// SHA256SUMS not available or does not cover this binary: warn and proceed
		// so that upgrades continue to work for releases that predate the manifest.
		fmt.Fprintf(m.err, "warning: SHA256SUMS not available for this release, skipping verification (%v)\n", err)
		return tmp.Name(), nil
	}

	if computed != expected {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("SHA-256 mismatch for %s: expected %s, got %s", binaryName, expected, computed)
	}

	fmt.Fprintf(m.out, "SHA-256 verified.\n")
	return tmp.Name(), nil
}

// fetchExpectedChecksum downloads the SHA256SUMS manifest at sumsURL and returns
// the expected hex digest for binaryName. It returns an error if the manifest
// cannot be fetched, returns a non-200 status, or does not contain an entry for
// the requested binary.
func (m *Manager) fetchExpectedChecksum(client *http.Client, sumsURL, binaryName string) (string, error) {
	resp, err := client.Get(sumsURL) //nolint:gosec // URL derived from release metadata
	if err != nil {
		return "", fmt.Errorf("fetching SHA256SUMS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SHA256SUMS returned %s", resp.Status)
	}

	// Standard sha256sum format: "<hex>  <filename>" (two spaces) or "<hex> <filename>".
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[1] == binaryName {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading SHA256SUMS: %w", err)
	}

	return "", fmt.Errorf("no entry for %s in SHA256SUMS", binaryName)
}

func (m *Manager) createBackupTo(baseDir string) (string, error) {
	ts := m.now().UTC().Format("20060102-150405")
	dest := filepath.Join(baseDir, ts)

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", fmt.Errorf("creating backup directory: %w", err)
	}

	if err := copyFile(m.cfg.BinaryPath, filepath.Join(dest, m.cfg.BinaryName)); err != nil {
		return "", fmt.Errorf("backing up binary: %w", err)
	}

	if _, err := os.Stat(m.cfg.DBPath); err == nil {
		if err := copyFile(m.cfg.DBPath, filepath.Join(dest, "sensor_data.db")); err != nil {
			return "", fmt.Errorf("backing up database: %w", err)
		}
	}

	fmt.Fprintf(m.out, "Backup created: %s\n", dest)
	return dest, nil
}

func (m *Manager) restoreBackup(backupPath string) error {
	binaryBackup := filepath.Join(backupPath, m.cfg.BinaryName)
	if _, err := os.Stat(binaryBackup); err != nil {
		return fmt.Errorf("backup binary not found at %s: %w", binaryBackup, err)
	}

	fmt.Fprintln(m.out, "Stopping velocity-report...")
	if err := m.systemctl("stop", m.cfg.ServiceName); err != nil {
		return fmt.Errorf("stopping service: %w", err)
	}

	fmt.Fprintln(m.out, "Restoring binary...")
	if err := m.installBinary(binaryBackup, m.cfg.BinaryPath); err != nil {
		return fmt.Errorf("restoring binary: %w", err)
	}

	dbBackup := filepath.Join(backupPath, "sensor_data.db")
	if _, err := os.Stat(dbBackup); err == nil {
		fmt.Fprintln(m.out, "Restoring database...")
		if err := m.installBinary(dbBackup, m.cfg.DBPath); err != nil {
			fmt.Fprintf(m.err, "warning: database restore failed: %v\n", err)
		}
	}

	fmt.Fprintln(m.out, "Starting velocity-report...")
	if err := m.systemctl("start", m.cfg.ServiceName); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}

	fmt.Fprintf(m.out, "Rollback complete (restored from %s).\n", filepath.Base(backupPath))
	return nil
}

func (m *Manager) installBinary(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".velocity-ctl-install-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := io.Copy(tmp, srcFile); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}

	if err := os.Chmod(tmpName, 0o755); err != nil {
		os.Remove(tmpName)
		return err
	}

	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return err
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}

	return out.Close()
}

func (m *Manager) runMigrations() error {
	return m.runner.Run(m.cfg.BinaryPath, "migrate", "up", "--db-path", m.cfg.DBPath)
}

func (m *Manager) systemctl(action, unit string) error {
	return m.runner.Run("systemctl", action, unit)
}
