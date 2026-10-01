# P1-D: shared interface and file terminology

Make shared interfaces use the same words without changing the operation behind a command or the
meaning of a persisted field.

- **Status:** Planned; outlined, not implemented by #631
- **Layers:** Cross-cutting (schema, Go, APIs, CLI, web, macOS and documentation)
- **Target:** Phase 1; release allocation follows programme sizing and compatibility gates
- **Companion plans:** [programme and item ledgers](platform-vocabulary-and-data-model-plan.md) (Items 4, 5, 10, 12, 14, 15 and 16)
- **Canonical:** [Platform hub](../platform/PLATFORM.md)

## Dependencies

Follows P1-A. Integrate names selected by P1-B and P1-C, then complete the Phase 1 compatibility
checks.

## Delivery outline

1. Rename existing CLI/flag/config surfaces and provide aliases that invoke the existing operation.
2. Align source, dataset, protocol and file spellings through compatibility mappings; reserve existing protocol numbers and accept existing persisted inputs.
3. Define VRLOG display versus observation layouts and pose/heading/yaw/frame terms accurately; rename a field only when its meaning is identical.
4. Update existing public-page names and redirects only where the current page already represents the new concept; publish the consolidated alias inventory.

## PR structure

Use separate CLI/config, protocol/file and public-copy PRs. A field whose meaning differs waits for
its Phase 2 contract.

## Acceptance

- [ ] Existing command/flag spellings work and identify their replacements; golden fixtures differ only in authorised spellings.
- [ ] Both VRLOG layouts remain readable by their existing readers; no conversion or new stream capability is required.
- [ ] Protocol/config compatibility, old public links and affected clients pass; all Phase 1 surfaces and aliases are accounted for.

## Boundaries and risks

No port lifecycle, new operation, manifest feature, changed coordinate conversion, new survey
model, export tier or generated ID format.

The programme's original decisions and detailed ledger remain authoritative for this outline. Where
an implementation needs a new decision, record it before writing the migration or changing a wire
contract. Keep existing compatibility rules and the applicable surface tests.
