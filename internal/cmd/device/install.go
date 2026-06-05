package device

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Deploy files embedded into the binary so the image carries no separate
// copies. `velocity device install <component>` writes them to their canonical
// system paths; the image stage scripts invoke that instead of shipping files.

//go:embed files/lidar-network.conf
var lidarNetworkConf []byte

//go:embed files/99-velocity-report.rules
var udevRadarRules []byte

//go:embed files/wpa_supplicant.conf
var wpaSupplicantConf []byte

// installComponent is an embedded deploy file and where it lands on disk.
type installComponent struct {
	data []byte
	dest string
	mode os.FileMode
}

var installComponents = map[string]installComponent{
	"network": {lidarNetworkConf, "/etc/network/interfaces.d/lidar", 0o644},
	"udev":    {udevRadarRules, "/etc/udev/rules.d/99-velocity-report.rules", 0o644},
	"wifi":    {wpaSupplicantConf, "/etc/wpa_supplicant/wpa_supplicant.conf", 0o600},
}

func installUsage() string {
	names := make([]string, 0, len(installComponents))
	for n := range installComponents {
		names = append(names, n)
	}
	sort.Strings(names)
	return "usage: velocity device install <" + strings.Join(names, "|") + ">"
}

// runInstall writes an embedded deploy component to its canonical path. It is
// idempotent: a second call rewrites the file and re-applies the mode.
func runInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	handled, err := parseCommandFlags(fs, args)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}

	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("%s", installUsage())
	}
	name := rest[0]
	c, ok := installComponents[name]
	if !ok {
		return fmt.Errorf("unknown component %q: %s", name, installUsage())
	}

	if err := writeComponent(c, ""); err != nil {
		return err
	}
	fmt.Printf("installed %s -> %s\n", name, c.dest)
	return nil
}

// writeComponent writes c under root (root="" targets the real filesystem; a
// non-empty root is used by tests). Parent directories are created and the file
// mode is enforced even when the file already exists.
func writeComponent(c installComponent, root string) error {
	dest := filepath.Join(root, c.dest)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(dest), err)
	}
	if err := os.WriteFile(dest, c.data, c.mode); err != nil {
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	if err := os.Chmod(dest, c.mode); err != nil {
		return fmt.Errorf("chmod %s: %w", dest, err)
	}
	return nil
}
