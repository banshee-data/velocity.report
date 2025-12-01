package main

import (
	"fmt"
	"strings"
)

// Fixer handles repairing broken installations
type Fixer struct {
	Target        string
	SSHUser       string
	SSHKey        string
	IdentityAgent string
	BinaryPath    string
	DryRun        bool
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

	// Check 3: Binary exists and is executable
	if err := f.checkBinary(exec); err != nil {
		fmt.Printf("❌ Binary issue: %v\n", err)
		if f.BinaryPath != "" {
			fmt.Println("   Attempting to fix...")
			if err := f.fixBinary(exec); err != nil {
				fmt.Printf("   ⚠️  Could not fix: %v\n\n", err)
				issues++
			} else {
				fmt.Println("   ✅ Fixed")
				fixed++
			}
		} else {
			fmt.Println("   ⚠️  No binary provided (use --binary flag to fix)")
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

	// Check 6: Database exists (warning only, don't try to fix)
	if err := f.checkDatabase(exec); err != nil {
		fmt.Printf("⚠️  Database issue: %v\n", err)
		fmt.Println("   Note: Database must be created by running the service")
	} else {
		fmt.Println("✅ Database exists")
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

	// Check ownership
	debugLog("Checking directory ownership")
	ownerOutput, err := exec.Run("stat -c '%U:%G' /var/lib/velocity-report 2>/dev/null || stat -f '%Su:%Sg' /var/lib/velocity-report 2>/dev/null")
	if err == nil && !strings.Contains(ownerOutput, "velocity") {
		return fmt.Errorf("directory has incorrect ownership: %s (expected velocity:velocity)", strings.TrimSpace(ownerOutput))
	}

	return nil
}

func (f *Fixer) fixDataDirectory(exec *Executor) error {
	debugLog("Creating and fixing data directory")
	_, err := exec.RunSudo("mkdir -p /var/lib/velocity-report && chown velocity:velocity /var/lib/velocity-report && chmod 755 /var/lib/velocity-report")
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
