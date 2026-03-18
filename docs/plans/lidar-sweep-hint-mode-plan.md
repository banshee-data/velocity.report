# HINT Sweep Mode — Remaining Polish Items

- **Status:** Complete (57/57 items complete) ✅
- **Layers:** Cross-cutting (L8 Analytics infrastructure)

- **Full design and implementation status:** [`docs/lidar/operations/hint-sweep-mode.md`](../lidar/operations/hint-sweep-mode.md)

## Remaining Items

### Prerequisites (macOS Visualiser)

- [x] **P1** — Display existing labels in `LabelPanelView`
  - [x] Accept selected `RunTrack?` in `LabelPanelView`
  - [x] Pre-populate `lastAssignedLabel` / `lastAssignedQuality` from run-track data
  - [x] Show checkmark on matching button for current label state
  - [x] Add "↻ carried" badge for carried-over labels
- [x] **P2** — Remove Export Labels button
  - [x] Remove "Export Labels" button from `SidePanelView`
  - [x] Remove `exportLabels()` from `AppState`

### Phase 5: Svelte Sweeps Page Updates

- [x] Inline "Continue" button for `awaiting_labels` state
- [x] Add `HINTState`, `HINTRound`, `LabelProgress` types to `lidar.ts`

### Phase 6: Mode Description Updates

- [x] Add page subtitle shared across modes
