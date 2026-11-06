# Acceptance criteria records

Predeclared acceptance limits that a scorer reads and cites by path and SHA-256 digest. Each is
hand-authored configuration, pinned by a decision before any held-out result is read.

| File                           | What it is                                                                 |
| ------------------------------ | -------------------------------------------------------------------------- |
| `following-limits.schema.json` | The following-criteria layout (schema version 1)                           |
| `following-limits.draft.json`  | The unpinned template: every value `null`, no decision. It chooses nothing |

The limits are set in [l8fol0004](../../data/experiments/l8fol/l8fol0004.md)'s decide step. To pin
them, copy the template to a new file, fill every value, and set `status` to `pinned` with the
`D-NN` and the date. Never edit a pinned file: an amendment is a new file under a new decision, so
the scores read under the old one still cite its digest. The format, how the scorer reads it, and
what it admits are in the
[point annotation tool guide](../../docs/lidar/operations/point-annotation-tool.md#the-following-criteria-record).
The reader is
[`internal/lidar/perframeeval/following_criteria.go`](../../internal/lidar/perframeeval/following_criteria.go).
