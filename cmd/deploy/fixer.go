package main

import (
	"fmt"
	"strings"
)

// Fixer handles repairing broken installations
type Fixer struct {
	Target          string
	SSHUser         string
	SSHKey          string
	IdentityAgent   string
	BinaryPath      string
	RepoURL         string
	BuildFromSource bool
	DryRun          bool
}

// Fix performs a comprehensive repair of the installation
func (f *Fixer) Fix() error {
	exec := NewExecutor(f.Target, f.SSHUser, f.SSHKey, f.IdentityAgent, f.DryRun)

	fmt.Println("🔧 Starting installation repair...")

	issues := 0
	fixed := 0

	// Check 1: Service user exists
	if err := f.checkServiceUser(exec); err != nil {
		fmt.Printf("❌ Service user issue: %v\n", err)
		fmt.Println("   Attempting to fix...")
		if err := f.fixServiceUser(exec); err != nil {
			fmt.Printf("   ⚠️  Could not fix: %v\n\n", err)
			issues++
		} else {
			fmt.Println("   ✅ Fixed")
			fixed++
		}
	} else {
		fmt.Println("✅ Service user 'velocity' exists")
	}

	// Check 2: Data directory exists with correct permissions
	if err := f.checkDataDirectory(exec); err != nil {
		fmt.Printf("❌ Data directory issue: %v\n", err)
		fmt.Println("   Attempting to fix...")
		if err := f.fixDataDirectory(exec); err != nil {
			fmt.Printf("   ⚠️  Could not fix: %v\n\n", err)
			issues++
		} else {
			fmt.Println("   ✅ Fixed")
			fixed++
		}
	} else {
		fmt.Println("✅ Data directory exists with correct permissions")
	}

	// Check 3: Source code repository (for Python scripts and optional builds)
	sourceDir := "/opt/velocity-report"
	if err := f.checkSourceCode(exec, sourceDir); err != nil {
		fmt.Printf("⚠️  Source code: %v\n", err)
		fmt.Println("   Attempting to clone/update...")
		if err := f.fixSourceCode(exec, sourceDir); err != nil {
			fmt.Printf("   ⚠️  Could not fix: %v\n", err)
			fmt.Println("   Note: Source code is needed for Python PDF generation scripts")
		} else {
			fmt.Println("   ✅ Fixed - source code cloned")
			fixed++
		}
	} else {
		fmt.Println("✅ Source code exists")
	}

	// Check 4: Binary exists and is executable
	if err := f.checkBinary(exec); err != nil {
		fmt.Printf("❌ Binary issue: %v\n", err)
		if f.BuildFromSource {
			fmt.Println("   Attempting to build from source...")
			if err := f.buildBinaryFromSource(exec, sourceDir); err != nil {
				fmt.Printf("   ⚠️  Could not build: %v\n\n", err)
				issues++
			} else {
				fmt.Println("   ✅ Built and installed from source")
				fixed++
			}
		} else if f.BinaryPath != "" {
			fmt.Println("   Attempting to fix...")
			if err := f.fixBinary(exec); err != nil {
				fmt.Printf("   ⚠️  Could not fix: %v\n\n", err)
				issues++
			} else {
				fmt.Println("   ✅ Fixed")
				fixed++
			}
		} else {
			fmt.Println("   ⚠️  No binary provided (use --binary or --build-from-source flag)")
			issues++
		}
	} else {
		fmt.Println("✅ Binary exists and is executable")
	}

	// Check 4: Systemd service file exists
	if err := f.checkServiceFile(exec); err != nil {
		fmt.Printf("❌ Service file issue: %v\n", err)
		fmt.Println("   Attempting to fix...")
		if err := f.fixServiceFile(exec); err != nil {
			fmt.Printf("   ⚠️  Could not fix: %v\n\n", err)
			issues++
		} else {
			fmt.Println("   ✅ Fixed")
			fixed++
		}
	} else {
		fmt.Println("✅ Systemd service file exists")
	}

	// Check 5: Service is enabled
	if err := f.checkServiceEnabled(exec); err != nil {
		fmt.Printf("❌ Service not enabled: %v\n", err)
		fmt.Println("   Attempting to fix...")
		if err := f.fixServiceEnabled(exec); err != nil {
			fmt.Printf("   ⚠️  Could not fix: %v\n\n", err)
			issues++
		} else {
			fmt.Println("   ✅ Fixed")
			fixed++
		}
	} else {
		fmt.Println("✅ Service is enabled")
	}

	// Check 6: Database exists and is in correct location
	dbLocation, err := f.findDatabase(exec)
	if err != nil {
		fmt.Printf("⚠️  Database issue: %v\n", err)
		fmt.Println("   Note: Database must be created by running the service")
	} else if dbLocation != "/var/lib/velocity-report/sensor_data.db" {
		fmt.Printf("⚠️  Database found in wrong location: %s\n", dbLocation)
		fmt.Println("   Attempting to fix...")
		if err := f.fixDatabaseLocation(exec, dbLocation); err != nil {
			fmt.Printf("   ⚠️  Could not fix: %v\n\n", err)
			issues++
		} else {
			fmt.Println("   ✅ Fixed - database moved to correct location")
			fixed++
		}
	} else {
		fmt.Println("✅ Database exists in correct location")
	}

	// Check 7: Service configuration uses correct db-path
	if err := f.checkServiceConfig(exec); err != nil {
		fmt.Printf("❌ Service configuration issue: %v\n", err)
		fmt.Println("   Attempting to fix...")
		if err := f.fixServiceConfig(exec); err != nil {
			fmt.Printf("   ⚠️  Could not fix: %v\n\n", err)
			issues++
		} else {
			fmt.Println("   ✅ Fixed")
			fixed++
		}
	} else {
		fmt.Println("✅ Service configured with correct --db-path")
	}

	// Check 8: Database schema is up to date (warning only)
	if dbLocation != "" {
		if err := f.checkDatabaseSchema(exec); err != nil {
			fmt.Printf("⚠️  Database schema issue: %v\n", err)
			fmt.Println("   Run manually: velocity-report migrate baseline <version> && velocity-report migrate up")
		} else {
			fmt.Println("✅ Database schema is up to date")
		}
	}

	// Summary
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if issues == 0 {
		fmt.Println("✅ All checks passed! Installation is healthy.")
		if fixed > 0 {
			fmt.Printf("   Repaired %d issue(s)\n", fixed)
		}
	} else {
		fmt.Printf("⚠️  %d issue(s) remain. See details above.\n", issues)
		if fixed > 0 {
			fmt.Printf("   Successfully repaired %d issue(s)\n", fixed)
		}
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Try to start service if all critical checks passed
	if issues == 0 {
		fmt.Println("\n🚀 Attempting to start service...")
		if err := f.startService(exec); err != nil {
			fmt.Printf("⚠️  Could not start service: %v\n", err)
			fmt.Println("   Check logs with: sudo journalctl -u velocity-report.service -n 50")
		} else {
			fmt.Println("✅ Service started successfully")
		}
	}

	if issues > 0 {
		return fmt.Errorf("%d issue(s) could not be fixed", issues)
	}
	return nil
}

func (f *Fixer) checkServiceUser(exec *Executor) error {
	debugLog("Checking if service user 'velocity' exists")
	output, err := exec.Run("id velocity >/dev/null 2>&1 && echo 'exists' || echo 'not found'")
	if err != nil {
		return fmt.Errorf("failed to check user: %w", err)
	}
	if !strings.Contains(output, "exists") {
		return fmt.Errorf("user 'velocity' does not exist")
	}
	return nil
}

func (f *Fixer) fixServiceUser(exec *Executor) error {
	debugLog("Creating service user 'velocity'")
	output, err := exec.RunSudo("useradd --system --no-create-home --shell /usr/sbin/nologin velocity 2>&1 || true")
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	// Check if error was because user already exists (exit 9)
	if strings.Contains(output, "already exists") {
		debugLog("User already exists, continuing")
		return nil
	}
	// Verify user was created
	return f.checkServiceUser(exec)
}

func (f *Fixer) checkDataDirectory(exec *Executor) error {
	debugLog("Checking data directory /var/lib/velocity-report")
	output, err := exec.Run("test -d /var/lib/velocity-report && echo 'exists' || echo 'not found'")
	if err != nil {
		return fmt.Errorf("failed to check directory: %w", err)
	}
	if strings.TrimSpace(output) != "exists" {
		return fmt.Errorf("directory /var/lib/velocity-report does not exist")
	}

	// Get the actual service user from systemd config
	debugLog("Checking service user configuration")
	serviceUserOutput, err := exec.Run("systemctl show velocity-report.service -p User --value 2>/dev/null || echo 'velocity'")
	serviceUser := strings.TrimSpace(serviceUserOutput)
	if serviceUser == "" {
		serviceUser = "velocity" // fallback to default
	}

	// Check ownership
	debugLog("Checking directory ownership (expecting %s)", serviceUser)
	ownerOutput, err := exec.Run("stat -c '%U:%G' /var/lib/velocity-report 2>/dev/null || stat -f '%Su:%Sg' /var/lib/velocity-report 2>/dev/null")
	if err == nil && !strings.Contains(ownerOutput, serviceUser) {
		return fmt.Errorf("directory has incorrect ownership: %s (expected %s:%s)", strings.TrimSpace(ownerOutput), serviceUser, serviceUser)
	}

	return nil
}

func (f *Fixer) fixDataDirectory(exec *Executor) error {
	debugLog("Creating and fixing data directory")

	// Get the actual service user from systemd config
	serviceUserOutput, err := exec.Run("systemctl show velocity-report.service -p User --value 2>/dev/null || echo 'velocity'")
	if err != nil {
		return fmt.Errorf("failed to get service user: %w", err)
	}
	serviceUser := strings.TrimSpace(serviceUserOutput)
	if serviceUser == "" {
		serviceUser = "velocity" // fallback to default
	}

	debugLog("Fixing data directory with user: %s", serviceUser)
	_, err = exec.RunSudo(fmt.Sprintf("mkdir -p /var/lib/velocity-report && chown %s:%s /var/lib/velocity-report && chmod 755 /var/lib/velocity-report", serviceUser, serviceUser))
	if err != nil {
		return fmt.Errorf("failed to fix directory: %w", err)
	}
	return nil
}

func (f *Fixer) checkBinary(exec *Executor) error {
	debugLog("Checking binary at /usr/local/bin/velocity-report")
	output, err := exec.Run("test -f /usr/local/bin/velocity-report && echo 'exists' || echo 'not found'")
	if err != nil {
		return fmt.Errorf("failed to check binary: %w", err)
	}
	if strings.TrimSpace(output) != "exists" {
		return fmt.Errorf("binary /usr/local/bin/velocity-report does not exist")
	}

	// Check if executable
	debugLog("Checking binary is executable")
	execOutput, err := exec.Run("test -x /usr/local/bin/velocity-report && echo 'executable' || echo 'not executable'")
	if err == nil && strings.TrimSpace(execOutput) != "executable" {
		return fmt.Errorf("binary exists but is not executable")
	}

	return nil
}

func (f *Fixer) fixBinary(exec *Executor) error {
	debugLog("Installing binary from %s", f.BinaryPath)

	// Copy binary to temp location
	tempPath := "/tmp/velocity-report-fix"
	if err := exec.CopyFile(f.BinaryPath, tempPath); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Move to install path and set permissions
	_, err := exec.RunSudo(fmt.Sprintf("mv %s /usr/local/bin/velocity-report && chown root:root /usr/local/bin/velocity-report && chmod 0755 /usr/local/bin/velocity-report", tempPath))
	if err != nil {
		return fmt.Errorf("failed to install binary: %w", err)
	}

	return nil
}

func (f *Fixer) checkServiceFile(exec *Executor) error {
	debugLog("Checking service file at /etc/systemd/system/velocity-report.service")
	output, err := exec.Run("test -f /etc/systemd/system/velocity-report.service && echo 'exists' || echo 'not found'")
	if err != nil {
		return fmt.Errorf("failed to check service file: %w", err)
	}
	if strings.TrimSpace(output) != "exists" {
		return fmt.Errorf("service file does not exist")
	}
	return nil
}

func (f *Fixer) fixServiceFile(exec *Executor) error {
	debugLog("Installing systemd service file")

	// Write service file to temp location
	tempFile := "/tmp/velocity-report.service"
	if err := exec.WriteFile(tempFile, serviceContent); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Move to systemd directory and reload
	_, err := exec.RunSudo(fmt.Sprintf("mv %s /etc/systemd/system/velocity-report.service && systemctl daemon-reload", tempFile))
	if err != nil {
		return fmt.Errorf("failed to install service file: %w", err)
	}

	return nil
}

func (f *Fixer) checkServiceEnabled(exec *Executor) error {
	debugLog("Checking if service is enabled")
	output, err := exec.RunSudo("systemctl is-enabled velocity-report.service 2>/dev/null || echo 'disabled'")
	if err != nil {
		return fmt.Errorf("failed to check if service is enabled: %w", err)
	}
	if strings.TrimSpace(output) != "enabled" {
		return fmt.Errorf("service is not enabled")
	}
	return nil
}

func (f *Fixer) fixServiceEnabled(exec *Executor) error {
	debugLog("Enabling service")
	_, err := exec.RunSudo("systemctl enable velocity-report.service")
	if err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}
	return nil
}

func (f *Fixer) checkDatabase(exec *Executor) error {
	debugLog("Checking database at /var/lib/velocity-report/sensor_data.db")
	output, err := exec.Run("test -f /var/lib/velocity-report/sensor_data.db && echo 'exists' || echo 'not found'")
	if err != nil {
		return fmt.Errorf("failed to check database: %w", err)
	}
	if strings.TrimSpace(output) != "exists" {
		return fmt.Errorf("database does not exist (will be created on first run)")
	}
	return nil
}

func (f *Fixer) findDatabase(exec *Executor) (string, error) {
	debugLog("Searching for sensor_data.db in common locations")

	// Check expected location first
	if output, _ := exec.Run("test -f /var/lib/velocity-report/sensor_data.db && echo '/var/lib/velocity-report/sensor_data.db'"); strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	// Check working directory (common misconfiguration)
	if output, _ := exec.Run("test -f /home/*/code/velocity.report/sensor_data.db && find /home/*/code/velocity.report -name 'sensor_data.db' 2>/dev/null | head -1"); strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	// Check old user directory
	if output, _ := exec.Run("test -f /home/david/code/velocity.report/sensor_data.db && echo '/home/david/code/velocity.report/sensor_data.db'"); strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output), nil
	}

	return "", fmt.Errorf("database not found in any expected location")
}

func (f *Fixer) fixDatabaseLocation(exec *Executor, currentLocation string) error {
	debugLog("Moving database from %s to /var/lib/velocity-report/sensor_data.db", currentLocation)

	// Stop service first
	if _, err := exec.RunSudo("systemctl stop velocity-report.service 2>/dev/null || true"); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	// Ensure target directory exists
	if err := f.fixDataDirectory(exec); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Move database
	cmd := fmt.Sprintf("mv %s /var/lib/velocity-report/sensor_data.db && chown velocity:velocity /var/lib/velocity-report/sensor_data.db", currentLocation)
	if _, err := exec.RunSudo(cmd); err != nil {
		return fmt.Errorf("failed to move database: %w", err)
	}

	return nil
}

func (f *Fixer) checkServiceConfig(exec *Executor) error {
	debugLog("Checking service ExecStart configuration")
	output, err := exec.Run("grep 'ExecStart=' /etc/systemd/system/velocity-report.service 2>/dev/null")
	if err != nil {
		return fmt.Errorf("failed to read service file: %w", err)
	}

	if !strings.Contains(output, "--db-path /var/lib/velocity-report/sensor_data.db") {
		return fmt.Errorf("service not configured with correct --db-path")
	}

	return nil
}

func (f *Fixer) fixServiceConfig(exec *Executor) error {
	debugLog("Updating service configuration with --db-path flag")

	// Update ExecStart line to include --db-path
	cmd := `sed -i 's|^ExecStart=/usr/local/bin/velocity-report$|ExecStart=/usr/local/bin/velocity-report --db-path /var/lib/velocity-report/sensor_data.db|' /etc/systemd/system/velocity-report.service`
	if _, err := exec.RunSudo(cmd); err != nil {
		return fmt.Errorf("failed to update service file: %w", err)
	}

	// Reload systemd
	if _, err := exec.RunSudo("systemctl daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	return nil
}

func (f *Fixer) checkDatabaseSchema(exec *Executor) error {
	debugLog("Checking database schema version")

	// Check if migrations table exists
	output, err := exec.Run("sqlite3 /var/lib/velocity-report/sensor_data.db \"SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations';\" 2>/dev/null")
	if err != nil || strings.TrimSpace(output) == "" {
		return fmt.Errorf("no migrations table found - needs baseline")
	}

	// Check current version
	versionOutput, err := exec.Run("sqlite3 /var/lib/velocity-report/sensor_data.db \"SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1;\" 2>/dev/null")
	if err != nil {
		return fmt.Errorf("could not read schema version: %w", err)
	}

	if strings.Contains(versionOutput, "1|") {
		return fmt.Errorf("schema is dirty (version: %s)", strings.TrimSpace(versionOutput))
	}

	return nil
}

func (f *Fixer) checkSourceCode(exec *Executor, sourceDir string) error {
	debugLog("Checking source code at %s", sourceDir)
	output, err := exec.Run(fmt.Sprintf("test -d %s/.git && echo 'exists' || echo 'not found'", sourceDir))
	if err != nil {
		return fmt.Errorf("failed to check source directory: %w", err)
	}
	if strings.TrimSpace(output) != "exists" {
		return fmt.Errorf("source code not found at %s", sourceDir)
	}
	return nil
}

func (f *Fixer) fixSourceCode(exec *Executor, sourceDir string) error {
	debugLog("Cloning/updating source code from %s to %s", f.RepoURL, sourceDir)

	// Check if git is installed
	if _, err := exec.Run("command -v git >/dev/null 2>&1"); err != nil {
		return fmt.Errorf("git is not installed on target system")
	}

	// Check if directory exists
	checkOutput, _ := exec.Run(fmt.Sprintf("test -d %s && echo 'exists' || echo 'not found'", sourceDir))

	if strings.Contains(checkOutput, "exists") {
		// Directory exists - try to update
		debugLog("Source directory exists, attempting git pull")
		if _, err := exec.RunSudo(fmt.Sprintf("cd %s && git fetch origin && git reset --hard origin/main", sourceDir)); err != nil {
			fmt.Println("   ⚠️  Could not update, attempting fresh clone...")
			if _, err := exec.RunSudo(fmt.Sprintf("rm -rf %s", sourceDir)); err != nil {
				return fmt.Errorf("failed to remove old source: %w", err)
			}
		} else {
			return nil
		}
	}

	// Clone fresh
	debugLog("Cloning repository")
	if _, err := exec.RunSudo(fmt.Sprintf("git clone %s %s", f.RepoURL, sourceDir)); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	// Set permissions
	if _, err := exec.RunSudo(fmt.Sprintf("chown -R velocity:velocity %s", sourceDir)); err != nil {
		debugLog("Warning: Could not set ownership on source directory: %v", err)
	}

	return nil
}

func (f *Fixer) buildBinaryFromSource(exec *Executor, sourceDir string) error {
	debugLog("Building binary from source at %s", sourceDir)

	// Check if Go is installed
	goVersion, err := exec.Run("go version 2>/dev/null")
	if err != nil {
		return fmt.Errorf("Go is not installed on target system. Install Go 1.21+ or use --binary flag instead")
	}
	debugLog("Found Go: %s", strings.TrimSpace(goVersion))

	// Check for libpcap-dev (needed for pcap builds)
	if output, _ := exec.Run("dpkg -l | grep libpcap-dev 2>/dev/null"); strings.TrimSpace(output) == "" {
		fmt.Println("   ⚠️  libpcap-dev not found - attempting to install...")
		if _, err := exec.RunSudo("apt-get update && apt-get install -y libpcap-dev"); err != nil {
			fmt.Println("   ⚠️  Could not install libpcap-dev, building without pcap support")
		}
	}

	// Build the binary
	debugLog("Building radar binary")
	buildCmd := fmt.Sprintf("cd %s && go build -o /tmp/velocity-report-new ./cmd/radar", sourceDir)
	if output, err := exec.Run(buildCmd); err != nil {
		return fmt.Errorf("build failed: %w\nOutput: %s", err, output)
	}

	// Stop service before replacing binary
	debugLog("Stopping service for binary replacement")
	exec.RunSudo("systemctl stop velocity-report.service 2>/dev/null || true")

	// Install the binary
	debugLog("Installing new binary")
	if _, err := exec.RunSudo("mv /tmp/velocity-report-new /usr/local/bin/velocity-report && chown root:root /usr/local/bin/velocity-report && chmod 0755 /usr/local/bin/velocity-report"); err != nil {
		return fmt.Errorf("failed to install binary: %w", err)
	}

	// Verify binary has --db-path flag
	if output, _ := exec.Run("/usr/local/bin/velocity-report --help 2>&1 | grep -q 'db-path' && echo 'ok'"); strings.TrimSpace(output) != "ok" {
		fmt.Println("   ⚠️  Warning: Built binary may not support --db-path flag")
	}

	return nil
}

func (f *Fixer) startService(exec *Executor) error {
	debugLog("Starting velocity-report service")
	_, err := exec.RunSudo("systemctl start velocity-report.service")
	if err != nil {
		return err
	}

	// Wait and check if active
	exec.Run("sleep 2")
	output, err := exec.RunSudo("systemctl is-active velocity-report.service")
	if err != nil || strings.TrimSpace(output) != "active" {
		return fmt.Errorf("service did not start properly (status: %s)", strings.TrimSpace(output))
	}

	return nil
}
