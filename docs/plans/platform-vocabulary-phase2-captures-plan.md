# P2-A: capture identity and clock integrity

Identify capture content independently of its filesystem location and make clock discontinuities
explicit. These change the data model and probe behaviour.

- **Status:** Planned; outlined, not implemented by #631
- **Layers:** Cross-cutting (schema, Go, APIs, CLI, web, macOS and documentation)
- **Target:** Phase 2; release allocation follows programme sizing and compatibility gates
- **Companion plans:** [programme and item ledgers](platform-vocabulary-and-data-model-plan.md) (Items 3 and D-27 probe backlog)
- **Canonical:** [Platform hub](../platform/PLATFORM.md)

## Dependencies

Starts after the Phase 1 exit. P2-B depends on the digest identity; survey exports consume its
provenance later.

## Delivery outline

1. Decide whether duplicate-digest rows represent one capture with locations or another explicit model; specify the upgrade and reprobe policy.
2. Compute whole-file SHA-256 during probing; retain capture-kind and sensor identity semantics without assuming a file path identifies content.
3. Keep first/last packet timestamps in file order; add earliest/latest timestamps and backward-step counts with a specified threshold.
4. Define handling of stepped captures, sequence exclusion/review and export part boundaries; add extent checks only after their provenance is available.

## PR structure

Separate duplicate/identity design, digest migration/probe implementation and clock-integrity
implementation. Measure probe cost and test recovery before rollout.

## Acceptance

- [ ] Moving a volume or using nested roots/symlinks does not create a new content identity after probing.
- [ ] Fixtures cover empty, truncated and non-monotonic captures, reprobes and failed reads; extrema and counts match packet order.
- [ ] Migration row-accounting preserves provenance and references; an unprobed capture is never given an invented digest.

## Boundaries and risks

No automatic deletion of duplicate files, shared queue rollout or survey authoring. The clock
threshold and duplicate resolution remain explicit design decisions.

The programme's original decisions and detailed ledger remain authoritative for this outline. Where
an implementation needs a new decision, record it before writing the migration or changing a wire
contract. Keep existing compatibility rules and the applicable surface tests.
