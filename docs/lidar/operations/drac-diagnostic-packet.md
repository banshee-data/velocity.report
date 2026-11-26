# DRAC diagnostic review handoff

- **Status:** twelve reserved review slots; selection and operator review blocked on development exports
- **Scope:** development-only diagnosis, no scoring, tuning or physical qualification
- **Manifest:** [twelve-window manifest](drac-review-windows.json)

## Available evidence and exact missing dependency

The cloud checkout contains the authored reports and configuration, but no l8fol0008 evidence
databases, following exports, source manifests or recordings. The Kirk development PCAP was
retrieved through its repository LFS path for existing tests; it is not a replacement for the
campaign exports. No sealed or held-out capture was accessed. The manifest therefore reserves
12 slots, with null times and track identities. It does **not** claim 12 selected or reviewed
windows, and carries no invented verdicts.

Provide only the three development cases in the
[source/v2 rerun report](following-identity-rerun-2026-11.md): `kirk0`,
`columbus-broadway`, `embarcadero-bryant`. Its archive inventory is
`/Volumes/banshee-results/velocity-campaign/l8fol0008-20261114-1752/evidence`, with verification
record SHA-256 `69994b5b22b09ced56ecb04f5877a240693a42baede4661a287b5176057786c0`.
That is a provenance locator, not a request to access the NAS or replace required local assets.
The owner must supply verified copies from retained local evidence into the cloud workspace.
Required files per case are:

1. Both repeat following evidence exports, their context bindings and schema-3 source manifest.
2. Completed observations database with the selected estimate version, parameter fingerprint
   and calibration metadata; include a consistent database snapshot rather than omit a WAL.
3. First scoring recording, header and replay sidecar, plus context recording for any proposed
   warm-up sample outside the scoring recording. A missing interval stays unavailable.
4. Current cohort manifest, actual frozen-split registry and usable eligibility ledger under
   the existing access rules. A synthetic empty registry used in tests is not campaign policy.
5. Per-file SHA-256 verification inventory and the campaign build/configuration identity.

Expected source/v2 IDs are pinned in the handoff manifest. Compare calibrations with the rerun
report and retain `config/experiments/weekend-following-baseline.json`, whose semantic parameter
fingerprint is `ab0c4035780dcc678bc21b112890c1db6cdc9c802eb3d1ab5d0f8d442e872d08`.
Re-authorise development use against current policy before loading samples. A stale manifest,
changed source/window, absent binding or protected split must stop that case. Do not borrow
validation sites, qualification material, or obsolete track IDs to fill the quota.

## Twelve-slot selection

Each development source receives four slots: an eligible closing control, a `no_leader` example,
an association/path/class exclusion, and a record-gap/missing-evidence example. The manifest's
rule chooses the earliest sample with readable ±1 second context in the scoring interval;
it prefers non-overlap, records overlap, and marks an absent stratum unavailable. Strata are
software outcomes used for selection, not expected reviewer answers.

Freeze selected sample and window times as decimal Unix-nanosecond strings, both track IDs,
source/calibration/version, export and recording hashes, candidate IDs/rejection reasons,
policy digest and decision/ledger reference before review. Preserve unknown leaders as null.
Check exact seek landing and context coverage; do not infer capture-relative offsets.

Earlier [Kirk nominations](following-baseline-reproducibility-2026-11.md#source-bound-review-nominations)
and [S2 nominations](following-development-prefixes-2026-11.md#source-bound-review-nominations)
are useful provenance examples. They belong to older exports. Their track IDs must not be
silently copied into the fresh source/v2 rerun; same timestamps do not establish identity.

## Review card and verdicts

Every card should show the frozen source/version, exact sample, both speeds, closing speed,
nominal gap and interval when available, DRAC value or exclusion, candidate leaders and reasons,
follower path and time coverage, observed returns/boxes, occlusion and fragmentation notes,
and uncertainty/missing fields. Keep estimator output separate from reviewer observations.

| Review classification      | Evidence needed                                                                                                                                                                | Do not infer from                                          |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------- |
| No true leader             | Sufficient visible roadway/path and temporal context establish no leading vehicle on the follower's path in this period                                                        | A `no_leader` software row alone                           |
| Missed leader              | Reviewer can identify a supported same-path physical leader, while the selected candidate is absent, suppressed or wrong; record where detection, identity or selection failed | An adjacent/crossing track or an unsupported extrapolation |
| Ambiguous association/path | Multiple plausible leaders, fragment correspondence, turn/merge path or lane membership cannot be resolved; retain candidate alternatives                                      | Choosing the nearest box to force a value                  |
| Insufficient evidence      | Missing context, record gap, unsupported endpoints, inadequate returns or uncertainty prevent a conclusion; name the missing evidence                                          | Treating missing data as evidence of no leader             |

An eligible-closing control may be recorded as `supported_pair` if supported; the four failure
classifications are not forced onto a sound control. Pair support does not validate the physical
gap, speeds or DRAC. Record reviewer, review time, evidence links and confidence/reason text.
A visible bumper is not an independent physical reference. Preserve non-final estimates and
uncertainty exclusions; do not relax gates or adjust parameters from this packet.

## Remaining execution

The next cloud step is provenance/policy preflight and deterministic selection from those
exports, followed by assembling the 12 review cards or explicit unavailable entries. The
Mac dependency is exact-source seek/step and human review of visible geometry, leader identity
and path. This cloud job cannot certify native seek landing or perform that operator review.
No export rerun, Mac execution, parameter adoption or physical accuracy claim is reported here.
