# PDF generation via Typst (go-typst)

- **Implementation status (branch `patrickod/go-typst`):** Typst is now the only production report engine. `report.GeneratePDF` renders both single-period and comparison reports via Typst (`report.GenerateTypst`), the HTTP API (`/api/generate_report`) and the `velocity report pdf` CLI both call it, and the typst binary is embedded into the velocity binary via the `internal/report/typst/typstbin` package (built with `-tags typst_embed`; the Pi image build downloads + embeds the linux/arm64 binary). The legacy engine path, templates, image packages, and runtime selectors have been removed.

- **Status:** Complete on this branch; Typst-only
- **Layers:** Cross-cutting (reporting infrastructure, deployment image)
- **Related:**
- **Canonical:** [pdf-reporting.md](../platform/operations/pdf-reporting.md)

- [PDF generation migration to Go](pdf-go-chart-migration-plan.md) (the active plan; this proposal supersedes the LaTeX-based phases)
- [Pure-Go LaTeX backend (NO-GO)](pdf-pure-go-latex-plan.md) — context for why the LaTeX engine choice matters
- [Precompiled LaTeX plan](pdf-latex-precompiled-format-plan.md) (D-08) — becomes obsolete if Typst is adopted
- [Distribution packaging plan](deploy-distribution-packaging-plan.md) (D-09)

**Goal:** Replace XeLaTeX as the typesetting engine in the Go report pipeline
with [Typst](https://typst.app/) via the
[`Dadido3/go-typst`](https://github.com/Dadido3/go-typst) wrapper. Keep the
charting and orchestration goals from the existing
[Go-chart migration plan](pdf-go-chart-migration-plan.md), but emit `.typ`
markup instead of `.tex` and invoke `typst` instead of `xelatex`.

The primary motivations are footprint and ergonomics: the Typst CLI is a
single ~30 MB static binary with built-in OpenType font handling, vs the
~800 MB TeX Live tree that motivated D-08.

---

## TL;DR

| Question                                          | Answer                                                                                                               |
| ------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| Does `go-typst` embed a Go-native Typst compiler? | **No.** It shells out to the `typst` CLI (or a Docker container) over stdio.                                         |
| Can it replace `xelatex` for these reports?       | **Yes** — Typst supports custom fonts (Atkinson Hyperlegible), images (PNG/SVG/PDF), tables, and two-column layouts. |
| Does it eliminate the TeX Live dependency?        | **Yes.** Replaces TeX Live + XeLaTeX with a single ~30 MB `typst` binary.                                            |
| Does it eliminate `rsvg-convert`?                 | **Yes** for charts: Typst has native SVG support, no rasterisation step needed.                                      |
| Is it production-stable?                          | Typst itself is at 0.14.x, used in production by many teams; `go-typst` README warns the API "may change".           |
| Recommended path                                  | Build a vertical-slice prototype on a feature branch, validate output parity, then commit to a phased migration.     |

---

## Why Typst instead of LaTeX

The active [Go-chart migration plan](pdf-go-chart-migration-plan.md) keeps
XeLaTeX as the typesetter. Two facts make Typst worth re-evaluating that choice:

1. **Footprint.** XeLaTeX requires a TeX Live distribution (~800 MB, the
   subject of plan D-08). Typst is a single ~30 MB statically linked binary
   with no kpathsea-style file search, no separate font cache, and no
   per-package install cycle.
2. **Font handling.** The custom Atkinson Hyperlegible setup currently
   demands `fontspec` (XeTeX-only) — the exact reason the
   [Pure-Go LaTeX feasibility study](pdf-pure-go-latex-plan.md) ended in
   NO-GO. Typst handles OpenType fonts as a first-class engine primitive
   (`#set text(font: "Atkinson Hyperlegible")`), with no preamble dance.

Secondary wins:

- Native SVG image handling (`#image("chart.svg")`) — eliminates the
  `rsvg-convert` SVG→PDF step proposed in the LaTeX plan.
- Modern error messages (line/column/source span) parsed by `go-typst`.
- Compilation is typically 5–20× faster than XeLaTeX on the same document.
- `text/template`-style data injection is unnecessary: Typst is a
  programmable markup language; data can be passed as native values via
  `typst.InjectValues`.

Costs to set against this:

- **Typst is not LaTeX.** Templates must be rewritten — Typst syntax is its
  own thing (closer to Markdown + Lua than to TeX). The
  [existing `.tex` templates referenced in the Go-chart plan](pdf-go-chart-migration-plan.md#latex-template-design)
  do not yet exist in this repo; we are choosing the template language now,
  not migrating away from one.
- **Typst is pre-1.0** (0.14.2 at time of writing). We accept some churn in
  exchange for the footprint and ergonomic wins.
- **`go-typst` shells out.** It is not an in-process Go reimplementation.
  We trade TeX Live's bulk for a 30 MB Rust binary that must be on `PATH`
  (or shipped alongside `velocity-report` on the device image).

---

## What `go-typst` actually is

`go-typst` is a thin Go wrapper around the Typst CLI. The README confirms:

> go-typst embeds no in-process Typst compiler. Instead, it communicates
> with external Typst binaries via stdio, meaning "no temporary files will
> be created."

The library exposes three "callers" implementing a `typst.Caller` interface:

| Caller       | Use case                                                             |
| ------------ | -------------------------------------------------------------------- |
| `CLI`        | Invoke a `typst` binary already on the system (or at a custom path). |
| `Docker`     | Pull and run the official Typst Docker image automatically.          |
| `DockerExec` | Run `typst` inside a long-lived named container.                     |

Tested Typst versions: 0.12.0–0.14.2. The library has zero non-stdlib Go
dependencies.

### API surface we'd actually use

```go
caller := typst.CLI{}                // or typst.CLI{ExecutablePath: "/opt/velocity/bin/typst"}
markup := bytes.NewBufferString(typstSource)
out, err := os.Create("report.pdf")
if err != nil { … }
defer out.Close()

err = caller.Compile(markup, out, &typst.OptionsCompile{
    Format:  typst.OutputFormatPDF,
    Inputs:  injectedValues,           // map[string]any → Typst sys.inputs
    FontPaths: []string{"/opt/velocity/fonts"},
    Root:    "/tmp/report-xxxxxx",     // for #image() resolution
})
```

That is essentially the entire API used by the proposed pipeline.

---

## Proposed architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Web UI  →  POST /api/generate_report                       │
│  (or CLI: velocity-report pdf --config report.json)         │
└───────────┬─────────────────────────────────────────────────┘
            │
            ▼
┌─────────────────────────────────────────────────────────────┐
│  Go: internal/report/                                       │
│  ├── report.go          ← orchestrator (Generate entry pt)  │
│  ├── config.go          ← report configuration structs      │
│  ├── chart/                                                 │
│  │   ├── timeseries.go  ← dual-axis percentile + count SVG  │
│  │   ├── histogram.go   ← velocity distribution SVG         │
│  │   ├── palette.go     ← colour constants (from DESIGN.md) │
│  │   └── svg.go         ← SVG rendering helpers             │
│  ├── typst/                                                 │
│  │   ├── render.go      ← go-typst Compile wrapper          │
│  │   ├── inject.go      ← marshal Go data → typst inputs    │
│  │   ├── templates/     ← go:embed Typst sources            │
│  │   │   ├── report.typ        (entry)                      │
│  │   │   ├── preamble.typ      (page setup, fonts, colors)  │
│  │   │   ├── overview.typ                                    │
│  │   │   ├── site_info.typ                                   │
│  │   │   ├── chart_section.typ                               │
│  │   │   ├── statistics.typ                                  │
│  │   │   └── science.typ                                     │
│  │   └── helpers_test.go                                    │
│  └── archive.go         ← .zip packaging (.typ + .svg + .pdf) │
└───────────┬─────────────────────────────────────────────────┘
            │  Go writes .typ + .svg charts to a temp dir
            ▼
┌─────────────────────────────────────────────────────────────┐
│  typst (CLI, ~30MB)  →  compiles report.typ → report.pdf    │
│  - Native SVG handling, no rsvg-convert                     │
│  - Atkinson Hyperlegible loaded via --font-path             │
└─────────────────────────────────────────────────────────────┘
```

The chart and archive packages are **identical** to the LaTeX plan — only
the templating layer and external compiler change.

---

## Template design

Typst templates use the Typst language directly. Unlike `text/template` over
LaTeX, there is no impedance mismatch between the data layer and the markup
layer: the document **is** a program that consumes structured input.

### Data injection

Two options, both supported by `go-typst`:

1. **`InjectValues` (preferred for structured data):** Marshal the
   `ReportData` struct as a Typst dictionary literal prepended to the
   document source. Inside the template, refer to fields directly:

   ```typst
   = #data.location
   Surveyed by #data.surveyor
   Period: #data.start_date — #data.end_date
   ```

2. **`OptionsCompile.Inputs` (for small key/value config):** Passed as
   `--input key=value` flags; available in the template via
   `sys.inputs.key`. Useful for run-id, debug flags, etc.

Charts, the map SVG, and any per-run dynamic assets are written to disk in
the run's temp directory and referenced via `#image("timeseries.svg")`.

### Template skeleton (illustrative)

```typst
// preamble.typ
#let setup(data) = {
  set page(
    paper: "a4",
    margin: (top: 1.8cm, bottom: 1.0cm, x: 1.0cm),
    header: header(data),
  )
  set text(font: "Atkinson Hyperlegible", size: 10pt)
  set par(justify: true)
}

#let palette = (
  p50: rgb("#fbd92f"),
  p85: rgb("#f7b32b"),
  p98: rgb("#f25f5c"),
  max: rgb("#2d1e2f"),
)

// report.typ (entry point)
#import "preamble.typ": setup, palette
#import "overview.typ": overview
#import "site_info.typ": site_info
#import "chart_section.typ": chart_section
#import "statistics.typ": statistics
#import "science.typ": science

#let data = json("data.json")     // or sys.inputs / InjectValues
#setup(data)

#columns(2)[
  #overview(data, palette: palette)
  #site_info(data)
]

#pagebreak()
#chart_section(data)
#statistics(data)
#science(data)
```

Each section is a Typst function taking `data` and returning content. This
maps cleanly to the section list already enumerated in the
[LaTeX plan's template data structure](pdf-go-chart-migration-plan.md#template-data-structure).

### Tables

Typst's built-in `table()` and `grid()` cover all current PyLaTeX usage
(`twocolumn_stats_table`, `histogram_table`, daily/hourly summaries). No
external package required.

### Two-column layout

`#columns(2)[ … ]` replaces the LaTeX `\twocolumn` toggle and the
`multicol` package. Fully native, no configuration.

---

## SVG and image handling

**Typst supports SVG natively** — `#image("chart.svg")` is a one-liner. This
deletes an entire workstream from the LaTeX plan:

- No `rsvg-convert` dependency.
- No SVG → PDF conversion step in the Go orchestrator.
- The `.zip` source archive can include the original SVGs unchanged, and
  they are also what the typesetter consumes.

The OpenStreetMap overlay SVG (currently passed through `cairosvg`/
`inkscape`/`rsvg-convert` per `map_utils.py`) goes through the same
`#image()` path with no conversion.

---

## Font handling

Typst loads fonts from:

1. The system font directory (skipped by default in our case).
2. Any directory passed via `--font-path` (or `OptionsCompile.FontPaths`).
3. Fonts embedded in the binary (Typst ships a small default set; we won't
   rely on those).

We will:

1. Keep the existing `pdf_generator/core/fonts/` Atkinson Hyperlegible TTF
   files.
2. Move them under `internal/report/fonts/` and embed via `go:embed` for
   programmatic extraction at runtime, **or** ship the directory alongside
   `velocity-report` and pass it via `FontPaths`.
3. Reference fonts in templates by family name only:
   `#set text(font: "Atkinson Hyperlegible")`.

The `fontspec`-shaped problem from the
[Pure-Go-LaTeX feasibility study](pdf-pure-go-latex-plan.md) does not exist
in Typst; OpenType is the engine's native font model.

---

## Deployment / packaging implications

### Single-binary goal (D-09)

`go-typst` is **not** a single-binary solution: a `typst` executable must be
present at runtime. Two options:

1. **Bundle `typst` next to `velocity-report`** in the device image
   (~30 MB). The Makefile downloads the appropriate ARM64 release during
   image build.
2. **Use `typst.Docker` caller** on hosts that have Docker. Not viable on
   the Raspberry Pi target — D-09 explicitly avoids Docker on device.

Recommendation: option 1. We trade the "single static binary" purity for a
~30 MB co-installed dependency, which is a 25× reduction over the TeX Live
tree D-08 was trying to slim down.

### Removal of D-08

D-08 (precompiled LaTeX format) is motivated entirely by TeX Live's size.
If Typst is adopted, D-08 becomes obsolete and can be archived. This is a
genuine simplification of the deployment plan.

### Web frontend / CLI surface

No public-API changes. `POST /api/generate_report` and the
`velocity-report pdf` subcommand keep their JSON contract; only the
internal pipeline changes.

---

## Risk register

| Risk                                          | Likelihood | Impact | Mitigation                                                                                                  |
| --------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------------------------------- |
| Typst pre-1.0 breaking changes                | Medium     | Medium | Pin to a specific Typst version in CI and on device. `go-typst`'s tested-versions list gives clear bounds.  |
| `go-typst` API churn                          | Low        | Low    | Vendor a thin internal wrapper so we can swap backends; the API surface we use is small.                    |
| Typst output is not pixel-identical to LaTeX  | High       | Low    | Goal is _equivalent_ not identical. Visual review against a frozen reference set in CI.                     |
| Atkinson Hyperlegible coverage of glyphs      | Low        | Low    | Same font set used today; Typst loads it directly. Validate during Phase 1 prototype.                       |
| Map SVG renders differently in Typst          | Medium     | Low    | SVG spec coverage is broad in Typst 0.13+; validate as part of map migration phase.                         |
| Typst binary not available on developer macOS | Low        | Low    | `make install-typst` target; documented in CONTRIBUTING.md.                                                 |
| Two-pipeline coexistence period               | Medium     | Medium | Keep Python pipeline behind a feature flag during rollout (one release cycle), as the LaTeX plan specifies. |

---

## Implementation phases

Phase numbering and scope mirror the [Go-chart migration plan](pdf-go-chart-migration-plan.md);
charts and orchestration are unchanged. **Only Phases 2 and 3 differ** from
the LaTeX plan.

### Phase 0: prototype and parity check; `S` (1–2 days)

1. Scaffold `internal/report/typst/` with a minimal `report.typ`.
2. Hand-author a Typst report from a frozen sample dataset.
3. Render side-by-side against the current Python output.
4. Confirm: fonts, colours, two-column layout, tables, SVG inclusion,
   page-break behaviour.
5. Document deltas; decide GO/NO-GO before further work.

**Acceptance:** A Typst-rendered PDF for one frozen sample passes visual
review against the corresponding Python-rendered PDF. No regressions in
text legibility, chart placement, or table formatting.

### Phase 1: Chart package; `M` — **identical to LaTeX plan**

Build SVG charts in Go (`internal/report/chart/`). Unchanged from the
[Go-chart migration plan, Phase 1](pdf-go-chart-migration-plan.md#phase-1-chart-package-foundation-internalreportchart-m).
The chart package emits SVG; downstream consumers (Typst or LaTeX) decide
how to embed.

### Phase 2: Typst template engine (`internal/report/typst/`); `S`

1. Author Typst templates for each report section (one `.typ` per section).
2. Implement `Render(data ReportData) (markup []byte, err error)` that
   marshals data via `typst.InjectValues` and prepends it to the entry
   template.
3. Implement compilation wrapper: `Compile(markup []byte, charts []Asset, fontDir string) ([]byte, error)`.
4. Snapshot tests on rendered Typst markup (deterministic output).
5. Integration test: render → compile → assert PDF non-empty, page count,
   embedded text searchable.

**Acceptance:** `Render` + `Compile` produce a valid PDF for a fixture
dataset; snapshot tests are stable.

### Phase 3: Report orchestrator; `S`

Wire charts + Typst templates + compilation together. Replaces
[Phase 3 of the LaTeX plan](pdf-go-chart-migration-plan.md#phase-3-report-orchestrator-internalreport-s).
Differences from that plan:

- No `rsvg-convert` step; SVGs handed to Typst directly.
- Compilation step calls `go-typst` instead of `os/exec` of `xelatex`.
- Temp-directory layout: a single dir containing `report.typ`,
  `data.json`, all SVGs, and the font directory.

**Acceptance:** End-to-end report generation from test database produces
valid PDF, byte-deterministic given fixed inputs.

### Phase 4: API and CLI integration; `S`

API and CLI integration now call Typst directly. There is no report-engine
feature flag or coexistence mode on this branch.

### Phase 5: Python deprecation and cleanup; `S`

Identical to [Phase 5 of the LaTeX plan](pdf-go-chart-migration-plan.md#phase-5-python-deprecation-and-cleanup-s).

### Phase 6: Map overlay migration; `S`

Same as the LaTeX plan, but the SVG → PDF conversion is removed entirely;
the SVG is included via `#image()`.

### Phase 7: Image build / packaging

1. Add `make install-typst` (developer machines).
2. Add Typst download step to the Pi image build.
3. Remove TeX Live install step from the image (the eventual D-08 win).
4. Archive D-08 as obsolete; update [pdf-reporting.md](../platform/operations/pdf-reporting.md).

---

## Open questions

1. **License compatibility:** Typst is Apache-2.0; `go-typst` is MIT. Both
   compatible with this repo's license. Confirm during Phase 0.
2. **Reproducibility:** Typst sets PDF metadata (CreationDate). For
   byte-deterministic output, set `--creation-timestamp` to a fixed value
   per run. Verify this knob exists in the Typst version we pin.
3. **Embedding fonts vs file path:** Performance test both — embedding
   fonts in the Go binary increases binary size by ~2 MB but eliminates an
   on-disk font directory.
4. **`go-typst` vs direct `os/exec`:** `go-typst` adds a thin abstraction.
   Worth keeping for the structured-error parsing alone, but reconsider if
   it pulls in unwanted complexity.

---

## Decision

**Recommended:** approve Phase 0 (prototype) on a feature branch. Reassess
the full migration after a side-by-side parity report against the current
Python output.
