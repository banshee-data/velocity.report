package headway

import "regexp"

// VerdictPattern matches language a headway report must never print
// (Sections 1 and 8.3 of docs/plans/lidar-behaviour-analytics-plan.md): no
// verdict, score, category or trait of a road user, in any form, including
// in negation. It is exported so that every package producing a report,
// the oracle's and the field run's, scans what it prints with one list.
var VerdictPattern = regexp.MustCompile(`(?i)tailgat|aggress|driver|risk|score|verdict|unsafe|danger|violat|offend|propensity|profil`)
