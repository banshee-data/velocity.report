// Package l8behaviour owns the road-user behaviour contracts of Layer 8
// (Analytics): what a behaviour result is, when it may be emitted, and why it
// was suppressed. It implements the Phase 6A trajectory subset and the Phase
// 6B following slice of docs/plans/lidar-behaviour-analytics-plan.md: the
// pointwise equations, the local following path, leader choice and encounter
// exposure.
//
// A passage is one road user traversing one site, never a driver. Results are
// observables with units, uncertainty and provenance, never verdicts: there is
// no tailgating score, and the named net-time-gap bands are descriptive bins
// with no established threshold.
//
// The package has five parts.
//
//   - Vocabularies (vocabulary.go): closed, registered token sets for
//     suppression reasons, observation support, estimate stage, estimation
//     lifecycle, endpoint source, motion class, path condition, candidate
//     disposition and the uncertainty and benchmark kinds. An unset value
//     refuses to serialise.
//   - Results (result.go, metrics.go): Uncertainty with separate scopes,
//     version and input provenance, Measurement (a value XOR a suppression
//     reason, never a misleading zero), Outcome (a categorical label that
//     keeps references to its evidence), the registered metric identities and
//     the class-applicability table.
//   - Trajectories (trajectory.go, adapter.go): the per-instant physical
//     estimate behaviour code reads, the production-emission guard, support
//     accounting, and the mapping from l5tracks' solid-body estimate.
//   - Following at an instant (pathframe.go, following.go, fixtures.go):
//     physical endpoints projected onto a shared path, bumper-to-bumper
//     spatial gap, net time gap, their linearised uncertainty and suppression,
//     and frozen analytic fixtures with known answers.
//   - Following over an encounter (localpath.go, pairing.go, encounter.go,
//     encounter_fixtures.go): a short, directed empirical path fitted per
//     follower from observed evidence, refused rather than guessed for weak
//     support, forks, merges, crossings and reversals; the nearest credible
//     leader, or ambiguous_leader when two are not separable; and following
//     exposure with valid-time denominators, band durations and rates,
//     interval uncertainty and suppression counts, run end to end by
//     AnalyseFollowing over frozen multi-frame scenarios.
//
// Production emission is gated. A result may reach a production surface only
// from a final-stage, established, observed estimate (G-SMO-1, Section 2.1),
// measured along a path fitted from final estimates; anything else is
// review-only and says why. An encounter over non-final estimates carries its
// values only in a review-labelled Provisional block.
//
// Dependency rule: as for every L8 package, this may depend on L1 to L7 and
// cross-cutting packages, never on transport, HTML, chart or client code. It
// reads l5tracks types and never changes l5tracks behaviour.
package l8behaviour
