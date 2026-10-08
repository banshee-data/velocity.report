# Development log

<!-- ignore-style-length -->

This is the chronological engineering journal: what changed, why it mattered, and the evidence
that made it worth recording. Entries are historical records, so new work belongs at the top and
older entries stay put, however tempting hindsight may be.

**Formatting:** one `## Month DD, YYYY - Theme` heading per UTC date, newest first, with no date ranges. Each bullet is one line in the past tense and ends with the pull request(s) that delivered it, `(#NNN)`. Unlanded branch work starts with `{branch-name}` until it merges. See `.github/STYLE.md` (Logs and registers).

## October 8, 2026 - Motion passes report progress, replays read each capture once, facet F0 on kirk0, human crash baselines, and the access gate answers #503's review

- {claude/capture-reads-once} Made a session's motion pass name the capture it is reading and stop when cancelled. It had reported "0 of 27" until it finished, and a cancelled pass read on regardless.
- {claude/capture-reads-once} Joined a session's captures from the extents the index probed instead of counting every capture first, which had read each one twice. Progress is written to the job row from its own goroutine, keeping only the newest update, so a slow write never stalls the read.
- {claude/capture-reads-once} Took replay's capture extents from the index too, for multi-file clips and single captures alike, so a replay no longer reads each file once to count it before reading it again. Case authoring now matches index rows by the exact file rather than the file name, which two folders of copies share.
- {claude/facet-f0-cpu-analysis-d2ae1e} Ran the facet F0 attribution protocol on kirk0's reviewed pack, tuning only, in the [F0 report](lidar/operations/facet-f0-attribution-kirk0-report.md): the control's end face sat a median 0.85 m short of 3,151 labelled masks' returns, and two slow trucks gave half the face scores.
- {claude/facet-f0-cpu-analysis-d2ae1e} Found the believed length shorter than the labelled span in 83 % of car and 97 % of truck instants; removing that forced shortfall moved the end-face median to −0.02 m, so the predeclared gate reading that kept arm D's case open rested on an extent error, and the gate needs restating.
- {claude/facet-f0-cpu-analysis-d2ae1e} Confirmed on labels that A2 lowered the end-face residual by 0.13 m at an AssA cost of 0.16, A1 doubled ID switches from 100 to 199, T5 moved no face toward the sensor, and the sparse tail began in the 50 to 80 m bin.
- {claude/facet-f0-cpu-analysis-d2ae1e} Corrected the F0 protocol's kirk0 capture digest to the git-lfs object ID both copies hash to.
- {claude/laughing-curie-2l2rl5} Analysed the Waymo sober-driving baseline preprint, its fatal-rate base paper and Valgo's Human Crash Baselines API against the crash-data integration plan: re-derived the exposure-reconstruction equations, showed by Poisson arithmetic that a single site cannot test an area rate, and set out benchmark-aligned day/night and weekday/weekend strata, an area-rate table for the safety-reference edition, and what a speed survey gives back to the benchmarks ([report](platform/operations/human-crash-baselines-analysis-2026-10.md)).
- {claude/laughing-curie-2l2rl5} Folded the analysis's five edits into the crash-data integration plan: benchmark-aligned temporal strata, an area-rate table in the safety-reference edition, site-to-area resolution with an area benchmark context block, a hosted-baselines exploration, and a multi-site research note, with the new rights rows, four risks, and two open questions answered; the backlog mirrors them at v0.5.10 and v0.6.8 ([plan](plans/platform-crash-data-integration-plan.md)).
- {patrickod/tailscale-acls} Dropped connections from the host on every listener while tailscaled forwarded to its port through a Serve handler the manager had not installed: a TCP forward or TCP Funnel had read as the host or as a forged tailnet identity (#503).
- {patrickod/tailscale-acls} Refused an unparseable forwarded address in `on` mode, let the view grant read the offline docs, and pinned the view inventory and the review's integration matrix through the installed wrappers (#503).
- {patrickod/tailscale-acls} Chose the image's access profile with `VELOCITY_ACCESS_PROFILE`, so an operator's drop-in survives unit changes, and documented Serve forwards, 100.64 LANs and what `off` leaves open (#503).
- {patrickod/tailscale-acls} Refused requests whose Host a DNS-rebinding page could send, in every profile: a page in a LAN browser could otherwise start Tailscale enrolment under `off` and read the login URL. `--allowed-hosts` adds names such as a reverse proxy's (#503).

## October 7, 2026 - The LiDAR web pages read as one workflow, and access hardening keeps viewing separate from control

- Ordered the LiDAR pages in one navigation group as the work runs, linked each record to the next, moved Scene Map out of the navigation and called the replay case a clip in web copy only (#707).
- Opened Captures on a configured volume instead of a dropped one showing a month-old "context canceled", and kept a session's motion periods when a rescan reproduces it: 24 of 31 development clips named a period an earlier scan had deleted (#707).
- Kept copies of a capture in different folders as separate sessions in their own Coverage lanes, put every day on one clock axis, showed probe progress, queued every missing motion pass at once and wrote dates as yyyy-mm-dd (#707).
- Made Tracks read-only and played a selected run from its own VRLOG, exported on first request and cached: the observation table it had played has no run column, so a kirk0 run showed 3,681 track IDs where it had 87 (#707).
- Ranked a Segments run once, with its score strip in the same response, and kept pack listings and recording series until their files change: each load had re-read about 60 MB of review sidecars and the whole recording from the USB volume (#707).
- Matched a clip's volume-relative capture path to a run's absolute one, which had never matched, so 474 of the latest 500 development runs named their clip instead of none; Runs also showed each run's parameter digest and labelling share (#707).
- Named each sweep's clip or capture and its objective on the Sweeps cards, a plain sweep reading "none, compared by hand", and headed the macOS run browser's Case column Source, since it holds a file name (#707).
- Reconciled the October workflow design with the vocabulary plan in the [LiDAR workflow UI plan](plans/lidar-ui-workflow-plan.md): candidates on Clips rather than a Windows page, a derived pack status rather than a writable one, and sites through deployments, with eleven backlog items from v0.5.9 to v0.6.6 (#707).
- {patrickod/tailscale-acls} Added an opt-in hardened HTTP profile with anonymous LAN aggregate viewing and existing ordinary PDF downloads, transport-independent operation/resource checks, and explicit Tailscale configuration/report/export permissions. Kept off/on compatibility and reserved maintenance/access management for OS-authorised tools; native users/groups remain later work (#503).
- {patrickod/tailscale-acls} Replaced the ten-minute stale-grant fallback with bounded, coalesced lookups and a five-second cache; removed the race in which an older admin answer could resurrect a revoked grant. Added a dedicated loopback Serve capability backend, origin checks, redacted disclosure and caller-permission UI (#503).
- {patrickod/tailscale-acls} Constrained alternate LiDAR HTTP to loopback in hardened mode and full gRPC in every mode. The gRPC audit found shared playback state changes and return-to-live effects, but no implemented settings/file/recording RPC; corrected the advertised recording capability. Recorded the historical trust policy, reproduced cache failure and release split in the [access-control plan](plans/platform-access-control-hardening-plan.md); Pi/live-tailnet acceptance remains open (#503).

## October 6, 2026 - PCAP read in place, run statistics, scene clipping and held-out v2, runtime correctness closed, and a speed-limit design

- Read PCAP packets in place on the caller's goroutine instead of through gopacket's channel: replay CPU fell about 10 % and the scheduler's share from 57-59 % to 34 %, with byte-identical outputs (#679).
- Measured that replaying from the NAS doubled wall time against the internal SSD at the same CPU, so the PCAP analysis guide recommended copying a case's captures to local disk first (#679).
- Recaptured the mac perf baselines, which the comparator had refused since #613 moved the tuning fingerprint, and recorded that a capture from a worktree stamps the wrong commit (#680).
- Clipped scene headway encounters that cross the scene window to it and recomputed their accounting, windows and measurements, so an encounter crossing an edge was no longer silently dropped (R4) (#686).
- Moved held-out scoring to v2: the plan pinned the reference set's content digest, unmatched and unscorable shares failed a report past their bounds, and each stratum needed distinct encounters (#688).
- Folded a statistical review into v2: an unevaluated instant counted as a suppression in its stratum, non-finite and duplicate estimates were refused, and R6 recorded the point-estimate gaps that remained (#688).
- Timed whole replay frames under `-campaign-metrics`: A2 moved the frame 2 to 7 % at p99 where its tracker update alone was 9 to 21 times B0's, leaving the cost screen's scope to decide (#689).
- Stored run statistics when an analysis run completes, served them at `GET /api/lidar/runs/{id}/statistics`, and wrote the `lidar_tracks` quality columns from the tracker's lifetime counters (#690).
- Counted stored occlusions only over gaps a track was seen again after, because a confirmed track's coast before deletion had given nearly every finished track about 14 (#690).
- Measured track length across a gap from the last observed point, so an occluded car no longer read 19.3 m over a true 23.2 m passage (#701).
- Added distinct confirmed tracks to the perf gate's workload identity, compared once a baseline carried them: 77 on kirk0 against a peak of 25 at once (#692).
- Closed the runtime correctness plan's Phases 3 and 4: magnitude-only radar rows became diagnostics rather than transit inputs, and `/api/capabilities` reported LiDAR ready or error from its startup (#691).
- Fixed a transit rebuild stripping other model versions' links in its window by scoping the link refresh to its own model version's transits (#691).
- Withdrew the per-period speed limit columns and [planned posted limits on the vector scene](plans/posted-speed-limits-plan.md): signed value and unit, road segments split at signs, and radar suppressed across a sign (#685).
- Read an overflowing Tailscale long-poll version as 0 rather than MaxUint64, which had held the request for its whole wait (#682).
- Cleared every Dependabot alert with overrides in the docs sites, web, Python tooling and `public_html`, and merged the bumps for `source-map-js`, `postcss-selector-parser`, KaTeX and Mermaid (#683, #697, #698, #699, #702, #703).
- Moved to Go 1.27.1 for `tailscale.com` 1.105.0-pre, dropped the removed netmap bit from the IPN bus watch, and held SvelteKit 3 back (#693).
- Fixed the public_html timeline strip tests, which failed outside a browser, and ran that suite in `make test` and CI (#695).
- Widened the lidarbench heap-stability bound to 20 % for shared CI runners, and added the tests Codecov found missing in #686 and #688 (#687, #700).
- {patrickod/tailscale-acls} Rebuilt #503's Tailscale capability-grant authorisation on current main: routes outside the gate also refused outsiders, and an unresolved peer got a 503 after a 10-minute grace.

## October 5, 2026 - Near-edge findings and follow-ups, span selection, raw outputs stay local, a scene-change recovery plan, and the devlog register

- Planned recovery from the Lombard-Laguna survey's scene change: 45 track IDs appear within 0.2 s around 33:21, existing IDs survive, and near-static tracks persist afterwards. The [sprint plan](plans/lidar-scene-change-recovery-sprint-plan.md) does not assume the sensor was nudged (#675).
- Scoped a ten-engineer-day sprint: full-prefix reproduction, bounded L3 diagnostics, cause selection, a default-off recovery candidate, a lifecycle and quality export, and reserved-case evaluation. Seven backlog items spread the work from v0.5.4 to v0.6.7 (#675).
- Wrote an experiment protocol for pose change, model or decoding faults, and occlusion, with stopped-road-user, coverage, identity, speed, and Raspberry Pi gates. A public survey refresh and any default-on recovery wait on that evidence, and no capture experiment has run yet (#675).
- Consolidated repository-wide agent guidance in `AGENTS.md`. `CLAUDE.md` imports it, and the coding standards link to the experiment policy. The migration preserves the project guide and keeps tool entry points from carrying independent copies of the new rules (#672).
- Added default-off `-campaign-metrics` exports to the offline baseline tool for confirmed intervals and Tracker.Update wall time, with interval, censoring, miss-only expiry and baseline-equivalence tests. The [completed campaign report](lidar/operations/near-edge-campaign-2026-10.md) preserves methods, all 29 case gates, durations, diagnostic comparisons, balanced timing, robustness, and tail interpretation. Raw evidence remains local (#672).
- Retain B0: A2 breaches observed update cost at all 29 cases and geometry at nine. Complete mean duration decreases at 22 of 23 core sites and by 43% at Claren. No diagnostic correction is selected; physical references, raw-geometry intervals, and Pi validation remain unresolved (#672).
- Added repository-wide agent instructions and a Git ignore for result directories. A pre-commit check and CI reject tracked `results` paths, including force-added output. The publication branch was rebuilt without raw campaign output commits; reusable instrumentation and the detailed written report remain reviewable. Agent rules explicitly require reports in `docs/` and preserve them when cleaning up output (#672).
- Finished S2.4 of the near-edge plan: the fixed-lag smoother ended a chain where a track's reference point changed, `fixed_lag_rts` combined with `near_edge_track`, and each refined estimate got a refined solid body in the same transaction (#676).
- Covered `lidar_track_solid_bodies` in the evidence oracle, listed only when a run wrote solid bodies, so a run without them kept a byte-identical oracle (#676).
- Profiled B0, the shadow and A2 at the October campaign's timing sites with a new `-cpuprofile-dir`: the update-cost breach was the solid body's extent admission, a full sort on 21 axes per face, which the shadow paid too (#676).
- Re-ran D2 on kirk0 under the current solver: OBB-centre identity switches fell from 146 to 128 against the medoid's 92, so the medoid stayed production on a smaller margin, and two errors in the per-frame guide were corrected (#676).
- Surveyed sensor geometry for the 23 tuning and screen cases, all full circle; the held-out case waits on a governed split, and transient evidence moved to the internal disk after SQLite on the USB volume proved seek-bound (#676).
- Replaced the sort in the solid body's span measurement with a selection of its two percentiles; every span stayed bit-identical, and kirk0 shadow and A2 replays wrote identical baselines and solid-body rows (#678).
- Timed B0, the shadow and A2 on main and the selection build with the campaign's balanced protocol: the solid body's p99 fell to 30 % to 40 % of main's, but A2 stayed 12 to 20 times B0's, so the cost screen still failed (#678).
- Re-ran the label-free D2 comparison on the 23 tuning and screen sites: six of seven measures held, and the OBB centre's contested terminations fell so that it went from worse at 15 sites to better at 16, matching kirk0's identity result (#678).
- Profiled A2 after span selection: the span search was under half of its update, and the near-edge measurement in association sorted every projection to read one percentile (#678).
- Selected that near-edge percentile instead of sorting: outputs stayed byte-identical, the shadow's p99 fell 10 % and A2's 8 % to 28 %, and A2 stayed 8 to 19 times B0's (#678).
- Rewrote the devlog to one line per bullet, one heading per UTC date and a PR number on every landed bullet, and gap-filled September 25 to October 5 (#677).
- Recorded what sprints 0.5.2.0 and 0.5.2.1 delivered in the backlog and plans, and made the devlog-update skill insert new days after the devlog's introduction (#677).

## October 4, 2026 - Physical references and facet authoring, publishing preflight, and two more screen sites

- Landed physical-reference authoring in the macOS Annotation window: body and per-frame pose editing, review state, a Compare mode over comparison reports, and raw intensity inspection, saved through one Go editing service and HTTP API (#656).
- Pinned exact physical-reference revisions in version 3 annotation splits, so scoring refuses stale or mismatched evidence, and added reproducible verification bundles checked by `lidar-ground-truth-eval verify-bundle`. The September 30 entry records the same PR's feature-proposal foundation (#656).
- Compared the shared Swift fixtures' floats in Go within 1e-9 relative (`testutil.JSONWithin`) instead of byte for byte. Go fuses multiply-adds on arm64 but not on amd64, so fixtures written on a Mac differed from Linux CI in the last digit (#656).
- Landed facet authoring: an operator marks one to four persistent vehicle parts as exact point subsets of an object, pins them to a body, and registers them across frames without moving the body origin (#673).
- Registered straight facets with a line constraint (`FeatureLineConstraint`) instead of an invented tangent anchor, kept tracker assistance visible through review, and let a split freeze optional facet proposals with their exact retained revisions (#673).
- Added offline body-pose proposals from confirmed facet correspondences (`POST /api/annotations/features/pose-proposal`), inspectable in the macOS window. A pose prior stays an explicit experiment input, never a target reference (#673).
- Fixed 14 of the 15 findings from a review of #673, one commit each with tests. The fifteenth, a body registration made against a different physical revision, became a divergence that the split's facet pin records rather than a refusal (#673).
- Kept references and facets out of the live tracker. No operator pilot or real-capture measurement has run, so nothing yet shows better boxes, trails, heading, or following metrics (#656, #673).
- Made the `scene-assets` preflight refuse a `./velocity` whose git sha differs from `HEAD`, because a published asset's provenance cannot be corrected afterwards (#666).
- Made `publish-scenes.py` follow `DB_PATH` and check the run records are readable before the first replay. A server and publisher reading different databases had looked like a clean batch until every export failed (#666).
- Added a publishing-run checklist to the agent guide: pull and build, check the version stamp, use one database, keep machine paths in `local.mk`, and pass `make scene-assets-status` first. A failed export reuses the completed VRLOG rather than replaying (#666).
- Cleared `go vet`'s two "json tag on unexported field" findings in tests and made `make lint-go` run `go vet ./...`, so CI's Lint job now fails on vet findings. The server test gained its missing check that `"skip"` does not resolve (#674).
- Landed the capture-schema guardrails in migration 000058 and the terminology-first programme plans recorded on October 1, after addressing the schema review comments (#631).
- Carried two rules from the retired schema v2 freeze plan into the vocabulary programme: a new or rebuilt foreign key declares its `ON DELETE` action and has a delete-path test. Deleting a clip cascaded to its reviewed labels, which no recording can rebuild (#671).
- Recovered the VRLOG recording contract review and async tracking plan revision written on September 22, left uncommitted in a Codex worktree. The review settles one processed-observation recording, catalogue-held results and annotations, and web scenes as derived exports (#670).
- F6s's 1st-mission and embarcadero-bryant runs are in. The tracked arm is lower than the control on all 19 screen sites run so far (a median of 76 mm lower steady p99), and its confirmed tracks and births per confirmation stay within gate 3's 5 % on every one. F7b's last screen site, howard-6th, completes T5's screen. Against the tracked arm on the 19 sites both have run, T5 is lower on 12, higher on 5 and identical on 2, a median of 7 mm lower, so it stays off. The September 30 count of 7 higher of 17 had included the two identical sites. 3rd-folsom, howard-6th and the coverage survey remain for F6s (#668).
- Took `results/s2-a1/`, `results/s2-f6s/` and `results/s2-f7b/` out of main. #652 to #654 had merged them, against the rule that raw experiment results live on their own pushed branch and main keeps the accounting. The plan names the three results branches, which hold every file that was here (#669).

## October 2, 2026 - Street scenes become street surveys

- Rebuilt the point-cloud assets for all 23 S2 corpus scenes from one build of main (`c962e323`), replayed at 0.5x through the live pipeline, so every published asset is stamped with, and reproducible from, that build (#664).
- Recovered from two failed batches: the first failed every scene with `unable to open database file`, and the second ran out of disk after 18. Reran the last five after freeing space (#664).
- Moved the published pages from `/scenes/` to `/surveys/` as "Street surveys", with a redirect at every old URL. The source directory kept its internal name, so the exporter still writes where it did (#665).
- Regenerated the 25 survey pages, the site index, and the map against the rebuilt assets, and updated the `tools/s2-hilbert` generators, reader, player, and speed-stats wording for `/surveys/<id>/` URLs (#665).

## October 1, 2026 - Terminology first, the F1 findings recovered, near-face scoring, and annotation history pruning

- Split the platform vocabulary programme into four terminology/compatibility work packages and seven feature/data-model work packages, with linked delivery outlines and a Phase 1 exit that preserves behaviour, values and relationships. The original item ledgers and decisions remain; their release allocations need re-baselining after the terminology scope is sized. Capture digests, clock metrics, shared jobs, radar identity/logging, recording conversion and site/survey consolidation follow in Phase 2 (#631).
- Rebased the #631 schema-review foundation onto main. Migration 000057 already holds track-estimate reference/support fields, so the capture-job and motion-period checks move to 000058 with their own tests and updated references. D-27 and those guardrails do not claim completion of the terminology rollout; the four Phase 1 work packages remain to be implemented (#631).
- Recovered the unpublished F1 write-up from #641: consider-on-entry (T2) reduced body-centre p99 by 4 % to 13 % alone, did not improve face-stable p99, and took 31 % more replay time. The record keeps the original build and results branch, and explains why later arms did not carry T2. Main already records F5 and the decision to stop T4; this recovery adds no tracking option. Corrected the face-leaving comment in the transition summary and used `\ast` in the geometry formula so the pinned Markdown formatter preserves it (#663).
- A reviewed kirk0 pack (20 road-user objects, 3,391 scored masks) gives part of what a physical reference would: where the sensor sees a face is where the face is, whatever the far side of the body does. `lidar-near-face-eval` puts a mask's returns in an arm's believed body axes and reports the normal residual of the end and side faces the sensor can see (positive: the believed face is out toward the sensor beyond the returns) and, where the end face's span covers 0.7 to 1.5 of the believed width, the tangent offset of the centre, the error a rank-one fix leaves unconstrained. A class-prior extent is kept in its own stratum, and every unscored instant is counted by reason (#659).
- `lidar-annotation-split-draft` drafts the version 1 split manifest for the window: every reviewed road user that would survive freezing, the rest listed with the reason. `-from-evidence` starts the window where a replay's solid-body rows start, which with `-warmup 20` is 6.2 s into kirk0, not 20 s, so about 3,100 of the 3,400 masks and 19 of the 20 objects fall inside it (#659).
- Plumbing was checked on real replays of kirk0, control and tracked: a pack built from the control arm's own faces scores the control arm at zero. That is not a result. The first run on the reviewed pack is open, and kirk0 is the capture the shadow work was developed on, so it is a tuning split and cannot be held out (#659).
- A labelled kirk0 pack (23 objects, 3,683 reviewed masks, 83 s) had grown a revision history of 1,968 snapshots and 37.9 GB in a day and a half. Both stores write the whole document pretty-printed, one point index to a line, about 25 MB at the end, and archive the exact previous bytes on every save, so history is saves times document size and the window never reads it back. `lidar-annotation-prune` plans a thinning (newest N, the oldest, one per interval, and any revision a frozen split pins), backs the removed revisions up into a verified compressed tar, and only then removes them under the writer lock, so the Annotation window can stay open (#658).
- Planned faster Annotation window saves, which froze the app for seconds per frame: write-behind batching, a writer actor off the main thread, a private recovery journal for durability, cheaper saves in the unchanged format, and thinning at archive time (#660).
- Bumped devalue to 5.9.4 in `web` (#662) and the documentation group (#661): three, markdown-it-anchor 10, and postcss for the public site, and KaTeX, markdown-it-anchor 10, and Mermaid 12 for the docs site.

## September 30, 2026 - The tracked arm is steadier, and feature proposals keep their own evidence

- Added a local Swift Feature Candidates mode over saved whole-object membership. A sphere seeds an overlapping feature subset, retained under a persistent ID with fresh support at each frame. The local Go service validates and revisions the shared recording-domain protobuf; feature saves leave membership and physical references alone. Accepted proposals are not independent truth (#656).
- Added one-frame translation previews with explicit accept/edit/reject/cancel decisions. The existing object matcher needed a feature-sized rival threshold: its one-metre default would otherwise hide competing small fragments. Rotation registration, body/part anchors and semantic recognition remain follow-ons, and the operator pilot on the three sites is still outstanding (#656).
- Separated measured intensity zero from the adjustable 1–255 gradient and the unavailable style. Updated the Swift, recording-domain and facet plans to distinguish the bounded authoring work from the 35–60 engineer-day conditional research programme. Desktop tests are not field evidence (#656).
- Planned the facet registration experiment: matched evaluation arms, reference requirements, stop/go criteria, and effort ranges, to test whether persistent local vehicle surfaces improve tracking and following enough to justify integration. Kept headway delivery independent of it (#657).
- Ran F6, the tracked arm (S2a) and the T1 with T3 control on the tuning partition on the Mac. The tracked arm lowers body-centre p99 by 26 mm on marina-webster-beach (0.158 to 0.132 m) and 20 mm on columbus-broadway (0.280 to 0.260 m), and the gap stays at 2.3 and 2.0. The five-point residual favours any filter, and on columbus the tracked series is still 1.15 times the per-frame point estimate's p99. Translation buys nothing. Removing the steady-run windows in which a lateral face enters lowers the tracked p99 more than the control's (0.120 to 0.071 m on marina, 0.260 to 0.153 m on columbus), which is what rank-one drift predicts: the filter carries the unconstrained direction on its prediction, and the side face corrects it in one step. T5, the medoid across that direction at every rank-one fix, is next (F7), and F6s screens the tracked arm on the 21 screen sites (#648).
- Built T5, the rank-one medoid, default-off (`solid_body_rank_one_medoid` and a tight setting): a fix by a front or rear face alone also takes the medoid across the body, with R plus the believed half-width squared, so the direction no face measures is held by evidence rather than the prediction. It stays off beside any found side face, where the medoid is most biased, and at side-face fixes, where the medoid's slide along the body would reach the speed. On a synthetic vehicle seen from behind changing lane by 2 m, the worst tracked lateral error falls from 0.534 to 0.247 m. F7 runs it on the tuning partition (#649).
- Built S2.3's ablation arm, A1 (`near_edge_track_a1`): the tracked arm with its body-centre tracks gated on the medoid, as a medoid-referenced track is, instead of A2's face residual. It is the do-nothing option that shows what A2 buys in identity, and it runs once on the tuning partition (#650).
- The Mac runs are in. The tracked arm is lower than the control on all 17 screen sites run so far (a median of 87 mm lower steady p99), and its confirmed tracks and births per confirmation stay within gate 3's 5 % on every one. A2 has 56 % and 46 % fewer lapses than A1, which gates on the medoid, on the tuning pair. T5 lowers the tuning pair's steady p99 by 11 and 14 mm but is better on 10 and worse on 7 of 17 screen sites, so it stays off: the rank-one drift is a small part of the lateral-entry tail. Next is the held-out score of the tracked arm without T5 (#655).
- Merged the A1, F6s, and F7b result sets into `results/` on main (#652, #653, #654). #669 took them out again on October 4, because raw results do not belong on main.
- Bumped undici to 7.30.0 (#642, #645), markdown-it to 15.0.2 (#643, #644), and brace-expansion to 1.1.21 (#646, #647) in the docs and public sites, and dompurify to 3.4.16 in the docs site (#651).

## September 29, 2026 - The near-edge update reaches the tracked filter, physical references, a frozen split, and one vocabulary

- Replaced the LiDAR data-model simplification plan with the [platform vocabulary and data model plan](plans/platform-vocabulary-and-data-model-plan.md), widened to both sensors. "Scene" had meant five things, "capture" and "source" seven each, and "site" thirteen identities (#630).
- Settled decisions V1 to V36 one noun at a time, among them track and tracker for both sensors (retiring transit), evidence to estimate to track, survey for the published scene, clip for the evaluation window, and one sites table (#630).
- Ledgered every surface against its target name and release, from 44 tables down to 39 through routes, protos, web, macOS, CLI, and on-disk formats. Eighteen items split into a forward-compatible v0.5.8 to v0.5.11 group and a data-moving v0.6.x group (#630).
- Added the [survey capture export plan](plans/platform-survey-capture-export-plan.md): parted pcapng on a 300 s capture-time grid for Hugging Face, with route parts published only by opt-in and both ends trimmed to a 500 m privacy radius (#630).
- Tidied the v0.5.2 backlog, added missing PR numbers to completed items, and repaired related links in three operations docs that had broken the docs build (#632).
- Added the geometry-coherent tracking proposal to `.prettierignore`. Prettier 3.9.9 read the asterisks in its inline maths as emphasis, corrupted the formula, and made the `format-docs` hook refuse every commit that staged Markdown (#639).
- Added `make check-docs-format`, run by `lint-docs` and a new blocking `md-format` CI job with the pinned prettier, so unformatted Markdown, or a prettier bump that rewrites a file, now fails CI rather than only the local hook (#639).
- Bumped the application dependency group, among them fonttools, ruff, three, and prettier 3.9.9, the release that broke the docs hook (#638).
- Ran T4, a per-face half-extent state on the solid body, on the tuning partition on the Mac (F5). It lowers the body-centre lateral p99 by 5 to 6 mm (0.158 to 0.152 m on marina-webster-beach, 0.280 to 0.275 m on columbus-broadway) and leaves the gap at 2.3 and 2.0 against S2.1's 1.25, so it is not kept. The steady-run anatomy, which moves to main, names lateral face entries as the tail: without them the steady p99 falls to 0.101 and 0.180 m. They mostly arrive while another face is fixed, which points at rank-one drift rather than a re-reference. F6 runs `near_edge_track` on the tuning partition next (#640).
- Froze the reviewed split: `velocity lidar annotation-split freeze` certifies membership review, pins each pack, its manifest, selection record and annotation revision by digest, and binds every corpus case to its captures by SHA-256, so a pack cut from a held-out capture cannot be tuned on under another name. A revision carries the whole history of what was tuned on, by capture span as well as by pack, so no later revision can hold out a car that an earlier one tuned on, whether the pack was dropped and re-added or re-cut wider. The evaluators given `-split-manifest` hold each case to its role, check the replayed capture against the case, and record the split; the digest is unkeyed, so it catches accidental edits, not deliberate ones (#636).
- Measured sensor coverage instead of declaring it by hand: `-survey-coverage` replays a case's default configuration and declares its range and azimuth sector from the online estimates. On kirk0 it reproduces the hand-declared 92 m range and finds no estimate in the 155° behind the sensor, which the hand declaration had assumed was covered (#636).
- Gave a pack an independent physical reference beside its annotation sidecar: per-object body dimensions and keyframes with explicit anchors, position and yaw bounds, an axis that may be front-rear ambiguous, bumper evidence and following gaps, each component with its evidence status and its own review, and an origin ledger that stops a tracker-assisted record from ever becoming independent. An `observed` claim is held to the mask's returns and keyframes, and a later membership edit marks the records it invalidates as stale. `velocity lidar annotation-reference import|validate` imports, or dry-runs an import, through the same checks (#635).
- Scored estimate versions against those references beside the mask-position comparison: body centre, yaw, extents, bumpers, unsigned ends, box overlap and following gap, each only where a reviewed independent reference has it and with its bound beside the error. A medoid or visible-box point is never promoted to a body centre, an ambiguous axis never gives a signed bumper error, and every expected instant is either scored or counted by reason. `lidar-ground-truth-eval perframe -physical-reference` adds the section and refuses a held-out split until error limits are pinned on tuning data (#637).
- Built S2.2 of the near-edge plan behind `near_edge_track`, default off. The solid body's state machine is one function over a state, covariance, reference and support value, which the shadow runs on its own filter and the new mode runs on the tracked one: a fix replaces the medoid update with the face updates, a faceless frame leaves the prediction alone, a lapse returns to the medoid, and a body-centre track is associated on the pair's face residual (A2), keeping the gate at two degrees of freedom. A re-reference is a translation before it is an update, so velocity is not kicked by half a body. On kirk0 the default replay and the shadow arms are byte-identical to main, the new arm runs identically twice, and its body-centre lateral p99 is 0.130 m against the default point estimates' 0.309 m, label-free and on one capture (#633).
- Added `reference_point` and `support_instant` to `lidar_track_estimates` in migration 000057, so every persisted estimate states its reference and support and no reader infers them from `measurement_source` (#634).
- Backfilled old rows with the adapter's existing mapping and added a trigger that refuses a new row leaving either column empty. Under the near-edge tracked state a medoid can feed a body-centre filter, so the old inference would fail (#634).

## September 28, 2026 - Segment finder and selectors land, the solid body follows its course, and physical reference tooling is scheduled

- Took the solid body's face axis from its own course instead of the lagging tracked heading while it moves. On columbus-broadway the turning tail goes: at 15 degrees per second or more the body's face-stable p99 falls from 0.369 to 0.143 m, below the point estimate's 0.193 m. Fixes rise 17 % to 26 % because spans along the course let widths converge. With hysteresis as well, the body's all-frame p99 falls 26 % to 33 % on both tuning sites, but stays 2.1 to 2.4 times the face-stable p99, so the transition tail is the half-extent error a new face brings (#624).
- Built the segment finder that the review queue script planned: Go finders for following, leader changes, lateral jumps, split flags, exposure and capture-random windows, `velocity lidar segments` and `velocity lidar annotation-clip`, a clip job on the capture queue, proposal layers and dismissals that survive a restart in the macOS tool, and a Segments page. A held-out window may only be chosen by traffic or at random, and a capture's random window comes first (#622).
- Found by replaying kirk0 through a scratch server that the run finders ranked nothing for any current run. They read `lidar_track_observations`, which an analysis replay does not write; in the development database all 67 runs completed since August store none. A run with no stored observations is now read from its VRLOG recording, which holds the same tracker state: 83 s of capture reads in 0.13 s and ranks 6 following windows (#622).
- Fixed what the review of #622 found: a case made without naming a finder stored `finder = ''` and could never be clipped; a window's peak time depended on map order; a failed or cancelled clip left its recording on disk; the Segments page did not redraw a row after making its case; and the SQL formatter had renamed the shipped column `lag` to `LAG` (#622).
- Reviewed the SQLite model behind the segment finder and answered the six findings that concern its own tables with migration 000055. A selection's role, finder, capture and window are now generated from its window document, so a column cannot say one thing and the document another; the finder catalogue and the held-out rule are `CHECK` constraints, with a Go test that fails when they and `segments.Allowed` disagree; and the run is a foreign key that is cleared, not cascaded, when the run is deleted (#626).
- Made a clip job and its link to a segment one write, so a worker can no longer claim a job that does not yet say what it cuts, and gave the pack a relative path and a digest. A retried job adopts a pack that an earlier attempt finished. Ran the migration on a schema-54 database made by replaying kirk0: the selection carried over, nothing was set aside, and the server re-linked the pack when it started (#626).
- Measured the deferred findings in the development database before deciding on them. No replay case disagrees with its first ordered file and no probed capture has contradictory bounds, but 24 of 31 replay cases name a source period that no longer exists, so session keys wait on a decision about capture identity. `MATRIX.md` now lists all 44 tables and 601 fields, read from SQLite itself and not from a regular expression over `schema.sql` (#626).
- Moved the choice of annotation windows into a versioned config file, `config/segment-selectors.defaults.json`: each selector names a finder, its parameters, one measure to rank by and the bounds a window must meet, with a label and a category for the Segments page. The six finders become standard selectors that return exactly what they did, pinned by a golden test over every window, order and score; `close_following` is the first new criterion. Held-out windows are still chosen only by the standard following, exposure and random selectors, now at their own parameters: a `min_gap` of 0 would have turned following into a split detector (#627).
- Migration 000056 records the selector that chose each window, as it ran, held to the window's finder and the held-out rule by a `CHECK`, and the clip job writes it into the pack's `segment.json`. An attempt that holds a person's annotations is no longer removed as unfinished when its segment record cannot be read (#627).
- Extended the TrueNAS storage runbook: moved the worker VM's `/home` onto its near-empty `/srv` partition without downtime, and fixed a `claude` CPU spin by switching the VM from a model-less custom CPU mode, which strips SSE4.2 and POPCNT, to host passthrough (#620).
- Recovered from an `e1000e` TX ring hang on the host's `eno1` that also orphaned the VM's macvtap NIC, then disabled EEE and TSO and added a systemd watchdog that recovers in seconds. Bridging `eno1` failed, probably on switch-side BPDU Guard (#620).
- Added the physical reference authoring/scoring dependency to Sprint 0.5.2.0. The Annotation window reviews point membership, class, and identity; optional pose storage does not provide a box editor, and the current reference builder uses visible-mask positions. Planned independent keyframe pose/extent references, component bounds and unknown geometry, separate review, and evaluator/inspector comparisons before the operator geometry and following-pair pass (#625).
- Aligned the annotation guide, dataset plan, state-estimation priorities, and MVP sprint plan with that boundary. Mask review can begin now and provisional integration can continue; completing the mask queue does not close the physical gates. This update changes documentation only and retains the existing acceptance thresholds (#625).
- Scored the solid body's best remedy, hysteresis with course-aligned faces, once on the held-out case, embarcadero-folsom. It does not reach gate 2: over body-centre frames its lateral p99 is 0.229 m against the point estimate's 0.204 m, where the gate wants half. Within runs on the same faces it is 0.118 m against 0.181 m, and it beats the point estimate in every heading-rate bin, so the turning fix holds on data it was not tuned on. Even that face-stable p99 is above gate 2's 0.102 m. Chose a per-face bias state (T4) as the next remedy, ahead of feeding the tracker (#629).
- {claude/s2-1-config-engine-docs} Drafted a tuning-config surface for the solid body: one `l5.cv_kf_v1.solid_body` block, nil by default so the fingerprint stands, with experiments as aliases and live parity or refusal. The branch also recorded F1 and F3 in the near-edge plan.

## September 27, 2026 - Solid bodies, adaptive noise, identity options and the provisional headway field run

- Consolidated the four 0.5.2.x slices left unfinished when the parallel agents stopped into one branch on main, bumped the version to 0.5.1-pre38, and re-ran the Go suite, the kirk0 pcap replays and the race build over the result (#614).
- Wired the Section 5.4 solid body: a default-off near-edge shadow estimator that never feeds back into association, course-aligned extent admission, a faceless body-centre lapse to the medoid, and one `lidar_track_solid_bodies` row per point estimate (migration 000052). On kirk0 its body-centre lateral residual is lower than the point estimate's at p95 and higher at p99 and max; with eight moving tracks and no held-out geometry that is wiring evidence, not G-GEO-1 (#614).
- Landed the Phase 3 adaptive-uncertainty harness behind `-experiment adaptive_uncertainty`, with a per-stratum calibration fit and G-UNC-1 predeclared. On kirk0 the fit lowers R towards a smaller scalar rather than separating the axes and row 2 fails for every arm, which points at the medoid's error shape, the thing Phase 2 changes (#614).
- Added the default-off S2 and K4 identity options (`TentativePriority`, `ClassIdentity`, `ContestedRejoin`) with their missing tests, including a scripted K4 scene where a car emerging beside a kerbside pedestrian no longer steals the pedestrian's track (IDF1 0.590 to 0.966) (#614).
- Ran the headway report over persisted estimates for the first time (`velocity report headway --db`, contract `headway_report_v2`). It finds following pairs on kirk0 and publishes nothing, because a persisted point estimate has no class, heading or extent; the solid-body rows now carry all three and are the next input to read (#614).
- Repaired the kirk0 occlusion-continuity smoke run, which #611's coverage refusal broke outside CI's race-only pcap job: it now replays only the coverage-free options and asserts the others are refused (#614).
- Added a review queue for choosing what to annotate: `scripts/lidar-following-windows.py` ranks windows by vehicle following and leader changes and places each in its capture file. On kirk0 almost all the following, and a lead car split into three tracks, sit in one nine-second window. Planned the segment finder, one-step clip and persisted proposals around it, with held-out windows chosen by traffic rather than tracker failure (#615).
- Gave the continuity replay a way to satisfy #611's coverage refusal: a declared, content-addressed sensor coverage that must name its source and bound its range. The kirk0 run now exercises the qualified arms under a measured 92 m envelope, splitting 1266 coasted instants into 567 occluded, 615 missed and 84 out of view (#617).
- Added a static-occluder scene, in which the tracker must admit it does not know why a car behind a wall vanished, and fixed the VRLOG shed test, whose 20 ms drain assumption failed under slow storage sync rather than CPU load (#617).
- Stopped calling a tracked position the body centre. The medoid and the centre of the visible box now say what they are, `cluster_medoid` and `visible_obb_centre`, at every stage, so a final-stage OBB row can no longer project a bumper; support tokens and acquisition times survive the adapter, and migration 000053 keeps the support token on each solid-body row (#618).
- Taught the headway field run to read the persisted solid bodies (`--solid-bodies`). On kirk0 point estimates now fit no follower path at all, and the solid bodies fit 7 of 60 and find 3 encounters, none valid yet: only 3 tracks end classed as rigid vehicles. The first real run also found that the shadow filter's covariance is asymmetric by float32 round-off, as the tracked filter's is (#618).
- Planned S2, the near-edge measurement as the tracker's own, and gave corpus runs a per-case solid-body summary so the plan can be tested site by site. On kirk0 the summary locates the shadow's p99 tail: within runs on the same faces the body's lateral p99 is 0.083 m against the point estimate's 0.178 m, while over all body-centre frames it is 0.364 m against 0.309 m, so face transitions come before feeding the tracker (#619).
- Tried two remedies for the solid body's face-transition tail, and measured faces from full cluster members. Admission hysteresis (a face must be usable on two consecutive frames) cuts the all-frame p99 by 11 % to 41 %, but leaves it about 2.5 times the face-stable p99 on kirk0, marina-webster-beach and columbus-broadway, against a target of 1.25; a consider term on a face's entry changes nothing. Columbus's remaining tail is turning: at 15 degrees per second or more the body's p99 is 0.369 m against the point estimate's 0.152 m, because the tracked heading that picks and orients a face lags the turn. Full members give exactly the fixes of the 256-point sample, and the solid body now refuses a near-edge fix without a declared sensor origin (#621).
- Ran S2's first corpus test on the Mac: the shadow at 256 and 1,024 sample points on marina-webster-beach, columbus-broadway and embarcadero-folsom. The larger sample changes no fix, fallback or run count, which the face-support rule predicts: a 256-point sample always puts 14 returns on the face, so only small clusters fail. Face changes carry the tail on every site. Over all body-centre frames the body's p99 is 19 % to 53 % above the point estimate's; within runs on the same faces it is lower on marina and embarcadero, but not on columbus-broadway, which has a second tail to explain (#619).
- Brought the Phase 0/1 corpus back in line with #613, which re-split franklin-mcallister into five captures while the corpus still declared three, so the corpus tool refused the case (#619).
- Added the 0.5.2 tailgating MVP sprint plan, fixing scope, acceptance criteria and delivery order around physical trajectory and following-distance measurement, and aligned the behaviour analytics, clock abstraction and state-estimation plans with it (#612).
- Hardened two wall-clock-sensitive VRLOG 1.1 tests that failed under a loaded machine: the writer behaved correctly, but the tests assumed a drain time and commit-stall bound that slow storage does not guarantee (#616).

## September 26, 2026 - LiDAR clock and sequence-wrap fixes, and the state-estimation MVP review

- Reviewed merged #596–609 and #611 against their descriptions and current call sites, keeping implementation, local evidence and physical acceptance separate: [review](lidar/operations/0.5.2-sprint-review.md). This pass changes documentation only (#612).
- Recorded the body adapter's medoid-as-centre defect, loss of detailed support, incomplete final body persistence, coverage-dependent replay/test mismatch, omitted scene-boundary encounters, and the distinction between pair totals and unique follower opportunity. Added missing reference-completeness checks to the promotion plan (#612).
- Replayed the existing kirk0 capture on #611 with the opt-in RTS comparison. Every horizon still fails the revision bound: per-frame maximum revision p99 is 0.629 m against <0.3 m; final Cartesian rows exist, but the physical body and field gates remain open (#612).
- Recorded passing focused estimator, metric, storage, parser, report and pipeline checks alongside the real failures: the continuity PCAP test now hits #611's coverage refusal, and the VRLOG shed test failed all ten isolated repetitions. Socket sandbox restrictions and the unhydrated 20 Hz capture are separate validation limits (#612).
- Planned the remaining 0.5.2 sprint around a provisional recorded-scene MVP and a separate qualified field exit. The new Following distance layer sits beside velocity vectors and exposes endpoints, uncertainty, speed, support, candidate decisions and suppression from the same version as the report; general trail embellishments remain later work (#612).
- Corrected stale E1.2/E1.4 status, scene-containment semantics and sensor-clock/continuity instructions without rewriting the dated experiment or earlier DEVLOG records. Pi/power-loss and independent live-worker work remain tracked, with an offline Mac integration path first (#612).
- Fixed `internal/lidar/l1packets/parse` so `TimestampModePTP`, `TimestampModeGPS`, and `TimestampModeInternal` now use the sensor's combined UTC timestamp instead of a boot-time offset. Packet time no longer steps backwards at each second boundary, and the PTP fallback returns to sensor time once timestamps advance again (#611).
- Fixed `l2frames.calculateFrameCompleteness` to rebuild the shortest circular interval across wrapped UDP sequence numbers, so a frame spanning `math.MaxUint32` no longer scans almost the whole sequence space or loops forever. Added wrap-specific tests for both the no-gap and missing-packet cases (#611).
- Closed the medium follow-ups from the PR-batch review: scene headway now requires full capture-window containment, replay-eval refuses absence-explaining occlusion-continuity experiments until explicit sensor coverage exists, `internal/db` applies connection-scoped SQLite PRAGMAs through the DSN on every pooled connection, and live observation capture drains queued frame callbacks before replay or tuning boundaries close the container (#611).

## September 25, 2026 - Plan hygiene and the worker's storage path

- Cleared thirteen plan-hygiene gate violations and graduated two Complete plans, `lidar-pipeline-state-model-plan.md` and `lidar-visualiser-stream-robustness-plan.md`, to symlinks onto their canonical hub (#594).
- Diagnosed why the job runner's worker VM (`bansheeworker`, introduced in #586) could not reach the TrueNAS pool over a local path: its macvtap NIC isolates it from its own host, and every bridge fix stripped the host's own address because that interface had no row in TrueNAS's network database (#595).
- Registered the host interface on its own first, then created the isolated bridge with zero outage, attached the VM's second NIC, and repointed its NFS mounts off a dead Tailscale address onto the new local path. Verified end to end, including across a guest reboot, at 603 MB/s (#595).
- Landed the headway contract stack (#596, #602, #606, #607, and #608): `internal/lidar/l8behaviour/` now defines the vocabulary, path pairing, persistence, report contract, scene API, and dashboard SVG for following metrics. Real scenes still show no interactions until the pipeline writes the persisted headway rows.
- Landed the capture-time and continuity stack (#597, #600, #603, and #605): tracker prediction, coast age, expiry, smoothing, and assignment now use capture-time-safe rules, exact rectangular assignment, default-off occlusion continuity, and opt-in fixed-lag RTS persistence. The smoother failed G-SMO-1 on kirk0, so every new stage stayed provisional.
- Landed the held-out per-frame evaluation harness (#598): reviewed references, split manifests, paired-arm comparison, IDF1, and the evaluator-side exact-assignment wrapper now pin how reassociation changes will be scored. The final acceptance run still depends on frozen reviewed splits from the annotation work.
- Landed the foreground-complete observation log (#599, #604, and #609): lineage-preserving L4 frame records, the VRLOG 1.1 commit chain, recovery, and opt-in live capture through `--lidar-observation-dir`. The live writer remained experimental pending Pi and power-loss evidence.
- Stopped `TestHesaiLiDAR_PCAPIntegration` holding every decoded frame in memory (#601), cutting the race-run peak from about 13.6 GB to under 1 GB so the LiDAR CI job no longer died by hosted-runner OOM.
- Landed state estimation: reproducible evidence, the measurement baseline, and the heading fixes that could be validated without it, with headway put first in the plans (#559).
- Regenerated the web VRLOG scenes with the current build and tuning hashes (#610).

## September 24, 2026 - State estimation: the OBB centre stays opt-in, and the branch against main

- A/B tested D2's OBB-centre measurement against the medoid on kirk0's annotated road users (30 objects, 771 frames). Recall was level, but the OBB centre switched identity 146 times against 92 and fragmented more, so the medoid stays the production measurement and `obb_centre_v1` becomes opt-in (21.1 D5) (#559).
- Fixed the label-free scorecard reading every cluster at its OBB centre whatever position the run tracked, which skewed how it classified track endings for medoid runs (#559).
- Replayed Columbus Broadway through the live server at 0.25x, once with the branch and once with main: heading acceptance rose from 0.605 to 0.789, median course alignment fell from 51.9° to 24.0°, and tracks, lifetimes and speed percentiles stayed where main has them: [record](lidar/operations/state-estimation-columbus-0p25x-vs-main.md) (#559).
- Found that the branch's server panicked when started outside the repository tree, as on a Pi, because `MustLoadDefaultConfig` looked for the tuning file on disk. Main's #593 embedded fallback resolves it on merge (#559).
- Moved the 2026-09 campaign's drivers, state and results out of the repository to a backup branch and the LiDAR volume, and recorded pass 7, whose findings existed only in commit messages. Its ground-truth scores are marked unreliable: they matched first-sighting rows (#559).
- Realigned the branch's plans with the headway-first backlog, retired the schema v2 freeze plan, the branch audit and the D2 readiness review, and cleared the CI failures that had skipped the Go tests since September 17 (#559).

## September 23, 2026 - Route capture, the job runner, and a backlog of design plans land

- Landed a route-capture design for two new moving-sensor rigs, a cargo bike and a backpack pedestrian: four capture regimes, a road model of intersections and segments, distance-band speed aggregates, and a portable-timing appendix establishing that today's point timestamps are not validated acquisition times (#580). Added the LiDAR hardware market-watch page with its selection rule, budget rule, shortlist, and first dated snapshot.
- Landed the offline placement design for a stationary capture: five stages from a rounded GPS fix through OSM and S2 candidate discovery, coarse pose from street and façade geometry, point-to-plane ICP against USGS 3DEP surfaces, to an accept-or-fail decision that reports an unresolved hypothesis rather than guessing at it (#588).
- Landed the vehicle-encyclopedia ideation, five documents proposing a Canadian vehicle mass and dimension catalogue, a distance-coarsened classification taxonomy that leaves the shipped seven-class vocabulary untouched, and a maths proposal framing vehicle naming as retrieval rather than classification, predicting a resolution floor set by the catalogue rather than the sensor (#587).
- Landed the decoupled-observation design, separating immutable L4 observations from revisable L5-and-later track estimates, with bounded look-ahead, revisions, finality, and provenance defined as a proposal with no runtime change (#582).
- Landed the shared VRLOG storage design that unifies capture, annotation-pack, and evaluation-evidence data on one binary and JSON model (#591), and rewrote the LiDAR market-watch page's dating convention: one evolving file rather than one per snapshot, with a "Last surveyed" pointer into its own log, since git history already keeps every earlier reading.
- Landed the `velocity worker` and `velocity jobs` runner (#586): captures are verified by a cached content hash before a job runs, attempt state lives as a directory tree that marks in-flight work lost on restart, and `state_estimation_baseline` and `track_scorecard` run from a `git worktree` staged at the job's own commit. Validated end to end against kirk0 on a live worker.
- Fixed `lidar_run_tracks` writing a track's first-sighting snapshot instead of its final state: measurement columns now flush from the newest values on a ten-second interval, on completion, and on failure, while the update statement never touches label columns, so a label written mid-run survives (#584). Flagged that ground-truth scores and per-track statistics computed from rows written before this change are suspect.
- Landed two rounds of annotation-tool fixes from live labelling sessions: re-proposing no longer discards the in-progress list, objects can be merged back together, the four elevation views each show their own 180-degree half instead of a mirrored overlap, and the column lattice and views turn with the site's recorded grid azimuth (#589); a guard blocking "new object from selection" on unsaved membership was removed because the split already preserves that selection, the default brush is now the sphere rather than the lasso, and the default view frames the foreground extent rather than the whole scan (#592).
- Found two real bugs running #586 against actual hardware: a bare sha256 digest produced an unhelpful validation error, and the worker panicked when started from outside the repository tree because its config lookup only tried relative paths (#593).
- Fixed a third bug in the same follow-up, caught by Copilot's review: the panic fix's own fallback silently blamed the wrong cause when the embedded config failed to parse. Re-validated all three fixes against a second, unrelated capture and a mid-job kill-and-restart (#593).
- Designed portable LiDAR and IMU acquisition timing, with a timing-qualification procedure for the backpack motion experiment and the current limits of timestamp handling recorded (#590).

## September 22, 2026 - Annotation lands, scored against the full corpus

- Landed the point-annotation toolset tracked here since September 7 (#579): immutable point packs, revision-safe sidecars, and the macOS labelling tool. Scored against the whole of kirk0 (832 frames, 61 objects, 4,606 masks), the shipped tracker defaults find 56% of labelled road-user object-frames and 20% of pedestrians, the first score of tracker output against independent, human-labelled ground truth.
- Landed four net-new LiDAR plans gathered from the branches that would implement them (#585): background-region overlays, an S2-anchored 0.5 m occupancy column grid that three separate jobs each wanted, a single review workflow from proposal to graded reference truth, and static-scene local geometry fitting.
- Fixed a leaked file handle in `runBenchmark`: `openClusterDump` had no matching close on any exit path (#583).
- Bumped the "application" dependency group across Go, Python, and web (#581).

## September 21, 2026 - Annotation: the unit of work becomes the object

- Measured the job on a real 200-frame pack before building more of the tool: about 43 moving clusters a frame, 8,600 object-frames in twenty seconds, and a median of six seconds to label one by hand. The 1,954 larger object-frames alone are three to six hours, so labelling frame by frame cannot reach minutes of ground truth (#579).
- Found that fifty minutes of labelling had produced nothing that counted. Reference truth is a reviewed mask of a reviewed object, "Mark reviewed" marked only the object, and no control could review a mask. Review now reaches the masks, per frame and per object, and a reviewed frame whose points change returns to proposed (#579).
- Fixed creating an object wiping the selection it was for: it reloaded the new object's empty saved mask over whatever was selected. Errors moved from the foot of a scrolling pane, off screen, to a status line that also says what to do next (#579).
- Added unattended propagation in `MaskPropagation.swift`: a voxelised footprint of the mask refitted frame by frame, saved once, stopping where the object is lost (scaled for range), doubles, has a rival fit, jumps, or meets another object's points. On a copy of the real pack a car labelled in 19 frames was carried to 167, its point count rising from 5 to 6,656 and falling to 54 (#579).
- The first attempt carried that car nowhere: a rival fit was any position a metre away scoring 80% as well, and a six-metre car moved a metre along itself still covers most of its own returns. A rival is now a separate peak with a valley before it (#579).
- Added `ObjectProposer.swift`. Foreground in the same half-metre voxel in four frames of five is fixed clutter, one proposal a patch; the rest is clustered on the column lattice and chained by the same footprint fit. Against the operator's own labels the car was one proposal in all 19 frames at a median overlap of 1.00, and 445 proposals covered 83% of the labelable foreground. Scoring every search offset at once took it from 137 s to 24 s in a debug build with identical output (#579).
- Chose to cluster the pack's points rather than read the run's clusters: a recording keeps cluster boxes and not their membership, and that run starts 218 tracks in the twenty seconds, which a reference should not inherit (#579).
- Packs now carry the settled background. The recording had 20 snapshots of about 67,000 returns, one every fifteen seconds, and the exporter skipped them as frames without points. They are ordered by record position: the first is stamped after every frame that follows it, which also ended any windowed export at frame 0, and the sequence number never moved (#579).
- Packs record the run's L4 height band, read from the composed config under the engine that ran, so "ground" in the tool is exactly what the clusterer never saw. The exporter refuses `full` coverage when every classed point is foreground: the newest pack claimed it with 1,076,843 points, all foreground (#579).
- Set the background-change threshold by measurement on a real pair of snapshots: a quarter of a metre flagged 6,507 of 67,015 returns, mostly range jitter; half a metre, 2,295; a metre, 271 (#579).
- Added a sixteen-sector completeness ring, a frame strip, object names over their points, and the main view's words (Run, Frame, Labelled by) after "Operator name" was read as the name of a track and saved as the author of 61 masks (#579).
- Wrote the operator's guide, [point-annotation-tool.md](lidar/operations/point-annotation-tool.md), and the delivery record in the annotation plan (#579).

## September 20, 2026 - Annotation: views an operator can work in

- Found why a region could not be selected: both orthographic views re-fitted to every sample and could not be panned or zoomed, so a car was a few pixels across in the same white as the wall behind it. The views now hold their framing, zoom about the cursor, and pan, through an AppKit input layer because SwiftUI has no scroll-wheel or second-button gesture (#579).
- Added a third, 3D view that reuses `MetalRenderer` on the pack sample, class toggles in the main view's colours, and frame sync with the main view by capture timestamp. A step made to follow the main view does not ask it to seek, or the two windows drag each other backwards during playback (#579).
- Added sphere and column brushes on a local 0.5 m lattice with eight half-metre voxels a column. The Playback menu's bare keys mean the Annotation window when it is in front; a bracket that arrives by both routes is applied once, by event timestamp (#579).
- Tested the viewport by mounting it in a window and delivering mouse and scroll events through its hit test, since a viewport that looks right and ignores the mouse passes every unit test (#579).
- Covered the annotation export's failure boundaries for Codecov (#579).

## September 19, 2026 - Annotation: review fixes, paths and the sandbox

- Split the annotation toolset out of the state-estimation branch into its own PR and addressed its review: `ResolvePathWithinDirectory` with a pinned canonical-path contract, the pack directory resolved through `resolveLidarDir`, and the packs directory created before anything is resolved inside it, which had made the first export on any machine a 500 (#579).
- Set the capture, recording, plots and annotation paths independently (`--lidar-vrlog-dir`, `--lidar-plots-dir`, `--lidar-annotation-dir`). Deriving the recording directory from the capture directory was why VRLOG replays returned 400 once captures moved to an external volume (#579).
- Let the sandboxed app open and save packs the server had just written: a remembered security-scoped bookmark for the packs folder, asked for once. Stopped the sheet pre-selecting "Full scene" coverage (#579).

## September 17, 2026 - Annotation: reachable from the app

- Wired the annotation toolset into the app: a window, a pack picker, `POST /api/lidar/runs/{run_id}/annotation-export`, and a sheet that generates a pack from a run without leaving the app. Leaving a pack is guarded against unsaved membership (#579).

## September 16, 2026 - Annotation: point-cloud editing toolset

- Added the point-cloud editing toolset for the physical reference pilot: pack reader, canonical-index lasso and slab selection, membership history, and the session rules for navigation and provenance. Made track and run IDs selectable and copyable (#579).
- Delivered the point-cloud annotation client in the macOS visualiser: orthographic lasso and rectangle selection over a depth slab against canonical pack indices, add/subtract, stroke undo/redo, a second view for the contamination check, operator provenance, and dirty-navigation protection. 94 Swift tests, with the pack reader tested against bytes the Go writer actually produced and pinned on both sides. Selection is through-slab rather than hidden-surface picking; a radius brush and reviewed propagation remain future work (#579).
- Rendered `/app/lidar/tracks` through the same three.js player as the published scenes, driven by a session built over live database observations, and ported track picking and missed-region marking into the 3D view. Deleted the 932-line flat canvas map and the dead background-grid fetch, per-track observation cache and overlay-offset controls it kept alive (#559).
- Implemented Kalman gaps K1 and K2 as opt-in options and left both defaults alone, because neither predicted failure mode reproduced. K1's coupled process noise is 2.7% of the normalised distance to a 2 m/s² manoeuvre against a gate 173x wider; K2's premise did not reproduce at all, the shipped naive covariance form losing exactly zero symmetry over 10,000 cycles. Both measurements are asserted rather than logged, so a tuning change that makes either bite fails the suite (#559).
- Found and fixed a real inversion behind gap P1: L5's Guard 2, which exists to refuse an ambiguous heading from a near-square cluster, guarded its division with `maxDim > 0` and so skipped the check entirely for a zero-extent box — accepting the _most_ ambiguous measurement while correctly refusing a 5 cm square. P2's negative-discriminant fallback is removed rather than corrected, by computing the discriminant as the non-cancelling `(c00-c11)² + 4c01²` (#559).
- Measured gap HW1 against 367 deployed background snapshots and 24,007,669 settled cells with a new `lidar-closeness-audit`: the sensor beats its own specification in situ (median spread 6 mm close in, 47 mm past 100 m), the modelled noise term supplies 86-98% of the acceptance window against the measured spread's 1.5-5%, and the median effective window reaches 1.11 m at 10-20 m and 8.99 m past 100 m. The detection cost remains unmeasured and is filed as a follow-on (#559).
- Attributed the resulting wide-spread tail by ring elevation and disconfirmed the feedback-loop hypothesis: the rings that report the most foreground (11.2-12.6%) carry the fewest high-spread cells (0.5-1.0%), inverting the prediction, and 38.7% of high-spread cells sit above the horizon where the beam never reaches the road (#559).
- Added the Section 5.4 solid-body estimate contract: a named physical reference point, the filter's own covariance block, a bimodal orientation belief carrying the 180-degree ambiguity explicitly, per-dimension beliefs with sigma and admissible-frame counts, motion class as a posterior, and the estimation lifecycle. Seeding states the medoid's known bias — half a body width, ~0.95 m for a vehicle — where the measurement noise would have claimed ~0.22 m about a position that may be a metre out (#559).
- Promoted the synthetic Pandar40P pass into `l4perception` as the seed of the evaluation corpus, so Section 3's evidence is reproducible in the repository. It records one correction to the plan: §3.1's stated ring band cannot see its own vehicle for most of the pass, so the generator fires the real hardware elevation table and reproduces the mechanism (the medoid pinned at exactly -0.900 m on 20 of 40 frames) rather than §3.2's per-frame figures (#559).
- Implemented the near-edge measurement model with Section 9.1's variable-rank measurement: one visible face constrains its own normal and leaves the perpendicular direction to the prediction, with a covariance tight along the constraint and effectively unbounded across it. Lateral error against the synthetic pass is 0.0000 m mean and max, against the OBB centre's 0.1391 m and 0.8038 m. Its bias is exactly the dimension-prior error one-for-one, which is what makes the solid body's sigma and provenance load-bearing (#559).
- Ran experiment E1 against the three-site corpus — 43,068 scored frames matching the committed baseline exactly, 195,389 observations, 160,011 estimates, every site repeat-verified byte-identical. E1.1 finds the medoid's lateral offset near zero end-on and rising monotonically to 0.35-0.41 of the body's half-width at broadside at all three placements, and the trend survives range stratification in every well-populated cell. **Section 3's hypothesis is confirmed: the medoid's error is deterministic in aspect angle, so no estimator can filter it out.** A sign bug in the first E1.3 implementation initially inverted its conclusion by letting vehicles passing on opposite sides cancel; E1.1's independent reference exposed it: [E1 record](lidar/operations/state-estimation-e1-lateral-error.md) (#559).
- Recaptured the `mac` perf baselines, which had recorded a workload the branch does not produce (1,611,256 foreground points against an actual 985,224) and so were being refused fail-closed. Diagnosed before recapturing: the branch's workload matches _main's_ baseline to within one cluster, so nothing had regressed and the stale file had captured a transient intermediate state (#559).
- Made the web build self-contained by resolving `three` from web's own dependency rather than from `public_html`, which CI has no reason to install — the build passed locally and failed in CI for that reason alone, taking the Go integration tests with it (#559).
- Replaced the sleep-based frame-timer tests with an injectable clock. `time.Sleep` guarantees a minimum and not a maximum, so asserting which stage was longest was at the mercy of the scheduler; exact durations also made the assertions stronger, catching a one-millisecond attribution error the previous bounds could not (#559).
- Rewrote the quarter-block lint in python comparing code points. Matching a literal bracket expression with grep was locale-dependent: outside a UTF-8 locale it matched byte-wise, and every character in U+2596-U+259F shares the em dash's lead byte, so `make lint` reported roughly eleven thousand findings locally while passing in CI. The check no longer needs to exclude itself, and excluding vendored code from black and ruff cleared `make lint-python`, whose only failures were Python 2 `print` statements in the libpcap submodule (#559).

## September 15, 2026 - Evidence oracle profiling & deterministic track identity

- Bumped the application dependency group with three updates (#578).
- Gave `FrameEvidenceStore` prepared statements and explicit cleanup, replacing per-write statement preparation on the batched persistence path (#559).
- Streamlined corpus replay preparation so a run reaches the evidence store without repeating manifest and source checks (#559).
- Persisted the tracker's deterministic `CreationSequence` alongside each estimate and keyed the evidence oracle on it. Two replays of the phase01 corpus had reported 99.2% of rows as different when every value was byte-identical: the random `track_id` was being hashed as content (#559).
- Added a plan proposing a v5 UUID seeded from `(site_id, sensor_id, frame_unix_nanos, cluster_id)`, reproducible across replays and collision-safe across restarts and sites: [design doc](plans/lidar-deterministic-track-identity-plan.md) (#559).
- Generalised the P11 ground-plane fit from one global plane to an independently-fitted plane per grid cell (`l3grid.RegionalGroundSurface`), falling back to the global fit where a cell has too few settled points. Measured against the three-site corpus: Marina 4.35% grade, Columbus 0.88%, Embarcadero 0.14% (#559).
- Stopped stamping every site with the same placeholder calibration identity. `siteCalibration` now builds a real per-site yaw rotation from `site-index.json`'s operator-measured `north_azimuth_deg`, so `calibration_id` genuinely varies by site. Mounting height and tilt stay reported as ground-fit evidence rather than folded into the identity, so it does not wobble with ordinary measurement noise (#559).

## September 14, 2026 - Batched frame evidence & source manifests

- Batched a frame's observations, estimates, and residuals into one SQLite transaction with prepared inserts, replacing frame-by-frame writes (#559).
- Added `lidar-evidence-oracle`, comparing batched writes against existing frame-by-frame evidence using table, ordered-row, payload, and linkage checks (#559).
- Added source-manifest verification, so a corpus run refuses to proceed when recorded source digests disagree with what is on disk (#559).
- Replaced `sql.Open` with `OpenReadOnly` on the analysis paths, with a unit test asserting the connection rejects writes (#559).
- Moved the Hugging Face dataset card out of the repository, and reformatted the backlog around independently deliverable outcomes (#559).

## September 13, 2026 - Deterministic scene capture & priors service review

- Landed deterministic scene capture milestones 1 and 2 (#576): frozen multi-angle stills and frozen-target bullet-time camera paths, with readiness and pixel-stability checks, a provenance manifest, and deterministic trail history across 26 files.
- Fixed layer-visibility initial state so a capture starts from a declared scene rather than whatever the viewer last displayed (#576).
- Added the spatial priors service review alongside the SF bootstrap plan references (#577), 1,920 lines across four documents.
- Prepared the settled static captures for Hugging Face publication, and stabilised the state-estimation replay path (#559).
- Replayed immutable multi-file cases end to end, crossing the five-minute PCAP roll-over inside a single replay case (#559).

## September 12, 2026 - Capture-time benchmark replay & immutable observations

- Made the perf benchmark replay in capture time (#568). It had called `ProcessFramePolarWithMask`, which stamps `time.Now()`, so L3 warm-up and freeze windows advanced with machine speed: two CI runners 27% apart in wall clock produced foreground counts 11% apart on the same capture.
- Established that this was the original defect behind the degenerate June baseline. A 30-second warm-up measured in CPU time cannot elapse inside a five-second replay, so the grid never settled (#568).
- Confirmed the fix by measurement: two runs reported identical foreground counts of 985,224 and cluster counts within 0.07%, and `l3-only` and `full` agreed after previously disagreeing by 33% (#568).
- Added Columbus scene controls, background and grid layer toggles, VRLOG provenance display, and a build-version readout to the published scene viewer (#574).
- Added the deterministic screenshot capture plan (#575).
- Persisted versioned state estimates and residuals through a new pipeline sink and SQLite store (#559).
- Added the immutable observation replay foundation with a phase01 corpus fixture, and recorded OBB centre measurement provenance so an estimate states which interpretation produced it (#559).
- Imported multi-file archive replay cases into the web replay-cases page, and added the SQLite schema v2 freeze-and-cleanup plan (#559).

## September 11, 2026 - Multi-file PCAP joins & published scene bundles

- Added `internal/lidar/capseq` for multi-file replay cases (#569). Field capture writes rolling five-minute PCAPs, so a static period routinely straddles a file boundary, and the previous workaround merged files into a unioned copy at 1.2 GB per five minutes.
- Graded every join against field tolerances: seamless at 10 ms or less, acceptable to 1 second, broken beyond that. Grading is data rather than failure, so a broken run still builds and names the join that broke (#569).
- Dropped a revolution straddling a merely acceptable join. Fabricating one frame from two moments half a second apart is worse than a missing frame, because L3 records it as observed background (#569).
- Published the generated scene catalogue and bundles for 25 scenes (#573): 509 compressed frame chunks holding 148,872 frames, with per-scene background, manifest, header, chunk index, timeline, and vantage data.

## September 10, 2026 - Archive site inventory & scene catalogue

- Indexed 23 sites across the three recording days, each with the captures it spans (#571). A day is one continuous recording, so a site is a static stretch inside it rather than a separate file.
- Found 9/1 invisible because all 47 of its captures were unanalysed. Re-analysing them as continuous streams gave six static stretches of 18 to 22 minutes, done per recording block because the recorder restarted at 16:29 (#571).
- Read site positions by eye from a photograph of the field map, accurate to about 450 m where a published scene gave an independent check. Four sites carry no position rather than a guess (#571).
- Bumped the application dependency group with three updates (#570).
- Grouped the scene list by level 10 area, made the field mark the only name a scene has, drew the S2 cells on the map, and carried two measured angles into the viewer (#569).

## September 8, 2026 - Annotation: immutable point packs

- Added immutable point packs in `internal/lidar/annotation`: canonical little-endian arrays, digests over the stored bytes, a pack digest that binds them, and a check for revision space exhaustion in the sidecar store (#579).
- Added `max_sample_points` to the L4 configuration, validation, and every fixture, bounding retained cluster points rather than leaving retention unlimited (#559).
- Added the L4BObserve and L4Perception modules with evidence-handling and clustering tests, and threaded sample points through L9 diagnostic frame processing (#559).
- Added immutable point packs and reference annotations, with a revision-space exhaustion check in sidecar storage (#579).
- Added a Pi performance benchmark runbook for capturing and refreshing baselines, and recorded recovery checkpoints across the affected plans (#559).
- Added a Python extractor for LiDAR jump candidates, with unit tests (#559).

## September 7, 2026 - Revision-safe annotation sidecars, VRLOG web profile & censored extent belief

- Added sidecar annotations for LiDAR packs with revision-safe storage: a non-blocking lock, atomic replace, retained revisions and conflict detection, so that a human reference object is stored apart from predicted track identities (#579).
- Added the VRLOG web profile and the scene catalogue publishing plan (#567), 10,180 lines across 70 files, including a Svelte scene route and a browser decoder generated from the same `.proto` under the existing drift gate.
- Defined the web profile as a narrowing of the existing VRLOG format rather than a new one: same directory layout, same `FrameBundle`, rotated at 100 frames and gzipped per chunk. An absent `profile` means a recorded VRLOG, so existing readers were unaffected (#567).
- Chose gzip over brotli because `DecompressionStream` supports `br` on Chromium alone (#567).
- Replaced the extent mean with a censored lower-bound belief, since a visible face bounds a body's extent from below rather than estimating it (#559).
- Added a bounded shape term to association and measured the duplicates it resolves (D2.2), and made host class a dimension of the performance matrix (#559).
- Added sidecar storage with revision management and conflict resolution (#579).

## September 6, 2026 - Heading abstention telemetry

- Split the heading-lock outcome three ways and marked forced releases, so an abstention is distinguishable from a rejection (#559).
- Attributed heading abstentions to their cause, bounded the axis hold, and reported acceptance rates (#559).
- Added the D2 measurement harness with the axis path defaulting to off, then re-measured the A/B comparison and relocated the D2.1 blocker (#559).

## September 5, 2026 - Headless replay harness & heading telemetry

- Added a headless PCAP replay harness and measured the Day 1 gate against it, with frame back-pressure making the harness deterministic (#559).
- Added the course-alignment metric (D1.1) and heading lock telemetry (D1.2) for LiDAR tracks, and released the OBB heading lock after repeated rejections (D1.3) (#559).
- Added the fragment guard and decoupled the ghost fade (D1.5, D1.6) (#559).
- Seeded the DBSCAN subsample from the point set rather than the clock, and sorted the association track list, which corrected an overstated determinism claim (#559).
- Added the unified scene semantics research proposal, and recorded the background warm-up requirement for replay captures (#559).

## September 3, 2026 - Perf gate rebuilt: a baseline that states what it measured

- Established that the nightly perf gate had been comparing against a recording of a pipeline that detected nothing. Built `lidar-bench` at the baseline's own commit (`72b5e11925ad`, 23 June) and ran it against `kirk0.pcapng` beside current `main` on one machine: **0 foreground points of 57,376,456, and 0 clusters**, against 1,626,782 and 14,137 today. The committed `cluster_time_ms`, `tracking_time_ms` and `classify_time_ms` of zero were literal, not rounding (#566).
- Traced the cause to settling never completing. Warm-up required `warmup_min_frames` **and** 30s of `warmup_duration_nanos` measured against system time, and the benchmark replays 83s of capture in under 5s wall, so the duration gate could never elapse and every point stayed background. #555's convergence early-exit is what turned detection on; the four "regressions" the gate reported afterwards were the cost of the pipeline finally doing its job (#566).
- Confirmed #555 introduced neither hot path it was blamed for: it never touched `background_region.go` or `cluster.go`, which date from #417/#407/#370. The mask retention and DBSCAN expansion growth were latent and were fixed in #565 (#566).
- Replaced the `pipeline.profile` enum with `engine: "none"` after review. The enum was a second mechanism answering a question each layer's `engine` selector already answered, it put the depth in a Go lookup table rather than in the file, and it left an `l3-only` config carrying — and validation _requiring_ — a fully populated `l4.dbscan_xy_v1` block whose nine parameters had no effect. The depth is now read off the selectors, so nothing can disagree with what runs (#566).
- Kept the closed set as a validation rule rather than an enumeration: disabled layers must form a suffix, so `l4.engine: "none"` beside a live L5 is rejected at load. Tracking cannot consume clusters that were never produced (#566).
- Made a benchmark state its workload. Documents now carry the profile, a SHA-256 fingerprint of the whole resolved tuning config, platform, and work counters — frames, foreground points, background points, clusters, tracks. The comparator refuses to compare runs whose identity differs and names the field that diverged, rather than emitting a delta between incomparable things (#566).
- Removed the zero-baseline skip. `if m.baseline == 0 { continue }` meant `cluster_time_ms` and `tracking_time_ms` — the two fields that would have exposed the June baseline — were the two never checked, for three months (#566).
- Fixed `heap_alloc_bytes`, which produced the "+6763%" headline. It was read without a preceding collection, so it reported whatever the heap happened to be mid-GC-cycle: five runs of identical code spanned 18.8–40.0 MB. With `runtime.GC()` before the read it is 17.0 MiB on every run of the full profile (#566).
- Dropped `frames_per_second` from the gated set as frames over the same wall clock already gated, which reported one runner slowdown as two independent regressions; `total_alloc_bytes` replaced it, being stable to under 1% (#566).
- Added an absolute frame budget of 98 ms, checked with or without a baseline. `capinfos` puts `kirk0` at 83.183383s over 832 frames — exactly 10.0 Hz — so the budget sits 2 ms inside the frame interval. Locally the full profile puts 0–1 of 832 frames over it, worst frame 115.0 ms, which is what the 1% allowance absorbs (#566).
- Made baselines the median of five runs rather than one sample, emitted whole so every field comes from the same execution. Five local repeats spanned 3.6–14.2% on wall clock, against a 30% gate threshold (#566).
- Retired `baseline-kirk0-ci.json` and `baseline-kirk0.json` rather than renaming them. They recorded a degenerate state, not a profile, and carrying the file forward under a new name would have enshrined it (#566).
- Found the capture workflow could not solve the problem it was written for: GitHub registers `workflow_dispatch` from the default branch alone, so a change needing a fresh baseline cannot dispatch one until after it lands — and needs the baseline to land. A path-filtered `pull_request` trigger closes the loop, firing exactly when a tuning change moves the fingerprint and invalidates the committed file (#566).
- Deleted `kirk0_benchmark.json`, a stray benchmark artefact committed by accident in #564, and ignored the pattern (#566).
- Bounded region identification memory to surviving regions (#565). Every `Region` carried a `CellMask` sized to the whole grid, and a freshly settled 40x1800 grid fragments into roughly 12,700 components before merging down to 50: about 915 MB of masks to keep 3.6 MB.
- Found none of it freed either, because `mergeSmallestRegions` dropped regions with `regions = regions[1:]` and the returned slice keeps the whole backing array reachable. Live heap over a kirk0 replay fell from 1.72 GiB to 22 MiB (#565).
- Threaded one scratch buffer through DBSCAN expansion and enqueued each point at most once, having appended every queried neighbourhood onto the seed list unfiltered. DBSCAN time fell from 8,523 ms to 4,225 ms, frame p95 from 74.7 ms to 37.9 ms, and total allocation from 62.2 GiB to 16.5 GiB (#565).
- Confirmed behaviour unchanged with a 500-trial differential run against the previous implementation, which produced identical labels on every point (#565).
- Bumped the application dependency group with three updates (#564).
- Recorded 49.22 GB of field LiDAR capture on `/Volumes/lidar/lidar/s2` from 10:00 to 17:04, 71 five-minute segments across sites `s2_sf_6`-`s2_sf_9`.

## September 2, 2026 - S2 artefact conformance

- Standardised the geographic plans on S2, and documented the cell hierarchy and positioning (#561).
- Threaded one canonical S2 tag pair through every artefact derived from a located static PCAP segment, so a detached capture stays coarse-addressable without opening a sidecar (#561).
- Ruled that tokens are copied from trusted split metadata and never recovered by parsing a source basename, and that storing only one of the pair is invalid (#561).
- Recorded S2 tags as execution provenance rather than part of parameter-set or run-config identity hashes (#561).
- Bumped the documentation dependency group with two updates (#563).
- Recorded 48.28 GB of field LiDAR capture on `/Volumes/lidar/lidar/s2` from 09:14 to 16:58, 71 five-minute segments across sites `s2_sf_2`-`s2_sf_6`.

## September 1, 2026 - S2 Hilbert SVG tool

- Added `tools/s2-hilbert`, a CLI generating SVG infographics and JSON data for S2 cell traversals along the Hilbert curve, with land and water masking (#562).
- Generated the orientation legend, the coarse level 12 examples, and the level 10 quad composite used in the geographic indexing documentation (#562).
- Wrote the hierarchy module to parse canonical and display S2 token forms and convert them to geographic geometry, keeping the core reusable from both Node and the browser (#562).
- Recorded 36.25 GB of field LiDAR capture on `/Volumes/lidar/lidar/s2` from 12:25 to 16:39: `s2-sf-0.pcapng` (12:25-12:46) plus 47 five-minute `s2-1` segments (12:48-16:39).

## August 31, 2026 - Stall closed, replay state repaired & clean VRLOGs

- Confirmed the visualiser stall fixed with an hour-long four-stream soak: 35,987 frames and 320.5MB per stream at a sustained 10 frames per second, with the visualiser attached throughout (#555).
- Recorded four gaps over a second across that hour, worst 2.144s, each seen by all four streams within 5ms. Independent connections do not stall in lockstep, so these are pauses in frame production rather than the transport; one coincides with a source change and three remain unattributed (#555).
- Wrote down what the result does not establish. Every observation is macOS with client and server on loopback, which never exercises a real round trip, so backlog items now call for the same soak against another host, the Linux build and a Raspberry Pi (#555).
- Fixed a replay start that failed after claiming the slot leaving the source on PCAP for good. Nothing corrected it afterwards because no replay had ended, so clients sat on `REPLAY (PCAP)` while live packets flowed underneath (#555).
- Made pausing a replay send one frame. Playback state reaches a client only on a frame and pausing stops frames, so the transition could never report itself: the server sat paused at frame 2518 of 6846 while the visualiser went on showing playback, pause looked dead and rate changes looked ignored (#555).
- Made a rate change while paused send a frame for the same reason (#555).
- Reported a silent live sensor as `IDLE` ahead of settling. A grid settles on arriving frames, so with the sensor quiet the count cannot advance and `SETTLING 00s` promised progress that could not happen (#555).
- Opened VRLOG recordings with a background frame, so replaying one shows a scene immediately. Recordings had inherited whatever order the capture happened to have, one carrying its background at frame 2 and another at frame 116 (#555).
- Ticked settle-before-recording and analysis mode by default on the `:8081` form, the pair being required together (#555).
- Added `cmd/tools/vrlog-check`, which reports encoding, frame count, first background index and a verdict using the recorder's own decoder. Six fresh recordings all carry their background at frame 0, against first-background indices of 111 to 616 across the older set (#555).
- Added `scripts/record-clean-vrlogs.sh` to re-record captures through the settle-before-recording flow, sending server-relative paths and asking the server what it can see rather than testing the local filesystem (#555).
- Added `make debug-grpc-soak` for long multi-client runs, each stream on its own connection so per-connection flow control stays visible (#555).
- Covered the source-switch regressions in Swift, including returning to live restarting the gRPC stream, which had no test at all and is how it shipped broken. The tests answer from a URLProtocol stub: an earlier draft posted to whatever server was listening (#555).
- Normalised the backlog: every completed entry now carries a bracketed PR number, the section is sorted by PR ascending, and all seventeen PR numbers above 450 were checked against GitHub (#555).

## August 30, 2026 - Replay handover & PR #555 close-out

- Removed three speculative compatibility layers after review: the duplicate proto `is_live` field and its Swift inference path, the settling fraction no view displayed, and the optional settling capability that had allowed production wiring to no-op while tests passed (#555).
- Kept settling state on live frames only and removed `AppState.isLive`, the third local spelling of the source. Replay frames now carry their recorded scene without inheriting the live grid's warm-up state (#555).
- Made a finished replay start the live listener while keeping the recording on screen, then hand the pipeline to live only after a new packet arrives. Packets from before parking do not count, and a later replay or operator action cancels the earlier watch (#555).
- Routed every gRPC subscription through the publisher's normal client-registration path, so a new client receives the current background even when the recording's next background frame is minutes away (#555).
- Cleared the cached replay background when returning to live, preventing a reconnecting client from drawing live foreground over the recording's scene after the grid reset (#555).
- Reported a live sensor with no packets for three seconds as `IDLE`; `SETTLING 04s` takes precedence, replay sources never inherit the silence flag, and rate controls appear only when a replay timeline exists (#555).
- Expanded close-out regression coverage across the actual Go playback-info composition path, proto-to-Swift decoding, live-silence boundaries, replay-stop background clearing, parked-replay handover, and the independent gRPC probe's gap accounting (#555).

## August 28, 2026 - Visualiser stall isolation & AttributeGraph cycles

- Added `make debug-grpc-probe`, a second client using Go's HTTP/2 stack, and streamed 2,000 frames / 14.6 MB without a server-side stall. That isolated the long pauses to the Swift client rather than the publisher (#555).
- Added timestamped, line-buffered app logging and a main-thread watchdog. The watchdog stayed responsive while the gRPC read loop paused, ruling out the working theory that a wedged main thread starved the transport (#555).
- Traced every captured AttributeGraph cycle to AppKit recomputing the key-view loop from a changing enabled state, then replaced dynamic `.disabled()` calls with `.inert(_:hint:)`; the measured cycle count fell from 24 to zero across a three-minute, 800-frame run (#555).
- Set the grpc-swift HTTP/2 window explicitly to 16 MB. The first measured build with it recorded zero stalls over 25 minutes and 11 connections, while the comparison builds recorded 7–30; longer field confirmation remains in the backlog rather than being claimed as proven (#555).

## August 27, 2026 - Adaptive settling & visualiser stream robustness

- Completed Phase 4 of the settling plan: the background grid now ends warm-up on measured convergence, so a quiet scene settled in 5.9 seconds against its 30 second ceiling: [design doc](lidar/operations/settling-time-optimisation.md) (#555).
- Found the settling decision implemented twice, in the background-update path and in foreground extraction. The copies had drifted, convergence went into the one the live pipeline does not run, and the feature was unreachable in production despite passing its tests (#555).
- Consolidated both paths onto `settlingCompleteLocked` and added a test asserting neither reimplements the decision. Lowered `warmup_min_frames` from 100 to 50, since that gate sets the floor under any settling time convergence can reach (#555).
- Made settling report itself: the plan it starts with, progress towards the frame minimum, and either the reason it completed or the unmet criterion holding it up (#555).
- Surfaced settling to operators through `GET /api/lidar/data_source` and proto `PlaybackInfo`, shown in the visualiser as a `SETTLING 5.9s` badge. An unsettled grid renders an empty scene, which is otherwise indistinguishable from a dead sensor (#555).
- Handed each newly subscribed gRPC client the current background, and sent a fresh snapshot as settling completes. A replay load published the background 42 ms before the client restarted its stream, so the grid never arrived (#555).
- Fixed a stream that ended signalling only "replay finished", which left the visualiser reporting itself connected after a server restart. One run showed five minutes of server uptime with zero client connections while the app claimed a live connection (#555).
- Replaced the Live control with a Live/Replay segmented picker, made returning to live restart the gRPC stream, and keyed inspector visibility on `showSidePanel` alone so the button can close it with a track selected (#555).
- Named the frame in the send-stall warning, which disproved the working hypothesis: the blocking frame was 39 points and 5.9 KB, in live mode, so frame size was never the cause. The client-side hang remains open: [design doc](plans/lidar-visualiser-stream-robustness-plan.md) (#555).

## August 26, 2026 - Frame loss accounting & replay parking

- Bumped the application dependency group with three updates (#558) and the documentation group with two (#560), and downgraded TypeScript to 6.x with a native alias for compatibility.
- Separated publish-stage from client-stage frame loss, which had been summed into one ratio that pegged at 50% and hid the real drop rate (#555).
- Stopped counting a source change as thousands of dropped frames, and summarised dropped frames rather than logging each one (#555).
- Bounded stream sends so a client that stops reading cannot stall the publisher, and reported the stall instead of severing the stream (#555).
- Delivered buffered frames in capture order, and confined the frame-rate throttle to replays so live input is no longer decimated (#555).
- Let a finished replay stay the data source rather than silently reverting to live, and closed the window where a teardown looked like an idle replay (#555).
- Added a Live toggle to the visualiser and moved Clear to the View menu (#555).
- Added and then split the LiDAR state estimation and vehicle behaviour plans, revising both to cover road users, the evidence split, and elevation (#559).

## August 19, 2026 - Background retention across source changes

- Reset the grid when returning to live, so a live scene is no longer composited onto a replay's retained background (#555).
- Refreshed the background on snapshot restore and reconciled live state on packet arrival (#555).
- Released the pipeline after a recording run, and cleared the background when a PCAP replay starts (#555).
- Let live packet presence decide what a finished replay does, rather than assuming the operator wanted live input back (#555).
- Kept the macOS API clients off Foundation's on-disk URL cache, which was the source of the `disk I/O error` messages the app logged: nothing in the app reads that cache (#555).

## August 18, 2026 - Offline homepage & replay teardown

- Served the homepage offline in the Raspberry Pi image (#553), and created the docs symlinks before the offline docs tests run.
- Added a live docs 404 spider for checking published documentation links (#553).
- Released `0.5.1-pre31` across the canonical version surfaces (#555).
- Kept generated build stamps out of git, so `BuildInfo.swift` no longer produces spurious diffs after every macOS build (#555).
- Consolidated replay teardown into one path, and made replays that end return to live by themselves (#555).
- Always released the replay slot, and traced every pipeline transition so state changes are attributable (#555).
- Stopped the live grid overwriting a replay's own background, never discarded a background frame in transit, and guarded the publisher's background bookkeeping fields (#555).

## August 17, 2026 - LiDAR pipeline state model

- Added `PipelineState` as the single store for what drives the pipeline and what is captured from it, replacing three stores that could disagree under two different mutexes: [design doc](plans/lidar-pipeline-state-model-plan.md) (#555).
- Migrated the lidar server's state fields onto it and removed the divergent `DataSourceManager` accessors, which held a shadow source production never wrote (#555).
- Reported VRLOG replay and recording state from the lidar HTTP API. `GET /api/lidar/playback/status` had returned a hardcoded live status, because its assignment site did not exist (#555).
- Stopped the live listener during VRLOG replay and suppressed live frames while one is active, so live and replayed frames no longer interleaved into the recorder (#555).
- Added `source_mode` and `recording` to proto `PlaybackInfo`, and made the Swift client read the mode instead of inferring it from `is_live` and `seekable` (#555).
- Marked the settling and recording passes of a two-pass PCAP replay, which had been indistinguishable from outside, and identified settled PCAP snapshots by their resolved path (#555).
- Fixed GitHub URL handling and documentation link resolution, and isolated the offline docs folder pages under test (#553).

## August 16, 2026 - Offline docs link validation

- Added link validation scripts for offline documentation integrity (#553).
- Merged TENET 7, cohesive releases (#556).

## August 15, 2026 - Replay windows & coverage gate

- Added start and duration parameters for PCAP replay to the lidar commands, and moved `Run` calls onto a `Config` struct so the window threads through one primitive rather than per-engine flags (#555).
- Clarified replay duration defaults, and documented the new parameters in the performance regression testing guide (#555).
- Updated the LiDAR test commands in CI for clarity and execution time (#555).
- Added tests behind a coverage gate (#554), and enhanced `import_paths_for_patterns` to support build tags.
- Clarified repository source link resolution and build metadata in the offline docs, and improved the link validation logic in the offline docs checker (#553).

## August 14, 2026 - Key documentation standardisation

- Standardised the opening TL;DR paragraphs and heading structure across the key top-level docs, covering `ARCHITECTURE.md`, `CHANGELOG.md`, `CODE_OF_CONDUCT.md`, `COMMANDS.md`, `DEBUGGING.md`, `MAGIC_NUMBERS.md`, `TENETS.md`, and the `data/` and `docs/` hubs (#552).
- Relocated the LOC and coverage chart in `README.md` and clarified its description (#552).
- Recorded the missing PR number against a completed backlog item (#552).

## August 13, 2026 - Embedded Tailscale installer

- Removed Tailscale from the Raspberry Pi image build and moved the installer into the `velocity` binary, so the image now ships with no Tailscale package, apt source, keyring, service, credentials, or state (#551).
- Added `internal/tailscaleinstall`, which downloads the pinned 1.102.2 static release over `net/http`, verifies a baked SHA-256, extracts only the `tailscale` and `tailscaled` entries, and installs under `/opt/velocity-report/tailscale/<version>/` with an atomic `current` and `/usr/local/bin` symlink refresh (#551).
- Hardened the installer against review findings: rejected tar entries over 64 MiB or short of their declared size, included the expected hash in checksum-mismatch errors, refused to overwrite non-symlink PATH entries, and fixed a `0700` extraction mode that stopped the non-root `velocity` account executing the client (#551).
- Kept the privileged boundary a literal sudo allowlist, so the `velocity` service account can run only the fixed install, enable, and disable argv vectors rather than receiving a wildcard grant (#551).
- Deleted the `image/stage-velocity/07-velocity-tailscale/` stage with its apt repository setup, keyring fetch, transient `curl` install, and image-time daemon masking, then updated the remote-access guide, the QEMU and Headscale plans, and the image plans to match (#551).
- Bumped the release to `0.5.1-pre29` across the canonical version surfaces (#551).
- Landed a grouped application dependency bump taking `modernc.org/sqlite` to 1.56.0, with a TypeScript/ESLint compatibility fix and a LiDAR replay test that now waits for the EOF pause (#550).

## August 12, 2026 - Fully-static Linux builds

- Added `make build-radar-static`, producing fully-static linux/amd64 and linux/arm64 binaries with no glibc, `libpcap.so`, or libnl dependency, built entirely inside a hermetic Docker image so contributors need only `docker` and `git` (#513).
- Built the toolchain around `zig cc -target <arch>-linux-musl` as the cgo compiler, with `libpcap.a` compiled from the `third_party/libpcap` submodule pinned to the 1.10.6 release tag, giving host-speed cross-compilation to both architectures without qemu (#513).
- Made the output reproducible: pinned `debian:bookworm-slim` by digest, SHA-256-verified the Go and zig tarballs before extract, defaulted `BUILD_TIME` to the HEAD committer date in UTC, and refused to build from a dirty tree unless `ALLOW_DIRTY=1` (#513).
- Split the Dockerfile into toolchain, libpcap, and build stages so the `libpcap.a` layer only invalidates when the submodule pointer moves, and sourced `GO_VERSION` from `go.mod` so the pin cannot drift (#513).
- Added a `--self-check` flag exercising DNS, UDP and TCP bind, and libpcap, plus `scripts/smoke-test-static.sh` to run the binary in a fresh `debian:bookworm-slim` of the matching architecture, and a `static-build-ci.yml` workflow covering both arches on every build-touching PR (#513).
- Pointed the image build at the static binary path and refreshed the deployment documentation for the `velocity device` commands (#513).
- {patrickod/tailscale-acls} Added an opt-in Tailscale ACL gate for admin and view access, then addressed the Copilot review on it.

## August 11, 2026 - Multi-sensor capabilities & serial rollout

- Replaced the single `radar` and `lidar` fields in `/api/capabilities` with named-object maps keyed by sensor name, updating the Go provider, API tests, frontend types, the Svelte capabilities store, and layout gating (#547).
- Preserved smart polling across the redesign, so radar-only deployments fetch once while LiDAR deployments keep polling for runtime state transitions (#547).
- Documented the empty `lidar: {}` semantics, dropped the unimplemented `last_reading_at` field from the plan and spec, and added `ready` to the status vocabulary, closing the review drift left by the superseded #430 (#547).
- Made enabled database configuration the runtime default for radar serial startup, kept the CLI `--port` as the fallback when no enabled configuration exists, and gave clearer startup errors for invalid saved configuration (#549).
- Added an "Apply enabled config" action on `/app/settings` backed by `POST /api/serial/reload`, with typed API client support and user-facing success and error feedback (#549).
- Validated the rollout on the deployed Pi and HAT at `/dev/ttySC1`, `19200 8N1`: device discovery, configured-port filtering, active-port protection, mismatched-settings diagnostics, and the safe no-op reload path all held while radar ingestion continued (#549).
- Deferred device and baud auto-detection, USB-adapter validation, and deliberately changed-setting reload to a later release, and said so plainly in the serial implementation plan (#549).
- Bumped the release to `0.5.1-pre27` and pointed the remote-release utility at the canonical `image/velocity-binaries/velocity` build (#549).
- {codex/four-platform-build-standardisation-plan} Added a four-platform build target standardisation plan and logged it in the backlog.

## August 10, 2026 - macOS Developer ID signing, LiDAR maths planning & OBB doc recovery

- Completed the first local VelocityVisualiser Developer ID release-signing run: verified a usable `Developer ID Application` identity, signed the app with hardened runtime and timestamping, packaged `VelocityVisualiser-0.5.1-pre26.dmg`, submitted it through the `velocity-report` `notarytool` keychain profile, stapled the accepted ticket, and passed `make verify-mac` (#425).
- Fixed the DMG packaging hang by making Finder layout automation timeout-bounded and best-effort, with `DMG_LAYOUT=0` and `DMG_LAYOUT_TIMEOUT_SECONDS` overrides for local or CI packaging (#425).
- Corrected the release packaging target so `make release-mac` packages the signed app instead of rebuilding after signing, and updated the release docs with the local keychain, notarytool, and GitHub Actions secret boundaries (#425).
- Made unsigned CI DMG outputs explicit: manual or tag runs without the full release secret set now upload `UNSIGNED`-labelled DMG files and artifacts, while partially configured signing secrets fail the workflow early (#425).
- Recovered the useful OBB heading-stability guidance from the superseded #397 as documentation only, keeping the shipped `obb_aspect_ratio_lock_threshold` default at `0.25` and framing `0.15` and `0.10` as replay-validation candidates rather than landed behaviour (#548).
- Added the v0.5.2 [LiDAR maths coherence plan](plans/lidar-maths-coherence-plan.md): bidirectional citations between `data/maths/` and the Go implementation, a formalised L6 confidence model, new maths notes for L8 analytics and L1/L2 timing and geometry, and an unmasked CI gate (#540).
- Reviewed the backlog for the v0.5.2 plans, auditing stale items and correcting overclaims, and added the shape-descriptor plan with a classifier analysis lane outline (#540).
- Corrected the subsample determinism claim and logged a DBSCAN replay defect against the maths plans (#540).
- Refreshed grouped application and documentation dependencies, taking `ruff` to 0.16.1 and `mermaid` to 11.16.1, and pinned TypeScript back to `^5.8.3` for typescript-eslint compatibility (#545, #546).

## August 2, 2026 - Archived prototype references

- Pointed the velocity-coherent plan at archived prototype tags after closing #391 and #390: the prototype was written against the pre-L1-L9 flat `internal/lidar/` layout, and the layered refactor removed every file it touched, so neither branch could be rebased (#544).
- Added a "Prototype source (archived)" section naming `archive/vc-prototype-391` and `archive/algo-selection-390` with `git show` and `git checkout` recipes, and annotated the file table with per-file line and test counts (#544).
- Confirmed no design content needed rescuing: the plan plus the velocity-foreground-extraction architecture doc and the maths proposal are a structural superset of the branch's design document (#544).
- Refreshed grouped documentation dependencies (#543).

## July 30, 2026 - pnpm 11, TypeScript 6 & CI repair

- Moved the web tooling to Node 22 and bumped `packageManager` to `pnpm@11.18.0`, migrating the settings that pnpm 11 renamed (#536, #537).
- Repaired a corrupt `pnpm-lock.yaml` that broke CI: fixed the prettier specifier format, removed duplicate entries, and cleaned up stale dependencies (#537, #541).
- Took TypeScript to 6.0.3 and landed four grouped application dependency bumps across the Go, web, and public-site ecosystems (#536, #537, #541, #542).

## July 29, 2026 - Dependency refresh

- Bumped `@sveltejs/kit` to 2.69.1 and `postcss` to 8.5.18 (#538, #539).

## July 13, 2026 - Dependency refresh

- Refreshed grouped documentation dependencies for the docs site (#533).

## June 24, 2026 - LiDAR PCAP tooling consolidation

- Consolidated the offline LiDAR PCAP tooling into `velocity lidar pcap-split`, folding capture and health scanning, the motion and static timeline, and segment splitting into one tool under a new `lidar` namespace in the multi-call binary (#531).
- Built `pcap-split` as a two-pass design: pass one classifies each frame via the `BackgroundManager`, pass two re-reads the capture and copies each packet into its segment by capture timestamp, so nothing buffers and the shared PCAP reader stays untouched (#531).
- Replaced the per-cell noise-deviation motion signal with a background-drift ratio. The old signal conflated a busy but parked scene with genuine driving and produced false motion on the soma3 capture, while drift ratio is scene-independent. Added `sensor_movement_drift_ratio_threshold` to the L3 config block, default `0.35` (#531).
- Extracted the pipeline timing benchmark into `internal/lidar/lidarbench` with a standalone `cmd/tools/lidar-bench` wrapper, then pointed `make test-perf` and the nightly CI step at it while preserving the `PerformanceMetrics` JSON schema so the committed baselines stayed valid (#531).
- Removed `cmd/tools/pcap-analyse` along with track CSV/JSON export, ML training-data export, and SQLite persistence, capturing the removed pieces as self-contained architecture outlines in `docs/plans/lidar-offline-analysis-tooling-plan.md` (#531).
- Added read-only per-frame settling accessors to `l3grid`, and made frame assembly flush the in-flight frame on close so capture order survives (#531).
- Bumped the version to `0.5.1-pre26` and rewrote the pcap-analysis and performance-regression-testing docs for the new layout (#531).
- {claude/strange-colden-a99dc2} Tested and documented the diagonal-Q process-noise gap (K1): the L5 prediction step omits the position-velocity coupling term of the continuous white-noise-acceleration model, underestimating cross-covariance and making the gating ellipse overconfident along the correlation direction.
- {claude/strange-colden-a99dc2} Added an opt-in Joseph-form covariance update (K2), default off, which stayed roughly 14 times more symmetric than the naive form over 10k adversarial predict and update cycles while remaining positive-definite.
- {claude/strange-colden-a99dc2} Documented the mean-absolute-deviation versus standard-deviation relationship in the L3 background spread (B1): `RangeSpreadMeters` tracks MAD, so a settled cell observing noise of 0.1 converges to about 0.0798, not 0.1.
- {claude/strange-colden-a99dc2} Implemented CLEAR MOT (MOTA and MOTP) in `l8analytics` (M1) as pure computation, reusing `l5tracks.HungarianAssign` for per-frame matching. Ground-truth ingestion and endpoint wiring remain follow-ups.

## June 19, 2026 - Dependency cleanup

- Resolved the Dependabot security-update blockers by locking `dompurify` to 3.4.11 and adding `esbuild` 0.28.1 for Vite (#530).
- Removed unused dependencies: `seaborn`, `scipy`, and `responses` from Python, `flake8` from the alignment tools, `cross-env` from the web package, and `autoprefixer` from the public site (#530).
- Formatted the LiDAR sweeps page with the CI-pinned Prettier version (#530).

## June 11, 2026 - LOC & coverage chart

- Added a nightly LOC and coverage chart: a horizontal stacked SVG breaking down lines of code across js, go, mac, markdown, and scripts, with a hatched overlay marking the uncovered share of each coded language (#509).
- Published the chart to a dedicated orphan `stats` branch so neither `main` nor `public_html` churn each night, force-pushing only when the SVG content actually changes (#509).
- Wired the `loc-coverage-chart.yml` workflow to `workflow_run` after nightly CI succeeds on main: it downloads the coverage artifacts from the triggering run, runs `cloc --vcs=git`, and parses the Go cover profile plus two LCOV files (#509).
- Added a `go-coverage` job producing the merged Go profile under `-tags=pcap`, published `coverage-go`, `coverage-web`, and `coverage-mac` artifacts from nightly CI, and added a `make loc-coverage-chart` target for iterating on the renderer locally (#509).
- Made the chart dark-mode aware: compacted the layout, placed labels adaptively inside or outside segments, switched the uncovered overlay to a transparent red diagonal hatch, and drove outlines and off-bar labels from colour-scheme CSS classes (#529).
- Moved the chart caption out of the SVG into `README.md`, dropped the in-chart title, footer, and percentages, and relabelled "markdown" as "docs" (#529).

## June 10, 2026 - Chart timezone fix & pre25 release

- Fixed a dashboard bug where the Vehicle Count card showed a non-zero count while the timeseries chart below it sat empty between roughly 5pm and midnight in timezones behind UTC. The card sent raw unix timestamps while the chart sent `YYYY-MM-DD` that the server re-interpreted as a calendar day, so the two endpoints queried different windows (#527).
- Settled on one date contract: `/api/radar_stats` and `/api/charts/{timeseries,histogram,comparison}` now take `start` and `end` as ISO 8601 instants, so the server does no calendar-day interpretation and the original bug cannot recur (#527).
- Made `tz` display-only for axis labels and response formatting, and had the frontend build instants from the picked day in the selected display timezone via DST-correct `Intl` offset maths (#527).
- Defaulted the queried range to end at tomorrow-local so today's data is always in range, and fell back to the browser zone when the server default is `UTC` (#527).
- Kept report generation on `YYYY-MM-DD`, since those values land on the report row, in the PDF, and in the report filename, where a colon-bearing instant cannot live (#527).
- Added `docs/radar/architecture/dates-and-timezones.md` as the canonical reference for the DB, the API, and the frontend; storage stays UTC unix epoch seconds throughout (#527).
- Cut `0.5.1-pre25`: refreshed `release.json` and the Raspberry Pi Imager catalogue, and taught `scripts/update-release-json.py` to match both the new versioned and the legacy binary asset names (#528).

## June 9, 2026 - Typst map editor, typstbin hardening, pre24 release & plan graduation

- Built the report map-editor integration: an SVG overlay with style selection, FOV-triangle overlay rendering, tile snapshots loaded only with user consent, and report-overlay visibility plus stale-preview detection (#522).
- Replaced server-side OSM rendering with the consent-gated client editor: removed the dead Overpass module, the unused `/api/map/overpass` proxy, and the server-side OSM site-map renderer with its Phase 0 typst-prototype driver (#522).
- Hardened `typstbin` with a private cache-dir fallback, Windows exec-bit handling, and rejection of asset names that escape the work directory, then refactored the package and added comprehensive rendering and binary-resolution tests (#522).
- Improved Typst report layout: histogram bucket handling, figure clearance and placement, and a surveyor/contact line that renders only when present (#522).
- Added PDF-metadata support with trailer-parsing tests for Typst-generated reports, and consolidated the PDF migration docs and plan links (#522).
- Encapsulated the Docker temp-cache removal in a dedicated build-script function and added a test covering the Docker Go cache cleanup (#523).
- Cut `0.5.1-pre24`: bumped the version, refreshed `release.json` and the Raspberry Pi Imager catalogue, added the Typst report engine entry to the changelog, and refreshed the homepage `stack.png` hero (#523, #525).
- Restructured the June 4 to 9 devlog entries around the actual daily cadence rather than squash-merge dates, splitting the nginx and TLS removal, the unified-binary work, and the Typst migration across the days they were done (#523).
- Graduated eight completed plans to symlinks pointing at the PDF reporting, distribution packaging, and CLI guide hubs, and inverted the visualiser performance investigation, which had been graduated backwards (#526).
- De-circularised those hubs now that their plans resolve back to them, and dropped drift-prone counts from the reference docs so they age better (#526).
- Landed a grouped application dependency bump taking `golang.org/x/sys` to 0.46.0 and `modernc.org/sqlite` to 1.52.0 (#524).

## June 8, 2026 - Typst PDF engine & TeX Live removal

- Implemented Typst PDF generation end to end: a renderer, data preparation, `.typ` templates, and source packaging, making Typst the default report engine in place of LaTeX (#522).
- Added the Typst binary toolchain: `scripts/download-typst.sh` fetches per-target release binaries, and the engine embeds into the `velocity` binary by default, skipped in CI for build speed (#522).
- Removed the legacy TeX stack: the `internal/report/tex` templates and scripts, `pylatex`, the minimal-TeX-Live image stage and CI steps, the VSCode LaTeX config, and the now-unused `gonum` dependency (#522).
- Made OpenStreetMap tiles opt-in for the report site map, defaulting to the offline schematic, hardened the tile cache directory, and replaced emoji POI glyphs with vector pictographs for PDF compatibility (#522).
- Added a third-party licences section to the settings page and drove the Typst report paper size from config (#522).
- Closed out the upgrade mechanism: validated the `BackupDatabase` source path against DSN injection, enforced a pure-Go ARM64 build when pcap support is unavailable, and clarified how a pre-0.5.1 binary handles a stray database (#519).

## June 7, 2026 - Versioned upgrade mechanism & appliance hardening

- Implemented the on-device upgrade mechanism: local-binary upgrade checks, release-asset handling, a consistent database backup before each upgrade, and a `retrySleep` seam to make the upgrade retry path testable, with tests for the update-release-JSON module and binary build paths (#519).
- Streamlined command routing and the install function for the unified binary, and added comprehensive tests for the `device` commands and SQL handling (#519).
- Updated the build scripts and Dockerfile for ARM64 binary compilation and added a coverage check for branch-added internal Go files (#519).
- Hardened the appliance runtime defaults, reasserted them after a cleanup pass, and aligned the CLI and operations docs with the unified `velocity` binary (#519).

## June 6, 2026 - Single-binary close-out, serial race & homepage hero

- Enumerated the `velocity-ctl` sudoers grant to safe verbs instead of a wildcard, split the pcap-pulling command packages into the libpcap CI job so `Test Core` builds without headers, shortened the git SHA in the MOTD banners, and repointed moved-command docs plus asset-naming to the unified `velocity` (#519).
- Closed out the single-binary transition: removed the transitional `velocity-ctl` shim (leaving `velocity-report` as the systemd alias) and added a read-only `velocity data sql` inspection subcommand in its place, keeping the `VERSION` marker for the binaries-only handoff (#519).
- Fixed a data race in the radar serial event fanout: `runEventFanout` now holds the read lock across the non-blocking subscriber sends, so an `Unsubscribe` close can no longer race a send on a closed channel (caught by `-race` in `TestSerialPortManager_EventFanout_FullChannel`) (#518).
- Fixed the public homepage hero by vendoring `three.core.js` alongside `three.module.js`: three 0.184.0 re-exports from the sibling core file, so the browser 404'd on it and the point-cloud scene never initialised (#521).
- Shrank the homepage `stack.png` from ~3.4 MB to ~1 MB and extended the `pdf-stack-render` tool with page-border options and output scaling (#521).
- Repaired the GitHub Pages docs deploy by switching it to the repo's `setup-node-pnpm` composite, which installs the pinned `pnpm@10.18.2` instead of a too-new pnpm that needs `node:sqlite` on Node 20.19.0 (#518).
- Hosted the macOS visualiser unit-test bundle in the app so its proto symbols resolve at link time, and switched `test-mac` to `xcodebuild test-without-building` (#518).
- Added a Go runtime pipeline correctness plan (`docs/plans/go-runtime-pipeline-correctness-plan.md`) and logged it in the backlog (#520).

## June 5, 2026 - Single `velocity` binary, versioned upgrades & Tailscale fixes

- Hardened the path after the TLS layer came out: dropped a `CapabilityBoundingSet` that broke `sudo` and Tailscale, drove Tailscale enrolment through `Start` rather than `EditPrefs`, long-polled `/api/tailscale/status`, pointed `tailscale serve` at the actual `--listen` port, and fixed the image build's `qemu-aarch64-static` architecture check and a stale `pigen_work` container left from cleanup (#517).
- Folded `velocity-report`, `velocity-ctl`, and the local-only `sweep` tool into one multi-call `velocity` binary that dispatches on `argv[0]` and a `serve | device | data | report | tune` namespace tree. `velocity-report` survives as the systemd-facing alias, and `velocity-ctl` was kept as a transitional shim forwarding to `velocity device …` (#519).
- Reworked on-device upgrades into a versioned layout under `/opt/velocity-report/versions/<v>/` with `current`/`previous` symlinks and an atomic `renameat2` swap: migrations run on the new binary before the swap, the running build is verified through `GET /api/version`, three versions are retained, and rollback is now a single symlink flip rather than a copy (#519).
- Embedded the deployment config into the binary — tuning defaults, the LiDAR network profile, udev rules, and the Wi-Fi `wpa_supplicant` fallback now ship via `go:embed` and `velocity device install` — then trimmed `python3-serial`, `minicom`, `jq`, and `curl` from the image's apt surface and deleted the vestigial `02-velocity-python` stage (#519).

## June 4, 2026 - nginx & self-signed TLS removal

- Dropped nginx and the first-boot self-signed local CA, serving plain HTTP on `:80`: the Go server now binds `:80` directly as the non-root `velocity` user via `CAP_NET_BIND_SERVICE`, so the first visit to `http://velocity.local` no longer involves a browser warning or a CA install, and the `--listen` default stays on loopback so production opts into LAN exposure deliberately (#517).
- Documented the HTTP-on-`:80` model, the nginx and TLS removal, and the opt-in Tailscale Serve path for browser-trusted HTTPS on the tailnet (#517).
- Taught the MOTD banner to read version and build metadata from the installed binary rather than baking it in at image build, pointed `dev-ssh-audit` at `/app/` instead of the `/` route that 302-redirects, and fixed dead documentation links (#517).

## June 3, 2026 - Radar listen hardening & dependency refresh

- Hardened the radar serial surface: the default `--listen` and `--lidar-listen` moved from wildcard `:PORT` to loopback `127.0.0.1:PORT`, so the API is localhost-only unless the operator opts into LAN exposure with `0.0.0.0:PORT` (#461).
- Landed a grouped application dependency bump (#516).

## June 2, 2026 - Dependency refresh

- Refreshed grouped documentation and application dependencies across ecosystems in two waves (#514, #515).

## June 1, 2026 - Serial configuration UI

- Added the serial port configuration UI across the Go API and Svelte frontend: device discovery via the `/dev/velocity-radar` udev symlink, sensor-model selection, and a serial test/activation flow under `/api/serial/*`, and retired the legacy `velocity-update` redirect stub in the same pass (#290).

## May 21, 2026 - Radiance-field scene-model review

- Added a decision note reviewing radiance-field methods (NeRF, 3D Gaussian splatting, neural occupancy and SDF fields, dynamic scene fields) against the planned classical L7 plus vector-scene-map path, concluding the classical route wins on the dimensions that matter here — privacy, explainability, deterministic replay, and the Raspberry Pi compute envelope (#512).

## May 17, 2026 - Homepage redesign & point cloud

- Redesigned the public homepage around an instrument aesthetic with LiDAR above the fold and a live point-cloud hero, on a standalone layout that owns its own visual shell without disturbing the guides and community pages (#510).
- Followed up with point-cloud rendering tweaks (#511).

## May 11, 2026 - Release prep, InlineSvgChart fix & `.vrlog` cleanup

- Prepared the 0.5.1 release docs: refreshed the changelog and versioning section, recorded the pre18 release notes, and sorted the backlog's completed entries (#504).
- Hardened `InlineSvgChart` by replacing raw SVG injection with same-origin `<img src>` loading, added theme-aware rendering, moved the repo baseline to Node 20.19.0, and added the offline-docs CI workflow (#505).
- Removed the legacy JSON `.vrlog` path across the Go server and the macOS visualiser: the `Track` speed-field unmarshalling fallback and the visualiser's legacy-JSON warning state are both gone (#506).
- Added the "Butterfly Net" v2 UI design — an interactive prototype, a design plan, and public-site passthrough for the design assets — and tidied the contributing guidance alongside it (#507).
- Fleshed out the v0.5.1 hardening and image-consolidation backlog and plans, adding the single-binary image-consolidation plan and swapping the retired `.vrlog` shim item for the serial-config-UI scope (#508).

## May 7, 2026 - Release pre18 & macOS CI fix

- Cut the `0.5.1-pre18` release: refreshed download links and asset checksums in `release.json` and the Raspberry Pi Imager catalogue (#501).
- Fixed the failing macOS CI run by updating `uuid` to 11.1.1 with matching pnpm overrides, and excluded `mac-ci.yml` from pull-request path triggers so it stops firing on unrelated changes (#502).

## May 6, 2026 - Docs hygiene, LiDAR docs consolidation & Tailscale web UI

- Fixed the remaining plan-hygiene blockers by adding missing canonical links to the CLI and config restructure plans and expanding the offline docs site notes with clearer ownership and build-boundary details (#499).
- Added the `style-fix` skill for repeatable STYLE.md checks and safe auto-fixes, including ignore markers for whole-file skips and length-only skips where mechanical cleanup would do more harm than good (#499).
- Corrected spelling and punctuation across multiple docs and refreshed this devlog so recently merged work no longer carries stale branch tags (#499).
- Consolidated the LiDAR docs cluster: pruned `docs/lidar/LIDAR.md` to a true index, symlinked `lidar-sidecar-overview.md` and `lidar-data-layer-model.md` to `LIDAR_ARCHITECTURE.md`, trimmed `foreground-tracking.md` to design rationale only, moved the ten-layer mermaid diagram into `ARCHITECTURE.md` as its canonical home, and trimmed the LiDAR sections of `ARCHITECTURE.md` to a summary plus link (#499).
- Moved the tuning glossary into `tuning-guide.md`, consolidated the UI documentation cluster (palette, proto-contract, plan moves), and fixed broken relative links in the moved performance investigation doc (#499).
- Updated `data/maths/MATHS.md` to reflect the engine-selectable config schema so the maths reference stops pointing at retired engine-specific knobs (#499).
- Landed the Tailscale work: web UI enable/disable for opt-in tailnet enrolment across Go and Svelte, plus image build and file-based config support, and bumped the version to `0.5.1-pre18` to mark the cut (#472).

## May 5, 2026 - Version ownership cleanup, release prep & docs bookkeeping

- Consolidated version ownership so routine version bumps now touch only `Makefile` and the macOS Xcode project; `set-version.sh --all` now follows that rule directly, and runtime-irrelevant package manifests are pinned to `0.0.0` instead of pretending to be canonical (#497).
- Removed the redundant `app-web-version` meta tag, marked the version-bump consolidation plan complete, and reflected the new single-source versioning approach in the plan and backlog docs (#497).
- Fixed the image build path to restore executable bits on staged binaries before packaging, then refreshed the release JSON metadata for the `0.5.1-pre17` cut (#496, #498).
- Graduated the version-bump consolidation plan into the asset naming architecture doc with explicit workflow and operational notes, and added completed backlog entries for the public protractor tool and embedded offline docs milestone (#499).
- Added Open Graph tags across site pages so shared links carry sane titles and previews instead of the usual social-media guesswork (#500).

## May 4, 2026 - CI build logic cleanup & dependency refresh

- Moved more release and image build logic out of GitHub Actions YAML and into repo-owned scripts and Make targets, added `ensure-dev-web-build`, and rewrote the offline docs stub page so the missing-build state is explained in plain English (#491).
- Fixed `pcap-analyse` to read the correct `TrackState`, `StartUnixNanos`, and `EndUnixNanos` fields, then added focused coverage so the mapping stays fixed (#492).
- Moved the image build container to `golang:1.26-bookworm`, removed the pinned pnpm version from setup, and stripped redundant workflow `chmod` steps that were doing busywork in public (#492, #496).
- Switched Dependabot back to pnpm for the workspace, adjusted its schedule and paths, and landed grouped documentation and application dependency updates in two waves (#489, #493, #494, #495).
- Consolidated version bumps to the Makefile and Xcode project, pinned unused package manifests to `0.0.0`, and cut the `0.5.1-pre17` release metadata (#497, #498).

## May 2, 2026 - Deterministic DHCP image behaviour

- Made DHCP configuration deterministic in the image build, bound the profile to `eth0`, and added first-boot networking checks so the appliance stops improvising with its own network stack (#488).
- Updated image-side troubleshooting to disable `NetworkManager-wait-online.service` and explain the DHCP failure mode more clearly in the README and image docs (#488).
- Enriched build and image metadata with version and git SHA information, added new release asset management/build targets, and cleaned up the stub HTML shown when frontend or docs assets are missing (#491).
- Added frontend asset request handling and corresponding tests, expanded dashboard test coverage, refined `Track` colour/class handling, and refreshed release metadata and download links for the `0.5.1-pre14` cut (#490).

## April 30, 2026 - PDF pipeline simplification & proto CI work

- Removed the Python PDF generator from current documentation, CI setup, Make targets, VS Code settings, and production expectations; the Go report pipeline is now the only supported PDF path, while Python stays for developer tooling only (#485).
- Updated report-pipeline docs, TeX dependency paths, and related scripts so the repository stops talking about removed PDF machinery as if it might still stroll back in later (#485).
- Refactored the proto pipeline so CI calls repo-owned scripts and Make targets instead of growing more shell inside workflow YAML, with better cache-key derivation, prerequisite checks, and clearer DEBUGGING notes (#486).
- {dd/ci/add-swift} Continued the Swift security and CI branch: added CodeQL coverage for Swift, fixed action pinning and native ARM Homebrew handling in proto setup, bumped grpc-swift-protobuf codegen, and polished the macOS About view.

## April 29, 2026 - Offline docs, report polish & dependency refresh

- Landed embedded offline docs Phase 1: added the offline docs site structure and styling, embedded docsite server and validation, `/docs` routing, `offlineDocsUrl` logic with tests, and a Docs navigation entry in the web UI (#480).
- Brought the offline docs experience closer to the main site with a Markdown-rendered index, KaTeX maths support, copied Mermaid ESM chunks, tree-view sidebar navigation, breadcrumb top bar, dark-mode toggle, sticky sidebar header, and accessibility fixes around the theme control (#480).
- Removed `tools/pdf-generator` and the rest of its paperwork trail across docs, CI, scripts, config, and versioning, and updated report-pipeline naming so the current Go implementation is described as the real thing rather than a rumour (#485).
- Continued report-pipeline polish: default paper size is now US Letter, table typography moved to Atkinson Hyperlegible Mono, long tables switched to flow-table handling, and histogram/time-series layouts got clearer labelling, legend placement, spacing, and tests (#481).
- Refreshed cross-ecosystem dependencies and bumped `postcss` in the docs site pipeline (#462, #476).

## April 28, 2026 - Image build repair & release workflow guardrails

- Fixed the failing image build path and tightened GitHub release creation checks so the workflow verifies tag existence before publishing instead of discovering reality at the most inconvenient moment (#479).
- Added CodeQL workflow coverage, repaired invalid action SHA pins, and addressed a sanitisation finding while the security tooling was already on the operating table.
- Continued the offline docs follow-up work from PR #480: added embed-stub steps to Go CI, made offline docs linting non-mutating, tracked review follow-ups in the backlog and plan docs, and suppressed stale link-check reports for runtime-resolved hrefs (#480).
- Kept pushing the report pipeline forward with p98/max reference lines, better histogram and caption layout, balanced long-table rendering, improved TeX spacing, and review-fix passes across Go, TeX, and docs (#481).

## April 27, 2026 - Deployment docs & packaging model cleanup

- Clarified setup and TLS guidance for the `0.5.1-pre8` release, refreshed release metadata and checksums, and removed stale nginx and local-CA assumptions from the docs where the system had already moved on (#477).
- Reworked deployment and packaging docs around the versioned single-binary install model, tighter sudo boundaries, clearer CLI architecture, and the current greenfield image assumptions (#478).
- Shipped the first full embedded offline docs path in code as well as prose: Go server support, docs site structure and styles, `offlineDocsUrl` logic with tests, Docs navigation wiring, and build/management scripts (#480).
- Fixed workflow brittleness by building the web frontend before cross-compiling release binaries, verifying tag existence before GitHub release creation, and bumping to `0.5.1-pre10` (#478, #479).
- Continued PDF/report design cleanup with updated table styling docs, Atkinson Hyperlegible Mono italic font assets, template refactors, and a Poppler-based PDF comparison script (#481).

## April 26, 2026 - Chart migration lands

- Completed the chart migration and Python deprecation pass: the Go chart and report pipeline is the production path, while Python is documented as developer tooling rather than deployment runtime (#455).
- Improved report and chart correctness around sparse hourly gaps, expanded charts, DST-safe date ranges, unit/source validation, cosine metadata, and source validation on chart endpoints (#455).
- Added key-metrics tables and further chart polish to the LaTeX report output, including adaptive ticks, histogram percentages, clearer period labels, tighter captions, and better spacing (#455).
- Continued the docs cleanup around embedded offline docs, setup privacy wording, TLS state, and release metadata so the narrative around the platform matches the system that actually exists (#477).

## April 25, 2026 - Pure-Go TeX study reaches a no-go verdict

- Added a feasibility study for replacing `xelatex` with the pure-Go `star-tex` engine, then recorded the practical answer: no, because the current templates rely on LaTeX and XeTeX features that `star-tex` does not implement (#475).
- Added a canonical PDF report design document and kept refining the Go/TeX report output with key-metrics tables, clearer labels, better row colours, improved margins, and more legible comparison-chart wording (#455).
- Hardened report and chart behaviour with DST-safe date ranges, cosine metadata derived from the requested ranges, validation of units and sources, source validation on chart endpoints, and stale preview clearing after SVG load failures (#455).
- Added time-gap detection and expanded-chart handling so sparse hourly data keeps linear timestamp spacing, with matching Go and web changes and tests (#455).
- Added helper scripts for generating test reports from the API and comparing generated PDFs with Poppler tools (#455).

## April 21, 2026 - Browser protractor tool added

- Added a minimal web protractor under `/tools/protractor` that reports alignment angle and colour-grades the acceptable 15-25 degree range (#470).

## April 20, 2026 - Pinout corrections & release automation planning

- Corrected the OPS700 harness diagrams to female-face views, updated the captions to match cable-end orientation, and trimmed the pinout layouts so the diagrams reflect the thing a human would actually see in hand (#473).
- Shared SVG-to-PNG rasterisation across the render tools, moved connector pinout generation into the `tools/` structure, and fixed `make wiring` for current WireViz flag behaviour (#473).
- Added the version-bump consolidation plan and linked it into backlog planning as follow-on work rather than leaving the idea to roam the repo unsupervised (#473).
- Expanded the release JSON automation plan with resolved open questions, a phased implementation checklist, security review guidance, and clearer `DEBUGGING.md` migration notes about flag ordering and `dialout` setup (#474).

## April 19, 2026 - Homepage copy, overlays & release tooling

- Refreshed the homepage hero, headline, CTA, and FAQ copy so the front page explains the project more directly and puts the sample report front and centre (#471).
- Added hardware-stack overlay diagrams for the setup guide and compressed the source photography so the diagrams explain the enclosure contents without hauling around unnecessary image bulk (#471).
- Introduced `scripts/update-release-json.py` with cached asset hashing, `.img.xz` hash extraction, validation mode, and a matching Make target for updating release metadata (#471).
- Refreshed `release.json` and the Raspberry Pi Imager JSON for the next prerelease cut and fixed a YAML parse warning in the action-pin workflow (#471).

## April 18, 2026 - Release metadata cleanup & Tailscale branch work

- Reworked release discovery around `releases.json` as the single source of truth, with per-asset version, URL, and SHA metadata instead of a channel-level grab bag that needed too much interpretation (#469).
- Taught `velocity-ctl upgrade` to consume that metadata directly, verify per-asset SHA-256 hashes before install, and handle prerelease ordering and fallback rules more sanely (#469).
- Added Tailscale to the base image and boot-partition configuration path so operators can opt into `*.ts.net` HTTPS for the web UI without local certificate wrangling (#472).

## April 17, 2026 - Supply-chain hardening & documentation trust fixes

- Added SHA verification to `velocity-ctl upgrade`: binary download is verified against the per-asset `sha256` published in `https://velocity.report/release.json` before install. Hash mismatch aborts; an empty `sha256` entry (older release metadata) warns and continues. Resolves the open TODO in `internal/ctl/manager.go` (#469).
- Pinned all GitHub Actions `uses:` lines across 13 CI workflow files from version tags to commit SHAs (`@<sha> # vX.Y.Z`). Priority order: `softprops/action-gh-release`, `peter-evans/create-pull-request`, `codecov/codecov-action`, then all remaining third-party and first-party actions (#469).
- Replaced CA certificate trust instructions in the setup guide with browser-native exception steps (Chrome/Edge and Firefox/Safari); removed from both step 3 and the troubleshooting table. The CA endpoint remains in nginx but is no longer the advertised path (#469).
- Promoted the Tailscale section from "optional" to "recommended for clean HTTPS"; added explanation that `*.ts.net` provides a valid Let's Encrypt certificate. Dashboard URL updated to `https://<hostname>.<tailnet>.ts.net` form (#469).
- Ran a security review (Malory) of Trivy, Axios, and OpenSSF supply-chain attack surfaces; resolved all three critical findings in this session (#469).

## April 16, 2026 - Setup guide polish & responsive tables

- Added responsive table styling with striped row backgrounds for the docs site (#445).
- Standardised title casing and improved table formatting in the setup guide (#445).
- Removed unused guide images (`guide-aim-sutro`, `guide-angel`) and added `link-ignore` annotations for image paths resolved at build time (#445).
- Bumped version to 0.5.1-pre5 (#445).

## April 15, 2026 - Rack geometry, isometric drawings & assembly prose

- Added fastener hole geometry to the rack model: crossbar, brace, and pipe connection definitions in `rack.json` (#445).
- Generated isometric BOM drawing with title block and combined drawing sheet (#445).
- Produced aiming and cosine-angle SVG overlays via `draw_overlays.py`; added matching guide images (#445).
- Rewrote T-frame assembly section of the setup guide for clarity and correct step ordering (#445).
- Updated cost estimates, deployment options, and cable-length references in the setup guide (#445).
- Refined homepage content and descriptions; updated gradient border styles for light and dark modes (#445).
- Added Makefile targets for wiring diagram generation; added `pyyaml` dependency (#445).
- Addressed Copilot PR review comments across docs, web, and Python code (#445).

## April 14, 2026 - Rack-mount drawing tools & docs site improvements

- Created rack-mount engineering drawing tools: `draw_rack.py`, `draw_overlays.py`, and `model.py` with `drawsvg` dependency (#445).
- Added Eleventy table-of-contents functionality with content preamble and body filters (#445).
- Switched docs site colour scheme to emerald for buttons, links, footer, and section titles (#445).
- Added guide hero image and updated setup guide with engineering drawing references (#445).
- Added `cheerio` dependency for HTML manipulation in Eleventy build (#445).
- Added Makefile targets for diagram installation, rendering, and stale docs-server cleanup (#445).
- Updated setup guide parts list and build overview formatting; removed redundant sections (#445).

## April 13, 2026 - RPi image CI fix & setup guide overhaul begins

- Fixed Raspberry Pi image CI by installing `qemu-user-static` before ARM64 builds; deduplicated QEMU setup action (#467).
- Began setup guide overhaul: added hardware photos, updated prerequisites, corrected dates and content (#445).
- Moved radar wiring documentation to a dedicated directory; added connector pinout SVG generation script (#445).
- Generated DE-9 and M12 connector pinout SVGs with connection parsing logic (#445).
- Updated homepage with Q&A section, enhanced inline code styling, and improved header navigation links (#445).
- Added PoE HAT details and mounting-angle measurement guidance to the setup guide (#445).

## April 12, 2026 - LiDAR docs restructure, directory indices & STYLE compliance

- Deprecated Python matplotlib chart rendering and updated STYLE.md guidelines (#463).
- Restructured `docs/lidar/` as a directory index: consolidated LIDAR.md and LIDAR_ARCHITECTURE.md, added layer summary and implementation status tables (#463).
- Created directory index files for `data/` subdirectories (DATA.md, DATA_STRUCTURES.md, EXPERIMENTS.md, EXPLORE.md, MATHS.md) with README symlinks (#463).
- Added PLATFORM.md, VISUALISER.md, and DOCS.md as documentation hub indices. Enhanced CONFIG.md with primary consumer references and CI parity check (#463).
- Expanded coding standards with file naming conventions. Fixed stale references: OPS243-C → OPS243-A, L8/L9 status, monitor → server (#463).
- Banned compilable code blocks and Author/Authors metadata in design documents. Updated DESIGN.md for chart rendering: matplotlib deprecation, SVG-first architecture, shared abstractions (#465).
- Stripped version-number references from 16 documentation files to make docs timeless. Improved LiDAR maths documentation clarity across classification and taxonomy files (#465).
- Integrated ascfix Markdown formatter into CI, then removed it: corrupts hand-crafted ASCII art. Documented findings as an operations note (#464).
- Removed compilable code blocks from 30 plan files and 8 hub docs: replaced Go, SQL, JSON, YAML, Protobuf, and other fenced blocks with field tables and prose. Banned Author/Authors metadata and added metadata audit to docs-release-prep (#465).
- Fixed tag-triggered release asset builds and image uploads in CI (#466).
- Tightened pnpm override ranges and added node engines constraint (#462).

## April 11, 2026 - README refresh, dependency fixes & documentation polish

- Graduated completed plans: API endpoint specifications, width-check and update-PR-description skills, and release-prep skill documentation (#460).
- Refreshed README.md: expanded alpha-software warning with security notes, broadened audience description, simplified developer commands, and added key-documents table (#463).
- Created MAGIC_NUMBERS.md documenting project-wide numeric constants. Added `check-backtick-paths.py` to detect stale file-path references in Markdown prose (#463).
- Standardised British English spelling (neighbour, metre) and heading capitalisation across documentation. Fixed link formatting in multiple files (#463).
- Added width-check skill for advisory prose line-width validation and update-PR-description skill for generating structured PR descriptions from branch diffs (#463).
- Added release-prep skill documentation. Updated documentation structure guidelines to prefer prose and tables over pre-built code blocks (#460).
- Added detailed API endpoint specifications for serial configuration and testing. Refactored transit deduplication documentation (#460).
- Fixed 19 dependabot alerts across Go, npm, and Python ecosystems (#462).

## April 10, 2026 - documentation standardisation & plan graduation

- Graduated 7 plans to symlinks, consolidated fragmented documentation, removed deprecated files, and fixed 23 dead links after the visualiser directory move (#459).
- Completed the opening-paragraph audit: added narrative introductions to 58 hub docs across LiDAR architecture, platform operations, radar, and UI directories (#459).
- Created the plan-graduation skill with slim hub-doc template and two-PR graduation rule. Added the docs-release-prep skill (#459, #460).
- Resolved the lidar-schema-robustness plan as complete. Added status lines to 7 plans, consolidated webserver-tuning into the fixit plan (#459).
- Added CI docs link health check and refined linting scripts. Auto-built the documentation site when missing from the RPi image (#459).
- Restricted default listen addresses to localhost. Reworked the `/command` allowlist into an advisory catalogue after confirming the OPS24x API command set is config/query-only and non-destructive (no firmware-flash command per AN-010-Z): the endpoint now forwards any command to the sensor, logging a warning for commands outside the documented catalogue rather than rejecting them. The catalogue (`internal/radar/commands.go`, now code+description and completed against AN-010-Z) is exposed read-only via `GET /api/commands` to back a future dashboard command dropdown (#461).

## April 9, 2026 - asset naming, Go report generation & RPi image security

- Implemented versioned asset naming across Go binaries, CI workflows, macOS builds, and pi-gen image output (#457). Binary names now include version and architecture suffixes.
- Separated the velocity service account from the login user in the RPi image (#458): dedicated system account for the service, restricted sudoers to the login user, fixed SSH key ownership.
- Added device entries and custom favicon for the docs site RPi Imager catalogue (#456). Expanded the OS list JSON with Raspberry Pi 5, 4, and 3 support.
- Switched CI Go setup to `go-version-file` for automatic version tracking (#451). Backed out the SonarCloud integration.
- Added the documentation standardisation audit plan with gate definitions and checklist (#452).
- Added Vite config symlink for worktree support (#454).
- Built Go-native SVG chart rendering: histogram and time-series packages with Atkinson Hyperlegible font support, LaTeX template rendering, and a PDF report generation subcommand with ZIP output (#455).
- Began the documentation audit: fixed 8 gate violations, graduated 15 complete plans to symlinks, consolidated the visualiser app directory, reclassified PCAP design docs as plans, and added config cross-references (#459).

## April 8, 2026 - map editor, homepage build & tooling

- Added confirmation modal for mode switching in `MapEditorInteractive` to prevent accidental data loss. Combined angle stepper and remove button into a single row for custom SVG mode (#436).
- Enhanced save error handling on the site settings page with user-visible feedback on failure (#436).
- Groomed the backlog (v0.5.1–v0.5.4): migrated Capabilities API to v0.5.3, Classification enum split to v0.5.8, Clock abstraction to v0.5.2, and PCAP motion tooling to v0.5.6. Reordered fix-it phases within v0.5.4 (#436).
- Fixed `orderedEndpoints` to use stable declaration order instead of shuffling (#436).
- Improved LaTeX log fatal error detection: normalised prefixed lines (`!` and `LaTeX Error:`) and added corresponding test cases (#436).
- Standardised `effective_start` timestamp format in migration 000034 and `schema.sql` (#436).
- Refactored `sync-schema.sh` to use awk for generating `INSERT OR IGNORE` fixture statements (#436).
- Refactored `dev-ssh.sh` to cleanly separate SSH options from remote command arguments (#436).
- Tightened link-checker skip predicate to match only `image/stage-*/files/` paths (#436).
- Updated `devlog-update` skill procedure: added `git fetch` step, gap-fill logic for incomplete existing entries, and broadened amend rules (#436).
- Restructured the homepage download section with per-platform SHA-256 hashes, RPi Imager JSON references, and Docker-based ARM64 cross-compilation (#453).
- Merged the error surface voice audit across Go, JavaScript, Python, and shell scripts (#449).

## April 7, 2026 - RPi image hardening, web map editor & backlog grooming

- Hardened the RPi image cleanup script: `dpkg --purge` of the camera stack triggered an `apt-get autoremove` cascade that removed `librsvg2-bin`, `network-manager`, and other runtime packages. Added `apt-mark manual` protection and a post-purge verification step (#436).
- Added build metadata stamping to the RPi image: `VR_VERSION`, `VR_BUILD_TIME`, and `VR_GIT_SHA` written to `/etc/velocity-report-build` and displayed in the MOTD login banners (#436).
- Refined MOTD Unicode art banners and added `check-quarter-blocks.sh` lint script to detect block-element characters that render incorrectly in some terminal fonts (#436).
- Enhanced TLS certificate generation (`velocity-generate-tls.sh`): improved directory permissions, certificate validity checks, and error diagnostics (#436).
- Built radar SVG map position override feature end-to-end: new `radar_svg_x`/`radar_svg_y` columns in the `site` table (migration 000033), Go model updates, Svelte `MapEditorInteractive` component with SVG upload and draggable position controls, PDF generator support, and clamping logic with 5% border margins (#436).
- Added `map-svg.ts` utility module with SVG rendering types and helper functions for the web frontend (#436).
- Refactored the web settings page layout for better responsiveness. Simplified `Field` component defaults and improved number input styling (#436).
- Added `isDateRangeStale` freshness check for `reportSettings` localStorage: dates older than 18 hours fall back to the default 14-day range. Non-date settings always restore. Shared helper in `$lib/reportSettings.ts` with Jest test coverage (#436).
- Groomed the backlog: split v0.5.6, added paper-implementation gap remediation items (P1/P2/P3), priority-sorted all milestones, performed domain analysis, moved items between milestones, and split v0.6.0 into Deployment & Packaging and macOS Local Server (#436).
- Added four new plan documents: [domain tag vocabulary](plans/domain-tag-vocabulary-plan.md), [asset naming standardisation](plans/asset-naming-plan.md), [PCAP motion detection and scene split](plans/pcap-motion-detection-and-split-plan.md), and [macOS local server](plans/macos-local-server-plan.md) (#436).
- Added `dev-ssh` and `dev-ssh-audit.sh` scripts for SSH access and remote RPi health checks (#436).
- Fixed LaTeX escaping in `ParameterTableBuilder` for monospace rendering. Fixed `fromDatetimeLocalToUnixSeconds` to manually parse datetime strings for consistent cross-browser behaviour (#436).
- Added `devlog-update` skill for automated development log updates from git history (#436).
- Improved LaTeX log error detection: added explicit fatal line signatures and refined pattern matching (#436).
- Hardened shell scripts: refactored `sync-schema.sh` migration logic to use null-delimited reads, enhanced `dev-ssh.sh` host key management with explicit confirmation, and switched `dev-ssh-audit.sh` to UTC timestamps (#436).
- Refined Markdown link checker to skip `image/stage-*/files/` directories. Clarified `velocity-deploy` removal in the remote host upgrade runbook (#436).
- Installed `fonts-noto-color-emoji` in the RPi image for map label rendering. Updated sudoers configuration for specific commands (#436).
- Added UTC timestamp guidelines to `STYLE.md` and `coding-standards.md` (#436).

## April 6, 2026 - Claude code init, security fixes, paper gap analysis & CI linting

- Initialised Claude Code configuration (#447): [CLAUDE.md](../CLAUDE.md), [.claude/agents/](../.claude/agents) (7 personas), [.claude/skills/](../.claude/skills) (8 workflow slash commands), and shared knowledge modules in [.github/knowledge/](../.github/knowledge).
- Updated vulnerable dependencies across Go, npm, and Python ecosystems (#441). Fixed a dev-mode path traversal vulnerability by normalising `requestedPath` before joining with `buildDir`.
- Added paper-vs-implementation gap analysis ([data/maths/paper-implementation-gap-analysis.md](../data/maths/paper-implementation-gap-analysis.md)): reviewed 24 papers across 11 subsystems, identified 35 gaps with P1/P2/P3 priority tiers (#446).
- Added `download-papers.py` script with DOI/URL resolution, SSRF guards, and dry-run mode (#446).
- Merged Go clock abstraction plan (#428): `timeutil.Clock` interface for pipeline timing, replay pacing, and benchmark instrumentation.
- Added Markdown CI workflows: `check-md-links` for dead link detection, `check-backtick-paths.py` for stale file path references, `public_html` CI for docs site linting. Wired into `make lint-docs` (#447).
- Refreshed [README.md](README.md) with sample PDF report and visualiser demo images (#443, #444). Added `STYLE.md` with writing conventions and British English guidelines.
- Added wiring diagrams and YAML configuration for OPS243-A radar sensor (`docs/hw/`) (#445).
- Updated agent preparedness documentation, added `fix-links` skill, and expanded coding standards with configuration and media guidelines (#443, #447).
- Added `age-color` terminal script and new Makefile log convenience targets.

## April 5, 2026 - README & ASCII art refresh

- Refreshed [README.md](README.md) structure: reorganised sections, updated project description and feature list (#443).
- Iterated on ASCII art header designs across documentation files (#443).

## April 1, 2026 - ASCII art update

- Updated ASCII art crosswalk and banner designs in documentation (#442).

## March 31, 2026 - voice quality audit & error message rewrite

- Completed the error surface voice audit across all subsystems: rewrote user-facing error messages in Go HTTP handlers, [internal/cmd/server](../internal/cmd/server) CLI, Python PDF tools, and Svelte web frontend to match the project voice (concise, helpful, no blame, diagnostic hints where useful) (#431, #449).
- Rewrote shell script messages and marked the voice audit plan complete (#431, #449).
- Centralised API error handling in the web frontend for consistent error message display (#436).
- Fixed TLS certificate generation to persist the CA across server certificate renewals, preventing trust breakage when the server cert is regenerated (#436).
- Updated MOTD ASCII art drafts for the RPi login banners (#436).
- Fixed missing `.invalidConfiguration` case in Swift `APIError` switch statements (#440).

## March 30, 2026 - deprecation signalling, autoTuner fix & homepage

- Fixed macOS `APIError` handling in `LabelAPIClientTests` (#440).
- Updated project-wide copy across JS, Go, and Python surfaces (#431): refreshed deprecation messages for legacy deployment targets.
- Fixed a race condition in `AutoTuner` completion persistence: reordered operations to prevent concurrent write conflicts (#436).
- Added Raspberry Pi image section to the homepage with download link and SHA256 checksum (commented out pending first release) (#436).

## March 29, 2026 - RPi image: web build, HTTPS & TLS, image cleanup

- Added Raspberry Pi download section to the homepage (#437): `release.json` data source, per-platform SHA256 hashes, clipboard fallback for copy buttons.
- Fixed homepage mobile layout (#439): resolved light/dark theming split and download card spacing.
- Refined Terry agent coaching workshop documentation (#438).
- Fixed "Web Frontend Not Built" in the RPi image: whitelisted [web/build/](../web/build) in `.dockerignore` and added a web build step before Go compilation in `build-image.sh`. <!-- link-ignore --> (#436)
- Moved TLS termination from Go server to nginx reverse proxy. Go server stays on `:8080` (plain HTTP), nginx handles HTTPS on port 443 (#436).
- Added first-boot TLS certificate generation (`velocity-generate-tls.sh`): per-device ECDSA P-256 local CA (10-year) and server cert (825-day). Idempotent, regenerates on expiry (#436).
- Exposed CA certificate at `GET /ca.crt` via nginx for browser trust installation (#436).
- Installed root-level project documents into the image for on-device reference (#436).
- Documented the TLS strategy in [docs/platform/operations/tls-local-certificates.md](platform/operations/tls-local-certificates.md) (#436).

## March 28, 2026 - RPi image: build pipeline, flash target & first-boot polish

- Consolidated Go version information printing into a single function across [internal/cmd/server](../internal/cmd/server) and [internal/cmd/device](../internal/cmd/device) (#436).
- Updated build process and documentation for RPi images: clarified `build-image.sh` sections and improved error handling (#436).
- Added timestamps to image filenames to prevent collisions during rebuilds (#436).
- Added installation step for `tuning.defaults.json` in the pi-gen run script so the service starts with a valid config (#436).
- Added `make flash-image` target for flashing images to SD card on macOS, with device detection and safety prompts (#436).
- Added `DISABLE_FIRST_BOOT_USER_RENAME` to pi-gen config to prevent the boot wizard overwriting the `velocity` user (#436).
- Refactored systemd service management to prevent crash-loop on boot: the service now waits for the database directory and starts cleanly even on first boot before the sensor is connected (#436).
- Added login MOTD banners warning about the default password and providing help commands (`velocity-ctl status`, `velocity-ctl upgrade`) (#436).
- Suppressed the first-boot user-creation wizard and cancelled pending user renames during image export (#436).
- Moved `data/align/` to its proper location within the reference data structure (#436).
- Enhanced the image build with reference data installation (alignment CSVs, sample VRLOGs) so the appliance ships with test datasets (#436).
- Added `make clean-images` to remove old `.img`, `.zip`, and older `.xz`/`.sha256`/`.info` files from the deploy directory, keeping only the latest build. Recovered 18 GB → 529 MB on first run (#436).
- Updated `.dockerignore` to clarify web frontend asset inclusion (#436).
- Moved [TENETS.md](../TENETS.md) to the repository root and updated all cross-references in [.github/copilot-instructions.md](../.github/copilot-instructions.md), agent files, and documentation (#436).

## March 27, 2026 - RPi image: Pi-gen integration & ARM64 cross-compilation

- Added Raspberry Pi image build support using pi-gen (bookworm-arm64 fork). The `image/` directory now contains pi-gen stage definitions, configuration, and the unified `build-image.sh` script (#436).
- Added `Dockerfile.build` for ARM64 cross-compilation of the Go binary on macOS (Apple Silicon). The Docker build compiles `velocity-report` and `velocity-ctl` for `linux/arm64` with CGO enabled via `zig cc` (#436).
- Added `.dockerignore` to control the Docker build context: whitelists only Go sources, web build output, static assets, and build scripts (#436).
- Improved image extraction and compression logic: the output pipeline now produces `.img.xz` with SHA-256 checksum and `.info` metadata files (#436).

## March 26, 2026 - velocity-ctl, setup guide & architecture docs

- Created `velocity-ctl` as the on-device management tool, replacing the remote `velocity-deploy` script. Implements `backup`, `rollback`, `status`, and `upgrade` subcommands. Designed to run _on_ the Raspberry Pi, not from a development machine (#436).
- Removed `velocity-update` shell script (superseded by `velocity-ctl upgrade`) (#436).
- Removed `velocity-deploy` references from CI workflows, Makefile, and documentation. Removed SSH configuration parsing code and associated tests from the Go codebase (#436).
- Updated CI ARM64 binary build process to produce both `velocity-report` and `velocity-ctl` (#436).
- Added setup guide publication plan with content placeholders and release checklist (#436).
- Added `os-list.json` for the Raspberry Pi Imager catalogue so users can flash images from the official Imager tool (#436).
- Enhanced the setup guide with backup and restore instructions for sensor data (#436).
- Updated architecture documents for ground plane extraction and GPS parsing (#436).

## March 25, 2026 - documentation refresh & RPi image planning

- Added [data/QUESTIONS.md](../data/QUESTIONS.md) with open research questions about hourly comparison data and speed distributions (#433).
- Refactored [README.md](README.md) structure and added [COMMANDS.md](../COMMANDS.md) with make targets reference and ASCII art headers (#434).
- Split the Raspberry Pi image plan into phased delivery (Phase 1 bootable image, Phase 2 OTA, Phase 3 fleet management) (#435).
- Added macOS code-signing and notarisation to the CI workflow and Makefile (#425): `codesign --deep`, `notarytool`, and `stapler`.
- Fixed notarisation auth handling for macOS 13+ keychain profiles and removed the unreliable `spctl` DMG check (#425).
- Added a pre-build guard to verify the visualiser `.app` bundle exists before attempting DMG creation (#425).
- Enhanced notarisation error handling with detailed logging and keychain path resolution (#425).

## March 24, 2026 - config consolidation, ERD refresh & workflow docs

- Consolidated LiDAR immutable/run-config plumbing across radar startup, storage, replay-case management, and backfill tooling (#429).
- Extracted testable helper paths in [internal/cmd/server](../internal/cmd/server) and added focused coverage for the run-config backfill tool (#429).
- Updated immutable run-config and replay-case operations docs, and replaced older scene-management implementation notes with the newer asset plan (#429).
- Switched plan-hygiene reporting to an advisory workflow in CI rather than a hard PR gate (#429).
- Refreshed schema visualisation tooling: added configurable SQLite ERD grouping, updated graph scripts, and regenerated `SCHEMA.svg` (#432).
- Expanded `MATRIX.md` and refreshed the Matrix Tracer agent around the live schema/documentation inventory workflow (#432).
- Tightened Go/API hygiene around JSON tags, dropped-error handling, and build metadata (#421).
- Added fresh plan docs for binary-size reduction, Go structural hygiene, and the dual-tool Claude/Copilot agent workflow architecture (#421, #432).
- Released 0.5.0 🌞 "Sunny Southeast": version set across Go/Python/web/macOS with expanded CHANGELOG (#297).
- {copilot/update-capabilities-check-radar-mode} Refactored the capabilities API to a multi-sensor named-object format, ultimately carried forward in #547 after #430 was superseded: per-sensor named objects in Go handlers and web stores.
- {copilot/update-capabilities-check-radar-mode} Added smart LiDAR capability polling: the web frontend retries startup failures, stops polling after a successful radar-only response, and keeps polling when LiDAR is present.
- {copilot/update-capabilities-check-radar-mode} Added a multi-sensor capabilities plan document and marked the response-shape/navigation work complete after implementation; LiDAR ready/error lifecycle wiring remains follow-up work.
- {copilot/update-capabilities-check-radar-mode} Expanded web test coverage with a multi-sensor `lidarState` derived store test.

## March 23, 2026 - capabilities gating, stream fix & host upgrade runbook

- Added capabilities API surfaces in radar/admin Go endpoints and covered them with new API and radar tests (#427).
- Wired capabilities through the web client and stores so unfinished LiDAR UI paths can stay hidden until backend support is present (#427).
- Updated the web frontend consolidation plan to reflect the new capability-gating approach (#427).
- Fixed `StreamFrames` hang conditions in replay endpoints to prevent stuck frame streams (#426).
- Added a remote-host upgrade runbook documenting safer deployment and upgrade flow for field systems (#424).

## March 22, 2026 - schema hardening, track cleanup & canonical docs

- Hardened the pre-v0.5.0 schema path across SQL and Go: added replay annotation/evaluation integrity migration work and strengthened label-path coverage (#420).
- Regenerated schema artefacts and refreshed `VRLOG_FORMAT.md`, LiDAR architecture docs, and schema-hardening documentation (#420).
- Added troubleshooting and planning material for schema robustness and garbage-track investigation (#420).
- Cleaned up LiDAR track persistence and analysis paths, including the new `lidar_all_tracks` view, schema updates, export/report changes, and related tests (#419).
- Made docs more canonical across Python/docs surfaces and standardised pre-v1.0 release naming (#422, #423).
- Added canonical docs plan-hygiene CI, 4-hub structure, and Canonical metadata to all 69 plan files with 45 populated hub doc stubs (#422).

## March 21, 2026 - L8-L10 refactor, CLI networking & storage cleanup

- Landed the large L8-L10 refactor across Go packages and docs: reshaped server/package boundaries, updated routes, and continued the monitor → endpoint/client split (#409).
- Moved LiDAR network settings fully to CLI/runtime flags, trimming duplicated config-file state and updating radar/config docs and tests (#415).
- Broke up large [internal/api](../internal/api) server files into narrower admin, middleware, radar, reports, sites, and timeline units (#412).
- Broke up large [internal/db](../internal/db), [internal/lidar/l2frames](../internal/lidar/l2frames), and [internal/lidar/l5tracks](../internal/lidar/l5tracks) files into more focused storage, frame-builder, and tracking units (#412).
- Split tuning/background internals into cleaner accessors, codec, validation, background-manager, and region-specific files (#417).
- Refactored SQLite access through shared storage interfaces for labels and server-facing codepaths (#418).
- Continued general tech-debt cleanup across reports, tracking update logic, transit tooling, and plan/backlog alignment (#411, #416).
- Fixed context propagation and serialmux race issues (#413).
- Refreshed ERD column layout in generated schema docs (#404).

## March 20, 2026 - config refactor & migration tooling

- Reworked the radar/LiDAR configuration model around cleaner startup plumbing and helper extraction in [internal/cmd/server](../internal/cmd/server) (#407).
- Added `config-migrate` with broad coverage for moving existing configs onto the new layout (#407).
- Added `config-validate` to check migrated/runtime configs before deployment (#407).
- Expanded radar flag and config-path test coverage substantially (#407).
- Propagated config changes through PCAP and settling tooling, with matching documentation and example refreshes (#407).
- Converted tuning API from flat dot-path keys to nested JSON format across Go and web surfaces (#407).
- Normalised American→British English: `neighbor` → `neighbour`, `meters` → `metres` across code, comments, and test names (#407).

## March 19, 2026 - breaking migration merge & immutable run config planning

- Landed the breaking schema cleanup across SQL, Go, web, and macOS surfaces (#400).
- Regenerated schema artefacts and aligned v0.5.0 backlog/plan updates around the migration (#400).
- Added the immutable run-config asset plan (#387).
- Added canonical project-files planning for documentation and AI/customisation structure (#405).
- Added Go cleanup planning covering structural hygiene, god-file splitting, and structured logging (#402).
- Expanded Go coverage across frame-builder, monitor, visualiser, analysis, DB, and HTTP utility codepaths (#401).
- Strengthened DB-boundary handling with alignment planning and import-check tooling (#406).

## March 18, 2026 - contributor refresh, schema planning & homepage download

- Rewrote [CONTRIBUTING.md](../CONTRIBUTING.md) with updated contributor guidance, personas, and workflow expectations (#399).
- Expanded the v0.5.0 breaking-schema update plan to cover migration sequencing and cleanup scope (#386).
- Added `VelocityVisualiser.app` download and promo assets to the homepage, including supporting media/conversion tooling (#332).
- {copilot/add-bumper-sticker-designs} Added a Cairo-based bumper sticker generator tool (#403): Python CLI producing SVG/PNG stickers with configurable text and gradient backgrounds.
- {copilot/add-bumper-sticker-designs} Addressed code review feedback on `y_frac` semantics, Cool-S proportions, and step divisor clarity.

## March 17, 2026 - shim removal, MATRIX inventory & sweep worker planning

- Removed remaining backward-compatibility shims across Go, macOS, and Python surfaces, with matching backlog and plan updates (#383).
- Added the Matrix Tracer agent and the new `list-matrix-fields.py` inventory tooling (#394).
- Expanded `MATRIX.md` substantially and updated supporting structure docs around live implementation inventory (#394).
- Added HINT metric observability planning and related data-structure remediation updates (#394).
- Added the distributed sweep-workers plan (#382).
- Added the 100-line plan and checked in experiment notes / documentation cleanup across docs and Python (#379, #395, #398).
- {codex/plan-remaining-obb-heading-stability} Added OBB heading stability plan (#397): documented remaining work for oriented bounding box heading estimation, covering temporal smoothing, velocity-aligned correction, and evaluation metrics.

## March 16, 2026 - header standardisation & backlog prune

- Standardised documentation headers to bullet-list metadata format (`- **Key:** value`) across the repo and updated related tooling/docs to match (#378).
- Pruned backlog and decisions entries via Flo's weekly planning pass, tightening milestone scope and decision tracking (#396).

## March 14, 2026 - data reorganisation, VRLOG refresh & UK spelling

- Reworked VRLOG labelling and replay contracts across Go, Swift, and tooling: refreshed label APIs, run-track endpoints, replay handlers, recorder encoding, analysis/report typing, and `gen-vrlog` / `vrlog-analyse` support (#381).
- Expanded macOS visualiser run-browser and labelling flows alongside the VRLOG changes (#381).
- Added and refreshed VRLOG analysis / format documentation, terminology notes, and related backlog references (#381).
- Reshaped the `data/` tree: moved structures and maths material into [data/structures/](../data/structures) and [data/maths/](../data/maths), added [data/explore/](../data/explore), and relocated exploratory analysis outputs (#385).
- Normalised British English and refreshed repo-wide cross-references, including `convergence-neighbour` naming and documentation path fixes (#385).

## March 12, 2026 - agent stack refresh, light-mode plan & SSE delivery fix

- Reworked AI-agent tooling into the current Appius/Euler/Flo/Grace/Malory/Ruth/Terry stack (#377).
- Added [.github/knowledge/](../.github/knowledge), [TENETS.md](../TENETS.md), portraits, and refreshed agent/instruction docs around the new stack (#377).
- Added the macOS visualiser [light-mode plan](plans/lidar-visualiser-light-mode-plan.md) and linked it into the backlog (#376).
- Fixed SSE subscribers receiving test payloads end-to-end: buffered serialmux subscriber channels, tightened subscriber registration timing, and updated replay reset handling in `AppState` (#380).

## March 11, 2026 - dual frame representation & naming standardisation

- Added [L2 dual representation](plans/lidar-l2-dual-representation-plan.md) in `LiDARFrame` (`Points` + `PolarPoints`), removed repeated polar rebuild work, propagated through adapters, PCAP/network ingestion, perception, pipeline stages, and tests (#364).
- Renamed `PeakSpeedMps` to `MaxSpeedMps` across analysis/reporting, classification, proto, Swift visualiser, and tests; CI fix for remaining references missed in the initial rename (#361, #367).
- Standardised docs naming conventions: all files to `lowercase-with-hyphens.md`, moved schema ERD SVG from [internal/db/](../internal/db) (#366).
- Overhauled LiDAR architecture Mermaid layer diagram with enhanced documentation and added Mermaid flowchart readability utility script (#362).
- Added [coordinate-flow audit](lidar/architecture/coordinate-flow-audit.md) documenting polar/Cartesian transitions per tracking stage with flowcharts, matrices, and single-projection strategies (#363).
- Added 52-entry BibTeX bibliography (`docs/references.bib`) covering L3f, L5h, L6e planned work and existing references (#365).
- Updated BACKLOG with v0.5.0 breaking-change/proto work split and shipped component status (#358).
- Activated shared web cache for worktrees: `make activate-web-cache` for Codex environment setup, reusable Make targets (#360).

## March 10, 2026 - PCAP playback fix & VRLOG analysis metrics

- Hardened PCAP playback and analysis paths: replay timing fixes, speed-ratio controls, datasource/monitor/dashboard updates, database write cleanup, per-frame timing traces in `pcap-analyse`, and matching macOS visualiser model/client changes (#352).
- Implemented all §12.1 "implementable now" VRLOG analysis metrics: per-track detail fields, comparison report, and supporting `vrlog-analyse` code and tests (#356).
- Documented outstanding data-science questions in [CONTRIBUTING.md](../CONTRIBUTING.md), linked reflective-sign pose-anchor proposal from maths README, added Mermaid validation tooling notes (#359).

## March 9, 2026 - LiDAR L7-L10 & metrics-first documentation

- Updated [LiDAR data layer model](lidar/architecture/LIDAR_ARCHITECTURE.md) from six-layer to ten-layer architecture: added L7 analytics, L8 endpoints, L9 client, L10 scene layers to docs and backlog (#351).
- Rewrote contributor personas in [CONTRIBUTING.md](../CONTRIBUTING.md), expanded role guidance for Swift/macOS, Svelte/web, PDF/matplotlib, and platform/observability (#346).
- Added [ticTacTail](plans/tictactail-platform-plan.md) VRLOG inspection command specification: CLI shape, package layout, modes, metrics, and aggregation windows (#347).
- Added `internal/lidar/monitor/` deprecation and migration plan to `server/`, `l7analytics/`, `l8presentation/` (#348).
- Updated 0.5.0 proto plan: [percentile aggregation](plans/speed-percentile-aggregation-alignment-plan.md) decisions, [metrics registry](plans/metrics-registry-and-observability-plan.md) strategy, tagging guidance, and observability/export mapping (#349).
- Added Jess agent standup and planning behaviours (#350).
- British English spelling lint check, backlog decision updates, Python CI bump (#353).

## March 8, 2026 - L7/L8/L9 client planning

- Added LiDAR [L7 analytics and L8 visualisation refactor plan](plans/lidar-l8-analytics-l9-endpoints-l10-clients-plan.md): phased implementation steps and checklist covering docs, L7/L8 boundaries, monitor/, and generated artefacts (#342).

## March 7, 2026 - platform simplification phase 1

- Implemented Phase 1 of [platform simplification](plans/platform-simplification-and-deprecation-plan.md): added deprecation warnings to legacy deployment targets and tools, updated documentation for `velocity-deploy` and Makefile targets (#344).

## March 6, 2026 - compatibility plans

- Documented visualiser speed summary schema improvements, platform simplification status, and updated backlog tracking for completed work (#343).

## March 5, 2026 - contributor guidance, agent prototyping & cleanup

- Added contributor personas and general work themes to [CONTRIBUTING.md](../CONTRIBUTING.md) (#340).
- Added Jess (PM) agent; renamed existing agents with role context: Hadaly→Hadaly (Dev), Ictinus→Ictinus (Architect), Malory→Malory (Pen Test), Thompson→Thompson (Writer) (#339).
- Removed tracked binaries (`gen-vrlog`, `replay-server`, `visualiser-server`), added to `.gitignore`, cleaned misplaced scripts (#341).
- Bumped application dependency group: `go-echarts/v2`, `go-sqlite3`, `google.golang.org/grpc`, `modernc.org/sqlite` (#337).

## March 1, 2026 - config restructure & maths refresh

- Added [`docs/plans/config-restructure-plan.md`](plans/config-restructure-plan.md) documenting the migration from flat to layer-scoped nested tuning config schema (#335).
- Updated ML solver expansion and [velocity-coherent foreground extraction](plans/lidar-velocity-coherent-foreground-extraction-plan.md) maths documents to match new configuration and modelling direction (#335).
- Refreshed backlog references around the config and maths workstream (#335).

## February 28, 2026 - macOS build metadata

- Updated macOS build/release process so build metadata refreshes on every build; improved licensing display in About view; added release signing readiness tasks to backlog (#333).

## February 27, 2026 - DMG packaging & UI polish

- Added versioned DMG export for VelocityVisualiser: automated packaging scripts, Finder-layout automation (`create-dmg`), CI wiring, and updated build/getting-started documentation (#298).
- Second round of macOS UI polish: label taxonomy consolidation, track labelling and navigation improvements, enhanced test coverage for visualiser components (#329).
- Bumped version to `0.5.0-pre14` (#298).
- Added initial code-signing and notarisation pipeline (#425): Makefile targets for `codesign`, `notarytool`, and `stapler`, wired into the CI release workflow.

## February 26, 2026 - replay EOF debugging, build metadata cI/Test plumbing & format docs

- Added [data/structures/README.md](../data/structures/README.md) and [data/structures/VRLOG_FORMAT.md](../data/structures/VRLOG_FORMAT.md), and updated LiDAR architecture/documentation references to the new data format docs (#331).
- Expanded macOS visualiser diagnostics while debugging replay EOF behaviour: added `DevLogger` (replacing `os.Logger`) and increased logging coverage in `AppState`, `ContentView`, `RunBrowserView`, and `RunBrowserState` (#329).
- Simplified debug log output in `AppState` by removing privacy-attribute noise and improving message formatting clarity (#329).
- Added detailed `VisualiserClient` replay RPC diagnostics (`seek()` / `play()` / stream restart path) including connection-state and response logging (#329).
- Hardened replay restart handling in `AppState`: on replay completion, it tries `seek(logStartTimestamp)` + `play()` first and falls back to a full gRPC stream restart if RPCs fail or no playback RPC client is available (#329).
- Updated Go visualiser VRLOG replay loop to pause at EOF (instead of stopping), keep the replay loaded, and reset replay pacing timestamps so restart-from-beginning via `Seek(0)` + `Play()` works cleanly (#329).
- Extended `BuildInfo.swift` generation to macOS test runs and CI workflows (`mac-ci`, `nightly-full-ci`) so `gitSHA`/`buildTime` metadata is available consistently (#329).
- Refined `AboutView` / `ContentView` layout and separated version/build info into clearer lines (#329).

## February 25, 2026 - visualiser test coverage, replay completion & UI refinement

- Refactored `ContentView.swift` to extract testable helper functions: `KeyAction` enum (25 cases), `handleKeyPress()`, `assignLabelByIndex()`, standalone row/toolbar/panel views (#329).
- Extended keyboard label shortcuts from 4 → 9 (keys 1–9 map to `ObjectClass` proto enum order: noise, dynamic, pedestrian, cyclist, bird, bus, car, truck, motorcyclist) (#329).
- Fixed label panel sync: replaced `@State lastAssignedLabel` with computed `currentLabel` derived from `appState.userLabels`, so keyboard shortcuts and button clicks both update the highlight (#329).
- Added up/down arrow key track navigation (`selectNextTrack` / `selectPreviousTrack`) using `trackListOrder` published by `TrackListView`, so navigation follows the visible sort order (first-seen or max-speed) (#329).
- Consolidated Track Inspector and Label Panel: removed duplicate track/run ID display; header now shows full run ID in grey; label section uses "Labels" subheading instead of separate "Label Track" panel (#329).
- Reorganised Track Inspector components and enhanced data display (3D bounding-box labels, sparkline colour coding, speed display logic) (#329).
- Added replay completion handling across Go + macOS visualiser playback: server pauses at EOF instead of closing the stream, macOS playback controls/AppState detect replay completion, and replay state tests were updated for the new behaviour (#329).
- Added/expanded build metadata plumbing for the macOS visualiser: generated `BuildInfo.swift` during macOS builds, ignored generated `BuildInfo.swift` in git, and surfaced build information in `AboutView` (#329).
- Improved playback/replay robustness diagnostics: enhanced `VisualiserClient` stream handling/error logging and `AppState` replay completion detection with additional tests (#329).
- Added rendering options / UI controls for velocity arrows and updated related Swift tests (#329).
- Refactored `RunBrowserView`, expanded UI coverage (`UICoverageBoostTests` / comprehensive view tests), and fixed `FilterBarView` layout sizing consistency issues (#329).
- Applied Swift safety/polish fixes (#330): reduced force-unwraps, addressed actor-isolation issues, normalised whitespace, and fixed range-slider edge cases.
- Test coverage on `ContentView.swift` improved from 55.97% → 86.65% (3045/3514 lines). Total unit tests: 1032 (#329).

## February 24, 2026 - label taxonomy consolidation & UI polish

- Added `ObjectClass` protobuf enum and updated `Track` and `LabelEvent` messages to use it (replaces free-form string classification) (#328).
- SQL migrations (`000029`) to expand and normalise label vocabulary across the database (#328).
- Go server: renamed `ClassLabel` → `ObjectClass`, added `ClassifyFeatures` method for VRLOG replay reclassification (#328).
- SvelteKit frontend: expanded `DetectionLabel` union type, updated colours and tests (#328).
- macOS visualiser: updated classification labels, tag pills in `TrackListView` for classification and quality indicators (#328).
- Added classification maths specification and [label vocabulary consolidation plan](plans/label-vocabulary-consolidation-plan.md) (#328).
- Refactored playback state management: `PlaybackControlsDerivedState`, `PlaybackRPCClient` protocol, stream termination handling (#328).
- Added `trackMaxSpeed` to `AppState` and updated `TrackHistoryGraphView` to use persistent max speed with dashed line indicator (#328).
- Added `AboutView` with licensing information; renamed app references to VelocityReport (#328).
- User labels caching for immediate UI feedback; playback state reset on new VRLOG replay (#328).
- Enhanced `TimeDisplayView` with fallback for frame-index display (#328).
- Renamed `format-markdown` Makefile target to `format-docs` for clarity (#328).
- Completed HINT sweep polish items: TypeScript types, Continue button, carried badge, `exportLabels` removal, page subtitle (#328).
- Completed [Python venv consolidation](plans/tooling-python-venv-consolidation-plan.md) to single root `.venv/` (PR #320) (#320).
- Added [PDF generation migration-to-Go plan](plans/pdf-go-chart-migration-plan.md) (PR #321, decision D-17) (#321).
- Completed SWEEP/HINT platform hardening and polish backlog items; updated BACKLOG.md milestones (#328).
- Added `TestResults/` to `.gitignore` for Xcode test artifact cleanup (#329).
- Bumped version to `0.5.0-pre13` (#328).

## February 23, 2026 - settling evaluation, backlog alignment & vector scene map

- Implemented `settling-eval` CLI tool (`cmd/settling-eval`) for evaluating background grid convergence from PCAP files (#319).
- Added `SettlingReport` structure and JSON reporting methods to `l3grid` package (#319).
- Refactored settling-eval to use `l3grid.SettlingReport`, streamlining report generation (#319).
- Added unit tests for `IsSettlingComplete`, `EvaluateSettling` zero-cells edge case, and settling eval coverage (#319).
- Fixed race conditions in settling eval tests and VRLog seek tests (use atomic return values instead of `reader.CurrentFrame()`) (#319).
- Created [DECISIONS.md](DECISIONS.md) executive decisions register; resolved 16 design decisions with rationale (#315).
- Deprecated ROADMAP.md in favour of [BACKLOG.md](BACKLOG.md) as single source of truth for project-wide work items (#318).
- Moved `DESIGN.md` to [`docs/ui/DESIGN.md`](ui/DESIGN.md) and updated all cross-references (#319).
- Reorganised BACKLOG.md milestones: added v0.5.1 for serial port configuration UI, adjusted v0.7/v0.8 item placements (#319).
- Added 6 missing GitHub issues to BACKLOG.md (#4, #7, #8, #103, #122, #148); fixed #290→#11 reference.
- Updated [vector scene map](lidar/architecture/vector-scene-map.md) architecture: integrated OSM Simple 3D Buildings as structure priors, enhanced geometry prior service design (#319).

## February 22, 2026 - PCAP performance, OBB heading & region-adaptive parameters

- PCAP performance hardening (PR #313, 68 files): fixed `finalizeFrame` deadlock, added `foreground_max_input_points` tuning for DBSCAN decimation (#313).
- Used local random generator in `uniformSubsample` to avoid lock contention during concurrent DBSCAN calls (#313).
- Prevented miss counter advancement during frame throttle in tracking pipeline (#313).
- Updated `MaxFrameRate` to 25 in radar and tracking pipeline to prevent frame drops (#313).
- Added size-based chunk rotation to `.vrlog` recorder; prevented overflow in frame offset/length checks in replayer (#313).
- Refactored logging: removed debug flags, implemented structured log-level flag (`--log-level`), replaced trace logs with diagnostic logs (#313).
- Added backoff logging to PCAP real-time processing for diagnostics (#313).
- Added unit tests for DBSCAN subsampling, dropped frame handling, `ForegroundMaxInputPoints` config, and recorder/replayer error paths (#313).
- OBB heading stability (PR #310): added 90° jump rejection guard with heading source tracking; removed canonical-axis normalisation (#310).
- Standardised bbox dimensions: removed `_avg` duplication; both Swift and web visualisers use per-frame cluster dims (#310).
- Updated protobuf schema: renamed `bbox_length_avg` → `bbox_length`, moved `heading_source` to field 35 (#310).
- Added geometry-coherent tracking maths proposal with cross-references to existing proposals (#310).
- Implemented [region-adaptive parameters](lidar/operations/adaptive-region-parameters.md) (PR #307): per-region `ClosenessMultiplier` and `NeighborConfirmationCount` in `ProcessFramePolarWithMask` (#307).
- Added `UpdateConfig` method to `Tracker` for runtime config updates via WebServer (#307).
- Created [VISION.md](VISION.md) with project-wide vision statement and technical design language (#311).
- Bumped version to 0.5.0-pre12 (#313).

## February 21, 2026 - documentation reorganisation & maths specifications

- Reorganised [docs/plans/](plans) directory (PR #308, 35 files): consolidated and restructured plan documents (#308).
- Comprehensive LiDAR documentation overhaul (PR #295, 90 files): created [velocity-coherent foreground extraction](plans/lidar-velocity-coherent-foreground-extraction-plan.md) maths spec, added status fields to docs, created getting started guide and config parameter tuning guide (#295).
- Moved maths proposals to [data/maths/proposals/](../data/maths/proposals) with consistent naming convention (#295).
- Added [documentation standardisation plan](plans/platform-documentation-standardisation-plan.md) with metadata and validation gates (#295).
- Updated ROADMAP.md (PR #312): aligned roadmap entries with BACKLOG.md priorities (#312).
- Added macOS process profiling script (`macos_profile_lidar.sh`) and corresponding Makefile targets (#305).

## February 20, 2026 - design plans, test coverage & CI improvements

- Created [platform simplification and deprecation plan](plans/platform-simplification-and-deprecation-plan.md) (PR #300): deprecation signalling, deploy retirement gate, migration task list (#300).
- Created [quality coverage improvement plan](plans/platform-quality-coverage-improvement-plan.md) (PR #294): target 95.5% across all components, exclude `cmd/`, add logic extraction strategy and macOS Swift app (#294).
- Created LiDAR [multi-model ingestion and configuration](lidar/architecture/multi-model-ingestion-and-configuration.md) architecture proposal (PR #303): sensor auto-detection, DB-driven config (#303).
- Created LiDAR [network configuration](lidar/architecture/network-configuration.md) design document (PR #302): interface selection, network diagnostics, hot-reload UDP listener binding (#302).
- Created visualiser algorithm diffing design document (PR #299): visual comparison of algorithmic outputs (#299).
- CI performance improvements (PR #292): streamlined macOS CI, added nightly full CI, removed deprecated performance regression test (#292).
- Refactored test setup: replaced direct file handling with template DB cloning; simplified schema consistency checks (#292).
- Expanded Go test coverage (PR #289): added unit tests for debug logging, tracking pipeline, SQLite analysis run comparison, PCAP handling, SSE streaming, recorder error handling, track observations, labelling progress (#289).
- Added OBB serialisation to `frameBundleToProto` with tests (#293).
- Added lifecycle parameter sweep scripts and kirk0 configuration permutations for tuning (#293).
- Fixed out-of-bounds errors and tuned tracking parameters (PR #293) (#293).
- Updated [LiDAR data layer model](lidar/architecture/LIDAR_ARCHITECTURE.md) documentation with visualisation details (#293).
- Added serial configuration backend (#290): DB layer with `radar_serial_config`, API CRUD handlers, and a reload manager that hot-swaps serial port bindings without restarting the service.
- Added serial configuration UI to the Settings page: create/edit/delete dialogs, port test dialog, and deduplicated port options (#290).
- Expanded Go test coverage for serial config reload and added Jest threshold enforcement for the web frontend (#290).

## February 19, 2026 - LiDAR layer alignment & architecture review

- Implemented LiDAR 6-layer alignment refactor: split `l3grid/background.go` into persistence, export, and drift files (#280, #284).
- Split `monitor/webserver.go` into data-source and playback handler files (#284).
- Extracted domain comparison logic from `storage/sqlite` into `l6objects` (#284).
- Added HTTP method prefixes and middleware wrappers to route tables (#284).
- Fixed route conflict panic from duplicate route registrations (#284).
- Consolidated architecture docs, created [BACKLOG.md](BACKLOG.md) for deferred work items (#284).
- Documented further opportunities to reduce library size and complexity (#284).
- Completed review items 11–14 and P1–P3 from LiDAR layer alignment review (#284).
- Removed redundant method-not-allowed tests (mux handles 405 via method-prefixed routes) (#284).
- Migrated debug logging from `Debugf` to structured levels: `Opsf`, `Diagf`, `Tracef` (#287).
- Added shared colour palette module and CSS standard utility classes for frontend components (#286).
- Added [vector scene map](lidar/architecture/vector-scene-map.md) and 3D ground plane architecture documentation (#285).
- Added mathematical specifications and standardised config key ordering (#288).
- Added database naming standardisation guide and pull-request template (#291).

## February 18, 2026 - design system & teX/Chart updates

- Created [`DESIGN.md`](ui/DESIGN.md) with project-wide design principles and frontend design language (#282).
- Conducted comprehensive design review against `DESIGN.md`, producing improvement plan (#283).
- Updated Python PDF generator TeX configuration for minimal precompiled install (#281).
- Enhanced Svelte chart components (RadarOverviewChart) (#281).
- Updated Go CI integration tests to use minimal TeX tree (`build-texlive-minimal`) instead of full TeX Live install (#280).
- Trimmed CI TeX Live packages: dropped `texlive-fonts-extra` (~500 MB) and `latexmk`; added `--no-install-recommends` (#280).

## February 16, 2026 - LiDAR track improvements, 6-layer data model & refactor plans

- Implemented suspend and resume functionality for auto-tune sweeps with checkpoint persistence (#276).
- Added per-frame OBB dimensions and improved heading handling in LiDAR tracking (#276).
- Enhanced cluster size filtering, track pruning, and classification updates (#276).
- Serialised frame callback processing in `frame_builder.go` to prevent data races (#276).
- Added gap detection in `MapPane.svelte` to prevent spaghetti lines in track visualisations (#276).
- Added height band filtering parameters to tuning configuration (#276).
- macOS visualiser: added ground reference grid toggle, background grid points toggle, track filtering with dual-handle range slider (#270, #276).
- Bumped version to 0.5.0-pre8 and 0.5.0-pre9 (#270, #276).
- Created [LiDAR 6-layer data model](lidar/architecture/LIDAR_ARCHITECTURE.md) documentation (OSI-style) (#277).
- Added [LiDAR labelling QC enhancements](plans/lidar-visualiser-labelling-qc-enhancements-overview-plan.md) plan (#278).
- Created LiDAR refactor plans for package restructuring (#279).

## February 15, 2026 - HINT tuning system

- Implemented Human-Involved Numerical Tuning ([HINT](plans/lidar-sweep-hint-mode-plan.md)) system (renamed from RLHF) (#270).
- Replaced HTTP client with in-process `DirectBackend` for sweep runner, eliminating HTTP overhead (#270).
- Added long-polling for HINT status and PCAP completion (#270).
- Refactored label taxonomy: `good_vehicle`/`good_pedestrian` → `car`/`ped`; added `impossible` label (#270).
- Refactored all default parameters to load from `tuning.defaults.json` instead of hardcoded values (#270).

## February 14, 2026 - sweep schema fixes & documentation updates

- Fixed sweep parameter schema and config compatibility issues from PR review (#274).
- Updated documentation to reflect current code state: corrected Makefile target count (59→101), Go version (1.21+→1.25+), SQLite version (3.51.2), Python version (3.11+) (#274).
- Fixed broken doc links and wrong paths in [DEBUGGING.md](../DEBUGGING.md) (#274).
- Expanded repo structure in `copilot-instructions.md` (5→15 internal packages) (#274).
- Fixed setup guide frontmatter cost and typos (#274).
- Dependency update: bumped `markdown-it` 14.1.0→14.1.1 in docs site (#272).

## February 12, 2026 - precompiled LaTeX plan & test expansion

- Created design document for [precompiled LaTeX `.fmt`](plans/pdf-latex-precompiled-format-plan.md) support in PDF generator (#271).
- Expanded test coverage across Go, Python, and macOS components (#269).

## February 11, 2026 - RLHF score explainability, label provenance, vRLog replay & track labelling

- Implemented `RLHFTuner` engine with RLHF API endpoints and handler tests (#264).
- Added RLHF mode to sweep dashboard UI and Svelte sweeps page (#264).
- Implemented score component breakdown: `ScoreComponents`, `ScoreExplanation`, and `/explain` API endpoint (#264).
- Added class/time coverage gates for RLHF continue validation (#264).
- Implemented `label_source` provenance tracking with IoU-based confidence (#264).
- Added schema/version stamp fields in sweep persistence (migration 9.1) (#264).
- Fixed VRLog seek race condition; fixed int64 overflow in temporal spread calculation (#264).
- Boosted RLHF test coverage to 91.9%; web test coverage to 97% (#264).
- Created RLHF expansion plan with ML solver-inspired optimisation approach (#265).
- Designed [`velocity.report-imager`](plans/deploy-rpi-imager-fork-plan.md) (RPi Imager fork) for simplified deployment (#266).
- Added comprehensive Swift tests for macOS visualiser (#268).
- Expanded Go test coverage: Runner, sweep, tracking API, UDP listener, tracking pipeline (#267).
- Simplified packet handling by directly using polar points in `FrameBuilder` (#267).
- Bumped version to 0.5.0-pre7 (#264).
- Implemented `.vrlog` recording and replay for track labelling workflow (#262).
- Added `FrameRecorder` interface, `vrlog_path` field, and playback API endpoints (#262).
- Implemented VRLOG replay in visualiser publisher and gRPC control delegation (#262).
- macOS visualiser: run browser, run-track labelling support, side panel for track selection (#262).
- Added VRLOG safe directory configuration with absolute path validation (#262).
- Implemented background snapshot sending during VRLOG replay (#262).
- Enhanced tuning parameters and acceptance metrics tracking in background processing (#262).
- Added label taxonomy and classification details for detection and quality in LiDAR terminology (#262).
- Bumped version to 0.5.0-pre6 (#262).

## February 10, 2026 - label-aware auto-tuning & LiDAR refinement

- Implemented track label-aware auto-tuning: scoring incorporates human labels (#250).
- Enhanced LiDAR tuning refinement with updated parameters and thresholds (#259).
- Added PCAP analysis improvements for tune benchmarks (#254).
- Expanded Go unit test coverage across multiple packages (#253, #257).
- Created [frontend consolidation](plans/web-frontend-consolidation-plan.md) design document (#251).
- Designed Swift label plan for macOS visualiser track labelling workflow (#261).

## February 8, 2026 - documentation audit & roadmap

- Comprehensive audit of all 29 LiDAR documentation files against actual codebase (#246).
- Identified 12 discrepancies between docs and implementation status (#246).
- Updated README, devlog, and 9 other documentation files with current status (#246).
- Produced consolidated LiDAR roadmap with prioritised future work (P0–P3) (#246).
- Cross-referenced approved track labelling design document across relevant docs (#246).
- Implemented auto-tuning system (`sweep/auto.go`): iterative grid narrowing, multi-objective scoring (#246).
- Enhanced sweep dashboard: two new heatmaps (tracks, alignment), PARAM_SCHEMA with sane defaults (#246).
- Increased chart height from 300px to 450px, changed grid layout from 6 to 3 columns (#246).
- Fixed PCAP replay methods to use full file path for `pcap_file` parameter (#246).
- Created design document for [track labelling, ground truth evaluation, and label-aware auto-tuning](plans/lidar-track-labelling-auto-aware-tuning-plan.md) (8 phases) (#246).
- Created [dynamic algorithm selection](plans/lidar-architecture-dynamic-algorithm-selection-plan.md) design spec for LiDAR foreground extraction (#248).
- Designed `lidar_transits` table schema for dashboard/report integration (#246).
- Identified label API route gap: CRUD handlers exist but routes not registered (#246).

## February 7, 2026 - param sweep dashboard & auto-tune mode

- Consolidated LiDAR configuration into single config struct with fluent setters (#245).
- Implemented parameter sweep dashboard with ECharts bar charts and results table (#245).
- Added auto-tuning mode toggle and recommendation card to sweep dashboard (#246).
- Implemented settle mode support in sweep runner: `once` and `per_combo` (#245).
- Added iteration validation and clamping in `Sample` method (#245).
- Created sweep sampler, scoring, and chart data preparation utilities (#224).

## February 6, 2026 - macOS visualiser M5–M7

- M5: Algorithm upgrades: Hungarian association, ground removal, OBB estimation, occlusion coasting (#243).
- M6: Debug overlays: gating ellipses, association lines, residuals via gRPC; label API handlers (#243).
- M7: Performance hardening: Swift buffer pooling, PointCloudFrame reference counting, frame skip cooldown (#244).

## February 5, 2026 - macOS visualiser M3.5–M4 & settling time optimisation

- M3.5: Split streaming: background snapshots every 30s + foreground-only per-frame (78→3 Mbps) (#240).
- M4: Tracking interface refactor: `TrackerInterface`, `ClustererInterface`, golden replay tests (#240).
- Added [LiDAR settling time optimisation](lidar/operations/settling-time-optimisation.md) design document (#242).

## February 4, 2026 - macOS visualiser M0–M3

- M0: Schema + synthetic: protobuf schema, gRPC server, synthetic data generator, SwiftUI+Metal renderer (#237).
- M1: Recorder/replayer: `.vrlog` format, seek/pause/rate control, deterministic playback (#238).
- M2: Real point clouds: `FrameAdapter`, decimation modes, 70k+ points at 30fps (#239).
- M3: Canonical model: `FrameBundle` as single source of truth, LidarView + gRPC from same model (#239).
- Added app icon assets (#241).

## February 3, 2026 - macOS visualiser architecture

- Designed macOS visualiser architecture (SwiftUI + Metal + gRPC) (#236).

## February 2, 2026 - map feature & CI refactor

- Added interactive map component for site visualisation (#233).
- Refactored CI pipeline with end-to-end test support (#232).
- Dependency updates across application group (#234).

## February 1, 2026 - documentation homepage

- Updated homepage spacing and added hamburger menu (#235).
- Refined documentation site styling (#235).

## January 31, 2026 - dependency injection & test coverage

- Implemented dependency injection interfaces: `CommandExecutor`, `UDPSocket`, `PCAPReader`, `DataSourceManager` (#229).
- Refactored `UDPListener` to use `SocketFactory` for testability (#229).
- Created `RealDataSourceManager` wrapping WebServer operations (#229).
- Added sweep sampler for parameter sweep iterations with CSV output (#224).
- Added `BackgroundFlusher` for periodic persistence with mock implementations (#224).
- Added `BackgroundConfig` with validation and fluent setters (#224).
- Added chart data preparation utilities for LiDAR visualisation (#224).
- Increased Go test coverage to 85.9% on critical packages (#224).
- Restructured documentation paths to use `public_html` (#228).
- Added JavaScript unit tests for `TRACK_COLORS` in LiDAR types (#230).
- Extended WebServer and BackgroundManager test coverage (#231).
- Bumped version to 0.4.2 (#229).

## January 30, 2026 - test coverage expansion

- Increased Go test coverage from ~38% to 73.1% on testable packages (#213).
- Added comprehensive tests: monitoring, serialmux factory/mock, LiDAR arena, quality scoring (#213).
- Added admin routes integration tests and serialmux extended tests (#217).
- Enhanced tracking tests: `GetAllTracks`, speed history, quality metrics, spatial coverage (#217).
- Added CONTRIBUTING guide (#214).
- Fixed CI version check scripts and changelog links (#219, #221).

## January 29, 2026 - release v0.4.0 & setup guide

- Released version 0.4.0 with comparison reports, site config, and transit worker (#209).
- Created comprehensive setup guide for Citizen Radar deployment (#199).
- Added code coverage badges (Go, Python, Web) with Codecov integration (#211, #212).
- Added coverage documentation and CI workflow updates (#211).
- Enhanced documentation site: dark mode, Tailwind v4 CSS, KaTeX math rendering (#199).
- Fixed documentation CI to use pnpm instead of yarn (#208).

## January 25, 2026 - radar config SCD & comparison reports

- Implemented site config periods with cosine correction (Type 6 SCD pattern) (#196).
- Added boundary hour filtering to `RadarObjectRollupRange` for improved data accuracy (#196).
- Added histogram aggregation with configurable `max_bucket` (#196).
- Enhanced PDF report generation: detailed data tables, velocity unit support, percentile lines (#196).
- Added `compare_source` to `ReportRequest` for dual-source comparison (#196).
- Added `min_speed_used` to `RadarStatsResult` API responses (#196).
- Implemented save/load report settings from local storage (#196).
- Schema ordering fix: foreign key dependency-aware table creation (#196).
- Added security validation for PDF path (path traversal prevention) (#196).
- Bumped version to 0.4.0-pre9 (#196).

## January 20, 2026 - transit worker inspector

- Added full-history run capability to transit worker and API (#195).
- Implemented transit CLI: `analyse`, `delete`, `migrate`, `rebuild` commands (#195).
- Added transit deduplication plan and tests (#195).
- Enhanced transit worker UI with run management features (#195).
- Updated model version default from `rebuild-full` to `hourly-cron` (#195).
- Bumped version to 0.0.4-pre8 (#195).

## January 19, 2026 - comparison report generator

- Added comparison-period report generation with dual-period outputs (T1/T2) (#192).
- Enhanced PDF report with comparison metrics and improved labelling (#192).
- Refactored `chart_builder` to use integer indices for x-axis (#192).
- Added PDF generator version tracking in `set-version` script (#192).
- Bumped version to 0.0.4-pre7 (#192).

## January 17, 2026 - track visualisation UI fixes

- Fixed click detection to check track history, not just head position (#190).
- Filtered (0,0) noise points from rendering (backend and frontend) (#190).
- Added timestamp sorting for coherent track history lines (#190).
- Progressive track reveal during playback (point-by-point as timeline advances) (#190).
- Added pagination (50 tracks/page) with navigation controls to TrackList component (#190).
- Added "Min Observations" filter (1+/5+/10+/20+/50+) to filter noise tracks (#190).
- Fixed timeline sync with pagination (TimelinePane shows paginated subset via `onPaginatedTracksChange` callback) (#190).
- Fixed label truncation (removed `.slice(-6)`, increased sidebar width to 500px, left margin to 120px) (#190).
- Increased `HitsToConfirm` from 3 to 5 (tracks require 5 consecutive observations before confirmation) (#190).
- Added physical plausibility checks: `MaxReasonableSpeedMps=30.0`, `MaxPositionJumpMeters=5.0` in Mahalanobis gating (#190).
- Increased API limit from 100 to 1000 tracks (`getTrackHistory` default=500) (#190).

## January 16, 2026 - adaptive region segmentation

- Implemented adaptive region segmentation in BackgroundGrid for distance-aware thresholds (#190).
- Added `RegionParams` struct with configurable `ThresholdMultiplier` and `WarmupFrames` per region (#190).
- Default regions: near (0-30m, 1.0x), mid (30-60m, 1.2x), far (60-100m, 1.5x), extended (100m+, 2.0x) (#190).
- Fixed `WarmupFramesRemaining` initialisation logic (reset on grid clear, decrement per frame) (#190).
- Added region identification to `ProcessFramePolarWithMask` for per-point threshold scaling (#190).
- Added region dashboard HTML template embedded in Go webserver via `html/template` (#190).
- Fixed JSON serialisation for region parameters in API responses (#190).
- Aggressively tuned thresholds to achieve <30 false positives target (from 150+) (#190).
- Fixed potential XSS vulnerability (code scanning alert #33) with HTML escaping (#190).

## January 13, 2026 - warmup trails fix

- Fixed false positive foreground classifications during grid warmup (~100 frames) (#188).
- Implemented dynamic threshold multiplier: 4x at count=0, tapering to 1x at count=100 (#188).
- Fixed `recFg` accumulation during cell freeze (reset to 0 on thaw) (#188).
- Added track quality metrics for investigation (#188).
- Enhanced grid plotter visualisation for debugging (#188).

## January 11, 2026 - PCAP analyse benchmarking

- Extended pcap-analyse tool with performance benchmarking capabilities (#187).
- Added CI pipeline integration for automated benchmark runs (#187).
- Improved frame processing performance metrics (#187).

## January 9, 2026 - test coverage expansion

- Added comprehensive unit tests across LiDAR pipeline (~3,363 lines of new test code) (#186).
- Improved coverage for background processing, clustering, and tracking modules (#186).
- Added edge case testing for frame builder and parser (#186).

## January 6, 2026 - foreground forwarding & debugging

- Implemented foreground point forwarding to UDP port 2370 for downstream processing (#176).
- Added PCAP realtime replay mode for development/testing (#176).
- Created parameter tuning UI for runtime adjustment of background model settings (#176).
- Added AV Range Image Format Alignment architecture document (dual return handling) (#182).
- Fixed build failure with stub generation for pcap-disabled builds (#180).
- Applied security fixes for path validation (#176).

## January 3, 2026 - DB stats API

- Added `GET /api/db/stats` endpoint for database size and table statistics (#174).
- Implemented path traversal security hardening in file operations (#174).
- Added duplicate snapshot cleanup utility (#174).

## January 2, 2026 - LiDAR documentation updates

- Restructured LiDAR documentation for improved navigation (#173).
- Added velocity-coherent foreground extraction design document (1,456 lines) (#173).
- Merged in upstream dependency updates for documentation and application packages (#171, #172).

## December 28, 2025 - LiDAR alignment planning

- Created comprehensive alignment planning document for multi-sensor fusion (#170).
- Documented clustering algorithms comparison (DBSCAN vs HDBSCAN) (#170).
- Added occlusion handling strategies and static pose alignment procedures (#170).
- Designed motion capture architecture for dynamic calibration (#170).

## December 16, 2025 - AV-LIDAR integration plan

- Added comprehensive integration plan for LIDAR frame analyser (913 lines) (#167).
- Documented frame data flow from sensor to classification pipeline (#167).
- Specified integration points with AV perception stack (#167).

## December 10, 2025 - LiDAR track visualisation UI

- Implemented `MapPane.svelte` canvas component for real-time track rendering (#158).
- Added `TimelinePane.svelte` with D3-based SVG timeline and playback controls (#158).
- Created `TrackList.svelte` sidebar with track selection and filtering (#158).
- Added background grid overlay API for spatial visualisation (#158).
- Integrated WebSocket updates for live track streaming.

## December 9, 2025 - background grid standards & PCAP split

- Documented LiDAR background grid export standards and format options (#160).
- Added pcap-split tool design specification (1,094 lines) for PCAP file segmentation (#159).
- Defined foreground research export formats for offline benchmarking and ML work.

## December 2, 2025 - CLI architecture guide

- Created CLI comprehensive guide with long-term architecture vision (1,715 lines) (#155).
- Documented command structure, flag conventions, and extension patterns (#155).
- Added LiDAR foreground tracking API implementation notes (Phases 2.9-3.6) (#144).

## December 1, 2025 - release 0.3.0, phases 3.6-3.7 analysis tooling & ML pipeline roadmap

- Implemented `velocity-deploy` deployment manager (install, upgrade, rollback, backup, health commands) (#117).
- Added hourly transit worker job with UI toggle for background processing (#146).
- Created `scan_transits` backfill tool for historical data processing (#146).
- Baselined all components to version 0.2.0, then bumped to 0.3.0 (#145, #152).
- Reorganised SQL migrations, inserted original schema as 000001 (#151).
- Added `set-version.sh` utility for cross-codebase version updates (#156).
- Created time-partitioned data tables specification (2,980 lines) (#147).
- Added speed limit schedules feature spec (school zones, time-based limits) (#154).
- Completed security review of data partitioning plan (CVE fixes for path traversal, SQL injection, race conditions) (#150).
- Implemented `AnalysisRun` type for versioned parameter configurations with `params_json` storage (#144).
- Added `RunParams` type capturing all configurable parameters (background, clustering, tracking, classification) (#144).
- Created `RunTrack` type extending track data with user labels and quality flags for ML training (#144).
- Implemented `AnalysisRunStore` with database operations: `InsertRun()`, `CompleteRun()`, `GetRun()`, `ListRuns()` (#144).
- Added track management: `InsertRunTrack()`, `GetRunTracks()`, `UpdateTrackLabel()` (#144).
- Created labelling progress API: `GetLabelingProgress()`, `GetUnlabeledTracks()` (#144).
- Added split/merge detection types: `RunComparison`, `TrackSplit`, `TrackMerge` for run comparison (#144).
- Renumbered phases: 4.0→3.7 (Analysis Run), 4.1→4.0 (Labelling UI), 4.2→4.1 (ML Training) (#144).
- Created comprehensive ML Pipeline Roadmap documentation (now tracked in [docs/lidar/operations/track-labelling-ui-implementation.md](lidar/operations/track-labelling-ui-implementation.md) and related LiDAR plan docs) (#144).
- Planned Phase 4.0 Track Labelling UI: SvelteKit routes, track browser, trajectory viewer, labelling panel (#144).
- Planned Phase 4.1 ML Classifier Training: feature extraction, Python training pipeline, Go model deployment (#144).
- Planned Phase 4.2 Parameter Tuning: grid search, quality metrics, objective function optimisation (#144).
- Recommended implementation order: ✅3.7 → 4.0 → 4.2 (parallel) → 4.1 → 4.3 (#144).
- Implemented `cmd/tools/pcap-analyze/main.go` for batch PCAP processing through full tracking pipeline (#144).
- Full pipeline: parse UDP → build frames → background subtraction → clustering → tracking → classification (#144).
- Added output formats: JSON (complete results), CSV (track table), binary foreground research blobs (#144).
- Computed track speed-summary features from speed history (#144).
- Added `SpeedHistory()` getter to `TrackedObject` for external speed-summary computation (#144).
- Added `GetAllTracks()` method to `Tracker` for retrieving all tracks including deleted (#144).
- Added build tag `pcap` to `integration_test.go` for conditional test execution (#144).

## November 30, 2025 - phases 2.9-3.5 tracking pipeline, SQL schema, REST API & pose simplification

- Removed `internal/lidar/pose.go` and `internal/lidar/pose_test.go` (deferred to future phase) (#144).
- Removed pose-related fields from `ForegroundFrame`, `TrainingFrameMetadata`, `TrainingDataFilter`, `TrackObservation`, `WorldCluster`, `TrackSummary` (#144).
- Updated SQL schemas to remove `pose_id` columns from `lidar_clusters`, `lidar_tracks`, `lidar_track_obs` (#144).
- Updated `track_store.go` and `track_api.go` to use simplified signatures (#144).
- Research export data stored in polar (sensor) frame, which is pose-independent (#144).
- Implemented `TrackAPI` struct in `internal/lidar/monitor/track_api.go` (#144).
- Added endpoints: `GET /api/lidar/tracks`, `GET /api/lidar/tracks/active`, `GET /api/lidar/tracks/{id}` (#144).
- Added endpoints: `PUT /api/lidar/tracks/{id}`, `GET /api/lidar/tracks/{id}/observations` (#144).
- Added endpoints: `GET /api/lidar/tracks/summary`, `GET /api/lidar/clusters` (#144).
- Created JSON response structures: `TrackResponse`, `ClusterResponse`, `TracksListResponse`, `TrackSummaryResponse` (#144).
- Supports both in-memory tracker (real-time) and database queries (#144).
- Created migration `000009_create_lidar_tracks.up.sql` with `lidar_clusters`, `lidar_tracks`, `lidar_track_obs` tables (#144).
- Implemented persistence in `track_store.go`: `InsertCluster()`, `InsertTrack()`, `UpdateTrack()`, `InsertTrackObservation()` (#144).
- Added queries: `GetActiveTracks()`, `GetTrackObservations()`, `GetRecentClusters()` (#144).
- Implemented rule-based classification in `classification.go` with object classes: pedestrian, car, bird, other (#144).
- Added speed-history summary computation for track classification features (#144).
- Added `ObjectClass`, `ObjectConfidence`, `ClassificationModel` fields to `TrackedObject` (#144).
- Phase 2.9: Implemented `ProcessFramePolarWithMask()` for per-point foreground/background classification (#144).
- Phase 3.0: Added `WorldPoint` struct and `TransformToWorld()` with pose support (#144).
- Phase 3.1: Implemented `SpatialIndex` with Szudzik pairing, `DBSCAN()` with eps=0.6m, minPts=12 (#144).
- Phase 3.2: Implemented Kalman tracking with `TrackedObject`, `Tracker`, Mahalanobis gating (#144).
- Added track lifecycle: Tentative → Confirmed → Deleted with hits/misses counting (#144).
- Added classification research export support: `ForegroundFrame`, `EncodeForegroundBlob()`/`DecodeForegroundBlob()` (#144).

## November 29, 2025 - distribution & tracking plans

- Created distribution and packaging plan document (1,636 lines) (#124).
- Created LIDAR foreground extraction and tracking implementation plan v2 with polar/world frame separation (#125).
- Documented Szudzik pairing function for spatial indexing (#125).

## November 19, 2025 - database migration system

- Implemented database migration system using golang-migrate (#121).
- Created 12 migration files covering schema evolution (#121).
- Integrated migration CLI commands into main binary (#121).

## November 14, 2025 - migration system design

- Added database migration system design document (#106).
- Evaluated golang-migrate vs custom solution (#106).
- Documented migration file conventions and versioning strategy (#106).

## November 7, 2025 - LaTeX security fix

- Fixed LaTeX injection vulnerability (CVE-9.8 severity) with `escape_latex()` function (#99).
- Sanitised all user inputs before PDF generation (#99).
- Fixed JavaScript download bug in report generation (#99).

## November 6, 2025 - Python venv consolidation & AI agents

- Consolidated Python virtual environments to single repository root `.venv/` (#92).
- Added Malory (security) and Thompson (communications) custom AI agents (#94).
- Merged multiple dependabot dependency updates (#63 to #90).

## November 5, 2025 - docs restructure & path security

- Restructured Eleventy documentation site with syntax highlighting, typography plugin, breadcrumbs (#58).
- Added community page to documentation (#58).
- Implemented path validation to prevent traversal attacks in file operations (#59).

## November 4, 2025 - build metadata & PCAP API

- Added GIT SHA and build time to HTML meta tags via `set-build-env.js` (#56).
- Moved PCAP/live data source flag from CLI to runtime API (`POST /api/lidar/source`) (#55).
- Updated Makefile naming conventions (162+ line changes) (#57).

## November 1, 2025 - PCAP security & grid visualisation

- Implemented path traversal protection with `--lidar-pcap-dir` flag using `filepath.Join()` + `filepath.Abs()` + prefix checking (#54).
- Added file validation: regular files only, `.pcap`/`.pcapng` extensions required, 403 Forbidden for path escape (#54).
- Added systemd integration: service auto-creates PCAP directory via `ExecStartPre` (#54).
- Enhanced 4K-optimised dashboard (25.6×14.4" @ 150 DPI): 3 polar/spatial charts + 4 stacked metric panels (#54).
- Added PCAP snapshot mode with configurable interval/duration, auto-numbered directories, metadata JSON (#54).
- Created API helper scripts: grid reset, PCAP replay, background status fetching (#54).
- Added Makefile targets for noise sweep/multisweep plotting (#54).
- Added Python plotting tools: polar/cartesian heatmaps with live and PCAP replay modes (#54).
- Consolidated DEBUG-LOGGING-PLAN, GRID-ANALYSIS-PLAN, GRID-HEATMAP-API, LIDAR-PCAP-Debug docs into sidecar overview (#54).

## October 31, 2025 - grid analysis API & debug logging

- Added `GET /api/lidar/grid_heatmap` endpoint for spatial bucket aggregation (40 rings × 120 azimuth buckets) (#54).
- Implemented `GetGridHeatmap()` with configurable bucket size and settled threshold (#54).
- Response includes summary stats and per-bucket metrics: fill/settle rates, mean range/times seen, frozen cells (#54).
- Added Python plotting tools: polar (ring vs azimuth) and cartesian (X-Y) heatmaps (#54).
- Created noise analysis scripts: `plot_noise_sweep.py`, `plot_noise_buckets.py` (#54).
- Added comprehensive logging: grid reset timing, API call logs, rate-limited population tracking (#54).
- Enhanced FrameBuilder diagnostics: eviction logging, frame callback, improved azimuth wrap detection (#54).
- Re-enabled `SeedFromFirstObservation` with `--lidar-seed-from-first` flag (#54).
- Added settle time flag, configurable background flush interval and frame buffer timeout (#54).
- Added Makefile targets: dev-go, log-go-tail, log-go-cat, dev-go-pcap (#54).
- Fixed frame eviction callback delivery bug (#54).

## October 30, 2025 - PCAP debugging & development tools

- Enhanced frame eviction logging and finalised frame callback delivery path (#54).
- Added diagnostics for non-zero channel counts in ParsePacket (#54).
- Improved azimuth wrap detection for large negative jumps (>180°) (#54).
- Added --debug flag for frame completion and PCAP parsing logs (#54).
- Created local API helper scripts for PCAP replay and background status (#54).
- Consolidated dev-go logic into reusable run_dev_go function in Makefile (#54).
- Added log-go-cat and log-go-tail targets (#54).
- Corrected log directory name in .gitignore (#54).

## October 29, 2025 - configuration & documentation cleanup

- Updated lidar configuration flags for clarity and consistency (#54).
- Enhanced documentation for database path and command flags (#54).
- Added `SeedFromFirstObservation` parameter for PCAP mode background initialisation (#54).
- Removed outdated Frontend Units Override Feature documentation (#54).

## October 28, 2025 - PCAP support foundation

- Added PCAP file replay support with BPF filtering for multi-sensor files (#54).
- Integrated with existing parser and frame builder for seamless replay (#54).
- Added background persistence during PCAP replay with configurable flush intervals (#54).
- Added `--lidar-pcap-mode` flag to disable UDP listening for replay-only mode (#54).
- Added `POST /api/lidar/pcap/start` endpoint for triggering PCAP replay via API (#54).
- Updated LiDAR sidecar overview with classification, filtering, and metrics implementation details (#54).

## October 27, 2025 - formatting & linting

- Added formatting and linting commands to Makefile (#51).

## October 23, 2025 - sites, timezones & JavaScript tests

- Updated file prefix conventions for reports (#49).
- Added timezone handling for sites (#49).
- Added JavaScript test suite with CI integration (#49).

## October 21, 2025 - Vite security update

- Bumped vite from 7.1.5 to 7.1.11 for security fix (#48).

## October 14, 2025 - PDF generator cleanup

- Cleaned up PDF generator code and structure (#46).

## October 13, 2025 - report templates & tests

- Tweaked LaTeX report templates (#44).
- Added report generation tests (#44).

## October 3, 2025 - PDF report initialisation

- Initialised PDF report generation with LaTeX templates (report.pdf) (#42).

## September 27, 2025 - background parameters & multisweep

- Added configurable parameters: `closeness_multiplier`, `neighbor_confirmation_count` (#40).
- Created multisweep tool for parameter exploration (#40).
- Fixed export destination to use `os.TempDir()` (#39).

## September 23, 2025 - background diagnostics & monitor APIs

- Centralised runtime diagnostics with [internal/monitoring](../internal/monitoring) logger and per-manager `EnableDiagnostics` flag (#38).
- Added BackgroundManager helpers: `SetNoiseRelativeFraction`, `SetEnableDiagnostics`, `GetAcceptanceMetrics`, `ResetAcceptanceMetrics`, `GridStatus`, `ResetGrid` (#38).
- Added monitor API endpoints: `GET/POST /api/lidar/params`, `GET /api/lidar/acceptance`, `POST /api/lidar/acceptance/reset`, `GET /api/lidar/grid_status`, `POST /api/lidar/grid_reset` (#38).
- Created `cmd/bg-sweep` CLI: incremental & settle modes, per-noise grid reset, live bucket discovery, CSV output (#38).

## September 22, 2025 - background model fixes & snapshot export

- Wired BackgroundManager into LiDAR pipeline with self-contained snapshots (#38).
- Persisted per-ring elevation angles (`ring_elevations_json`) with each `lidar_bg_snapshot` (#38).
- Centralised snapshot-to-ASC export with elevation fallbacks (#38).
- Added backfill tool for populating `ring_elevations_json` in existing snapshots (#38).
- Improved `ProcessFramePolar`: restrict neighbour confirmation to same-ring, update spread EMA relative to previous mean (#38).
- Fixed concurrent SQLite update pattern to avoid SQLITE_BUSY (#38).

## September 21, 2025 - server & serialMux consolidation

- Centralised HTTP server and UI paths into [internal/api](../internal/api) (#38).
- Standardised on single SQLite DB (`sensor_data.db`) in [internal/db](../internal/db) (#38).
- Added LiDAR background snapshot persistence with manual HTTP trigger (#38).
- Added `--disable-radar` flag and robust `DisabledSerialMux` (#38).
- Merged duplicate LiDAR webservers; canonical monitor accepts injected `*db.DB` and `SensorID` (#38).
- Moved radar event handlers to [internal/serialmux/handlers.go](../internal/serialmux/handlers.go), classification to `parse.go` (#38).
- Added unit tests for serialmux (DisabledSerialMux, classification, config parsing, event handlers) (#38).

## September 20, 2025 - snapshot & persistence improvements

- Hardened BackgroundGrid persistence with RW-mutexes and copy-under-read (#38).
- Added DB access for snapshots via GetLatestBgSnapshot helper (#38).
- Added monitor endpoint to fetch, gunzip/gob-decode and summarise stored snapshots (#38).
- Moved manual persist endpoint into lidar monitor webserver (#38).

## September 19, 2025 - backgroundManager & polar processing

- Introduced BackgroundManager registry with `NewBackgroundManager` constructor (#38).
- Added managers discoverable via `GetBackgroundManager`/`RegisterBackgroundManager` (#38).
- Implemented snapshot serialisation (gob + gzip) with `Persist` method (#38).
- Added `InsertBgSnapshot` in lidar DB layer (#38).
- Implemented `ProcessFramePolar`: bin by ring/azimuth, EMA updates, neighbour-confirmation, freezing heuristics (#38).

## September 18, 2025 - polar-first refactor

- Centralised spherical→Cartesian math into `transform.go` helper (#38).
- Introduced `PointPolar` type; parser now emits polar-first (#38).
- Added `FrameBuilder.AddPointsPolar([]PointPolar)`, removed legacy `AddPoints([]Point)` (#38).
- UDP listener forwards polar points directly (#38).

## September 17, 2025 - background model & transform design

- Designed sensor-frame background model (ring × azimuth) for foreground masking (#38).
- Two-level settling per cell: fast noise settling, slow parked-object settling (#38).
- Designed BackgroundGrid snapshot persistence and warm-start on load (#38).
- Planned spherical→Cartesian refactor with polar/cartesian point type split (#38).
- Planned world-grid (height-map / ground estimate) on masked Cartesian points (#38).

## September 15, 2025 - velocity graph

- Added velocity graph component to web frontend (#37).

## September 13, 2025 - LiDAR frame parsing & test improvements

- Implemented LiDAR packet parsing into complete 360° frames (#35).
- Added units, velocity, and timezone configuration support (#36).
- Eliminated implementation dependencies in parse tests with local test constants (#35).
- Fixed boundary conditions in PCAP extraction loop bounds (#35).
- Streamlined extractUDPPayloads by removing redundant conditional checks (#35).

## September 12, 2025 - frame builder tests & time-based detection

- Fixed 3 previously failing frame builder tests with realistic production data patterns (#35).
- Moved PCAP integration test to [internal/lidar/integration_test.go](../internal/lidar/integration_test.go) (#35).
- Created `internal/lidar/testdata/` directory following Go conventions (#35).
- Increased test point counts to 60,000 points matching production (#35).
- Implemented hybrid frame detection: time-based primary with azimuth validation (#35).
- Integrated motor speed extraction from packet tail (bytes 8-9) (#35).
- Added dynamic frame duration based on actual RPM (50ms at 1200 RPM, 100ms at 600 RPM) (#35).
- Added --sensor-name flag for flexible deployment (#35).
- Enhanced code documentation in extract.go with packet structure details (#35).

## September 11, 2025 - memory optimisation & frame rate fixes

- Analysed Hesai Pandar40P UDP packet structure via Wireshark (#35).
- Discovered Ethernet tail issue: extra 4 bytes appended to UDP packets (#35).
- Fixed tail offset from last 6 bytes to last 10 bytes (#35).
- Validated correct UDP sequence extraction and point parsing (#35).
- Confirmed proper frame characteristics: ~69,000 points per frame, ~100ms duration (#35).

## September 8, 2025 - LiDAR parser initialisation

- Initialised LiDAR parser for Hesai Pandar40P protocol (#34).
- Added UDP listener for sensor data ingestion (#34).
- Created basic packet parsing structure (#34).

## September 5, 2025 - telraam integration

- Added Python tool to fetch Telraam traffic counting data (#33).

## September 2, 2025 - uniFi protect integration

- Added Python tool to fetch UniFi Protect camera data (#31).

## August 27, 2025 - production web assets

- Fixed and bundled production web assets in Go binary (#30).

## August 26, 2025 - frontend integration & middleware

- Integrated Svelte frontend with Go backend server (#25).
- Added Flush method to loggingResponseWriter for proper streaming (#26).
- Moved DB schema declaration to `.sql` file (#27).
- Fixed RadarObjects query and migration (#28).
- Added VSCode settings for development (#28).

## August 25, 2025 - Svelte dashboard

- Created first dashboard slices with Svelte, svelte-ux, layerchart (#23).
- Fixed Svelte theme configuration (#24).
- Added Tailwind CSS styling (#23).

## August 21, 2025 - favicon & logging middleware

- Added favicon serving (#22).
- Implemented LoggingMiddleware with colour-coded HTTP status codes (#22).

## August 20, 2025 - radar stats API

- Added `/api/radar_stats` endpoint for aggregated radar statistics (#21).

## July 23, 2025 - documentation merge

- Merged gh-pages branch into main for unified documentation (#16).

## July 10, 2025 - README enhancement

- Added ASCII art logo to README (#14).

## June 28, 2025 - code of conduct

- Added CODE_OF_CONDUCT.md for community guidelines (#13).

## June 27, 2025 - radarObject parsing & project structure

- Rebased and implemented RadarObject parsing (#12).
- Restructured into [internal/cmd/server/](../internal/cmd/server), `internal/*` packages (#12).
- Renamed project to velocity.report (#12).
- Added Apache 2.0 licence (#12).
- Implemented JSON parsing for radar data (#12).
- Added unit tests for parsing (#12).

## June 3, 2025 - radar objects table

- Created `radar_objects` SQL table.
- Added RadarObjects database functions.

## May 30, 2025 - live tail & command fixes

- Fixed live tail WebSocket functionality.
- Fixed command sending to serial port.

## May 26, 2025 - serial port configuration

- Updated serial port configuration handling.
- Updated event list SQL queries.

## May 22, 2025 - serialMux abstraction

- Initialised SerialMux for multiple subscriber support.
- Fixed graceful shutdown handling.
- Fixed x/net dependabot warning.

## May 16, 2025 - dev server & web skeleton

- Created working dev server configuration.
- Began web app skeleton with package namespace.
- Added systemd unit file for deployment.
- Fixed nested `/api/` route handling.
- Added `/debug/tailsql` and `/debug/backup` routes.

## May 12, 2025 - SQLite migration & serial tests

- Replaced DuckDB with SQLite (modernc.org/sqlite for pure Go).
- Added serialReader unit tests.
- Implemented bufio for serial port buffering.

## April 11, 2025 - command logging & handler extraction

- Added command logging to database.
- Extracted serialPortHandler into separate function.

## March 21, 2025 - serial reader & API improvements

- Implemented uptime parsing and validation.
- Added readline counter for serial data.
- Improved serial reader reliability.
- Updated backup filenames and intervals.
- Renamed execute endpoint.
- Set commandID from database.
- Added JSON response logging.
- Fixed timestamp casting in SQL.
- Added API verb support for commands.

## March 20, 2025 - baud rate & schema updates

- Set serial baud rate to 19.2k.
- Updated schema to v0.0.2 with uptime field.
- Added POST /execute endpoint.

## March 17, 2025 - project initialisation

- Initialised repository with Go 1.24.
- Created initial server structure with Gin HTTP framework.
- Set up serial port reader for radar sensor.
- Added gocron for backup scheduling.
- Configured DuckDB for initial data storage.
- Added .gitignore for database files and backup directory.
