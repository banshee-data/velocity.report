package eligibilitytest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitycli"
	"github.com/banshee-data/velocity.report/internal/testutil"
)

// ConfigureCLI gives a test its own real manifest, frozen-split registry and
// migrated use ledger. It grants only the named fixture's bytes.
func ConfigureCLI(t *testing.T, capture, cohort string) string {
	t.Helper()
	root := t.TempDir()
	testutil.AssertNoError(t, os.MkdirAll(filepath.Join(root, "splits"), 0755))
	id, err := eligibility.NewHasher().Capture(capture)
	testutil.AssertNoError(t, err)
	manifest := Manifest(t, root, map[string]any{
		"sites":    []any{Site("fixture-site", cohort)},
		"captures": []any{CaptureEntry("fixture-capture", "fixture-site", id.SHA256, id.Basename)},
	})
	ledger := filepath.Join(root, "uses.db")
	d, err := db.NewDB(ledger)
	testutil.AssertNoError(t, err)
	testutil.AssertNoError(t, d.Close())
	t.Setenv(eligibilitycli.EnvManifest, manifest)
	t.Setenv(eligibilitycli.EnvAnnotationRoot, root)
	t.Setenv(eligibilitycli.EnvLedgerDB, ledger)
	return root
}
