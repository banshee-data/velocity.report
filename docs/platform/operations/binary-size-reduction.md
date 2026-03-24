# Binary Size Reduction

Hub doc for binary-size governance. The velocity-report Linux ARM64 binary
must stay below 40 MB. See the active plan for root cause, phases, and
checklist.

- **Plan:** [binary-size-reduction-plan](../../plans/binary-size-reduction-plan.md)
- **Target:** < 40 MB production binary (currently ~211 MB, almost entirely stale embeds)
- **CI gate:** `scripts/check-binary-size.sh` (planned)

## Key Facts

| Segment                | Size   | Notes                                  |
| ---------------------- | ------ | -------------------------------------- |
| Stale `static/` embeds | 172 MB | Root cause — build hygiene, not Svelte |
| Go code + all deps     | 38 MB  | Includes SQLite, gRPC, protobuf, gonum |
| `web/build/` (current) | 1.1 MB | The actual SvelteKit build             |

## Phases

1. **Eliminate stale embeds** — remove `//go:embed static/*`, serve favicon from `WebBuildFiles` (~172 MB saving)
2. **Strip debug symbols** — add `-s -w` to production `LDFLAGS` (~8–12 MB saving)
3. **CI binary size gate** — fail builds exceeding 45 MB threshold
4. **Optional further reductions** — echarts removal, lazy Leaflet, UPX (diminishing returns)
