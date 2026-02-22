# LiDAR Pipeline Getting Started

Quick-start runbook for starting, validating, and switching LiDAR pipeline data sources.

## 1. Prerequisites

1. Repository checked out and dependencies installed.
2. A valid LiDAR tuning config (for example `config/tuning.defaults.json`).
3. Optional PCAP file available under your configured PCAP directory.

## 2. Start the Pipeline (Live Sensor)

```bash
make dev-go-lidar
```

Expected:

- Radar/web server and LiDAR monitor start.
- LiDAR monitor API is available on `http://127.0.0.1:8081`.

## 3. Verify Health

```bash
curl -s "http://127.0.0.1:8081/api/lidar/status?sensor_id=hesai-pandar40p" | jq .
curl -s "http://127.0.0.1:8081/api/lidar/data_source?sensor_id=hesai-pandar40p" | jq .
```

Expected:

- `data_source` should be `live` after startup.
- No API errors from status endpoints.

## 4. Switch to PCAP Replay

```bash
curl -s -X POST "http://127.0.0.1:8081/api/lidar/pcap/start?sensor_id=hesai-pandar40p" \
  -H "Content-Type: application/json" \
  -d '{"pcap_file":"example.pcap"}' | jq .
```

Check source:

```bash
curl -s "http://127.0.0.1:8081/api/lidar/data_source?sensor_id=hesai-pandar40p" | jq .
```

## 5. Switch Back to Live

```bash
curl -s "http://127.0.0.1:8081/api/lidar/pcap/stop?sensor_id=hesai-pandar40p" | jq .
```

Confirm:

```bash
curl -s "http://127.0.0.1:8081/api/lidar/data_source?sensor_id=hesai-pandar40p" | jq .
```

## 6. Useful Commands

```bash
# Live grid snapshots
make stats-live

# PCAP snapshot run (set PCAP path)
make stats-pcap PCAP=/absolute/path/to/capture.pcap
```

## 7. Next Docs

- `docs/lidar/operations/data-source-switching.md`
- `docs/lidar/operations/pcap-analysis-mode.md`
- `docs/lidar/operations/quickstart-pipeline-fix.md`
- `docs/lidar/operations/config-param-tuning.md`
