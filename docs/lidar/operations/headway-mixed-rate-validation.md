# Mixed-rate following exposure validation

This check asks whether a single-site net-time-gap report weights observations by supported
duration, rather than treating each recorded frame as an equal share of time. It also checks
that the report keeps an excluded interval visible without treating it as a measured gap.

## Cloud check

The run used Go 1.27.1 and the report code at `396a005f6777d031296024e6bd6d8125ad1dc271`.
The [analytic test](../../../internal/report/headway/mixed_rate_test.go) sends two straight
follower/leader pairs through `AnalyseFollowing`, the report builder and the report JSON
contract. One pair follows at a 1 s net gap for 10 supported seconds at 10 Hz. The other
has a 3 s net gap for 2 supported seconds at 20 Hz. A second arm adds two consecutive coasted
instants, or 0.2 s, to the first encounter without reducing its 10 s of supported time.

| Quantity                                                      | Clean | With coasting |
| ------------------------------------------------------------- | ----- | ------------- |
| Short-gap duration / supported opportunity in both encounters | 10/12 | 10/12         |
| Short-gap duration / accounted encounter time in the chart    | 10/12 | 10/12.2       |
| Explicitly excluded time, with reason `not_observed`          | 0 s   | 0.2 s         |
| Encounters with median net time gap below 2 s                 | 1/2   | 1/2           |

The valid-time rate stays at 83.33%, but the accounted-time chart share falls to 81.97% in the
coasting arm. The clean raw-frame ratio, 100/140 or 71.43%, is neither of these denominators.
The report's two encounter-level medians are 1 s and 3 s; its summary midpoint median is 2 s.
These quantities describe the two included encounters, not all follower exposure or all
traffic at the site. The 2 s band is descriptive, not a safety standard.

```sh
go test ./internal/report/headway -run '^TestReportMixedRateFollowingExposure$' -count=1 -v
go test ./internal/report/headway ./internal/lidar/l8behaviour -count=1
```

## Before wider-period reporting

The builder rejects duplicate capture IDs. In a separate cloud probe, the same source rows with
a different report-local capture ID were accepted twice: the report counted two encounters and
20 s where one source supplied one encounter and 10 s. The probe demonstrates a gap, not a
behaviour to lock into a regression test. The report source string is a locator, not a proven
capture identity; neither an equal locator nor a different display ID can settle deduplication.

For a wider-period report, pin source/run provenance, the analysis revision and the metric
definition. Use immutable recording or frame identity to prevent overlapping replays from
counting an observation twice, while keeping distinct recordings separate. Specify an episode
identity and a boundary policy for encounters spanning adjacent periods. The existing scene
API clips by whole instants and can suppress a clipped piece below minimum opportunity; do not
merge the already-suppressed scene summaries. Instead, rebuild from observations or retain
equivalent duration-by-gap and support state keyed by source-scoped encounter, then apply the
opportunity rule and compute encounter summaries on the composed window.

Add an acceptance test that cuts an encounter between frame boundaries and verifies that
recomposing disjoint periods preserves counts and supported/excluded durations under the
declared boundary policy. Also test two display IDs for the same input and two distinct inputs
with similar metadata. Never take percentiles of previously published site percentiles. Keep the
first reports focused on individual sites, without citywide rankings.

The synthetic result validates accounting in the existing report builder, not a production
period-merging implementation, a roadside measurement, or a threshold's physical validity.
