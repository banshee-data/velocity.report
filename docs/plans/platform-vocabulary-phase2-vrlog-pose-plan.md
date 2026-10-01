# P2-E: VRLOG consolidation and pose contracts

Unify recording infrastructure and make substantive frame/stage contracts explicit. Legacy display
snapshots cannot supply evidence that was never recorded.

- **Status:** Planned; outlined, not implemented by #631
- **Layers:** Cross-cutting (schema, Go, APIs, CLI, web, macOS and documentation)
- **Target:** Phase 2; release allocation follows programme sizing and compatibility gates
- **Companion plans:** [programme and item ledgers](platform-vocabulary-and-data-model-plan.md) (Items 10, 12 and 13)
- **Canonical:** [Platform hub](../platform/PLATFORM.md)

## Dependencies

Starts after the Phase 1 exit. Legacy conversion follows the display adapter; coordinate-contract
changes follow the relevant tracking evidence gates.

## Delivery outline

1. Specify stream/profile and frame/stage semantics, including capability refusals and compatibility with each existing layout.
2. Provide a display adapter and a verified converter from 0.5 recordings to a display-profile container.
3. Replace legacy recorder/replayer paths only after playback and evidence consumers accept the replacement; preserve provenance and checkpoints.
4. Version method identities affected by stage/frame meaning changes; retire legacy evidence tables only under the required migration/retention checks.

## PR structure

Separate contract/capability, adapter, converter, recorder replacement and pose/stage migrations
rather than one container rewrite.

## Acceptance

- [ ] Corpus recordings convert and play back frame for frame; originals are retained until conversion is verified.
- [ ] Display-only conversions are refused by consumers requiring analysis evidence; both provenance and fidelity remain explicit.
- [ ] Frame/yaw/heading and final-stage round trips pass; recovery, interrupted writes and reader capability refusals are covered.

## Boundaries and risks

No reconstructed raw evidence, new default live capture or unverified device/power-loss claim.
Those retain the VRLOG companion plan's gates.

The programme's original decisions and detailed ledger remain authoritative for this outline. Where
an implementation needs a new decision, record it before writing the migration or changing a wire
contract. Keep existing compatibility rules and the applicable surface tests.
