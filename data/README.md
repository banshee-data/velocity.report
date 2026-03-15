# Data Workspace

This directory is the working home for stable data contracts, mathematical foundations, and revisit-worthy analysis artifacts. It keeps the maths and data-science material close to the datasets, scripts, and exploratory outputs they describe without mixing them into the main product and platform docs under `/docs`.

## Recommended structure

- `data/structures/` — canonical data contracts and structural references
  - format specifications such as `VRLOG_FORMAT.md` and `HESAI_PACKET_FORMAT.md`
  - schema artifacts such as `SCHEMA.svg`, while the generator itself lives under `scripts/sqlite-erd/`
- `data/maths/` — durable mathematical foundations and proposal-grade algorithm notes
  - implementation-backed maths docs at the top level
  - forward-looking proposals under `data/maths/proposals/`
- `data/explore/` — exploratory investigations, parameter sweeps, and deep dives worth revisiting
  - each study keeps its own scripts, raw outputs, and findings together
  - current candidates: `convergence-neighbour/`, `multisweep-graph/`, `noise_investigation/`, `kirk0-lifecycle/`
- `data/align/` — acquisition and alignment tooling that prepares external datasets for analysis

## Architecture decision record

**Decision:** organise `data/` around intent, not file type: stable references in `structures/`, durable maths in `maths/`, and time-bounded investigations in `explore/`.

**Why:** the old split forced people to bounce between `/docs` for theory and `/data` for actual experiments. Putting both under `data/` makes the data-science workflow easier to navigate, while still keeping product, operational, and subsystem documentation under `/docs`.

**Alternatives considered:**

1. **Do nothing** — lowest effort, but keeps maths disconnected from the experiments and specs it explains.
2. **Keep maths in `/docs` and move only `docs/data/`** — smaller change, but still splits one data-science workflow across two top-level homes.
3. **Create many new buckets now (`tools/`, `exports/`, `raw/`, `derived/`)** — more explicit, but premature for the current repository shape and harder to keep tidy.

**Recommendation:** move stable references into `data/`, keep repository-owned build helpers in `scripts/`, move the obvious deep dives into `explore/`, and defer any finer-grained substructure until there is repeated pressure for it.

## System boundary diagram

```text
 +-------------------------- repository top level --------------------------+
 | /docs/...                         /scripts/...                           |
 | subsystem architecture, plans     repo-owned generators and helpers      |
 +----------------------------------+--------------------------------------+
                                    |
                                    | references stable data contracts
                                    v
+------------------------------- /data ---------------------------------+
| +---------------------------+  +-----------------------------------+  |
| | structures/               |  | maths/                            |  |
| | file formats, schemas,    |  | implemented maths + proposals     |  |
| | packet contracts, ERD     |  |                                   |  |
| +---------------------------+  +-----------------------------------+  |
|                                                                        |
| +---------------------------+  +-----------------------------------+  |
| | explore/                  |  | align/                            |  |
| | deep dives, sweeps,       |  | external-data preparation tools   |  |
| | revisit-worthy findings   |  | and import helpers                |  |
| +---------------------------+  +-----------------------------------+  |
+------------------------------------------------------------------------+
```

## Failure registry

| Area               | Failure mode                                                     | Recovery                                                                                                                                |
| ------------------ | ---------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| `data/structures/` | A spec or schema asset drifts from implementation                | update the canonical spec and the referencing docs together; validate links and regenerate artifacts such as `SCHEMA.svg`               |
| `data/maths/`      | A maths note points at stale plans or code paths after refactors | keep relative links current during moves and treat `data/maths/` as reference material that must be updated alongside algorithm changes |
| `data/explore/`    | Exploratory work gets stranded without enough context to revisit | keep scripts, outputs, and write-ups in the same study folder and add a short findings note before considering it durable               |
| `data/align/`      | Tooling drifts from the shared repo environment                  | prefer the repository `.venv` and document any extra dependencies close to the tool                                                     |

## Placement rules

1. Put something in `data/structures/` when another part of the repo should treat it as a stable contract.
2. Put something in `data/maths/` when it explains the model, derivation, assumptions, or proposal behind an algorithm.
3. Put something in `data/explore/` when it is an investigation, sweep, comparison, or write-up you may want to revisit later.
4. Keep `data/align/` for tooling that fetches, cleans, or aligns outside datasets before analysis.
5. Build-support generators that the repository depends on belong in `scripts/`, even when they emit artifacts into `data/structures/`.
6. If an exploration hardens into a long-lived spec, promote the conclusion into `structures/` or `maths/` and leave the raw study in `explore/`.
