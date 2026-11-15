# Local evidence storage

Required evidence and large working files live on the directly attached `/Volumes/lidar`
volume. The NAS holds final archival copies. A disconnected NAS must not prevent local
development, replay, alignment, review or validation.

## What stays local

Keep the complete inputs needed to reproduce or inspect current work: captures and VRLOGs,
labels, campaign evidence, database records and their referenced blobs, prepared geometry,
building and roof products, dataset indexes and the source subsets those indexes reference.
An apparently derived file is still required evidence when no complete, tested local recipe
and local inputs can reproduce it.

Use existing directories on lidar where possible. New large working sets can live under
`/Volumes/lidar/evidence/<project>/<set>/`. Record machine-specific paths in untracked
configuration such as `local.mk`. Keep authored findings, provenance identifiers and reusable
implementation in Git; raw evidence remains outside the checkout.

Small active databases may stay on the internal disk when filesystem or locking requirements
demand it. They must still have all required blobs and inputs available locally. Never move
an open database as an ordinary file: stop its owner or use its supported backup operation,
including WAL contents and referenced objects.

## Cleanup and moves

1. Inventory services and the work that still needs each candidate. Trace configuration,
   symlinks, indexes, manifests, database references and caches into their dependency set.
   File names and old archive receipts do not establish that a file is disposable.
2. Move required large files from the internal disk to lidar by copying first. Preserve the
   directory structure and capture identity; do not reorganise captures without approval.
   If lidar lacks space, stop and review retention with the user.
3. Save a manifest with relative paths, sizes and SHA-256 hashes outside Git. Independently
   re-read and hash the destination, then compare the complete manifest. Size and timestamps
   alone do not verify a copy. Preserve a receipt that identifies both exact trees.
4. Update configuration, symlinks and embedded absolute paths to the verified local copy.
   Keep the originals until representative replay, alignment and review succeed with the
   NAS disconnected. Check a cold start as well as an already warm cache.
5. Propose deletion of redundant internal-disk copies only after these checks. Hand the user
   a guarded dry run; deletion remains the user's step in the free-space workflow.

Files already on lidar are retained by default. An archive `PASS` receipt proves only that a
particular copy was verified at a particular time. It never authorises deleting the last local
copy of required evidence. Optional NAS archival can happen later, when connectivity permits;
it is not a prerequisite for local work or internal-disk cleanup verified against lidar.

## When storage disappears

A dangling symlink can make directory creation fail with `File exists`, even though the
requested child does not exist. Inspect the link and its target before changing anything.
Do not remove the link and create an empty store: databases may still refer to the missing
blobs, and a fresh directory hides the evidence gap.

Recover from a verified local copy on lidar and repair the complete dependency set. If the
only copy is on an unavailable NAS, report exactly what is missing and which operations are
blocked. Preserve local records and do not regenerate or substitute different evidence while
claiming the original set was recovered.

The November visualiser alignment failure exposed this distinction: the temporary Gerardus
data directory linked its blobs, prepared sites and reference caches to `banshee-results`.
When that mount disappeared, local cell databases alone could not supply building reference
geometry. A local database is not a complete local evidence set.

See the [free-space workflow](../../../.claude/skills/free-space/SKILL.md) for the read-only
survey and guarded handoff. The [worker hygiene runbook](worker-hygiene.md) covers optional
archival copies from workers; it does not change the local retention rule above.
