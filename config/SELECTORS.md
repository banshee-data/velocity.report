# Segment selectors

[segment-selectors.defaults.json](segment-selectors.defaults.json) defines the ways the
**Segments** page and `velocity lidar segments` choose capture windows for annotation. Each
selector has its own label and category, and ranks windows by its own criterion. The file is
config as code: change it in a pull request, and the server reads it when it starts.

## What a selector is

A selector is a finder, the parameters it runs at, the measure that ranks what it finds, and the
bounds a window must meet:

```json
{
  "id": "close_following",
  "label": "Close following",
  "category": "Traffic",
  "description": "Most time with a follower within 15 m of its leader, and at least 2 s of it.",
  "finder": "following",
  "parameters": {
    "window_seconds": 10,
    "min_speed": 3,
    "max_heading_deg": 20,
    "min_gap": 3,
    "max_gap": 15,
    "max_lateral": 2.5
  },
  "score": { "measure": "pair_seconds", "order": "descending" },
  "require": [{ "measure": "pair_seconds", "min": 2, "max": null }]
}
```

| Key           | Meaning                                                                                                                |
| ------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `id`          | Lower case letters, digits and underscores. It names the selector in the API, on the command line and in every choice. |
| `label`       | What the Segments page shows. Unique, ignoring case.                                                                   |
| `category`    | The group the page shows it in. Categories appear in the order the file first uses them.                               |
| `description` | One sentence for the page.                                                                                             |
| `finder`      | The pass that measures windows: `following`, `leader_changes`, `lateral_jump`, `split_flags`, `exposure` or `random`.  |
| `parameters`  | Exactly the parameters the finder reads, listed below. The rest keep their defaults.                                   |
| `score`       | The measure that ranks windows, and its `order`: `descending` puts the largest first and `ascending` the smallest.     |
| `require`     | Bounds a window must meet, each with a `min` and a `max`. `null` is no bound, both bounds are inclusive, `[]` is none. |

The file is read as strictly as the [tuning file](CONFIG.md): an unknown key, a missing key, a
`null` parameter, and a parameter the finder does not read are all errors. A file that does not
parse stops the server; it never falls back to the defaults.

### Parameters each finder reads

| Finder                        | Parameters                                                                            |
| ----------------------------- | ------------------------------------------------------------------------------------- |
| `following`, `leader_changes` | `window_seconds`, `min_speed`, `max_heading_deg`, `min_gap`, `max_gap`, `max_lateral` |
| `lateral_jump`                | `window_seconds`, `jump_threshold`, `jump_max_gap_seconds`                            |
| `split_flags`                 | `window_seconds`                                                                      |
| `exposure`                    | `window_seconds`, `min_speed`                                                         |
| `random`                      | `window_seconds`, `min_speed`, `random_seed`                                          |

A parameter the finder ignores keeps its default because every parameter is part of a window's
identity: one that changes nothing must not change which window is which.

### Measures

A selector ranks and bounds only by what its finder's pass computes. None is negative.

| Measure                         | Computed by                               | Unit            |
| ------------------------------- | ----------------------------------------- | --------------- |
| `pair_frames`, `pair_seconds`   | `following`, `leader_changes`             | frames, seconds |
| `pairs`, `followers`, `leaders` | `following`, `leader_changes`             | tracks          |
| `leader_changes`                | `following`, `leader_changes`             | changes         |
| `closest_gap_m`                 | `following`, `leader_changes`             | metres          |
| `tracks`                        | every finder but `random`                 | tracks          |
| `events`                        | `lateral_jump`, `split_flags`, `exposure` | observations    |
| `max_residual_m`                | `lateral_jump`                            | metres          |
| `draw`                          | `random`                                  | 0 to 1          |

The finder keeps its own peak frame, where the macOS tool opens a pack, and leaves out the same
windows it always did: `leader_changes` offers only windows with a change of leader, and
`exposure` only windows with moving traffic. A selector's bounds then narrow what is left.

## The standard selectors

The file must define six, one per finder, under the finder's own name and with its standard
definition: default parameters, ranked by the finder's own measure, largest first, with nothing
required. Their label, category, description and place in the file are free to change. `finder=`
in the API and `--finder` on the command line name them, as they named finders before selectors
existed.

| Standard selector | Ranked by        |
| ----------------- | ---------------- |
| `following`       | `pair_frames`    |
| `leader_changes`  | `leader_changes` |
| `lateral_jump`    | `max_residual_m` |
| `split_flags`     | `events`         |
| `exposure`        | `events`         |
| `random`          | `draw`           |

## Held-out windows

A window chosen because the tracker fails there flatters any candidate scored on it. Held-out
windows are therefore chosen only by the standard `following`, `exposure` and `random` selectors,
at their own parameters. The file cannot widen that, and neither can a request: a held-out ranking
that gives parameters of its own is refused.

The rule is about definitions, not measures, because whether a measure can seek failure depends
on everything around it. A `min_gap` of 0 makes a following pair of the two halves of one split
vehicle, and ranking following smallest first finds one-frame pairs. Widening the rule is a change
to the code, with its reason recorded.

Every chosen window records the selector that chose it, as it ran, and whether it could choose a
held-out window. The database refuses a held-out window whose selector could not. The record goes
into the pack's `segment.json`, so the evaluation can say how its references were chosen.

## Identity

A window's identity names its finder, source, role, parameters and start, not its selector. Two
selectors with the same finder and parameters offer the same windows under the same IDs, so a
window already chosen shows as chosen under both, and relabelling or regrouping a selector changes
nothing that was chosen or cut.

A selector's digest names what it does: its id, finder, parameters, score and bounds, but not its
label, category or description. The page sends the digest it ranked with when it makes a case,
and the server refuses the case if the selector has changed since.

## Adding a selector

1. Add it to [segment-selectors.defaults.json](segment-selectors.defaults.json), where the page
   should show it.
2. Validate the file with `make selectors-validate`.
3. See what it ranks on a run with `velocity lidar segments --db <db> --run <run> --selector <id>`.
4. Restart the server. `GET /api/lidar/segments/selectors` lists what it read, from where.

Keep a selector when it surfaces windows the others do not, on a run long enough to tell.

## Where the file is read from

| Reader                    | File                                                                                                                                   |
| ------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Server                    | `--lidar-segment-selectors PATH`; without it, this file when the server runs from the repository, and otherwise the copy in the binary |
| `velocity lidar segments` | `--selectors PATH`, with the same default                                                                                              |
