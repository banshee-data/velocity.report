# Hardware

Canonical reference for sensor specifications, interfaces, and target platform.

## Target Platform

- **Hardware:** Raspberry Pi 4, 4 GB RAM, 64 GB SD card
- **Architecture:** ARM64 Linux
- **Constraints:** Limited CPU/memory compared with desktop; local-only deployment
- **Cross-compilation:** `make build-radar-linux` produces ARM64 binary from desktop

## Radar Sensor

**Model:** OmniPreSense OPS243A

| Parameter    | Value                       |
| ------------ | --------------------------- |
| Technology   | Doppler radar, 10.525 GHz   |
| Interface    | Serial (USB-to-RS-232)      |
| Device path  | `/dev/ttyUSB0`              |
| Baud rate    | 19200                       |
| Data format  | 8N1                         |
| Output mode  | JSON (structured events)    |
| Measurements | Speed, magnitude, direction |

### Serial Commands

Two-character commands sent via serial:

| Command | Purpose                |
| ------- | ---------------------- |
| `OJ`    | Set JSON output mode   |
| `??`    | Query current settings |
| `A!`    | Report all detections  |
| `OM`    | Set magnitude mode     |
| `Om`    | Clear magnitude mode   |

### Data Output

Each detection produces a JSON event:

```json
{ "speed": 28.5, "magnitude": 3456, "direction": "inbound", "uptime": 12345.67 }
```

**Status:** Production-ready. High-confidence data path.

## LIDAR Sensor

**Model:** Hesai Pandar40P (P40)

| Parameter    | Value                |
| ------------ | -------------------- |
| Beams        | 40                   |
| Interface    | Ethernet (RJ45, PoE) |
| Protocol     | UDP                  |
| Sensor IP    | 192.168.100.202      |
| Listener IP  | 192.168.100.151      |
| Subnet       | 192.168.100.0/24     |
| Data rate    | 10–20 Hz frame rate  |
| Points/frame | Up to 70,000         |

### Network Configuration

The Raspberry Pi uses dual network configuration:

- **LIDAR subnet:** 192.168.100.151/24 (dedicated listener)
- **Local LAN:** 192.168.1.x (API + gRPC server)

### Processing Pipeline

```
UDP packets → Decoder → FrameBuilder (360° rotation assembly)
  → BackgroundManager (EMA grid: 40 rings × 1800 azimuth bins)
  → Foreground extraction → Clustering (DBSCAN)
  → Tracking (Kalman filter) → Object classification
```

**Status:** Experimental. Lower test coverage. Not yet production-deployed.

## gRPC Visualiser Interface

| Parameter  | Value                               |
| ---------- | ----------------------------------- |
| Port       | 50051                               |
| Protocol   | protobuf (`velocity_visualiser.v1`) |
| Modes      | Live, Replay (.vrlog), Synthetic    |
| Frame rate | 10–20 Hz                            |
| Latency    | < 50 ms end-to-end                  |

See `proto/velocity_visualiser/v1/visualiser.proto` for the full schema.
