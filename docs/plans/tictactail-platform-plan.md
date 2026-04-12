# TicTacTail Platform Plan

- **Status:** Proposed
- **Layers:** Cross-cutting (platform library)
- **Decision:** D-23 — [DECISIONS.md](../DECISIONS.md)
- **Backlog:** v0.8 — [BACKLOG.md](../BACKLOG.md)
- **Canonical:** [tictactail-library.md](../platform/architecture/tictactail-library.md)

- **Design specification (working name, ownership, core contract, aggregation, rendering, performance):** see [tictactail-library.md](../platform/architecture/tictactail-library.md).

## Working Name

Selected working name: `TicTacTail`

Why:

- `tic tac` captures cadence and regular refresh
- `tail` captures persistent history output
- distinctive enough for a standalone package/repo name

Use `TicTacTail` in code/docs unless we later have a strong reason to rename it.

## Goal

Build a generic platform that takes flat key/value samples, refreshes a live
surface quickly, emits aligned history rows for one active aggregate window,
and keeps the heavy logic outside app-specific code.

The split should be:

1. `tictactail`: all aggregation, live refresh, history rendering, alignment,
   colours, spinner, and output mechanics

2. application emitter/projector: choose keys and feed flat samples
3. thin adapter: CLI command or route glue

The VRLOG checker should be a small import/config layer on top of this.

---

## Likely Consumers

Beyond VRLOG:

- LiDAR live status tails
- sweep progress tails
- deploy progress / health tails
- transit rebuild tails
- generic operator CLIs with history + live row output

## Recommendation

Adopt `TicTacTail` as the working package/repo name, keep the first version to
one active aggregate window and one main pane, keep the API small, and keep
memory and buffer use strictly bounded. VRLOG should remain a thin
emitter/projector on top.