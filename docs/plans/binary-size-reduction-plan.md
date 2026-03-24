# Binary Size Reduction Plan (v0.5.x)

- **Status:** Active
- **Layers:** Cross-cutting (Go build, web frontend, CI)
- **Target:** v0.5.0 — ship a binary < 40 MB, with CI enforcement to prevent regression
- **Companion plans:**
  [web-frontend-consolidation-plan.md](web-frontend-consolidation-plan.md)
- **Canonical:** [binary-size-reduction](../platform/operations/binary-size-reduction.md)

## Motivation

The Linux ARM64 binary is **211 MB**. The intended ceiling is < 40 MB. The cause is
now understood and almost entirely mechanical — this is not a framework problem.

GSA analysis (`gsa velocity-report-linux-arm64 -f json --compact`) reveals:

| Segment                      | Size     | % of binary | What it is                                                       |
| ---------------------------- | -------- | ----------- | ---------------------------------------------------------------- |
| `static/` embed (stale)      | 172.0 MB | 81%         | 200 accumulated SvelteKit builds never cleaned before `go build` |
| Go code + all dependencies   | 38.2 MB  | 18%         | SQLite, gRPC, protobuf, gonum, echarts, crypto, runtime, etc.    |
| `web/build/` embed (current) | 1.1 MB   | 1%          | The actual single-version SvelteKit build                        |

**The frontend is 1.1 MB.** The 175 MB attributed to the frontend in the GSA output is
almost entirely stale build artifacts in `static/` that were embedded alongside the
current build because `go:embed static/*` globs everything in the directory, and
SvelteKit's content-hashed filenames mean old builds coexist with new ones.

## Root Cause

```
assets.go:
  //go:embed static/*      ← embeds 180 MB of stale builds
  var StaticFiles embed.FS

  //go:embed web/build/*   ← embeds 1.1 MB current build
  var WebBuildFiles embed.FS
```

`static/` is in `.gitignore` but is not cleaned before `go build`. Each `pnpm run build`
(in dev mode, serving from `./static`) writes content-hashed files
(`start.<hash>.js`, `start.<hash>.css`, `app.<hash>.js`, `nodes/<N>.<hash>.js`) into
`static/_app/immutable/`. Old files are never removed. At the time of measurement:

- 200 versions of `start.<hash>.js` (each ~1 MB) = **~167 MB** in `entry/` alone
- 134 stale CSS files = **~4.8 MB** in `assets/`
- 1,890 stale node chunk files = **~7.4 MB** in `nodes/`

`web/build/` (the SvelteKit `adapter-static` output) contains only the current build:
28 files, 1.1 MB total (348 KB gzipped). It is already referenced by `WebBuildFiles`
and serves `/app/` routes in production mode.

`StaticFiles` is used in production **only for `/favicon.ico`**. Everything else
under `/app/` is served from `WebBuildFiles`. In dev mode, `StaticFiles` is not
used — the server reads from the filesystem (`http.Dir("./static")`).

## Current Frontend Profile

| Metric                               | Value                                   |
| ------------------------------------ | --------------------------------------- |
| Pages                                | 9 routes                                |
| Components                           | 6 Svelte components                     |
| Source lines (no tests)              | ~11,000                                 |
| Runtime dependencies                 | 3 (leaflet, layerstack/utils, d3-scale) |
| UI framework deps                    | svelte-ux, layerchart, tailwindcss      |
| Build output (uncompressed)          | 1.1 MB                                  |
| Build output (gzipped)               | 348 KB                                  |
| `start.js` (Svelte runtime + router) | 1,020 KB                                |
| `start.css` (Tailwind)               | 108 KB                                  |
| Node chunks (9 pages)                | 874 B                                   |

**Verdict:** The frontend is already small. No framework change is needed. The problem is
entirely in the build pipeline — stale files accumulating in `static/`.

## Phase 1: Eliminate Stale Embeds (v0.5.0) — saves ~172 MB

This phase alone drops the binary from 211 MB to ~39 MB.

### 1.1 Remove `static/` from `go:embed`

`StaticFiles` is used in production only for `/favicon.ico`. That can be served from
`WebBuildFiles` (which already contains `favicon.ico`). After the change:

```go
// assets.go — after
package radar

import "embed"

//go:embed web/build/*
var WebBuildFiles embed.FS
```

Update `internal/api/server.go` to serve `/favicon.ico` from `WebBuildFiles` (production)
or `./web/build` (dev) instead of `StaticFiles`.

**Functionality sacrificed:** None. `static/` was a dev convenience that leaked into the
production binary. Dev mode already reads from the filesystem.

### 1.2 Clean `static/` before dev builds

Add to the web build script or Makefile:

```makefile
build-web:
	@rm -rf web/build
	@echo "Building web frontend..."
	...existing build command...
```

For dev mode, add a `clean-static` target or modify the dev server startup to clean
`static/_app/immutable/` before rebuilding:

```makefile
clean-static:
	rm -rf static/_app/immutable/entry/* static/_app/immutable/assets/* static/_app/immutable/nodes/*
```

### 1.3 Dev mode: serve from `web/build/` not `static/`

Change the dev-mode file server to read from `./web/build` instead of `./static`.
This eliminates the need for `static/` entirely and means dev and production share
the same file tree.

```go
if devMode {
    staticHandler = http.FileServer(http.Dir("./web/build"))
} else {
    staticHandler = http.FileServer(http.FS(radar.WebBuildFiles))
}
```

### Expected result after Phase 1

| Segment         | Before     | After      |
| --------------- | ---------- | ---------- |
| Stale `static/` | 172.0 MB   | 0 MB       |
| `web/build/`    | 1.1 MB     | 1.1 MB     |
| Go code + deps  | 38.2 MB    | 38.2 MB    |
| **Total**       | **211 MB** | **~39 MB** |

## Phase 2: Strip Debug Symbols — saves ~8–12 MB

The current `LDFLAGS` do not include `-s -w` (strip symbol table and DWARF debug info).
Adding these to production builds is standard practice:

```makefile
LDFLAGS_PROD := -s -w $(LDFLAGS)
```

Apply to `build-radar-linux` and `build-radar-linux-pcap` targets only (keep debug
symbols for local/dev builds).

Expected saving: ~25–30% of the Go code segment = **~8–12 MB**.

### Expected result after Phase 2

| Segment            | Size       |
| ------------------ | ---------- |
| `web/build/`       | 1.1 MB     |
| Go code (stripped) | ~27 MB     |
| **Total**          | **~28 MB** |

## Phase 3: CI Binary Size Gate — prevent regression

Add a CI check that fails if the binary exceeds a threshold:

```bash
#!/bin/bash
# scripts/check-binary-size.sh
MAX_SIZE_MB=45  # headroom above 28 MB target
BINARY="velocity-report-linux-arm64"

if [ ! -f "$BINARY" ]; then
  echo "Binary not found: $BINARY"
  exit 1
fi

SIZE_BYTES=$(stat -f%z "$BINARY" 2>/dev/null || stat -c%s "$BINARY" 2>/dev/null)
SIZE_MB=$((SIZE_BYTES / 1024 / 1024))

if [ "$SIZE_MB" -gt "$MAX_SIZE_MB" ]; then
  echo "FAIL: Binary is ${SIZE_MB} MB (limit: ${MAX_SIZE_MB} MB)"
  echo "Run 'make clean-static' and ensure no stale embeds."
  exit 1
fi

echo "OK: Binary is ${SIZE_MB} MB (limit: ${MAX_SIZE_MB} MB)"
```

Wire into `make lint`:

```makefile
lint: lint-go lint-python lint-web check-binary-size
```

## Phase 4: Further Reductions (optional, v0.5.x)

These are diminishing returns but worth considering:

### 4.1 Replace embedded `echarts.min.js` with lightweight alternative

`internal/lidar/l9endpoints/l10clients/assets/echarts.min.js` is 1.1 MB embedded for
the LiDAR status/dashboard pages. If these pages migrate to the Svelte frontend (tracked
by [web-frontend-consolidation-plan](web-frontend-consolidation-plan.md)), echarts can
be removed from the Go binary. Saving: ~1 MB.

### 4.2 UPX compression

`upx --best` typically achieves 50–60% compression on Go binaries. Would reduce a
28 MB stripped binary to ~12–15 MB. Trade-off: slower startup (decompression), harder
to debug crashes, some security scanners flag UPX-compressed binaries. **Not recommended
for production** on resource-constrained Raspberry Pi — the decompression overhead
matters on ARM64 with limited RAM.

### 4.3 Lazy-load Leaflet

The 1 MB `start.js` bundle includes Leaflet (map library). If the map is only used on
the site detail page, Leaflet could be dynamically imported (`import('leaflet')`) so it
is code-split into a separate chunk loaded on demand. This does not reduce binary size
(the chunk is still embedded) but improves initial page load. Worth doing independently.

## Framework Assessment

The user asked whether Svelte is the right framework given the small footprint. Summary:

| Factor                  | Assessment                                                                             |
| ----------------------- | -------------------------------------------------------------------------------------- |
| Build output            | 1.1 MB (348 KB gzipped) — already tiny                                                 |
| Runtime complexity      | 9 pages, 6 components, minimal state — well within Svelte's sweet spot                 |
| Alternative: Preact     | Similar output size (~1 MB), less ecosystem, migration cost                            |
| Alternative: vanilla JS | Smaller runtime, but manual routing/reactivity — maintenance burden                    |
| Alternative: htmx       | ~14 KB runtime, but requires server-rendered HTML — architectural shift                |
| Verdict                 | **Keep Svelte.** The framework is not the problem. The build artifact accumulation is. |

No functionality needs to be removed. The frontend is lean. The only thing that was
excessive was the build hygiene.

## What Gets Sacrificed

| Sacrifice                             | Impact                                                                                           |
| ------------------------------------- | ------------------------------------------------------------------------------------------------ |
| `static/` embed removed               | None in production; dev mode path changes                                                        |
| Debug symbols stripped (prod only)    | Stack traces in production crashes are less readable; mitigated by keeping symbols in dev builds |
| `echarts.min.js` removal (if pursued) | Only if LiDAR status pages move to Svelte frontend                                               |

## Risks

| Risk                                         | Likelihood | Impact | Mitigation                                                            |
| -------------------------------------------- | ---------- | ------ | --------------------------------------------------------------------- |
| Dev mode behaviour change breaks workflow    | Medium     | Low    | Test dev mode with `./web/build` path before merging                  |
| CI size gate too tight, blocks valid PRs     | Low        | Low    | Set threshold at 45 MB (60% headroom above ~28 MB target)             |
| Stripped binary makes crash debugging harder | Low        | Medium | Keep debug symbols in local/dev builds; deploy with `GOTRACEBACK=all` |

## Checklist

### Outstanding

- [ ] Remove `//go:embed static/*` from `assets.go` and delete `StaticFiles` var (`S` effort)
- [ ] Update `internal/api/server.go` to serve favicon from `WebBuildFiles` (`S` effort)
- [ ] Change dev-mode handler to read from `./web/build` instead of `./static` (`S` effort)
- [ ] Add `rm -rf web/build` to `build-web` target for clean builds (`S` effort)
- [ ] Add `-s -w` to `LDFLAGS` for production build targets (`S` effort)
- [ ] Create `scripts/check-binary-size.sh` and wire into `make lint` (`S` effort)
- [ ] Validate binary size after changes — target < 40 MB (`S` effort)
- [ ] Update `web/` README if dev workflow changes (`S` effort)

### Deferred

- [ ] Remove embedded `echarts.min.js` — gated on LiDAR status page migration to Svelte
- [ ] Evaluate UPX for deployments where binary size is critical — not recommended for RPi
- [ ] Lazy-load Leaflet via dynamic import — tracked as general frontend optimisation
