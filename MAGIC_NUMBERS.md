```
 _____         _        _____           _
|     |___ ___|_|___   |   | |_ _ _____| |_ ___ ___ ___
| | | | .'| . | |  _|  | | | | | |     | . | -_|  _|_ -|
|_|_|_|__,|_  |_|___|  |_|___|___|_|_|_|___|___|_| |___|
          |___|
```

Canonical register for project-wide numeric constants in velocity.report.

Use this document when you need to answer any of these questions quickly:

- Why is this value fixed?
- Is this value configurable?
- Where does this value come from?

This file tracks operational constants and algorithm defaults that materially affect behaviour.
It does not attempt to list every numeric literal in tests, fixtures, generated outputs,
or third-party code.

## Editorial and documentation standards

| Constant                          | Value       | Meaning                                                           | Source                                                                                                                                                                                                                   |
| --------------------------------- | ----------- | ----------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Prose width                       | 100 columns | Single prose line-width target across docs and code style tooling | [.github/STYLE.md](.github/STYLE.md), [docs/platform/operations/documentation-standards.md](docs/platform/operations/documentation-standards.md), [scripts/check-prose-line-width.py](scripts/check-prose-line-width.py) |
| Spec and architecture size target | 800 lines   | Advisory target for architecture/spec docs; not a hard gate       | [.github/STYLE.md](.github/STYLE.md#L99)                                                                                                                                                                                 |

## Network and endpoint ports

These defaults are configurable unless noted otherwise.

| Port  | Surface                                                      | Default source                                                                               | Runtime status                 |
| ----- | ------------------------------------------------------------ | -------------------------------------------------------------------------------------------- | ------------------------------ |
| 2368  | LiDAR raw forward (`--lidar-forward-port`)                   | [cmd/radar/radar.go](cmd/radar/radar.go#L72)                                                 | Active default                 |
| 2369  | LiDAR UDP ingest (`--lidar-udp-port`)                        | [cmd/radar/radar.go](cmd/radar/radar.go#L68)                                                 | Active default                 |
| 2370  | LiDAR foreground forward (`--lidar-foreground-forward-port`) | [cmd/radar/radar.go](cmd/radar/radar.go#L75)                                                 | Active default                 |
| 8080  | Main HTTP API/UI listen address (`--listen`)                 | [cmd/radar/radar.go](cmd/radar/radar.go#L50)                                                 | Active default                 |
| 8081  | LiDAR monitor HTTP listen (`--lidar-listen`)                 | [cmd/radar/radar.go](cmd/radar/radar.go#L67)                                                 | Active default                 |
| 8082  | Worker HTTP surface (`--worker-listen`)                      | [docs/lidar/architecture/distributed-sweep.md](docs/lidar/architecture/distributed-sweep.md) | Planned/distributed sweep docs |
| 8090  | Docs site Eleventy dev server (`--port`)                     | [public_html/package.json](public_html/package.json#L9)                                      | Dev only                       |
| 50051 | gRPC visualiser stream (`--lidar-grpc-listen`)               | [cmd/radar/radar.go](cmd/radar/radar.go#L80)                                                 | Active default                 |

## Sensor and geometry invariants

| Constant                 | Value     | Meaning                                          | Source                                  |
| ------------------------ | --------- | ------------------------------------------------ | --------------------------------------- |
| LiDAR beam count         | 40        | Hesai Pandar40P channel count                    | [ARCHITECTURE.md](ARCHITECTURE.md)      |
| LiDAR frame rate         | 10 Hz     | Nominal rotation/frame cadence                   | [ARCHITECTURE.md](ARCHITECTURE.md)      |
| Typical points per frame | ~70,000   | Nominal frame density                            | [ARCHITECTURE.md](ARCHITECTURE.md)      |
| Background grid shape    | 40 × 1800 | Beam rows × azimuth bins in the background model | [ARCHITECTURE.md](ARCHITECTURE.md#L654) |

## Tuning defaults (configurable)

Canonical source: [config/tuning.defaults.json](config/tuning.defaults.json)

### Pipeline

| Key                | Default |
| ------------------ | ------- |
| `buffer_timeout`   | `500ms` |
| `min_frame_points` | `1000`  |
| `flush_interval`   | `60s`   |

## Hard-coded numeric constants (non-config)

These values are currently embedded in code and should be changed deliberately.

| Constant                   | Value             | Why it exists                                             | Source                                                                                            |
| -------------------------- | ----------------- | --------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| `obbCovarianceEpsilon`     | `1e-9`            | Numerical stability threshold in OBB covariance maths     | [internal/lidar/l4perception/obb.go](internal/lidar/l4perception/obb.go#L10)                      |
| `hungarianlnf`             | `1e18`            | Sentinel "infinite" assignment cost in Hungarian matching | [internal/lidar/l5tracks/hungarian.go](internal/lidar/l5tracks/hungarian.go#L17)                  |
| `occlusionThresholdNanos`  | `200000000`       | Gap threshold (200 ms) counted as occlusion               | [internal/lidar/l5tracks/tracking_metrics.go](internal/lidar/l5tracks/tracking_metrics.go#L298)   |
| Frame gap assumption       | `100000000`       | 100 ms per frame for 10 Hz occlusion frame estimate       | [internal/lidar/l5tracks/tracking_metrics.go](internal/lidar/l5tracks/tracking_metrics.go#L307)   |
| `slowFrameThresholdMs`     | `50.0`            | Pipeline performance warning threshold                    | [internal/lidar/pipeline/tracking_pipeline.go](internal/lidar/pipeline/tracking_pipeline.go#L239) |
| `healthSummaryInterval`    | `100`             | Frames between health summaries                           | [internal/lidar/pipeline/tracking_pipeline.go](internal/lidar/pipeline/tracking_pipeline.go#L240) |
| `timingWindowSize`         | `100`             | Rolling timing window size                                | [internal/lidar/pipeline/tracking_pipeline.go](internal/lidar/pipeline/tracking_pipeline.go#L241) |
| `maxBackgroundChartPoints` | `5000`            | Cap for debug chart background points                     | [internal/lidar/pipeline/tracking_pipeline.go](internal/lidar/pipeline/tracking_pipeline.go#L308) |
| `logInterval`              | `5s`              | gRPC diagnostics log cadence                              | [internal/lidar/l9endpoints/grpc_server.go](internal/lidar/l9endpoints/grpc_server.go#L287)       |
| `slowSendThresholdMs`      | `50`              | gRPC send-slow threshold                                  | [internal/lidar/l9endpoints/grpc_server.go](internal/lidar/l9endpoints/grpc_server.go#L288)       |
| `sendTimeoutMs`            | `100`             | gRPC send timeout threshold                               | [internal/lidar/l9endpoints/grpc_server.go](internal/lidar/l9endpoints/grpc_server.go#L289)       |
| `maxConsecutiveSlowSends`  | `3`               | Enter frame-skip mode after repeated slow sends           | [internal/lidar/l9endpoints/grpc_server.go](internal/lidar/l9endpoints/grpc_server.go#L290)       |
| `minConsecutiveFastSends`  | `5`               | Exit skip mode after sustained fast sends                 | [internal/lidar/l9endpoints/grpc_server.go](internal/lidar/l9endpoints/grpc_server.go#L291)       |
| m/s → mph conversion       | `2.2369362920544` | Exact conversion factor for user-facing speed units       | [internal/units/velocity.go](internal/units/velocity.go#L37)                                      |
| m/s → km/h conversion      | `3.6`             | Exact conversion factor for metric speed display          | [internal/units/velocity.go](internal/units/velocity.go#L39)                                      |

## Change policy

When you change a constant listed here:

1. Update this file in the same PR.
2. Update the canonical source (`config/tuning.defaults.json` or code constant).
3. Update any user-facing docs that mention the old value.

If a number changes often in production tuning, it probably belongs in config rather than code.
