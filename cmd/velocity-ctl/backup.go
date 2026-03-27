package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	outputDir := fs.String("output", backupDir, "Directory to store backups")
	if err := fs.Parse(args); err != nil {
		return err
	}

	_, err := createBackupTo(*outputDir)
	return err
}

// createBackup creates a timestamped backup in the default backup directory.
// Returns the path to the backup directory.
func createBackup() (string, error) {
	return createBackupTo(backupDir)
}

func createBackupTo(baseDir string) (string, error) {
	ts := time.Now().UTC().Format("20060102-150405")
	dest := filepath.Join(baseDir, ts)

	if err := os.MkdirAll(dest, 0755); err != nil {
		return "", fmt.Errorf("creating backup directory: %w", err)
	}

	// Back up the binary.
	if err := copyFile(binaryPath, filepath.Join(dest, binaryName)); err != nil {
		return "", fmt.Errorf("backing up binary: %w", err)
	}

	// Back up the database (if it exists).
	if _, err := os.Stat(dbPath); err == nil {
		if err := copyFile(dbPath, filepath.Join(dest, "sensor_data.db")); err != nil {
			return "", fmt.Errorf("backing up database: %w", err)
		}
	}

	fmt.Printf("Backup created: %s\n", dest)
	return dest, nil
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
