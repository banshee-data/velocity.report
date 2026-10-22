# P1-A: terminology contract and companion plans

Give every existing concept one agreed name before changing its consumers. The contract describes
current concepts and marks proposed capabilities as future work.

- **Status:** Superseded by [D-31](../DECISIONS.md#d-31--renames-are-cutovers-tombstones-for-people-aliases-for-the-app) (October 20, 2026): the vocabulary is published in the programme plan and the decision this package would have recorded is D-31, because D-28 went to the research briefing; the alias inventory is the [compatibility register](../platform/operations/compatibility-register.md)
- **Layers:** Cross-cutting (schema, Go, APIs, CLI, web, macOS and documentation)
- **Target:** Phase 1; release allocation follows programme sizing and compatibility gates
- **Companion plans:** [programme and item ledgers](platform-vocabulary-and-data-model-plan.md) (Item 1)
- **Canonical:** [Platform hub](../platform/PLATFORM.md)

## Dependencies

This is the first terminology package. P1-B, P1-C and P1-D use its accepted definitions and alias
inventory.

## Delivery outline

1. Inventory the names and meanings across the schema, Go stores, APIs, CLI, web, macOS, public pages and persisted files.
2. Publish the vocabulary in PLATFORM.md, LIDAR.md, DATA_STRUCTURES.md and the tuning glossary; use D-31 and the terminology-first delivery rule.
3. Reconcile the catalogue, export, archive-ingest, route-capture, VRLOG, geography and typed-ID companion plans with that vocabulary.
4. Create an alias inventory with each old spelling, new spelling, consumer, compatibility period and owning follow-up plan.

## PR structure

PR 1 records vocabulary and D-31; PR 2 aligns companion plans and publishes the consumer/alias
inventory.

## Acceptance

- [ ] Definitions distinguish captures, evidence, estimates, tracks, clips, selections, labels, annotations, sites, deployments and surveys.
- [ ] Every existing surface is assigned to a terminology package; future schema relationships are identified as Phase 2.
- [ ] D-31 exists and every companion plan uses the same definitions; relative links and documentation checks pass.

## Boundaries and risks

No runtime code, schema feature, new ID generation or conversion is implemented. A future noun must
not imply that its storage model exists.

The programme's original decisions and detailed ledger remain authoritative for this outline. Where
an implementation needs a new decision, record it before writing the migration or changing a wire
contract. Keep existing compatibility rules and the applicable surface tests.

## Vocabulary contract

The [published platform vocabulary](../platform/PLATFORM.md#platform-vocabulary) governs
this plan: capture is a static packet file; clip is an evaluation or annotation window;
run is a processing pass; labels are track verdicts; annotations are reviewed point masks.
Site means place, deployment means one sensor’s placement over a period, and survey means
one deployment’s measurement over a period. Scene is reserved for physical geometry.
Historical schema inventories describe their original state; implementation follows the
[D-31 cutovers](platform-vocabulary-and-data-model-plan.md#phase-1-under-d-31-three-renames)
and the [compatibility register](../platform/operations/compatibility-register.md).
New storage relationships remain Phase 2.
