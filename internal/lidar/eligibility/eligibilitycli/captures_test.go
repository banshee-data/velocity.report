package eligibilitycli_test

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitycli"
	fx "github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitytest"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/labelworkdb"
	"github.com/banshee-data/velocity.report/internal/version"
)

func TestAuthorizeCapturesRecordsWholeCaptureDecision(t *testing.T) {
	for _, scenario := range []string{"development", "qualification", "validation", "held-out", "unknown-second", "missing-input", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			capture, hash := fx.Capture(t, t.TempDir(), "fixture.pcap", "authored capture bytes")
			role := "development"
			if scenario == "qualification" || scenario == "validation" {
				role = scenario
			}
			root := fx.ConfigureCLI(t, capture, role)
			inputs := []string{capture}
			switch scenario {
			case "held-out":
				p := fx.Pack(t, root, fx.PackOptions{Capture: "fixture", Run: "old", PCAPBasename: filepath.Base(capture)})
				fx.Bind(t, fx.LabelSetDir(t, p), filepath.Base(capture), hash)
				fx.Freeze(t, root, "protected.json", p, annotation.SplitRoleHeldOut, nil)
			case "unknown-second":
				other, _ := fx.Capture(t, t.TempDir(), "other.pcap", "unknown bytes")
				inputs = append(inputs, other)
			case "missing-input":
				inputs = append(inputs, filepath.Join(t.TempDir(), "absent.pcap"))
			case "empty":
				inputs = nil
			}
			guard := eligibilitycli.RegisterFlags(flag.NewFlagSet("test", flag.ContinueOnError))
			err := guard.AuthorizeCaptures("fixture-evaluator", inputs)
			allowed := scenario == "development"
			if allowed && err != nil {
				t.Fatal(err)
			}
			if !allowed && !errors.Is(err, eligibility.ErrNotEligible) {
				t.Fatalf("expected eligibility refusal: %v", err)
			}
			ledger, closeLedger, err := labelworkdb.OpenUseLedger(os.Getenv(eligibilitycli.EnvLedgerDB))
			if err != nil {
				t.Fatal(err)
			}
			defer closeLedger()
			uses, _, err := ledger.List(eligibility.UseQuery{Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			if len(inputs) == 0 || scenario == "missing-input" {
				// An absent file has no source identity. The ledger rejects
				// that row atomically; it must not record the first input as
				// admitted when the overall request failed closed.
				if len(uses) != 0 {
					t.Fatalf("unexpected partial ledger: %+v", uses)
				}
				return
			}
			if len(uses) != len(inputs)*len(eligibility.Measures) {
				t.Fatalf("got %d ledger rows", len(uses))
			}
			decision := "denied"
			if allowed {
				decision = "allowed"
			}
			for _, u := range uses {
				if u.Decision != decision || u.EntryPoint != "fixture-evaluator" || u.Purpose != string(eligibility.PurposeDevelopmentEvaluation) || u.IntervalStartNs != 0 || u.IntervalEndNs != 0 || u.PolicyDigest == "" || u.BuildVersion != version.Version || u.BuildGitSHA != version.GitSHA {
					t.Fatalf("incorrect provenance: %+v", u)
				}
			}
		})
	}
}

func TestAuthorizeCapturesRequiresWorkingPolicyAndLedger(t *testing.T) {
	for _, missing := range []string{eligibilitycli.EnvManifest, eligibilitycli.EnvAnnotationRoot, eligibilitycli.EnvLedgerDB} {
		t.Run(missing, func(t *testing.T) {
			path, _ := fx.Capture(t, t.TempDir(), "fixture.pcap", "authored bytes")
			fx.ConfigureCLI(t, path, "development")
			t.Setenv(missing, "")
			g := eligibilitycli.RegisterFlags(flag.NewFlagSet("test", flag.ContinueOnError))
			if err := g.AuthorizeCaptures("fixture-evaluator", []string{path}); !errors.Is(err, eligibility.ErrNotEligible) {
				t.Fatalf("missing %s: %v", missing, err)
			}
		})
	}
}
