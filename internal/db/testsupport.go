package db

import (
	"path/filepath"
	"testing"
)

// NewTestDB opens a temporary SQLite database via the canonical DB bootstrap
// path used in production. Tests in other packages should prefer this helper
// over re-implementing schema.sql execution, PRAGMAs, or migration baselines.
func NewTestDB(tb testing.TB) (*DB, func()) {
	tb.Helper()

	dbPath := filepath.Join(tb.TempDir(), "test.db")
	opened, err := NewDB(dbPath)
	if err != nil {
		tb.Fatalf("failed to create test DB: %v", err)
	}

	return opened, func() {
		_ = opened.Close()
	}
}
