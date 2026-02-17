package l5tracks

import (
	"github.com/banshee-data/velocity.report/internal/lidar"
)

// Function re-exports for track-to-cluster association.

// HungarianAssign solves the assignment problem using the Hungarian algorithm.
// Given a cost matrix, it returns an assignment vector where assignment[i] is
// the column index assigned to row i, or -1 if unassigned.
var HungarianAssign = lidar.HungarianAssign
