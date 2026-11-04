package lanes

import (
	"testing"
)

// Synthetic streets only: drawn lanes and proposals made up for the rule.

func scoreLane(id, layerTag string, x0, y0, x1, y1 float64, tags map[string]string) Feature {
	t := map[string]string{"street": "KIRKHAM ST"}
	for k, v := range tags {
		t[k] = v
	}
	return Feature{FeatureID: id, RowID: "r", Kind: KindTravelLane, XYZ: []float64{x0, y0, 0, x1, y1, 0}, Tags: t}
}

func TestScoreProposalsAgainstDrawnLanes(t *testing.T) {
	ref := map[string]string{"reference": "lane0002"}
	drawn := ScoreFeatures([]Feature{
		scoreLane("d-e1", "", 0, -1.8, 60, -1.8, ref),
		scoreLane("d-e2", "", 0, -5.1, 60, -5.1, ref),
		scoreLane("d-w1", "", 60, 1.8, 0, 1.8, ref),
		scoreLane("not-a-reference", "", 0, 9, 60, 9, nil),
	}, "reference", "lane0002", false)
	if len(drawn) != 3 {
		t.Fatalf("drawn %d", len(drawn))
	}
	high := map[string]string{"confidence": "high"}
	medium := map[string]string{"confidence": "medium"}
	proposals := ScoreFeatures([]Feature{
		scoreLane("p-e1", "", 2, -1.6, 58, -1.6, high), // 0.2 m off: found
		scoreLane("p-e2", "", 5, -5.6, 30, -5.6, high), // 0.5 m off and short: paired, not found
		scoreLane("p-w1", "", 58, 2.0, 2, 2.0, medium), // medium: counted, not judged
		scoreLane("p-x", "", 0, 30, 60, 30, high),      // on no drawn lane: false
		scoreLane("ungraded", "", 0, 0, 1, 0, nil),
	}, "", "", true)
	if len(proposals) != 4 {
		t.Fatalf("proposals %d", len(proposals))
	}
	s := ScoreProposals(proposals, drawn)
	if len(s.Pairs) != 3 || s.DrawnTotal != 3 || s.DrawnFound != 1 {
		t.Fatalf("pairs %+v found %d of %d", s.Pairs, s.DrawnFound, s.DrawnTotal)
	}
	byID := map[string]ScorePair{}
	for _, p := range s.Pairs {
		byID[p.ProposalID] = p
	}
	if p := byID["p-e1"]; p.DrawnID != "d-e1" || p.RMSEM != 0.2 || p.MeanOffsetM != 0.2 || p.DrawnCoverage < 0.9 {
		t.Fatalf("p-e1 %+v", p)
	}
	if p := byID["p-e2"]; p.DrawnID != "d-e2" || p.RMSEM != 0.5 || p.MeanOffsetM != -0.5 {
		t.Fatalf("p-e2 %+v", p)
	}
	if len(s.FalseHigh) != 1 || s.FalseHigh[0] != "p-x" {
		t.Fatalf("false %v", s.FalseHigh)
	}
	if s.Clause1RMSE || s.Clause2Count || s.Clause3Found || s.Clause4NoFalse || s.Passed {
		t.Fatalf("clauses %+v", s)
	}
	// Two eastbound drawn, two eastbound proposals; one each westbound; the
	// false proposal is its own street direction, so the counts disagree.
	for _, d := range s.Directions {
		if d.Direction == "KIRKHAM ST E" && (d.Drawn != 2 || d.High != 3) {
			t.Fatalf("eastbound counts %+v", d)
		}
	}

	// A clean site passes.
	clean := ScoreFeatures([]Feature{scoreLane("p1", "", 1, -1.7, 59, -1.7, high), scoreLane("p2", "", 59, 1.9, 1, 1.9, high)}, "", "", true)
	ok := ScoreProposals(clean, ScoreFeatures([]Feature{scoreLane("d1", "", 0, -1.8, 60, -1.8, ref), scoreLane("d2", "", 60, 1.8, 0, 1.8, ref)}, "reference", "lane0002", false))
	if !ok.Passed || ok.DrawnFound != 2 {
		t.Fatalf("clean site %+v", ok)
	}
	// Nothing drawn passes nothing.
	if ScoreProposals(clean, nil).Passed {
		t.Fatal("a site with no drawing passed")
	}
}

func TestScoreDrawnByAuthorLeavesApprovedLanesOut(t *testing.T) {
	mine := scoreLane("d1", "", 0, -1.8, 60, -1.8, nil)
	mine.Author = "lane0002"
	approved := scoreLane("d2", "", 0, -5, 60, -5, map[string]string{"proposal": "gms:p"})
	approved.Author = "lane0002"
	other := scoreLane("d3", "", 0, 5, 60, 5, nil)
	other.Author = "someone"
	got := ScoreDrawnBy([]Feature{mine, approved, other}, "lane0002")
	if len(got) != 1 || got[0].ID != "d1" {
		t.Fatalf("drawn by author: %+v", got)
	}
	if len(ScoreDrawnBy([]Feature{mine}, "")) != 0 {
		t.Fatal("an empty author selected lanes")
	}
}
