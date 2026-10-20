# Release cadence: a version names a build that shipped (v0.5.x)

- **Status:** Proposed. Expands option b of P1 in the [architecture review](platform-architecture-review-plan.md#p1-the-version-is-a-theme-not-a-tag)
- **Layers:** Cross-cutting (release, changelog, release metadata, backlog bucket names, image)
- **Target:** the overdue release now; the cadence from the release after it
- **Decisions:** D-09, D-10, D-22 as restated; DECISIONS.md principle 1 ("Ship the install story early")
- **Companion plans:** [deploy-single-binary-image-consolidation-plan](deploy-single-binary-image-consolidation-plan.md) (its last gate), [release-json-automation-plan](release-json-automation-plan.md), [platform-planning-limits-plan](platform-planning-limits-plan.md) (bucket names)
- **Canonical:** [distribution-packaging.md](../platform/operations/distribution-packaging.md)

Cut the release that `main` has been carrying for five months, then release whenever `main`
carries a change a person on a street would notice and the release gate is green. A backlog
bucket is named for the release it becomes only when that release is the next one.

## Motivation

The release engineering is done and unused: static builds, versioned installs with atomic
rollback, an image, `velocity device upgrade`. What is missing is a release.

| Fact (checked against GitHub on October 20, 2026) | Value                                                                 |
| ------------------------------------------------- | --------------------------------------------------------------------- |
| Latest full release                               | v0.5.0, published March 25, 2026                                      |
| CHANGELOG                                         | a `[0.5.1] - 2026-05-11` section, but no v0.5.1 tag or release exists |
| Latest prerelease                                 | v0.5.1-pre31, September 1, 2026                                       |
| Makefile `VERSION`                                | `0.5.1-pre45`; pre32 to pre45 untagged                                |
| `release.json` read by `velocity device upgrade`  | stable 0.5.0; prerelease 0.5.1-pre25, six prereleases behind          |
| CHANGELOG `Unreleased`                            | holds a breaking change for reproducing paced replays                 |
| PRs merged October 11 to 19                       | 50                                                                    |

A person who installs today, or runs `velocity device upgrade` on the stable channel, gets
March's build: before the single binary, the Typst cutover and the image consolidation that the
CHANGELOG describes as 0.5.1. The backlog's next bucket is "v0.5.2 Tailgating metric +
distribution", 55 items that will not be done this year, so nothing can be called v0.5.2 either.

## Current state

What has held 0.5.1, from the plans and the backlog:

| Gate                                                                              | Owner plan                        | Size                    |
| --------------------------------------------------------------------------------- | --------------------------------- | ----------------------- |
| CI smoke test that the image binds `:80` in a chroot before export                | Single-binary image consolidation | `S`                     |
| Physical Pi OS bookworm and trixie validation of the static Tailscale install     | Single-binary image consolidation | operator, about a day   |
| #503 Tailscale ACL roles, a PR conflicted since the `cmd/radar` removal           | v0.5.1 bucket                     | `S` to rebase, or close |
| #290 serial configuration: a USB adapter and a changed-setting reload to validate | v0.5.1 bucket                     | operator, `S`           |

None of these is a reason to withhold five months of fixes from people who install the image.
Two are operator validations that can be done on the release candidate itself, and two are
features that can move to the next release.

## Design / approach

### A version names a build; a programme names an intent

A release is cut from `main` when two things are true: `main` carries at least one change a person
on a street, a labeller or an operator would notice, and the release gate passes. A programme of
work (the following-distance metric, the vocabulary cutovers) is a plan with phases; it never
holds a version number hostage. The backlog's release buckets name only the next release; work
further out sits under its programme's heading until it is next.

### The release gate

The gate is what `/release-prep` already runs (format, lint, tests, builds, agent drift, docs,
changelog), plus three items it does not:

1. The perf gate green on the release commit.
2. The image built and its `:80` smoke test passed.
3. A physical Pi 5 boot of the image: service up, API answering on `:80`, a report generated.

A gate failure blocks the release; an open feature item never does.

### Channels

The prerelease channel follows `main`: a prerelease is tagged at least weekly while `main` moves,
and `release.json`'s prerelease entry is updated in the same step, so a tester on the prerelease
channel is never six builds behind. The stable channel moves only on a release.

### Version numbers

Patch releases carry fixes and additive features; a minor release carries a breaking change to
an operator-facing contract (a migration that cannot be rolled back, a removed route or flag
after its tombstone release, a changed default that changes a published number). The L3
consolidation in `Unreleased` is breaking for anyone reproducing a published scene, so it is
named in the release notes' first section whatever the number.

## Scope

### Item 1: cut the overdue release

**Summary:** clear the two `S` gates, move the two features out, and cut the release from `main`.

**Steps:**

1. Land the `:80` chroot smoke test (`S`).
2. Move #503 to the next release or close it, and move #290's validation to the operator checklist of the release candidate.
3. Fold `Unreleased` into the release's CHANGELOG section and date it on the day of the tag, so no section is dated before its tag again.
4. Run the gate; the operator boots the image on a Pi 5 and validates the Tailscale install and the serial reload on it.
5. Tag, publish, and update `release.json`'s stable channel.

**Milestone:** now; the release number is Ruth's decision of record

### Item 2: the cadence rule

**Summary:** the rule above, in distribution-packaging.md and in `/release-prep`.

**Steps:**

1. Distribution-packaging.md gains the cadence, the gate and the channels.
2. `/release-prep` gains the perf, image and Pi boot gates, and refuses a CHANGELOG section dated without a tag.
3. A version check in CI fails when a CHANGELOG section has a date and no matching tag exists after the release workflow ran.

**Milestone:** the release after Item 1

### Item 3: the prerelease channel kept current

**Summary:** a weekly prerelease tag while `main` moves, and `release.json`'s prerelease entry updated in the same step.

**Steps:**

1. A scheduled workflow that tags a prerelease when `main` moved since the last one, through the existing release targets.
2. `release.json` updated by the same run; the release-json automation plan's security review applies.

**Milestone:** the release after Item 1

### Item 4: bucket names follow releases

**Summary:** the backlog names only the next release; programmes keep their own headings.

**Milestone:** with the planning limits plan's backlog lint

## Dependencies

- Item 1 needs the operator at a Pi 5 for about a day.
- Item 3 rides on the release-json automation plan's protection changes.
- Item 4 is the planning limits plan's.

## Risks

| Risk                                                         | Likelihood | Impact | Mitigation                                                                              |
| ------------------------------------------------------------ | ---------- | ------ | --------------------------------------------------------------------------------------- |
| The release carries a regression five months of PRs hid      | Medium     | Medium | The Pi boot and report gate; the stable channel keeps the previous version for rollback |
| Weekly prereleases add release noise                         | Low        | Low    | They change only the prerelease channel                                                 |
| The breaking L3 change surprises someone reproducing a scene | Medium     | Low    | First section of the release notes; the previous L3 kept as a parameter set             |

## Checklist

### Outstanding

- [ ] Item 1: cut the overdue release (`S`, plus a day of operator validation)
- [ ] Item 2: the cadence rule (`S`)
- [ ] Item 3: the prerelease channel kept current (`S`)
- [ ] Item 4: bucket names follow releases (`S`)

### Deferred

- [ ] Date-based versions: rejected; SemVer is in the version check, the image names and npm

### Accepted residuals (no action planned)

- [ ] The macOS DMG keeps the same version as the binary; its own cadence is a release of the same tag
