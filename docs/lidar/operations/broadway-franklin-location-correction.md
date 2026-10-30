# Broadway at Franklin: capture location correction

- **Status:** Location corrected; registration rerun awaits the aerial source
- **Scope:** S2 archive visit `s06`, September 1, 2026, 15:10–15:29 PDT
- **Related:** [archive index](../../../tools/s2-archive/README.md),
  [registration experiment](gerardus-static-registration-2026-10.md),
  [background-window experiment](gerardus-background-window-2026-10.md)

The operator identified this capture as Broadway at Franklin, at WGS 84
`(37.7955, -122.4249)`. The incorrect point, `(37.7952, -122.4269)`, was about 179 m away.
This is an operator-provided junction position, not a surveyed sensor pose.

The site is `broadway-franklin`, with S2 L16 `808580c27`. Its L13 `808580c4`,
L10 `808581`, archive visit `s06`, five contributing captures and recording window are unchanged.
The window is `2026-09-01T15:10:31.646959-07:00` through
`2026-09-01T15:29:09.163086-07:00`, 1,117.517 seconds.

Names and coordinates were replaced in the field mark, index, public catalogue, scene export,
corpus selectors, experiment specifications and reports. The local dataset, export sidecar,
database clip and Gerardus documentation were corrected too. The website keeps one redirect
from the former survey URL. There is no alias registry or migration compatibility layer.

The raw dataset capture is
`raw/lidar/20260901T1510_80858-0c4_broadway-franklin.pcapng`:
2,695,469,364 bytes, SHA-256
`8169d57498379e2d5c0f70720287cb435c6240b58ecfdc9267cbde1e19e0c704`, verified during the move.
Its five source captures are the September 1 S2 files numbered `00029`–`00033`.
No packet selection, frame data or measured result changed.

The scene's source VRLOG SHA-256 remains
`51142d48e683913b70ed3c525eb6a97aa4b848d3e609788cd9d9f5e5f3e48a3e`, build
`0.5.1-pre45`, generated `2026-10-20T07:40:35Z`, tuning hash
`sha256:ade9f1afa5b40adadae0b97fc5c9f70d9ce69ef6fd2a13000a9c0880fcdd2303`.
The coverage declaration's content address changes because its source description contains
the corrected name; capture, observation, parameter and build digests remain unchanged.
The 359° grid orientation, 99° north orientation and assumed 25 mph limit have not been revalidated.

The main local database clip is `42da7631-20a7-4481-89ce-2d0132175d31`.
Its description, coordinates and S2 association were corrected; SQLite's integrity check passed.
The mounted dataset and Gerardus documentation edits are local changes, outside this repository PR.

In `pose0001`, this case failed with a heading 92.6° from the incorrect hint.
In `pose0002`, all four windows failed. Those measured outcomes are retained with the corrected
case label, but they do not establish failure at the corrected junction. A new reference bundle
and registration rerun are required; no pose has been accepted by this correction.

The archived 3DEP source paths are `/mnt/results/sf-gis/CA_SanFrancisco_1_B23` and
`/Volumes/banshee-results/sf-gis/CA_SanFrancisco_1_B23`. Neither is mounted here.
Restore the source and rerun the original arms and background windows before revising
registration conclusions. Keep the original numerical evidence intact.
