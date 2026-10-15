# Label sets

A label set holds reference objects and their per-frame membership for one capture. It cites
returns by `(packet key, block, channel)`, where the packet key is a digest of the UDP payload the
sensor sent. So one label set applies to every run of the capture, at any settings, and to every
file that holds its packets: the source roll, a clipped excerpt, or the published PCAPNG. History is
a journal of deltas, one line per save, not a full copy per save.

- **Format:** [`internal/lidar/annotation/labelset/README.md`](../../../internal/lidar/annotation/labelset/README.md)
- **Design:** [capture-bound annotation format plan](../../plans/lidar-capture-bound-annotation-format-plan.md)

## Where things live

One layout, locally and in the Hugging Face dataset, where `<root>` is `labels/`:

```text
<root>/
  <capture>/                 one folder per capture, named for its file's stem
    <set>/                   a label set: labels.json, the head files (masks split into masks/ by
                             window), journal.ndjson and .lock
      views/<view>/          a pack linked to the set; label-set-link.json -> ../..
    packs/<run>/             a pack not in a set yet (local only)
  clip-<job>-<n>/pack/       a Segments clip job's pack, in the job's folder beside its recording
  splits/                    frozen splits
```

| Path                  | What it is                                                                                            |
| --------------------- | ----------------------------------------------------------------------------------------------------- |
| a pack                | One run's foreground of a capture, with `packets.bin` and `returns.bin`                               |
| `label-set-link.json` | In a pack: names the label set holding its membership, by id and path relative to the pack            |
| `<view>`              | `<cut time>-<run>`, for example `20260929-184718-ad8b9438`; the run id and digest are in its manifest |

Locally, `<root>` is the server's `--lidar-annotation-dir` (`LIDAR_ANNOTATION_DIR` in `local.mk`). An
export lands in its capture's `packs/`. The first save from a pack creates its label set in the
capture folder and links the pack where it is; `labels nest` later moves linked packs into their
set's `views/`. A clip job's folder stays where the job wrote it, since the job's row names it;
once its pack is linked, `nest` moves the pack and the server finds it again by its segment.

Annotation is downstream of foreground separation. Every mask names, as its domain, the foreground
extraction it was made on. A return outside that foreground was never shown to the labeller and is
unlabelled for scoring, never a negative. Label on a generous foreground: what it leaves out, a
label set cannot measure.

## Commands

All are `velocity lidar labels <command>`; built through `make`, so records name the build.

| Command        | Use                                                                                                                                                       |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `create`       | Start a label set from a pack and link the pack                                                                                                           |
| `attach`       | Add another pack of the same capture as a second domain and link it                                                                                       |
| `bind`         | Check a capture file holds the label set's packets; record its SHA-256 in `applies_to`. Changes no labels                                                 |
| `verify`       | Replay the journal and check every revision's state digest                                                                                                |
| `info`         | Counts, head revision and digest, bindings, and where a migrated set came from                                                                            |
| `project`      | Print a linked pack's membership in the pack's own samples and point indices, at the head or `--revision N`                                               |
| `publish`      | Copy the current state, without the journal, to an empty directory; `--replace` refreshes a published copy that is behind and has not been edited         |
| `publish-view` | Copy a linked pack beside a published label set as its view, with its data and physical references, after checking the view as a fresh copy would hold it |
| `check-view`   | Check a view and its label set as a fresh copy of their tracked files would hold them, with no history behind them                                        |
| `rehead`       | After a Git merge, make the head files on disk the next revision; keep your own `labels.json` in the conflict                                             |
| `relayout`     | Rewrite the head in the storage layout, from the single-file layout or with another `--window-chunk`; the state is unchanged                              |
| `nest`         | Move a tree into the layout: sets into their capture folder, linked packs into their set's `views/`, the rest into `packs/`; `--dry-run` prints the moves |
| `census`       | Count packet keys in capture files and report duplicates                                                                                                  |

Everything that reads a pack's membership reads it through the label set: splits, physical
references, features, the per-frame and near-face evaluators, and the server's pack listing. It is
written through the label set too, by the macOS window's save or, in Go, `labelset.SaveMembership`.
The first save to a pack with no link creates a label set in its capture folder,
`<root>/<capture>/<capture stem>-<id>`, and links the pack. Freezing a split over a linked
pack binds its case to the label set's verified captures by SHA-256, so a replay of any verified
file holds its role and a different file does not.

## Where it shows

| Place                                   | What it shows                                                                                                                                                                                                    |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| macOS annotation window, label set line | The label set's name and revision; each verified capture file with a short SHA-256, packets found and frames covered (or the `bind` command when there are none); and, for a migrated set, the pack it came from |
| macOS annotation window, replay check   | With "Follow the main view" on: whether the main view's current frame holds this label set's packets (same capture, partly, a different capture, or cannot check when frames carry no packet keys)               |
| macOS annotation window, no label set   | "No label set yet": the first save makes one in the capture folder; buttons attach the pack to an existing label set of the same capture, with how many packets they share                                       |
| macOS annotation window, version 1 pack | Refused when opened, with where its labels went and the `annotation-v1-reader` tag that keeps the last build that reads it                                                                                       |
| Web, Segments page, Annotation packs    | Every pack: its label set and revision, or that it has no labels yet; reviewed work; and each verified capture file                                                                                              |

The replay check compares the packet keys on the gRPC stream with the label set's packet table, so it
is a check of content, whichever verified file the main view replays. A live source, a VRLOG recorded
before return identity, or an older server sends no keys, and the window says it cannot check.

## Version 1 packs

Version 1 packs had no returns, and kept their membership in the pack as `annotations.json` with a
full copy per revision. All of them were migrated to label sets on 2026-10-12, by an exact mapping
from each point to its decoded return; the method and results are in the
[migration report](annotation-label-set-migration.md). This build reads version 2 only: the Go
reader, the server's listing and the macOS window refuse a version 1 pack and say why. The
originals are archived in `work/annotation-v1/` on the LiDAR volume, outside the repository, and the
git tag `annotation-v1-reader` keeps the last build that reads them and the `labels migrate`
command that carried them across.

## Binding and publishing

`bind` reads a whole capture file, so bind once per file and expect it to take as long as reading
the file. A file that holds none of the packets, or holds one twice, is refused.

`tools/s2-archive/stage-label-sets.py` publishes label sets into the Hugging Face dataset, joined to
a capture row only by `raw_sha256`. See the [dataset layout](../../../tools/s2-archive/huggingface-dataset-layout.md#label-sets).
A set already published at the same head is left as it is, and `--replace` refreshes one that is
behind its source. It never writes over a copy someone has edited. After `labels nest` has moved published files in a
clone, `--refresh-paths` rewrites every row's `label_set_paths` and `annotation_pack_paths` from
what is under `labels/`.

### Publishing a view

A label set holds no points, so nothing can be drawn or edited from it alone. A **view** is a pack
published in its label set's `views/` folder: `--view PACK` stages the pack's label set (found
through its link) and runs `labels publish-view`. The view gets the pack's data and physical
references, and its link is rewritten to the set it sits in:

```text
labels/<capture stem>/
  <label set name>/      the label set: labels.json and the head files
    views/<view>/        its view: manifest, samples, points, packets, returns, background,
                         physical-references.json, label-set-link.json -> ../..
```

Publish the run the labels were drawn on, so the view shows every labelled return. `publish-view`
refuses a pack whose foreground is not one of the set's domains, since the first save from it
would add that foreground whole: `attach` the pack and republish the set first. It also refuses
any physical reference pinned to a revision a copy without history cannot rebuild. Facets,
proposal layers, locks and revision archives stay behind.

### Editing from the dataset

Anyone with the dataset and a build of this repository can edit a published label set. Its files
on disk are then the new state, and the change is a small commit. The label set's text is plain
Git, one record per line, so the Hub shows the edited lines; only `packets.bin` and the view's
binary files go through LFS/Xet.

1. Clone the dataset with git and Git Xet installed.
2. `git checkout -b <your branch>`.
3. Start the server with `--lidar-annotation-dir=<clone>/labels`. The window saves membership
   itself, but physical-reference and facet saves go through the server, which finds every view of
   every capture under that directory.
4. In the macOS window, **Open Annotation Pack…** the view, and grant `labels/<capture stem>` when
   asked: the label set around the view is written there.
5. Edit and save. The first save of a published set copies its head to `journal-base/` and starts a
   local journal; both are gitignored, with the locks, the commit marker and the revision archives
   the editors write.
6. Before committing, run
   `velocity lidar annotation-reference repin --pack <view> --author <you>`, then
   `velocity lidar labels check-view --pack <view>`. A review pins the membership it was made
   against, and a later copy has no history to rebuild it. `repin` moves every review whose frames
   the edit did not touch to the new head. `check-view` checks the view as a fresh copy of the
   tracked files would hold, and names any review that needs doing again.
7. `git status` lists `labels.json`, the chunk files the edit touched, and the view's
   `physical-references.json`. Commit them. To propose the change, open a pull request on the
   dataset's Hub page, then `git push origin <your branch>:refs/pr/<number>`.

A one-mask edit of kirk0's view changes three lines of `labels.json` (the head), one line of
`masks/000000-000099.ndjson`, and 56 lines of `physical-references.json` (the re-pin of 25
reviews).

#### Merging another branch

Git merges the label set's files line by line, so two people who edited different masks merge
cleanly there. `labels.json` conflicts, because each branch moved the head, and so does
`physical-references.json` when both re-pinned their reviews. To finish the merge:

1. Keep your own branch's copy of both files, then make the merged files the next revision:

   ```bash
   git checkout --ours -- "$SET/labels.json" "$VIEW/physical-references.json"
   velocity lidar labels rehead --labels "$SET" --author "$NAME"
   ```

   `rehead` refuses a merge that gives one return to two objects, and adds the merge to your local
   journal.

2. `annotation-reference repin`, then `labels check-view`, as for any edit. Commit.

Measured on kirk0: two clones each removed one return from a different mask in the same chunk
file. Git merged the chunk itself and conflicted on the two files above. `rehead` made revision
2,235, holding both edits. `repin` moved all 25 reviews, and `check-view` passed.

#### Switching branches

The journal, its base and the commit marker are local and ignored, so they stay when you check
out another branch. If the journal records edits to a different head, opening the label set
refuses it rather than applying them: switch back to the branch it belongs to, or move
`journal.ndjson` and `journal-base/` aside to edit this one. Your edits stay safe in the branch's
commits either way.

#### What else to know

- **Splits land in `labels/splits/`.** A split frozen through the server is written there. Commit
  it only if it is meant to be published.
- **Facets break for later copies.** A facet pins the label set revision it was authored on, so it
  stops loading once membership moves past that revision. `check-view` reports it; there is no
  facet re-pin yet.
- **Every text file must stay under 10 MiB.** Hugging Face refuses a push in which any commit holds
  a larger plain-Git file. `check-view` and the staging script report one. Running `labels relayout`
  with a smaller `--window-chunk` splits the chunks finer.

## Reader levels

`labels.json`, a pack's `manifest.json` and `physical-references.json` can say, in `min_reader`,
the lowest reader level that reads them correctly. A build refuses a file above its own level and
says to update, rather than reading it wrongly or dropping what it does not know. `schema_version`
changes when a format breaks; `min_reader` rises when a writer starts using something an older
reader would misread. A file without it is level 1, and a save keeps the level it read. This build
reads level 1; the levels are listed in the
[label set README](../../../internal/lidar/annotation/labelset/README.md#reader-levels).

## Limits

- Only the Hesai Pandar40P layout (10 blocks × 40 channels) is decoded.
- A pack can be cut only from a VRLOG recorded by a build that carries return identity; re-record
  older runs by replaying their PCAP.
- Physical references, feature proposals and proposal layers stay bound to a pack (one run's view).
  Their membership pins name label set revisions through the projection, so after a membership
  edit the reviews need `annotation-reference repin` before the edit is published.
