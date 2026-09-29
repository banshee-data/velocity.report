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

// corpusSplit holds a corpus run to its frozen split. Every selected case
// must have a role in it; a held-out score replays held-out cases only, and
// any other run replays none, as annotation.FrozenSplit.CaseRoles rules. A
// held-out claim without a split is refused: it would name nothing it was
// held out of. With no split the run is as it always was, and the summary
// says so by carrying no split.
func corpusSplit(path string, heldOut bool, selected corpus) (*splitRecord, map[string]*replayeval.SplitUse, error) {
	if path == "" {
		if heldOut {
			return nil, nil, fmt.Errorf("-held-out needs -split-manifest: a held-out score names the frozen split it was held out of")
		}
		return nil, nil, nil
	}
	f, err := annotation.LoadFrozenSplit(path)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]string, len(selected.Cases))
	for i, c := range selected.Cases {
		ids[i] = c.ID
	}
	roles, err := f.CaseRoles(ids, heldOut)
	if err != nil {
		return nil, nil, err
	}
	uses := make(map[string]*replayeval.SplitUse, len(roles))
	for id, role := range roles {
		uses[id] = &replayeval.SplitUse{SplitDigest: f.SplitDigest, Revision: f.Revision, CaseID: id, Role: string(role), HeldOut: heldOut}
	}
	return &splitRecord{Digest: f.SplitDigest, Revision: f.Revision, FileSHA256: f.FileDigest, HeldOut: heldOut}, uses, nil
}
