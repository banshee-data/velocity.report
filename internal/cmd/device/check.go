package device

import (
	"flag"

	"github.com/banshee-data/velocity.report/internal/ctl"
)

// runCheck implements `velocity device check`: a read-only report of whether a
// newer release is available, with no download or install. It is the dedicated
// verb that replaces the overloaded `upgrade --check` in the public surface
// (the flag form is retained for one release of compatibility).
func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	prerelease := fs.Bool("prerelease", false, "Include pre-release tags when checking")
	configPath := fs.String("config", "", "Optional path to velocity config JSON (default: ~/.velocity-ctl.json)")
	handled, err := parseCommandFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	cfgIncludePrereleases, err := loadIncludePrereleases(*configPath)
	if err != nil {
		return err
	}

	opts := ctl.UpgradeOptions{
		IncludePrereleases: *prerelease || cfgIncludePrereleases,
	}

	return ctlManager.RunUpgradeWithOptions(true, "", opts)
}
