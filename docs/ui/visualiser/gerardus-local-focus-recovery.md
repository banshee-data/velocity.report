# Gerardus local focus-site recovery

The four weekend sites are Kirkham at 9th Avenue, Columbus at Broadway,
Embarcadero at Bryant and Lombard at Broderick. Their recording inventory is in
[the weekend review record](weekend-focus-recording-inventory.md).
Ashbury at Downey was subsequently added to the same local working set. All five
sites passed the November 15 native load checks with the Mac's NAS shares unmounted;
the [five-site verification](#five-site-verification-with-the-nas-unmounted) records
the tested recordings and remaining limits.

## Storage and restart

The restored working set is
`/Volumes/lidar/evidence/gerardus/focus-offline-20261114`. Its `serve.sh` starts
Gerardus on `127.0.0.1:50061`; an optional first argument changes the listen address.
The initial recovery used `0.2.0+1e4620fe1dc2-dirty`. The five-site verification
used `0.2.0+683b09dac4bc`, from the work subsequently merged in
[Gerardus #88](https://github.com/banshee-data/gerardus/pull/88), with `--offline`,
`--serve-scans` and `--no-warm`. Its sandbox profile blocks the NAS mount paths
and outbound connections. Keep `/Volumes/lidar` mounted while using this set.

Prepared bundles, scan blobs, geometry products, public-source caches and city
tiles live on lidar. Active SQLite stores live at
`/Users/david/Library/Application Support/Gerardus/focus-offline-20261114`:
the attached volume is Journaled HFS+, so the active databases retain internal
filesystem locking. All 28 cell databases and the map database have independently
hash-verified backups on lidar. SQLite backup operations included committed WAL
contents; each destination passed `PRAGMA integrity_check`.

The source NAS tree and original temporary working directory are retained.
The latter is now `/private/tmp/velocity-gerardus-data.pre-local-20261114`;
`/private/tmp/velocity-gerardus-data` redirects to the restored local data.
This is a five-site prepared working set, not a replacement for the complete
city catalogue. Registration databases and blobs retain their historical records.
The full city roof bake is not enabled: the original service had no such bake
configured, and enabling one would change the geometry being reviewed.

Runtime aerial index roots were rebased to local copies. Original indexes remain
alongside them as `index.source.json`; immutable source attribution in bundles
and records was preserved. The local raw EPT mirror contains 2,792 files covering
the four L16 cells and their immediate neighbours, including the dataset metadata.
It is a bounded source subset, not the complete city survey.

The visualiser uses HTTP `http://127.0.0.1:8080`, frame stream
`127.0.0.1:50051` and Gerardus `127.0.0.1:50061`. Its City tiles override is empty,
so tile URLs come from the local Gerardus server rather than a NAS folder.

## Kirkham scan choice

The focus clip is `c93a44e6-1f8b-4da2-89f5-8ef587b60b3a`. Its selected accepted
scan is `sha256:9f547025e08237d02163c19d5a6f7e3caa20e513e7283ae974581573e410bbb3`,
current record `gms:01M5XF38FPFQGF26BKZWK8DTEF`.

The other focus-clip scan, `sha256:8eb57e2f36ec764c1560e20736112258c65b016e5725af990a36bcaa74c738c2`,
contains the same 65,252-point blob. The accepted translations differ by about
4.4 nanometres and quaternion components by about 2e-10. These are numerical
duplicates, not evidence of distinct sensor positions. Both records are retained.
Their legacy capture timestamps describe replay-era metadata; they were not
rewritten as physical capture times.

The visualiser remembers an explicitly selected accepted scan per L16 and clip.
Automatic placement does not overwrite that choice. A later run UUID can reuse
the same choice; a missing or no longer accepted current head fails closed. Explicitly
accepting a replacement scan updates the choice. Backend jobs should pass the
record ID in the local binding manifest: a desktop preference does not change
the server's registration history or resolve every ambiguous CLI request.

## Verification record

The cold test started an isolated service on port 50561 with NAS file access and
outbound connections denied. A negative control failed to read a known existing
NAS `site.pb` with `Operation not permitted`. For each site, the probe requested
the site model and streamed all 14 supported reference layers to EOF. Every
stream reported completion. Model IDs, bundle IDs, accepted-scan counts, item
counts and deterministic protobuf stream SHA-256 hashes matched the NAS-backed
baseline exactly across all 56 streams.
The normal port 50061 passed the same comparison after a second cold start.
The local HTTP tileset and each focus site's GLB were fetched successfully and
matched their local file bytes. The visualiser discovered all 8,428 city tiles
from this endpoint with its NAS override removed.

The copy receipts cover 11,359 dependency and tile files, 2,792 raw EPT files and
29 database snapshots: 7,148,676,936 bytes in total, excluding the service binary
and verification receipts. Source bytes were hashed during copying; destinations
were independently re-read and hashed. A stopped, partial city-wide cache copy
was retained outside the active data directory; it is not required by the launcher.

| Site                  | Accepted scans | Aerial points | Registered points | Ground cells | Box wall features |
| --------------------- | -------------: | ------------: | ----------------: | -----------: | ----------------: |
| Kirkham at 9th Avenue |              7 |     2,635,638 |           332,483 |      203,250 |            19,623 |
| Columbus at Broadway  |              1 |     3,250,021 |            36,766 |      203,250 |            23,606 |
| Embarcadero at Bryant |              1 |     2,970,735 |            51,198 |      203,250 |             4,477 |
| Lombard at Broderick  |              1 |     2,950,793 |           119,967 |      203,250 |            22,066 |

These whole-bundle counts differ from the visualiser's radius-filtered view.
Kirkham's native alignment pane successfully loaded buildings, roofs, ground
and all seven accepted scans. Explicitly selecting the focus scan persisted
`alignment.preferredScan.8085875ef.c93a44e6-1f8b-4da2-89f5-8ef587b60b3a`
with the selected scan ID in the app's sandboxed preferences.
After reconnecting to port 50061, placement reused that choice at heading 235.3°.
The foreground replay reached its 1:23 end with a 69,718-point background snapshot
available. The running viewer is the rebuilt app under
`tools/visualiser-macos/build/Build/Products/Release/VelocityVisualiser.app`;
the separately signed `/Applications` copy was not replaced.

`make build-mac` succeeded. All 27 `AlignmentSessionTests` passed, including
selection across sessions and new run IDs, and refusal of stale or rejected
heads. The broader native suite reported 2,840 passes and one unfinished
`RoadTraverseBenchmarkTests/tenMinuteTraverseSettlesAndStaysCheap()` runner exit;
the full suite is not claimed green. The geometry stream test is separate from
rendering performance or ten-minute road traversal acceptance.
Subsequently, [velocity.report #1042](https://github.com/banshee-data/velocity.report/pull/1042)
reported 849 XCTest and 1,995 Swift Testing cases passing on its final combined head.
That later suite result was not re-run as part of the data recovery checks.

Recovery receipts are retained outside Git under the restored working set.
`verification/receipt-manifest.json` covers the diagnostic receipts;
`copy-manifest.json`, `ept-copy-manifest.json` and
`database-snapshot-manifest.json` identify the restored evidence. The exact
accepted run/clip/scan/record bindings are in `focus-bindings.json`.
They include source/destination copy hashes, database integrity and row counts,
index path repairs, the producer binary hash and reference-stream comparisons.
Originals were preserved. These checks establish local data availability and
software behaviour; they do not establish physical alignment accuracy or adopt
new geometry or tuning parameters.

## Five-site verification with the NAS unmounted

On November 15, the Mac's `banshee-results`, `banshee-captures` and `media` shares
were unmounted using ordinary, non-forced unmounts. `mount` showed no `smbfs`
volumes before the cold start and after the native checks. The Gerardus launcher
continued to deny NAS paths and outbound connections. The visualiser and main
Gerardus process were restarted; the local HTTP/frame-stream server remained on
ports 8080/50051. Its stamped producer was `0.5.1-pre52`, commit `b9109a19feaa`.

For each site, the probe streamed all 14 reference layers to EOF and required
non-empty aerial points, registered scans, ground mesh and box geometry. All 70
streams completed. The four original focus sites matched every field and stream
hash in their November 14 NAS-backed baseline. Ashbury was checked against its
restored local generation, whose survey cut differs from the earlier NAS cut;
it is not described as a byte-identical restoration of that earlier generation.

Each recording below was then explicitly opened through **Analysis Runs**, paused
after foreground playback advanced, and placed with **Align → Place from hint**.
The pane resolved the clip named by the run. Buildings, roof faces, ground and
registered scans were visible in the native renderer with both servers connected.
No new pose acceptance was written. Counts below describe the native view's
150 m reference radius, not the whole-bundle stream counts above.

| Site                  | Run ID                                                 | Foreground interval | Building parts | Roof planes | Building walls | Accepted scans | Registered points | Ground cells |
| --------------------- | ------------------------------------------------------ | ------------------: | -------------: | ----------: | -------------: | -------------: | ----------------: | -----------: |
| Ashbury at Downey     | `c705d3d2-d250-485c-bbc2-0896d6b0e8d9`                 |         89.998027 s |            349 |       1,110 |          5,641 |              2 |            87,299 |       86,976 |
| Kirkham at 9th Avenue | `cac4f6e4-441b-4c22-9f06-080428cd1261`                 |         83.084513 s |            301 |         847 |          5,011 |              7 |           330,865 |       86,674 |
| Columbus at Broadway  | `8d884a74-6c0a-4666-a873-5c2181c28207`                 |      2,032.825674 s |            277 |         961 |          5,077 |              1 |            36,611 |       91,137 |
| Embarcadero at Bryant | `9f4a5f41-ec66-4a24-9b50-7a432146209f`                 |      1,362.317242 s |             41 |         156 |            796 |              1 |            50,128 |       90,198 |
| Lombard at Broderick  | `replay-052fa966-5cdf-4792-9186-7fefc4682e29-1c36dd9e` |      1,201.707292 s |            266 |         669 |          5,144 |              1 |           117,901 |       86,070 |

All five capture and VRLOG paths resolved to `/Volumes/lidar`, and independent
playback-status receipts confirmed VRLOG mode with `full_frames=false`. Background
snapshots were available in all five views. Kirkham retained the selected scan
`sha256:9f547025e08237d02163c19d5a6f7e3caa20e513e7283ae974581573e410bbb3`
after the fresh app start and drew its accepted 235.3° heading.

Ashbury's existing foreground VRLOG is a 90-second preview, not its complete
20.5-minute capture. A fresh EOF decode read 905 records: 736 foreground rotations,
165 empty rotations and four background snapshots, with no full-cloud frames.
Its producer was `0.5.1-pre47`, commit `ab787541c3cd`. This check neither generated
a new full-length Ashbury recording nor reconciled its historical database frame
counter. The four complete focus recordings retain the earlier full-file decode
evidence; this native session checked loading and a short playback interval for each.

One client restart was observed during the first Lombard load. Reloading the exact
run restored its clip context and passed the native geometry check. These results
establish local availability and successful native loads, not crash-free continuous
playback. The separately reported point-cloud renderer crash remains under
investigation in a separate worktree. After reconnecting a client to an already
running replay, explicitly reopen the run before relying on its alignment context.

Raw receipts and diagnostic probe sources remain outside Git under
`/Volumes/lidar/reviews/five-site-offline-20261115/verification`, including mount
inventories, 70 stream hashes, per-site run/model/playback replies, native findings,
the Ashbury EOF decode and Kirkham's persisted preference. The shares were left
unmounted at the end of verification; `/Volumes/lidar` remained mounted.
