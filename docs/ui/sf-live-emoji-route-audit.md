# Live SF emoji route audit

The November 24, 2026 browser audit drives twenty courses between city
intersections. It checks whether the car can collect both emojis, stay on
its intended level, and leave the crossing onto supported pavement. The
courses include I-280, US 101, the Stockton and Broadway tunnels, the
Toll Plaza and Geary underpasses, and the Music Concourse Garage.

## Method

Chromium runs the public game in a fresh browser context for each course.
The driver confirms manual takeover and sends steering, throttle and brake
keys along the exported route. The game still selects contact surfaces and
processes collisions. The runner checks the manifest before each drive,
observes both emoji pickups and records covered tunnel contact. It checks
page errors and requests to Arrow or the city tile API.

The route definitions combine the Alemany and Geary plan with the park
and garage definitions from the Music Concourse plan. The course IDs in
the results identify the selected twenty definitions. The original plan
uses 23 mph for streets and 40 mph for highway guidance. The eastbound
Toll Plaza retry capped the street approach at 4.2 m/s from station
2,118 m through 2,180 m to make space for its tight turn.
[Alemany and Geary plan](../../tools/sf-drive/road-audit-alemany-geary-routes.json).
[Music Concourse plan](../../tools/sf-drive/road-audit-music-concourse-routes.json).

At one-metre intervals of travelled distance, the runner samples the
car's elevation when both adjacent animation samples report supported
contact. It derives grade from consecutive supported samples. A dip is
an elevation more than 0.5 m below the chord between samples four metres
either side. Grade and dip results describe the supported portions of
these game drives. The separate [citywide terrain screen](sf-road-route-audit.md#geary-boulevard-and-fillmore-street)
covers the wider ground map.

The public manifest changed during the audit when the Geary–Fillmore
crossing shipped. The exported course definitions matched across those
two revisions after excluding the manifest digest. Twenty-five fixed-step
regression results, including the paired tunnel courses, matched exactly.
Sixteen drives started on the parent manifest and four started on the new
manifest.

- Parent game manifest SHA-256 `4beb3c03f588b06661f88560fcb28c8b1ccd085b5565196af73e8daaee0bdfaf`.
- Geary–Fillmore game manifest SHA-256 `b2b15a9f62589ba554507bd94b8c0fa17748213836b7acac598d29c4e55970df`.
- Emoji placement SHA-256 `bcc1a3f4116282e4aefbfa4299731cef517e9c6ad0521a97c5fa30313eb5c64d`.

## Results

All twenty courses collected both emojis over 63.14 km. The initial
Toll Plaza run crashed during a street turn before its tunnel approach.
The slower retry completed through the tunnel. The runs used the lower
Geary crossing and both directions of the Stockton and Broadway tunnels.
The garage drive used the Music Concourse bore. The two long I-280 drives
covered both a citywide southbound traverse and the Alemany left exit.

The steepest sampled grade was 26.7% on the US 101 Bayview course.
Course p95 grades ranged from 1.3% to 20.2%. Supported samples showed
zero dips deeper than 0.5 m over 8 m. All completed courses reported
zero page errors and zero requests to Arrow or the city tile API.

| Course ID                            | Distance | Grade p95 | Peak grade | Dips |
| ------------------------------------ | -------: | --------: | ---------: | ---: |
| `stockton-northbound`                |  2.67 km |      6.0% |       8.0% |    0 |
| `stockton-southbound`                |  3.54 km |     10.3% |      14.5% |    0 |
| `broadway-west`                      |  4.75 km |     16.7% |      23.3% |    0 |
| `toll-eastbound`                     |  2.61 km |      8.6% |      22.0% |    0 |
| `fremont-under-deck`                 |  1.09 km |      7.6% |       8.9% |    0 |
| `i280-south-city-span`               |  8.80 km |      7.6% |      17.5% |    0 |
| `us101-bayview`                      |  3.60 km |     20.2% |      26.7% |    0 |
| `dogpatch-twenty-third`              |  2.22 km |     15.1% |      20.0% |    0 |
| `mission-creek`                      |  1.10 km |      1.3% |       4.2% |    0 |
| `north-waterfront`                   |  2.07 km |      2.0% |       5.0% |    0 |
| `presidio`                           |  1.33 km |     11.2% |      18.5% |    0 |
| `park-west`                          |  3.38 km |      5.1% |       7.3% |    0 |
| `sunset`                             |  3.73 km |     17.1% |      23.8% |    0 |
| `great-highway`                      |  1.73 km |      1.8% |       5.0% |    0 |
| `diamond-heights`                    |  3.01 km |     11.7% |      22.1% |    0 |
| `mission-southbound`                 |  1.23 km |      2.6% |       3.9% |    0 |
| `fifth-alemany-left-exit-to-portola` |  7.49 km |      7.9% |      26.5% |    0 |
| `geary-masonic-west`                 |  2.17 km |      8.3% |      11.3% |    0 |
| `broadway-east-to-embarcadero`       |  4.51 km |     16.8% |      23.5% |    0 |
| `garage-north-to-south`              |  2.09 km |      5.4% |      16.6% |    0 |

## Fillmore ground approaches

The six focused Geary–Fillmore drives reached the ground at their intended
departures on the live site. Both lower drives entered the covered view.
Every drive stayed grounded after entering its baked road. The initial
westbound lower drive logged 4.63 m of approach travel between browser
frames marked airborne. Four instrumented retries from 10, 12, 14 and
16 m before the entrance reached the exit. The 16 m retry reproduced the
flag with less than 1 mm of sampled separation from the ground grid.
The smoke test now measures vertical clearance on these approaches using
the 18 cm cutoff for the game's airtime display. It continues to require
zero airborne travel on the baked road. The full rerun passed all six
checks and measured a maximum approach clearance of 1.3 mm. Both lower
lanes returned to the exterior view grounded.
