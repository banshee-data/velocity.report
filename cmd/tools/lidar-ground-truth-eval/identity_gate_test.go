package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
)

// A failed acceptance run must still write its comparison and clause verdict,
// so automation cannot confuse a successful scorer invocation with acceptance.
func TestIdentityGateWritesFailureBeforeNonzeroExit(t *testing.T) {
	f := perFrameFixture(t)
	var stdout, stderr bytes.Buffer
	code := runPerFrame(perFrameArgs(f, "-identity-gate"), &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	var c perframeeval.Comparison
	if err := json.Unmarshal(stdout.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if c.IdentityAcceptance == nil || c.IdentityAcceptance.Verdict != "fail" || len(c.IdentityAcceptance.Clauses) != 5 {
		t.Fatalf("missing failure evidence: %+v", c.IdentityAcceptance)
	}
}
