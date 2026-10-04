# P2-D: radar ports, identity and operational records

Give radar data explicit sensor/port provenance and record its operational work, while keeping the
existing tracking algorithm intact.

- **Status:** Planned; outlined, not implemented by #631
- **Layers:** Cross-cutting (schema, Go, APIs, CLI, web, macOS and documentation)
- **Target:** Phase 2; release allocation follows programme sizing and compatibility gates
- **Companion plans:** [programme and item ledgers](platform-vocabulary-and-data-model-plan.md) (Items 5, 8, 9 and 17)
- **Canonical:** [Platform hub](../platform/PLATFORM.md)

## Dependencies

Starts after the Phase 1 exit. Port identity precedes backfill; radar job integration follows P2-C
subject columns and reader isolation.

## Delivery outline

1. Introduce ports and lifecycle rules; decide the source of truth between saved configuration and startup flags.
2. Add sensor/port identity and backfill only where provenance is defensible; retain uncertainty rather than inventing identity.
3. Move tracker/controller code to its owned package, integrate radar_track_build jobs and record commands actually sent.
4. Plan and time the detection-key/deduplication rebuild separately, including rejects, storage requirements and a maintenance window.

## PR structure

Use separate ports, identity/backfill, tracker/jobs, command log and timed rebuild PRs. Device
timing remains a distinct acceptance gate.

## Acceptance

- [ ] Tracker fixtures preserve thresholds, method identity and output rows through the package move.
- [ ] Sensor backfill is accounted for; every sent command has the required record and reads do not become command events.
- [ ] Radar-owned jobs survive server restart without being claimed by another executor; the rebuild meets its published device/time/space envelope.

## Boundaries and risks

No radar file replay, new tracking method or implied identity for historical rows without evidence.

The programme's original decisions and detailed ledger remain authoritative for this outline. Where
an implementation needs a new decision, record it before writing the migration or changing a wire
contract. Keep existing compatibility rules and the applicable surface tests.
