# SF solo safety practice

Milestone 1 adds an optional, deliberately authored solo course to `/sf/`. Select
**Map & game → Game mode → Solo safety practice**, close the menu and accelerate.
This is a testable practice slice, not a representation of Harrison or a validated
measure of real driving safety. Production score weights remain open.

## Contracts and scope

| Change                            | Primary | Also affects  | Trigger and behaviour                                                                                       | Evidence or limitation                                                              |
| --------------------------------- | ------- | ------------- | ----------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| Authored 300 m road and crest     | World   | Physics, Data | Start at x = −150 m, drive east to the finish at x = 150 m                                                  | `crest-practice-v1`, local metres; no city tile changes                             |
| Contact and ballistic travel      | Physics | World, UI     | Road falling away releases support; existing horizontal momentum and gravity continue until landing         | Slow and fast crest regressions; point support, no suspension or vehicle damage     |
| Separate solo run                 | Goals   | UI, Data      | Finish on the road after landing, or abort and restart                                                      | No time bonus, rank, best time or weighted score; city records untouched            |
| Compact cues and stopped evidence | UI      | Goals, Data   | Limit, unknown coverage, kerb contact and off-road cues occupy the existing strip; detail stays in the menu | Existing keyboard/touch controls, cameras and mirror retained                       |
| Simulation-time accounting        | Data    | Goals         | Record excess bands and continuous excursions, retaining separate kerb evidence                             | Deterministic fixed-tick tests; severity/duration available without adopted weights |

**Traffic:** no lead actor, headway, DRAC, intersection or stop-line coaching is implemented.
Those remain Milestone 2. The city emoji game retains its existing physics, pickups and
record keys. The new contact model is scoped to the verified practice geometry; applying
it to ambiguous city surfaces is deferred. Switching to practice preserves city progress;
an already-running city trial's real-time clock continues, including time spent in practice.

## World and Physics

The course is 12 m wide, with an 80 m wide ground apron. Its analytic profile is
`4 × (1 + cos(πx / 50))` metres for `|x| < 50`, zero elsewhere. Ground and road share
that profile. The visible mesh samples it every metre. The analytical maximum difference
between a chord and this profile is under 2 mm. The crest radius is approximately
63.3 m. These are deliberately authored geometry and dimensions, not inferred street data.

At 120 Hz, grounded movement uses the existing bicycle model, hill force and off-road
friction. When following the sampled surface would require downward acceleration greater
than gravity, contact releases. The launch interval carries the prior velocity and heading;
throttle, braking, steering and tyre friction have no effect in the air. Vertical motion
uses 9.81 m/s² gravity. Contact returns on intersection with the surface; the vertical
velocity then follows its tangent. The model omits suspension, impacts and body rotation
dynamics. Missing ground freezes movement and excludes accounting until restart.

The authored kerbs are 15 cm high at y = ±6 m. A swept oriented vehicle envelope detects
crossing or sustained overlap near those edges when the vehicle is low enough to touch.
These contacts supply event evidence; they do not cause a crash, impulse or damage.
The footprint is the existing 4.46 × 1.92 m rectangle, used as a tyre-envelope approximation.
This is not a claim that the city renderer's kerb lines have physical collision geometry.

## Goals, UI and Data

The first driving input starts the run; the finish requires x ≥ 150 m, the vehicle centre
within the road and grounded contact. Reversing and returning to the course are allowed.
There is no elapsed-time reward or tie-breaker. Reset car, R and New solo run abort the
current attempt, close active events and start at the same initial state. The preceding
summary remains in the menu for the session; raw event arrays are local to the attempt.
Nothing is written to time-trial storage. Leaving practice aborts an unfinished attempt.

Controls remain under UI: the same touch pad and keyboard bindings apply. Opening the
menu releases held controls and pauses solo motion. Menu, hidden-page and focus-loss
simulation intervals are excluded, not compliant. The simulation uses the existing
120 Hz fixed tick and caps each rendered frame's contribution at 50 ms. Thus elapsed
simulation time does not include all wall time during long stalls or background suspension.
No physical-device frame-rate guarantee follows from browser emulation.

The authored **20 mph practice limit** applies on the road before x = 100 m. Beyond the
gold line, the final 50 m has unknown limit coverage. Off-road positions have unknown
limits too. The UI and source label the limit as authored, not posted or sourced from SF.
Only positive excess over a known limit enters `(0,5)`, `[5,10)` and `≥10` mph bands.
Zero is compliant; exactly 5 and 10 enter the next band. Limit transitions retain their
simulation timestamp and provenance. Unknown time never counts as compliant. Evaluated
time equals known plus unknown time; elapsed equals evaluated plus excluded time.

A continuous centre departure from the road is one excursion, ending at confirmed
re-entry or run termination. Airborne position above the drivable road is still on-road.
Excursions retain start/end, evaluated seconds off-road and peak speed. Kerb strikes are
rising edges of contact, retaining time, speed and lateral approach speed. Sustained
contact produces one strike; release and renewed contact produce separate trace rows.
A manoeuvre remains open until the entire envelope is back inside the road and clear
of the kerb (outer edge < 5.8 m). All strikes and departures before that clearance share
one incident ID. Pauses do not split a manoeuvre. Both traces remain inspectable in the
menu without double-counting the manoeuvre. These counts are evidence, not score weights.

## Controlled comparisons

The analytic crest tests start at x = −25 m with the local tangent velocity: an 8 m/s
pass stays grounded, while a 30 m/s pass takes off and lands. A separate whole-course
comparison starts from rest at x = −150 m, with 120 Hz inputs. The controlled arm uses
full throttle below 19 mph and 30% brake otherwise; the rushed arm holds full throttle.

| Whole-course arm  | Simulation elapsed | Excess (0,5) | Excess [5,10) | Excess ≥10 mph | Peak excess | Unknown time | Airborne |
| ----------------- | ------------------ | ------------ | ------------- | -------------- | ----------- | ------------ | -------- |
| 19 mph controller | 35.950 s           | 0            | 0             | 0              | 0           | 5.883 s      | 0        |
| Full throttle     | 10.942 s           | 0.358 s      | 0.358 s       | 7.617 s        | 70.397 mph  | 1.183 s      | 2.183 s  |

Both had zero course incidents. Faster elapsed time changes no score because none is
awarded. These scripted comparisons establish accounting and contact behaviour only;
paired human playtests are still required before choosing production weights or asserting
that the feedback encourages safer driving.

## Harrison at Beale: reproduced dip, unresolved road-level attribution

`node tools/sf-drive/inspect-harrison.mjs` performs a bounded map-only diagnostic of every
road row named Harrison St/Street, sampling centrelines at no more than 2 m spacing. On
manifest SHA-256 `48b30e1eeee788e89ed01db81ee4d37c4224b62dcf7902de85bcdc653c634a61`
(build `2026-11-20T18:26Z`, Gerardus `0.2.0+b928e792b74a`), it found 59 rows,
1,918 segments and 5,759 samples across 10 tiles, zero missing height samples and eight
sample transitions across tile ownership. The largest adjacent height difference was
1.4875 m; this alone does not distinguish a hill from a fault. The tool reports tile IDs.

David subsequently located the report at **Harrison Street and Beale Street**. The same
tool now resolves their paired signs, projects the midpoint onto Harrison, and samples a
200 m straight approach along its local centreline tangent in both directions. This is
a diagnostic path, not an asserted lane or the user's unspecified original speed.
The projected junction is `(4300.075676, 3048.876782)` in the manifest's local frame;
the southwest heading is −2.379446 radians (northeast adds π).

| Distance along Harrison, southwest positive | Terrain height (NAVD88) | Owning tile |
| ------------------------------------------- | ----------------------- | ----------- |
| −40 m                                       | 13.081 m                | `80858074`  |
| −20 m                                       | 14.339 m                | `8085807c`  |
| Junction, 0 m                               | 7.992 m                 | `8085807c`  |
| +20 m                                       | 16.866 m                | `8085807c`  |
| +40 m                                       | 18.180 m                | `8085807c`  |

The 0.5 m profile reaches 7.633 m near +0.5 m. The valley is **inside** `8085807c`,
not at the ownership transition roughly 30 m northeast. Its 8 m source grid itself
contains low nodes (for example, 7.9 m at `(4302,3046)`), so this is not just a raised
asphalt mesh intersecting otherwise smooth terrain. Current road paint and vehicle
height both follow the same single ground field. This establishes a reproducible dip
in the bake and the current contact path, without establishing the correct road level.

Two 120 Hz legacy-physics probes start 100 m from the junction at 20 mph, apply full
throttle below 20 mph and 30% braking otherwise, and traverse to the opposite end.
They query the same axle slope and centre/axle height presentation used by `/sf/`.
Obstacle collision/recovery is excluded from these diagnostic probes; they isolate
height/contact behaviour, not a complete user drive.

| Approach  | Completed 200 m | Minimum queried ground | Car presentation height there | Speed there | Largest per-tick presentation-height change |
| --------- | --------------- | ---------------------- | ----------------------------- | ----------- | ------------------------------------------- |
| Southwest | 22.292 s        | 7.517 m                | 8.652 m                       | 22.091 mph  | 0.076 m                                     |
| Northeast | 22.183 s        | 7.518 m                | 8.651 m                       | 24.973 mph  | 0.089 m                                     |

Both stay in the legacy height-following path: it has no airborne state, so it follows
the valley even when an actual vehicle could lose contact. `--trace` prints every
physics tick (position, queried terrain, presented car height, speed, contact semantics,
and tile); redirect that output outside the checkout. The tile SHA-256 values are
`e0a807840f1038b23748dafa3c615af37a8828fe7020eaf4bac8342090d71a4d`
for `8085807c.json.gz` and
`d7ee63d11132d7dc23a75cd9fe361e95e0e5e5a636d5d068479565d5f2dcf6f9`
for `80858074.json.gz`.

A bounded Chromium check also rendered the valley from both Harrison approaches
without page errors. A test-only manifest start placed the car 60 m away and disabled
the initial checkpoint snap; held throttle used the unchanged city physics. Both
30-second approach waits expired before the car reached the 8 m junction radius
(the HUD still reported Beale 40 m and 20 m ahead respectively). Captures showed the
dip, but a complete live browser traversal is **not** claimed. The numerical traversal
results above come from the isolated 120 Hz probes, not those browser attempts.

No independent road/deck-height evidence is available in these tiles. A single height
field cannot distinguish an elevated street from the street beneath it; that is an
attribution hypothesis here, not a verified source correction. The next step is to
compare the junction with source ground and supported road/deck observations using the
[layered-road contract](../plans/ui-sf-layered-roads-plan.md), then scope a supported
surface repair. Flattening or bridging this valley by interpolation could erase a real
lower road. No map tiles were changed or rebaked, and the authored crest remains an
independent test. The original report's precise lane and speed remain unknown.

## Verification

Run `make test-public-html` and `make build-docs`. The solo regressions cover exact band
boundaries, changed/missing limits, accounting, continuous excursions, contact deduplication,
airborne road classification, launch/landing, missing geometry and identical 120 Hz input
replays batched at 30, 60 and 120 Hz rendering. The test file is
`public_html/src/js/__tests__/sf-solo.test.mjs`.

Serve `public_html/_site` locally and run `node tools/sf-drive/smoke-solo.cjs` with Playwright
available on Node's module path and Chromium installed (`CHROMIUM_PATH` overrides its path).
`SF_BASE_URL` defaults to `http://127.0.0.1:8090`. The script exercises desktop completion,
reset, return to city and storage isolation, plus 390 × 690 touch input, release, restart,
in-car mirror and the compact HUD area budget. Physical phones, other browser engines,
manual accessibility review and paired human playtests remain outstanding.
