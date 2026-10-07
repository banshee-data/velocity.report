# P1-B: LiDAR terminology across existing surfaces

Make existing LiDAR operations use the agreed names across storage and clients. A clip remains the
same evaluation window after its names change.

- **Status:** Planned; outlined, not implemented by #631
- **Layers:** Cross-cutting (schema, Go, APIs, CLI, web, macOS and documentation)
- **Target:** Phase 1; release allocation follows programme sizing and compatibility gates
- **Companion plans:** [programme and item ledgers](platform-vocabulary-and-data-model-plan.md) (Items 2, 3, 6 and 7)
- **Canonical:** [Platform hub](../platform/PLATFORM.md)

## Dependencies

Follows P1-A. Coordinate shared CLI and protocol spellings with P1-D; no capture digest or
selection consolidation is required.

## Landed ahead of this plan

The web UI coherence PR (#707) applied the agreed words to user-facing copy only. It changed no
route, type, response key, query parameter or schema name, so every step below remains.

- The LiDAR navigation is one group, spelt LiDAR, in workflow order: Captures, Clips, Segments,
  Runs, Tracks, Sweeps. Scene Map left the navigation and is reached from Clips as "Clip
  locations".
- "Replay case", and "scene" where it meant a case, read "clip" on the Clips, Runs, Tracks,
  Captures and Segments pages and in the web API client's error messages.
- The macOS run browser's Case column is headed Source, because the cell holds the run's source
  file name rather than a clip (§ Ledger 6 is amended to match). It becomes Clip once runs record
  their clip.

## Delivery outline

1. Trace clip/run and capture/sequence/period/volume names through existing schema metadata, store methods, handlers and response types.
2. Rename existing metadata without changing stored values, keys, row counts or relationships; update indexes, triggers and embedded schema references.
3. Follow each slice through web and macOS copy, navigation, API clients and label/annotation type names.
4. Keep old external route, field and persisted-file spellings readable through explicit aliases or versioned compatibility readers.

## PR structure

PRs follow consumer boundaries: clip/run names; capture index names; label/annotation names. Each
carries its store, API and client changes together.

## Acceptance

- [ ] Old and new routes perform the same operation and return equivalent values after normalising renamed keys.
- [ ] Migrated rows and relationships match the pre-migration fixture; fresh and migrated databases expose the same schema.
- [ ] Web and macOS client checks cover renamed surfaces; replay windows, labels and recording behaviour remain unchanged.

## Boundaries and risks

No digest identity, selection folding, label merge, candidates-panel redesign or new storage
relationship. Scene retains its geometry sense.

The programme's original decisions and detailed ledger remain authoritative for this outline. Where
an implementation needs a new decision, record it before writing the migration or changing a wire
contract. Keep existing compatibility rules and the applicable surface tests.
