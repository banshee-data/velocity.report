# Offline following and experimental DRAC evidence

Export a repeatable review record from one explicit source and estimate version. The export
retains all readable follower timelines, including no-leader periods, alongside fitted paths,
candidate decisions, endpoint evidence and both projected speeds. It makes no physical
qualification claim.

- **Status:** Implemented offline evidence path; physical qualification remains pending
- **Related:** [Behaviour plan](../../plans/lidar-behaviour-analytics-plan.md),
  [metrics registry](../../platform/architecture/metrics-registry.md)

## Run

Build through `make build-radar-local`, then use the existing headway command:

```bash
./velocity-report-local report headway --db /absolute/evidence.db \
  --source source/v1/CONTENT_ID --boxes \
  --evidence-context /absolute/binding.json --output /absolute/local-evidence \
  --cohort-manifest config/cohorts/cohort-manifest.json \
  --annotation-root /absolute/annotations --eligibility-db /absolute/use-ledger.db
```

The evidence mode writes JSON and does not invoke the PDF renderer. It opens the estimate
database read-only without migration and reads one transaction snapshot. The eligibility
ledger is a separate writable input: the developer-diagnostic decision must be recorded before
samples load. Prefer a completed replay evidence database. A missing database fails rather than
creating a replacement. Use `--solid-bodies --stage online` for retained solid-body evidence,
or the existing estimator, observation-model and parameter selectors when several versions exist.

The binding is hand-authored JSON with two members, `context` and `source`. Its context requires
`site_namespace`, `site_id`, `source_id`, `binding_id` and `policy_id`. The binding identifies the
reviewed source-to-site record, rather than deriving a site from timestamp overlap. The policy
must match the actual eligibility decision. The CLI fills `analysis_build_git_sha` from its
binary stamp. That stamp describes the analyser; it does not establish the original producer's
build. Build through Make so the stamp is useful.

The `source` member uses the existing observation capture identity: `ClipID`, ordered
`CapturePaths`, ordered `CaptureSHA256s` (64 hexadecimal digits without a prefix), and `ExtractorID`.
These are the identities used when the replay created the source, not arbitrary replacement
values. Each capture is rehashed; the reconstructed source must equal `--source` and the context
source. Its cohort assignment must match the explicit site. The verified source manifest is
retained in the output. Validation and qualification cohorts remain protected by the normal
eligibility rules.

When the original capture paths are unavailable on this machine, set the context's optional
`capture_files` to the local locations in the same order. Each location is hashed against the
original source manifest; relocating a file does not alter the declared source identity.

## Interpret the evidence

Each row represents a follower sample and its half-open interval to the next sample. The last
sample has zero duration. Intervals beyond the configured record-gap bound are accounted as
record gaps, with no evaluated DRAC. Pairing exclusions, free flow, following eligibility and
following exclusions have separate buckets; the complete decision and reasons remain available.
These buckets partition readable follower-time. They are not unique scene exposure or physical
encounter counts: simultaneous followers count separately. Source rows that cannot become
readable trajectories are reported in `read_quality`; their unknown duration is not fabricated.
A source with no readable trajectories is refused.

The experimental DRAC calculation has its own applicability. It is not limited to valid headway
samples, and headway's speed floor is not a DRAC floor. Non-closing, invalid gap, missing inputs
and unsupported evidence remain explicit. Evidence-admission failures carry `ineligible_evidence`
and their L8 reason without a calculation. Actual predicted support stays visible on the retained
point and never becomes observed exposure.

The export retains the existing marginal sigmas, but does not convert them into independent hard
bounds or claim a relative-speed covariance. Absent DRAC bounds mean missing uncertainty, not
zero error. No DRAC threshold, production metric or safety claim is adopted.

## Repeat and review

The filename identifies the site namespace/site, source, analysis schema/version/build and
eligibility policy. Binding labels and local capture paths are provenance rather than new
analysis identities: changing them under the same identity produces a content conflict instead
of silently adding a duplicate. Identical regeneration is a no-op;
different bytes under that identity are refused. The writer re-derives rows and accounting before
publishing, and concurrent writers publish only a completed file. New versions coexist. Keep
outputs outside Git, and report methods, provenance and edited findings in an authored report.
Use a local disk staging directory for this writer: it uses exclusive temporary-file creation
and hard links, which a NAS mount may refuse. A filesystem error leaves the existing final
record intact; copy verified completed files to archival storage afterwards.

Tests cover deterministic regeneration, complete free-flow timelines, missing intervals, source
and version mismatch, corrupted summaries, conflicting writes, concurrent repeat writes, cohort
refusal and a fixture-based CLI export. They establish software behaviour, not field accuracy.
