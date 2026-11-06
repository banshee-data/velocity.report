package perframeeval

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

// Leader choice is right, a wrong leader, a missed leader or refused, read
// against what the record says, and a wrong leader is wrong whatever gap it
// would give.
func TestJudgeLeaderChoice(t *testing.T) {
	const L, M = "obj_lead", "obj_other"
	for _, c := range []struct {
		name          string
		decision      annotation.FollowingDecision
		answer, chose string
		leaderMatched bool
		outcome       string
		reason        string
	}{
		{"the record's leader", annotation.FollowingLeader, AnswerLeader, L, true, LeaderRight, ""},
		{"another reviewed body", annotation.FollowingLeader, AnswerLeader, M, true, LeaderWrong, ReasonLeaderTrackOtherBody},
		{"an unreviewed body while the leader is matched", annotation.FollowingLeader, AnswerLeader, "", true, LeaderWrong, ReasonLeaderTrackNotTheLeaders},
		{"an unidentified body and an unmatched leader", annotation.FollowingLeader, AnswerLeader, "", false, LeaderUnscored, ReasonLeaderUnidentifiable},
		{"free flow behind a leader", annotation.FollowingLeader, AnswerFreeFlow, "", true, LeaderMissed, ""},
		{"ambiguity behind a leader", annotation.FollowingLeader, AnswerAmbiguousLeader, "", true, LeaderRefused, AnswerAmbiguousLeader},
		{"no common path behind a leader", annotation.FollowingLeader, AnswerNoCommonPath, "", true, LeaderRefused, AnswerNoCommonPath},
		{"free flow with no leader", annotation.FollowingNoLeader, AnswerFreeFlow, "", false, LeaderRight, ""},
		{"any leader with no leader", annotation.FollowingNoLeader, AnswerLeader, M, false, LeaderWrong, ReasonRecordNamesNoLeader},
		{"an unidentified leader with no leader", annotation.FollowingNoLeader, AnswerLeader, "", false, LeaderWrong, ReasonRecordNamesNoLeader},
		{"a suppression with no leader", annotation.FollowingNoLeader, AnswerAmbiguousLeader, "", false, LeaderRefused, AnswerAmbiguousLeader},
		{"a suppression when ambiguous", annotation.FollowingAmbiguous, AnswerAmbiguousLeader, "", false, LeaderRight, ""},
		{"no common path when ambiguous", annotation.FollowingAmbiguous, AnswerNoCommonPath, "", false, LeaderRight, ""},
		{"a single leader when ambiguous", annotation.FollowingAmbiguous, AnswerLeader, L, false, LeaderWrong, ReasonRecordAmbiguous},
		{"free flow when ambiguous", annotation.FollowingAmbiguous, AnswerFreeFlow, "", false, LeaderMissed, ""},
	} {
		recordLeader := ""
		if c.decision == annotation.FollowingLeader {
			recordLeader = L
		}
		outcome, reason := judgeLeaderChoice(c.decision, recordLeader, c.answer, c.chose, c.leaderMatched)
		if outcome != c.outcome || reason != c.reason {
			t.Errorf("%s: %s (%s), want %s (%s)", c.name, outcome, reason, c.outcome, c.reason)
		}
	}
}

func TestAnswerOf(t *testing.T) {
	for _, c := range []struct {
		d    l8behaviour.PairingDecision
		want string
	}{
		{l8behaviour.PairingDecision{LeaderTrackID: "seq-000002"}, AnswerLeader},
		{l8behaviour.PairingDecision{Reason: l8behaviour.ReasonAmbiguousLeader}, AnswerAmbiguousLeader},
		{l8behaviour.PairingDecision{Reason: l8behaviour.ReasonNoCommonPath}, AnswerNoCommonPath},
		{l8behaviour.PairingDecision{}, AnswerFreeFlow},
	} {
		if got := answerOf(c.d); got != c.want {
			t.Errorf("%+v: %s, want %s", c.d, got, c.want)
		}
	}
}

// An arm with no trajectories cannot be analysed, and says so; it is not an
// error, since the arm's geometry is still scored.
func TestAnalyseLeadersRecordsWhatItCannotRead(t *testing.T) {
	got := analyseLeaders(PhysicalArm{})
	if got.summary.Unavailable == "" || got.summary.Method != l8behaviour.FollowingEncounterMethodID || got.summary.ParamsHash == "" {
		t.Fatalf("summary %+v", got.summary)
	}
}
