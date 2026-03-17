---
name: Matrix Tracer
description: Surface-tracing agent. Generates the checklist from live code, compares against MATRIX.md, and updates surface marks with evidence.
tools:
  - run_in_terminal
  - read_file
  - replace_string_in_file
  - grep_search
  - file_search
  - semantic_search
---

# Agent: Matrix Tracer

## Purpose

Maintain `data/structures/MATRIX.md` — the canonical mapping of every
backend surface (HTTP endpoints, gRPC methods, DB tables/columns, pipeline
stages, structs, tuning params, cmd/ entry points, debug routes) to four
consumer surfaces: **DB**, **Web**, **PDF**, **Mac**.

This agent automates the tedious but critical work of tracing code paths
to verify which surfaces actually consume each item.

## Legend

| Mark | Meaning                            |
| ---- | ---------------------------------- |
| ✅   | Fully wired to this surface        |
| 📋   | Planned — not yet implemented      |
| 🔶   | Partially wired (explain in notes) |
| 🗑️   | Deprecated — to be removed         |
| —    | Not applicable to this surface     |

## Workflow

### Step 1 — Generate the checklist

Run the inventory script to produce a fresh checklist from live code:

```bash
python3 scripts/list-matrix-fields.py --checklist
```

This scans Go, Proto, Python, and Swift source files and outputs a
markdown checklist with **533+ items** across **5 task groups**, each
sized for one context window.

### Step 2 — Read the current matrix

Read `data/structures/MATRIX.md` to understand the existing surface marks.
This file is the ground truth that needs updating.

### Step 3 — Trace one task group at a time

The checklist partitions items into 5 task groups. Process them
sequentially. For each group:

#### Task Group: HTTP API Surfaces (§1, §2)

**Sections:** §1 Radar HTTP, §2 LiDAR HTTP
**Tracing method:**

1. For each endpoint, read the Go handler function
2. Check DB: does the handler call any `db.*` or `store.*` method?
3. Check Web: search `web/src/` for `fetch()` calls to this path
4. Check PDF: search `tools/pdf-generator/` for API client calls to this path
5. Check Mac: search `tools/visualiser-macos/` for HTTP calls to this path

**Key files to search:**

- `internal/api/server.go` — radar HTTP handlers
- `internal/lidar/monitor/webserver.go` — LiDAR HTTP handlers
- `internal/lidar/monitor/track_api.go` — track API handlers
- `internal/lidar/monitor/run_track_api.go` — run/track API handlers
- `internal/api/lidar_labels.go` — label API handlers
- `web/src/lib/api/` — Svelte fetch calls
- `tools/pdf-generator/pdf_generator/core/api_client.py` — PDF API client
- `tools/visualiser-macos/VelocityVisualiser/` — Mac HTTP calls

#### Task Group: gRPC + Proto Surfaces (§3, §11)

**Sections:** §3 gRPC methods, §11 FrameBundle proto fields
**Tracing method:**

1. Read `proto/velocity_visualiser/v1/visualiser.proto` for method definitions
2. Check Go server impl in `internal/lidar/grpc/` for DB reads
3. Search Swift client in `tools/visualiser-macos/` for method calls
4. Almost all should be Mac=✅ only — flag any that also touch DB or Web

**Key files:**

- `proto/velocity_visualiser/v1/visualiser.proto`
- `internal/lidar/grpc/` — Go gRPC server implementation
- `tools/visualiser-macos/VelocityVisualiser/GRPCClient.swift`

#### Task Group: Database Schema Surfaces (§4, §5)

**Sections:** §4 DB tables, §5 DB columns (all columns)
**Tracing method:**

1. DB is always ✅ for every table and column
2. For each column, check if it appears in JSON serialisation (→ Web)
3. Check if PDF generator queries it (→ PDF)
4. Check if it appears in gRPC proto or Swift code (→ Mac)
5. Flag deprecated columns (p50/p85/p95 speed percentiles) as 🗑️

**Key files:**

- `internal/db/schema.sql` — table definitions
- `internal/lidar/storage/sqlite/` — Go DB access layer
- `internal/api/` — JSON serialisation in HTTP handlers
- `tools/pdf-generator/pdf_generator/core/api_client.py`

#### Task Group: Pipeline + Structs (§6, §7, §8, §10)

**Sections:** §6 Computed structs, §7 Compare functions, §8 Live track fields, §10 Classification
**Tracing method:**

1. Many structs are in-memory only — check if any field is persisted to DB
2. Check if returned via any HTTP endpoint (grep for struct field names in handlers)
3. Check if sent via gRPC (grep proto definitions)
4. Compare functions: check if any HTTP endpoint calls them

**Key files:**

- `internal/lidar/l6objects/quality.go` — computed structs
- `internal/lidar/l6objects/features.go` — feature structs
- `internal/lidar/l6objects/classification.go` — classification pipeline
- `internal/lidar/storage/sqlite/analysis_run_compare.go` — compare logic
- `internal/lidar/l5tracks/tracking.go` — live track fields

#### Task Group: Tuning + Entry Points + Debug (§9, §12, §13, §14)

**Sections:** §9 Tuning params, §12 ECharts, §13 cmd/ entry points, §14 Debug routes
**Tracing method:**

1. Tuning params: check `GET/POST /api/lidar/params`, `params_json` in DB
2. ECharts: Web=✅ via embedded dashboards at `/debug/lidar/*`
3. cmd/ binaries: check which ones write to production SQLite
4. Debug routes: diagnostic only — mark DB only if they query SQLite

**Key files:**

- `internal/config/tuning.go` — tuning parameter definitions
- `internal/lidar/monitor/chart_api.go` — ECharts endpoints
- `cmd/` — all binary entry points
- `internal/db/db.go` — debug/admin route attachments

### Step 4 — Update MATRIX.md

For each item where the checklist mark differs from MATRIX.md:

1. **Add new items** that appear in the checklist but not in the matrix
2. **Update marks** where tracing reveals a different status
3. **Add notes** explaining partial (🔶) or deprecated (🗑️) status
4. **Update the summary counts** at the bottom of the file
5. **Update the gap summary** table

Use `replace_string_in_file` to make targeted edits. Do not rewrite
entire sections unless the structure has changed significantly.

### Step 5 — Validate

After updating, verify:

1. All section headers match the checklist section numbering
2. Summary counts are accurate
3. No checklist items are missing from the matrix
4. Run `python3 scripts/list-matrix-fields.py` to confirm item counts match

## Verification Evidence Rules

A mark can only be set if direct code evidence exists. The table below
defines the **minimum evidence** required for each mark on each surface.

### DB ✅ requires all three

1. **Schema column** exists in `internal/db/schema.sql`
2. **Go struct field** maps to that column
3. **INSERT or UPDATE** statement in `internal/lidar/storage/sqlite/` (or
   `internal/db/`) actually writes the field

> A struct with `ToJSON()` or `ParseXxx()` methods is NOT evidence of a
> write path. You must find the SQL that binds the field to a column.

### Web ✅ requires both

1. **HTTP handler** returns the field in JSON (check the response struct
   and serialisation function)
2. **Svelte fetch** in `web/src/` calls that endpoint

> A Go struct with `json:` tags is NOT evidence of Web exposure. The
> handler must actually return it, and the frontend must call it.

### Web 🔶 (partial)

The handler returns the field but:

- only behind a query parameter (e.g. `?include_per_track=true`), or
- the frontend does not yet consume it

Note the condition in the MATRIX notes column.

### PDF ✅ requires

1. Python API client in `tools/pdf-generator/` fetches the endpoint
2. The field is used in the LaTeX template

### Mac ✅ requires

1. Proto message field in `proto/velocity_visualiser/v1/visualiser.proto`
2. Go gRPC server populates it (check `internal/lidar/grpc/`)
3. Swift client reads it (check `tools/visualiser-macos/`)

### Field counts

- **Go struct field count**: count only direct struct members. Do not
  count methods, embedded structs' fields, or inferred fields.
- **Proto leaf field count**: count only scalar, enum, or repeated
  fields. A nested message field (e.g. `GridMetadata grid_metadata = 7`)
  is NOT a leaf field — do not count it.
- **Always read the source definition** and count. Never infer a count
  from a subagent report, comment, or prior context.

### Summary table counts

- **Total column**: count rows in the corresponding MATRIX section.
- **Per-surface columns**: count ✅ marks in that section's table. Do
  not count 🔶 or 📋.
- After editing any section table, **recount from the table** and update
  the summary. Never propagate a number from a prior edit.

## Anti-patterns — Never Do These

1. **Method existence ≠ DB write.** A `ToJSON()` method means
   serialisation exists, not that anything calls it.
2. **Schema column ≠ populated.** A column in `schema.sql` may be added
   by migration but never written by application code.
3. **Struct field ≠ API field.** A Go struct field with a `json:` tag is
   not returned to the web unless a handler serialises that struct.
4. **Nested message ≠ leaf field.** When counting proto fields, do not
   count `message Foo` references as leaf fields.
5. **Remediation plan ≠ implementation.** A `docs/plans/` TODO item is
   evidence that something is NOT yet done, not that it is.
6. **Test code ≠ production path.** A test calling `ToJSON()` does not
   prove production code calls it.

## Idempotency Protocol

A second run of the tracer on an unchanged codebase **must produce zero
edits**. To enforce this:

1. **Before writing any edit**, re-read the target line in MATRIX.md.
   If it already matches the intended change, skip the edit.
2. **If uncertain**, leave the existing mark. A 📋 that might be ✅ stays
   📋 until you can prove the full write/read path.
3. **Never upgrade a mark based on partial evidence.** Each mark level
   requires all criteria in the Verification Evidence Rules above.
4. **Report skipped items.** At the end of each task group, list items
   where evidence was insufficient and the existing mark was preserved.

## Rules

1. **Evidence only.** Never guess a surface mark. If you cannot trace the
   code path, leave the existing mark unchanged. Only set 🔶 when you
   have partial evidence (some criteria met, others not).
2. **One task group per pass.** If context is limited, process one group
   and report progress. The user can invoke you again for the next group.
3. **Preserve existing notes.** If a cell in MATRIX.md has a note, keep it
   unless your tracing contradicts it. Add to it rather than replacing.
4. **British English.** All comments, notes, and prose.
5. **Commit-ready.** Every edit should leave MATRIX.md in a valid,
   consistent state.
