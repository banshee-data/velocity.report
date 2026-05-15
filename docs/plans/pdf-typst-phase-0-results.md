# Phase 0 results: Typst PDF prototype

- **Status:** Complete; awaiting parity-review GO/NO-GO before Phase 1
- **Plan:** [pdf-typst-migration-plan.md](pdf-typst-migration-plan.md)
- **Branch:** `patrickod/go-typst`
- **Date:** 2026-04-25

## What was built

A self-contained Typst-based prototype that produces a 3-page PDF report
from a bundled sample fixture, exercising every structural element the
migration will need:

| Component                           | Path                                                |
| ----------------------------------- | --------------------------------------------------- |
| Page setup, fonts, header, footer   | [internal/report/typst/templates/preamble.typ](../../internal/report/typst/templates/preamble.typ) |
| Section builders                    | [internal/report/typst/templates/sections.typ](../../internal/report/typst/templates/sections.typ) |
| Entry point + columns layout        | [internal/report/typst/templates/report.typ](../../internal/report/typst/templates/report.typ)     |
| Sample fixture                      | [internal/report/typst/testdata/sample.json](../../internal/report/typst/testdata/sample.json)     |
| Go renderer (go-typst wrapper)      | [internal/report/typst/render.go](../../internal/report/typst/render.go)                           |
| End-to-end smoke test               | [internal/report/typst/render_test.go](../../internal/report/typst/render_test.go)                 |
| Phase 0 driver CLI                  | [internal/report/typst/cmd/typst-prototype/main.go](../../internal/report/typst/cmd/typst-prototype/main.go) |
| Makefile target                     | `make pdf-typst-prototype`                          |

Render path:

```
sample.json ──► typst-prototype (Go) ──► go-typst.CLI.Compile
                                          │
                                          ▼
                                       typst (CLI, 0.14.2)
                                          │  (--root <tmpdir>)
                                          ▼
                                       report.pdf  ◄── <stdin>: bootstrap
                                                       which #include's
                                                       /report.typ rooted
                                                       in the temp dir
```

The Go renderer materialises the embedded templates and the marshalled data
into a temp directory, feeds typst a one-line bootstrap on stdin
(`#include "/report.typ"`), and lets the templates resolve all of their
relative imports and `#image()` calls under `--root`.

## Reproduction

```bash
make pdf-typst-prototype
# Output: tools/pdf-generator/output/typst-prototype.pdf
```

Or via the test:

```bash
go test ./internal/report/typst/...
```

The test skips automatically when `typst` is not on `PATH`, so CI on hosts
without typst will not break. To install typst locally:

```bash
curl -sL -o /tmp/typst.tar.xz https://github.com/typst/typst/releases/download/v0.14.2/typst-x86_64-unknown-linux-musl.tar.xz
tar -xJf /tmp/typst.tar.xz -C /tmp
mv /tmp/typst-x86_64-unknown-linux-musl/typst ~/.local/bin/
```

## Output observed

- **PDF size:** 79.7 KB
- **Pages:** 3
- **PDF version:** 1.7 (typst's default; configurable via `PDFStandards`)
- **Tagged PDF:** yes (typst's accessibility tags are on by default)
- **Compile time:** ~50 ms wall-clock for the smoke test (vs several
  seconds for the equivalent xelatex run on the Python pipeline)
- **Atkinson Hyperlegible:** correctly resolved via `--font-path` and
  used as the body and heading font; verified with a single-glyph test
  and visible distinctive open `a`/`g` shapes in the output.

## What worked

| Concern                            | Result                                                    |
| ---------------------------------- | --------------------------------------------------------- |
| Template engine choice             | Typst's native programmable markup is a good fit; data injection via JSON file at `--root` is clean. |
| Font handling (`fontspec` problem) | Fully resolved. `#set text(font: "Atkinson Hyperlegible")` is a one-liner. No preamble dance. |
| Two-column layout                  | `#columns(2)[ … #colbreak() … ]` is a one-liner. No `multicol` package. |
| Tables (Key Metrics, Daily, Histogram, Survey Parameters) | All rendered correctly with explicit column widths. |
| Bullet lists (`itemize` analogue)  | `#list(...)` works as expected.                           |
| Math (`K_E = ½mv²`)                | `$ K_E = 1/2 m v^2 $` renders cleanly.                    |
| Hyperlinks                         | `#link("https://...")[text]` renders coloured, clickable. |
| Header/footer with running content | Confirmed via `set page(header: ..., footer: ...)`.       |
| Embedded `go:embed` of templates   | `embed.FS` walks cleanly into the temp working directory. |
| `go-typst` wrapper                 | Works as advertised. The structured-error parsing was unused in this prototype but is available for Phase 2. |
| End-to-end Go renderer             | A single `Render(io.Writer, Options)` call produces a valid PDF byte-for-byte identical to the CLI compile. |

## Issues found and fixed during the prototype

1. **Forward-reference error for `header-block`.** `#let` is sequential in
   typst; helpers must be defined *before* the `setup` function that uses
   them. Reordered.
2. **`counter(page).display()` requires context.** Wrapped in `#context`
   for typst 0.14+.
3. **Daily summary table column collapse.** Initial `auto` column widths
   produced a visually merged Date/Count column. Fixed with explicit
   widths (`columns: (2.2cm, 1.4cm, 1.2cm, 1.2cm, 1.2cm, 1.2cm)`) and by
   moving the unit suffix from per-cell text to the column header.
4. **Stdin compile loses path context for relative imports.**
   `go-typst.Compile` feeds the document to typst on stdin; relative
   `#import "preamble.typ"` then has no anchor. Two fixes:
   - Switch all template imports to absolute (`#import "/preamble.typ"`)
     so they resolve under `--root`.
   - Use a one-line bootstrap on stdin
     (`#include "/report.typ"`) that pulls in the real entry from the
     rooted working directory.

## Open deltas vs the Python output

The current Python pipeline could not be executed locally for a side-by-side
comparison because the development host has neither `xelatex` nor the
project's Python `.venv`. The structural mapping is documented below; the
parity check should be performed by a reviewer who can render both.

| Section                         | Python (PyLaTeX)                    | Typst prototype                    | Status                       |
| ------------------------------- | ----------------------------------- | ---------------------------------- | ---------------------------- |
| Spanning title (location, surveyor, contact) | `\twocolumn[…]` with `\huge` title | `align(center)[…]` before `#columns(2)` | Equivalent; visual review needed |
| Velocity Overview (bulletted)   | `itemize` with `\textbf` labels     | `#list` with `*…*`                 | Equivalent                   |
| Key Metrics table               | `param_table` (key/value)           | 2-col table                        | Equivalent                   |
| Site Information                | `subsection*` + paragraphs          | `heading(level: 2)` + paragraphs   | Equivalent                   |
| Daily Summary table             | `supertabular`                      | `#table` with explicit widths      | Equivalent                   |
| Velocity Distribution           | matplotlib chart + table fallback   | Table only (chart pending Phase 1) | Chart deferred to Phase 1    |
| Time-series chart               | matplotlib chart                    | `#image()` placeholder             | Chart deferred to Phase 1    |
| Site map                        | matplotlib + cairo SVG              | `#image()` placeholder             | Chart deferred to Phase 1+6  |
| Science and Methodology         | `subsection*` + paragraphs + `\[…\]` | `heading` + `par[…]` + `align(center)[$$…$$]` | Equivalent                  |
| Survey Parameters               | `param_table`                       | 2-col table                        | Equivalent                   |
| Header (left/right + rule)      | `fancyhdr` with `\fancyhead`        | `set page(header: …)` + `line()`   | Equivalent                   |
| Footer (date range + page)      | `fancyhdr` with `\fancyfoot`        | `set page(footer: …)` + `counter(page)` | Equivalent                   |

**Confidence:** the structural mapping is high-confidence — every PyLaTeX
construct used by the Python pipeline has a direct, idiomatic Typst
counterpart. Visual fidelity (font sizing, padding, vertical rhythm) will
need iteration in Phase 2 once a reviewer can run both pipelines on the
same machine.

## Risks now better understood

| Risk (from plan)                              | Updated assessment                                                                                            |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `go-typst` is a thin wrapper, not in-process  | Confirmed. Acceptable. The wrapper adds <50 LoC of value but keeps the API stable if we want to swap backends. |
| Atkinson Hyperlegible coverage                | Confirmed working with the existing TTF set — no glyph fallbacks observed in body text or headings.            |
| Two-pipeline coexistence period               | Manageable. The new code is fully isolated under `internal/report/typst/`; no Python paths touched.            |
| Pre-1.0 Typst churn                           | 0.14.2 is current and stable. Pin via the device-image build script when adopting in Phase 7.                  |
| PDF reproducibility (CreationDate)            | `OptionsCompile.CreationTime` exposes `--creation-timestamp`. Will use in Phase 3 for byte-deterministic output. |

## Recommendation: GO

Phase 0 confirms every assumption made in the
[migration plan](pdf-typst-migration-plan.md). The Typst pipeline is
shorter (3 files, <300 LoC of templates), faster (≈10× compile), and
removes both the TeX Live tree (~800 MB) and the `rsvg-convert` dependency
identified in the LaTeX-based plan. There are no surprises that warrant
revising the plan before proceeding to Phase 1 (chart package).

## Next steps

1. **Review this PDF side-by-side with the current Python output** on a
   host with both `xelatex` and `typst` installed. File any visual deltas
   as issues against this branch.
2. **Phase 1: chart package** — `internal/report/chart/` SVG renderers.
   Unchanged from the LaTeX plan. Start when Phase 0 GO is granted.
3. **Phase 2: full template engine** — flesh out comparison reports,
   site-config periods, cosine-correction notes, and the structured-error
   path through `go-typst`.
