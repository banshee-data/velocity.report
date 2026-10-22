# LiDAR pipeline getting started

Quick-start runbook for starting, validating, and switching LiDAR pipeline data sources.

## 1. Prerequisites

1. Repository checked out and dependencies installed.
2. A valid LiDAR tuning config (for example [config/tuning.defaults.json](../../../config/tuning.defaults.json)).
3. Optional PCAP file available under your configured PCAP directory.

## 2. Start the pipeline (live sensor)

```bash
make dev-go-lidar
```

Expected:

- Radar/web server and the LiDAR pipeline start.
- The LiDAR API is served by the main server on `http://127.0.0.1:8080`, and the
  LiDAR Status page is at `http://127.0.0.1:8080/app/lidar/status`.

## 3. Verify health

```bash
curl -s "http://127.0.0.1:8080/api/lidar/status?sensor_id=hesai-pandar40p" | jq .
curl -s "http://127.0.0.1:8080/api/lidar/data_source?sensor_id=hesai-pandar40p" | jq .
```

Expected:

- `data_source` should be `live` after startup.
- No API errors from status endpoints.

## 4. Switch to PCAP replay

```bash
curl -s -X POST "http://127.0.0.1:8080/api/lidar/pcap/start?sensor_id=hesai-pandar40p" \
  -H "Content-Type: application/json" \
  -d '{"pcap_file":"example.pcap"}' | jq .
```

Check source:

```bash
curl -s "http://127.0.0.1:8080/api/lidar/data_source?sensor_id=hesai-pandar40p" | jq .
```

## 5. Switch back to live

```bash
curl -s "http://127.0.0.1:8080/api/lidar/replay/stop?sensor_id=hesai-pandar40p" | jq .
```

Confirm:

```bash
curl -s "http://127.0.0.1:8080/api/lidar/data_source?sensor_id=hesai-pandar40p" | jq .
```

## 6. Useful commands

```bash
# Grid status as JSON
make api-grid-status

# Background grid as SVG (fill rate; other metrics: settle_rate,
# unsettled_ratio, frozen_ratio, mean_times_seen)
curl -s "http://127.0.0.1:8080/api/charts/grid?sensor_id=hesai-pandar40p&metric=fill_rate" -o grid.svg
```

The LiDAR Debug page (`/app/lidar/debug`) shows the same grid view with a
metric selector, beside the foreground, cluster, track and traffic charts.

## 7. Next docs

- [docs/lidar/operations/data-source-switching.md](data-source-switching.md)
- [docs/lidar/operations/pcap-analysis-mode.md](pcap-analysis-mode.md)
- [docs/lidar/troubleshooting/pipeline-diagnosis.md](../troubleshooting/pipeline-diagnosis.md)
- [docs/lidar/operations/config-param-tuning.md](config-param-tuning.md)
