package ctl

import (
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

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/version"
)

const (
	defaultReleaseMetaURL = "https://velocity.report/release.json"
	defaultInstallRoot    = "/opt/velocity-report"
	defaultBinaryName     = "velocity"
	defaultServiceName    = "velocity-report.service"
	defaultBackupDir      = "/var/lib/velocity-report/backups"
	defaultDBPath         = "/var/lib/velocity-report/sensor_data.db"
	defaultVersionURL     = "http://127.0.0.1/api/version"
	defaultRetain         = 3

	versionsSubdir = "versions"
	currentName    = "current"
	previousName   = "previous"
)

// Config describes the on-disk layout and runtime parameters of the versioned
// install at InstallRoot:
//
//	<InstallRoot>/versions/<v>/<BinaryName>   real binaries, one dir per version
//	<InstallRoot>/current  -> versions/<v>     active version (symlink)
//	<InstallRoot>/previous -> versions/<old>   one-shot rollback target (symlink)
//
// /usr/local/bin/velocity{,-report} are symlinks to current/<BinaryName>
// and are owned by the image stage, not this manager.
type Config struct {
	ReleaseMetaURL  string
	InstallRoot     string
	BinaryName      string
	ServiceName     string
	BackupDir       string
	DBPath          string
	Retain          int // number of versions to keep under versions/ (incl. current+previous)
	RequestTimeout  time.Duration
	DownloadTimeout time.Duration
	VerifyDelay     time.Duration
	VerifyTimeout   time.Duration // poll /api/version for up to this long after restart (0 = skip)
	VersionURL      string        // GET /api/version of the running server
	CurrentVersion  string
	GOOS            string
	GOARCH          string
}

type UpgradeOptions struct {
	IncludePrereleases bool
}

func DefaultConfig() Config {
	return Config{
		ReleaseMetaURL:  defaultReleaseMetaURL,
		InstallRoot:     defaultInstallRoot,
		BinaryName:      defaultBinaryName,
		ServiceName:     defaultServiceName,
		BackupDir:       defaultBackupDir,
		DBPath:          defaultDBPath,
		Retain:          defaultRetain,
		RequestTimeout:  30 * time.Second,
		DownloadTimeout: 5 * time.Minute,
		VerifyDelay:     2 * time.Second,
		VerifyTimeout:   60 * time.Second,
		VersionURL:      defaultVersionURL,
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
	if cfg.ReleaseMetaURL == "" {
		cfg.ReleaseMetaURL = defaultReleaseMetaURL
	}
	if cfg.InstallRoot == "" {
		cfg.InstallRoot = defaultInstallRoot
	}
	if cfg.BinaryName == "" {
		cfg.BinaryName = defaultBinaryName
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
	if cfg.Retain == 0 {
		cfg.Retain = defaultRetain
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
	if cfg.VersionURL == "" {
		cfg.VersionURL = defaultVersionURL
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

// --- on-disk layout helpers ------------------------------------------------

func (m *Manager) versionsDir() string        { return filepath.Join(m.cfg.InstallRoot, versionsSubdir) }
func (m *Manager) currentLink() string        { return filepath.Join(m.cfg.InstallRoot, currentName) }
func (m *Manager) previousLink() string       { return filepath.Join(m.cfg.InstallRoot, previousName) }
func (m *Manager) versionDir(v string) string { return filepath.Join(m.versionsDir(), v) }
func (m *Manager) versionBin(v string) string {
	return filepath.Join(m.versionDir(v), m.cfg.BinaryName)
}
func (m *Manager) currentBinary() string     { return filepath.Join(m.currentLink(), m.cfg.BinaryName) }
func relVersionTarget(v string) string       { return versionsSubdir + "/" + v }
func versionFromTarget(target string) string { return filepath.Base(target) }

// --- upgrade ---------------------------------------------------------------

func (m *Manager) RunUpgrade(checkOnly bool, binaryFile string) error {
	return m.RunUpgradeWithOptions(checkOnly, binaryFile, UpgradeOptions{})
}

// RunCheck reports whether a newer release is available without downloading or
// installing anything. It is the read-only form behind `velocity device check`.
func (m *Manager) RunCheck(opts UpgradeOptions) error {
	return m.RunUpgradeWithOptions(true, "", opts)
}

func (m *Manager) RunUpgradeWithOptions(checkOnly bool, binaryFile string, opts UpgradeOptions) error {
	if binaryFile != "" {
		if checkOnly {
			return fmt.Errorf("cannot combine check-only mode with a local binary upgrade")
		}
		return m.applyLocalBinary(binaryFile)
	}

	latest, assetURL, expectedSHA, err := m.fetchLatestRelease(opts.IncludePrereleases)
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
			channelLabel := "stable"
			if latestSV.isPrerelease() {
				channelLabel = "pre-release"
			}
			fmt.Fprintf(m.out, "Current: v%s\n", current)
			fmt.Fprintf(m.out, "Latest %s: v%s\n", channelLabel, latest)
			fmt.Fprintf(m.out, "\nCurrent version is newer than the latest %s release.\n", channelLabel)
			if currentSV.isPrerelease() && !opts.IncludePrereleases {
				fmt.Fprintln(m.out, "You are on a pre-release. Run 'sudo velocity device upgrade --prerelease' to check for newer pre-releases.")
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
		fmt.Fprintln(m.out, "\nRun 'sudo velocity device upgrade' to apply.")
		return nil
	}

	fmt.Fprintf(m.out, "Downloading from %s...\n", assetURL)
	tmpFile, err := m.downloadToTemp(assetURL, expectedSHA)
	if err != nil {
		return fmt.Errorf("downloading release: %w", err)
	}
	defer os.Remove(tmpFile)

	return m.applyVersionedUpgrade(latest, tmpFile)
}

func (m *Manager) RunBackup(outputDir string) (string, error) {
	if outputDir == "" {
		outputDir = m.cfg.BackupDir
	}
	return m.createBackupTo(outputDir)
}

// RunRollback flips `current` back to `previous` via one atomic symlink swap
// and restarts the service. It does NOT down-migrate the database: schema
// changes are forward-only, so if the active version applied a
// forward-incompatible migration the per-upgrade DB backup under BackupDir is
// the rescue path.
func (m *Manager) RunRollback() error {
	prevTarget, err := os.Readlink(m.previousLink())
	if err != nil {
		return fmt.Errorf("no previous version to roll back to (%s): %w", m.previousLink(), err)
	}
	curTarget, _ := os.Readlink(m.currentLink())

	fmt.Fprintln(m.err, "Note: rollback does not down-migrate the database. If the current "+
		"version applied a forward-incompatible migration, restore the matching "+
		"backup from "+m.cfg.BackupDir+" instead.")

	fmt.Fprintf(m.out, "Rolling back to %s\n", versionFromTarget(prevTarget))
	if err := swapCurrent(m.currentLink(), prevTarget); err != nil {
		return fmt.Errorf("activating previous version: %w", err)
	}
	// previous now points at what current used to be.
	_ = os.Remove(m.previousLink())
	if curTarget != "" {
		if err := os.Symlink(curTarget, m.previousLink()); err != nil {
			fmt.Fprintf(m.err, "warning: could not update previous symlink: %v\n", err)
		}
	}

	if err := m.systemctl("restart", m.cfg.ServiceName); err != nil {
		return fmt.Errorf("restarting service: %w", err)
	}
	m.verifyRunningVersion(versionFromTarget(prevTarget))
	fmt.Fprintf(m.out, "Rollback complete (now on %s).\n", versionFromTarget(prevTarget))
	return nil
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
	LinuxArm64 releaseAsset `json:"linux_arm64"`
	MacArm64   releaseAsset `json:"mac_arm64"`
	Visualiser releaseAsset `json:"visualiser"`
}

type releaseAsset struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
}

func (m *Manager) applyLocalBinary(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("binary file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("expected a file, got directory: %s", path)
	}

	// A locally-supplied binary carries no release version; stage it under a
	// timestamped manual version so it slots into the versioned layout.
	v := "local-" + m.now().UTC().Format("20060102-150405")
	fmt.Fprintf(m.out, "Applying local binary as version %s: %s\n", v, path)
	// The timestamp is an install-slot identifier, not the version reported by
	// the binary. Verify against the embedded version so successful offline
	// upgrades do not always stall for VerifyTimeout and emit a false warning.
	want := binaryReportedVersion(path)
	return m.applyVersionedUpgrade(v, path, want)
}

func binaryReportedVersion(path string) string {
	cmd := exec.Command(path)
	// The multi-call dispatcher keys off argv[0]. Local upgrade files commonly
	// have names such as velocity-candidate, so explicitly select the canonical
	// `velocity version` applet regardless of the source filename.
	cmd.Args = []string{"velocity", "version"}
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseReportedVersion(output)
}

func parseReportedVersion(output []byte) string {
	for _, field := range strings.Fields(string(output)) {
		if len(field) > 1 && field[0] == 'v' && field[1] >= '0' && field[1] <= '9' {
			return strings.TrimPrefix(field, "v")
		}
	}
	return ""
}

// applyVersionedUpgrade stages srcBinary as versions/<v>/<BinaryName>, runs the
// NEW binary's migrations before the swap, atomically activates the version,
// restarts the service, verifies the running version, and prunes old versions.
func (m *Manager) applyVersionedUpgrade(v, srcBinary string, reportedVersion ...string) error {
	vdir := m.versionDir(v)
	if err := os.MkdirAll(vdir, 0o755); err != nil {
		return fmt.Errorf("creating version dir %s: %w", vdir, err)
	}
	dst := m.versionBin(v)
	fmt.Fprintf(m.out, "Installing %s...\n", dst)
	if err := m.installBinary(srcBinary, dst); err != nil {
		return fmt.Errorf("installing binary: %w", err)
	}

	fmt.Fprintln(m.out, "Backing up database...")
	if _, err := m.createBackupTo(m.cfg.BackupDir); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	// Migrate with the NEW binary before the swap. The old service is still
	// running on the old binary; forward-only additive migrations are tolerated
	// by the old schema reader until the restart below.
	fmt.Fprintln(m.out, "Running database migrations (new binary)...")
	if err := m.runMigrations(dst); err != nil {
		return fmt.Errorf("migration failed; current version left unchanged: %w", err)
	}

	// Point of no return: atomic activation of the new version.
	fmt.Fprintln(m.out, "Activating new version...")
	if err := m.activate(v); err != nil {
		return fmt.Errorf("activating version %s: %w", v, err)
	}

	fmt.Fprintln(m.out, "Restarting velocity-report...")
	if err := m.systemctl("restart", m.cfg.ServiceName); err != nil {
		return fmt.Errorf("restarting service: %w", err)
	}

	want := v
	if len(reportedVersion) > 0 && reportedVersion[0] != "" {
		want = reportedVersion[0]
	}
	m.verifyRunningVersion(want)

	if err := m.prune(); err != nil {
		fmt.Fprintf(m.err, "warning: pruning old versions failed: %v\n", err)
	}

	fmt.Fprintf(m.out, "Upgrade complete (now on v%s).\n", v)
	return nil
}

// activate points current at versions/<v> atomically and repoints previous at
// the version current referenced before the swap.
func (m *Manager) activate(v string) error {
	newTarget := relVersionTarget(v)
	prevTarget, _ := os.Readlink(m.currentLink())
	if prevTarget == "" {
		// First activation: no current symlink yet.
		return os.Symlink(newTarget, m.currentLink())
	}
	if err := swapCurrent(m.currentLink(), newTarget); err != nil {
		return err
	}
	_ = os.Remove(m.previousLink())
	if err := os.Symlink(prevTarget, m.previousLink()); err != nil {
		fmt.Fprintf(m.err, "warning: could not update previous symlink: %v\n", err)
	}
	return nil
}

// prune removes version directories beyond the newest Retain, always keeping
// the targets of current and previous.
func (m *Manager) prune() error {
	entries, err := os.ReadDir(m.versionsDir())
	if err != nil {
		return err
	}

	keep := map[string]bool{}
	if t, err := os.Readlink(m.currentLink()); err == nil {
		keep[versionFromTarget(t)] = true
	}
	if t, err := os.Readlink(m.previousLink()); err == nil {
		keep[versionFromTarget(t)] = true
	}

	var versions []string
	for _, e := range entries {
		if e.IsDir() {
			versions = append(versions, e.Name())
		}
	}
	// Newest first: semver descending, with unparseable names sorted lexically
	// after parseable ones.
	sort.Slice(versions, func(i, j int) bool {
		vi, oki := parseSemver(versions[i])
		vj, okj := parseSemver(versions[j])
		switch {
		case oki && okj:
			return compareSemver(vi, vj) > 0
		case oki != okj:
			return oki
		default:
			return versions[i] > versions[j]
		}
	})

	kept := len(keep)
	for _, v := range versions {
		if keep[v] {
			continue
		}
		if kept < m.cfg.Retain {
			kept++
			continue
		}
		if err := os.RemoveAll(m.versionDir(v)); err != nil {
			fmt.Fprintf(m.err, "warning: removing old version %s: %v\n", v, err)
		}
	}
	return nil
}

// verifyRunningVersion polls GET /api/version until the running build matches
// want or VerifyTimeout elapses. It is best-effort: a mismatch or timeout logs
// a warning but is not fatal (the operator decides whether to roll back).
func (m *Manager) verifyRunningVersion(want string) {
	if m.cfg.VerifyTimeout <= 0 {
		return
	}
	deadline := m.now().Add(m.cfg.VerifyTimeout)
	for {
		got, err := m.fetchRunningVersion()
		if err == nil && got == want {
			fmt.Fprintf(m.out, "Service is running v%s.\n", got)
			return
		}
		if !m.now().Before(deadline) {
			fmt.Fprintf(m.err, "warning: could not confirm the running version is v%s within %s\n", want, m.cfg.VerifyTimeout)
			return
		}
		m.sleep(m.cfg.VerifyDelay)
	}
}

func (m *Manager) fetchRunningVersion() (string, error) {
	resp, err := m.httpClient.Get(m.cfg.VersionURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("/api/version returned %s", resp.Status)
	}
	var v struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	return v.Version, nil
}

// fetchLatestRelease fetches velocity.report/release.json and returns the
// version string, download URL, and expected SHA-256 hex digest for the
// appropriate channel and platform.
func (m *Manager) fetchLatestRelease(includePrereleases bool) (version, downloadURL, expectedSHA string, err error) {
	resp, err := m.httpClient.Get(m.cfg.ReleaseMetaURL)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("releases metadata returned %s", resp.Status)
	}

	var meta releasesMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return "", "", "", fmt.Errorf("parsing release.json: %w", err)
	}

	asset, err := pickAsset(meta.Stable, m.cfg.GOOS, m.cfg.GOARCH)
	if err != nil {
		return "", "", "", err
	}
	if includePrereleases {
		pre, err := pickAsset(meta.Prerelease, m.cfg.GOOS, m.cfg.GOARCH)
		if err == nil && pre.Version != "" && releaseAssetIsNewer(pre, asset) {
			asset = pre
		}
	}
	if asset.Version == "" {
		return "", "", "", fmt.Errorf("no version found in release.json for %s/%s", m.cfg.GOOS, m.cfg.GOARCH)
	}
	if asset.URL == "" {
		return "", "", "", fmt.Errorf("release %s has no download URL for %s/%s", asset.Version, m.cfg.GOOS, m.cfg.GOARCH)
	}
	return asset.Version, asset.URL, asset.SHA256, nil
}

func releaseAssetIsNewer(candidate, current releaseAsset) bool {
	if current.Version == "" {
		return true
	}

	candidateSV, candidateOK := parseSemver(candidate.Version)
	currentSV, currentOK := parseSemver(current.Version)
	switch {
	case candidateOK && currentOK:
		return compareSemver(candidateSV, currentSV) > 0
	case candidateOK != currentOK:
		return candidateOK
	default:
		return candidate.Version > current.Version
	}
}

// pickAsset returns the asset for the caller's platform. The device manager
// only upgrades the server binary; the visualiser is macOS-only and not an
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

// downloadToTemp downloads the binary at url, verifies its SHA-256 against
// expectedSHA (from release.json), and returns the path to a temp file. If
// expectedSHA is empty (older release metadata), verification is skipped with a
// warning.
func (m *Manager) downloadToTemp(url, expectedSHA string) (string, error) {
	client := &http.Client{Timeout: m.cfg.DownloadTimeout}
	resp, err := client.Get(url) //nolint:gosec // URL comes from release metadata
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %s", resp.Status)
	}

	tmp, err := os.CreateTemp("", "velocity-upgrade-*")
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

	if expectedSHA == "" {
		fmt.Fprintf(m.err, "warning: no expected SHA-256 in release metadata, skipping verification\n")
		return tmp.Name(), nil
	}

	if computed != expectedSHA {
		os.Remove(tmp.Name())
		lastSlash := strings.LastIndex(url, "/")
		binaryName := url[lastSlash+1:]
		return "", fmt.Errorf("SHA-256 mismatch for %s: expected %s, got %s", binaryName, expectedSHA, computed)
	}

	fmt.Fprintf(m.out, "SHA-256 verified.\n")
	return tmp.Name(), nil
}

// createBackupTo snapshots the live binary (resolved via the current symlink)
// and the database into a timestamped directory.
func (m *Manager) createBackupTo(baseDir string) (string, error) {
	ts := m.now().UTC().Format("20060102-150405")
	dest := filepath.Join(baseDir, ts)

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", fmt.Errorf("creating backup directory: %w", err)
	}

	if src := m.currentBinary(); fileExists(src) {
		if err := copyFile(src, filepath.Join(dest, m.cfg.BinaryName)); err != nil {
			return "", fmt.Errorf("backing up binary: %w", err)
		}
	}

	if fileExists(m.cfg.DBPath) {
		dbDst := filepath.Join(dest, "sensor_data.db")
		// VACUUM INTO captures a consistent snapshot even of a live WAL database
		// (the service is still running during an upgrade). A plain copy of the
		// main .db file would be stale or torn, which matters because this backup
		// is the rollback rescue path. Fall back to a raw copy only if the source
		// is not a usable SQLite file (e.g. an empty fixture), and say so.
		if err := db.BackupDatabase(m.cfg.DBPath, dbDst); err != nil {
			fmt.Fprintf(m.err, "warning: consistent DB backup failed (%v); falling back to a raw file copy\n", err)
			if err := copyFile(m.cfg.DBPath, dbDst); err != nil {
				return "", fmt.Errorf("backing up database: %w", err)
			}
		}
	}

	fmt.Fprintf(m.out, "Backup created: %s\n", dest)
	return dest, nil
}

// installBinary copies src into dst atomically (temp file in dst's directory,
// chmod 0755, rename).
func (m *Manager) installBinary(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".velocity-install-*")
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

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// runMigrations runs `migrate up` using the supplied binary (the newly-staged
// version), against the live database.
//
// Ownership note: the upgrade runs as root. In the normal flow the old service
// is still running and holds the database open in WAL mode, so the -wal/-shm
// files already exist and are owned by the `velocity` service user; root's
// writes append to those existing files without changing ownership, so the
// service can still write them after the restart. The only way this leaves
// root-owned WAL files behind is if the service is not running during the
// upgrade (no -wal to reuse) — operators should not stop the service before
// `velocity device upgrade`.
func (m *Manager) runMigrations(binPath string) error {
	return m.runner.Run(binPath, "data", "migrate", "up", "--db-path", m.cfg.DBPath)
}

func (m *Manager) systemctl(action, unit string) error {
	return m.runner.Run("systemctl", action, unit)
}
