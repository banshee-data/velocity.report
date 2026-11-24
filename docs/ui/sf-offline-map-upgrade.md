# SF offline map upgrades

This investigation checks a completed offline download across two pinned map releases in
real Chromium. It separates cached map availability from saved emoji and checkpoint state.

## Scope and provenance

Data is the primary anchor. Goals covers the collected emoji and checkpoint, UI covers
the completion and coverage messages, and World constrains whether an old tile can safely
be used with a new manifest. There is no Physics or Traffic behaviour change in this PR.

| Arm           | Git commit                                 | Map revision       | Modules      |
| ------------- | ------------------------------------------ | ------------------ | ------------ |
| Old, PR #1158 | `5af9d8629e6ae1c084c931b471b2c6bbceb49bcc` | `c63a335182fdbe34` | `a94f5d4404` |
| New, PR #1159 | `41c7ccf3948a002ec5ec35566a5f6225958e3f2e` | `6c3972a007c14ff4` | `1912acf4ae` |

Both contain 157 tiles and the same trial base and emoji-placement identities. The two
releases differ in four tile payloads: `8085807c`, `80858084`, `808f7e2c`, and `808f7fd4`.
The remaining 153 tile payloads are byte-identical. The compressed tile files total
31,058,159 bytes in the old release and 31,075,746 bytes in the new release. These are
payload sizes, not measured browser disk allocation.

PR #1115 tests interrupted response bodies and quota errors within one worker revision.
It does not exercise browser worker installation, activation, or a map revision change.
The much older #1115 release also precedes a progress-key change, so this investigation
uses adjacent map releases to avoid conflating progress identity with cache replacement.

## Method

Build both pinned revisions with `make build-docs`. Run the checked-in
[`smoke-offline-upgrade.cjs`](../../tools/sf-drive/smoke-offline-upgrade.cjs) with Playwright
on `NODE_PATH` and Chromium available:

```sh
CHROMIUM_PATH=/usr/bin/chromium node tools/sf-drive/smoke-offline-upgrade.cjs \
  /workspace/map-upgrade-old/public_html/_site \
  /workspace/map-upgrade/public_html/_site
```

The harness serves both unmodified built sites at one ephemeral localhost origin and
switches the server root during the run. It creates a disposable browser context; it does
not attach to a user's browser, clear real storage, or access a deployed site. Chromium
154.0.8037.57 uses software WebGL in the cloud environment.

The old page opens `/sf/?at=embarcadero` and uses the menu's **Download all tiles** button.
The harness seeds a checkpoint ten metres before a real quest target, with no collected
emoji. It then goes offline, reloads, takes manual control, drives through the pickup, and
reloads again. The pickup is produced by the game, not inserted into the collection.

With the old tab still open, the server changes to the new release and the harness calls
`ServiceWorkerRegistration.update()`, waiting for `controllerchange`. This deterministically
requests the browser's real update lifecycle rather than waiting for its periodic check.
It tests tile fetches from that old document, a fresh offline page, another offline reload,
and the same cases after using the new page's download button. It reads Cache Storage and
localStorage as evidence; it does not inject a worker or mock those APIs.

The default mode characterises the present failure. Add `--require-continuity` to require
that a tile in the completed old download remains fetchable after activation. That mode
is intended to fail on the pinned releases and become the acceptance guard for a remedy.

## Results

The full default scenario passed its characterisation assertions. The completed old
download did **not** remain usable through the newly activated worker.

| Browser state                                            | Offline tile fetch                             | Play state                                                 | Emoji and checkpoint                          |
| -------------------------------------------------------- | ---------------------------------------------- | ---------------------------------------------------------- | --------------------------------------------- |
| Old release, completed download, offline reload          | Available                                      | Grounded; 157/157 coverage                                 | 1/137 emojis restored at the saved checkpoint |
| Old tab left open after new worker activation            | Fails for a tile still in the old cache        | Resident scene remains grounded; old UI still says 157/157 | Preserved                                     |
| Fresh offline page after activation, before redownload   | Fails; six tile requests failed during startup | Contact stays `loading`; 0/157 coverage                    | 1/137 displayed; checkpoint retained          |
| Another fresh offline reload before redownload           | Still unavailable                              | Contact stays `loading`                                    | Preserved                                     |
| New release after complete redownload and offline reload | Available                                      | Grounded; 157/157 coverage                                 | 1/137 restored at the same checkpoint         |
| Old tab after redownload                                 | Same previously failing fetch succeeds         | Resident scene remains grounded                            | Preserved                                     |

The old tab's 157/157 indication counts its original cache, while its replacement worker
reads the new cache. Its resident scene masks the failure until another tile is needed.
The generic download prompt after `controllerchange` does not explicitly warn that the
completed offline package is no longer being served.

The retained checkpoint was `(4402.2434332737, 3412.5029982110073)` with heading
`1.5374753309166493`. After the final offline reload the car's displayed coordinates were
`(4402.243, 3412.503)`. The collected token was `8085806c`. Saved elapsed time continued
to change; this test does not claim the entire progress object is byte-identical.

A direct request for unchanged tile `8085800c` failed with `Failed to fetch` after
activation, despite the old cache containing it. After redownload the same request
returned 513 bytes in both documents. The startup tile failures and ungrounded fresh
page establish a gameplay consequence beyond that direct fetch probe.

### Cache retention

| Point                                  | Old shell entries | Old tile entries | New shell entries | New tile entries |
| -------------------------------------- | ----------------- | ---------------- | ----------------- | ---------------- |
| Old complete download                  | 50                | 157              | Absent            | Absent           |
| New worker activated, before the probe | 50                | 157              | 50                | Absent           |
| New complete download                  | 50                | 157              | 50                | 157              |
| Final offline reload                   | 50                | 157              | 50                | 157              |

The probe creates the empty new tile cache, as does the worker's first tile request.
One real upgrade therefore accumulates two complete tile sets, 314 entries. The server
recorded 314 successful tile requests over the complete scenario. The combined compressed
payload size is 62,133,905 bytes, excluding shells and browser storage overhead. Longer
sequences were not measured. Continued accumulation follows from the worker having no
cache deletion path, but browser eviction can change retention.

### Limits

This is a cloud Chromium test at a localhost origin with software WebGL, not a production
CDN or mobile-browser test. It tests one adjacent release pair with compatible progress
identity, not migrations that change the trial base or emoji placement. It does not test
quota pressure during an upgrade, browser eviction, process restart, or every old-module
interaction with changed geometry. A fresh page and fresh reload were tested within the
same disposable context. The old scene remaining grounded does not prove that driving
through all changed tiles with old modules is safe.

## Mechanism

The [worker](../../public_html/src/sf/offline.njk) installs the shell, calls
`skipWaiting()`, and claims clients on activation. `tileResponse()` opens only the active
revision's tile cache. A miss goes to the network even when an old cache still contains
that URL. Activation has no cache retirement step. Progress lives separately in
localStorage under the compatible trial identity, which explains its survival in this pair.

## Remedy boundary

Do not implement a generic `caches.match()` fallback across map revisions. Four payloads
changed in this small upgrade, and tile URLs do not carry revision identity. Serving an
old tile under the new manifest can mix geometry revisions.

Any continuity remedy must treat the shell, manifest, route, and tiles as a compatible
package. Cache retirement must account for a tab that still runs old modules. Removing
all old caches immediately on activation would destroy the completed download without
making the replacement complete.

## Smallest safe remedy, proposed separately

For an immediate narrow mitigation, make `controllerchange` explicitly invalidate the
old document's offline coverage claim and show that the map has changed and must be
downloaded again. This corrects the misleading UI but **does not preserve offline
continuity**. Do not describe that mitigation as fixing the download loss.

To preserve the completed offline download, stage the replacement as a complete compatible
package before allowing it to replace the old worker. When a complete old download exists,
fill and validate the new revision's tiles during installation; a failed transfer or quota
error must leave the old worker/package usable. Avoid forced takeover of an old document:
let the replacement wait until old clients close, or require an explicit coordinated reload.
Only then retire superseded caches. This retains the existing revision-specific storage
model rather than introducing unverified cross-revision tile reuse.

That proposal needs separate tests for interrupted upgrades, quota failure, waiting workers,
and old clients. It is not implemented in this investigation. The current reproduction is
a starting guard; its characterisation assertions for the present failure and retained
caches must change alongside the selected lifecycle policy.

## Validation

- Both pinned sites built through `make build-docs`.
- Disposable Chromium completed the full characterisation scenario with no page errors.
- A second complete run with `--require-continuity` exited 1 at the intended assertion:
  `A completed offline download stopped serving tiles after worker activation`.
- `make test-public-html`: 531 tests passed.
- `make lint` reached the documentation spelling check and failed on existing
  `serialization` and `normalized` in `docs/plans/lidar-survey-lane-exports-plan.md`.
- `make test` passed its Go stage and stopped in Python with 472 passed and six failed.
  Four failures/subfailures in worker-hygiene archive tests could not access `/dev/mqueue`;
  two podcast-publication tests rejected their `/tmp` destinations as inside a Git checkout.
  These unrelated checks are not changed by this PR; later aggregate targets did not run.
- Document structure, Mermaid, geometry wording, agent-document links, relative links,
  devlog fragments, and the no-results guard passed.

Raw logs stay outside Git in the cloud workspace. The reusable harness and this authored
summary are the reviewable evidence. No production code was changed, merged, or deployed.
