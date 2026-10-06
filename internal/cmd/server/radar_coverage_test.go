package server

import (
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/tailscale"
)

func TestTailscaleServeTarget(t *testing.T) {
	tests := []struct {
		name   string
		listen string
		want   string
	}{
		{"dev port", "127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"image port", "0.0.0.0:80", "http://127.0.0.1:80"},
		{"all interfaces", "[::]:8080", "http://127.0.0.1:8080"},
		{"host omitted", ":9090", "http://127.0.0.1:9090"},
		// Anything without a parseable port falls back to the package
		// default rather than publishing a broken target.
		{"no port", "127.0.0.1", tailscale.LocalServeHTTPTarget},
		{"empty", "", tailscale.LocalServeHTTPTarget},
		{"trailing colon only", "127.0.0.1:", tailscale.LocalServeHTTPTarget},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tailscaleServeTarget(tc.listen); got != tc.want {
				t.Errorf("tailscaleServeTarget(%q) = %q, want %q", tc.listen, got, tc.want)
			}
		})
	}
}

func TestParseMigrateCommandArgsSeparatorAndHelp(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantPositionals []string
		wantDBPath      string
		wantExplicit    bool
	}{
		{
			name:            "bare double dash passes the rest through",
			args:            []string{"--db-path", "/tmp/a.db", "--", "up", "-not-a-flag"},
			wantPositionals: []string{"up", "-not-a-flag"},
			wantDBPath:      "/tmp/a.db",
			wantExplicit:    true,
		},
		{
			name:            "double dash with nothing after it",
			args:            []string{"--"},
			wantPositionals: []string{},
			wantDBPath:      "default.db",
		},
		{
			name:            "--help short-circuits to help",
			args:            []string{"--help"},
			wantPositionals: []string{"help"},
			wantDBPath:      "default.db",
		},
		{
			name:            "-h short-circuits to help",
			args:            []string{"-h"},
			wantPositionals: []string{"help"},
			wantDBPath:      "default.db",
		},
		{
			name:            "db path is read before help",
			args:            []string{"--db-path=/tmp/b.db", "-h"},
			wantPositionals: []string{"help"},
			wantDBPath:      "/tmp/b.db",
			wantExplicit:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			positionals, dbPath, explicit, err := parseMigrateCommandArgs(tc.args, "default.db")
			if err != nil {
				t.Fatalf("parseMigrateCommandArgs: %v", err)
			}
			if strings.Join(positionals, ",") != strings.Join(tc.wantPositionals, ",") {
				t.Errorf("positionals = %v, want %v", positionals, tc.wantPositionals)
			}
			if dbPath != tc.wantDBPath {
				t.Errorf("dbPath = %q, want %q", dbPath, tc.wantDBPath)
			}
			if explicit != tc.wantExplicit {
				t.Errorf("explicitDBPath = %v, want %v", explicit, tc.wantExplicit)
			}
		})
	}
}

func TestParseMigrateCommandArgsRejectsBadFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "--db-path with no value",
			args:    []string{"--db-path"},
			wantErr: "flag needs an argument",
		},
		{
			name:    "--db-path= with an empty value",
			args:    []string{"--db-path="},
			wantErr: "flag needs an argument",
		},
		{
			name:    "unknown flag",
			args:    []string{"--nope"},
			wantErr: "unknown migrate flag",
		},
		{
			name:    "unknown short flag",
			args:    []string{"-x"},
			wantErr: "unknown migrate flag",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := parseMigrateCommandArgs(tc.args, "default.db")
			if err == nil {
				t.Fatalf("parseMigrateCommandArgs(%v) succeeded, want an error", tc.args)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestRuntimeSerialFactory(t *testing.T) {
	t.Run("disabled reload yields no factory", func(t *testing.T) {
		// A nil factory is what tells the serial manager that hot reload is
		// off, so this must stay nil rather than becoming a no-op function.
		if got := runtimeSerialFactory(false); got != nil {
			t.Error("runtimeSerialFactory(false) returned a factory, want nil")
		}
	})

	t.Run("enabled reload builds a real mux", func(t *testing.T) {
		factory := runtimeSerialFactory(true)
		if factory == nil {
			t.Fatal("runtimeSerialFactory(true) = nil, want a factory")
		}
		// Opening a device that does not exist exercises the factory body and
		// surfaces the failure rather than panicking.
		_, err := factory("/dev/ttyVELOCITYDOESNOTEXIST", defaultRuntimeSerialOptions())
		if err == nil {
			t.Error("factory opened a nonexistent device, want an error")
		}
	})
}

func TestRuntimeSerialSnapshotRequiresDatabaseWhenReloadEnabled(t *testing.T) {
	// Reload reads the enabled configuration from the database, so being
	// asked to do that without a handle is a programming error, not a
	// fallback to the CLI port.
	_, err := runtimeSerialSnapshot(nil, "/dev/ttyUSB0", true, true)
	if err == nil {
		t.Fatal("runtimeSerialSnapshot with a nil database succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "without a database handle") {
		t.Errorf("error = %v, want it to name the missing database handle", err)
	}
}

func TestRuntimeSerialSnapshotEmptyPortYieldsEmptySnapshot(t *testing.T) {
	// With no database lookup and a blank --port, there is nothing to
	// describe; the caller turns this into the "serial port is required"
	// startup error.
	snap, err := runtimeSerialSnapshot(nil, "   ", true, false)
	if err != nil {
		t.Fatalf("runtimeSerialSnapshot: %v", err)
	}
	if snap.PortPath != "" {
		t.Errorf("PortPath = %q, want empty", snap.PortPath)
	}
	if snap.Source != "" {
		t.Errorf("Source = %q, want empty", snap.Source)
	}
}

func TestRuntimeSerialSnapshotTrimsCLIPortPath(t *testing.T) {
	snap, err := runtimeSerialSnapshot(nil, "  /dev/ttyUSB0\n", true, false)
	if err != nil {
		t.Fatalf("runtimeSerialSnapshot: %v", err)
	}
	if snap.PortPath != "/dev/ttyUSB0" {
		t.Errorf("PortPath = %q, want the trimmed path", snap.PortPath)
	}
	if snap.Source != "cli" {
		t.Errorf("Source = %q, want %q", snap.Source, "cli")
	}
	if snap.Options != defaultRuntimeSerialOptions() {
		t.Errorf("Options = %+v, want the runtime defaults", snap.Options)
	}
}

func TestDefaultRuntimeSerialOptionsMatchTheOPS243(t *testing.T) {
	opts := defaultRuntimeSerialOptions()
	if opts.BaudRate != 19200 || opts.DataBits != 8 || opts.StopBits != 1 || opts.Parity != "N" {
		t.Errorf("defaultRuntimeSerialOptions() = %+v, want 19200 8N1", opts)
	}
}

func TestResolveDataCommandDBPathWith(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		installed  bool
		want       string
	}{
		{
			name:       "default path on an installed appliance moves to /var/lib",
			configured: defaultRuntimeDBPath,
			installed:  true,
			want:       deployedRuntimeDBPath,
		},
		{
			name:       "default path off-appliance stays relative",
			configured: defaultRuntimeDBPath,
			installed:  false,
			want:       defaultRuntimeDBPath,
		},
		{
			// An explicit path is the operator's choice and is never
			// rewritten, appliance or not.
			name:       "explicit path is preserved on an appliance",
			configured: "/tmp/custom.db",
			installed:  true,
			want:       "/tmp/custom.db",
		},
		{
			name:       "surrounding whitespace is trimmed",
			configured: "  /tmp/custom.db  ",
			installed:  true,
			want:       "/tmp/custom.db",
		},
		{
			name:       "empty stays empty",
			configured: "   ",
			installed:  true,
			want:       "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveDataCommandDBPathWith(tc.configured, tc.installed); got != tc.want {
				t.Errorf("resolveDataCommandDBPathWith(%q, %v) = %q, want %q",
					tc.configured, tc.installed, got, tc.want)
			}
		})
	}
}

func TestInstalledApplianceLayoutPresentForTestBinary(t *testing.T) {
	// The test binary lives in the Go build cache, not under
	// /opt/velocity-report/versions/, so this must report false. It exercises
	// the os.Executable + EvalSymlinks path without needing an install.
	if installedApplianceLayoutPresent() {
		t.Error("installedApplianceLayoutPresent() = true for the test binary, want false")
	}
}

func TestVisitedFlagsEmptyBeforeParse(t *testing.T) {
	// serveFlags is package state shared with Main; without an explicit parse
	// in this test binary nothing has been visited.
	if got := visitedFlags(); len(got) != 0 {
		t.Errorf("visitedFlags() = %v, want empty before any flag is set", got)
	}
}
