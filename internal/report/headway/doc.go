// Package headway is the first headway report of Section 10.4 of
// docs/plans/lidar-behaviour-analytics-plan.md: observed following exposure
// for leader/follower encounters at one site, rendered through the Go report
// pipeline (SVG charts in internal/report/chart, Typst in
// internal/report/typst).
//
// The contract is the Report type, versioned as ContractID and written as
// the report's data.json. It is built from l8behaviour following analyses
// held in memory; this package stores nothing and runs no estimator.
//
// What the contract guarantees:
//
//   - Status. Every report is a synthetic oracle or provisional; promoted is
//     reserved and refused until the field promotion gates exist. The status
//     is a closed vocabulary that refuses to serialise unset, and its label is
//     printed on every page and every chart. A report whose trajectories come
//     from the analytic fixture estimator must be a synthetic oracle, and a
//     synthetic oracle may contain nothing else.
//   - Names. Every metric, reason, stage, source, role and condition is the
//     l8behaviour registry id or vocabulary token, verbatim. The report
//     invents no metric names, and names a passage and a site interaction,
//     never a road user's character.
//   - Values. A value comes from an encounter's production measurements when
//     the encounter is final, and from its review-labelled provisional block
//     otherwise; which block was read is recorded on the row. A suppressed
//     value is shown as its reason and no number, never as zero.
//   - Unsupported time. The intervals of an encounter that are not valid
//     following time are listed by reason, and the review-only predicted gap
//     is listed apart with its coast age. Neither enters an aggregate.
//   - Aggregates. Encounter values are pooled only within one version group
//     (estimate stage, estimator, observation model, parameter hash and
//     method with its parameter hash), and only where supported, with the
//     sample count, the suppressed count by reason and the opportunity
//     denominator. The time-weighted net-time-gap distribution covers the
//     encounters whose band exposure is supported, and every other second of
//     accounted encounter time is shown beside it under its reason.
//   - Bands. The named bands are descriptive bins with no established
//     threshold, rendered as neutral rules; the report carries no verdict,
//     score or category about any road user.
//
// Staging (Section 10.4): this package delivers the sprint 0.5.2.3 oracle
// (Oracle and OracleInput) and the contract the sprint 0.5.2.4 provisional
// slice will feed from persisted encounters. Field promotion is not built.
package headway
