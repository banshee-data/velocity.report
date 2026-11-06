package perframeeval

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval/evalfixture"
)

// A score cites the record it was read under, by path as named and content
// digest, in the reference identity, so a changed limit changes the digest;
// every reviewed following instant carries the arm's own leader choice.
func TestPhysicalScoreCitesFollowingCriteria(t *testing.T) {
	f := physFixture(t)
	cfg := physConfig(f)
	plain, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Physical.Reference.FollowingCriteria != nil || plain.Physical.A.FollowingAcceptance.Criteria != nil {
		t.Fatal("a score cites criteria it was not given")
	}

	dir := filepath.Dir(f.SplitManifestPath)
	draft := pinnedTestCriteria()
	draft.Status, draft.Decision, draft.PinnedOn = CriteriaDraft, "", ""
	writeCriteria(t, dir, "draft.json", draft)
	cfg.Physical.FollowingCriteria, cfg.Physical.FollowingCriteriaDir = "draft.json", dir
	c, err := Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cite := c.Physical.Reference.FollowingCriteria
	if cite == nil || cite.Path != "draft.json" || cite.Status != CriteriaDraft || !strings.HasPrefix(cite.Digest, "sha256:") ||
		c.Physical.A.FollowingAcceptance.Criteria.Digest != cite.Digest {
		t.Fatalf("citation %+v", cite)
	}
	if c.Physical.Reference.Digest == plain.Physical.Reference.Digest {
		t.Fatal("citing criteria left the reference identity's digest unchanged")
	}
	if !strings.Contains(strings.Join(c.Physical.A.Caveats, " "), "are a draft, not pinned") {
		t.Fatalf("caveats %v", c.Physical.A.Caveats)
	}

	// The fixture's follower is a reviewed leader record; the leader has a
	// reviewed no_leader record. Each reviewed instant carries a leader
	// choice, and the fixture's leader is free flow where it is keyed.
	var leaderChoices, rightNoLeader int
	for _, in := range c.Physical.A.Following {
		if in.LeaderChoice == nil {
			continue
		}
		leaderChoices++
		if in.Decision == annotation.FollowingNoLeader && in.LeaderChoice.Outcome == LeaderRight {
			rightNoLeader++
		}
	}
	fa := c.Physical.A.FollowingAcceptance
	if leaderChoices != len(c.Physical.A.Following) || rightNoLeader == 0 || fa.Leader.Unavailable != "" ||
		fa.Leader.ParamsHash != leaderChoiceParams().Hash() {
		t.Fatalf("leader choices %d of %d, %d right no_leader, summary %+v", leaderChoices, len(c.Physical.A.Following), rightNoLeader, fa.Leader)
	}
	// One body pair cannot meet a floor of three.
	for _, s := range fa.Strata {
		if s.Verdict != FollowingInsufficient {
			t.Fatalf("stratum %+v", s)
		}
	}
	if !strings.Contains(RenderMarkdown(*c), "### Following acceptance") {
		t.Fatal("the Markdown has no following acceptance section")
	}
}

// A held-out split is refused without a record and under a draft, and
// admitted once the cited record is pinned.
func TestHeldOutPhysicalScoringNeedsPinnedCriteria(t *testing.T) {
	f := physFixture(t)
	m := f.Manifest()
	m.Splits[0].Role = annotation.SplitRoleHeldOut
	if err := evalfixture.WriteSplitManifest(f.SplitManifestPath, m); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(f.SplitManifestPath)
	draft := pinnedTestCriteria()
	draft.Status, draft.Decision, draft.PinnedOn = CriteriaDraft, "", ""
	draftPath := writeCriteria(t, dir, "draft.json", draft)
	pinnedPath := writeCriteria(t, dir, "pinned.json", pinnedTestCriteria())

	cfg := physConfig(f)
	if _, err := Run(cfg); !errors.Is(err, ErrPhysicalHeldOut) || !strings.Contains(err.Error(), "no following-criteria record") {
		t.Fatalf("held out without a record: %v", err)
	}
	cfg.Physical.FollowingCriteria = draftPath
	if _, err := Run(cfg); !errors.Is(err, ErrPhysicalHeldOut) || !strings.Contains(err.Error(), "not pinned by a decision") {
		t.Fatalf("held out under a draft: %v", err)
	}
	cfg.Physical.FollowingCriteria = filepath.Join(dir, "absent.json")
	if _, err := Run(cfg); err == nil || errors.Is(err, ErrPhysicalHeldOut) {
		t.Fatalf("an unreadable record: %v", err)
	}
	cfg.Physical.FollowingCriteria = pinnedPath
	c, err := Run(cfg)
	if err != nil {
		t.Fatalf("held out under a pinned record: %v", err)
	}
	if c.Physical.Reference.SplitRole != annotation.SplitRoleHeldOut || !c.Physical.Reference.FollowingCriteria.Pinned() {
		t.Fatalf("reference %+v", c.Physical.Reference)
	}
	if cv := c.Physical.A.Caveats[0]; !strings.Contains(cv, "is held out, admitted under following criteria following-limits-test") ||
		!strings.Contains(cv, "D-99") {
		t.Fatalf("held-out caveat %q", cv)
	}
}
