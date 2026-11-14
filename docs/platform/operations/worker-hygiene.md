# Worker storage hygiene

- **Scope:** Linux workers, including arrow-worker
- **Tool:** [worker-hygiene.py](../../../scripts/worker-hygiene.py)

The tool inventories local storage, copies selected data to the expected NAS export,
and independently hashes both trees before recording a receipt. Copying retains the
original. Removal is a separate command for the operator, with another full verification.
Run it from outside the source directory after stopping every writer to that directory.
Process inspection is a guard, not a lock against a writer starting after inspection.
Do not archive a live database: stop its owner and checkpoint it first.

## Survey and choose

```bash
python3 scripts/worker-hygiene.py scan
sudo python3 scripts/worker-hygiene.py worktrees --repo /srv/banshee/velocity.report
```

The worktree command defaults to a plan. It keeps the main checkout, locked or missing
worktrees, active paths, recent trees, commits outside local `origin/main`, and every tree
with modified, untracked, or ignored files. Fetch the repository first so the merge check
uses a current remote reference. An operator may add `--apply` after reviewing the plan;
Git removes eligible checkouts without force and preserves branches. Squash-merged
branches may need individual review because their commits are not ancestors of main.
The script requires root for a complete process scan, including file mappings and bind
aliases. Missing visibility refuses the operation. It never cleans package or module caches.

A read-only survey on November 14, 2026 found about 22 GB of evidence and 1.5 GB of
captures under `/srv/banshee`, alongside several linked worktrees. These are candidate
sizes, not promised savings. One worktree lives inside `evidence/s2-t0/src`; archive
individual data directories rather than the whole evidence tree. A 14 GB Gerardus
checkout needs review for active data and ignored files before any worktree removal.
The root filesystem had only 592 MB free, separately from the 26 GB available on `/srv`.
Moving `/srv` data does not free root space. Inspect root usage separately, for example
`sudo du -x -h --max-depth=1 /`, before choosing package or log maintenance.

## Copy, verify, then remove separately

The destination must already exist on the expected NAS export. For example, to
archive an explicitly selected, stopped run into the results share:

```bash
sudo python3 scripts/worker-hygiene.py archive /srv/banshee/evidence/OLD-RUN /mnt/results
# Review the plan, then repeat with --apply to copy and verify.
sudo python3 scripts/worker-hygiene.py archive /srv/banshee/evidence/OLD-RUN /mnt/results --apply
sudo python3 scripts/worker-hygiene.py remove-verified /mnt/results/ARCHIVE/.worker-archive.json
# The operator repeats remove-verified with --apply only after reviewing its verification.
```

Each copy uses a new directory and a SHA-256 receipt containing the exact file set,
including empty directories. It refuses repositories, nested mounts, links, special
files, unexpected exports, and mounts lacking `rw,hard`. Copy errors leave the original
and any partial archive for inspection; no successful receipt is written. Removal
checks the original and archive again against the receipt. A changed or added file
keeps the original. Receipts are local evidence and never belong in Git.

The NAS may refuse exclusive creates and metadata changes. Copies sync file contents
without preserving ownership, modes, or timestamps: this archives experiment data,
not an executable installation or a permissions backup. Treat the NAS and receipt as
trusted operator storage. Stop writers throughout copy and removal; the script does
not coordinate unrelated services or concurrent cleanup invocations. For captures,
choose an explicitly approved archive destination, preserving the original layout.

## Mount and harness checks

The observed data mounts used NFSv3 over TCP with `hard,timeo=30,retrans=2`:
`/mnt/captures` was read-only and `/mnt/results` read-write. Preserve hard mounts for
write integrity; `timeo` and `retrans` do not bound an outage. A blocked kernel NFS
operation can outlive an application timeout. Test recovery during a planned outage.
`local_lock=none` is not evidence that locking is disabled: validate cross-process
locking before changing it.

Inspect both effective mounts and server exports. Keep captures and published label
inputs read-only server-side. Consider `nosuid,nodev,noexec` for data mounts after
checking other users of those shares. Keep builds, active SQLite databases, scratch,
and the eligibility ledger on local storage. The harness already supports local
`build_root` and `stage_attempts`; see the [harness runbook](experiment-harness.md).
Restrict the results dataset ACL and mapped writer identity at the NAS: the observed
harness directories were world-writable, and client `chmod` is refused there.

Preserve the service user's preloaded Go module cache: offline builds use
`GOPROXY=off`. Retain failed archive staging, label inputs, active attempt output,
and service databases. Neither cache age nor a large directory proves it is disposable.

Run `make test-worker-hygiene` for the portable safety regressions. They also run
in the Python suite and the worker hygiene PR workflow. Tests use temporary trees
and mocked mounts; the live NAS copy check remains a separate operational check.
