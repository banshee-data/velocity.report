package db

import (
	"io/fs"
	"testing"
)

// TestEmbeddedMigrationsFS verifies the embedded migrations filesystem structure
func TestEmbeddedMigrationsFS(t *testing.T) {
	// Test with DevMode off (embedded FS)
	origDevMode := DevMode
	DevMode = false
	defer func() { DevMode = origDevMode }()

	// List root of migrationsFS
	t.Log("Listing root of embedded migrationsFS:")
	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		t.Fatalf("Failed to read root of migrationsFS: %v", err)
	}
	for _, entry := range entries {
		t.Logf("  %s (dir: %v)", entry.Name(), entry.IsDir())
	}

	// Try reading the migrations subdirectory
	t.Log("\nListing migrations/ subdirectory:")
	entries, err = fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("Failed to read migrations/ subdirectory: %v", err)
	}
	for i, entry := range entries {
		if i < 5 { // Show first 5
			t.Logf("  %s", entry.Name())
		}
	}
	t.Logf("  ... (%d total files)", len(entries))

	// Test getMigrationsFS
	t.Log("\nTesting getMigrationsFS():")
	migFS, err := getMigrationsFS()
	if err != nil {
		t.Fatalf("getMigrationsFS() failed: %v", err)
	}

	entries, err = fs.ReadDir(migFS, ".")
	if err != nil {
		t.Fatalf("Failed to read getMigrationsFS result: %v", err)
	}
	t.Logf("getMigrationsFS() returned %d entries", len(entries))
	for i, entry := range entries {
		if i < 5 { // Show first 5
			t.Logf("  %s", entry.Name())
		}
	}
}
