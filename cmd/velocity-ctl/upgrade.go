package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/ctl"
)

type upgradeDotConfig struct {
	IncludePrereleases bool `json:"include_prereleases"`
}

var userHomeDir = os.UserHomeDir

func loadIncludePrereleases(configPath string) (bool, error) {
	path := configPath
	if path == "" {
		home, err := userHomeDir()
		if err != nil {
			// No home directory available (rare in service contexts):
			// treat as no config rather than failing upgrades.
			return false, nil
		}
		path = filepath.Join(home, ".velocity-ctl.json")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("reading upgrade config %s: %w", path, err)
	}

	var cfg upgradeDotConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false, fmt.Errorf("parsing upgrade config %s: %w", path, err)
	}

	return cfg.IncludePrereleases, nil
}

func runUpgrade(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	checkOnly := fs.Bool("check", false, "Check for updates without applying")
	binaryFile := fs.String("binary", "", "Apply a local binary file (offline upgrade)")
	includePrereleases := fs.Bool("include-prereleases", false, "Allow upgrade to pre-release tags")
	configPath := fs.String("config", "", "Optional path to velocity-ctl config JSON (default: ~/.velocity-ctl.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfgIncludePrereleases, err := loadIncludePrereleases(*configPath)
	if err != nil {
		return err
	}

	opts := ctl.UpgradeOptions{
		IncludePrereleases: *includePrereleases || cfgIncludePrereleases,
	}

	return ctlManager.RunUpgradeWithOptions(*checkOnly, *binaryFile, opts)
}
