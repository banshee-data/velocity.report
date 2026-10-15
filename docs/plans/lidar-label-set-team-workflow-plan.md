# Label sets for a team: one tree, mergeable edits, proposals from Go, no server to edit

kirk0's label set is published, with a view a researcher can open, edit and propose back as a
Hub pull request ([capture-bound annotation plan](lidar-capture-bound-annotation-format-plan.md),
#744). Doing that today takes a 74 GB clone, a running server pointed at one capture's folder,
two commands before every commit, a hand-resolved conflict on every merge, and a ten-minute wait
for proposals that each labeller computes alone. This plan removes each of those before labelling
is handed to more than one person. It changes the tree a label set lives in, teaches Git to merge
label sets, moves the object proposer to Go, adds a diff, a partial clone, editing without a
server, a reader-level field, and the checks a labeller sees before a reviewer does.

- **Status:** In progress: A to D built on stacked branches (2026-10-14); E and F planned (§9)
- **Layers:** L9 Endpoints (server), L10 Clients (macOS annotation window), annotation storage,
  dataset publication
- **Target:** before a second labeller is given a view; items 7, 1, 4 and 2 first (§4)
- **Companion plans:** [capture-bound annotation format](lidar-capture-bound-annotation-format-plan.md),
  [physical reference review](lidar-physical-reference-review-plan.md),
  [point annotation and object dataset](lidar-point-annotation-and-object-dataset-plan.md)
- **Canonical:** [label sets](../lidar/operations/label-sets.md),
  [dataset layout](../../tools/s2-archive/huggingface-dataset-layout.md)

## 1. What a labeller meets today

Measured on 2026-10-14 with kirk0's published view and the two new VRU packs
(`e44ae459…`, Columbus at Broadway; `10715f01…`, Embarcadero at Folsom).

| Step                   | Today                                                                                                                                                                                                                                                     |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Getting the files      | A full clone: 74 GB of captures for a 65 MB label set and view                                                                                                                                                                                            |
| Finding the view       | `labels/<stem>/kirk0-reference/` beside `labels/<stem>/ad8b9438-c1d4-40f0-bd0a-f1852c984569-20260929-184718.156560000-v2/`; nothing in the names says which is the set, which is the view, or that one belongs to the other                               |
| Opening it             | The server must run with `--lidar-annotation-dir=<clone>/labels/<stem>`, one capture at a time, because it lists packs one level down only (`handleAnnotationPacks`, `resolvePhysicalPack`)                                                               |
| Proposals              | Swift computes them in the window: 688 proposals in 114 s and 1,016 in 82 s for the two VRU packs; the layers (40 MB and 33 MB of JSON) are gitignored, so every labeller recomputes them, and two labellers can get different ones from different builds |
| Before each commit     | `annotation-reference repin` then `labels check-view`, by hand; a one-mask edit re-pins 25 reviews, 56 lines of `physical-references.json`                                                                                                                |
| Merging two branches   | `labels.json` and `physical-references.json` always conflict; the fix is `checkout --ours`, `rehead`, `repin`, `check-view`, from the guide                                                                                                               |
| Reading a pull request | Lines of ndjson; nothing says "two pedestrians added, one mask changed in windows 200–299"                                                                                                                                                                |
| An old build           | `schema_version` must match exactly, so any addition to `labels.json` refuses every older reader, and a new reader cannot say which older ones are safe                                                                                                   |
| Mistakes               | Found by a reviewer, after the pull request, or by `rehead` refusing a return given to two objects                                                                                                                                                        |

## 2. Decisions proposed

1. **One tree, nested.** A view lives inside its label set, and both live under the capture:
   `<root>/<capture>/<set>/views/<view>/`. The same layout locally and in the dataset, so
   publishing is a copy and the server's `--lidar-annotation-dir` is the root of every capture.
2. **Git finishes a merge, not the labeller.** Merge drivers and a `pre-merge-commit` hook run
   `rehead`, the re-pin and `check-view`; a review pins the masks it rests on, not a revision,
   so an unrelated edit no longer touches `physical-references.json`.
3. **Proposals are computed once, in Go, and published with the view.** The Swift proposer is
   ported byte for byte, proved by parity fixtures, then removed. Layers go through LFS, so a
   clone has them and a server is not needed to see them.
4. **A pull request describes itself.** `labels diff` reads two states, from directories or Git
   revisions, and prints what changed in objects and masks, in Markdown for the Hub.
5. **A labeller clones one capture's labels, not the corpus**, and edits it with the window alone.
6. **A file says the oldest reader that can read it.** `min_reader` in `labels.json`,
   `manifest.json` and `physical-references.json`, separate from `schema_version`.
7. **The window warns, never blocks.** The same rules, by id, run in the window and in
   `labels check`.

Clean cutovers, as for version 2: the nested layout and mask-pinned reviews replace the old
forms; a `labels nest` command moves what exists, and nothing is read in two layouts.

## 3. The work

Sizes are in §4. Every change lands with its tests in the same commit.

### 3.1 Nested layout (item 1)

**Today.** Locally, sets are in `<annotation dir>/labels/<set>/` and packs beside `labels/`,
linked by `label-set-link.json` (`../labels/<set>`). In the dataset, set and view are siblings
under `labels/<stem>/`, linked by `../<set>`. The server lists packs one level under
`--lidar-annotation-dir` and resolves a handle of at most two parts
([segment_packs_api.go](../../internal/lidar/server/segment_packs_api.go),
[physical_reference_api.go](../../internal/lidar/server/physical_reference_api.go)). The window
creates sets in `<annotation dir>/labels`, scans it for `attach` candidates, and grants the
folder its link resolves to
([LabelSetMembership.swift](../../tools/visualiser-macos/VelocityVisualiser/Annotation/LabelSetMembership.swift),
[AnnotationWindow.swift](../../tools/visualiser-macos/VelocityVisualiser/UI/AnnotationWindow.swift)).

**Layout.**

```text
<root>/                                  --lidar-annotation-dir locally; labels/ in the dataset
  <capture stem>/
    <set name>/                          the label set: labels.json, head files, masks/
      views/<view>/                      a pack of this set: manifest, points, packets, returns,
                                         background, physical-references.json, proposals/
                                         label-set-link.json -> ../..
    packs/<run>/                         a pack not yet in a set (local only; never published)
    splits/                              frozen splits, as today
```

- The view name is `<yyyymmdd>-<run id, first 8>`, e.g. `20260929-ad8b9438`; the run id and
  the pack digest stay in `manifest.json`. The `-v2` suffix goes.
- The capture stem is the raw file's stem. Locally that is `kirk0` for the kirk0 packs; the
  staging script maps it to the dataset row's stem by `raw_sha256`, as it does today.
- An export lands in `packs/<run>/`. The first save, or `attach`, creates or opens the set and
  moves the pack to `views/<view>/` by rename on the same volume, then writes the link. The
  window grants `<root>/<capture stem>/` once, which covers the move.

**Changes.**

- Go `labelset`: `Nest(root)` moves every set and pack into the layout and rewrites links;
  `labels nest --root DIR [--dry-run]` prints the moves first. `Create`, `Attach`, `Publish`,
  `PublishView` and `CheckView` take the new paths; the link is validated as `../..`.
- Server: the pack listing walks `<stem>/<set>/views/*` and `<stem>/packs/*`; a handle is the
  path relative to the root, validated against the layout, so `--lidar-annotation-dir` is the
  root locally and `<clone>/labels` in the dataset. The export endpoint and the Segments clip
  job write to `packs/`. The Segments page shows capture, set and view.
- Swift: `labelsDirectory` becomes the capture folder; `create` and `attach` do the move and
  re-open the pack at its new path; `candidates()` scans the capture's sets; the grant is the
  capture folder.
- `stage-label-sets.py`: publishes the set and `views/<view>`, writes `annotation_pack_paths`
  with the new path; one commit in the dataset moves kirk0's published files (Git records the
  renames; the LFS pointers do not change).
- Docs: the layout tables in the guide and the dataset layout; the README's labels section.

**Tests.** Nest on a fixture tree with a linked pack, an unlinked pack and a migrated `-v2`
name; the listing and handle resolution over the nested tree, with a traversal attempt refused;
a Swift first save that moves the pack and still opens it; `check-view` through the `../..`
link; the staging test's expected paths.

**Risk.** Frozen splits may name pack paths: the inventory in `Nest` must find and rewrite them
or refuse. The move happens while the window has the pack open; the rename is atomic, and the
window must swap its URL before the link is written.

### 3.2 Git merge driver and mask-pinned reviews (item 2)

**Why a driver alone is not enough.** Git runs a merge driver on one file with its three
versions; in `merge-ort` every file is merged before the working tree is written, so the driver
for `labels.json` cannot see the merged `masks/` to compute the new head. The head has to be
made after the merge, before the commit.

**Design.**

- `.gitattributes` in the dataset: `labels/**/labels.json merge=labelset-head` and
  `labels/**/physical-references.json merge=labelset-refs`.
- `velocity lidar labels merge-driver head %O %A %B`: writes ours with a `merge` record holding
  theirs' and the base's `{revision, state}`, exit 0. `Open` refuses a manifest with a `merge`
  record ("finish the merge: labels finish-merge"); only `finish-merge` reads it.
- `velocity lidar labels merge-driver refs %O %A %B`: a three-way merge of records by id. A
  record changed on one side wins; a record changed on both sides is a conflict and exits 1,
  with ours and theirs left as `physical-references.json.ours` and `.theirs`.
- `velocity lidar labels finish-merge --root labels/`: for every set with a `merge` record,
  `rehead` over the merged files (refusing a return in two objects), the re-pin, `check-view`,
  then `git add` of the results. The dataset ships `.githooks/pre-merge-commit` that runs it,
  so `git merge` stops uncommitted on any refusal. A rebase or a cherry-pick runs no such hook:
  the guide says to run `finish-merge` by hand, and `check-tree` catches a forgotten one.
- `velocity lidar labels git-setup --repo DIR`: sets `core.hooksPath`, the two drivers and
  `diff.lfs.textconv`. Hooks and drivers are per clone, so this is step one of the guide.
- **Mask-pinned reviews.** `reviewed_against` gains `masks`: a digest over the canonical masks
  of the review's support frames (what `sameMask` compares today). Drift is "the masks digest
  differs", with no history needed, so `RepinUnchangedReviews` has nothing to move after an
  unrelated edit, and `physical-references.json` changes only when a reviewed object's masks
  do. `revision` and `digest` stay as a record of when. The physical-reference schema bumps,
  with `min_reader` (§3.7); `repin` moves existing pins once.
- The Hub's merge button runs no drivers. A pull request that conflicts on `labels.json` is
  rebased by its author, or merged locally by a maintainer and pushed; `check-tree` (§3.4)
  reports a tree with an unfinished merge.

**Tests.** The two-clone merge test in `publish_view_test.go` repeated through the drivers and
the hook with real `git`: a clean merge commits revision N+1 with both edits and no changed
pins; a double-edited record stops the merge. A fixture with a `merge` record is refused by
`Open`. Drift with a mask digest: unchanged masks are not drift; a changed support frame is.

### 3.3 Object proposer in Go, wired into the window (item 3)

**Today.** [ObjectProposer.swift](../../tools/visualiser-macos/VelocityVisualiser/Annotation/ObjectProposer.swift)
(486 lines): a 0.5 m column lattice; `Persistence` marks a voxel fixed clutter when it is
occupied in 80% of frames; `clusters` joins free returns eight-connected in plan, seeded at 8
returns; chains follow clusters frame to frame within 2 m, allowing 3 misses, kept at 5 frames;
`FixedPatches` proposes what persists; `guess` and `guessFixed` name a type. Iteration is sorted,
so a pack proposes the same objects every time. Output is two layers,
`proposals/cluster_chain@1.json` and `persistent_voxels@1.json`, bound to the pack digest
([ProposalLayerStore.swift](../../tools/visualiser-macos/VelocityVisualiser/Annotation/ProposalLayerStore.swift)),
which the window loads on open. The layers are gitignored in the dataset.

**Changes.**

- `internal/lidar/annotation/propose/`: `Persistence`, `Clusters`, `Chains`, `FixedPatches`,
  the type guesses and the layer writer, with the Swift constants and iteration order. Float
  sums are made in the same order as Swift; where `simd` reductions differ, the Go side matches
  them explicitly, because the parity fixture is byte for byte.
- `velocity lidar annotation-pack propose --pack P [--force]` writes the layers; the export
  endpoint and the clip job take `propose: true` and write them with the pack.
- Server: `POST /api/annotations/packs/propose {pack, pack_digest}` runs the proposer and
  reports progress; the window's **Propose objects** calls it when connected and reloads the
  layers from disk. Without a server the button says the layers are absent and how to get
  them (§3.6).
- The layers are published with the view. The gitignore line for `proposals/` goes; a
  `labels/**/proposals/** filter=lfs` rule carries them, since 40 MB of JSON is over the Hub's
  plain-Git limit and no one diffs a layer. `publish-view` copies them and `check-view` checks
  each layer's pack digest. A maintainer refreshes them with `--force` after a proposer change,
  and `algorithm_version` says which proposer made them.
- Swift: `ObjectProposer` is removed once parity holds; `ObjectProposal`, the layer reader and
  the proposal UI stay.

**Tests.** A small synthetic pack under `testdata/` with a parked object, a moving object and
speckle; the Go layers are the fixture and the Swift test asserts the same bytes, as the label
set parity tests do. The two VRU packs reproduce their counts (688 and 1,016 moving) from Go.
The endpoint refuses a digest mismatch, as `resolvePhysicalPack` does.

### 3.4 `labels diff` (item 4)

- `velocity lidar labels diff A B [--format text|markdown|json]`, where A and B are set
  directories, or `--git <rev>..<rev> --path labels/<stem>/<set>` read through `git cat-file`
  (the text is plain Git, so no checkout is needed).
- Reports, from the canonical states: objects added, removed and relabelled; masks added,
  removed and changed, by window range and object, with return counts; domains and bindings
  added; the head revisions. Markdown is the block for a Hub pull request description.
- `--quiet` exits 0 when no label changed, for scripts.
- `velocity lidar labels check-tree --root labels/` runs `check-view` on every view, reports
  an unfinished merge, and prints the diff against a base revision. It is what a Hub-side
  check would run; the bot itself is out of scope (§8).

**Tests.** Diffs over fixtures with each kind of change; the Git mode against a temporary
repository with two commits; the Markdown output as a golden file.

### 3.5 Partial clone (item 5)

The clone's plain Git is small: captures, views and `packets.bin` are Xet pointers. A clone
that skips the smudge, checks out one capture's folder, and pulls that folder's LFS objects is
the whole labelling kit:

```bash
GIT_LFS_SKIP_SMUDGE=1 git clone --no-checkout git@hf.co:datasets/banshee-data/sf-street-speeds.git
cd sf-street-speeds
git sparse-checkout set --no-cone labels/<stem> manifest.json README.md .gitattributes .gitignore .githooks
git checkout main
git lfs pull --include="labels/<stem>/**"
velocity lidar labels git-setup --repo .
```

- `tools/s2-archive/clone-labels.sh <stem> [dir]` runs it, and the guide opens with it.
- Measure what a plain-Git clone transfers, and whether the Hub honours `--filter=blob:none`;
  record both in the guide. Expected: tens of megabytes, then 65 MB of LFS for kirk0.
- `hf download banshee-data/sf-street-speeds --include "labels/<stem>/**"` is the read-only
  form, for scoring without Git.

**Tests.** The script against a local bare repository with the dataset's rules and one
published view: the result passes `check-view`, and nothing outside `labels/<stem>` is
checked out. A `test_clone_labels.py` beside `test_stage_label_sets.py`.

### 3.6 Editing with no server (item 6)

**Today.** Membership saves are local. The server does physical references (save, review,
history, restore, validate, fit), facets, splits, the pack listing and, after §3.3, proposals.

**Changes.**

- Swift writes `physical-references.json` itself: the pinned save, the revision archive, the
  content digest and the mask-pinned review of §3.2, byte for byte with Go, proved by parity
  fixtures. Which of `validate` and `fit` already run in the window is surveyed first
  ([PhysicalFit.swift](../../tools/visualiser-macos/VelocityVisualiser/Annotation/PhysicalFit.swift)
  exists); any that only run in Go are ported or left to the server, which the survey decides.
- **Open dataset folder…** picks a `labels/` root, lists captures, sets and views from the
  tree, and grants that folder; the server's listing is not needed to open a view.
- Proposals are read from the published layers; **Propose objects** without a server explains.
- Facets stay server-only and unpublished: the facet controls are disabled with a reason.
- A status line in the window says what the server is doing for it, or that there is none.

**Tests.** The Swift physical save against the Go fixtures; an end-to-end Swift test that
opens a published view from a temporary dataset folder, edits a mask and a review, saves, and
leaves exactly the files `check-view` expects.

### 3.7 `min_reader` (item 7)

- `labels.json`, a pack's `manifest.json` and `physical-references.json` gain `min_reader`: the
  lowest reader level that reads the file correctly. A writer sets it to the level its
  features need; readers carry a level and refuse `min_reader` above it with the build that
  has it. `schema_version` stays for breaks. Level 1 is the storage layout as published;
  mask-pinned reviews (§3.2) and published layers (§3.3) are level 2.
- Outside the state digest: it lives in the manifest, so no digest, journal or pin moves.
- Go `readManifest` and Swift `LabelSetStore.read` check it; `stage-label-sets.py` and
  `check-view` refuse a set newer than the build they run; the parity fixtures carry it.

### 3.8 Checks as warnings in the window (item 8)

Rules carry stable ids, and `velocity lidar labels check --labels SET [--pack VIEW]
--format json` runs the same rules, so a reviewer's check and a labeller's panel agree.

| Id                  | Warns when                                                                                        |
| ------------------- | ------------------------------------------------------------------------------------------------- |
| `single-window`     | An object has masks in one window only                                                            |
| `gap`               | An object's masks skip more than N windows between two masks                                      |
| `implausible-speed` | An object's centroid moves faster between masks than its class allows (4, 15 and 40 m/s by class) |
| `shared-return`     | One return is in two objects' masks in a frame (`rehead` refuses this; the window says it first)  |
| `outside-domain`    | A mask cites a return outside its domain                                                          |
| `tiny-mask`         | A mask has fewer than N returns, or only uncertain ones                                           |
| `twin`              | Two objects' footprints overlap heavily in consecutive windows: one thing labelled twice          |
| `review-drift`      | A reviewed physical record's masks changed since the review (§3.2)                                |

- Swift: a `LabelChecks` module runs on open and after each save, incrementally for the edited
  objects; a Checks panel lists warnings with counts and jumps to the frame. Saving is never
  blocked.
- Go: the rules in `labelset` and `annotation`, with a fixture per rule; a parity test runs both
  sides over one fixture and compares the ids and counts.

## 4. Sequencing and pull requests

The items change each other's ground, so this order keeps every pull request small enough to
review and lands the cheapest wins first:

| PR  | Items | Why in this order                                                                                                                | Size |
| --- | ----- | -------------------------------------------------------------------------------------------------------------------------------- | ---- |
| A   | 7, 1  | `min_reader` is a day and lets later format changes declare themselves; the layout changes every path the rest of the plan names | L    |
| B   | 4, 2  | The diff is what the merge hook and a pull request report; mask-pinned reviews halve the conflict surface before anyone merges   | L    |
| C   | 5     | The clone recipe depends on the nested paths and the hooks                                                                       | S    |
| D   | 8     | Independent; a labeller sees mistakes before the first team pull request                                                         | M    |
| E   | 3     | The largest port, isolated; its published layers are what F reads                                                                | L    |
| F   | 6     | Needs A's listing-free tree, B's pinning and E's layers                                                                          | L    |

Each pull request branches from `main` after the previous one merges; this branch holds the
plan and the first pull request. The data moves (§5) follow A, B and E and are recorded in an
operations report, not in a pull request.

## 5. Data moves

Nothing here is in Git. Each is done once, with a dry run first and the backup the command
makes.

1. After A: `labels nest` over the local root (five sets, seven packs, the two VRU packs in
   `packs/`); one dataset commit moving kirk0's published set and view, with the README and
   `annotation_pack_paths` updated.
2. After B: `repin` once per set to add mask digests to existing pins; a dataset commit for
   kirk0's view; `.gitattributes`, `.githooks/` and the guide's setup step.
3. After E: layers generated from Go for every local view and published for kirk0's; the
   gitignore line removed.

## 6. Risks

| Risk                                              | Handling                                                                                                                            |
| ------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| The Go proposer does not reproduce Swift's floats | The parity fixture is the gate; where `simd` reductions differ, Go matches their order explicitly, and the VRU counts are the check |
| A clone without the hooks merges wrongly          | `Open` refuses a manifest with a `merge` record, so the result cannot be read, saved or published until `finish-merge` runs         |
| The Hub's merge button bypasses the drivers       | Documented: conflicting pull requests are rebased by the author or merged locally; `check-tree` reports an unfinished merge         |
| The Hub may not honour partial-clone filters      | The recipe works without them, since plain Git is small; the measurement in §3.5 says how small                                     |
| Moving a pack the window has open                 | Rename on the same volume, the capture folder granted beforehand, the window's URL swapped before the link is written               |
| Frozen splits or scripts name the old paths       | `Nest` inventories path references and refuses to move while one is unresolved                                                      |
| Published layers add 70 MB of LFS per view        | Regenerable, so a maintainer can drop and refresh them; they are not in the state digest                                            |

## 7. Open questions

1. Should an export be able to land straight in a set (`label_set` on the export request),
   removing the `packs/` state for the common case? Proposed: later; the move on first save
   covers it with no API change.
2. Does a genuine double edit of one physical record need a merge UI, or is `.ours`/`.theirs`
   with a documented resolution enough for now? Proposed: enough.
3. Which speed limits and gap sizes do the checks use, and are they tuning config
   (`config/tuning.defaults.json`) or constants in the rules? Proposed: constants, named in the
   rule's message, until a second site disagrees.

## 8. Not in scope

- A Hub-side bot that runs `check-tree` on pull requests: it needs hosting; `check-tree` is
  written so that it can be the bot's one command.
- Facets in published views, and a facet re-pin.
- Adjudication between two labellers of the same windows, and a labelling guide with a
  versioned class list: both are workflow, not tooling, and come after a second labeller
  exists.
- Object identity across captures; dataset release versioning.

## 9. Implementation

| PR  | Branch                              | State            |
| --- | ----------------------------------- | ---------------- |
| A   | `dd/lidar/label-sets-layout`        | Built 2026-10-14 |
| B   | `dd/lidar/label-sets-merge`         | Built 2026-10-14 |
| C   | `dd/lidar/label-sets-partial-clone` | Built 2026-10-14 |
| D   | `dd/lidar/label-sets-checks`        | Planned          |
| E   | not started                         | Planned          |
| F   | not started                         | Planned          |

Decided while building A:

- **A first save does not move the pack.** It creates the set in the capture folder and links the
  pack where it is, `../../<set>` from `packs/<run>/`. Moving a pack the window holds open, and a
  pack a clip job's row names, was more risk than the layout is worth; `labels nest` moves linked
  packs into `views/` when nothing is open, and a dataset only ever holds views in `views/`.
- **A clip job's folder stays at the root** as `clip-<job>-<n>/pack/`, a pack position of its
  own, because its row names it and its retries look for it there. Once `nest` moves a linked clip
  pack into `views/`, the server finds it by its segment record and leaves the row alone.
- **View names run to the second**, `<yyyymmdd>-<hhmmss>-<run, first 8>`, so two cuts of one run
  on one day do not collide.
- **An export is written hidden at the root**, `.export-<run>-<time>`, then renamed into its
  capture's `packs/` once the pack says which capture it is.
- **`min_reader` is written only above level 1**, so a file that needs nothing new stays readable
  by the build that merged #744. Level 1 is the only level so far; B's mask-pinned reviews are the
  first reason to raise it.
- **Splits stay in `<root>/splits/`**, not per capture: a split spans captures.
- The Segments page still lists packs by folder; showing capture, set and view there is left out.

Decided while building B:

- **A mask pin is a digest**, of the object's masks at the review's support samples (members,
  uncertain returns, status). When the pinned revision is still retained, drift names the sample
  that changed; without it, drift says the reviewed samples changed. A pin written before level 2
  is judged by its revision, as before.
- **`repin` upgrades pins**: a review pinned to the current membership without masks gets them,
  so one `repin` per view brings kirk0's 25 reviews to level 2.
- **The head driver takes the other branch's head when only it moved**, and records a merge only
  when both did. It adds the other branch's verified files and any field only it changed.
- **Two hooks, not one.** Git runs `pre-merge-commit` for a merge that commits itself and
  `pre-commit` for one committed after a conflict; both run `finish-merge`, which does nothing
  unless a `labels.json` records a merge. The hooks name the `velocity` binary `git-setup` ran
  from, so they go in the clone's own `.git/hooks`, not a tracked `.githooks/` as §3.2 had it.
- **A conflicting physical reference leaves `.theirs` beside it**, the other branch's whole file,
  rather than conflict markers in JSON.

Decided while building C:

- **No partial clone.** The Hub accepts `--filter=blob:none` and then fails to fetch the blobs
  it left out ("expected 'packfile'"), so the script takes every plain-Git blob: a 2 MB pack,
  since the captures are LFS/Xet pointers in Git. Measured for kirk0: 111 MB in 18 s.
- **The hooks are not part of the clone.** The sparse checkout leaves out `.githooks/`; the hooks
  `git-setup` writes go in `.git/hooks` (B).
