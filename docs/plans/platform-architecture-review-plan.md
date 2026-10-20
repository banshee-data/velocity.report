# Architecture review: anti-patterns, unfinished work, and the shape of the project (v0.5.x)

- **Status:** Proposed, for planning review (Flo) and decision (Ruth); changes no code
- **Layers:** Cross-cutting (Go packages and CLI roles, schema, web, macOS, public site, experiments, planning)
- **Target:** v0.5.8 for the deletions and the boundary gates; a re-baselined backlog for the rest
- **Companion plans:** [platform-vocabulary-and-data-model-plan](platform-vocabulary-and-data-model-plan.md), [go-codebase-structural-hygiene-plan](go-codebase-structural-hygiene-plan.md), [go-cmd-extraction-plan](go-cmd-extraction-plan.md), [cmd-server-decomposition-plan](cmd-server-decomposition-plan.md), [lidar-worker-pool-and-results-hub-plan](lidar-worker-pool-and-results-hub-plan.md), [lidar-job-queue-implementation-plan](lidar-job-queue-implementation-plan.md), [platform-simplification-and-deprecation-plan](platform-simplification-and-deprecation-plan.md), [web-frontend-consolidation-plan](web-frontend-consolidation-plan.md), [platform-experiment-queue-plan](platform-experiment-queue-plan.md), [binary-size-reduction-plan](binary-size-reduction-plan.md)
- **Canonical:** [PLATFORM.md](../platform/PLATFORM.md)

A read of the backlog, the plans and the code as they stand at `acdb03e`, written to answer four
questions: which habits have become anti-patterns and what replaces them; which half-finished
migrations and renames to finish, cut down or delete; what belongs in another project; and how
one binary should carry several roles. It ends with the backlog changes proposed to Flo, the
design work handed to Appius, and three experiments that put numbers on the role question.

## Summary

The core architecture is sound and should be kept: one Go binary, SQLite, a frozen L1 to L10
layer model, Svelte for workflow, Metal for fidelity, Typst for reports, and experiments that
write their rule before their replays. What has gone wrong is at the edges, and most of it is the
same mistake in different clothes: the project adds a second way of doing something and keeps
the first.

1. **Planning has outrun delivery.** 307 open backlog items in 23 release buckets, 149 active plan files holding 53,560 lines, and a version string still at `0.5.1-pre45` five months after the changelog dated 0.5.1. The v0.5.2 bucket holds 55 items against the register's own cap of ten to twelve.
2. **Renames are planned as alias programmes, not cutovers.** The vocabulary programme has 11 sub-plans, 18 items and a retirement item in v0.6.7; none has touched code. Meanwhile one concept carries four names across four tiers, and "scene" was renamed away from one table and given to another.
3. **The one binary links everything for every role.** `velocity jobs` needs no internal package yet ships libpcap, Typst, gRPC and tailsql. The frame model lives in the gRPC package, so 37 packages link gRPC; an alias package drags the pipeline into L1 ingest; the LiDAR layer rules are documented but nothing enforces them.
4. **Two of everything.** Three job systems, two HTTP listeners serving the same routes in three registration styles, four logging styles in fifteen copied files, eight configuration mechanisms, five chart stacks, five JavaScript package roots, and 231 Make targets against a simplification plan that baselined 118.
5. **Experiments are the strongest practice in the repository and still leak.** 63 specs with a rule-first discipline, but the `approved` state has never been used, 31 answered specs point an arm at a defaults file that has since changed, and 14 reports have no spec.

The recommendation is a reduction: delete before you build, cut the vocabulary programme to three
one-release cutovers, make roles a property of the import graph rather than a set of binaries,
move the public survey site out of the product repository in two steps, and tag what is on `main`
as v0.5.2 before another fifty PRs land in "Unreleased".

## Current state

Everything below was measured on `acdb03e` on October 19, 2026. Counts are from `wc -l`,
`go list` and ripgrep over non-test files unless stated.

### Planning and releases

| Measure                                      | Value                                                                                                            |
| -------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Version string (Makefile)                    | `0.5.1-pre45`; the changelog dates 0.5.1 to May 11, 2026; no tags in the checkout                                |
| PRs merged October 11 to 19                  | 50, all into the changelog's Unreleased section, one of them a breaking change                                   |
| Open backlog items                           | 307 in 23 release buckets; 204 complete                                                                          |
| Largest bucket                               | v0.5.2 with 55 items in five sprint sub-buckets; DECISIONS.md caps a milestone at 10 to 12                       |
| Backlog item length                          | 372 characters on average; 36 items over 600; the file's own rule is two sentences                               |
| Plan files                                   | 185, of which 36 are graduated symlinks; 149 active, 53,560 lines                                                |
| Plan status words                            | 80 proposed, draft or planned; 11 `Active` and 7 `In progress` (18 active under D-32); 9 implemented; 5 complete |
| Vocabulary programme                         | 1 programme plan (1,294 lines) and 11 sub-plans, every one "outlined, not implemented"                           |
| Visualiser proposals from February 2026      | 10 plans; twelve visualiser hub docs carry a "Source plan" header, most describing features that do not exist    |
| Markdown mass under docs/, data/ and config/ | 516 files, 138,786 lines; plans are 39 % of it                                                                   |
| Experiment specs                             | 63 (38 answered, 25 proposed, 0 approved); 37 dated reports; 18 parameter sets                                   |

### Code

| Measure                              | Value                                                                                                                                                    |
| ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go packages                          | 106 (30 under cmd/, 76 under internal/); 193,083 non-test lines, 262,531 test lines                                                                      |
| LiDAR share                          | 143,549 non-test lines, 74 % of the Go; 30 packages beside the nine layer packages                                                                       |
| Files over the 600-line soft ceiling | 75 non-generated source files; 17 over 1,000; `server.Main` is 890 lines with 42 global flags                                                            |
| Make targets                         | 231 unique (180 in `make help`), 2,857 lines; the simplification plan's February baseline was 118                                                        |
| Schema                               | 45 tables, 62 migrations; `radar_commands` and `radar_command_log` have no reader or writer, `radar_transit_links` no reader, `lidar_clusters` no writer |
| Listeners                            | :8080 and :8081 both serve every LiDAR route; :50051 gRPC; :8084 worker; three route-registration styles                                                 |
| Logging                              | 393 `log.*` calls, 439 `Opsf/Diagf/Tracef`, 199 stderr writes, 15 copied `debug.go` files, no slog, no metrics registry                                  |
| Configuration                        | 8 mechanisms, 87 config structs, about 600 flag definitions; defaults loaded by walking `../..` from inside layer packages                               |
| Role closures                        | `cmd/velocity` reaches 69 internal packages; `cmd/jobs` needs 0 but links the lot; `cmd/tune` links the server to get an HTTP client                     |
| Layer rule violations                | 6 edges (L1 to L3, L1 to db, L6 to L8, L4b to L4, db to l3grid, the root alias package into pipeline); no check enforces the rule                        |
| Engines                              | 8 registered in config, 3 accepted by the runtime; 5 have typed parameter blocks and validation and no implementation                                    |
| UI surfaces                          | Svelte 19.1k lines; legacy :8081 dashboards 7.0k lines plus go-echarts; public site 20k lines plus 66 MB of scene assets in git; macOS 70.4k Swift       |
| JavaScript roots                     | 5 package roots with their own lockfiles; three.js and prettier at different versions between them                                                       |
| Python                               | 57 scripts, 22,812 lines; 12 referenced by nothing; `tox.ini` lists 7 of 31 test files                                                                   |
| Agent definitions                    | 7 personas kept twice (.claude and .github), 32 to 194 lines of drift each; the drift check is not in CI                                                 |

## Findings

| Area                                 | Current state                                                                                               | Severity | Release view                         |
| ------------------------------------ | ----------------------------------------------------------------------------------------------------------- | -------- | ------------------------------------ |
| Release cadence                      | Five months of work in Unreleased; version string and changelog disagree                                    | High     | Now: tag v0.5.2 from `main`          |
| Backlog shape                        | 307 items, paragraphs not items, 55 in one bucket, progress narrative inline                                | High     | Now: re-baseline (Flo)               |
| Plan corpus                          | 149 active plans, 80 not started, two ready to graduate, twelve hub docs headed by a source plan            | Medium   | Now: WIP limit and parking           |
| Vocabulary programme                 | 18 items, multi-release aliases, nothing in code; collisions already appearing (`lidar_scenes` twice)       | High     | v0.5.8: three cutovers               |
| Role closures                        | Every role links everything; cgo always on; no closure or layer gate                                        | High     | v0.5.8: gate, then the moves         |
| Frame model in the transport package | `FrameBundle` in `l9endpoints`; 37 packages link gRPC                                                       | High     | v0.5.8                               |
| Alias package and upward edges       | `internal/lidar/aliases.go`; six layer violations; ingest links SQLite and tailsql                          | High     | v0.5.8                               |
| Job systems                          | Three queues, two ID formats, a worker that compiles the repository at run time                             | High     | v0.5.8: one queue (Item 11 moved up) |
| Duplicate surfaces                   | Legacy :8081 dashboards, chart JSON API with no client, five chart stacks, dead export code                 | Medium   | v0.5.8: delete                       |
| SQL boundary                         | Type aliases defeat the check; raw SQL in five packages; `internal/db` owns LiDAR tables and embeds tailsql | Medium   | v0.5.8                               |
| Lifecycle                            | No supervisor; seven Start/Stop signatures; the transit worker lives in the DB package; HTTP owns ingest    | Medium   | v0.5.9                               |
| Logging and configuration            | Four logging styles; eight configuration mechanisms; five phantom engines                                   | Medium   | v0.5.9                               |
| Public survey stack                  | A second product in the monorepo; 66 MB of assets copied into every Pi image                                | Medium   | v0.6.6: extract in two steps         |
| Experiment registry                  | `approved` unused; arms drift with the defaults; reports without specs                                      | Medium   | Now: pin the fingerprint             |
| Test corpus                          | 263k test lines, 203 `time.Sleep` sites, 6.4k lines of Jest testing legacy JS slated for deletion           | Low      | With the deletions                   |

## Anti-patterns and their fixes

Each item states the finding with evidence, two or three options with their burden, and the
recommendation. Effort uses the backlog's `S`, `M`, `L` scale.

### Planning and documentation

#### P1. The version is a theme, not a tag

**Finding.** `VERSION := 0.5.1-pre45` while CHANGELOG.md dates 0.5.1 to May 11, 2026. The
Unreleased section carries the L3 consolidation, which is a breaking change for reproducing a
paced replay. Fifty PRs landed in nine days. The backlog's "v0.5.2 Tailgating metric" is a
programme of 55 items that will not be done this year, so nothing can be called v0.5.2 and
nothing is tagged. Release engineering is in place (static builds, versioned installs, atomic
swaps) and unused.

| Option                                                                                                    | Effort | Risk                                                                                        |
| --------------------------------------------------------------------------------------------------------- | ------ | ------------------------------------------------------------------------------------------- |
| a. Keep the themed buckets and wait for the tailgating metric                                             | none   | Another five months in Unreleased; the Pi runs May's build                                  |
| b. Tag what is on `main` as v0.5.2 now, retitle the bucket to what shipped, carry the programme as a plan | `S`    | The changelog's v0.5.2 entry is "consolidated L3, published box, label sets", which is true |
| c. Switch to date-based versions                                                                          | `M`    | Breaks the version check and the image naming                                               |

**Do b.** A version names a build, a programme names an intent; the backlog conflates them. After
the tag, a release is cut whenever `main` carries a user-visible change and the perf gate is
green, and the themed titles move to programme plans.

#### P2. The backlog is a progress log wearing a list's clothes

**Finding.** Items average 372 characters and 36 exceed 600, against the file's own two-sentence
rule. Items carry results ("on kirk0 the solid bodies fit 7 of 60"), branch names, and what
landed in which PR. v0.5.2 holds 55 items in five sprint sub-buckets. The register's own
principle six says ten to twelve per milestone.

| Option                                                                                                   | Effort | Risk                                                               |
| -------------------------------------------------------------------------------------------------------- | ------ | ------------------------------------------------------------------ |
| a. Leave it; the detail is useful to whoever is deep in the work                                         | none   | Nobody else can read it; priorities are invisible                  |
| b. Move narrative into each plan's checklist, cap items at two sentences by lint, cap a bucket at twelve | `M`    | One grooming pass by Flo; the plans grow by what the backlog loses |
| c. Replace the backlog with issues                                                                       | `L`    | Loses the single file the skills and CI read                       |

**Do b.** The `backlog-prune` skill and `check-prose-line-width.py` already exist; an item-length
check is a twenty-line addition. The sprint sub-buckets become the programme plan's phases.

#### P3. Plans are written faster than they are built

**Finding.** 149 active plans; 80 are proposed, draft or planned; the largest is 3,100 lines. Two
plans say they are ready to graduate (go-runtime-pipeline-correctness and
lidar-captures-multi-file-cases) and have not. Ten visualiser proposals from February, and twelve
hub docs in all, spawned a hub doc stub under docs/ui/visualiser/ that begins "Source plan:" and
describes a feature that does not exist, which inverts the graduation rule in
[canonical-plan-graduation.md](../platform/architecture/canonical-plan-graduation.md).

| Option                                                                                                                                                           | Effort | Risk                                                |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ | --------------------------------------------------- |
| a. Leave the corpus; plans are cheap to write                                                                                                                    | none   | They are not cheap to read, link-check or keep true |
| b. A WIP limit of twelve Active plans; everything else Proposed or Parked with a one-line reason; graduate the two; move the ten hub stubs back into their plans | `M`    | A judgement call per plan, once                     |
| c. Delete the parked plans                                                                                                                                       | `S`    | Loses design work that cost real thought            |

**Do b.** Add `Parked` to the status vocabulary and an advisory in `report-plan-hygiene` that
lists Active plans without a linked PR in the last 90 days. A hub doc records shipped behaviour;
a plan records intent.

#### P4. Rename by alias programme

**Finding.** The vocabulary programme is right about the problem (scene has five meanings, site
has thirteen identities, capture seven) and wrong about the method. Eighteen items each add an
alias and a retirement item removes them in v0.6.7, two minor versions later. D-20 already
decided "no temporary dual-format shims". No item has started, and while it waited migration 039
reused `lidar_scenes` for a second concept.

| Option                                                                                                                                                           | Effort | Risk                                                |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ | --------------------------------------------------- |
| a. Run the programme as written: eighteen items, aliases, retirement in v0.6.7                                                                                   | `XL`   | Two names for everything for a year; never finishes |
| b. Three one-release cutovers (clip, sites, labels), each one PR series: migration rename, Go types, routes, UI copy, with old names treated as D-20 allows (P5) | `L`    | Three breaking releases, each small and announced   |
| c. Rename nothing; document the four names                                                                                                                       | `S`    | The next feature adds a fifth                       |

**Do b.** Order by consumer count: clip first (Item 2: `lidar_replay_cases` to `lidar_clips`,
`ReplayCase` to `Clip`, `/api/lidar/scenes` to `/api/lidar/clips`), then one `sites` table
(Item 14, which unblocks every map and the survey export), then one label table (Item 7). The
remaining items become feature work on the roles (jobs, Item 11) or wait for a consumer. Each
cutover treats an old name by who still depends on it, as P5 sets out: most need no alias at
all.

#### P5. Decisions that the code does not keep

**Finding.** A decision is recorded once and never re-read. Of the 29 in
[DECISIONS.md](../DECISIONS.md), 22 were checked against the code on October 19: 9 are kept, 5 have
drifted and 2 have partly drifted, 2 have been overtaken by delivered work without being restated,
3 are decided and unbuilt, and 1 is pending as scheduled. The other 7 were not checked. Nothing in
CI, the skills or the plan template asks whether a change keeps or breaks a decision, so a plan can
contradict one without citing it: the vocabulary programme proposes eighteen alias items under a
decision (D-20) that forbids temporary aliases, and does not mention D-20.

##### Audit

The audit table moved to its own hub doc, the [decision
audit](../platform/operations/decision-audit.md), which release preparation refreshes. On October
19, 22 of the 29 decisions were checked: 9 kept, 5 drifted and 2 partly drifted, 2 overtaken by
delivered work, 3 unbuilt and 1 pending. Ruth's pass on October 20 corrected three details
(`release.json` is read in `internal/ctl`, D-18 has a second call site in `features.go`, and D-25
has also drifted on persona length, 117 to 286 non-empty lines against about 40 to 80), restated
seven entries, and recorded every state there.

##### How a decision stays kept

| Option                                                                                                                                                    | Effort | Risk                                      |
| --------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ | ----------------------------------------- |
| a. Leave the register as a history                                                                                                                        | none   | It is read as the present                 |
| b. Each decision names what keeps it and carries a state; plans cite the decisions they rely on or would break; the audit above is re-run at release prep | `S`    | Some decisions can only be kept by review |
| c. Re-decide everything                                                                                                                                   | `L`    | Churn, and the same drift a year later    |

**Do b**, in four parts:

1. **Kept by.** Each decision gains one line naming the check, test or artefact that fails when the decision is broken: the closure test for D-09, the Make check for D-14, the register test for D-20, `verify-static-elf.sh` for D-26. Where only review can keep it, the line names the skill that reviews it. A decision that can name neither is a preference and belongs in the vision.
2. **State.** Kept, Drifted (naming the backlog item that fixes it), Overtaken (restated in the next edit), Unbuilt (naming its release), or Superseded by D-NN. Nothing is deleted from the register.
3. **Plans cite decisions.** The plan template gains a `Decisions` metadata line listing what the plan relies on or proposes to supersede; `/plan-review` checks a plan against the decisions its subject touches and refuses one that contradicts a decision it does not cite.
4. **Audit at release.** `/release-prep` re-runs the audit table above; `/weekly-retro` takes the review-only decisions in turn.

This stays inside DECISIONS.md's own cadence: Ruth records the states and the restatements in
one edit, D-30 to D-34 join with their Kept-by lines from the start, and D-04, D-07, D-16 and
D-22 are restated in the same edit.

##### D-20 in practice: who can still depend on an old name

D-20 allows two exceptions: database upgrade detection, and aliases that are architecturally
necessary. The register below shows that "necessary" has been read as "convenient". A precise
test, taken from tenet 7: **an old name, field or format is necessary only while something the
release does not replace depends on it.**

A release replaces the binary, the web app, the embedded docs and the bundled dependencies, and
with them every Make target, script, skill and doc in the repository at that commit. It does not
replace four things: the files on a flashed device that `velocity device upgrade` leaves alone
(the systemd unit, sudoers, the profile aliases); the macOS app, which ships as its own DMG; data
at rest (databases, recordings, packs, label sets, an operator's tuning files); and anything
published outside the repository (site URLs, the Hugging Face layout, `release.json`).

Each kind of dependent gets one treatment: an in-release caller is renamed in the same change, a
person typing an old name gets a tombstone, the macOS app gets a registered alias until the next
DMG, a device file gets an alias or a rewrite, data at rest gets a migration or converter, and a
published URL gets a permanent redirect. The table of treatments and lifetimes is in the
[compatibility
register](../platform/operations/compatibility-register.md#who-may-still-depend-on-an-old-name).

##### The register

The register of the nineteen surviving names moved to its own hub doc, the
[compatibility register](../platform/operations/compatibility-register.md), with each row's
consumer, treatment and removal release, and the treatments table in full.

**Keeping it kept.** The register is one row per surviving name, its consumer, its treatment
and the release that removes it. A test
fails when the Makefile's `VERSION` reaches a row's removal release, so an alias cannot outlive
its row. Route aliases are declared through the route table with the row they belong to, flags
through one tombstone helper, and the Make check below refuses an alias recipe; a dual reader in
code is caught by `/review-pr`, which reads the register.

**What it does to the vocabulary programme.** Most of its aliases are never created. Tables
rename by migration; Go types, web routes, UI copy, scripts and docs rename in one PR; the CLI
and flags get tombstones; only the routes the macOS app calls get registered aliases, and the
public `/scenes/` pages get permanent redirects to `/surveys/`. Item 18, the retirement release,
shrinks to the rows that remain in the register. This keeps D-20 as written and revises the
first draft of ADR-2 below.

##### Make targets

D-14's Make half is the clearest drift: 118 targets in February, 231 now, 2,857 lines, a
hand-written help block that lists 177 targets, omits 54 and echoes two sentence fragments as
if they were targets, 68 targets that nothing calls, and 27 CI steps that bypass Make although
the coding standards say workflows call Make targets. The full inventory, with a disposition for
every target, is in the
[simplification plan](platform-simplification-and-deprecation-plan.md#1-make-target-candidates).
In short:

| Group                                                                                     | Targets | Disposition                                                                             |
| ----------------------------------------------------------------------------------------- | ------: | --------------------------------------------------------------------------------------- |
| Aliases (`build-radar-*`, `build-ctl*`, `build-tools`, `*-embed`, config and maths twins) |      17 | Delete; callers renamed in the same PR; one tombstone rule for one release              |
| `api-*` curl wrappers, all aimed at :8081                                                 |      16 | Delete the Make wrappers; the scripts stay in `scripts/api/lidar` with their README     |
| `migrate-*` wrappers of `velocity data migrate`                                           |       7 | Delete; the CLI is the interface                                                        |
| Python plot and stats wrappers                                                            |       5 | Delete, with `tools/grid-heatmap` (D-17)                                                |
| Wrappers of `velocity lidar` subcommands                                                  |       3 | Delete with the thin `cmd/tools` wrappers                                               |
| Conveniences nothing calls                                                                |      19 | Delete, or move to `scripts/` if someone runs one by hand                               |
| Variants that fold into one target and a variable                                         |      17 | Fold: logs, dev kills, cleans, the five JavaScript installs, the two public-site builds |
| Names that still say `radar` or call the public site "docs"                               |      11 | Rename; callers (CI, the image build, skills) in the same PR                            |

That leaves 147 targets, and the folds add one, `clean`: 148, inside a budget of 150. Three
structural changes keep it there: help generated from each target's `##` comment instead of a
hand-kept echo block; a `check-make` lint that refuses an alias recipe and an undocumented public
target and reports the count against the budget; and CI steps that call Make targets rather than
scripts. Splitting the Makefile into includes by concern is worth doing after the trim, not before
it.

#### P6. Experiment arms that point at a moving target

**Finding.** The experiment registry is the best practice in the repository and three leaks cost
it credibility. The `approved` status has never been set on any of 63 specs, so the one gate the
format has is skipped by its own skill. 31 answered specs name `config/tuning.defaults.json` as
an arm, and that file has changed twice since; only the report's commit says what ran. Fourteen
reports have no spec, and the status vocabulary differs between specs, the dependency graph, the
local units and the planned hub.

| Option                                                                                                                                                                                      | Effort | Risk                                        |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ | ------------------------------------------- |
| a. Leave it; the reports carry commits                                                                                                                                                      | none   | A spec stops being a record of its own arms |
| b. The validator records the defaults' fingerprint in each arm at creation and refuses an answered spec whose arm drifted; drop `approved` or make `mark` require it; one status vocabulary | `S`    | A one-off backfill of 38 specs              |
| c. Move experiments to a database                                                                                                                                                           | `L`    | Loses the Git review of a rule              |

**Do b.** The fingerprint already exists (`TuningConfig.Fingerprint`); the spec gains one field.

### Code and architecture

#### C1. Every role links everything, and cgo is always on

**Finding.** `internal/cmd/root/root.go` imports all seven applets eagerly; `VELOCITY_BUILD_TAGS`
always includes `pcap`. `velocity jobs` has an empty internal closure and still ships libpcap,
Typst, gRPC and tailsql. `velocity tune` links the whole server because the HTTP client lives in
`internal/lidar/server/client.go`. A no-pcap build is possible in principle (nothing outside the
tag needs cgo) and no target builds one, so every cross-compile needs the Docker toolchain.

| Option                                                                                                                                            | Effort | Risk                                                    |
| ------------------------------------------------------------------------------------------------------------------------------------------------- | ------ | ------------------------------------------------------- |
| a. Keep one binary, one build, as now                                                                                                             | none   | Roles cannot be reasoned about; the Pi carries a worker |
| b. Keep one binary; make each role's closure a CI assertion and fix the graph until the assertions hold; add a `nopcap` build for hub and clients | `M`    | The graph fixes are the moves in the roles section      |
| c. Split into binaries per role                                                                                                                   | `L`    | Breaks D-09, the image, the installer and tenet 7       |

**Do b.** The user is right to like one binary. What a role needs is a composition root and a
dependency closure, not an artefact; the closure is checkable with `go list -deps` today.
Experiment host0001 measures whether a role-scoped build is worth anything beyond the gate.

#### C2. The alias package is a layer-rule bypass

**Finding.** `internal/lidar/aliases.go` imports pipeline, storage/sqlite and l2 to l6. Its only
production consumers are `l1packets/network` and `l9endpoints`, which use it for four logging
functions. The result is that UDP ingest links SQLite, `internal/db` and tailsql, against the
rule in AGENTS.md. Five more upward edges exist: L1 to L3 (foreground extraction inside the PCAP
reader), L1 to db (an unused field), L6 to L8 (re-exports), L4b to L4, and `internal/db` to
`l3grid`. No lint target enforces the layer rule.

**Do this.** Delete the alias package; move the logging functions to `internal/lidar/logutil`;
fix the five edges; add the layer rule and the role closures to `make lint-go` as a Go test over
`go list` output. `M`, and it is the gate that keeps every later move honest.

#### C3. The domain frame model lives in the transport package

**Finding.** `FrameBundle`, `Track`, `TrackState` and `PointCloudFrame` are defined in
`internal/lidar/l9endpoints/model.go`. Annotation, segments, scene export, analysis and the VRLOG
recorder import them, so 37 packages link gRPC, including `cmd/tools/config-validate`. The
recorder is nested under the gRPC package for the same reason.

**Do this.** Move the model to a transport-free package (`l2frames` for frames and points; a
small `internal/lidar/model` package if tracks need a home above L5); `l9endpoints` becomes a
converter to proto and nothing else; the recorder moves beside `storage/vrlog`. `M`.

#### C4. Three job systems and a worker that compiles the repository

**Finding.** `lidar_capture_jobs` (SQLite, kinds by trigger), `lidar_segment_clip_jobs` (a second
table for one kind), and a directory-tree queue in `internal/lidar/jobs/runner` served by
`velocity worker`, with two ID formats. The worker needs git, a Go toolchain and libpcap headers
because it builds `cmd/tools/<tool>` at a job's commit. The hub plan makes the NAS the authority
before a second worker exists. No October report cites the worker; the runs were shell lanes.

| Option                                                                                                                                                                                | Effort | Risk                                               |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ | -------------------------------------------------- |
| a. Build the hub as planned (NAS authority, upload bundles, three plans)                                                                                                              | `L`    | A distributed system for one Mac and one blade     |
| b. One queue in SQLite owned by `velocity serve`; the worker claims over HTTP; kinds are compiled into the binary; a job records build version, tuning fingerprint and capture digest | `M`    | Kinds must be on `main` to run, which is the point |
| c. Delete the worker and keep shell lanes                                                                                                                                             | `S`    | Experiments stay hand-run and unrepeatable         |

**Do b.** It is vocabulary Item 11 moved from v0.5.11 to v0.5.8 and widened to absorb the worker
pool and job queue plans. Experiment host0002 checks that an in-binary kind reproduces a built
tool row for row.

#### C5. God package, god function, HTTP that owns ingest

**Finding.** `internal/lidar/server` is 15.6k source and 39k test lines across 53 files with 29
internal imports, including `internal/api` (a reverse edge). It owns UDP ingest, PCAP replay,
capture jobs, sweeps, scenes, segments, annotations and charts, and starts the live listener from
`Start()` and from HTTP handlers. `server.Main` is 890 lines and 42 global flags, and the same
package hosts the `data` and `report` CLIs. Two Draft plans describe the same file.

**Do this.** Merge [cmd-server-decomposition-plan](cmd-server-decomposition-plan.md) and
[go-cmd-extraction-plan](go-cmd-extraction-plan.md) into one, and add a `Service` interface
(`Run(ctx) error`) with an errgroup supervisor in the serve composition root. Ingest lifecycle
leaves the HTTP package; the transit worker leaves `internal/db` for `internal/radar`. `L`, but
it is the second half of making roles real.

#### C6. Boundaries that are nominal

**Finding.** The SQL boundary check greps for the `database/sql` import; `sqlite.SQLDB` and
`sqlite.SQLTx` are type aliases that let any package run SQL. Raw SQL sits in `api/lidar_labels.go`
(a `LabelStore` exists and is unused), `segments/read.go`, `server/run_track_api.go`,
`perframeeval/evalfixture` and two tools. `internal/db`, the "radar core" package, owns three
LiDAR tables and embeds tailsql and tsweb, so every DB consumer links Tailscale. Defaults are
loaded by `MustLoadDefaultConfig` walking `../..` from inside l3, l4, l5, l6 and storage. The root
embed package mixes web and docs assets with tuning defaults, so four evaluation tools need
`web/build` to compile.

**Do this.** Check for `sqlite.SQLDB` use outside the two storage packages, or better, make the
store interfaces the only API; move the LiDAR tables and the admin routes out of `internal/db`;
inject defaults from the composition root; split the root embed package into assets and
defaults. Four `S` items and one `M`.

#### C7. Two of everything on the surface

**Finding.** Every LiDAR route is on :8080 and :8081; the web dev proxy sends half the API to
each. Three route styles coexist with 98 manual method checks. `/api/scenes` and
`/api/lidar/scenes` mean different things. The chart JSON API (about 600 lines) has no client.
go-echarts dashboards (7.0k lines) are the only place to start a sweep, and 6.4k lines of Jest
test them from the web tree. Grid and region views are rendered five ways including a 1,413-line
matplotlib tool. Three site maps. Six link checkers. Five JavaScript roots.

**Do this.** The consolidation plan's Phase 1 is "one day of consumer-free deletions" and has not
started. Do it, then move sweep start to Svelte (Phase 5) and retire :8081 (Phase 2). One pnpm
workspace; one Go-served SVG for grid views. `M` in total, most of it deletion.

#### C8. Experiment switches accrete as configuration

**Finding.** 22 solid-body booleans in the tuning schema and 67 named default-off experiments in
`replayeval/experiments.go` overlap (the code checks both). Five engines are registered,
validated and refused by every entry point. `tuning.optimised.json` and three sweep files sit
outside the README-row test. The backlog carries W8, a retirement list for superseded experiments,
as an item for v0.5.7.

**Do this.** Every experiment arm has a config key or is deleted when its spec is answered; the
spec's `reclaim` field already says when. Delete the five phantom engines now; re-add one when it
is built. `S`.

#### C9. Tests that measure the wrong thing

**Finding.** 262k test lines against 193k source; 23 test files over 1,200 lines, one at 6,324;
203 `time.Sleep` sites in 40 files; the v0.9.0 goal is 95.5 % line coverage in every package.
None of this caught six layer violations, a nominal SQL boundary or a build that links a sensor
driver into a job client.

**Do this.** Replace the coverage target with boundary tests: closures, layer rules, store
interfaces, and a serve smoke test. Delete the Jest tests for the legacy dashboards with the
dashboards. `M`, mostly deletion.

## Half-finished work: finish, cut down, or delete

Ranked by what it unblocks. "Delete" means the code and the plan both go; "cut down" means the
plan shrinks to what the code shows is worth finishing.

| Rank | Work                                                                                                                                                                                                                    | State today                                                                          | Decision                                                 | Effort | Release                |
| ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ | -------------------------------------------------------- | ------ | ---------------------- |
| 1    | Legacy :8081 dashboards, chart JSON API, go-echarts                                                                                                                                                                     | Fully present; Phase 1 of the consolidation plan ("one day") not started             | Delete                                                   | `S`    | v0.5.8                 |
| 2    | Dead code: five phantom engines, `httputil`, `fsutil`, `adapters/track_export.go`, `training_data.go`, L6 curation helpers, three thin `cmd/tools` wrappers, two one-shot backfills, 12 unreferenced scripts, `tox.ini` | Zero production callers                                                              | Delete                                                   | `S`    | v0.5.8                 |
| 3    | Dead tables: `radar_commands`, `radar_command_log`, `radar_transit_links`, `lidar_clusters`                                                                                                                             | No reader, no writer, or write-only                                                  | Delete                                                   | `S`    | v0.5.8                 |
| 4    | Layer gate and role closures                                                                                                                                                                                            | Rules documented, nothing enforced                                                   | Finish                                                   | `M`    | v0.5.8                 |
| 5    | Alias package and six upward edges                                                                                                                                                                                      | Present                                                                              | Finish                                                   | `M`    | v0.5.8                 |
| 6    | Frame model out of the gRPC package                                                                                                                                                                                     | Not started                                                                          | Finish                                                   | `M`    | v0.5.8                 |
| 7    | One job queue (Item 11) absorbing worker pool and job queue plans                                                                                                                                                       | Three systems; worker builds from source                                             | Cut down                                                 | `M`    | after the Phase 1 exit |
| 8    | Vocabulary programme                                                                                                                                                                                                    | 18 items, none started                                                               | Cut down to clip, sites, labels                          | `L`    | v0.5.8 to v0.5.10      |
| 9    | cmd/server decomposition and cmd extraction                                                                                                                                                                             | Two Draft plans on one 1,489-line file                                               | Merge and finish                                         | `L`    | v0.5.9                 |
| 10   | Lifecycle supervisor; transit worker out of `internal/db`                                                                                                                                                               | Not started                                                                          | Finish                                                   | `M`    | v0.5.9                 |
| 11   | SQL boundary made real; `LabelStore` wired                                                                                                                                                                              | Nominal; `LabelStore` exists and is unused                                           | Finish                                                   | `M`    | v0.5.9                 |
| 12   | Structured logging                                                                                                                                                                                                      | Draft, unscheduled, "no backlog item"; 15 copied `debug.go`                          | Finish minimally with slog                               | `M`    | v0.5.9                 |
| 13   | Configuration: path-walking defaults, env reads in L1, duplicate structs                                                                                                                                                | Phase 2B proposed                                                                    | Finish                                                   | `M`    | v0.5.9                 |
| 14   | Clock abstraction phases A to C                                                                                                                                                                                         | `timeutil.Clock` has no production importer; 7 ad hoc `now` fields; 257 `time.Now()` | Finish                                                   | `M`    | v0.5.9                 |
| 15   | Deterministic track identity                                                                                                                                                                                            | Specification; scenes "not yet reproducible" (#783)                                  | Finish                                                   | `M`    | v0.5.9                 |
| 16   | Run tracks missing six quality columns                                                                                                                                                                                  | Snapshot loses data the live table prunes                                            | Finish                                                   | `S`    | v0.5.8                 |
| 17   | VRLOG JSON 0.5 decoder                                                                                                                                                                                                  | Three recordings on one machine still use it                                         | Delete after converting                                  | `S`    | v0.5.8                 |
| 18   | Simplification plan                                                                                                                                                                                                     | Approved in February; targets grew from 118 to 231                                   | Trim to 148 (P5, Project F)                              | `M`    | v0.5.8                 |
| 19   | Scene assets in git and in the Pi image                                                                                                                                                                                 | 66 MB, growing per site                                                              | Finish: LFS or Hugging Face, image carries a pointer     | `S`    | v0.6.6                 |
| 20   | Svelte 5 runes migration                                                                                                                                                                                                | 1 of 25 files                                                                        | Finish one file per touched route, or state the deferral | `M`    | v0.7.0                 |
| 21   | #503 Tailscale ACL roles                                                                                                                                                                                                | A PR conflicted since v0.5.1                                                         | Decide: rebase or close                                  | `S`    | Now                    |
| 22   | Ten February visualiser proposals and their hub stubs                                                                                                                                                                   | Nothing built; the annotation window superseded several                              | Park; delete the stubs                                   | `S`    | Now                    |
| 23   | Dynamic algorithm selection                                                                                                                                                                                             | "Branch-history design, not active on main"                                          | Park                                                     | `S`    | Now                    |
| 24   | TicTacTail (D-23)                                                                                                                                                                                                       | No code, one consumer                                                                | Retire the decision                                      | `S`    | Now                    |
| 25   | Typed UUID prefixes                                                                                                                                                                                                     | Proposed; depends on the nouns                                                       | Park until the cutovers                                  | none   | v0.6.x                 |
| 26   | Two plans ready to graduate                                                                                                                                                                                             | go-runtime-pipeline-correctness, lidar-captures-multi-file-cases                     | Graduate                                                 | `S`    | Now                    |

## Project boundaries: what to split out and what to keep

Five tests decide whether something belongs in this repository:

1. **Release unit.** Does it ship in the versioned unit of tenet 7 (binary, UI, docs, bundled dependencies)? If yes, it stays.
2. **Data model.** Does it read or write the SQLite schema or the VRLOG container directly? If yes, it stays until the contract is versioned.
3. **Audience.** Is it for the operator on the street, or for the public, a researcher, or a labelling team?
4. **Cadence.** Does it change on the binary's release rhythm, or on its own?
5. **Clone cost.** Does keeping it cost every clone, build and image?

| Candidate                                                                 | Tests failed                 | Decision                                    | How                                                                                                                                                                                                                                                                  |
| ------------------------------------------------------------------------- | ---------------------------- | ------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Public survey site, S2 archive tooling, scene publishing, 66 MB of assets | 1, 3, 4, 5                   | **Split, in two steps**                     | First a versioned scene-player package consumed by web, the site and the capture harness; then the site, `tools/s2-archive`, `tools/s2-hilbert` and the assets move to their own repository. `velocity scene export` and its NDJSON format are the contract and stay |
| Vehicle encyclopedia                                                      | 1, 3, 4                      | **Split** (the plan already says so)        | A separate open project; velocity.report embeds a versioned subset. Nothing in-repo until the spikes report                                                                                                                                                          |
| Spatial priors service                                                    | 1, 3, 4                      | **Split** (the plan already says so)        | Its own repository; the opt-in client is the only in-repo code, behind tenet 4                                                                                                                                                                                       |
| Podcast transcription, paper download, market tooling                     | 1, 3, 4                      | **Split**                                   | Move the tools out; the briefings they feed are docs and stay                                                                                                                                                                                                        |
| TicTacTail                                                                | none, because nothing exists | **Do not build**                            | Retire D-23 unless a second consumer appears                                                                                                                                                                                                                         |
| macOS visualiser                                                          | 4                            | **Keep, for now**                           | 70k lines with its own CI and DMG, but the proto and the 13 HTTP paths are unversioned and the label-set model is mirrored in Go. Revisit after a versioned proto                                                                                                    |
| Worker and job queue                                                      | none                         | **Keep** as a role                          | Same binary, same schema                                                                                                                                                                                                                                             |
| Radar                                                                     | none                         | **Keep**                                    | The core product                                                                                                                                                                                                                                                     |
| Embedded docs site                                                        | none                         | **Keep**                                    | Tenet 7                                                                                                                                                                                                                                                              |
| Agent framework (.claude, .github, knowledge modules, skills)             | none                         | **Keep**, single-source                     | Generate one side from the other; run the drift check in CI                                                                                                                                                                                                          |
| Hardware drawings (rack, pinouts, overlays)                               | 3, 4                         | **Keep** until a hardware repository exists | Docs tooling; no coupling                                                                                                                                                                                                                                            |
| Experiment registry and reports                                           | none                         | **Keep**                                    | They are the engineering record                                                                                                                                                                                                                                      |

The survey stack is the one that matters. It is a product with its own audience and cadence,
it is the reason the repository carries five JavaScript roots, and the Pi image copies it in
whole. The exporter stays, because the exporter is what a release changes.

## Package and role structure: one binary, several roles

### Roles

The binary stays one artefact. A role is a composition root under `internal/cmd/` plus a
dependency closure that CI asserts. The user's three roles are kept with one addition: the Pi
runs edge and hub in one process, and the analysis fleet runs them apart.

| Role                       | Entry today                       | What it does                                                                                                         | Must link                                                                   | Must not link                                          |
| -------------------------- | --------------------------------- | -------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- | ------------------------------------------------------ |
| **edge** (sensor host)     | `velocity serve` with sensors     | Radar serial and LiDAR UDP ingest, L1 to L6 live, VRLOG recording, gRPC publishing, local storage, API, web, reports | pipeline, l1 to l6, storage, api, report, serialmux, l9endpoints, tailscale | worker kinds, sweep, replayeval, annotation tooling    |
| **hub** (the storage role) | `velocity serve` with sensors off | The authority for data at rest: SQLite, API, web, reports, the job queue, the published-scene index                  | storage, api, report, jobs, docsite                                         | pcap, pipeline, serialmux, grpc                        |
| **worker**                 | `velocity worker`                 | Claims one job at a time: replay, sweep, scorecard, scene export, benchmark; writes a bundle; never the authority    | pipeline, l1 to l8, replayeval, sweep, scene, storage (local), jobs         | serialmux, tailscale, typst, web and docs embeds, api  |
| **device** (client)        | `velocity device`                 | Install, upgrade, rollback, backup on the Pi                                                                         | ctl, version                                                                | everything else; today it links db for one backup call |
| **jobs** (client)          | `velocity jobs`                   | Submit, watch, fetch                                                                                                 | nothing internal                                                            | everything                                             |
| **tools** (dev host only)  | `cmd/tools/*`, `velocity lidar …` | Evaluation and annotation tooling needing cgo                                                                        | as needed                                                                   | not shipped; D-26 already says so                      |

The web app, the macOS app and the browser are L10 clients of hub and edge; they are not roles.

### Boundary diagram

```text
                 sensor host (Pi)                        analysis fleet
   ┌──────────────────────────────────────┐    ┌──────────────┐   ┌──────────────┐
   │ velocity serve  = edge + hub         │    │ hub (NAS/Mac)│   │ worker (swan)│
   │                                      │    │ velocity     │   │ velocity     │
   │ radar serial ─┐   ┌─ L1..L6 live     │    │ serve        │   │ worker       │
   │ LiDAR UDP ────┤   │  VRLOG recorder  │    │              │   │              │
   │               ▼   ▼                  │    │ SQLite ◄─────┼───┼─ claim/result│
   │          pipeline ──► storage (SQLite)│    │ job queue    │   │  over HTTP   │
   │                        │             │    │ API + web    │   │ pipeline     │
   │  gRPC :50051 ◄─────────┤             │    │ reports      │   │ replay/sweep │
   │  API + web :8080 ◄─────┤             │    │ scene index  │   │ scene export │
   │  reports (Typst) ◄─────┘             │    └──────▲───────┘   │ local bundle │
   └──────────────────────────────────────┘           │           └──────────────┘
          ▲                 ▲                         │
   macOS visualiser     browser                 velocity jobs (client)
   (L10, gRPC + HTTP)   (L10, HTTP)             macOS visualiser (annotation)

   Rule: edge and hub share one schema; a worker never writes the hub's database;
         a job carries build version, tuning fingerprint and capture digest;
         kinds are compiled into the binary that runs them.
```

### What has to move for the roles to be real

| Move                                                               | Why                                                                                                       | Owner  | Effort |
| ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------- | ------ | ------ |
| Delete `internal/lidar/aliases.go`; logging into `logutil`         | Ingest links SQLite and tailsql through four logging functions                                            | Appius | `S`    |
| Fix the five upward edges (C2)                                     | L1 depends on L3 and db; L6 on L8; db on l3grid                                                           | Appius | `S`    |
| Layer rule and role closures as a `go list` test in `make lint-go` | Nothing enforces the rules the docs state                                                                 | Appius | `S`    |
| Frame model out of `l9endpoints`; recorder beside `storage/vrlog`  | 37 packages link gRPC                                                                                     | Appius | `M`    |
| Split the root embed package                                       | Evaluation tools need `web/build` to compile                                                              | Appius | `S`    |
| HTTP client out of `internal/lidar/server`                         | `tune` links the server                                                                                   | Appius | `S`    |
| One queue implementation with an HTTP claim API; in-binary kinds   | Three queues; the worker compiles the repository (deferred to the vocabulary Phase 1 exit, D-30 and D-27) | Appius | `M`    |
| `Service` interface and errgroup supervisor in the serve root      | Seven lifecycle signatures; HTTP owns ingest                                                              | Appius | `M`    |
| Transit worker from `internal/db` to `internal/radar`              | A worker inside a storage package                                                                         | Appius | `S`    |
| LiDAR tables and admin routes out of `internal/db`                 | The core package carries LiDAR and Tailscale                                                              | Appius | `M`    |
| Defaults injected from the root; no `../..` walking in layers      | Hidden state; `LIDAR_TIMESTAMP_MODE` read in L1                                                           | Appius | `S`    |
| `nopcap` build target for hub and clients                          | Cross-compiling needs Docker only because of libpcap                                                      | Appius | `S`    |

### Failure registry

| Component              | How it fails                                | What the system does                                                                                                        |
| ---------------------- | ------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| edge ingest            | Sensor unplugged; UDP silence; serial reset | Supervisor restarts the ingest service with backoff; the API reports the source state; storage and reports keep serving     |
| edge storage           | Disk full; WAL cannot checkpoint            | Ingest drops with a counted record (the VRLOG plan's loss record); the API serves read-only; device reports it              |
| hub queue              | Hub down while a worker runs                | The worker finishes into its local bundle and retries the result; a claim lease expires and the job becomes claimable again |
| worker                 | Crash mid-job; wrong build                  | Lease expiry; the job records the build version it ran under and refuses a mismatched kind                                  |
| worker reproducibility | Same job, different rows on another host    | host0003 measures it; until it passes, a job is comparable only within one host class                                       |
| gRPC publisher         | Client falls behind                         | Per-client drop accounting (already planned in v0.5.5); never blocks ingest                                                 |
| reports                | Typst fails                                 | The API returns the error with the data.json it built; nothing else is affected                                             |
| clients                | Hub unreachable                             | `jobs` and the macOS app show the last known state; nothing is written                                                      |

### Build and release of roles

One `make build-velocity` as now; one static artefact per architecture as now. The closures are
asserted in CI on every PR. A `nopcap` build exists for hub and clients so that a Mac can
cross-compile a hub for the NAS without Docker; whether to ship it is host0001's question. No
role gets its own binary, image or installer.

## Architecture decision records (proposed)

Decided on October 20, 2026: all five were amended and entered as D-30 to D-34, and D-35
withdraws TicTacTail; see [Decisions and expansions](#decisions-and-expansions-october-20-2026).
The table records the proposals as reviewed.

| ADR   | Decision                                                                                                                                                                                                                                                                                                                                                                      | Alternatives considered                                                   | Evidenced by                                                 |
| ----- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- | ------------------------------------------------------------ |
| ADR-1 | One binary carries the roles edge, hub, worker and the clients device and jobs; a role is a composition root plus a CI-asserted closure; kinds are compiled into the binary that runs them; restates D-09                                                                                                                                                                     | Binaries per role; the NAS as a separate hub authority                    | The closure test in `make lint-go`                           |
| ADR-2 | Renames are one-release cutovers that keep D-20: an old name survives only while something the release does not replace depends on it (the macOS app, a device file the upgrade leaves alone, data at rest, a published URL), as a registered alias, a migration or a redirect; people get tombstones, not aliases; the vocabulary programme is cut to clip, sites and labels | The eighteen-item alias programme; one-release aliases on the API and CLI | The compatibility register test                              |
| ADR-3 | Planning limits: twelve Active plans, twelve items per release bucket, two sentences per item, sprint sub-buckets live in programme plans; a version is tagged from `main`, not promised to a theme                                                                                                                                                                           | Themed versions; unlimited plans                                          | `report-plan-hygiene` and the backlog lint                   |
| ADR-4 | A hub doc records shipped behaviour; a plan records intent; a hub stub for an unbuilt feature is removed                                                                                                                                                                                                                                                                      | Hub-first documentation                                                   | No hub doc with a "Source plan" header and no implementation |
| ADR-5 | The public survey site, its publishing tooling and its assets leave the product repository in two steps; `velocity scene export` is the contract                                                                                                                                                                                                                              | Keep the monorepo; LFS only                                               | No scene assets in git; the image carries a pointer          |

## Decisions and expansions, October 20, 2026

Ruth's pass decided the ADRs, the audit's states and six arbitrations; Flo's pass re-baselined the
backlog. The decisions are in [DECISIONS.md](../DECISIONS.md) as D-30 to D-35, with seven entries
restated and a `Kept by:` line on every audited entry. Where they differ from this review's
proposals, the decisions govern.

### What Ruth decided against this review's proposals

| This review proposed                                                                            | Decided                                                                                                                                                |
| ----------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Tag `main` as v0.5.2 (P1)                                                                       | Cut v0.5.1 from `main`: there is no v0.5.1 tag or release, the prerelease line users follow is 0.5.1, and the latest full release is v0.5.0 from March |
| Sprint sub-buckets and narrative move into plans; the backlog shows the next twelve (P2, ADR-3) | Narrative and sequence may move to plans; every outstanding item stays in BACKLOG.md; overflow moves to the next slot, never out (D-32)                |
| Twelve Active plans (P3)                                                                        | Twelve active bodies of work, counting `In progress` as active and a programme with its phase plans once; 18 are active today                          |
| Hub docs record only shipped behaviour (ADR-4)                                                  | A hub doc states what ships and may carry a design if it says so; a stub presenting an unbuilt feature as present is removed (D-33)                    |
| The scene-player package first, then the site leaves (ADR-5)                                    | The asset move first; the package only when the external repository needs it, pinned to a revision (D-34)                                              |
| One queue in v0.5.8 (C4)                                                                        | Design adopted (D-30); implementation after the vocabulary Phase 1 exit, as D-27 records; no new source-built worker kinds meanwhile                   |
| Three cutovers including the sites merge (P4)                                                   | Three Phase 1 renames, terminology only; the sites merge and one label table are Phase 2 (D-31)                                                        |
| TicTacTail retired from the backlog                                                             | Withdrawn as D-35, superseding D-23                                                                                                                    |

### Where each recommended option went

Ruth applied the split test to each and found an existing home for every one, so none needed a
new plan, and five drafted plans were folded back before merge.

| Option                       | Home                                                                                                                                                                                                                                                                                            |
| ---------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| P1 release cadence           | [Work unit F](deploy-single-binary-image-consolidation-plan.md#work-unit-f-cut-v051-and-a-cadence-after-it-d-32-s) of the single-binary image consolidation plan: the v0.5.1 cut under six conditions, and the cadence after it                                                                 |
| P2 and P3 planning limits    | D-32 and D-33; the statuses and limits in [canonical-plan-graduation.md](../platform/architecture/canonical-plan-graduation.md#statuses-and-limits-d-32); the backlog header; the plan template; this review's Flo items for the regrooming                                                     |
| P4 vocabulary cutovers       | [Phase 1 under D-31](platform-vocabulary-and-data-model-plan.md#phase-1-under-d-31-three-renames) in the vocabulary programme plan, with its sub-plans' statuses updated                                                                                                                        |
| P5 decision keeping and D-20 | `Kept by:` lines in DECISIONS.md; the [decision audit](../platform/operations/decision-audit.md) and the [compatibility register](../platform/operations/compatibility-register.md); the plan template's `Decisions` line                                                                       |
| P6 experiment fingerprints   | [Section 12](platform-experiment-queue-plan.md#12-keeping-a-spec-true-to-what-ran) of the experiment queue plan: `approved` removed, arms pinned at creation                                                                                                                                    |
| C1 roles as closures         | [Work unit G](deploy-single-binary-image-consolidation-plan.md#work-unit-g-roles-as-asserted-closures-d-30-m) of the same consolidation plan, and the decided design in [go-package-structure.md](../platform/architecture/go-package-structure.md#roles-in-the-one-binary-decided-design-d-30) |
| C4 one job queue             | [Section 12](lidar-worker-pool-and-results-hub-plan.md#12-one-queue-kinds-in-the-binary-d-30) of the worker pool plan, now the single active plan for the queue; the job queue map is superseded into it                                                                                        |

### Open questions for the owner

1. Whether a declared breaking change in 0.x raises the minor version. It does not block the v0.5.1 cut.
2. #503 (rebase or close) and the remainder of #290 (an operator validation), both moved to the next slot before the cut.
3. Retire `grid-heatmap` (the default under restated D-17), or keep a Go-served grid view.
4. D-16's v1.0 placement, after posted speed limits.
5. Which scene data may be public, before the survey site leaves (D-34 step 2).
6. The cadence bound: eight weeks of unreleased user-visible change is the proposed default.
7. When the NAS hub is deployed.
8. D-25's persona length: restate the bound or trim the personas; run the drift check in CI meanwhile.
9. The seven unaudited decisions (D-06, D-10, D-12, D-13, D-15, D-28, D-29), at the first release audit.

## Proposed backlog changes

For Flo's planning review. The governance rule holds: nothing is deleted; items are split,
consolidated, moved or completed.

### By bucket

| Bucket                      | Proposal                                                                                                                                                                                                                                                                  |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| v0.5.1                      | Decide #503 (rebase or close); move #290's remainder to v0.8.0 with the auto-detect item; close the bucket                                                                                                                                                                |
| v0.5.2                      | Retitle to what `main` ships (consolidated L3, published box, label sets, replays from nothing); tag it; move the five sprint sub-buckets into a "following distance programme" plan with the 0.5.2 sprint plan as its checklist; keep at most twelve items in the bucket |
| v0.5.3 to v0.5.7            | Keep as programme phases under the same plan; the backlog shows only the next twelve                                                                                                                                                                                      |
| v0.5.8 (hygiene)            | Pull in: the deletions (rank 1 to 3), the gate and the moves (4 to 7, 16, 17), vocabulary Item 11, #252 surface consolidation from v0.7.0; push out: typed UUIDs, maths coherence Items 1, 4 and 5, `time.Sleep` reduction                                                |
| v0.5.9                      | Add the lifecycle, SQL boundary, logging, configuration, clock and deterministic identity items (rank 9 to 15)                                                                                                                                                            |
| v0.5.10, v0.5.11            | Vocabulary Items 7 and 8 become the labels cutover; Items 9 and 11 are absorbed by the one-queue item; the rest wait for a consumer                                                                                                                                       |
| v0.6.0                      | The Project B item's deploy targets and `cmd/deploy` are already gone; close it with the v0.5.1 PR that removed them once its docs task is checked                                                                                                                        |
| v0.6.4 macOS local server   | Keep, after the roles: the Mac runs a hub and a worker locally, which is what the plan wants                                                                                                                                                                              |
| v0.6.5 design system        | Park until v0.7.0                                                                                                                                                                                                                                                         |
| v0.6.6 web scene publishing | Add the two extraction steps of ADR-5; the asset move comes first                                                                                                                                                                                                         |
| v0.7.0 united frontend      | #252 moves to v0.5.8; the rest stays                                                                                                                                                                                                                                      |
| v0.8.0                      | Retire TicTacTail Phase 1 to deferred (D-23 superseded)                                                                                                                                                                                                                   |
| v0.9.0                      | Replace "coverage ≥ 95.5 %" with boundary tests and a serve smoke test                                                                                                                                                                                                    |
| v∞                          | Add the parked plans with their one-line reasons                                                                                                                                                                                                                          |

### New items

Added to v0.5.8 in this PR, each linking back here:

- Role closure and layer gate in `make lint-go`
- Delete the alias package and fix the six layer edges
- Frame model out of the gRPC package
- Split the root embed package
- Delete the dead code and dead tables the review lists
- Experiment specs pin the defaults' fingerprint; one status vocabulary
- Make targets to a budget of 150 (split from the v0.5.10 consolidation item, which keeps the stats surfaces and CLI flags)
- Compatibility register for D-20, and the removal of the shims no exception covers
- Decision audit kept current: Kept-by lines, states, and plans that cite decisions
- Scene assets out of git and out of the image (v0.6.6)

### Items to merge

- cmd-server-decomposition and go-cmd-extraction: one plan, one item
- Worker pool, job queue implementation and vocabulary Item 11: one item, "one queue"
- Simplification programme and the Makefile trim: the trim is its own v0.5.8 item (Project F of the simplification plan)

## Proposed experiments

Three specs in a new `host` area (roles, hosts and reproducibility), written under the registry's
rules with their rule before any run. They put numbers on the two questions this review cannot
settle by reading: whether a role-scoped build is worth more than the gate, and whether a worker
reproduces a result across hosts.

| Spec     | Question                                                                                                                                   | Rule in one line                                                                                                              |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------- |
| host0001 | What does each role cost on the Pi 5 and the Mac: the one binary against builds without Typst, without pcap, and worker-only               | Keep one binary unless the edge build shrinks by a quarter and idle memory by a sixth; the closure gate is adopted regardless |
| host0002 | Does a kind compiled into the binary reproduce the tool the worker builds from a checkout, row for row, and how much sooner does it answer | Adopt in-binary kinds when the rows are identical on kirk0 and three corpus cases and time to first result at least halves    |
| host0003 | Does one capture under one fingerprint give the same rows on the Mac, the blade and the Pi                                                 | Identical track rows across hosts, or the difference is attributed and the deterministic identity item moves up               |

Updates to existing specs, for their owner rather than this PR: l3bg0009's arms describe the
pre-consolidation L3 and should be restated against l3bg0025; perf0002's note and three reports
cite a parameter set that was deleted on adoption and should name the commit instead; the 31
answered specs whose arm is the defaults file gain the fingerprint they ran under once the
validator carries the field.

## Handoff

| Who    | What                                                                                                                                                             |
| ------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Flo    | The backlog re-baseline above; the v0.5.2 tag decision with Ruth; the WIP limit; the parking pass over the 80 unstarted plans                                    |
| Appius | The moves table, in the order given; the merged cmd/server plan; the one-queue design against host0002; the closure test first, because it keeps the rest honest |
| Ruth   | ADR-1 to ADR-5 as D-30 to D-34 with Kept-by lines; the states from the P5 audit; restating D-04, D-07, D-16 and D-22; the fate of D-09, D-14, D-20 and D-23      |
| Euler  | host0003's attribution method when rows differ (float order, clock, goroutine scheduling)                                                                        |
| Malory | The worker's claim API before it is built: token scope, what a result bundle may contain, and that a worker can never write the hub's database                   |
| Terry  | The backlog item lint's wording and the `Parked` status in the plan hygiene docs                                                                                 |

## Dependencies

- The closure test (C2) precedes every move; without it the moves regress.
- The one-queue item precedes host0002; host0001 and host0003 need only a Pi 5 and the static build.
- The clip cutover precedes the survey export and the sites cutover precedes every map item.
- The scene-player package precedes the survey site's extraction.

## Risks

| Risk                                                               | Likelihood | Impact | Mitigation                                                                                |
| ------------------------------------------------------------------ | ---------- | ------ | ----------------------------------------------------------------------------------------- |
| The deletions remove something a shell lane or a Mac workflow uses | Medium     | Low    | Each deletion PR lists its consumers from the surveys; the Mac workflows run before merge |
| The moves collide with the perception branches in flight           | High       | Medium | Land the gate first; the moves are mechanical and small; rebase cost is known             |
| Three cutovers are three breaking releases                         | Certain    | Low    | Each is announced in the changelog; aliases on the public API and CLI for one release     |
| The survey site extraction stalls on the asset move                | Medium     | Low    | The asset move is independent and first                                                   |
| Role closures make a worker-only feature harder to add             | Low        | Low    | The test names the closure; widening it is a one-line, reviewed change                    |
| host0001 shows no benefit from role builds                         | Likely     | None   | That is a result: one build, one binary, and the gate                                     |

## Checklist

### Complete

- [x] Four read-only surveys: unfinished migrations and renames; package topology and role closures; UI and tooling surfaces; experiment infrastructure
- [x] Findings, anti-patterns, the ranked list, the boundary decisions, the roles design and the ADRs
- [x] Three experiment specs in the `host` area; backlog items under v0.5.8; devlog entry
- [x] P5 explored: a decision audit, the D-20 consumer test and register, and a disposition for every Make target

### Outstanding

- [ ] Flo: backlog re-baseline and the parking pass (`M`)
- [ ] Ruth: ADR-1 to ADR-5 decided (`S`)
- [ ] Ruth: decision states and Kept-by lines from the P5 audit (`S`)
- [ ] Appius: the Make trim (Project F) and the compatibility register with its test (`M`)
- [ ] Appius: the closure test and the first three moves (`M`)
- [ ] Appius: one queue, in-binary kinds, host0002 run (`M`)
- [ ] The v0.5.2 tag (`S`)
- [ ] host0001 and host0003 run on the Pi 5 (`M`)

### Deferred

- [ ] Role-scoped binaries: only if host0001 says so
- [ ] The survey site repository: after the scene-player package exists

### Accepted residuals (no action planned)

- [ ] The macOS visualiser stays in the repository until the proto and the annotation API are versioned
- [ ] Two Eleventy sites remain; their purposes differ
