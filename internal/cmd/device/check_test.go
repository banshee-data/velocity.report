package device

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/ctl"
)

func TestRunCheckHelpReturnsNil(t *testing.T) {
	if err := runCheck([]string{"--help"}); err != nil {
		t.Fatalf("runCheck --help returned error: %v", err)
	}
}

func TestRunCheckReportsLatestStable(t *testing.T) {
	tmp := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stable":{"linux_arm64":{"version":"0.5.2","url":"https://example.com/bin","sha256":""}}}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	old := ctlManager
	ctlManager = ctl.NewManager(ctl.Config{
		ReleaseMetaURL:  server.URL,
		InstallRoot:     filepath.Join(tmp, "opt"),
		BackupDir:       filepath.Join(tmp, "backups"),
		DBPath:          filepath.Join(tmp, "sensor_data.db"),
		CurrentVersion:  "0.5.1",
		GOOS:            "linux",
		GOARCH:          "arm64",
		RequestTimeout:  time.Second,
		DownloadTimeout: time.Second,
	}, nil, cmdFakeRunner{}, &out, &out)
	defer func() { ctlManager = old }()

	if err := runCheck(nil); err != nil {
		t.Fatalf("runCheck failed: %v", err)
	}
	if !strings.Contains(out.String(), "Latest:  v0.5.2") {
		t.Fatalf("expected latest version in output, got %q", out.String())
	}
}

func TestRunCheckAllowsPrereleaseFromFlagOrConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"flag", []string{"--prerelease"}},
		{"config", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			configPath := filepath.Join(tmp, "velocity-ctl.json")
			args := tc.args
			if tc.name == "config" {
				if err := os.WriteFile(configPath, []byte(`{"include_prereleases": true}`), 0o644); err != nil {
					t.Fatal(err)
				}
				args = []string{"--config", configPath}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"stable":{"linux_arm64":{"version":"0.5.2","url":"https://example.com/stable","sha256":""}},"prerelease":{"linux_arm64":{"version":"0.6.0-rc1","url":"https://example.com/rc","sha256":""}}}`))
			}))
			defer server.Close()

			var out bytes.Buffer
			old := ctlManager
			ctlManager = ctl.NewManager(ctl.Config{
				ReleaseMetaURL:  server.URL,
				InstallRoot:     filepath.Join(tmp, "opt"),
				BackupDir:       filepath.Join(tmp, "backups"),
				DBPath:          filepath.Join(tmp, "sensor_data.db"),
				CurrentVersion:  "0.5.1",
				GOOS:            "linux",
				GOARCH:          "arm64",
				RequestTimeout:  time.Second,
				DownloadTimeout: time.Second,
			}, nil, cmdFakeRunner{}, &out, &out)
			defer func() { ctlManager = old }()

			if err := runCheck(args); err != nil {
				t.Fatalf("runCheck failed: %v", err)
			}
			if !strings.Contains(out.String(), "Latest:  v0.6.0-rc1") {
				t.Fatalf("expected prerelease in output, got %q", out.String())
			}
		})
	}
}

func TestRunCheckReturnsConfigParseError(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "velocity-ctl.json")
	if err := os.WriteFile(configPath, []byte(`{"include_prereleases":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runCheck([]string{"--config", configPath}); err == nil {
		t.Fatal("expected config parse error")
	}
}
