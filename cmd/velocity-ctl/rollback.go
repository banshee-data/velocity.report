package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runRollback(args []string) error {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Find the most recent backup.
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return fmt.Errorf("reading backup directory %s: %w", backupDir, err)
	}

	// Filter to backup directories (timestamped).
	var backups []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "20") {
			backups = append(backups, e.Name())
		}
	}

	if len(backups) == 0 {
		return fmt.Errorf("no backups found in %s", backupDir)
	}

	sort.Strings(backups)
	latest := backups[len(backups)-1]
	backupPath := filepath.Join(backupDir, latest)

	fmt.Printf("Rolling back to backup: %s\n", latest)
	return restoreBackup(backupPath)
}

func restoreBackup(backupPath string) error {
	// Check backup contains the expected files.
	binaryBackup := filepath.Join(backupPath, binaryName)
	if _, err := os.Stat(binaryBackup); err != nil {
		return fmt.Errorf("backup binary not found at %s: %w", binaryBackup, err)
	}

	// Stop service.
	fmt.Println("Stopping velocity-report...")
	if err := systemctl("stop", serviceName); err != nil {
		return fmt.Errorf("stopping service: %w", err)
	}

	// Restore binary.
	fmt.Println("Restoring binary...")
	if err := installBinary(binaryBackup, binaryPath); err != nil {
		return fmt.Errorf("restoring binary: %w", err)
	}

	// Restore database if backup exists.
	dbBackup := filepath.Join(backupPath, "sensor_data.db")
	if _, err := os.Stat(dbBackup); err == nil {
		fmt.Println("Restoring database...")
		if err := installBinary(dbBackup, dbPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: database restore failed: %v\n", err)
		}
	}

	// Start service.
	fmt.Println("Starting velocity-report...")
	if err := systemctl("start", serviceName); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}

	fmt.Printf("Rollback complete (restored from %s).\n", filepath.Base(backupPath))
	return nil
}
