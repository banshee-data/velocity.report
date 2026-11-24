package eligibilitytest

import (
	"flag"
	"os"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitycli"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/labelworkdb"
)

func TestConfigureCLIProvisionsOnlyNamedFixture(t *testing.T) {
	capture, hash := Capture(t, t.TempDir(), "fixture.pcap", "synthetic bytes")
	root := ConfigureCLI(t, capture, "development")
	if os.Getenv(eligibilitycli.EnvAnnotationRoot) != root {
		t.Fatal("annotation root not installed")
	}
	manifest, err := eligibility.LoadManifest(os.Getenv(eligibilitycli.EnvManifest))
	if err != nil || manifest == nil {
		t.Fatalf("manifest: %v", err)
	}
	g := eligibilitycli.RegisterFlags(flag.NewFlagSet("test", flag.ContinueOnError))
	if err := g.AuthorizeCaptures("fixture-provision-test", []string{capture}); err != nil {
		t.Fatal(err)
	}
	other, _ := Capture(t, t.TempDir(), "unknown.pcap", "other bytes")
	if err := g.AuthorizeCaptures("fixture-provision-test", []string{other}); err == nil {
		t.Fatal("fixture policy admitted unrelated bytes")
	}
	ledger, closeLedger, err := labelworkdb.OpenUseLedger(os.Getenv(eligibilitycli.EnvLedgerDB))
	if err != nil {
		t.Fatal(err)
	}
	defer closeLedger()
	uses, _, err := ledger.List(eligibility.UseQuery{CaptureSHA256: hash, Limit: 100})
	if err != nil || len(uses) != len(eligibility.Measures) {
		t.Fatalf("ledger: %d rows, %v", len(uses), err)
	}
	for _, u := range uses {
		if u.Decision != "allowed" || u.CaptureSHA256 != hash {
			t.Fatalf("unexpected use: %+v", u)
		}
	}
}
