# Native trail support and historical display anchors

- **Status:** Engineering implementation; physical/visual acceptance remains open
- **Scope:** Native continuation of the [HTTP/web trail contract](trail-support-cloud-2026-11.md)

Native trails discarded L5's support tokens and drew filter XY history at a fixed height as
one green strip. That joined missing history and showed coasted predictions as observations,
while the rendered box could have its own published centre.

## Producer and wire contract

Each newly produced L5 history point now snapshots the published display centre and base after
the frame's box update. Filter XY, acquisition time, length accumulation and estimation remain
unchanged. Display geometry is held separately, with the state frame's nanoseconds. Birth,
observed and coasted samples use the same capture-time contract. An origin-suppressed update
cannot overwrite an older sample. The anchor is a small enum in history rather than a string
allocation per sample.

L9 copies that snapshot into a trail point. Additive protobuf fields carry support, estimate
stage, reference anchor, height, revision and source tokens. An additive frame timestamp
preserves the existing acquisition timestamp; rendering uses frame time when recorded. Modern snapshots are explicitly
online; old/restored history retains unrecorded stage/support and a legacy XY-only anchor.
Missing revision/source identity remains empty. No final or provisional estimate is fabricated,
and this change does not select persisted final trajectories for publication.

HTTP filter-history responses also expose the optional state-frame nanoseconds, alongside
unchanged acquisition timestamps. Old history has no invented state-frame time.

## Native rendering

The native client preserves the wire metadata. A pure geometry helper reconstructs independent
edges for each displayed frame, so seeking and sequential display share one path. It refuses
non-finite points, future samples, non-increasing times, gaps over 500 ms and duplicate trail
identities. Source, revision, stage and anchor changes discard older geometry for that trail.
Hidden, absent and deleted identities draw no trail, and a frame without tracks clears the old
trail buffer.

Observed edges are green; predictions are dashed amber, and unrecorded support is dashed grey.
Explicit provisional/final observations are cyan/purple; predicted support takes precedence.
Each modern point uses its historical box base plus 5 cm, rather than today's box position or
height. Legacy XY-only points keep their explicitly unknown 10 cm display plane. All segments
are independent line pairs, including world-distance dashes. History and total emitted geometry
are bounded. The control tooltip explains the styles.

## Automated validation

Go tests check birth/observed/coasted snapshots, separate acquisition/state times, immutable
older display geometry, unknown legacy metadata and protobuf round-trip preservation, including
nanoseconds beyond JavaScript integer precision. Existing history, box and adapter tests cover
unchanged filter history and published boxes.

`make test-mac-trails` compiles the actual pure Swift helper with a standalone contract test.
It needs no Metal device or app host. The macOS build CI invokes it before building the app.
The contract exercises support/stage styling, historical height, capture gaps and rewind,
seeking, future/invalid samples, expired/hidden identities, duplicate IDs, timestamp overflow,
revision/source/anchor changes and geometry limits. Generated Swift protobufs continue to be
produced by the existing macOS generation job; the Go messages were regenerated with the pinned
protoc 33.4 and protoc-gen-go 1.36.11 tool versions.

Human inspection of recorded stops, turns, occlusion/re-entry and revised physical histories
remains necessary for visual acceptance. The separate uncertainty, smoothing and physical
reference gates remain open. No tracker tuning default or smoothing horizon changes here.

## Recorded engineering verification

The implementation at `b9501f58f56801f70c4f2898c0a373ccb4f559d9` passed the full Go suite, Go/documentation lint, TypeScript checks and all 861 web tests. The Makefile native binary reported that source commit. PR [#947](https://github.com/banshee-data/velocity.report/pull/947) also passed macOS CI, including the standalone Swift geometry contracts and the Xcode app build, before landing. These automated checks complete compilation and numeric rendering-contract verification; they do not replace recorded-scene visual review or promote a final trajectory stage.
