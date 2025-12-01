package main

import (
	"testing"
)

func TestRollback_Structure(t *testing.T) {
	r := &Rollback{
		Target:  "localhost",
		SSHUser: "testuser",
		SSHKey:  "/test/key",
		DryRun:  true,
	}

	if r.Target != "localhost" {
		t.Errorf("Target = %s, want localhost", r.Target)
	}
	if r.SSHUser != "testuser" {
		t.Errorf("SSHUser = %s, want testuser", r.SSHUser)
	}
	if r.SSHKey != "/test/key" {
		t.Errorf("SSHKey = %s, want /test/key", r.SSHKey)
	}
	if !r.DryRun {
		t.Error("Expected DryRun to be true")
	}
}

func TestRollback_Execute_NoBackup(t *testing.T) {
	t.Skip("Skipping test that requires sudo and service installation")

	r := &Rollback{
		Target:  "localhost",
		SSHUser: "",
		SSHKey:  "",
		DryRun:  true,
	}

	// This will fail because there's no backup to rollback to
	err := r.Execute()
	if err == nil {
		t.Log("Note: Rollback succeeded (unexpected in test environment)")
	} else {
		t.Logf("Rollback failed as expected: %v", err)
	}
}

func TestRollback_DryRunMode(t *testing.T) {
	t.Skip("Skipping test that requires sudo and service installation")

	r := &Rollback{
		Target: "localhost",
		DryRun: true,
	}

	if !r.DryRun {
		t.Error("Expected DryRun to be true")
	}

	// Dry run should not actually perform rollback
	err := r.Execute()
	if err == nil {
		t.Log("Dry run completed without error")
	} else {
		// Error is expected since there's no backup
		t.Logf("Dry run error (expected): %v", err)
	}
}

func TestRollback_RemoteTarget(t *testing.T) {
	r := &Rollback{
		Target:  "pi@192.168.1.100",
		SSHUser: "pi",
		SSHKey:  "/home/user/.ssh/id_rsa",
		DryRun:  false,
	}

	if r.Target != "pi@192.168.1.100" {
		t.Errorf("Target = %s, want pi@192.168.1.100", r.Target)
	}
	if r.SSHUser != "pi" {
		t.Errorf("SSHUser = %s, want pi", r.SSHUser)
	}
	if r.DryRun {
		t.Error("Expected DryRun to be false")
	}
}

func TestRollback_EmptyFields(t *testing.T) {
	r := &Rollback{
		Target:  "",
		SSHUser: "",
		SSHKey:  "",
		DryRun:  false,
	}

	// Empty target should be allowed (defaults to localhost)
	if r.Target != "" {
		t.Errorf("Empty Target should remain empty, got %s", r.Target)
	}
	if r.DryRun {
		t.Error("Expected DryRun to be false")
	}
}

func TestRollback_FieldTypes(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		dryRun  bool
		wantErr bool
	}{
		{"localhost dry run", "localhost", true, false},
		{"localhost actual", "localhost", false, true}, // Would need user confirmation
		{"remote dry run", "192.168.1.100", true, false},
		{"remote actual", "192.168.1.100", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Rollback{
				Target: tt.target,
				DryRun: tt.dryRun,
			}

			if r.Target != tt.target {
				t.Errorf("Target = %s, want %s", r.Target, tt.target)
			}
			if r.DryRun != tt.dryRun {
				t.Errorf("DryRun = %v, want %v", r.DryRun, tt.dryRun)
			}
		})
	}
}
