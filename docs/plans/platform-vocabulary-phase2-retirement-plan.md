# P2-G: compatibility retirement

Remove compatibility surfaces only after their replacements are deployed and consumers have had a
release to move. Cleanup follows adoption.

- **Status:** Planned; outlined, not implemented by #631
- **Layers:** Cross-cutting (schema, Go, APIs, CLI, web, macOS and documentation)
- **Target:** Phase 2; release allocation follows programme sizing and compatibility gates
- **Companion plans:** [programme and item ledgers](platform-vocabulary-and-data-model-plan.md) (Item 18)
- **Canonical:** [Platform hub](../platform/PLATFORM.md)

## Dependencies

Starts after the Phase 1 exit and all relevant replacements; the programme's Item 18 requires every
other item to have shipped one release earlier.

## Delivery outline

1. Audit the alias inventory against external consumers, deployed clients, persisted files and release history.
2. Remove eligible routes, commands, flags, wire spellings and readers only after their compatibility period.
3. Remove redundant projections/advisory columns with migration row-accounting and the lifecycle tests that previously held them.
4. Update operator migration guidance and retain historical decisions/migrations as history.

## PR structure

Group retirement PRs by consumer/release boundary; schema drops and external-interface removal
remain independently reviewable.

## Acceptance

- [ ] Every removed surface has a replacement and evidence of the required compatibility release.
- [ ] Greps find retired names only in permitted history/migrations; current clients and upgrade fixtures pass.
- [ ] Dropped data is accounted for and restore/rollback limitations are documented before deployment.

## Boundaries and risks

No alias is removed merely to make a grep pass. No unrelated retention cleanup or deferred feature
is included.

The programme's original decisions and detailed ledger remain authoritative for this outline. Where
an implementation needs a new decision, record it before writing the migration or changing a wire
contract. Keep existing compatibility rules and the applicable surface tests.
