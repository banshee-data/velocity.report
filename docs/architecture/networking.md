# Networking Design Principles

This document describes the network architecture, listener segmentation, and access control model for velocity.report.

## Deployment Model

velocity.report runs on a Raspberry Pi 4 deployed on a private LAN alongside traffic sensors. The device has two network interfaces:

| Interface | Purpose | Typical address |
|-----------|---------|----------------|
| Ethernet (`eth0`) | Sensor subnet — LiDAR UDP packets | `192.168.100.x` (static) |
| Wi-Fi/Ethernet (`wlan0`/`eth1`) | Management — HTTP API, SSH, Tailscale | DHCP or static |

The sensor subnet is a dedicated point-to-point link between the Raspberry Pi and the LiDAR sensor. No other hosts should be present on this subnet.

## Listener Architecture

The server binds three distinct listener classes, each with a different trust level:

### 1. Public HTTP listener (`:8080`)

The radar API server. Serves the Svelte frontend and read-only data APIs.

**Endpoints**: `/events`, `/api/radar_stats`, `/api/config`, `/api/sites`, `/api/timeline`, `/api/generate_report`, static assets.

**Trust level**: LAN-accessible. Read-only data endpoints and the web UI. No destructive operations.

### 2. LiDAR monitor listener (`:8081`)

The LiDAR monitor HTTP server. Provides real-time LiDAR status, tuning, and data source control.

**Route categories and access intent**:

| Category | Example endpoints | Access |
|----------|------------------|--------|
| Status/read-only | `GET /api/lidar/status`, `GET /api/lidar/traffic` | LAN |
| Tuning | `POST /api/lidar/params`, `POST /api/lidar/sweep/*` | LAN |
| Data source switching | `POST /api/lidar/pcap/start`, `POST /api/lidar/pcap/stop` | LAN |
| Destructive | `POST /api/lidar/runs/clear` | Feature-gated (`VELOCITY_REPORT_ENABLE_DESTRUCTIVE_LIDAR_API=1`) |
| Debug dashboards | `/debug/lidar/*` | LAN |

**Trust level**: LAN-accessible. Tuning and data source endpoints modify runtime behaviour but do not destroy persisted data. The `/api/lidar/runs/clear` endpoint is behind `featureGate` and disabled by default.

### 3. Admin/debug listener (Tailscale `tsweb.Debugger`)

Sensitive diagnostic endpoints attached to the radar mux via `tsweb.Debugger`. Protected by Tailscale's `AllowDebugAccess` which restricts access to loopback IPs and authenticated Tailscale peers.

**Endpoints**: `/debug/send-command` (serial port), `/debug/send-command-api`, `/debug/tail` (live serial stream), `/debug/db-stats`, `/debug/backup`, `/debug/tailsql/`, `/debug/pprof/*`.

**Trust level**: Tailscale-authenticated only. These endpoints can send raw commands to the radar sensor, download database backups, and execute arbitrary SQL queries. They are never accessible from the open LAN.

## Route Segmentation

```
┌─────────────────────────────────────────────────────┐
│                   Raspberry Pi                       │
│                                                      │
│  ┌──────────────┐    ┌───────────────┐               │
│  │ Radar API    │    │ LiDAR Monitor │               │
│  │ :8080        │    │ :8081         │               │
│  │              │    │               │               │
│  │ /events      │    │ /api/lidar/*  │               │
│  │ /api/radar_* │    │ /debug/lidar/*│               │
│  │ /api/sites   │    │               │               │
│  │ /static/*    │    │ featureGate:  │               │
│  │              │    │  runs/clear   │               │
│  │ tsweb:       │    │               │               │
│  │  /debug/*    │    └───────────────┘               │
│  │  (Tailscale) │                                    │
│  └──────────────┘                                    │
│                                                      │
│  ┌──────────────┐    ┌───────────────┐               │
│  │ UDP listener │    │ Tailscale     │               │
│  │ :2369        │    │ (tsnet)       │               │
│  │ LiDAR pkts   │    │ Admin access  │               │
│  └──────────────┘    └───────────────┘               │
└─────────────────────────────────────────────────────┘
```

## Access Control Mechanisms

### Current: Tailscale route segmentation

The primary access control mechanism is **network-level segmentation** using [Tailscale](https://tailscale.com/) and its `tsweb` library:

1. **`tsweb.Debugger`** — Admin routes (serial commands, DB backup, tailsql, pprof) are registered via `tsweb.Debugger(mux)`, which enforces `AllowDebugAccess`. This checks that the request originates from either:
   - A loopback IP (`127.0.0.1`, `::1`)
   - An authenticated Tailscale peer

   Unauthenticated LAN requests to `/debug/*` receive **403 Forbidden**.

2. **`featureGate`** — Destructive endpoints (e.g. `runs/clear`) are wrapped with `featureGate("VELOCITY_REPORT_ENABLE_DESTRUCTIVE_LIDAR_API", handler)`, requiring an explicit environment variable to be set. In production deployments this variable is unset, so the endpoint returns 404.

3. **Path-traversal protection** — PCAP file endpoints validate paths against `--lidar-pcap-dir` to prevent directory traversal attacks (see `internal/security`).

### Current limitations

- The HTTP listeners bind to all interfaces (`:8080`, `:8081`) by default. On a shared LAN, any host can reach the LiDAR tuning APIs.
- There is no per-request authentication on the public or LiDAR listeners. The threat model assumes a private deployment LAN with no untrusted hosts.
- The Svelte frontend makes unauthenticated API calls to the local server.

### Mitigations

For field deployments on shared or semi-trusted networks:

1. **Bind to specific interfaces** — Use `--listen 127.0.0.1:8080` and `--lidar-listen 127.0.0.1:8081` to restrict access to localhost, then expose via Tailscale only.
2. **Firewall rules** — Use `iptables`/`nftables` to restrict access to ports 8080/8081 to specific source IPs or the Tailscale interface (`tailscale0`).
3. **Tailscale ACLs** — Use Tailscale ACL policies to control which peers can reach the device.

## Future Work: User API Authentication

User-level API authentication is deferred. When needed, the planned approach is:

1. **Bearer token authentication** — A shared secret or API key for programmatic access to the HTTP APIs. Suitable for single-device deployments.
2. **Tailscale identity headers** — For Tailscale-proxied requests, use `Tailscale-User-Login` headers to identify the caller. This provides SSO-like auth without managing credentials.
3. **Scope-based authorisation** — Map authenticated identities to permission scopes (read, tune, admin) to enforce least-privilege access.

This work is tracked in [BACKLOG.md](../../BACKLOG.md) and [design-review-and-improvement-plan.md](../plans/design-review-and-improvement-plan.md).
