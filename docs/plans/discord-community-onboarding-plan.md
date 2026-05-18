# Discord community onboarding plan

The homepage now invites two kinds of people into the community:

- residents and advocates who want help measuring a street
- engineers and researchers who want to improve radar, LiDAR, reports, and the surrounding tooling

The Discord server should make both paths obvious within one minute of joining. It does not need many channels. It needs clear doors.

## Current read

Based on the visible server shape, the useful public channels are:

- `#hello-world`
- `#help`
- `#hardware`
- `#github`

There are also private and admin channels. Keep those private in public docs. Newcomers need to know where they can act, not how the back room is wired.

## Target behaviour

### `#hello-world`

Keep this as the first stop. Pin a prompt:

> Tell us what brought you here: a street you want to measure, a sensor you are trying to mount, a report you want to produce, or a technical area you want to improve.

This gives residents, hardware builders, engineers, researchers, and civic users a way to introduce themselves without guessing the house style.

### `#help`

Use this for setup and report support. Ask for:

- Pi model
- radar or LiDAR hardware
- release version
- what they tried
- what happened
- logs, screenshots, or photos when useful

Open one thread per substantial problem. Anything that repeats should become a setup-guide or troubleshooting update.

### `#hardware`

Use this for field equipment:

- sensors
- mounting
- aiming
- weatherproofing
- enclosures
- cables
- calibration notes
- photos from real deployments

This is where practical detail belongs. A good photo of a bad cable route can save more time than a beautiful diagram nobody can reproduce.

### `#github`

Decide whether this is a human coordination channel or a bot/feed channel.

If it is for humans, keep it for "is anyone already working on this?", PR/release chatter, and short design triage. Bugs, feature requests, and decisions should still move to GitHub issues or PRs.

If it is mostly automation, rename it to make that obvious, for example `#repo-feed`, and create a separate human channel only when there is enough traffic to justify it.

## Recommended additions

### `#announcements`

Add before the audience grows. Keep it read-only or tightly moderated.

Use it for:

- releases
- setup-guide updates
- important hardware notes
- validated field lessons
- community calls or events, if those start happening

### `#lidar-research` or `#perception`

Add or surface this once LiDAR discussion grows beyond occasional messages.

Use it for:

- clustering
- tracking
- classification
- ground modelling
- radar-plus-LiDAR fusion
- replay packs
- maths and evaluation

This keeps research conversation from drowning setup help, and keeps setup help from interrupting research work.

## Public copy rule

Public pages should say:

- Discord is for live help and working conversations.
- GitHub is for durable issues, feature requests, pull requests, and decisions.
- People who are unsure should start in `#hello-world` or `#help`.

Do not expose private/admin channels in public copy.
