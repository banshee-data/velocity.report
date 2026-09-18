package main

import "testing"

func TestResolveDBPath(t *testing.T) {
	if got := resolveDBPath("specific.db", "fallback.db"); got != "specific.db" {
		t.Errorf("resolveDBPath with specific set = %q, want %q", got, "specific.db")
	}
	if got := resolveDBPath("", "fallback.db"); got != "fallback.db" {
		t.Errorf("resolveDBPath with specific empty = %q, want %q", got, "fallback.db")
	}
}

func TestValidateArgs(t *testing.T) {
	cases := []struct {
		name                    string
		reference, candidate    string
		referenceDB, candidate2 string
		wantErr                 bool
	}{
		{"both set, distinct IDs, same db", "ref-1", "cand-1", "a.db", "a.db", false},
		{"missing reference", "", "cand-1", "a.db", "a.db", true},
		{"missing candidate", "ref-1", "", "a.db", "a.db", true},
		{"both missing", "", "", "a.db", "a.db", true},
		{"same run, same db", "run-1", "run-1", "a.db", "a.db", true},
		{"same run id, different db is fine", "run-1", "run-1", "a.db", "b.db", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateArgs(tc.reference, tc.candidate, tc.referenceDB, tc.candidate2)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateArgs(%q, %q, %q, %q) error = %v, wantErr %v", tc.reference, tc.candidate, tc.referenceDB, tc.candidate2, err, tc.wantErr)
			}
		})
	}
}

func TestRunRequiresBothIDs(t *testing.T) {
	if code := run([]string{}); code != 2 {
		t.Errorf("run with no args = %d, want 2 (usage error)", code)
	}
	if code := run([]string{"-reference-run-id", "a"}); code != 2 {
		t.Errorf("run with only reference-run-id = %d, want 2", code)
	}
	if code := run([]string{"-reference-run-id", "a", "-candidate-run-id", "a"}); code != 2 {
		t.Errorf("run with identical IDs against the same default db = %d, want 2", code)
	}
}

func TestRunUnknownDatabase(t *testing.T) {
	code := run([]string{
		"-db", "/nonexistent/path/does-not-exist.db",
		"-reference-run-id", "ref",
		"-candidate-run-id", "cand",
	})
	if code != 1 {
		t.Errorf("run against a missing database = %d, want 1", code)
	}
}

func TestRunSeparateDatabasesAllowsSameRunID(t *testing.T) {
	// Different dbs never collide on identical run IDs at the flag-validation
	// stage; it should fail later, on actually opening the (nonexistent)
	// database files, not on the identical-ID check.
	code := run([]string{
		"-reference-db", "/nonexistent/reference.db",
		"-candidate-db", "/nonexistent/candidate.db",
		"-reference-run-id", "same-id",
		"-candidate-run-id", "same-id",
	})
	if code != 1 {
		t.Errorf("run with same ID across two distinct (missing) databases = %d, want 1 (open/query failure, not a usage error)", code)
	}
}
