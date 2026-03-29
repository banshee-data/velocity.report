package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/version"
)

const (
	githubRepo      = "banshee-data/velocity.report"
	releasesAPI     = "https://api.github.com/repos/" + githubRepo + "/releases/latest"
	binaryName      = "velocity-report"
	binaryPath      = "/usr/local/bin/" + binaryName
	serviceName     = "velocity-report.service"
	backupDir       = "/var/lib/velocity-report/backups"
	dbPath          = "/var/lib/velocity-report/sensor_data.db"
	requestTimeout  = 30 * time.Second
	downloadTimeout = 5 * time.Minute
)

// githubRelease is the subset of the GitHub Releases API response we need.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

// githubAsset represents a single release asset.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func runUpgrade(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ExitOnError)
	checkOnly := fs.Bool("check", false, "Check for updates without applying")
	binaryFile := fs.String("binary", "", "Apply a local binary file (offline upgrade)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Offline upgrade from local binary.
	if *binaryFile != "" {
		return applyLocalBinary(*binaryFile)
	}

	// Online upgrade: check GitHub releases.
	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	current := version.Version

	if latest == current {
		fmt.Printf("Already up to date (v%s).\n", current)
		return nil
	}

	fmt.Printf("Current: v%s\n", current)
	fmt.Printf("Latest:  v%s\n", latest)

	if *checkOnly {
		fmt.Println("\nRun 'sudo velocity-ctl upgrade' to apply.")
		return nil
	}

	// Find the correct asset for this architecture.
	assetName := fmt.Sprintf("velocity-report-%s-%s", runtime.GOOS, runtime.GOARCH)
	var assetURL string
	for _, a := range release.Assets {
		if a.Name == assetName {
			assetURL = a.BrowserDownloadURL
			break
		}
	}
	if assetURL == "" {
		// Try the ARM64-specific naming convention used in CI.
		for _, a := range release.Assets {
			if a.Name == "velocity-report-arm64" {
				assetURL = a.BrowserDownloadURL
				break
			}
		}
	}
	if assetURL == "" {
		return fmt.Errorf("no binary asset found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	// Download to a temporary file.
	fmt.Printf("Downloading %s...\n", assetURL)
	tmpFile, err := downloadToTemp(assetURL)
	if err != nil {
		return fmt.Errorf("downloading release: %w", err)
	}
	defer os.Remove(tmpFile)

	// Backup, stop, install, start.
	return applyUpgrade(tmpFile)
}

func applyLocalBinary(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("binary file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("expected a file, got directory: %s", path)
	}
	fmt.Printf("Applying local binary: %s\n", path)
	return applyUpgrade(path)
}

func applyUpgrade(newBinaryPath string) error {
	// 1. Backup current state.
	fmt.Println("Creating backup...")
	backupPath, err := createBackup()
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}
	fmt.Printf("Backup saved to %s\n", backupPath)

	// 2. Stop the service.
	fmt.Println("Stopping velocity-report...")
	if err := systemctl("stop", serviceName); err != nil {
		return fmt.Errorf("stopping service: %w", err)
	}

	// 3. Replace the binary.
	fmt.Println("Installing new binary...")
	if err := installBinary(newBinaryPath, binaryPath); err != nil {
		// Attempt rollback on failure.
		fmt.Fprintf(os.Stderr, "install failed: %v — attempting rollback\n", err)
		_ = restoreBackup(backupPath)
		_ = systemctl("start", serviceName)
		return fmt.Errorf("installing binary: %w", err)
	}

	// 4. Run database migrations.
	fmt.Println("Running database migrations...")
	if err := runMigrations(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: migration failed: %v\n", err)
		// Non-fatal — the new binary may not have new migrations.
	}

	// 5. Start the service.
	fmt.Println("Starting velocity-report...")
	if err := systemctl("start", serviceName); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}

	// 6. Verify.
	fmt.Println("Verifying service status...")
	time.Sleep(2 * time.Second)
	if err := systemctl("is-active", serviceName); err != nil {
		fmt.Fprintf(os.Stderr, "warning: service may not be running — check 'velocity-ctl status'\n")
	} else {
		fmt.Println("Upgrade complete. Service is active.")
	}

	return nil
}

func fetchLatestRelease() (*githubRelease, error) {
	client := &http.Client{Timeout: requestTimeout}
	resp, err := client.Get(releasesAPI)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parsing release JSON: %w", err)
	}
	return &release, nil
}

func downloadToTemp(url string) (string, error) {
	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(url) //nolint:gosec // URL is from GitHub Releases API, not user input
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
	tmp.Close()

	checksum := hex.EncodeToString(h.Sum(nil))
	fmt.Printf("Downloaded %d bytes (SHA-256: %s)\n", written, checksum)

	return tmp.Name(), nil
}

func installBinary(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Write atomically via temp file in the same directory.
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

	// Ensure data is flushed to disk before rename.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	tmp.Close()

	if err := os.Chmod(tmpName, 0755); err != nil {
		os.Remove(tmpName)
		return err
	}

	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return err
	}

	return nil
}

func runMigrations() error {
	cmd := exec.Command(binaryPath, "migrate", "up", "--db-path", dbPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func systemctl(action, unit string) error {
	cmd := exec.Command("systemctl", action, unit)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
