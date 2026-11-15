---
name: free-space
description: Survey storage and plan verified moves of required large files to the local lidar volume. Preserve required local evidence; the NAS holds optional archival copies. Read-only scan first; the user deletes originals.
argument-hint: "[--min-mb N]"
---

# Skill: free-space

Survey the internal disk and directly attached `lidar` volume, then propose a short,
reviewable cleanup plan. Follow the canonical [local evidence storage policy](../../../docs/platform/operations/local-evidence-storage.md).
The NAS is a final archival store. It must never be needed to read required evidence or
run local development, replay, alignment or validation.

## Procedure

### 1. Scan

```bash
scripts/free-space-scan.sh
scripts/free-space-scan.sh --min-mb 500
```

The scan is read-only. It reports free space, large items, possible caches and idle
worktrees. File-name classes and old archive receipts are leads, not retention decisions.
NAS availability is not a prerequisite. If lidar is unavailable, do not create a substitute
directory at its mount path or move required files to the NAS instead.

### 2. Establish dependencies

Check running services with `lsof`, configuration, symlinks, dataset indexes and database
references. Required evidence includes captures, VRLOGs, labels, campaign evidence, cell
databases and blobs, prepared sites, building/roof products and referenced source subsets.
Keep the whole dependency set local, including what a cold start needs.

| Group                                     | Action                                                                      |
| ----------------------------------------- | --------------------------------------------------------------------------- |
| Required large files on the internal disk | Copy to lidar, verify independently and update local paths                  |
| Required evidence already on lidar        | Keep; an archive receipt does not permit deletion                           |
| Proven regenerable caches                 | Propose deletion only when complete local inputs and a tested recipe remain |
| Idle worktrees                            | Run `scripts/worktree-sweep.sh` dry run; the user applies removal           |
| Optional archival copies                  | Copy to the NAS when available; preserve required local copies              |
| Unknown ownership or retention            | Ask; never infer disposability from a name                                  |

Keep active databases in place until their owner stops or supplies a consistent backup.
Never reorganise captures without approval. Skip iCloud-backed libraries unless asked.

### 3. Copy and verify

Preserve existing local layouts; new evidence sets may use
`/Volumes/lidar/evidence/<project>/<set>/`. Check capacity before copying. Keep manifests
and receipts outside Git. Copy with SHA-256 hashes, independently re-read every destination
file and compare complete manifests. Size and mtime are insufficient, including on exFAT.
Do copies only; never delete during a copy run.

Update symlinks, machine configuration and absolute paths inside indexes to the verified
local copy. Test representative workflows with the NAS disconnected, including a cold
start. Do not replace a missing blob store with an empty directory or silently use another
dataset. Record missing local dependencies as blockers if no verified local copy exists.

### 4. Hand over deletion

Only redundant internal-disk copies or explicitly approved retired evidence qualify.
Write a guarded script with a dry-run mode that checks the exact verified destination,
complete manifest, unchanged source, current availability and retention decision. For
required evidence, the verified destination must be local on lidar; a NAS `PASS` receipt
does not qualify. Run the dry run and let the user perform deletion. Never run `rm -rf`
yourself.

### 5. Report

Report available space, safe candidates, retained dependencies, verified local copies and
unresolved decisions. Clearly separate optional archival from local recovery. Re-run the
scan after the user deletes.

## Related

- [Local evidence storage](../../../docs/platform/operations/local-evidence-storage.md)
- [Read-only scan](../../../scripts/free-space-scan.sh)
- [Idle worktree survey](../../../scripts/worktree-sweep.sh)
