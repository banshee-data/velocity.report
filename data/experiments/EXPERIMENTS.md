# EXPERIMENTS

Every experiment the project runs, as one spec per experiment: what it asks, what it depends on,
what it runs, and how long that should take. A spec is intent and lives in Git; where an
experiment is in its execution is the queue's business, and raw outputs never come here (see
[AGENTS.md](../../AGENTS.md)). The design is the
[experiment queue plan](../../docs/plans/platform-experiment-queue-plan.md).

- [DEPENDENCIES.md](DEPENDENCIES.md): the dependency graph and the open queue, generated from the
  specs. Do not edit it; run `make experiments-graph`.
- `<area>/`: one directory per subject area, each with a `README.md` whose header registers it,
  and the area's specs.
- [try/](try): the older free-form proposals. Each moves into an area as a spec when it is next
  picked up.

## Slugs

A spec is named by a slug: an area of four to six characters (a lower-case letter, then letters or
digits) and a four-digit number, for example `l3bg0006` or `label0001`. The file is
`<area>/<slug>.md`, numbers run from 0001 within an area and are never reused, and
`go run ./cmd/tools/experiments new <area>` prints the next free one. A new area is a new
directory with a `README.md` whose YAML header gives `area`, `title`, `layers` and `owner`.

| Area     | Subject                               |
| -------- | ------------------------------------- |
| `l3bg`   | L3 background model                   |
| `l4clus` | L4 clustering                         |
| `geom`   | Solid-body geometry                   |
| `perf`   | Performance on the deployment target  |
| `label`  | Labelled data: capture, label, freeze |

## A spec

A spec is a Markdown file whose YAML header the validator reads and whose body is the proposal.
[l3bg0006](l3bg/l3bg0006.md) is a complete answered example; [l3bg0009](l3bg/l3bg0009.md) and
[label0001](label/label0001.md) show manual steps and dependencies.

| Field        | Required                      | Meaning                                                                                                                                                                                   |
| ------------ | ----------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `id`         | yes                           | The slug; must match the file name and a registered area.                                                                                                                                 |
| `title`      | yes                           | One line.                                                                                                                                                                                 |
| `status`     | yes                           | `proposed`, `approved`, `answered` or `retired`. Authoring state only: queued, running and waiting are the queue's, never written here.                                                   |
| `owner`      | yes                           | Who approves and answers it.                                                                                                                                                              |
| `created`    | yes                           | `YYYY-MM-DD`.                                                                                                                                                                             |
| `asks`       | yes                           | The questions it answers: the repository path of the Markdown that poses each (briefing, plan, report, proposal), with an optional `#anchor`, or a decision id such as `D-28`.            |
| `depends_on` | no                            | Slugs whose answers this one needs first.                                                                                                                                                 |
| `report`     | once answered                 | The report under `docs/` that answers it. A proposed spec may name the report it intends.                                                                                                 |
| `reason`     | once retired                  | Why it will not run.                                                                                                                                                                      |
| `requires`   | no                            | `capabilities` a worker must offer, `disk_gb` of free scratch space before a step starts, `cores`.                                                                                        |
| `reclaim`    | no                            | Who may take over a step whose worker stopped: `any` (the default: a worker configured to reclaim orphans), `origin` (only the worker that started it) or `never` (the operator decides). |
| `arms`       | if a step fans out over arms  | Each a `name`, a `tuning` file (`config/tuning.defaults.json` or a parameter set under `config/experiments/`) and any `replay` options (`-experiment` names).                             |
| `cases`      | if a step fans out over cases | A named `corpus` and a `count`, optionally the `sites`.                                                                                                                                   |
| `steps`      | yes                           | In order; see below.                                                                                                                                                                      |
| `actual`     | no                            | For an answered spec: the `wall` time it took and on which `worker`, to calibrate the next estimate.                                                                                      |

**Steps.** Each has an `id`, a `kind`, an `estimate`, and optionally `after` (the steps it waits
for; by default the one before it, `-` for none), `over` (`arms`, `cases` or both, to fan out),
`params`, a `gate` and a `note`. A kind is one of three classes:

- **Job kinds**, which a worker runs: the closed set in
  [internal/lidar/jobs](../../internal/lidar/jobs/kinds.go), never a command line.
- **Planned kinds**, designed but not yet runnable by a worker: `bench_sweep`,
  `ground_truth_eval`, `corpus_screen`, `scene_variants`, `pi_timing` and `chart`. An experiment
  with one is not ready for a worker until the kind is built.
- **Manual kinds**, which a person closes by recording an artefact the queue checks:
  `manual:implement` (a commit on main), `manual:capture` (a registered capture),
  `manual:label` (track labels meeting the step's `gate`), `manual:annotate` (a pack digest),
  `manual:review` (a graded pack), `manual:freeze` (a frozen split), `manual:decide` (a `D-NN`)
  and `manual:write` (the merged report).

**Estimates** are in hours and minutes, never days: a person's day and a machine's are not the
same length. A job step gives `each` (per arm per case, with `over`) or `total`; a manual step
gives `human`. `worker` names the class the estimate is for: `m1pro`, `swan`, `pi5` or `person`.
An experiment's estimate is its machine time along its longest chain of steps, plus the person
time of its manual steps.

**The body** is the proposal: the question, the method, what counts as success, what could
mislead. Once answered, a short answer and the report link; the report itself is in `docs/`.

## Commands

```bash
make check-experiments                         # validate every spec; DEPENDENCIES.md must be current
make experiments-graph                         # regenerate DEPENDENCIES.md
go run ./cmd/tools/experiments next            # open experiments and what each waits for
go run ./cmd/tools/experiments show l3bg0009   # one spec's steps, fan-out and estimate
go run ./cmd/tools/experiments new l3bg        # the next free slug in an area
```

`go test ./internal/experiments` runs the same checks, so a spec cannot merge invalid or without
its graph.
