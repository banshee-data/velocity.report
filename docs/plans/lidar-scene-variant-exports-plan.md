# Scene variant exports: one window, several tuning configs, as web scenes

A tool that replays one window of a site once for each named tuning config, through the replay
start that now takes `tuning_config`, and exports each run as a web scene the existing player
opens unchanged. The set of exports, with a manifest that says exactly which file, fingerprint and
build made each one, is how an arm is shown rather than only scored.

- **Status:** Tooling built, unit-tested and run end to end against a server on kirk0
  ([first run](#first-run-against-a-server)); the `geometry-convergence` set itself has not been
  run, so its windows are unverified
- **Canonical:** [config/CONFIG.md](../../config/CONFIG.md)
- **Layers:** L9 Endpoints (replay start, configs API), offline tooling, the web scene player
  (read only)
- **Related:** [Experiment parameter sets](../../config/CONFIG.md#experiment-parameter-sets),
  [PCAP analysis mode](../lidar/operations/pcap-analysis-mode.md),
  [Web scene export](lidar-web-scene-export-plan.md),
  [Deterministic scene capture](lidar-deterministic-scene-capture-plan.md),
  [Web scene assets](../../tools/s2-archive/README.md#web-scene-assets),
  [Geometry convergence](lidar-tracker-geometry-convergence-plan.md),
  [End-on truck windows](../lidar/operations/end-on-truck-window-spike-2026-10.md)

## What landed, and what this reuses

#720 made a replay's parameters a file someone can name. Everything this plan needs to drive a run
and record what it ran already exists; the gap is doing it several times over one window and
keeping the results together.

| Piece                 | Where it is                                                                                                                                      | What the tool uses it for                                               |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------- |
| Config listing        | `GET /api/lidar/configs`: every `.json` under the config directory, with `kind`, `fingerprint`, `applicable`, `reason`                           | Preflight: refuse a set whose file cannot be applied, before any replay |
| Config apply          | `tuning_config` on `POST /api/lidar/pcap/start`, the field the :8081 replay form's dropdown sends                                                | The one way a variant's parameters reach the pipeline                   |
| Start response        | `tuning_config` and `tuning_fingerprint` echoed back                                                                                             | The file and fingerprint each variant ran, as the server saw them       |
| Run provenance        | `lidar_run_records.run_config_id` to the immutable run config; its params hash becomes the VRLOG's and the export's `tuning_hash`                | Proof that two variants really ran different parameters                 |
| Replay and export     | `publish-scenes.py`: `plan()` resolves a site to packets, `replay_stretch()` replays and finds the run, `export()` calls `velocity scene export` | The whole replay and export path, unchanged                             |
| Published scene shape | `assets/` with `manifest.json`, `part-000/` and `background/`, as the scene player reads                                                         | The layout every variant is written in                                  |
| Stills                | `tools/scene-capture`: a recipe names a source export, one frame and the views                                                                   | Matched stills of every variant at one frame and one set of views       |
| Recording comparison  | `make vrlog-compare`                                                                                                                             | An optional summary of each variant against the baseline                |

The file fingerprint and the export's `tuning_hash` are different identities. The fingerprint is a
hash of the config file as loaded; `tuning_hash` is the params hash of the run config the server
recorded. A variant records both.

## What the tool adds

One command, run against the same server and database `make scene-assets` uses:

```bash
make scene-variants SET=tools/s2-archive/scene-variants/geometry-convergence.json
```

For each site in the set and each variant in it, it replays the same packets with the variant's
`tuning_config`, exports the recording with the same exporter and settings as a published scene,
and writes the result where the player can open it. It then writes one manifest for the set.

## Rules that keep it DRY

1. **No second replay path.** `replay_stretch()` gains an optional `tuning_config` and passes it on
   the start request. The new script loads `publish-scenes.py` as a module, the way its tests
   already do, and calls the same `plan()`, `replay_stretch()` and `export()`. Nothing is copied.
2. **No second exporter.** Every variant is `velocity scene export` with the published settings:
   stride 2, the 400-frame transient dropped at export, a background export beside it.
3. **The published layout, exactly.** A variant's `assets/` is shaped like
   `public_html/src/scenes/<site>/assets/`, so the player opens it with no change, and promoting a
   variant to the site is the existing swap rather than a conversion.
4. **Configs are named as the dropdown names them**: a path relative to the server's config
   directory, such as `experiments/solid-body-a2.json`.
5. **One environment.** The Makefile targets reuse `SCENE_ASSETS_ENV` and the binary and server
   half of `scene-assets-preflight`, split out as `scene-build-preflight`, so the replay speed,
   settle mode, corpus root, binary stamp and database are the ones a published scene is made
   with. The site half stays with `scene-assets`; a set resolves its own sites.

## The set file

A set is hand-authored configuration, checked in beside the script, in
`tools/s2-archive/scene-variants/`. Not under `config/`, because every `.json` there is listed by
the configs API and would appear, disabled, in the replay form's dropdown.

```json
{
  "version": 1,
  "set_id": "geometry-convergence",
  "description": "The near-edge arms against the defaults on the W7 end-on truck windows.",
  "baseline": "defaults",
  "variants": [
    { "id": "defaults", "label": "Shipped defaults", "tuning_config": "tuning.defaults.json" },
    { "id": "a2", "label": "Solid body A2", "tuning_config": "experiments/solid-body-a2.json" },
    {
      "id": "alignment",
      "label": "Alignment candidate",
      "tuning_config": "experiments/alignment-candidate.json"
    },
    {
      "id": "convergence",
      "label": "Geometry convergence candidate",
      "tuning_config": "experiments/geometry-convergence-candidate.json"
    }
  ],
  "sites": [
    { "site": "1st-mission", "offset_seconds": 100, "duration_seconds": 100 },
    { "site": "3rd-folsom", "offset_seconds": 200, "duration_seconds": 60 }
  ]
}
```

Validation, before anything runs:

- `set_id` and every variant `id` are lower-case slugs, and variant ids are unique.
- `baseline` names one of the variants.
- **Every variant names a file.** "Keep the server's current parameters" is not a variant: after
  the first replay the server's current parameters are the previous variant's, so the same set
  would mean different things depending on its order.
- A `tuning_config` is relative, stays inside the config directory and ends in `.json`, the same
  rules the server applies.
- A site is one the site index and corpus resolve, as for `make scene-assets`. A window is optional;
  without one the whole site is replayed.

## The window

A window is measured from the first packet of the site's capture. The
[end-on truck report](../lidar/operations/end-on-truck-window-spike-2026-10.md) gives its seconds
"into the segment" of its corpus pass, which may not start at the first packet, so the first set's
windows are checked by eye on the first run and corrected in the set file before anything relies
on them. The replay starts the warm-up earlier, `TRANSIENT_FRAMES` at 10 Hz (40 s), and the export
drops those frames exactly as a published scene does. A window too close to the start for its
warm-up is refused. Every variant of a site replays the same files over the same interval; the
set records the interval once and each variant's frame count beside it.

## Output: local, never committed

A set is a comparison made for review, not a published page, so it is local output under the
repository rule on raw outputs. It goes outside the checkout by default:

```make
SCENE_VARIANTS_DIR ?= $(LIDAR_DATA_DIR)/scene-variants
```

```text
$(SCENE_VARIANTS_DIR)/<set_id>/
  set.json                        the set manifest, rewritten after each variant
  <site>/
    <variant>/
      assets/                     manifest.json, part-000/, background/: a published scene's shape
      .done                       the variant's file fingerprint, for resuming
    compare/<variant>.json        vrlog-compare against the baseline, when asked for
  recipes/<site>/<variant>.json   scene-capture recipes, when a base recipe is given
```

`set.json` holds, for the set: the set file's path and digest, the server's build version and git
sha, the replay settings, and the config the server was running before the batch and is left on
after it. For each site: the files and interval replayed. For each variant: the config path, the
fingerprint `GET /api/lidar/configs` listed, the fingerprint the start response echoed, the run
id, the VRLOG path and digest, the export's `tuning_hash`, its frame count and duration, and the
minutes it took.

## Checks the tool makes

| Check              | When           | What it refuses or reports                                                                                                                                                                                                                               |
| ------------------ | -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Configs applicable | Before any run | A variant whose file is missing, not `kind: tuning`, or not `applicable`, with the server's `reason`                                                                                                                                                     |
| Build stamp        | Before any run | `./velocity` not built from `HEAD`, through `scene-build-preflight`                                                                                                                                                                                      |
| Server and records | Before any run | The server not answering, or the run records unreadable, as for `make scene-assets`                                                                                                                                                                      |
| Fingerprint echo   | After a replay | A start response whose `tuning_fingerprint` differs from the one listed: the file changed during the batch                                                                                                                                               |
| The right run      | After a replay | A run record created before the start request. Today `newest_run()` takes the newest run of the file, which is ambiguous once one capture is replayed several times; the lookup gains a lower bound on `created_at`, and `make scene-assets` gets it too |
| Distinct params    | After the set  | Two variants whose exports carry the same `tuning_hash`: one file's changes did not reach the pipeline, so the comparison is void                                                                                                                        |
| One build          | After the set  | Variants exported by different builds                                                                                                                                                                                                                    |

Resuming follows `make scene-assets`: a variant with a `.done` marker is skipped, unless the
marker's fingerprint differs from the file's current one, which means the file was edited since
and the variant is made again. `FORCE=1` remakes every variant.

The server is left on the last variant's parameters. There is no endpoint that applies a file
without starting a replay, so the tool says, at the end and in `set.json`, which config the server
is now running.

## Viewing a set

- **In the player.** Each variant's `assets/` is a scene export; `mountScenePlayer` takes its
  `manifest.json` as it takes a published one. The scene-capture harness's server already mounts a
  local export directory beside the shared player, so local viewing needs no new server.
- **As matched stills.** Given a hand-authored base recipe (a frame and a set of views),
  `make scene-variants-recipes` writes one recipe per variant with only `source` and `output`
  changed. `make capture-scene RECIPE=...` renders each, so every variant is seen at the same
  instant from the same cameras.
- **As numbers.** `COMPARE=1` runs `make vrlog-compare` for each variant against the baseline. It
  compares each whole recording, warm-up included, not only the exported window, and its report
  gives its version as `dev`, because the target runs the tool with `go run`. The scorers that
  decide anything (per-frame evaluation, the solid-body summary) are run as they are today; the set
  does not replace them.

## Cost

At the published replay settings a window takes about 2.2 times its length, warm-up included.

| Set                                     | Replayed per variant | Variants | Replay, about |
| --------------------------------------- | -------------------- | -------- | ------------- |
| `geometry-convergence`, 1st-mission     | 140 s                | 4        | 21 min        |
| `geometry-convergence`, 3rd-folsom      | 100 s                | 4        | 15 min        |
| One whole 20-minute site, four variants | 20 min               | 4        | 3 h           |

`make scene-variants-status` prints the same estimate for a set before it is run.

## Privacy

A variant is the same export a published scene is: tracks and a background, no point-cloud clip
unless one is asked for, and the exporter's privacy invariants unchanged. It stays on the operator's
machine.

## What this cannot establish

A set shows what each config did to the same packets. It does not say which is right: a box that
looks better is not a measurement, and an unchanged picture can reproduce a wrong estimate. The
scored comparison is the predeclared criteria and the scorers they name. The set is for review,
for finding what to score, and for showing a result once it is scored.

## First run against a server

The question was whether the tool works against a real server: the configs listing, the replay
start with `tuning_config`, the fingerprint echo, the run lookup, the export, the manifest and the
resume. The `geometry-convergence` set could not be the test. Its captures are on the operator's
volume only: the published dataset on Hugging Face has its README but no manifest and no
captures.

The method was one smoke set on kirk0, the repository's own capture, fetched through Git LFS. The
four configs of `geometry-convergence` were run on a 30 s window from 45 s, against a dev server
from `make dev-go-lidar` on build `c1288cb2`, at the published replay settings. A scratch site
index and corpus manifest named kirk0, outside the checkout.

| Variant       | Config file                                       | Fingerprint, listed and echoed | `tuning_hash` | Frames | Minutes |
| ------------- | ------------------------------------------------- | ------------------------------ | ------------- | -----: | ------: |
| `defaults`    | `tuning.defaults.json`                            | `fd35b0b28fc1`                 | `2319c8f8…`   |    153 |     2.4 |
| `a2`          | `experiments/solid-body-a2.json`                  | `1fb29f8bea7f`                 | `462efa22…`   |    153 |     2.4 |
| `alignment`   | `experiments/alignment-candidate.json`            | `f55b8ad99081`                 | `8ca672f3…`   |    153 |     2.4 |
| `convergence` | `experiments/geometry-convergence-candidate.json` | `6d86dd759ee6`                 | `3244e196…`   |    153 |     2.4 |

What it showed:

- Every check passed and the set recorded no problems. Each variant's echoed fingerprint equalled
  the listed one, and the four `tuning_hash` values are distinct: an applied file does reach the
  run's recorded configuration, which the distinct-parameters check depends on.
- The defaults' `tuning_hash` equals the published 1st-mission scene's, which predates the field:
  `tuning.defaults.json` applied at the replay start records the same parameters as a server
  started on it.
- Every variant replayed 70 s, the window and its warm-up, and exported 153 frames over 30.4 s.
- A second run skipped all four variants, and `--status` read them as done.
- `--compare` wrote a comparison for each variant against the baseline.
- The scene-capture harness rendered every variant's `assets/` from per-variant recipes at one
  frame and two views. The defaults and the convergence candidate draw visibly different boxes for
  the same vehicles.
- The server was left on the convergence candidate, and the run said so.

What it does not establish: anything about the configs' accuracy, which the scorers measure, and
anything about the `geometry-convergence` windows, which only a run on their captures can check.
The raw outputs (the database, the four VRLOGs, the exports, the comparisons and the stills) stayed
in a scratch directory and are not kept.

## Checklist

On the tooling branch, `ai/tools/scene-variant-exports`:

- [x] `replay()` takes an optional `tuning_config` and `replay_stretch()` keeps its contract; the
      run lookup takes a lower bound on `created_at`; the manifest write and the swap are shared
      helpers; tests for each in `test_publish_scenes.py` (`S`)
- [x] `export-scene-variants.py`: set validation, preflight against `GET /api/lidar/configs`,
      the window, replay and export per variant, `set.json`, resume markers, the distinct-params and
      one-build checks, `--status` (`M`)
- [x] `test_export_scene_variants.py` with no server: validation, window arithmetic, preflight
      refusal, fingerprint echo, duplicate `tuning_hash`, resume, the manifest, a whole batch with
      the replay mocked; wired into `PYTHON_TEST_PATHS`, where the other S2 archive tests run (`M`)
- [x] Makefile: `scene-variants`, `scene-variants-status`, `scene-variants-recipes`,
      `SCENE_VARIANTS_DIR`, reusing `SCENE_ASSETS_ENV` and `scene-build-preflight` (`S`)
- [x] Per-variant scene-capture recipes from a base recipe (`S`)
- [x] The first set, `geometry-convergence.json`, on the W7 windows (`S`)
- [x] The runbook in the S2 archive README beside Web scene assets, and a pointer from
      [CONFIG.md](../../config/CONFIG.md#experiment-parameter-sets) under Running a set (`S`)
- [x] A first run against a server, end to end on kirk0, with resume, comparison and stills
      ([first run](#first-run-against-a-server)) (`S`)
- [ ] The `geometry-convergence` set made where its captures are, its windows checked by eye
      against the trucks the report names and corrected in the set file (`S`)

### Deferred

- [ ] A page that plays two variants side by side on one clock, or switches between them in place
- [ ] Publishing a set to the public site, and how a scene page would offer its variants
- [ ] Repeating the baseline at the end of a set to show the order does not matter

## Open questions

1. Should a set ever be published? Each variant of a 20-minute site is 2 to 4 MB of assets, as
   the published 1st-mission and 3rd-folsom scenes are, and the scene pages serve one recording
   each.
2. Is `vrlog-compare`'s summary worth keeping in the set, or should the set only ever point at the
   scorers' reports?
3. Should the batch end by applying the baseline again, so the server is left where it started?
   That costs one short replay, since applying a file today means starting one.
