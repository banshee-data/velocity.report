package db

import (
	"path/filepath"
	"sync"
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
	if _, err := opened.Exec("PRAGMA foreign_keys = ON"); err != nil {
		tb.Fatalf("failed to enable foreign_keys for test DB: %v", err)
	}

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			_ = opened.Close()
		})
	}
	tb.Cleanup(cleanup)

	return opened, cleanup
}
