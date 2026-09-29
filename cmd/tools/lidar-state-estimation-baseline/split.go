package main

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// splitRecord is phase0-summary.json's record of the frozen split a corpus
// run was made under, and whether it was declared a held-out score.
type splitRecord struct {
	Digest     string `json:"split_digest"`
	Revision   int    `json:"revision"`
	FileSHA256 string `json:"file_sha256"`
	HeldOut    bool   `json:"held_out"`
}

// ids is the corpus's case IDs in replay order.
func (c corpus) ids() []string {
	ids := make([]string, len(c.Cases))
	for i, cc := range c.Cases {
		ids[i] = cc.ID
	}
	return ids
}

// corpusSplitUse is a corpus run's frozen split: the split, its record for
// the summary, and each selected case's use of it.
type corpusSplitUse struct {
	split  *annotation.FrozenSplit
	record *splitRecord
	uses   map[string]*replayeval.SplitUse
}

// corpusSplit holds a corpus run to its frozen split. Every selected case
// must have a role in it; a held-out score replays held-out cases only, and
// any other run replays none, as annotation.FrozenSplit.CaseRoles rules. A
// held-out claim without a split is refused: it would name nothing it was
// held out of. With no split the run is as it always was, and the summary
// says so by carrying no split; nothing then stops a held-out case from
// replaying, so the held-out guarantee covers only runs given a split.
func corpusSplit(path string, heldOut bool, selected corpus) (corpusSplitUse, error) {
	if path == "" {
		if heldOut {
			return corpusSplitUse{}, fmt.Errorf("-held-out needs -split-manifest: a held-out score names the frozen split it was held out of")
		}
		return corpusSplitUse{}, nil
	}
	f, err := annotation.LoadFrozenSplit(path)
	if err != nil {
		return corpusSplitUse{}, err
	}
	roles, err := f.CaseRoles(selected.ids(), heldOut)
	if err != nil {
		return corpusSplitUse{}, err
	}
	uses := make(map[string]*replayeval.SplitUse, len(roles))
	for id, role := range roles {
		uses[id] = &replayeval.SplitUse{SplitDigest: f.SplitDigest, Revision: f.Revision, CaseID: id, Role: string(role), HeldOut: heldOut}
	}
	return corpusSplitUse{split: f, uses: uses,
		record: &splitRecord{Digest: f.SplitDigest, Revision: f.Revision, FileSHA256: f.FileDigest, HeldOut: heldOut}}, nil
}

// checkCaptures holds every resolved case's captures to the split: a case's
// role binds to its captures, so a capture the index resolves for a case
// must be one of the captures the split names for it. Without a split there
// is nothing to check.
func (u corpusSplitUse) checkCaptures(cases []resolvedCorpusCase) error {
	if u.split == nil {
		return nil
	}
	for _, c := range cases {
		if err := u.split.CheckCaseCaptures(c.corpusCase.ID, c.paths); err != nil {
			return err
		}
	}
	return nil
}
