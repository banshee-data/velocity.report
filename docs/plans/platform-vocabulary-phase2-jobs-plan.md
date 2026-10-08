# P2-C: shared job subjects and queue ownership

Make every job name its subject and give server and worker scheduling one ownership rule. Renaming
the existing queue alone does not provide this behaviour.

- **Status:** Planned; outlined, not implemented by #631
- **Layers:** Cross-cutting (schema, Go, APIs, CLI, web, macOS and documentation)
- **Target:** Phase 2; release allocation follows programme sizing and compatibility gates
- **Companion plans:** [programme and item ledgers](platform-vocabulary-and-data-model-plan.md) (Items 3 and 11)
- **Canonical:** [Platform hub](../platform/PLATFORM.md)

## Dependencies

Starts after the Phase 1 exit. Its subject columns precede clip/radar job integration; shared
claiming is enabled only after contention tests pass.

## Delivery outline

1. Add subject, executor, claim and output metadata; backfill existing kinds with explicit row-accounting.
2. Replace migration 000058 kind triggers before renaming vrlog_record or adding another kind; preserve legacy job-state transitions.
3. Make readers select their supported kinds/executors and honour ownership during claims, restart and recovery.
4. Move distributed workers onto the shared queue with atomic claims and a documented failure/retry lifecycle.
5. Run sweeps as a job kind, so a sweep requested while another runs waits in the queue instead of failing with "sweep already in progress" ([workflow UI](lidar-ui-workflow-plan.md), T5).

## PR structure

Separate columns/backfill, reader isolation, atomic claims and worker migration; keep the queue
compatibility window explicit.

## Acceptance

- [ ] Every migrated job has a valid subject under the agreed rules; legacy orphaned jobs retain a recoverable disposition.
- [ ] Two workers and the server cannot run the same claimed job twice in a contention test.
- [ ] Restart/requeue behaviour does not steal another executor's work; failed and interrupted jobs retain their output/provenance contract.
- [ ] A sweep requested while one runs is queued, and starts when the running sweep ends.

## Boundaries and risks

No new radar algorithm or unreviewed job kinds. Ownership and retry policy are settled before
changing scheduling.

The programme's original decisions and detailed ledger remain authoritative for this outline. Where
an implementation needs a new decision, record it before writing the migration or changing a wire
contract. Keep existing compatibility rules and the applicable surface tests.
