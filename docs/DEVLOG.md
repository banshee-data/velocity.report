# Development log

## April 16, 2026 - Setup guide polish & responsive tables

- Added responsive table styling with striped row backgrounds for the docs site.
- Standardised title casing and improved table formatting in the setup guide.
- Removed unused guide images (`guide-aim-sutro`, `guide-angel`) and added `link-ignore` annotations for image paths resolved at build time.
- Bumped version to 0.5.1-pre5.

## April 15, 2026 - Rack geometry, isometric drawings & assembly prose

- Added fastener hole geometry to the rack model: crossbar, brace, and pipe connection definitions in `rack.json`.
- Generated isometric BOM drawing with title block and combined drawing sheet.
- Produced aiming and cosine-angle SVG overlays via `draw_overlays.py`; added matching guide images.
- Rewrote T-frame assembly section of the setup guide for clarity and correct step ordering.
- Updated cost estimates, deployment options, and cable-length references in the setup guide.
- Refined homepage content and descriptions; updated gradient border styles for light and dark modes.
- Added Makefile targets for wiring diagram generation; added `pyyaml` dependency.
- Addressed Copilot PR review comments across docs, web, and Python code.

## April 14, 2026 - Rack-mount drawing tools & docs site improvements

- Created rack-mount engineering drawing tools: `draw_rack.py`, `draw_overlays.py`, and `model.py` with `drawsvg` dependency.
- Added Eleventy table-of-contents functionality with content preamble and body filters.
- Switched docs site colour scheme to emerald for buttons, links, footer, and section titles.
- Added guide hero image and updated setup guide with engineering drawing references.
- Added `cheerio` dependency for HTML manipulation in Eleventy build.
- Added Makefile targets for diagram installation, rendering, and stale docs-server cleanup.
- Updated setup guide parts list and build overview formatting; removed redundant sections.

## April 13, 2026 - RPi image CI fix & setup guide overhaul begins

- Fixed Raspberry Pi image CI by installing `qemu-user-static` before ARM64 builds; deduplicated QEMU setup action (#467).
- Began setup guide overhaul: added hardware photos, updated prerequisites, corrected dates and content.
- Moved radar wiring documentation to a dedicated directory; added connector pinout SVG generation script.
- Generated DE-9 and M12 connector pinout SVGs with connection parsing logic.
- Updated homepage with Q&A section, enhanced inline code styling, and improved header navigation links.
- Added PoE HAT details and mounting-angle measurement guidance to the setup guide.

## April 12, 2026 - LiDAR docs restructure, directory indices & STYLE compliance

- Deprecated Python matplotlib chart rendering and updated STYLE.md guidelines (#463).
- Restructured `docs/lidar/` as a directory index: consolidated LIDAR.md and LIDAR_ARCHITECTURE.md, added layer summary and implementation status tables.
- Created directory index files for `data/` subdirectories (DATA.md, DATA_STRUCTURES.md, EXPERIMENTS.md, EXPLORE.md, MATHS.md) with README symlinks.
- Added PLATFORM.md, VISUALISER.md, and DOCS.md as documentation hub indices. Enhanced CONFIG.md with primary consumer references and CI parity check.
- Expanded coding standards with file naming conventions. Fixed stale references: OPS243-C → OPS243-A, L8/L9 status, monitor → server.
- Banned compilable code blocks and Author/Authors metadata in design documents. Updated DESIGN.md for chart rendering: matplotlib deprecation, SVG-first architecture, shared abstractions.
- Stripped version-number references from 16 documentation files to make docs timeless. Improved LiDAR maths documentation clarity across classification and taxonomy files.
- Integrated ascfix Markdown formatter into CI, then removed it: corrupts hand-crafted ASCII art. Documented findings as an operations note.
- Removed compilable code blocks from 30 plan files and 8 hub docs: replaced Go, SQL, JSON, YAML, Protobuf, and other fenced blocks with field tables and prose. Banned Author/Authors metadata and added metadata audit to docs-release-prep (#465).
- Fixed tag-triggered release asset builds and image uploads in CI (#466).
- {dd/fix/dependabot-updates} Tightened pnpm override ranges and added node engines constraint.

## April 11, 2026 - README refresh, dependency fixes & documentation polish

- Graduated completed plans: API endpoint specifications, width-check and update-PR-description skills, and release-prep skill documentation (#460).
- Refreshed README.md: expanded alpha-software warning with security notes, broadened audience description, simplified developer commands, and added key-documents table.
- Created MAGIC_NUMBERS.md documenting project-wide numeric constants. Added `check-backtick-paths.py` to detect stale file-path references in Markdown prose.
- Standardised British English spelling (neighbour, metre) and heading capitalisation across documentation. Fixed link formatting in multiple files.
- Added width-check skill for advisory prose line-width validation and update-PR-description skill for generating structured PR descriptions from branch diffs.
- Added release-prep skill documentation. Updated documentation structure guidelines to prefer prose and tables over pre-built code blocks.
- Added detailed API endpoint specifications for serial configuration and testing. Refactored transit deduplication documentation.
- {dd/fix/dependabot-updates} Fixed 19 dependabot alerts across Go, npm, and Python ecosystems.

## April 10, 2026 - documentation standardisation & plan graduation

- Graduated 7 plans to symlinks, consolidated fragmented documentation, removed deprecated files, and fixed 23 dead links after the visualiser directory move (#459).
- Completed the opening-paragraph audit: added narrative introductions to 58 hub docs across LiDAR architecture, platform operations, radar, and UI directories.
- Created the plan-graduation skill with slim hub-doc template and two-PR graduation rule. Added the docs-release-prep skill.
- Resolved the lidar-schema-robustness plan as complete. Added status lines to 7 plans, consolidated webserver-tuning into the fixit plan.
- Added CI docs link health check and refined linting scripts. Auto-built the documentation site when missing from the RPi image.
- {dd/fix/security-c2-c3} Hardened serial command injection protection and restricted default listen addresses to localhost.

## April 9, 2026 - asset naming, Go report generation & RPi image security

- Implemented versioned asset naming across Go binaries, CI workflows, macOS builds, and pi-gen image output (#457). Binary names now include version and architecture suffixes.
- Separated the velocity service account from the login user in the RPi image (#458): dedicated system account for the service, restricted sudoers to the login user, fixed SSH key ownership.
- Added device entries and custom favicon for the docs site RPi Imager catalogue (#456). Expanded the OS list JSON with Raspberry Pi 5, 4, and 3 support.
- Switched CI Go setup to `go-version-file` for automatic version tracking (#451). Backed out the SonarCloud integration.
- Added the documentation standardisation audit plan with gate definitions and checklist (#452).
- Added Vite config symlink for worktree support (#454).
- {worktree-chart-migration} Built Go-native SVG chart rendering: histogram and time-series packages with Atkinson Hyperlegible font support, LaTeX template rendering, and a PDF report generation subcommand with ZIP output.
- Began the documentation audit: fixed 8 gate violations, graduated 15 complete plans to symlinks, consolidated the visualiser app directory, reclassified PCAP design docs as plans, and added config cross-references.

## April 8, 2026 - map editor, homepage build & tooling

- Added confirmation modal for mode switching in `MapEditorInteractive` to prevent accidental data loss. Combined angle stepper and remove button into a single row for custom SVG mode.
- Enhanced save error handling on the site settings page with user-visible feedback on failure.
- Groomed the backlog (v0.5.1–v0.5.4): migrated Capabilities API to v0.5.3, Classification enum split to v0.5.8, Clock abstraction to v0.5.2, and PCAP motion tooling to v0.5.6. Reordered fix-it phases within v0.5.4.
- Fixed `orderedEndpoints` to use stable declaration order instead of shuffling.
- Improved LaTeX log fatal error detection: normalised prefixed lines (`!` and `LaTeX Error:`) and added corresponding test cases.
- Standardised `effective_start` timestamp format in migration 000034 and `schema.sql`.
- Refactored `sync-schema.sh` to use awk for generating `INSERT OR IGNORE` fixture statements.
- Refactored `dev-ssh.sh` to cleanly separate SSH options from remote command arguments.
- Tightened link-checker skip predicate to match only `image/stage-*/files/` paths.
- Updated `devlog-update` skill procedure: added `git fetch` step, gap-fill logic for incomplete existing entries, and broadened amend rules.
- Restructured the homepage download section with per-platform SHA-256 hashes, RPi Imager JSON references, and Docker-based ARM64 cross-compilation (#453).
- Merged the error surface voice audit across Go, JavaScript, Python, and shell scripts (#449).

## April 7, 2026 - RPi image hardening, web map editor & backlog grooming

- Hardened the RPi image cleanup script: `dpkg --purge` of the camera stack triggered an `apt-get autoremove` cascade that removed `librsvg2-bin`, `network-manager`, and other runtime packages. Added `apt-mark manual` protection and a post-purge verification step.
- Added build metadata stamping to the RPi image: `VR_VERSION`, `VR_BUILD_TIME`, and `VR_GIT_SHA` written to `/etc/velocity-report-build` and displayed in the MOTD login banners.
- Refined MOTD Unicode art banners and added `check-quarter-blocks.sh` lint script to detect block-element characters that render incorrectly in some terminal fonts.
- Enhanced TLS certificate generation (`velocity-generate-tls.sh`): improved directory permissions, certificate validity checks, and error diagnostics.
- Built radar SVG map position override feature end-to-end: new `radar_svg_x`/`radar_svg_y` columns in the `site` table (migration 000033), Go model updates, Svelte `MapEditorInteractive` component with SVG upload and draggable position controls, PDF generator support, and clamping logic with 5% border margins.
- Added `map-svg.ts` utility module with SVG rendering types and helper functions for the web frontend.
- Refactored the web settings page layout for better responsiveness. Simplified `Field` component defaults and improved number input styling.
- Added `isDateRangeStale` freshness check for `reportSettings` localStorage: dates older than 18 hours fall back to the default 14-day range. Non-date settings always restore. Shared helper in `$lib/reportSettings.ts` with Jest test coverage.
- Groomed the backlog: split v0.5.6, added paper-implementation gap remediation items (P1/P2/P3), priority-sorted all milestones, performed domain analysis, moved items between milestones, and split v0.6.0 into Deployment & Packaging and macOS Local Server.
- Added four new plan documents: [domain tag vocabulary](plans/domain-tag-vocabulary-plan.md), [asset naming standardisation](plans/asset-naming-plan.md), [PCAP motion detection and scene split](plans/pcap-motion-detection-and-split-plan.md), and [macOS local server](plans/macos-local-server-plan.md).
- Added `dev-ssh` and `dev-ssh-audit.sh` scripts for SSH access and remote RPi health checks.
- Fixed LaTeX escaping in `ParameterTableBuilder` for monospace rendering. Fixed `fromDatetimeLocalToUnixSeconds` to manually parse datetime strings for consistent cross-browser behaviour.
- Added `devlog-update` skill for automated development log updates from git history.
- Improved LaTeX log error detection: added explicit fatal line signatures and refined pattern matching.
- Hardened shell scripts: refactored `sync-schema.sh` migration logic to use null-delimited reads, enhanced `dev-ssh.sh` host key management with explicit confirmation, and switched `dev-ssh-audit.sh` to UTC timestamps.
- Refined Markdown link checker to skip `image/stage-*/files/` directories. Clarified `velocity-deploy` removal in the remote host upgrade runbook.
- Installed `fonts-noto-color-emoji` in the RPi image for map label rendering. Updated sudoers configuration for specific commands.
- Added UTC timestamp guidelines to `STYLE.md` and `coding-standards.md`.

## April 6, 2026 - Claude code init, security fixes, paper gap analysis & CI linting

- Initialised Claude Code configuration (#447): [CLAUDE.md](../CLAUDE.md), [.claude/agents/](../.claude/agents) (7 personas), [.claude/skills/](../.claude/skills) (8 workflow slash commands), and shared knowledge modules in [.github/knowledge/](../.github/knowledge).
- Updated vulnerable dependencies across Go, npm, and Python ecosystems (#441). Fixed a dev-mode path traversal vulnerability by normalising `requestedPath` before joining with `buildDir`.
- Added paper-vs-implementation gap analysis ([data/maths/paper-implementation-gap-analysis.md](../data/maths/paper-implementation-gap-analysis.md)): reviewed 24 papers across 11 subsystems, identified 35 gaps with P1/P2/P3 priority tiers.
- Added `download-papers.py` script with DOI/URL resolution, SSRF guards, and dry-run mode (#446).
- Merged Go clock abstraction plan (#428): `timeutil.Clock` interface for pipeline timing, replay pacing, and benchmark instrumentation.
- Added Markdown CI workflows: `check-md-links` for dead link detection, `check-backtick-paths.py` for stale file path references, `public_html` CI for docs site linting. Wired into `make lint-docs`.
- Refreshed [README.md](README.md) with sample PDF report and visualiser demo images (#443, #444). Added `STYLE.md` with writing conventions and British English guidelines.
- Added wiring diagrams and YAML configuration for OPS243-A radar sensor (`docs/hw/`).
- Updated agent preparedness documentation, added `fix-links` skill, and expanded coding standards with configuration and media guidelines.
- Added `age-color` terminal script and new Makefile log convenience targets.

## April 5, 2026 - README & ASCII art refresh

- Refreshed [README.md](README.md) structure: reorganised sections, updated project description and feature list.
- Iterated on ASCII art header designs across documentation files.

## April 1, 2026 - ASCII art update

- Updated ASCII art crosswalk and banner designs in documentation (#442).

## March 31, 2026 - voice quality audit & error message rewrite

- Completed the error surface voice audit across all subsystems: rewrote user-facing error messages in Go HTTP handlers, [cmd/radar](../cmd/radar) CLI, Python PDF tools, and Svelte web frontend to match the project voice (concise, helpful, no blame, diagnostic hints where useful).
- Rewrote shell script messages and marked the voice audit plan complete.
- Centralised API error handling in the web frontend for consistent error message display.
- Fixed TLS certificate generation to persist the CA across server certificate renewals, preventing trust breakage when the server cert is regenerated.
- Updated MOTD ASCII art drafts for the RPi login banners.
- Fixed missing `.invalidConfiguration` case in Swift `APIError` switch statements.

## March 30, 2026 - deprecation signalling, autoTuner fix & homepage

- Fixed macOS `APIError` handling in `LabelAPIClientTests` (#440).
- Updated project-wide copy across JS, Go, and Python surfaces (#431): refreshed deprecation messages for legacy deployment targets.
- Fixed a race condition in `AutoTuner` completion persistence: reordered operations to prevent concurrent write conflicts.
- Added Raspberry Pi image section to the homepage with download link and SHA256 checksum (commented out pending first release).

## March 29, 2026 - RPi image: web build, HTTPS & TLS, image cleanup

- Added Raspberry Pi download section to the homepage (#437): `release.json` data source, per-platform SHA256 hashes, clipboard fallback for copy buttons.
- Fixed homepage mobile layout (#439): resolved light/dark theming split and download card spacing.
- Refined Terry agent coaching workshop documentation (#438).
- Fixed "Web Frontend Not Built" in the RPi image: whitelisted [web/build/](../web/build) in `.dockerignore` and added a web build step before Go compilation in `build-image.sh`. <!-- link-ignore -->
- Moved TLS termination from Go server to nginx reverse proxy. Go server stays on `:8080` (plain HTTP), nginx handles HTTPS on port 443.
- Added first-boot TLS certificate generation (`velocity-generate-tls.sh`): per-device ECDSA P-256 local CA (10-year) and server cert (825-day). Idempotent, regenerates on expiry.
- Exposed CA certificate at `GET /ca.crt` via nginx for browser trust installation.
- Installed root-level project documents into the image for on-device reference.
- Documented the TLS strategy in [docs/platform/operations/tls-local-certificates.md](platform/operations/tls-local-certificates.md).

## March 28, 2026 - RPi image: build pipeline, flash target & first-boot polish

- Consolidated Go version information printing into a single function across [cmd/radar](../cmd/radar) and [cmd/velocity-ctl](../cmd/velocity-ctl).
- Updated build process and documentation for RPi images: clarified `build-image.sh` sections and improved error handling.
- Added timestamps to image filenames to prevent collisions during rebuilds.
- Added installation step for `tuning.defaults.json` in the pi-gen run script so the service starts with a valid config.
- Added `make flash-image` target for flashing images to SD card on macOS, with device detection and safety prompts.
- Added `DISABLE_FIRST_BOOT_USER_RENAME` to pi-gen config to prevent the boot wizard overwriting the `velocity` user.
- Refactored systemd service management to prevent crash-loop on boot: the service now waits for the database directory and starts cleanly even on first boot before the sensor is connected.
- Added login MOTD banners warning about the default password and providing help commands (`velocity-ctl status`, `velocity-ctl upgrade`).
- Suppressed the first-boot user-creation wizard and cancelled pending user renames during image export.
- Moved `data/align/` to its proper location within the reference data structure.
- Enhanced the image build with reference data installation (alignment CSVs, sample VRLOGs) so the appliance ships with test datasets.
- Added `make clean-images` to remove old `.img`, `.zip`, and older `.xz`/`.sha256`/`.info` files from the deploy directory, keeping only the latest build. Recovered 18 GB → 529 MB on first run.
- Updated `.dockerignore` to clarify web frontend asset inclusion.
- Moved [TENETS.md](../TENETS.md) to the repository root and updated all cross-references in [.github/copilot-instructions.md](../.github/copilot-instructions.md), agent files, and documentation.

## March 27, 2026 - RPi image: Pi-gen integration & ARM64 cross-compilation

- Added Raspberry Pi image build support using pi-gen (bookworm-arm64 fork). The `image/` directory now contains pi-gen stage definitions, configuration, and the unified `build-image.sh` script.
- Added `Dockerfile.build` for ARM64 cross-compilation of the Go binary on macOS (Apple Silicon). The Docker build compiles `velocity-report` and `velocity-ctl` for `linux/arm64` with CGO enabled via `zig cc`.
- Added `.dockerignore` to control the Docker build context: whitelists only Go sources, web build output, static assets, and build scripts.
- Improved image extraction and compression logic: the output pipeline now produces `.img.xz` with SHA-256 checksum and `.info` metadata files.

## March 26, 2026 - velocity-ctl, setup guide & architecture docs

- Created `velocity-ctl` as the on-device management tool, replacing the remote `velocity-deploy` script. Implements `backup`, `rollback`, `status`, and `upgrade` subcommands. Designed to run _on_ the Raspberry Pi, not from a development machine.
- Removed `velocity-update` shell script (superseded by `velocity-ctl upgrade`).
- Removed `velocity-deploy` references from CI workflows, Makefile, and documentation. Removed SSH configuration parsing code and associated tests from the Go codebase.
- Updated CI ARM64 binary build process to produce both `velocity-report` and `velocity-ctl`.
- Added setup guide publication plan with content placeholders and release checklist.
- Added `os-list.json` for the Raspberry Pi Imager catalogue so users can flash images from the official Imager tool.
- Enhanced the setup guide with backup and restore instructions for sensor data.
- Updated architecture documents for ground plane extraction and GPS parsing.

## March 25, 2026 - documentation refresh & RPi image planning

- Added [data/QUESTIONS.md](../data/QUESTIONS.md) with open research questions about hourly comparison data and speed distributions.
- Refactored [README.md](README.md) structure and added [COMMANDS.md](../COMMANDS.md) with make targets reference and ASCII art headers.
- Split the Raspberry Pi image plan into phased delivery (Phase 1 bootable image, Phase 2 OTA, Phase 3 fleet management).
- {dd/mac/dmg-signing} Added macOS code-signing and notarisation to the CI workflow and Makefile (#425): `codesign --deep`, `notarytool`, and `stapler`.
- {dd/mac/dmg-signing} Fixed notarisation auth handling for macOS 13+ keychain profiles and removed the unreliable `spctl` DMG check.
- {dd/mac/dmg-signing} Added a pre-build guard to verify the visualiser `.app` bundle exists before attempting DMG creation.
- {dd/mac/dmg-signing} Enhanced notarisation error handling with detailed logging and keychain path resolution.

## March 24, 2026 - config consolidation, ERD refresh & workflow docs

- Consolidated LiDAR immutable/run-config plumbing across radar startup, storage, replay-case management, and backfill tooling.
- Extracted testable helper paths in [cmd/radar](../cmd/radar) and added focused coverage for the run-config backfill tool.
- Updated immutable run-config and replay-case operations docs, and replaced older scene-management implementation notes with the newer asset plan.
- Switched plan-hygiene reporting to an advisory workflow in CI rather than a hard PR gate.
- Refreshed schema visualisation tooling: added configurable SQLite ERD grouping, updated graph scripts, and regenerated `SCHEMA.svg`.
- Expanded `MATRIX.md` and refreshed the Matrix Tracer agent around the live schema/documentation inventory workflow.
- Tightened Go/API hygiene around JSON tags, dropped-error handling, and build metadata.
- Added fresh plan docs for binary-size reduction, Go structural hygiene, and the dual-tool Claude/Copilot agent workflow architecture.
- Released 0.5.0 🌞 "Sunny Southeast": version set across Go/Python/web/macOS with expanded CHANGELOG.
- {copilot/update-capabilities-check-radar-mode} Refactored the capabilities API to a multi-sensor named-object format (#430): per-sensor named objects in Go handlers and web stores.
- {copilot/update-capabilities-check-radar-mode} Added smart LiDAR polling: web frontend polls LiDAR endpoints only when capabilities indicates hardware is present.
- {copilot/update-capabilities-check-radar-mode} Added a multi-sensor capabilities plan document and marked it complete after implementation.
- {copilot/update-capabilities-check-radar-mode} Expanded web test coverage with a multi-sensor `lidarState` derived store test.

## March 23, 2026 - capabilities gating, stream fix & host upgrade runbook

- Added capabilities API surfaces in radar/admin Go endpoints and covered them with new API and radar tests.
- Wired capabilities through the web client and stores so unfinished LiDAR UI paths can stay hidden until backend support is present.
- Updated the web frontend consolidation plan to reflect the new capability-gating approach.
- Fixed `StreamFrames` hang conditions in replay endpoints to prevent stuck frame streams.
- Added a remote-host upgrade runbook documenting safer deployment and upgrade flow for field systems.

## March 22, 2026 - schema hardening, track cleanup & canonical docs

- Hardened the pre-v0.5.0 schema path across SQL and Go: added replay annotation/evaluation integrity migration work and strengthened label-path coverage.
- Regenerated schema artefacts and refreshed `VRLOG_FORMAT.md`, LiDAR architecture docs, and schema-hardening documentation.
- Added troubleshooting and planning material for schema robustness and garbage-track investigation.
- Cleaned up LiDAR track persistence and analysis paths, including the new `lidar_all_tracks` view, schema updates, export/report changes, and related tests.
- Made docs more canonical across Python/docs surfaces and standardised pre-v1.0 release naming.
- Added canonical docs plan-hygiene CI, 4-hub structure, and Canonical metadata to all 69 plan files with 45 populated hub doc stubs.

## March 21, 2026 - L8-L10 refactor, CLI networking & storage cleanup

- Landed the large L8-L10 refactor across Go packages and docs: reshaped server/package boundaries, updated routes, and continued the monitor → endpoint/client split.
- Moved LiDAR network settings fully to CLI/runtime flags, trimming duplicated config-file state and updating radar/config docs and tests.
- Broke up large [internal/api](../internal/api) server files into narrower admin, middleware, radar, reports, sites, and timeline units.
- Broke up large [internal/db](../internal/db), [internal/lidar/l2frames](../internal/lidar/l2frames), and [internal/lidar/l5tracks](../internal/lidar/l5tracks) files into more focused storage, frame-builder, and tracking units.
- Split tuning/background internals into cleaner accessors, codec, validation, background-manager, and region-specific files.
- Refactored SQLite access through shared storage interfaces for labels and server-facing codepaths.
- Continued general tech-debt cleanup across reports, tracking update logic, transit tooling, and plan/backlog alignment.
- Fixed context propagation and serialmux race issues.
- Refreshed ERD column layout in generated schema docs.

## March 20, 2026 - config refactor & migration tooling

- Reworked the radar/LiDAR configuration model around cleaner startup plumbing and helper extraction in [cmd/radar](../cmd/radar).
- Added `config-migrate` with broad coverage for moving existing configs onto the new layout.
- Added `config-validate` to check migrated/runtime configs before deployment.
- Expanded radar flag and config-path test coverage substantially.
- Propagated config changes through PCAP and settling tooling, with matching documentation and example refreshes.
- Converted tuning API from flat dot-path keys to nested JSON format across Go and web surfaces.
- Normalised American→British English: `neighbor` → `neighbour`, `meters` → `metres` across code, comments, and test names.

## March 19, 2026 - breaking migration merge & immutable run config planning

- Landed the breaking schema cleanup across SQL, Go, web, and macOS surfaces.
- Regenerated schema artefacts and aligned v0.5.0 backlog/plan updates around the migration.
- Added the immutable run-config asset plan.
- Added canonical project-files planning for documentation and AI/customisation structure.
- Added Go cleanup planning covering structural hygiene, god-file splitting, and structured logging.
- Expanded Go coverage across frame-builder, monitor, visualiser, analysis, DB, and HTTP utility codepaths.
- Strengthened DB-boundary handling with alignment planning and import-check tooling.

## March 18, 2026 - contributor refresh, schema planning & homepage download

- Rewrote [CONTRIBUTING.md](../CONTRIBUTING.md) with updated contributor guidance, personas, and workflow expectations.
- Expanded the v0.5.0 breaking-schema update plan to cover migration sequencing and cleanup scope.
- Added `VelocityVisualiser.app` download and promo assets to the homepage, including supporting media/conversion tooling.
- {copilot/add-bumper-sticker-designs} Added a Cairo-based bumper sticker generator tool (#403): Python CLI producing SVG/PNG stickers with configurable text and gradient backgrounds.
- {copilot/add-bumper-sticker-designs} Addressed code review feedback on `y_frac` semantics, Cool-S proportions, and step divisor clarity.

## March 17, 2026 - shim removal, MATRIX inventory & sweep worker planning

- Removed remaining backward-compatibility shims across Go, macOS, and Python surfaces, with matching backlog and plan updates.
- Added the Matrix Tracer agent and the new `list-matrix-fields.py` inventory tooling.
- Expanded `MATRIX.md` substantially and updated supporting structure docs around live implementation inventory.
- Added HINT metric observability planning and related data-structure remediation updates.
- Added the distributed sweep-workers plan.
- Added the 100-line plan and checked in experiment notes / documentation cleanup across docs and Python.
- {codex/plan-remaining-obb-heading-stability} Added OBB heading stability plan (#397): documented remaining work for oriented bounding box heading estimation, covering temporal smoothing, velocity-aligned correction, and evaluation metrics.

## March 16, 2026 - header standardisation & backlog prune

- Standardised documentation headers to bullet-list metadata format (`- **Key:** value`) across the repo and updated related tooling/docs to match.
- Pruned backlog and decisions entries via Flo's weekly planning pass, tightening milestone scope and decision tracking.

## March 14, 2026 - data reorganisation, VRLOG refresh & UK spelling

- Reworked VRLOG labelling and replay contracts across Go, Swift, and tooling: refreshed label APIs, run-track endpoints, replay handlers, recorder encoding, analysis/report typing, and `gen-vrlog` / `vrlog-analyse` support.
- Expanded macOS visualiser run-browser and labelling flows alongside the VRLOG changes.
- Added and refreshed VRLOG analysis / format documentation, terminology notes, and related backlog references.
- Reshaped the `data/` tree: moved structures and maths material into [data/structures/](../data/structures) and [data/maths/](../data/maths), added [data/explore/](../data/explore), and relocated exploratory analysis outputs.
- Normalised British English and refreshed repo-wide cross-references, including `convergence-neighbour` naming and documentation path fixes.

## March 12, 2026 - agent stack refresh, light-mode plan & SSE delivery fix

- Reworked AI-agent tooling into the current Appius/Euler/Flo/Grace/Malory/Ruth/Terry stack.
- Added [.github/knowledge/](../.github/knowledge), [TENETS.md](../TENETS.md), portraits, and refreshed agent/instruction docs around the new stack.
- Added the macOS visualiser [light-mode plan](plans/lidar-visualiser-light-mode-plan.md) and linked it into the backlog.
- Fixed SSE subscribers receiving test payloads end-to-end: buffered serialmux subscriber channels, tightened subscriber registration timing, and updated replay reset handling in `AppState`.

## March 11, 2026 - dual frame representation & naming standardisation

- Added [L2 dual representation](plans/lidar-l2-dual-representation-plan.md) in `LiDARFrame` (`Points` + `PolarPoints`), removed repeated polar rebuild work, propagated through adapters, PCAP/network ingestion, perception, pipeline stages, and tests.
- Renamed `PeakSpeedMps` to `MaxSpeedMps` across analysis/reporting, classification, proto, Swift visualiser, and tests; CI fix for remaining references missed in the initial rename.
- Standardised docs naming conventions: all files to `lowercase-with-hyphens.md`, moved schema ERD SVG from [internal/db/](../internal/db).
- Overhauled LiDAR architecture Mermaid layer diagram with enhanced documentation and added Mermaid flowchart readability utility script.
- Added [coordinate-flow audit](lidar/architecture/coordinate-flow-audit.md) documenting polar/Cartesian transitions per tracking stage with flowcharts, matrices, and single-projection strategies.
- Added 52-entry BibTeX bibliography (`docs/references.bib`) covering L3f, L5h, L6e planned work and existing references.
- Updated BACKLOG with v0.5.0 breaking-change/proto work split and shipped component status.
- Activated shared web cache for worktrees: `make activate-web-cache` for Codex environment setup, reusable Make targets.

## March 10, 2026 - PCAP playback fix & VRLOG analysis metrics

- Hardened PCAP playback and analysis paths: replay timing fixes, speed-ratio controls, datasource/monitor/dashboard updates, database write cleanup, per-frame timing traces in `pcap-analyse`, and matching macOS visualiser model/client changes.
- Implemented all §12.1 "implementable now" VRLOG analysis metrics: per-track detail fields, comparison report, and supporting `vrlog-analyse` code and tests.
- Documented outstanding data-science questions in [CONTRIBUTING.md](../CONTRIBUTING.md), linked reflective-sign pose-anchor proposal from maths README, added Mermaid validation tooling notes.

## March 9, 2026 - LiDAR L7-L10 & metrics-first documentation

- Updated [LiDAR data layer model](lidar/architecture/LIDAR_ARCHITECTURE.md) from six-layer to ten-layer architecture: added L7 analytics, L8 endpoints, L9 client, L10 scene layers to docs and backlog.
- Rewrote contributor personas in [CONTRIBUTING.md](../CONTRIBUTING.md), expanded role guidance for Swift/macOS, Svelte/web, PDF/matplotlib, and platform/observability.
- Added [ticTacTail](plans/tictactail-platform-plan.md) VRLOG inspection command specification: CLI shape, package layout, modes, metrics, and aggregation windows.
- Added `internal/lidar/monitor/` deprecation and migration plan to `server/`, `l7analytics/`, `l8presentation/`.
- Updated 0.5.0 proto plan: [percentile aggregation](plans/speed-percentile-aggregation-alignment-plan.md) decisions, [metrics registry](plans/metrics-registry-and-observability-plan.md) strategy, tagging guidance, and observability/export mapping.
- Added Jess agent standup and planning behaviours.
- British English spelling lint check, backlog decision updates, Python CI bump.

## March 8, 2026 - L7/L8/L9 client planning

- Added LiDAR [L7 analytics and L8 visualisation refactor plan](plans/lidar-l8-analytics-l9-endpoints-l10-clients-plan.md): phased implementation steps and checklist covering docs, L7/L8 boundaries, monitor/, and generated artefacts.

## March 7, 2026 - platform simplification phase 1

- Implemented Phase 1 of [platform simplification](plans/platform-simplification-and-deprecation-plan.md): added deprecation warnings to legacy deployment targets and tools, updated documentation for `velocity-deploy` and Makefile targets.

## March 6, 2026 - compatibility plans

- Documented visualiser speed summary schema improvements, platform simplification status, and updated backlog tracking for completed work.

## March 5, 2026 - contributor guidance, agent prototyping & cleanup

- Added contributor personas and general work themes to [CONTRIBUTING.md](../CONTRIBUTING.md).
- Added Jess (PM) agent; renamed existing agents with role context: Hadaly→Hadaly (Dev), Ictinus→Ictinus (Architect), Malory→Malory (Pen Test), Thompson→Thompson (Writer).
- Removed tracked binaries (`gen-vrlog`, `replay-server`, `visualiser-server`), added to `.gitignore`, cleaned misplaced scripts.
- Bumped application dependency group: `go-echarts/v2`, `go-sqlite3`, `google.golang.org/grpc`, `modernc.org/sqlite`.

## March 1, 2026 - config restructure & maths refresh

- Added [`docs/plans/config-restructure-plan.md`](plans/config-restructure-plan.md) documenting the migration from flat to layer-scoped nested tuning config schema.
- Updated ML solver expansion and [velocity-coherent foreground extraction](plans/lidar-velocity-coherent-foreground-extraction-plan.md) maths documents to match new configuration and modelling direction.
- Refreshed backlog references around the config and maths workstream.

## February 28, 2026 - macOS build metadata

- Updated macOS build/release process so build metadata refreshes on every build; improved licensing display in About view; added release signing readiness tasks to backlog.

## February 27, 2026 - DMG packaging & UI polish

- Added versioned DMG export for VelocityVisualiser: automated packaging scripts, Finder-layout automation (`create-dmg`), CI wiring, and updated build/getting-started documentation.
- Second round of macOS UI polish: label taxonomy consolidation, track labelling and navigation improvements, enhanced test coverage for visualiser components.
- Bumped version to `0.5.0-pre14`.
- {dd/mac/dmg-signing} Added initial code-signing and notarisation pipeline (#425): Makefile targets for `codesign`, `notarytool`, and `stapler`, wired into the CI release workflow.

## February 26, 2026 - replay EOF debugging, build metadata cI/Test plumbing & format docs

- Added [data/structures/README.md](../data/structures/README.md) and [data/structures/VRLOG_FORMAT.md](../data/structures/VRLOG_FORMAT.md), and updated LiDAR architecture/documentation references to the new data format docs.
- Expanded macOS visualiser diagnostics while debugging replay EOF behaviour: added `DevLogger` (replacing `os.Logger`) and increased logging coverage in `AppState`, `ContentView`, `RunBrowserView`, and `RunBrowserState`.
- Simplified debug log output in `AppState` by removing privacy-attribute noise and improving message formatting clarity.
- Added detailed `VisualiserClient` replay RPC diagnostics (`seek()` / `play()` / stream restart path) including connection-state and response logging.
- Hardened replay restart handling in `AppState`: on replay completion, it tries `seek(logStartTimestamp)` + `play()` first and falls back to a full gRPC stream restart if RPCs fail or no playback RPC client is available.
- Updated Go visualiser VRLOG replay loop to pause at EOF (instead of stopping), keep the replay loaded, and reset replay pacing timestamps so restart-from-beginning via `Seek(0)` + `Play()` works cleanly.
- Extended `BuildInfo.swift` generation to macOS test runs and CI workflows (`mac-ci`, `nightly-full-ci`) so `gitSHA`/`buildTime` metadata is available consistently.
- Refined `AboutView` / `ContentView` layout and separated version/build info into clearer lines.

## February 25, 2026 - visualiser test coverage, replay completion & UI refinement

- Refactored `ContentView.swift` to extract testable helper functions: `KeyAction` enum (25 cases), `handleKeyPress()`, `assignLabelByIndex()`, standalone row/toolbar/panel views.
- Extended keyboard label shortcuts from 4 → 9 (keys 1–9 map to `ObjectClass` proto enum order: noise, dynamic, pedestrian, cyclist, bird, bus, car, truck, motorcyclist).
- Fixed label panel sync: replaced `@State lastAssignedLabel` with computed `currentLabel` derived from `appState.userLabels`, so keyboard shortcuts and button clicks both update the highlight.
- Added up/down arrow key track navigation (`selectNextTrack` / `selectPreviousTrack`) using `trackListOrder` published by `TrackListView`, so navigation follows the visible sort order (first-seen or max-speed).
- Consolidated Track Inspector and Label Panel: removed duplicate track/run ID display; header now shows full run ID in grey; label section uses "Labels" subheading instead of separate "Label Track" panel.
- Reorganised Track Inspector components and enhanced data display (3D bounding-box labels, sparkline colour coding, speed display logic).
- Added replay completion handling across Go + macOS visualiser playback: server pauses at EOF instead of closing the stream, macOS playback controls/AppState detect replay completion, and replay state tests were updated for the new behaviour.
- Added/expanded build metadata plumbing for the macOS visualiser: generated `BuildInfo.swift` during macOS builds, ignored generated `BuildInfo.swift` in git, and surfaced build information in `AboutView`.
- Improved playback/replay robustness diagnostics: enhanced `VisualiserClient` stream handling/error logging and `AppState` replay completion detection with additional tests.
- Added rendering options / UI controls for velocity arrows and updated related Swift tests.
- Refactored `RunBrowserView`, expanded UI coverage (`UICoverageBoostTests` / comprehensive view tests), and fixed `FilterBarView` layout sizing consistency issues.
- Applied Swift safety/polish fixes (#330): reduced force-unwraps, addressed actor-isolation issues, normalised whitespace, and fixed range-slider edge cases.
- Test coverage on `ContentView.swift` improved from 55.97% → 86.65% (3045/3514 lines). Total unit tests: 1032.

## February 24, 2026 - label taxonomy consolidation & UI polish

- Added `ObjectClass` protobuf enum and updated `Track` and `LabelEvent` messages to use it (replaces free-form string classification).
- SQL migrations (`000029`) to expand and normalise label vocabulary across the database.
- Go server: renamed `ClassLabel` → `ObjectClass`, added `ClassifyFeatures` method for VRLOG replay reclassification.
- SvelteKit frontend: expanded `DetectionLabel` union type, updated colours and tests.
- macOS visualiser: updated classification labels, tag pills in `TrackListView` for classification and quality indicators.
- Added classification maths specification and [label vocabulary consolidation plan](plans/label-vocabulary-consolidation-plan.md).
- Refactored playback state management: `PlaybackControlsDerivedState`, `PlaybackRPCClient` protocol, stream termination handling.
- Added `trackMaxSpeed` to `AppState` and updated `TrackHistoryGraphView` to use persistent max speed with dashed line indicator.
- Added `AboutView` with licensing information; renamed app references to VelocityReport.
- User labels caching for immediate UI feedback; playback state reset on new VRLOG replay.
- Enhanced `TimeDisplayView` with fallback for frame-index display.
- Renamed `format-markdown` Makefile target to `format-docs` for clarity.
- Completed HINT sweep polish items: TypeScript types, Continue button, carried badge, `exportLabels` removal, page subtitle.
- Completed [Python venv consolidation](plans/tooling-python-venv-consolidation-plan.md) to single root `.venv/` (PR #320).
- Added [PDF generation migration-to-Go plan](plans/pdf-go-chart-migration-plan.md) (PR #321, decision D-17).
- Completed SWEEP/HINT platform hardening and polish backlog items; updated BACKLOG.md milestones.
- Added `TestResults/` to `.gitignore` for Xcode test artifact cleanup.
- Bumped version to `0.5.0-pre13`.

## February 23, 2026 - settling evaluation, backlog alignment & vector scene map

- Implemented `settling-eval` CLI tool (`cmd/settling-eval`) for evaluating background grid convergence from PCAP files.
- Added `SettlingReport` structure and JSON reporting methods to `l3grid` package.
- Refactored settling-eval to use `l3grid.SettlingReport`, streamlining report generation.
- Added unit tests for `IsSettlingComplete`, `EvaluateSettling` zero-cells edge case, and settling eval coverage.
- Fixed race conditions in settling eval tests and VRLog seek tests (use atomic return values instead of `reader.CurrentFrame()`).
- Created [DECISIONS.md](DECISIONS.md) executive decisions register; resolved 16 design decisions with rationale.
- Deprecated ROADMAP.md in favour of [BACKLOG.md](BACKLOG.md) as single source of truth for project-wide work items.
- Moved `DESIGN.md` to [`docs/ui/DESIGN.md`](ui/DESIGN.md) and updated all cross-references.
- Reorganised BACKLOG.md milestones: added v0.5.1 for serial port configuration UI, adjusted v0.7/v0.8 item placements.
- Added 6 missing GitHub issues to BACKLOG.md (#4, #7, #8, #103, #122, #148); fixed #290→#11 reference.
- Updated [vector scene map](lidar/architecture/vector-scene-map.md) architecture: integrated OSM Simple 3D Buildings as structure priors, enhanced geometry prior service design.

## February 22, 2026 - PCAP performance, OBB heading & region-adaptive parameters

- PCAP performance hardening (PR #313, 68 files): fixed `finalizeFrame` deadlock, added `foreground_max_input_points` tuning for DBSCAN decimation.
- Used local random generator in `uniformSubsample` to avoid lock contention during concurrent DBSCAN calls.
- Prevented miss counter advancement during frame throttle in tracking pipeline.
- Updated `MaxFrameRate` to 25 in radar and tracking pipeline to prevent frame drops.
- Added size-based chunk rotation to `.vrlog` recorder; prevented overflow in frame offset/length checks in replayer.
- Refactored logging: removed debug flags, implemented structured log-level flag (`--log-level`), replaced trace logs with diagnostic logs.
- Added backoff logging to PCAP real-time processing for diagnostics.
- Added unit tests for DBSCAN subsampling, dropped frame handling, `ForegroundMaxInputPoints` config, and recorder/replayer error paths.
- OBB heading stability (PR #310): added 90° jump rejection guard with heading source tracking; removed canonical-axis normalisation.
- Standardised bbox dimensions: removed `_avg` duplication; both Swift and web visualisers use per-frame cluster dims.
- Updated protobuf schema: renamed `bbox_length_avg` → `bbox_length`, moved `heading_source` to field 35.
- Added geometry-coherent tracking maths proposal with cross-references to existing proposals.
- Implemented [region-adaptive parameters](lidar/operations/adaptive-region-parameters.md) (PR #307): per-region `ClosenessMultiplier` and `NeighborConfirmationCount` in `ProcessFramePolarWithMask`.
- Added `UpdateConfig` method to `Tracker` for runtime config updates via WebServer.
- Created [VISION.md](VISION.md) with project-wide vision statement and technical design language.
- Bumped version to 0.5.0-pre12.

## February 21, 2026 - documentation reorganisation & maths specifications

- Reorganised [docs/plans/](plans) directory (PR #308, 35 files): consolidated and restructured plan documents.
- Comprehensive LiDAR documentation overhaul (PR #295, 90 files): created [velocity-coherent foreground extraction](plans/lidar-velocity-coherent-foreground-extraction-plan.md) maths spec, added status fields to docs, created getting started guide and config parameter tuning guide.
- Moved maths proposals to [data/maths/proposals/](../data/maths/proposals) with consistent naming convention.
- Added [documentation standardisation plan](plans/platform-documentation-standardisation-plan.md) with metadata and validation gates.
- Updated ROADMAP.md (PR #312): aligned roadmap entries with BACKLOG.md priorities.
- Added macOS process profiling script (`macos_profile_lidar.sh`) and corresponding Makefile targets.

## February 20, 2026 - design plans, test coverage & CI improvements

- Created [platform simplification and deprecation plan](plans/platform-simplification-and-deprecation-plan.md) (PR #300): deprecation signalling, deploy retirement gate, migration task list.
- Created [quality coverage improvement plan](plans/platform-quality-coverage-improvement-plan.md) (PR #294): target 95.5% across all components, exclude `cmd/`, add logic extraction strategy and macOS Swift app.
- Created LiDAR [multi-model ingestion and configuration](lidar/architecture/multi-model-ingestion-and-configuration.md) architecture proposal (PR #303): sensor auto-detection, DB-driven config.
- Created LiDAR [network configuration](lidar/architecture/network-configuration.md) design document (PR #302): interface selection, network diagnostics, hot-reload UDP listener binding.
- Created visualiser algorithm diffing design document (PR #299): visual comparison of algorithmic outputs.
- CI performance improvements (PR #292): streamlined macOS CI, added nightly full CI, removed deprecated performance regression test.
- Refactored test setup: replaced direct file handling with template DB cloning; simplified schema consistency checks.
- Expanded Go test coverage (PR #289): added unit tests for debug logging, tracking pipeline, SQLite analysis run comparison, PCAP handling, SSE streaming, recorder error handling, track observations, labelling progress.
- Added OBB serialisation to `frameBundleToProto` with tests.
- Added lifecycle parameter sweep scripts and kirk0 configuration permutations for tuning.
- Fixed out-of-bounds errors and tuned tracking parameters (PR #293).
- Updated [LiDAR data layer model](lidar/architecture/LIDAR_ARCHITECTURE.md) documentation with visualisation details.
- {copilot/update-serial-configuration-ui} Added serial configuration backend (#290): DB layer with `serial_configs` table, API CRUD handlers, and a reload manager that hot-swaps serial port bindings without restarting the service.
- {copilot/update-serial-configuration-ui} Added serial configuration web UI: settings page with create/edit/delete dialogs, port test dialog, and `uniquePortPaths` deduplication.
- {copilot/update-serial-configuration-ui} Expanded Go test coverage for serial config reload and added Jest threshold enforcement for the web frontend.

## February 19, 2026 - LiDAR layer alignment & architecture review

- Implemented LiDAR 6-layer alignment refactor: split `l3grid/background.go` into persistence, export, and drift files.
- Split `monitor/webserver.go` into data-source and playback handler files.
- Extracted domain comparison logic from `storage/sqlite` into `l6objects`.
- Added HTTP method prefixes and middleware wrappers to route tables.
- Fixed route conflict panic from duplicate route registrations.
- Consolidated architecture docs, created [BACKLOG.md](BACKLOG.md) for deferred work items.
- Documented further opportunities to reduce library size and complexity.
- Completed review items 11–14 and P1–P3 from LiDAR layer alignment review.
- Removed redundant method-not-allowed tests (mux handles 405 via method-prefixed routes).
- Migrated debug logging from `Debugf` to structured levels: `Opsf`, `Diagf`, `Tracef`.
- Added shared colour palette module and CSS standard utility classes for frontend components.
- Added [vector scene map](lidar/architecture/vector-scene-map.md) and 3D ground plane architecture documentation.
- Added mathematical specifications and standardised config key ordering.
- Added database naming standardisation guide and pull-request template.

## February 18, 2026 - design system & teX/Chart updates

- Created [`DESIGN.md`](ui/DESIGN.md) with project-wide design principles and frontend design language.
- Conducted comprehensive design review against `DESIGN.md`, producing improvement plan.
- Updated Python PDF generator TeX configuration for minimal precompiled install.
- Enhanced Svelte chart components (RadarOverviewChart).
- Updated Go CI integration tests to use minimal TeX tree (`build-texlive-minimal`) instead of full TeX Live install.
- Trimmed CI TeX Live packages: dropped `texlive-fonts-extra` (~500 MB) and `latexmk`; added `--no-install-recommends`.

## February 15-16, 2026 - HINT tuning system & LiDAR track improvements

- Implemented Human-Involved Numerical Tuning ([HINT](plans/lidar-sweep-hint-mode-plan.md)) system (renamed from RLHF).
- Replaced HTTP client with in-process `DirectBackend` for sweep runner, eliminating HTTP overhead.
- Added long-polling for HINT status and PCAP completion.
- Refactored label taxonomy: `good_vehicle`/`good_pedestrian` → `car`/`ped`; added `impossible` label.
- Implemented suspend and resume functionality for auto-tune sweeps with checkpoint persistence.
- Added per-frame OBB dimensions and improved heading handling in LiDAR tracking.
- Enhanced cluster size filtering, track pruning, and classification updates.
- Serialised frame callback processing in `frame_builder.go` to prevent data races.
- Added gap detection in `MapPane.svelte` to prevent spaghetti lines in track visualisations.
- Added height band filtering parameters to tuning configuration.
- macOS visualiser: added ground reference grid toggle, background grid points toggle, track filtering with dual-handle range slider.
- Refactored all default parameters to load from `tuning.defaults.json` instead of hardcoded values.
- Bumped version to 0.5.0-pre8 and 0.5.0-pre9.
- Created [LiDAR 6-layer data model](lidar/architecture/LIDAR_ARCHITECTURE.md) documentation (OSI-style).
- Added [LiDAR labelling QC enhancements](plans/lidar-visualiser-labelling-qc-enhancements-overview-plan.md) plan.
- Created LiDAR refactor plans for package restructuring.

## February 14, 2026 - sweep schema fixes & documentation updates

- Fixed sweep parameter schema and config compatibility issues from PR review.
- Updated documentation to reflect current code state: corrected Makefile target count (59→101), Go version (1.21+→1.25+), SQLite version (3.51.2), Python version (3.11+).
- Fixed broken doc links and wrong paths in [TROUBLESHOOTING.md](../TROUBLESHOOTING.md).
- Expanded repo structure in `copilot-instructions.md` (5→15 internal packages).
- Fixed setup guide frontmatter cost and typos.
- Dependency update: bumped `markdown-it` 14.1.0→14.1.1 in docs site.

## February 12, 2026 - precompiled LaTeX plan & test expansion

- Created design document for [precompiled LaTeX `.fmt`](plans/pdf-latex-precompiled-format-plan.md) support in PDF generator.
- Expanded test coverage across Go, Python, and macOS components.

## February 11, 2026 - RLHF score explainability & label provenance

- Implemented `RLHFTuner` engine with RLHF API endpoints and handler tests.
- Added RLHF mode to sweep dashboard UI and Svelte sweeps page.
- Implemented score component breakdown: `ScoreComponents`, `ScoreExplanation`, and `/explain` API endpoint.
- Added class/time coverage gates for RLHF continue validation.
- Implemented `label_source` provenance tracking with IoU-based confidence.
- Added schema/version stamp fields in sweep persistence (migration 9.1).
- Fixed VRLog seek race condition; fixed int64 overflow in temporal spread calculation.
- Boosted RLHF test coverage to 91.9%; web test coverage to 97%.
- Created RLHF expansion plan with ML solver-inspired optimisation approach.
- Designed [`velocity.report-imager`](plans/deploy-rpi-imager-fork-plan.md) (RPi Imager fork) for simplified deployment.
- Added comprehensive Swift tests for macOS visualiser.
- Expanded Go test coverage: Runner, sweep, tracking API, UDP listener, tracking pipeline.
- Simplified packet handling by directly using polar points in `FrameBuilder`.
- Bumped version to 0.5.0-pre7.

## February 10-11, 2026 - vRLog replay & track labelling

- Implemented `.vrlog` recording and replay for track labelling workflow.
- Added `FrameRecorder` interface, `vrlog_path` field, and playback API endpoints.
- Implemented VRLOG replay in visualiser publisher and gRPC control delegation.
- macOS visualiser: run browser, run-track labelling support, side panel for track selection.
- Added VRLOG safe directory configuration with absolute path validation.
- Implemented background snapshot sending during VRLOG replay.
- Enhanced tuning parameters and acceptance metrics tracking in background processing.
- Added label taxonomy and classification details for detection and quality in LiDAR terminology.
- Bumped version to 0.5.0-pre6.

## February 9-10, 2026 - label-aware auto-tuning & LiDAR refinement

- Implemented track label-aware auto-tuning: scoring incorporates human labels.
- Enhanced LiDAR tuning refinement with updated parameters and thresholds.
- Added PCAP analysis improvements for tune benchmarks.
- Expanded Go unit test coverage across multiple packages.
- Created [frontend consolidation](plans/web-frontend-consolidation-plan.md) design document.
- Designed Swift label plan for macOS visualiser track labelling workflow.

## February 8, 2026 - documentation audit & roadmap

- Comprehensive audit of all 29 LiDAR documentation files against actual codebase.
- Identified 12 discrepancies between docs and implementation status.
- Updated README, devlog, and 9 other documentation files with current status.
- Produced consolidated LiDAR roadmap with prioritised future work (P0–P3).
- Cross-referenced approved track labelling design document across relevant docs.
- Implemented auto-tuning system (`sweep/auto.go`): iterative grid narrowing, multi-objective scoring.
- Enhanced sweep dashboard: two new heatmaps (tracks, alignment), PARAM_SCHEMA with sane defaults.
- Increased chart height from 300px to 450px, changed grid layout from 6 to 3 columns.
- Fixed PCAP replay methods to use full file path for `pcap_file` parameter.
- Created design document for [track labelling, ground truth evaluation, and label-aware auto-tuning](plans/lidar-track-labelling-auto-aware-tuning-plan.md) (8 phases).
- Created [dynamic algorithm selection](plans/lidar-architecture-dynamic-algorithm-selection-plan.md) design spec for LiDAR foreground extraction.
- Designed `lidar_transits` table schema for dashboard/report integration.
- Identified label API route gap: CRUD handlers exist but routes not registered.

## February 7, 2026 - param sweep dashboard & auto-tune mode

- Consolidated LiDAR configuration into single config struct with fluent setters.
- Implemented parameter sweep dashboard with ECharts bar charts and results table.
- Added auto-tuning mode toggle and recommendation card to sweep dashboard.
- Implemented settle mode support in sweep runner: `once` and `per_combo`.
- Added iteration validation and clamping in `Sample` method.
- Created sweep sampler, scoring, and chart data preparation utilities.

## February 5-6, 2026 - macOS visualiser M3.5–M7

- M3.5: Split streaming: background snapshots every 30s + foreground-only per-frame (78→3 Mbps).
- M4: Tracking interface refactor: `TrackerInterface`, `ClustererInterface`, golden replay tests.
- M5: Algorithm upgrades: Hungarian association, ground removal, OBB estimation, occlusion coasting.
- M6: Debug overlays: gating ellipses, association lines, residuals via gRPC; label API handlers.
- M7: Performance hardening: Swift buffer pooling, PointCloudFrame reference counting, frame skip cooldown.
- Added [LiDAR settling time optimisation](lidar/operations/settling-time-optimisation.md) design document.

## February 3-4, 2026 - macOS visualiser M0–M3

- Designed macOS visualiser architecture (SwiftUI + Metal + gRPC).
- M0: Schema + synthetic: protobuf schema, gRPC server, synthetic data generator, SwiftUI+Metal renderer.
- M1: Recorder/replayer: `.vrlog` format, seek/pause/rate control, deterministic playback.
- M2: Real point clouds: `FrameAdapter`, decimation modes, 70k+ points at 30fps.
- M3: Canonical model: `FrameBundle` as single source of truth, LidarView + gRPC from same model.
- Added app icon assets.

## February 2, 2026 - map feature & CI refactor

- Added interactive map component for site visualisation.
- Refactored CI pipeline with end-to-end test support.
- Dependency updates across application group.

## February 1, 2026 - documentation homepage

- Updated homepage spacing and added hamburger menu.
- Refined documentation site styling.

## January 31, 2026 - dependency injection & test coverage

- Implemented dependency injection interfaces: `CommandExecutor`, `UDPSocket`, `PCAPReader`, `DataSourceManager`.
- Refactored `UDPListener` to use `SocketFactory` for testability.
- Created `RealDataSourceManager` wrapping WebServer operations.
- Added sweep sampler for parameter sweep iterations with CSV output.
- Added `BackgroundFlusher` for periodic persistence with mock implementations.
- Added `BackgroundConfig` with validation and fluent setters.
- Added chart data preparation utilities for LiDAR visualisation.
- Increased Go test coverage to 85.9% on critical packages.
- Restructured documentation paths to use `public_html`.
- Added JavaScript unit tests for `TRACK_COLORS` in LiDAR types.
- Extended WebServer and BackgroundManager test coverage.
- Bumped version to 0.4.2.

## January 30, 2026 - test coverage expansion

- Increased Go test coverage from ~38% to 73.1% on testable packages.
- Added comprehensive tests: monitoring, serialmux factory/mock, LiDAR arena, quality scoring.
- Added admin routes integration tests and serialmux extended tests.
- Enhanced tracking tests: `GetAllTracks`, speed history, quality metrics, spatial coverage.
- Added CONTRIBUTING guide.
- Fixed CI version check scripts and changelog links.

## January 29, 2026 - release v0.4.0 & setup guide

- Released version 0.4.0 with comparison reports, site config, and transit worker.
- Created comprehensive setup guide for Citizen Radar deployment.
- Added code coverage badges (Go, Python, Web) with Codecov integration.
- Added coverage documentation and CI workflow updates.
- Enhanced documentation site: dark mode, Tailwind v4 CSS, KaTeX math rendering.
- Fixed documentation CI to use pnpm instead of yarn.

## January 25, 2026 - radar config SCD & comparison reports

- Implemented site config periods with cosine correction (Type 6 SCD pattern).
- Added boundary hour filtering to `RadarObjectRollupRange` for improved data accuracy.
- Added histogram aggregation with configurable `max_bucket`.
- Enhanced PDF report generation: detailed data tables, velocity unit support, percentile lines.
- Added `compare_source` to `ReportRequest` for dual-source comparison.
- Added `min_speed_used` to `RadarStatsResult` API responses.
- Implemented save/load report settings from local storage.
- Schema ordering fix: foreign key dependency-aware table creation.
- Added security validation for PDF path (path traversal prevention).
- Bumped version to 0.4.0-pre9.

## January 20, 2026 - transit worker inspector

- Added full-history run capability to transit worker and API.
- Implemented transit CLI: `analyse`, `delete`, `migrate`, `rebuild` commands.
- Added transit deduplication plan and tests.
- Enhanced transit worker UI with run management features.
- Updated model version default from `rebuild-full` to `hourly-cron`.
- Bumped version to 0.0.4-pre8.

## January 19, 2026 - comparison report generator

- Added comparison-period report generation with dual-period outputs (T1/T2).
- Enhanced PDF report with comparison metrics and improved labelling.
- Refactored `chart_builder` to use integer indices for x-axis.
- Added PDF generator version tracking in `set-version` script.
- Bumped version to 0.0.4-pre7.

## January 17, 2026 - track visualisation UI fixes

- Fixed click detection to check track history, not just head position.
- Filtered (0,0) noise points from rendering (backend and frontend).
- Added timestamp sorting for coherent track history lines.
- Progressive track reveal during playback (point-by-point as timeline advances).
- Added pagination (50 tracks/page) with navigation controls to TrackList component.
- Added "Min Observations" filter (1+/5+/10+/20+/50+) to filter noise tracks.
- Fixed timeline sync with pagination (TimelinePane shows paginated subset via `onPaginatedTracksChange` callback).
- Fixed label truncation (removed `.slice(-6)`, increased sidebar width to 500px, left margin to 120px).
- Increased `HitsToConfirm` from 3 to 5 (tracks require 5 consecutive observations before confirmation).
- Added physical plausibility checks: `MaxReasonableSpeedMps=30.0`, `MaxPositionJumpMeters=5.0` in Mahalanobis gating.
- Increased API limit from 100 to 1000 tracks (`getTrackHistory` default=500).

## January 14-16, 2026 - adaptive region segmentation

- Implemented adaptive region segmentation in BackgroundGrid for distance-aware thresholds.
- Added `RegionParams` struct with configurable `ThresholdMultiplier` and `WarmupFrames` per region.
- Default regions: near (0-30m, 1.0x), mid (30-60m, 1.2x), far (60-100m, 1.5x), extended (100m+, 2.0x).
- Fixed `WarmupFramesRemaining` initialisation logic (reset on grid clear, decrement per frame).
- Added region identification to `ProcessFramePolarWithMask` for per-point threshold scaling.
- Added region dashboard HTML template embedded in Go webserver via `html/template`.
- Fixed JSON serialisation for region parameters in API responses.
- Aggressively tuned thresholds to achieve <30 false positives target (from 150+).
- Fixed potential XSS vulnerability (code scanning alert #33) with HTML escaping.

## January 13, 2026 - warmup trails fix

- Fixed false positive foreground classifications during grid warmup (~100 frames).
- Implemented dynamic threshold multiplier: 4x at count=0, tapering to 1x at count=100.
- Fixed `recFg` accumulation during cell freeze (reset to 0 on thaw).
- Added track quality metrics for investigation.
- Enhanced grid plotter visualisation for debugging.

## January 11, 2026 - PCAP analyse benchmarking

- Extended pcap-analyse tool with performance benchmarking capabilities.
- Added CI pipeline integration for automated benchmark runs.
- Improved frame processing performance metrics.

## January 9, 2026 - test coverage expansion

- Added comprehensive unit tests across LiDAR pipeline (~3,363 lines of new test code).
- Improved coverage for background processing, clustering, and tracking modules.
- Added edge case testing for frame builder and parser.

## January 6, 2026 - foreground forwarding & debugging

- Implemented foreground point forwarding to UDP port 2370 for downstream processing.
- Added PCAP realtime replay mode for development/testing.
- Created parameter tuning UI for runtime adjustment of background model settings.
- Added AV Range Image Format Alignment architecture document (dual return handling).
- Fixed build failure with stub generation for pcap-disabled builds.
- Applied security fixes for path validation.

## January 3, 2026 - DB stats API

- Added `GET /api/db/stats` endpoint for database size and table statistics.
- Implemented path traversal security hardening in file operations.
- Added duplicate snapshot cleanup utility.

## January 2, 2026 - LiDAR documentation updates

- Restructured LiDAR documentation for improved navigation.
- Added velocity-coherent foreground extraction design document (1,456 lines).
- Merged in upstream dependency updates for documentation and application packages.

## December 28, 2025 - LiDAR alignment planning

- Created comprehensive alignment planning document for multi-sensor fusion.
- Documented clustering algorithms comparison (DBSCAN vs HDBSCAN).
- Added occlusion handling strategies and static pose alignment procedures.
- Designed motion capture architecture for dynamic calibration.

## December 16, 2025 - AV-LIDAR integration plan

- Added comprehensive integration plan for LIDAR frame analyser (913 lines).
- Documented frame data flow from sensor to classification pipeline.
- Specified integration points with AV perception stack.

## December 10, 2025 - LiDAR track visualisation UI

- Implemented `MapPane.svelte` canvas component for real-time track rendering.
- Added `TimelinePane.svelte` with D3-based SVG timeline and playback controls.
- Created `TrackList.svelte` sidebar with track selection and filtering.
- Added background grid overlay API for spatial visualisation.
- Integrated WebSocket updates for live track streaming.

## December 9, 2025 - background grid standards & PCAP split

- Documented LiDAR background grid export standards and format options.
- Added pcap-split tool design specification (1,094 lines) for PCAP file segmentation.
- Defined foreground research export formats for offline benchmarking and ML work.

## December 2, 2025 - CLI architecture guide

- Created CLI comprehensive guide with long-term architecture vision (1,715 lines).
- Documented command structure, flag conventions, and extension patterns.
- Added LiDAR foreground tracking API implementation notes (Phases 2.9-3.6).

## December 1, 2025 - release 0.3.0 & infrastructure

- Implemented `velocity-deploy` deployment manager (install, upgrade, rollback, backup, health commands).
- Added hourly transit worker job with UI toggle for background processing.
- Created `scan_transits` backfill tool for historical data processing.
- Baselined all components to version 0.2.0, then bumped to 0.3.0.
- Reorganised SQL migrations, inserted original schema as 000001.
- Added `set-version.sh` utility for cross-codebase version updates.
- Created time-partitioned data tables specification (2,980 lines).
- Added speed limit schedules feature spec (school zones, time-based limits).
- Completed security review of data partitioning plan (CVE fixes for path traversal, SQL injection, race conditions).

## December 1, 2025 - phase 3.7 analysis run infrastructure

- Implemented `AnalysisRun` type for versioned parameter configurations with `params_json` storage.
- Added `RunParams` type capturing all configurable parameters (background, clustering, tracking, classification).
- Created `RunTrack` type extending track data with user labels and quality flags for ML training.
- Implemented `AnalysisRunStore` with database operations: `InsertRun()`, `CompleteRun()`, `GetRun()`, `ListRuns()`.
- Added track management: `InsertRunTrack()`, `GetRunTracks()`, `UpdateTrackLabel()`.
- Created labelling progress API: `GetLabelingProgress()`, `GetUnlabeledTracks()`.
- Added split/merge detection types: `RunComparison`, `TrackSplit`, `TrackMerge` for run comparison.
- Renumbered phases: 4.0→3.7 (Analysis Run), 4.1→4.0 (Labelling UI), 4.2→4.1 (ML Training).

## December 1, 2025 - ML pipeline roadmap

- Created comprehensive ML Pipeline Roadmap documentation (now tracked in [docs/lidar/operations/track-labelling-ui-implementation.md](lidar/operations/track-labelling-ui-implementation.md) and related LiDAR plan docs).
- Planned Phase 4.0 Track Labelling UI: SvelteKit routes, track browser, trajectory viewer, labelling panel.
- Planned Phase 4.1 ML Classifier Training: feature extraction, Python training pipeline, Go model deployment.
- Planned Phase 4.2 Parameter Tuning: grid search, quality metrics, objective function optimisation.
- Recommended implementation order: ✅3.7 → 4.0 → 4.2 (parallel) → 4.1 → 4.3.

## December 1, 2025 - phase 3.6 PCAP analysis tool

- Implemented `cmd/tools/pcap-analyze/main.go` for batch PCAP processing through full tracking pipeline.
- Full pipeline: parse UDP → build frames → background subtraction → clustering → tracking → classification.
- Added output formats: JSON (complete results), CSV (track table), binary foreground research blobs.
- Computed track speed-summary features from speed history.
- Added `SpeedHistory()` getter to `TrackedObject` for external speed-summary computation.
- Added `GetAllTracks()` method to `Tracker` for retrieving all tracks including deleted.
- Added build tag `pcap` to `integration_test.go` for conditional test execution.

## November 30, 2025 - pose simplification

- Removed `internal/lidar/pose.go` and `internal/lidar/pose_test.go` (deferred to future phase).
- Removed pose-related fields from `ForegroundFrame`, `TrainingFrameMetadata`, `TrainingDataFilter`, `TrackObservation`, `WorldCluster`, `TrackSummary`.
- Updated SQL schemas to remove `pose_id` columns from `lidar_clusters`, `lidar_tracks`, `lidar_track_obs`.
- Updated `track_store.go` and `track_api.go` to use simplified signatures.
- Research export data stored in polar (sensor) frame, which is pose-independent.

## November 30, 2025 - phase 3.5 track/Cluster REST API

- Implemented `TrackAPI` struct in `internal/lidar/monitor/track_api.go`.
- Added endpoints: `GET /api/lidar/tracks`, `GET /api/lidar/tracks/active`, `GET /api/lidar/tracks/{id}`.
- Added endpoints: `PUT /api/lidar/tracks/{id}`, `GET /api/lidar/tracks/{id}/observations`.
- Added endpoints: `GET /api/lidar/tracks/summary`, `GET /api/lidar/clusters`.
- Created JSON response structures: `TrackResponse`, `ClusterResponse`, `TracksListResponse`, `TrackSummaryResponse`.
- Supports both in-memory tracker (real-time) and database queries.

## November 30, 2025 - phases 3.3-3.4 SQL schema & classification

- Created migration `000009_create_lidar_tracks.up.sql` with `lidar_clusters`, `lidar_tracks`, `lidar_track_obs` tables.
- Implemented persistence in `track_store.go`: `InsertCluster()`, `InsertTrack()`, `UpdateTrack()`, `InsertTrackObservation()`.
- Added queries: `GetActiveTracks()`, `GetTrackObservations()`, `GetRecentClusters()`.
- Implemented rule-based classification in `classification.go` with object classes: pedestrian, car, bird, other.
- Added speed-history summary computation for track classification features.
- Added `ObjectClass`, `ObjectConfidence`, `ClassificationModel` fields to `TrackedObject`.

## November 30, 2025 - phases 2.9-3.2 foreground tracking pipeline

- Phase 2.9: Implemented `ProcessFramePolarWithMask()` for per-point foreground/background classification.
- Phase 3.0: Added `WorldPoint` struct and `TransformToWorld()` with pose support.
- Phase 3.1: Implemented `SpatialIndex` with Szudzik pairing, `DBSCAN()` with eps=0.6m, minPts=12.
- Phase 3.2: Implemented Kalman tracking with `TrackedObject`, `Tracker`, Mahalanobis gating.
- Added track lifecycle: Tentative → Confirmed → Deleted with hits/misses counting.
- Added classification research export support: `ForegroundFrame`, `EncodeForegroundBlob()`/`DecodeForegroundBlob()`.

## November 29, 2025 - distribution & tracking plans

- Created distribution and packaging plan document (1,636 lines).
- Created LIDAR foreground extraction and tracking implementation plan v2 with polar/world frame separation.
- Documented Szudzik pairing function for spatial indexing.

## November 19, 2025 - database migration system

- Implemented database migration system using golang-migrate.
- Created 12 migration files covering schema evolution.
- Integrated migration CLI commands into main binary.

## November 14, 2025 - migration system design

- Added database migration system design document.
- Evaluated golang-migrate vs custom solution.
- Documented migration file conventions and versioning strategy.

## November 7, 2025 - LaTeX security fix

- Fixed LaTeX injection vulnerability (CVE-9.8 severity) with `escape_latex()` function.
- Sanitised all user inputs before PDF generation.
- Fixed JavaScript download bug in report generation.

## November 6, 2025 - Python venv consolidation & AI agents

- Consolidated Python virtual environments to single repository root `.venv/`.
- Added Malory (security) and Thompson (communications) custom AI agents.
- Merged multiple dependabot dependency updates.

## November 5, 2025 - docs restructure & path security

- Restructured Eleventy documentation site with syntax highlighting, typography plugin, breadcrumbs.
- Added community page to documentation.
- Implemented path validation to prevent traversal attacks in file operations.

## November 4, 2025 - build metadata & PCAP API

- Added GIT SHA and build time to HTML meta tags via `set-build-env.js`.
- Moved PCAP/live data source flag from CLI to runtime API (`POST /api/lidar/source`).
- Updated Makefile naming conventions (162+ line changes).

## November 1, 2025 - PCAP security & grid visualisation

- Implemented path traversal protection with `--lidar-pcap-dir` flag using `filepath.Join()` + `filepath.Abs()` + prefix checking.
- Added file validation: regular files only, `.pcap`/`.pcapng` extensions required, 403 Forbidden for path escape.
- Added systemd integration: service auto-creates PCAP directory via `ExecStartPre`.
- Enhanced 4K-optimised dashboard (25.6×14.4" @ 150 DPI): 3 polar/spatial charts + 4 stacked metric panels.
- Added PCAP snapshot mode with configurable interval/duration, auto-numbered directories, metadata JSON.
- Created API helper scripts: grid reset, PCAP replay, background status fetching.
- Added Makefile targets for noise sweep/multisweep plotting.
- Added Python plotting tools: polar/cartesian heatmaps with live and PCAP replay modes.
- Consolidated DEBUG-LOGGING-PLAN, GRID-ANALYSIS-PLAN, GRID-HEATMAP-API, LIDAR-PCAP-Debug docs into sidecar overview.

## October 31, 2025 - grid analysis API & debug logging

- Added `GET /api/lidar/grid_heatmap` endpoint for spatial bucket aggregation (40 rings × 120 azimuth buckets).
- Implemented `GetGridHeatmap()` with configurable bucket size and settled threshold.
- Response includes summary stats and per-bucket metrics: fill/settle rates, mean range/times seen, frozen cells.
- Added Python plotting tools: polar (ring vs azimuth) and cartesian (X-Y) heatmaps.
- Created noise analysis scripts: `plot_noise_sweep.py`, `plot_noise_buckets.py`.
- Added comprehensive logging: grid reset timing, API call logs, rate-limited population tracking.
- Enhanced FrameBuilder diagnostics: eviction logging, frame callback, improved azimuth wrap detection.
- Re-enabled `SeedFromFirstObservation` with `--lidar-seed-from-first` flag.
- Added settle time flag, configurable background flush interval and frame buffer timeout.
- Added Makefile targets: dev-go, log-go-tail, log-go-cat, dev-go-pcap.
- Fixed frame eviction callback delivery bug.

## October 30, 2025 - PCAP debugging & development tools

- Enhanced frame eviction logging and finalised frame callback delivery path.
- Added diagnostics for non-zero channel counts in ParsePacket.
- Improved azimuth wrap detection for large negative jumps (>180°).
- Added --debug flag for frame completion and PCAP parsing logs.
- Created local API helper scripts for PCAP replay and background status.
- Consolidated dev-go logic into reusable run_dev_go function in Makefile.
- Added log-go-cat and log-go-tail targets.
- Corrected log directory name in .gitignore.

## October 29, 2025 - configuration & documentation cleanup

- Updated lidar configuration flags for clarity and consistency.
- Enhanced documentation for database path and command flags.
- Added `SeedFromFirstObservation` parameter for PCAP mode background initialisation.
- Removed outdated Frontend Units Override Feature documentation.

## October 28, 2025 - PCAP support foundation

- Added PCAP file replay support with BPF filtering for multi-sensor files.
- Integrated with existing parser and frame builder for seamless replay.
- Added background persistence during PCAP replay with configurable flush intervals.
- Added `--lidar-pcap-mode` flag to disable UDP listening for replay-only mode.
- Added `POST /api/lidar/pcap/start` endpoint for triggering PCAP replay via API.
- Updated LiDAR sidecar overview with classification, filtering, and metrics implementation details.

## October 27, 2025 - formatting & linting

- Added formatting and linting commands to Makefile.

## October 23, 2025 - sites, timezones & JavaScript tests

- Updated file prefix conventions for reports.
- Added timezone handling for sites.
- Added JavaScript test suite with CI integration.

## October 21, 2025 - Vite security update

- Bumped vite from 7.1.5 to 7.1.11 for security fix.

## October 14, 2025 - PDF generator cleanup

- Cleaned up PDF generator code and structure.

## October 13, 2025 - report templates & tests

- Tweaked LaTeX report templates.
- Added report generation tests.

## October 3, 2025 - PDF report initialisation

- Initialised PDF report generation with LaTeX templates (report.pdf).

## September 27, 2025 - background parameters & multisweep

- Added configurable parameters: `closeness_multiplier`, `neighbor_confirmation_count`.
- Created multisweep tool for parameter exploration.
- Fixed export destination to use `os.TempDir()`.

## September 23, 2025 - background diagnostics & monitor APIs

- Centralised runtime diagnostics with [internal/monitoring](../internal/monitoring) logger and per-manager `EnableDiagnostics` flag.
- Added BackgroundManager helpers: `SetNoiseRelativeFraction`, `SetEnableDiagnostics`, `GetAcceptanceMetrics`, `ResetAcceptanceMetrics`, `GridStatus`, `ResetGrid`.
- Added monitor API endpoints: `GET/POST /api/lidar/params`, `GET /api/lidar/acceptance`, `POST /api/lidar/acceptance/reset`, `GET /api/lidar/grid_status`, `POST /api/lidar/grid_reset`.
- Created `cmd/bg-sweep` CLI: incremental & settle modes, per-noise grid reset, live bucket discovery, CSV output.

## September 22, 2025 - background model fixes & snapshot export

- Wired BackgroundManager into LiDAR pipeline with self-contained snapshots.
- Persisted per-ring elevation angles (`ring_elevations_json`) with each `lidar_bg_snapshot`.
- Centralised snapshot-to-ASC export with elevation fallbacks.
- Added backfill tool for populating `ring_elevations_json` in existing snapshots.
- Improved `ProcessFramePolar`: restrict neighbour confirmation to same-ring, update spread EMA relative to previous mean.
- Fixed concurrent SQLite update pattern to avoid SQLITE_BUSY.

## September 21, 2025 - server & serialMux consolidation

- Centralised HTTP server and UI paths into [internal/api](../internal/api).
- Standardised on single SQLite DB (`sensor_data.db`) in [internal/db](../internal/db).
- Added LiDAR background snapshot persistence with manual HTTP trigger.
- Added `--disable-radar` flag and robust `DisabledSerialMux`.
- Merged duplicate LiDAR webservers; canonical monitor accepts injected `*db.DB` and `SensorID`.
- Moved radar event handlers to [internal/serialmux/handlers.go](../internal/serialmux/handlers.go), classification to `parse.go`.
- Added unit tests for serialmux (DisabledSerialMux, classification, config parsing, event handlers).

## September 20, 2025 - snapshot & persistence improvements

- Hardened BackgroundGrid persistence with RW-mutexes and copy-under-read.
- Added DB access for snapshots via GetLatestBgSnapshot helper.
- Added monitor endpoint to fetch, gunzip/gob-decode and summarise stored snapshots.
- Moved manual persist endpoint into lidar monitor webserver.

## September 19, 2025 - backgroundManager & polar processing

- Introduced BackgroundManager registry with `NewBackgroundManager` constructor.
- Added managers discoverable via `GetBackgroundManager`/`RegisterBackgroundManager`.
- Implemented snapshot serialisation (gob + gzip) with `Persist` method.
- Added `InsertBgSnapshot` in lidar DB layer.
- Implemented `ProcessFramePolar`: bin by ring/azimuth, EMA updates, neighbour-confirmation, freezing heuristics.

## September 18, 2025 - polar-first refactor

- Centralised spherical→Cartesian math into `transform.go` helper.
- Introduced `PointPolar` type; parser now emits polar-first.
- Added `FrameBuilder.AddPointsPolar([]PointPolar)`, removed legacy `AddPoints([]Point)`.
- UDP listener forwards polar points directly.

## September 17, 2025 - background model & transform design

- Designed sensor-frame background model (ring × azimuth) for foreground masking.
- Two-level settling per cell: fast noise settling, slow parked-object settling.
- Designed BackgroundGrid snapshot persistence and warm-start on load.
- Planned spherical→Cartesian refactor with polar/cartesian point type split.
- Planned world-grid (height-map / ground estimate) on masked Cartesian points.

## September 15, 2025 - velocity graph

- Added velocity graph component to web frontend.

## September 13, 2025 - LiDAR frame parsing & test improvements

- Implemented LiDAR packet parsing into complete 360° frames.
- Added units, velocity, and timezone configuration support.
- Eliminated implementation dependencies in parse tests with local test constants.
- Fixed boundary conditions in PCAP extraction loop bounds.
- Streamlined extractUDPPayloads by removing redundant conditional checks.

## September 12, 2025 - frame builder tests & time-based detection

- Fixed 3 previously failing frame builder tests with realistic production data patterns.
- Moved PCAP integration test to [internal/lidar/integration_test.go](../internal/lidar/integration_test.go).
- Created `internal/lidar/testdata/` directory following Go conventions.
- Increased test point counts to 60,000 points matching production.
- Implemented hybrid frame detection: time-based primary with azimuth validation.
- Integrated motor speed extraction from packet tail (bytes 8-9).
- Added dynamic frame duration based on actual RPM (50ms at 1200 RPM, 100ms at 600 RPM).
- Added --sensor-name flag for flexible deployment.
- Enhanced code documentation in extract.go with packet structure details.

## September 11, 2025 - memory optimisation & frame rate fixes

- Analysed Hesai Pandar40P UDP packet structure via Wireshark.
- Discovered Ethernet tail issue: extra 4 bytes appended to UDP packets.
- Fixed tail offset from last 6 bytes to last 10 bytes.
- Validated correct UDP sequence extraction and point parsing.
- Confirmed proper frame characteristics: ~69,000 points per frame, ~100ms duration.

## September 8, 2025 - LiDAR parser initialisation

- Initialised LiDAR parser for Hesai Pandar40P protocol.
- Added UDP listener for sensor data ingestion.
- Created basic packet parsing structure.

## September 5, 2025 - telraam integration

- Added Python tool to fetch Telraam traffic counting data.

## September 2, 2025 - uniFi protect integration

- Added Python tool to fetch UniFi Protect camera data.

## August 27, 2025 - production web assets

- Fixed and bundled production web assets in Go binary.

## August 26, 2025 - frontend integration & middleware

- Integrated Svelte frontend with Go backend server.
- Added Flush method to loggingResponseWriter for proper streaming.
- Moved DB schema declaration to `.sql` file.
- Fixed RadarObjects query and migration.
- Added VSCode settings for development.

## August 25, 2025 - Svelte dashboard

- Created first dashboard slices with Svelte, svelte-ux, layerchart.
- Fixed Svelte theme configuration.
- Added Tailwind CSS styling.

## August 21, 2025 - favicon & logging middleware

- Added favicon serving.
- Implemented LoggingMiddleware with colour-coded HTTP status codes.

## August 20, 2025 - radar stats API

- Added `/api/radar_stats` endpoint for aggregated radar statistics.

## July 23, 2025 - documentation merge

- Merged gh-pages branch into main for unified documentation.

## July 10, 2025 - README enhancement

- Added ASCII art logo to README.

## June 28, 2025 - code of conduct

- Added CODE_OF_CONDUCT.md for community guidelines.

## June 27, 2025 - radarObject parsing & project structure

- Rebased and implemented RadarObject parsing.
- Restructured into [cmd/radar/](../cmd/radar), `internal/*` packages.
- Renamed project to velocity.report.
- Added Apache 2.0 license.
- Implemented JSON parsing for radar data.
- Added unit tests for parsing.

## June 3, 2025 - radar objects table

- Created `radar_objects` SQL table.
- Added RadarObjects database functions.

## May 30, 2025 - live tail & command fixes

- Fixed live tail WebSocket functionality.
- Fixed command sending to serial port.

## May 26, 2025 - serial port configuration

- Updated serial port configuration handling.
- Updated event list SQL queries.

## May 22, 2025 - serialMux abstraction

- Initialised SerialMux for multiple subscriber support.
- Fixed graceful shutdown handling.
- Fixed x/net dependabot warning.

## May 16, 2025 - dev server & web skeleton

- Created working dev server configuration.
- Began web app skeleton with package namespace.
- Added systemd unit file for deployment.
- Fixed nested `/api/` route handling.
- Added `/debug/tailsql` and `/debug/backup` routes.

## May 12, 2025 - SQLite migration & serial tests

- Replaced DuckDB with SQLite (modernc.org/sqlite for pure Go).
- Added serialReader unit tests.
- Implemented bufio for serial port buffering.

## April 11, 2025 - command logging & handler extraction

- Added command logging to database.
- Extracted serialPortHandler into separate function.

## March 21, 2025 - serial reader & API improvements

- Implemented uptime parsing and validation.
- Added readline counter for serial data.
- Improved serial reader reliability.
- Updated backup filenames and intervals.
- Renamed execute endpoint.
- Set commandID from database.
- Added JSON response logging.
- Fixed timestamp casting in SQL.
- Added API verb support for commands.

## March 20, 2025 - baud rate & schema updates

- Set serial baud rate to 19.2k.
- Updated schema to v0.0.2 with uptime field.
- Added POST /execute endpoint.

## March 17, 2025 - project initialisation

- Initialised repository with Go 1.24.
- Created initial server structure with Gin HTTP framework.
- Set up serial port reader for radar sensor.
- Added gocron for backup scheduling.
- Configured DuckDB for initial data storage.
- Added .gitignore for database files and backup directory.
