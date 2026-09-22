# Annotation-scored tuning

Scores a pipeline run against a point annotation pack instead of against proxy
metrics, and sweeps tuning parameters on that score.

The benchmark's work counters say how many clusters a config produced. They
cannot say whether those were the real ones. A pack labelled by hand can, so
these scripts turn one into per-frame ground truth and rank configs by how much
of it each run actually found.

| Script          | Does                                                                         |
| --------------- | ---------------------------------------------------------------------------- |
| `gt_extract.py` | Pack → per-frame ground truth: centroid, extent and point count per object   |
| `score_run.py`  | One run's `-clusters-output` dump → recall, precision, F1 against that truth |
| `mkconfig.py`   | Tuning config with dotted-key overrides applied to the defaults              |
| `sweep.py`      | Runs a shuffled factorial, scores each run, then hill-climbs on the best     |
| `analyse.py`    | Reads the results and reports the trade between recall and precision         |

```bash
python3 gt_extract.py <pack-dir> ground-truth.json
python3 sweep.py ./lidar-bench <work-dir> <repo> <hours> <parallel-runs>
python3 analyse.py <work-dir>/results.jsonl
```

Build the benchmark first: `go build -tags pcap ./cmd/tools/lidar-bench`.

These are analysis scripts, not product code. Scoring a run against a pack is
worth having as a `velocity` subcommand in Go; see the write-up for why it is
not one yet.

Method, results and caveats: [annotation-scored tuning](../../../docs/lidar/operations/annotation-scored-tuning.md).
