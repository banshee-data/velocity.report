# HINT Sweep Mode — Remaining Polish Items

**Status**: In progress (46/57 items complete)

**Full design and implementation status:** [`docs/lidar/operations/hint-sweep-mode.md`](../lidar/operations/hint-sweep-mode.md)

## Remaining Items

### Prerequisites (macOS Visualiser)

- [ ] **P1** — Display existing labels in `LabelPanelView`
  - [ ] Accept selected `RunTrack?` in `LabelPanelView`
  - [ ] Pre-populate `lastAssignedLabel` / `lastAssignedQuality` from run-track data
  - [ ] Show checkmark on matching button for current label state
  - [ ] Add "↻ carried" badge for carried-over labels
- [ ] **P2** — Remove Export Labels button
  - [ ] Remove "Export Labels" button from `SidePanelView`
  - [ ] Remove `exportLabels()` from `AppState`

### Phase 5: Svelte Sweeps Page Updates

- [ ] Inline "Continue" button for `awaiting_labels` state
- [ ] Add `HINTState`, `HINTRound`, `LabelProgress` types to `lidar.ts`

### Phase 6: Mode Description Updates

- [ ] Add page subtitle shared across modes
