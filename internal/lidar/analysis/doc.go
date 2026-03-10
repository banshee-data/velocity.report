// Package analysis generates structured JSON reports from .vrlog recordings.
//
// It provides two operations:
//
//   - [GenerateReport]: analyse a single .vrlog and produce an AnalysisReport
//   - [CompareReports]: compare two AnalysisReports and produce a ComparisonReport
//
// The package reads already-recorded vrlog snapshots via [recorder.Replayer]
// and computes track quality, speed distribution, and frame-level statistics
// without re-running the perception pipeline.
package analysis
