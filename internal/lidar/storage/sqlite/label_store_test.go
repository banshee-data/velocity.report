package sqlite

import (
	"strings"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

func TestLabelStore_UpdateLabel_NilUpdates(t *testing.T) {
	db, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	store := NewLabelStore(db)
	err := store.UpdateLabel("label-1", nil)
	if err == nil {
		t.Fatal("expected error for nil updates")
	}
	if !strings.Contains(err.Error(), "updates cannot be nil") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLabelStore_UpdateLabel_MissingUpdatedAtNs(t *testing.T) {
	db, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	store := NewLabelStore(db)
	err := store.UpdateLabel("label-1", &LidarLabel{})
	if err == nil {
		t.Fatal("expected error for missing UpdatedAtNs")
	}
	if !strings.Contains(err.Error(), "UpdatedAtNs must be set") {
		t.Fatalf("unexpected error: %v", err)
	}
}
