---
name: free-space
description: Find space to free on the internal disk and the lidar volume, and plan moving captures, results and media to the NAS shares with verified copies. Read-only scan first; the user deletes originals.
argument-hint: "[--min-mb N]"
---

# Skill: free-space

Survey the internal disk and the `lidar` volume, then propose a short, reviewable list of
moves to the NAS (`arrow.local`). The scan never changes anything. Copies come second, and
deleting originals is always the user's step.

## Usage

```
/free-space
/free-space --min-mb 500
```

## Procedure

### 1. Scan

```bash
scripts/free-space-scan.sh                  # threshold 1000 MB
scripts/free-space-scan.sh --min-mb 500     # pass the skill's --min-mb through unchanged
```

It reports free space, NAS mount state, large items under the home folder, `~/Library`,
`~/code`, the lidar volume, idle git worktrees (`scripts/worktree-sweep.sh`, dry run) and
files over 1 GiB. Each candidate carries a class, a suggested destination and whether a
verified NAS copy is already on record.

The record check is a **name match** against `validate_*.txt`, so it can be wrong both ways: a
similarly named folder can match a different tree, and a tree copied under another name shows
"no PASS record". Treat it as a lead and confirm against the manifests before proposing deletion.

If a NAS share is missing, the scan prints the remount command. Do not start copies until all
three shares are mounted.

### 2. Sort candidates into four groups

| Group       | What                                                          | Action                                                                     |
| ----------- | ------------------------------------------------------------- | -------------------------------------------------------------------------- |
| Regenerable | build caches, `DerivedData`, `node_modules`, `.gocache`, logs | Propose deletion (the user runs it)                                        |
| Worktrees   | idle agent worktrees                                          | `scripts/worktree-sweep.sh` dry run, then the user runs it with `--apply`  |
| NAS-bound   | `captures` -> `banshee-captures`, `results` -> `banshee-results`, `media` -> `media` | Copy, verify, then hand over a guarded delete |
| Review      | anything else                                                 | Ask; never guess                                                           |

Rules that always apply:

- **Captures are never reorganised without approval.** Copy them as they are.
- **Keep what is in use.** A running service's database (check with `lsof`) stays put.
- **A symlinked copy-back is fine** (for example `sensor_data` linking to the lidar volume),
  but only after the NAS copy is verified.
- Skip `david2.photoslibrary`-style iCloud-backed libraries unless asked.

### 3. Copy with verification (NAS-bound items only)

Hash at copy time, then verify independently. Do not trust file size and mtime: the macOS
exFAT driver can zero file tails without changing either.

1. Copy with a hashing copier that writes a manifest (`rel<TAB>size<TAB>mtime<TAB>sha256`).
2. Re-hash on the NAS side. Prefer a worker VM with the share mounted over NFS; hashing over
   SMB from a laptop on Wi-Fi is slow and a dropped mount reports every file as missing.
3. Compare manifests. SMB name mapping turns `? : " * < > | \` into U+F020 to U+F027, so undo
   that mapping and NFC-normalise before comparing.
4. Record the verdict in `validate_<name>.txt` containing `VERDICT: PASS` (the scan reads
   these to mark an item `name-match PASS`).

Do copies only. Never delete during a copy run.

### 4. Hand over deletion

Write one guarded script per batch that, for each item, removes it only if:

- its `validate_*.txt` says `VERDICT: PASS`,
- the copy is present right now with the expected file count,
- nothing in the original is newer than the copy's start.

Anything else is kept with a reason. Give it a `--dry-run` mode, run the dry run, report the
result, and let the user run the real thing. Do not run `rm -rf` yourself.

### 5. Report

Lead with the numbers: free space before, what each group would free, what needs the user's
decision. Re-run the scan after the user deletes, and report free space again.

## Failure modes

- **Wi-Fi drop unmounts SMB.** Check `mount` before concluding anything is lost.
- **Case and Unicode differences** in Photos libraries (`resources` vs `Resources`, NFC vs
  NFD): compare case-folded and normalised.
- **Locked or active worktrees** are kept by `worktree-sweep.sh`; do not override.

## Related

- [scripts/free-space-scan.sh](../../../scripts/free-space-scan.sh) (read-only scan)
- [scripts/worktree-sweep.sh](../../../scripts/worktree-sweep.sh) (idle worktree removal; dry run by default)
