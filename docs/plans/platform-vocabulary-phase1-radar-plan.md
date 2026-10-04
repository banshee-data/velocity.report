# P1-C: radar terminology across existing surfaces

Call radar readings, detections and tracks by the shared vocabulary while preserving the existing
grouping algorithm and report results.

- **Status:** Planned; outlined, not implemented by #631
- **Layers:** Cross-cutting (schema, Go, APIs, CLI, web, macOS and documentation)
- **Target:** Phase 1; release allocation follows programme sizing and compatibility gates
- **Companion plans:** [programme and item ledgers](platform-vocabulary-and-data-model-plan.md) (Items 8 and 9)
- **Canonical:** [Platform hub](../platform/PLATFORM.md)

## Dependencies

Follows P1-A. Coordinate common dataset and CLI aliases with P1-D; no port table, sensor backfill
or new queue is required.

## Delivery outline

1. Inventory transit, reading and detection names in radar tables, Go types, routes, CLI and report copy.
2. Apply metadata and identifier renames with the existing keys, columns, values and relationships preserved.
3. Add route and dataset-spelling aliases around the current handlers and tracker; update consumers and reference documents.
4. Keep tracker placement, timing, gap threshold, model identity and scheduling unchanged.

## PR structure

PR 1 covers storage/types; PR 2 carries API/report consumers and aliases. Separate further where
compatibility can be proved independently.

## Acceptance

- [ ] A fixture produces the same tracks and report values before and after, normalising names only.
- [ ] Old and new API/dataset spellings remain equivalent and documented.
- [ ] Schema row-accounting, radar tests and documentation checks pass; retired product words remain only in compatibility and history.

## Boundaries and risks

No sensor/port identity columns, tracker package move, command logging, detection-key rebuild or
shared scheduling.

The programme's original decisions and detailed ledger remain authoritative for this outline. Where
an implementation needs a new decision, record it before writing the migration or changing a wire
contract. Keep existing compatibility rules and the applicable surface tests.
