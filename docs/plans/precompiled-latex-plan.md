# Precompiled LaTeX Format Plan

**Status**: Draft — awaiting review before implementation
**Date**: 13 February 2026
**Parent**: [RPi Imager Fork Design § 4.6 Option B](rpi-imager-fork-design.md)
**Goal**: Replace ~800 MB `texlive-xetex` installation with a minimal vendored TeX
tree and precompiled `.fmt` file, reducing the Raspberry Pi image by ~700–750 MB
while preserving byte-identical PDF output.

## Problem Summary

The full `texlive-xetex` + `texlive-fonts-recommended` + `texlive-latex-extra`
installation adds ~800 MB uncompressed to the Pi image. This is the single
largest dependency. The PDF generator only uses a small subset of that
installation:

| Used today                            | Not needed on Pi             |
| ------------------------------------- | ---------------------------- |
| XeTeX engine binary (`xelatex`)       | tlmgr, texdoc, other engines |
| ~10 `.sty` packages (see § 2)         | Thousands of unused packages |
| Atkinson Hyperlegible fonts (bundled) | System-wide TeX fonts        |
| fontspec, hyperref support files      | Full CTAN mirror             |

## Current Architecture

```
pdf_generator/core/
├── pdf_generator.py      ← orchestrates report, calls doc.generate_pdf()
├── document_builder.py   ← builds PyLaTeX Document, loads packages/fonts
├── chart_builder.py      ← matplotlib/seaborn charts → PDF figures
├── chart_saver.py        ← saves matplotlib figures as PDF
├── dependency_checker.py ← validates xelatex + Python packages
└── fonts/                ← 6 bundled Atkinson Hyperlegible TTF files
```

**Compilation flow** (today):

1. Python builds report data, renders charts via matplotlib → embedded PDF figures
2. `DocumentBuilder` constructs a PyLaTeX `Document` with `\usepackage{...}` calls
3. `pdf_generator.py` calls `doc.generate_pdf(compiler="xelatex")` (fallback chain:
   xelatex → lualatex → pdflatex)
4. PyLaTeX writes a `.tex` file and shells out to the compiler
5. XeTeX reads the `.tex`, loads each `.sty` from the TeX tree, and produces PDF

**Python and matplotlib remain required** — charts (histograms, time-series,
speed distributions) are rendered by matplotlib/seaborn as PDF figures that get
`\includegraphics`'d into the LaTeX document.

## LaTeX Packages in Use

Loaded by `DocumentBuilder.add_packages()`:

| Package        | Purpose                                     |
| -------------- | ------------------------------------------- |
| `fancyhdr`     | Page headers and footers                    |
| `graphicx`     | `\includegraphics` for chart PDFs           |
| `amsmath`      | `\tfrac` and math formatting                |
| `titlesec`     | Section heading customisation               |
| `hyperref`     | Clickable URLs in header/footer             |
| `fontspec`     | Atkinson Hyperlegible font loading          |
| `caption`      | Bold, sans-serif captions                   |
| `supertabular` | Tables that break across columns            |
| `float`        | `[H]` float placement                       |
| `array`        | Column-spec modifiers `>{...}`, `<{...}`    |
| `geometry`     | Page margins (loaded implicitly by PyLaTeX) |

Plus implicit dependencies pulled in by these packages (e.g. `l3kernel`,
`l3packages`, `xparse`, `kvoptions`, `etoolbox`, `hyperref`'s backend `.def`
files).

## Design

### Two Modes of Operation

| Mode            | When used                         | TeX source                   |
| --------------- | --------------------------------- | ---------------------------- |
| **Development** | Iterating on report layout/design | Full `texlive-xetex` install |
| **Production**  | Deployed Pi running headless      | Minimal vendored TeX tree    |

The pdf-generator must work identically in both modes. The only difference is
_where_ the TeX engine and support files come from.

### Mode Detection

A single environment variable controls which mode is active:

```
VELOCITY_TEX_ROOT=/opt/velocity-report/texlive-minimal
```

| `VELOCITY_TEX_ROOT`       | Behaviour                                   |
| ------------------------- | ------------------------------------------- |
| **unset / empty**         | Development mode — use system `xelatex`     |
| **set to directory path** | Production mode — use vendored minimal tree |

When `VELOCITY_TEX_ROOT` is set, the pdf-generator:

1. Prepends `$VELOCITY_TEX_ROOT/bin` to `PATH` (custom xelatex binary)
2. Sets `TEXMFHOME=$VELOCITY_TEX_ROOT/texmf`
3. Sets `TEXMFDIST=$VELOCITY_TEX_ROOT/texmf-dist`
4. Optionally uses the precompiled format:
   `xelatex -fmt=velocity-report` (loads packages from `.fmt` instead of `.sty`)

### Minimal TeX Tree Layout

```
/opt/velocity-report/texlive-minimal/
├── bin/
│   └── xelatex                      ← XeTeX engine (statically linked or with deps)
├── texmf-dist/
│   ├── tex/
│   │   ├── latex/
│   │   │   ├── base/                ← LaTeX kernel (.cls, .sty)
│   │   │   ├── fancyhdr/
│   │   │   ├── graphics/            ← graphicx
│   │   │   ├── amsmath/
│   │   │   ├── titlesec/
│   │   │   ├── hyperref/
│   │   │   ├── fontspec/
│   │   │   ├── caption/
│   │   │   ├── supertabular/
│   │   │   ├── float/
│   │   │   ├── tools/               ← array.sty
│   │   │   └── ...                  ← transitive deps (l3kernel, etc.)
│   │   └── generic/
│   │       └── ...                  ← shared support files
│   ├── fonts/
│   │   └── ...                      ← only TeX-internal font metrics if needed
│   └── web2c/
│       ├── texmf.cnf                ← TeX configuration
│       └── xelatex/
│           └── velocity-report.fmt  ← precompiled format (optional, Phase 3)
└── texmf-var/                       ← writable area for ls-R, font caches
    └── ...
```

Estimated size: **30–60 MB** (vs ~800 MB for full texlive-xetex).

### Precompiled Format File (`.fmt`)

A `.fmt` file is a binary dump of the TeX engine's memory after loading macros.
Instead of parsing every `.sty` at compile time, the engine loads the `.fmt` in
a single read — faster startup and a guarantee that no packages are missing.

**Building the format:**

```bash
# From within the minimal tree, with TEXMF variables set:
xelatex -ini \
  -jobname=velocity-report \
  "&xelatex" \
  "\RequirePackage{geometry}" \
  "\RequirePackage{fancyhdr}" \
  "\RequirePackage{graphicx}" \
  "\RequirePackage{amsmath}" \
  "\RequirePackage{titlesec}" \
  "\RequirePackage{hyperref}" \
  "\RequirePackage{fontspec}" \
  "\RequirePackage[font=sf]{caption}" \
  "\RequirePackage{supertabular}" \
  "\RequirePackage{float}" \
  "\RequirePackage{array}" \
  "\dump"
```

This produces `velocity-report.fmt`. At runtime, `xelatex -fmt=velocity-report`
loads this format and the `.tex` file need not contain `\usepackage` lines for
these packages (they are already loaded).

**Important**: Using a precompiled format is an _optimisation_, not a
requirement. The minimal tree works without it — packages are still present as
`.sty` files. The `.fmt` provides:

- Faster compilation (estimated improvement, to be validated in Phase 6)
- Defence-in-depth: missing `.sty` cannot cause runtime failures
- Smaller total footprint (some transitive `.sty` deps can be omitted if baked
  into the format)

## Implementation Phases

### Phase 1: Audit TeX Dependencies

**Goal**: Produce an authoritative list of every file the TeX engine touches
when compiling a velocity.report PDF.

**Steps**:

1. Generate a representative report on a development machine with full TeX Live
2. Capture file access with `strace` (Linux) or `fs_usage` (macOS):
   ```bash
   strace -e openat -f xelatex report.tex 2>&1 | grep texlive
   ```
3. Parse output to extract unique `.sty`, `.cls`, `.def`, `.fd`, `.tfm`, `.cfg`
   files
4. Save the list as `tools/pdf-generator/tex/dependency-manifest.txt`

**Deliverable**: `dependency-manifest.txt` — one file path per line, relative to
the TeX Live root.

### Phase 2: Build Minimal TeX Tree

**Goal**: Assemble a self-contained directory containing only the files from the
manifest.

**Steps**:

1. Create `scripts/build-minimal-texlive.sh`:
   - Reads `dependency-manifest.txt`
   - Copies files from system TeX Live into a staging directory, preserving
     directory structure
   - Copies the `xelatex` binary (and any required shared libraries)
   - Generates `ls-R` index: `mktexlsr $STAGING/texmf-dist`
   - Writes `texmf.cnf` pointing at the minimal tree
2. Add Makefile target: `build-texlive-minimal`
3. Measure and record size of the resulting tree

**Deliverable**: `scripts/build-minimal-texlive.sh` + `build-texlive-minimal`
Makefile target.

### Phase 3: Precompile Format File

**Goal**: Produce `velocity-report.fmt` for faster runtime compilation.

**Steps**:

1. Create `tools/pdf-generator/tex/velocity-report.ini` — the format source:
   ```tex
   % velocity-report.ini — custom XeLaTeX format for velocity.report PDFs
   % Build: xelatex -ini velocity-report.ini
   \input xelatex.ini
   \RequirePackage{geometry}
   \RequirePackage{fancyhdr}
   \RequirePackage{graphicx}
   \RequirePackage{amsmath}
   \RequirePackage{titlesec}
   \RequirePackage{hyperref}
   \RequirePackage{fontspec}
   \RequirePackage[font=sf]{caption}
   \RequirePackage{supertabular}
   \RequirePackage{float}
   \RequirePackage{array}
   \dump
   ```
2. Extend `scripts/build-minimal-texlive.sh` to compile the `.ini` → `.fmt`
3. Place `.fmt` in `texmf-dist/web2c/xelatex/velocity-report.fmt`
4. Validate: `xelatex -fmt=velocity-report test.tex` produces correct output

**Deliverable**: `velocity-report.ini` + `.fmt` build step in the script.

### Phase 4: Code Changes — pdf-generator

**Goal**: Make the Python code work seamlessly in both development and production
modes.

#### 4.1 New module: `tex_environment.py`

```python
# tools/pdf-generator/pdf_generator/core/tex_environment.py
"""TeX environment configuration for development and production modes."""

import os
from dataclasses import dataclass
from typing import Optional


@dataclass
class TexEnvironment:
    """Resolved TeX environment paths and settings."""

    mode: str              # "development" or "production"
    tex_root: Optional[str]  # None for development
    compiler: str          # path to xelatex binary
    fmt_name: Optional[str]  # "velocity-report" or None
    env_vars: dict         # extra env vars for subprocess


def resolve_tex_environment() -> TexEnvironment:
    """Detect and resolve the TeX environment.

    Checks VELOCITY_TEX_ROOT to determine mode.
    Returns a TexEnvironment with resolved paths.
    """
    tex_root = os.environ.get("VELOCITY_TEX_ROOT", "").strip()

    if not tex_root:
        # Development mode — use system xelatex
        return TexEnvironment(
            mode="development",
            tex_root=None,
            compiler="xelatex",
            fmt_name=None,
            env_vars={},
        )

    # Production mode — use vendored minimal tree
    bin_dir = os.path.join(tex_root, "bin")
    compiler = os.path.join(bin_dir, "xelatex")
    texmf_dist = os.path.join(tex_root, "texmf-dist")

    env_vars = {
        "TEXMFHOME": os.path.join(tex_root, "texmf"),
        "TEXMFDIST": texmf_dist,
        "TEXMFVAR": os.path.join(tex_root, "texmf-var"),
        "PATH": bin_dir + os.pathsep + os.environ.get("PATH", ""),
    }

    # Check for precompiled format
    fmt_path = os.path.join(
        texmf_dist, "web2c", "xelatex", "velocity-report.fmt"
    )
    fmt_name = "velocity-report" if os.path.isfile(fmt_path) else None

    if fmt_name:
        # Point TEXFORMATS at the directory containing the .fmt so the
        # engine picks it up automatically — no PyLaTeX changes needed.
        fmt_dir = os.path.dirname(fmt_path)
        env_vars["TEXFORMATS"] = fmt_dir + os.pathsep

    return TexEnvironment(
        mode="production",
        tex_root=tex_root,
        compiler=compiler,
        fmt_name=fmt_name,
        env_vars=env_vars,
    )
```

#### 4.2 Changes to `document_builder.py`

When using a precompiled format (`fmt_name is not None`), the packages are
already loaded in the format. `add_packages()` must skip `\usepackage` calls
for packages baked into the `.fmt`:

```python
def add_packages(self, doc: Document, skip_preloaded: bool = False) -> None:
    if skip_preloaded:
        return  # All packages are in the precompiled format — skip entirely
    # ... existing package loading code ...
```

> **Invariant**: The package list in `add_packages()` and the
> `\RequirePackage` lines in `velocity-report.ini` must stay in sync. If a
> new package is added to `add_packages()`, it must also be added to the
> `.ini` file and the format rebuilt. A CI lint step (Phase 5) enforces this.

The `build()` method accepts an optional `TexEnvironment` and passes
`skip_preloaded=True` when `env.fmt_name` is set.

#### 4.3 Changes to `pdf_generator.py`

The compiler invocation changes to:

1. Call `resolve_tex_environment()` at the start of report generation
2. Use `env.compiler` instead of the hardcoded `"xelatex"` string
3. Inject `env.env_vars` into the subprocess environment (this includes
   `TEXFORMATS` when a `.fmt` is available, so the engine picks up the
   precompiled format automatically — no PyLaTeX changes needed; see Open
   Question 3 for alternatives)
4. In production mode, skip the lualatex/pdflatex fallback chain

The fallback chain becomes:

- **Production mode**: only `env.compiler` (no fallback — the minimal tree is
  the only option)
- **Development mode**: unchanged (`xelatex` → `lualatex` → `pdflatex`)

#### 4.4 Changes to `dependency_checker.py`

Update `_check_latex()` to handle both modes:

- **Development mode**: existing behaviour (check `shutil.which("xelatex")`)
- **Production mode**: validate that `VELOCITY_TEX_ROOT` exists, the binary is
  present and executable, and the `.fmt` file (if expected) is present

Add a new check: `_check_tex_environment()` that reports which mode is active
and whether the minimal tree is healthy.

### Phase 5: Makefile & Deploy Targets

**New targets:**

| Target                         | Purpose                                            |
| ------------------------------ | -------------------------------------------------- |
| `build-texlive-minimal`        | Build the minimal TeX tree from system TeX Live    |
| `build-tex-fmt`                | Compile `velocity-report.fmt` inside the tree      |
| `deploy-install-latex-minimal` | Copy minimal tree to Pi at `/opt/velocity-report/` |
| `validate-tex-minimal`         | Generate a test PDF and compare against reference  |

**Updated targets:**

| Target                 | Change                                                    |
| ---------------------- | --------------------------------------------------------- |
| `deploy-install-latex` | Add conditional: use minimal tree if available, else apt  |
| `pdf-report`           | No change needed — mode detected via environment variable |

### Phase 6: Validation & Testing

#### 6.1 Unit Tests

- `test_tex_environment.py` — test `resolve_tex_environment()` with and without
  `VELOCITY_TEX_ROOT`
- `test_dependency_checker.py` — test production-mode LaTeX checks
- `test_document_builder.py` — test `skip_preloaded` flag

#### 6.2 Integration Test

A Makefile target (`validate-tex-minimal`) that:

1. Generates a reference PDF using full TeX Live (development mode)
2. Generates the same PDF using the minimal tree (production mode)
3. Compares the two PDFs:
   - Visual comparison via `diff-pdf` or `pdftocairo` + `ImageMagick compare`
   - Page count and text extraction comparison via `pdftotext`

#### 6.3 Size Measurement

Record before/after in the plan:

| Metric                           | Before   | After (target) |
| -------------------------------- | -------- | -------------- |
| TeX install size (uncompressed)  | ~800 MB  | < 60 MB        |
| TeX install size (xz compressed) | ~250 MB  | < 15 MB        |
| PDF compilation time (Pi 4)      | baseline | ≤ baseline     |

### Phase 7: pi-gen Integration

Update the pi-gen stage (out of scope for this PR, documented for completeness):

1. Remove `texlive-xetex`, `texlive-fonts-recommended`, `texlive-latex-extra`
   from APT install list
2. Copy minimal tree to `/opt/velocity-report/texlive-minimal/`
3. Set `VELOCITY_TEX_ROOT` in the systemd service environment file
4. Validate PDF generation during image build

## Risks & Mitigations

| Risk                                        | Mitigation                                          |
| ------------------------------------------- | --------------------------------------------------- |
| Missing transitive `.sty` dependency        | `strace` audit captures all file accesses           |
| `.fmt` incompatible after TeX engine update | `.fmt` rebuilt by same engine that ships in tree    |
| Shared library mismatch on Pi               | Link `xelatex` statically or bundle `.so` files     |
| Report layout changes break format          | CI generates PDF in both modes and compares         |
| Developer forgets to audit after adding pkg | CI lint step checks `.ini` matches `add_packages()` |

## Migration Path to Option C

If user feedback indicates demand for custom LaTeX templates, the minimal tree
can be upgraded to a TinyTeX-based installation without changing the Python code:

1. Install TinyTeX at `/opt/velocity-report/texlive-minimal/`
2. Keep `VELOCITY_TEX_ROOT` pointing at the same directory
3. Users can run `tlmgr install <package>` to add packages
4. The precompiled `.fmt` continues to work for built-in reports

The `TexEnvironment` abstraction ensures the pdf-generator does not care whether
the tree is hand-curated or TinyTeX-managed.

## Open Questions

1. **Static vs dynamic xelatex binary** — Should we statically compile XeTeX for
   ARM64, or bundle the required `.so` files alongside the binary? Static is
   simpler but may be harder to build. _Recommendation_: start with copying the
   system binary + `ldd`-resolved libraries; switch to static if library
   versioning becomes painful.

2. **Font caching** — XeTeX uses `fontconfig` to discover fonts. The bundled
   Atkinson Hyperlegible fonts are loaded via absolute path in `fontspec`, so
   `fontconfig` is only needed for fallback fonts. Should we ship a minimal
   `fonts.conf`? _Recommendation_: test without it first; our fonts use explicit
   `Path=` so fontconfig may not be needed.

3. **PyLaTeX `compiler_args` support** — PyLaTeX's `generate_pdf()` does not
   natively support passing `-fmt=...` to the compiler. Options:
   - Patch PyLaTeX (upstream PR or local monkey-patch)
   - Use a wrapper shell script as the `compiler` argument
   - Set `TEXFORMATS` environment variable so the engine finds the `.fmt`
     automatically by name

   _Recommendation_: Use the `TEXFORMATS` environment variable approach — it
   requires no PyLaTeX changes and the engine picks up the format by matching
   the format name to the engine name. Alternatively, a thin wrapper script at
   `$VELOCITY_TEX_ROOT/bin/xelatex` that passes `-fmt=velocity-report` to the
   real binary keeps everything transparent.

4. **`geometry` package** — PyLaTeX adds `geometry` implicitly via
   `geometry_options` in the `Document` constructor. This package is included in
   the package table (§ 2), the `.ini` format source, and the `xelatex -ini`
   command examples above to ensure it is not missed during implementation.

## References

- [RPi Imager Fork Design § 4.6](rpi-imager-fork-design.md)
- [TeX format files — TeX FAQ](https://texfaq.org/FAQ-fmt)
- [PyLaTeX documentation](https://jeltef.github.io/PyLaTeX/current/)
- [TinyTeX — Yihui Xie](https://yihui.org/tinytex/)
- `tools/pdf-generator/pdf_generator/core/document_builder.py` — package list
- `tools/pdf-generator/pdf_generator/core/pdf_generator.py` — compiler invocation
- `tools/pdf-generator/pdf_generator/core/dependency_checker.py` — LaTeX checks
