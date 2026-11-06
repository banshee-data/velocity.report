package perframeeval

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

// Leader choice against the reviewed following records (review R6 of
// docs/lidar/operations/0.5.2-sprint-review.md).
//
// The gap a following record carries is scored between the predictions
// matched to the record's two bodies, whoever the estimator thought the
// follower was following. That says nothing about whether it chose that
// leader. Here the estimator's own pairing is read: the arm's trajectories
// are analysed by l8behaviour.AnalyseFollowing, and the pairing decision for
// the follower's matched track at the instant is set against the record.
//
//   - A record naming a leader is answered rightly by a leader whose track is
//     the one matched to that body at the sample. A leader matched to another
//     reviewed body is a wrong leader; so is a leader matched to none while
//     the record's leader is matched to a track, since the estimator then
//     passed over it. Free flow is a missed leader, and a suppressed choice
//     (ambiguous_leader, no_common_path) is refused.
//   - A no_leader record is answered rightly by free flow. Any leader is a
//     wrong leader, and a suppression is refused.
//   - An ambiguous record is answered rightly by a suppression. Any single
//     leader is a wrong leader, and free flow a missed leader.
//
// An instant the record cannot speak for (unreviewed, tracker-assisted), or
// where the bodies cannot be identified with tracks (no truth keyframe, no
// match), or the estimator never evaluated the follower there, is unscored
// with its reason. Bodies are reviewed object ids throughout: a track is only
// the means of finding which body the estimator chose.

// Leader-choice answers: what the estimator's pairing said.
const (
	AnswerLeader            = "leader"
	AnswerFreeFlow          = "free_flow"
	AnswerAmbiguousLeader   = "ambiguous_leader"
	AnswerNoCommonPath      = "no_common_path"
	AnswerNotEvaluated      = "not_evaluated"
	AnswerFollowerUnmatched = "follower_unmatched"
	AnswerUnavailable       = "unavailable"
)

// Leader-choice outcomes.
const (
	LeaderRight    = "right"
	LeaderWrong    = "wrong_leader"
	LeaderMissed   = "missed_leader"
	LeaderRefused  = "refused"
	LeaderUnscored = "unscored"
)

// LeaderChoiceOutcomes lists the outcomes in report order.
func LeaderChoiceOutcomes() []string {
	return []string{LeaderRight, LeaderWrong, LeaderMissed, LeaderRefused, LeaderUnscored}
}

// Reasons a leader choice is unscored, or a wrong leader is wrong.
const (
	ReasonNoFollowerKeyframe       = "no_follower_keyframe"
	ReasonLeaderChoiceUnavailable  = "leader_choice_unavailable"
	ReasonFollowerNotEvaluated     = "follower_not_evaluated"
	ReasonLeaderTrackNotTheLeaders = "leader_track_not_the_records_leader"
	ReasonLeaderTrackOtherBody     = "leader_track_is_another_reviewed_body"
	ReasonRecordNamesNoLeader      = "record_names_no_leader"
	ReasonRecordAmbiguous          = "record_ambiguous"
	ReasonLeaderUnidentifiable     = "leader_unidentifiable"
)

// LeaderChoice is the estimator's leader choice at one following instant,
// against the record.
type LeaderChoice struct {
	// FollowerTrack is the track matched to the record's follower.
	FollowerTrack string `json:"follower_track,omitempty"`
	Answer        string `json:"answer"`
	LeaderTrack   string `json:"leader_track,omitempty"`
	// LeaderObjectID is the reviewed body the chosen leader track was
	// matched to at the sample; empty when it was matched to none.
	LeaderObjectID string `json:"leader_object_id,omitempty"`
	Outcome        string `json:"outcome"`
	Reason         string `json:"reason,omitempty"`
}

// LeaderChoiceSummary is how the arm's leader choices were made and where
// they went.
type LeaderChoiceSummary struct {
	// Method and ParamsHash identify the analysis the pairing came from.
	Method     string `json:"method"`
	ParamsHash string `json:"params_hash,omitempty"`
	// Unavailable says why no leader choice could be read, when none could.
	Unavailable string `json:"unavailable,omitempty"`
	// Counts is the record's decision, then the outcome, then the count.
	Counts map[annotation.FollowingDecision]map[string]int `json:"counts"`
}

// leaderDecisions is an arm's pairing decisions, by follower track and
// capture time.
type leaderDecisions struct {
	summary   LeaderChoiceSummary
	decisions map[string]map[int64]l8behaviour.PairingDecision
}

// leaderChoiceParams are the bounds the arm's leader choice is analysed
// under: the field run's (internal/report/headway/fieldrun), which are the
// analytic scenarios' uncalibrated values. Their hash is reported, and a
// change to them is a change to every leader-choice score.
func leaderChoiceParams() l8behaviour.FollowingAnalysisParams {
	return l8behaviour.EncounterScenarioParams()
}

// analyseLeaders runs the following analysis over an arm's trajectories. A
// failure is recorded, not returned: the arm's geometry is still scored, and
// every leader choice is unscored with the reason.
func analyseLeaders(arm PhysicalArm) leaderDecisions {
	params := leaderChoiceParams()
	out := leaderDecisions{summary: LeaderChoiceSummary{Method: l8behaviour.FollowingEncounterMethodID, ParamsHash: params.Hash()}}
	if len(arm.trajectories) == 0 {
		out.summary.Unavailable = "the arm carries no trajectories to analyse"
		return out
	}
	a, err := l8behaviour.AnalyseFollowing(arm.trajectories, params)
	if err != nil {
		out.summary.Unavailable = fmt.Sprintf("following analysis failed: %v", err)
		return out
	}
	out.decisions = map[string]map[int64]l8behaviour.PairingDecision{}
	for _, tl := range a.Timelines {
		byTime := map[int64]l8behaviour.PairingDecision{}
		for _, d := range tl.Decisions {
			byTime[d.CaptureUnixNanos] = d
		}
		out.decisions[tl.FollowerTrackID] = byTime
	}
	return out
}

// answerOf reads a pairing decision as an answer.
func answerOf(d l8behaviour.PairingDecision) string {
	switch {
	case d.LeaderTrackID != "":
		return AnswerLeader
	case d.Reason == l8behaviour.ReasonAmbiguousLeader:
		return AnswerAmbiguousLeader
	case d.Reason == l8behaviour.ReasonNoCommonPath:
		return AnswerNoCommonPath
	case d.Reason != l8behaviour.ReasonUnspecified:
		return d.Reason.String()
	}
	return AnswerFreeFlow
}

// judgeLeaderChoice sets an answer against a record's decision. leaderObject
// is the reviewed body the chosen leader track was matched to (empty for
// none), and recordLeaderMatched says the record's leader was matched to a
// track at the sample.
func judgeLeaderChoice(decision annotation.FollowingDecision, recordLeader, answer, leaderObject string, recordLeaderMatched bool) (string, string) {
	suppressed := answer != AnswerLeader && answer != AnswerFreeFlow
	switch decision {
	case annotation.FollowingLeader:
		switch {
		case answer == AnswerFreeFlow:
			return LeaderMissed, ""
		case suppressed:
			return LeaderRefused, answer
		case leaderObject == recordLeader:
			return LeaderRight, ""
		case leaderObject != "":
			return LeaderWrong, ReasonLeaderTrackOtherBody
		case recordLeaderMatched:
			return LeaderWrong, ReasonLeaderTrackNotTheLeaders
		}
		// Neither the chosen track nor the record's leader is identified
		// with a body: the choice may be right or wrong.
		return LeaderUnscored, ReasonLeaderUnidentifiable
	case annotation.FollowingNoLeader:
		switch {
		case answer == AnswerFreeFlow:
			return LeaderRight, ""
		case suppressed:
			return LeaderRefused, answer
		}
		return LeaderWrong, ReasonRecordNamesNoLeader
	default: // ambiguous
		switch {
		case suppressed:
			return LeaderRight, ""
		case answer == AnswerFreeFlow:
			return LeaderMissed, ""
		}
		return LeaderWrong, ReasonRecordAmbiguous
	}
}

// leaderChoice reads the estimator's leader choice for one following
// instant. It returns nil for a record the scorer does not take as truth,
// whose instant already says why.
func (s *physicalScorer) leaderChoice(e expectedFollowing, leaders leaderDecisions) *LeaderChoice {
	f := e.ref
	if f.Review.Origin != annotation.OriginIndependent || f.Review.Status != annotation.StatusReviewed {
		return nil
	}
	lc := &LeaderChoice{}
	unscored := func(answer, reason string) *LeaderChoice {
		lc.Answer, lc.Outcome, lc.Reason = answer, LeaderUnscored, reason
		return lc
	}
	if f.Decision == annotation.FollowingLeader && !e.leaderInEpisode {
		return unscored("", ReasonLeaderOutsideEpisode)
	}
	if g, ok := s.pr.geometry[f.FollowerObjectID][e.sample]; !ok || !g.Truth {
		return unscored("", ReasonNoFollowerKeyframe)
	}
	m := s.match(e.sample)
	fi, ok := m.matched[f.FollowerObjectID]
	if !ok {
		return unscored(AnswerFollowerUnmatched, ReasonFollowerUnmatched)
	}
	follower := m.predictions[fi].body
	lc.FollowerTrack = follower.TrackKey
	if leaders.summary.Unavailable != "" {
		return unscored(AnswerUnavailable, ReasonLeaderChoiceUnavailable)
	}
	d, ok := leaders.decisions[follower.TrackKey][follower.TimestampNs]
	if !ok {
		return unscored(AnswerNotEvaluated, ReasonFollowerNotEvaluated)
	}
	lc.Answer = answerOf(d)
	if lc.Answer == AnswerLeader {
		lc.LeaderTrack = d.LeaderTrackID
		for obj, i := range m.matched {
			if m.predictions[i].body.TrackKey == d.LeaderTrackID {
				lc.LeaderObjectID = obj
			}
		}
	}
	_, recordLeaderMatched := m.matched[f.LeaderObjectID]
	lc.Outcome, lc.Reason = judgeLeaderChoice(f.Decision, f.LeaderObjectID, lc.Answer, lc.LeaderObjectID, recordLeaderMatched && f.LeaderObjectID != "")
	return lc
}
