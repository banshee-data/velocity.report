package api

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
)

var (
	apiTestTemplatePath string
)

func TestMain(m *testing.M) {
	code := runAPITestMain(m)
	os.Exit(code)
}

func runAPITestMain(m *testing.M) int {
	tmpDir, err := os.MkdirTemp("", "velocity-api-template-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create API test template directory: %v\n", err)
		return 1
	}

	apiTestTemplatePath = filepath.Join(tmpDir, "template.db")

	templateDB, err := db.NewDB(apiTestTemplatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize API test template DB: %v\n", err)
		_ = os.RemoveAll(tmpDir)
		return 1
	}

	if _, err := templateDB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		fmt.Fprintf(os.Stderr, "failed to checkpoint API test template DB: %v\n", err)
		_ = templateDB.Close()
		_ = os.RemoveAll(tmpDir)
		return 1
	}

	if err := templateDB.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to close API test template DB: %v\n", err)
		_ = os.RemoveAll(tmpDir)
		return 1
	}

	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	return code
}

func cloneAPITestDB(t *testing.T) string {
	t.Helper()

	if apiTestTemplatePath == "" {
		t.Fatal("API test template DB not initialized")
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	if err := copyFile(apiTestTemplatePath, dbPath); err != nil {
		t.Fatalf("failed to clone API test DB template: %v", err)
	}

	return dbPath
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}

	if err := out.Close(); err != nil {
		return err
	}

	return nil
}
