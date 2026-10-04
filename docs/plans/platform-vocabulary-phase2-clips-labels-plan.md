# P2-B: clip, selection and label consolidation

Give clips one selection record and labels one authoritative write path. This consolidates storage
and workflows beyond the Phase 1 names.

- **Status:** Planned; outlined, not implemented by #631
- **Layers:** Cross-cutting (schema, Go, APIs, CLI, web, macOS and documentation)
- **Target:** Phase 2; release allocation follows programme sizing and compatibility gates
- **Companion plans:** [programme and item ledgers](platform-vocabulary-and-data-model-plan.md) (Items 6 and 7)
- **Canonical:** [Platform hub](../platform/PLATFORM.md)

## Dependencies

Starts after the Phase 1 exit. Digest-based clip protection follows P2-A; clip-job integration
follows the required P2-C subject columns. Label consolidation can proceed independently.

## Delivery outline

1. Fold selections into clips with explicit selection/selector provenance, generated query fields and a digest-based held-out index.
2. Preserve the stored replay offsets without recomputation; set aside incompatible rows with their reason and row accounting.
3. Integrate candidates into the Clips workflow without losing hand-made clips, finder provenance or pack compatibility.
4. Consolidate label storage and writers with compatibility projections; preserve each existing label and its precedence policy.

## PR structure

Separate clip migration, held-out protection, candidate workflow and label consolidation PRs, each
with migration and client validation.

## Acceptance

- [ ] Hand-made and finder-made clips replay the same windows; conflicting migration rows are fully accounted for.
- [ ] A held-out capture cannot be selected for tuning under another path or location.
- [ ] Labels written through compatibility routes read back consistently and export once; client/workflow tests cover both legacy and new paths.

## Boundaries and risks

No silent window repair or label conflict resolution. Do not drop shipped storage merely because
new terminology is in place.

The programme's original decisions and detailed ledger remain authoritative for this outline. Where
an implementation needs a new decision, record it before writing the migration or changing a wire
contract. Keep existing compatibility rules and the applicable surface tests.
