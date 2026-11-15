# Weekend focus recording inventory

- **Status:** Four complete foreground recordings verified in the main catalogue; alignment dependencies remain unavailable
- **Related:** [Weekend evidence plan](../../plans/lidar-weekend-following-drac-plan.md), [Gerardus alignment](gerardus-alignment.md)

The November 14 weekend review now has complete, settled-first foreground recordings for
Kirkham at 9th Avenue, Columbus at Broadway, Embarcadero at Bryant and Lombard at Broderick.
They are indexed in the main `sensor_data.db`, served on HTTP port 8080. Refresh Analysis Runs
on that server; the separate review server on port 18080 has a different catalogue.

## Recording inventory

Durations describe the first to last recorded sensor rotations. Track counts describe indexed
software tracks, not independently identified vehicles.

| Site                  | Run ID                                                 | Duration (s) | Foreground rotations | Indexed tracks |
| --------------------- | ------------------------------------------------------ | -----------: | -------------------: | -------------: |
| Kirkham at 9th Avenue | `cac4f6e4-441b-4c22-9f06-080428cd1261`                 |    83.084513 |                  832 |             92 |
| Columbus at Broadway  | `8d884a74-6c0a-4666-a873-5c2181c28207`                 |  2032.825674 |               20,330 |          4,308 |
| Embarcadero at Bryant | `9f4a5f41-ec66-4a24-9b50-7a432146209f`                 |  1362.317242 |               13,625 |          4,013 |
| Lombard at Broderick  | `replay-052fa966-5cdf-4792-9186-7fefc4682e29-1c36dd9e` |  1201.707292 |               12,018 |          2,682 |

Each recording used a separate unrecorded settling pass before replaying the complete capture
from its first rotation. Foreground rotations and empty placeholders are accompanied by separate
background snapshots. The first background snapshot appeared within the first ten records.

Kirkham, Bryant and Columbus were recorded for this inventory. Lombard was imported with its
configuration lineage and track snapshots from the completed dedicated review database; its
original database and recording remain available. All four live in the existing local VRLOG
library, whose canonical root is `/Volumes/lidar/offload/sensor_data/lidar/vrlog`.

Every record was decoded to EOF. Verification rejected full-cloud frame types, checked header
counts and recorded bounds, and compared source and destination SHA-256 hashes for every copied
file. The three S2 rotation counts also matched the earlier complete scene-recording inventory.
The main server returned the expected clip, foreground mode, frame count and recording path for
each run. This establishes file and catalogue integrity; it does not establish rendered quality
or physical measurement accuracy.

## Catalogue repair

Linked 1,439 historical runs to existing located clips. Of these, 120 had matching capture
identity and complete recorded intervals within the indexed site bounds, allowing 200 ms for
packet/rotation boundaries. Another 1,319 named two historical Kirkham capture paths whose bytes
were verified identical to the canonical whole Kirkham capture. Their site can be assigned
because every subset remains at that site; missing historical replay intervals remain unknown.

The repair also supplied dataset identifiers for the 24 archive clips and filled 24 empty site
labels, including all four focus sites. Existing non-empty labels were retained. There were no
broken run-to-clip references after repair. Other ambiguous or insufficiently described runs
remain unassigned; the final catalogue has 82 such rows, including an empty failed setup attempt.

The server included the unrecorded settling pass in the four runs' counters and track snapshots.
For these review entries, frame and cluster totals were normalised to decoded recording content,
and unrecorded settling-pass track rows were excluded after checking they had no human labels.
Before-images are retained in the recovery receipts. Aggregate statistics were cleared because
they included both passes. This is a repair of these entries; the runtime's pass accounting still
needs a code fix before future recordings can rely on those totals without reconciliation.

## Gerardus disposition

Live registration-record queries confirmed accepted poses for all four sites:

| Site                  | L16 site ID | Accepted disposition                                                          |
| --------------------- | ----------- | ----------------------------------------------------------------------------- |
| Kirkham at 9th Avenue | `8085875ef` | Registered; multiple live accepted scans, including two naming the focus clip |
| Columbus at Broadway  | `808580f47` | Registered, `gms:01M5GKWEZRV8YEVPSRTA1YGE6Y`                                  |
| Embarcadero at Bryant | `80858070d` | Operator-accepted manual placement, `gms:01M684AR57ZPDAHJKC6ZTABDS8`          |
| Lombard at Broderick  | `808580d49` | Operator-accepted manual placement, `gms:01M65C8GPFW8M1PEW4F9M54TZY`          |

Alignment is not ready for an unattended handoff. `GetSiteModel` failed for all four because the
active service's prepared-site and blob paths reference the unavailable `/Volumes/banshee-results`
mount. A checked local experiment copy contained some dependencies but lacked the accepted Bryant
and Lombard scan blobs. Restore the complete matching dependencies locally, then verify a cold
alignment start with the NAS disconnected. Kirkham additionally needs an explicit accepted scan
selection; multiple live poses must not be collapsed into an arbitrary automatic choice.

## Frozen provenance and limits

The three new recordings were made by version `0.5.1-pre52`, build
`0359442e6d33d4de50df01fcfa0a8277736aecad`. Lombard's producer was
`9ee5ea36d5550b171e4ed773f3e23fd575b839b8`. The three new replay requests selected
`config/tuning.defaults.json`, byte SHA-256
`9ad7377fa5d356ba5f9d8756d48721708071da3295702350c835110b38b41675`.
The site-index SHA-256 was
`219c5649224da771cd9c204654334fac8a89e2392f8d31f2e494239a9a96dceb`.
The verified Kirkham capture SHA-256 was
`2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`.

Detailed decode, copy, catalogue before-image, main-API and registration receipts are retained
outside Git at `/Volumes/lidar/reviews/weekend-focus-20261114/verification`, with a per-file hash
manifest. Internal source copies were retained. The isolated replay server was stopped after
the recording passes completed.

These are production review recordings. They do not replace the frozen following-evidence
recordings or their source/window bindings, admit Lombard to the development cohort, validate
following or DRAC physically, or adopt tuning changes.
