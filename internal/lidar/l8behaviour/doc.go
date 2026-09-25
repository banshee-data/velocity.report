// Package l8behaviour owns the road-user behaviour contracts of Layer 8
// (Analytics): what a behaviour result is, when it may be emitted, and why it
// was suppressed. It implements the Phase 6A trajectory subset and the Phase
// 6B pointwise following slice of docs/plans/lidar-behaviour-analytics-plan.md.
//
// A passage is one road user traversing one site, never a driver. Results are
// observables with units, uncertainty and provenance, never verdicts: there is
// no tailgating score, and the named net-time-gap bands are descriptive bins
// with no established threshold.
//
// The package has four parts.
//
//   - Vocabularies (vocabulary.go): closed, registered token sets for
//     suppression reasons, observation support, estimate stage, estimation
//     lifecycle, endpoint source, motion class and the uncertainty and
//     benchmark kinds. An unset value refuses to serialise.
//   - Results (result.go, metrics.go): Uncertainty with separate scopes,
//     version and input provenance, Measurement (a value XOR a suppression
//     reason, never a misleading zero), Outcome (a categorical label that
//     keeps references to its evidence), the registered metric identities and
//     the class-applicability table.
//   - Trajectories (trajectory.go, adapter.go): the per-instant physical
//     estimate behaviour code reads, the production-emission guard, support
//     accounting, and the mapping from l5tracks' solid-body estimate.
//   - Following (pathframe.go, following.go, fixtures.go): physical endpoints
//     projected onto a shared path, bumper-to-bumper spatial gap, net time
//     gap, their linearised uncertainty and suppression, and frozen analytic
//     fixtures with known answers.
//
// Production emission is gated. A result may reach a production surface only
// from a final-stage, established, observed estimate (G-SMO-1, Section 2.1);
// anything else is review-only and says why. Building the shared path from
// final trajectories, choosing a leader among candidates and aggregating
// exposure over an encounter are the next increment, and plug in through
// PathFrame, EvaluateFollowing and FollowingPoint.SupportedOpportunity.
//
// Dependency rule: as for every L8 package, this may depend on L1 to L7 and
// cross-cutting packages, never on transport, HTML, chart or client code. It
// reads l5tracks types and never changes l5tracks behaviour.
package l8behaviour
