# LiDAR Network Configuration

**Status:** Proposed
**Author:** Architecture Team
**Created:** 2026-02-21
**Related:** `hesai_packet_structure.md`, `lidar_sidecar_overview.md`, `/docs/architecture/networking.md`

## Overview & Motivation

### Current State

The velocity.report LiDAR subsystem binds its UDP listener to a wildcard address (`:port`) at startup, accepting Hesai Pandar40P packets on whichever network interface they arrive. The listening port (default 2369), forwarding targets, and interface addresses are configured entirely through CLI flags on `cmd/radar` (the unified binary that hosts both radar and LiDAR subsystems). Changing any network parameter requires a process restart.

**Current CLI flags:**

```
-lidar-udp-port 2369              # Listening port
-lidar-forward                    # Enable packet forwarding
-lidar-forward-port 2368          # Forward destination port
-lidar-forward-addr localhost     # Forward destination address
-lidar-foreground-forward         # Enable foreground-only forwarding
-lidar-foreground-forward-port 2370
```

This model has three operational problems:

1. **No interface selection** — The listener binds to `0.0.0.0`, accepting packets from any NIC. On multi-homed hosts (e.g., Raspberry Pi with both eth0 for LiDAR and wlan0 for management), this is imprecise and prevents interface-specific diagnostics.
2. **No runtime diagnostics** — Operators cannot verify whether the Hesai sensor is reachable, which interface it is transmitting on, or whether UDP packets are actually arriving — without SSH access and `tcpdump`.
3. **No hot-reload** — Changing the listening port or interface requires restarting the service, interrupting data collection.

### Design Goals

- **Interface-aware binding**: Bind the UDP listener to a specific network interface (by name or IP address), not just a wildcard port
- **Network diagnostics API**: Enumerate local interfaces, report IP/gateway info, and probe whether Hesai traffic is arriving on expected ports
- **Live traffic verification**: Confirm whether UDP packets are being received and ingested without external tooling
- **Hot-reload without restart**: Allow interface/port changes to take effect without stopping the process or losing existing subscriber connections
- **Consistent with serial pattern**: Follow the `SerialPortManager` hot-reload architecture established for radar serial configuration
- **Privacy-first**: No traffic content inspection; only metadata (packet counts, source IPs, port numbers)

## Hesai Network Topology

### Standard Port Assignments

| Port | Direction         | Protocol | Purpose                                           |
| ---- | ----------------- | -------- | ------------------------------------------------- |
| 2368 | Sensor → Host     | UDP      | Primary point cloud data (legacy default)         |
| 2369 | Sensor → Host     | UDP      | Point cloud data (velocity.report default listen) |
| 2370 | Host → Downstream | UDP      | Foreground-only re-stream (reconstructed packets) |

The Hesai Pandar40P broadcasts UDP packets at ~1,400 packets/sec (~10 Mbps) on its configured data port. The sensor expects a dedicated ethernet segment (typically `192.168.1.0/24`) with the host NIC on the same subnet.

### Typical Deployment Topology

```
┌──────────────┐      Ethernet       ┌──────────────────────────┐
│ Hesai        │  192.168.1.201:2369 │ Raspberry Pi 4           │
│ Pandar40P    │ ──────────────────► │                          │
│              │                     │ eth0: 192.168.1.100/24   │
│              │                     │   └─ UDP :2369 listener  │
└──────────────┘                     │                          │
                                     │ wlan0: 192.168.0.50/24   │
                                     │   └─ Management/API      │
                                     │                          │
                                     │ tailscale0: 100.x.x.x   │
                                     │   └─ Remote admin        │
                                     └──────────────────────────┘
```

## Proposed Architecture

### Database Schema

A new `lidar_network_config` table stores the network binding configuration, following the `radar_serial_config` pattern:

```sql
CREATE TABLE lidar_network_config (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT    NOT NULL UNIQUE,
    interface_name  TEXT    NOT NULL DEFAULT '',
    bind_address    TEXT    NOT NULL DEFAULT '',
    udp_port        INTEGER NOT NULL DEFAULT 2369
                    CHECK (udp_port BETWEEN 1024 AND 65535),
    receive_buffer  INTEGER NOT NULL DEFAULT 4194304
                    CHECK (receive_buffer BETWEEN 65536 AND 67108864),
    enabled         INTEGER NOT NULL DEFAULT 0
                    CHECK (enabled IN (0, 1)),
    description     TEXT    NOT NULL DEFAULT '',
    sensor_model    TEXT    NOT NULL DEFAULT 'hesai-pandar40p'
                    CHECK (sensor_model LIKE 'hesai-%'),
    forward_enabled INTEGER NOT NULL DEFAULT 0
                    CHECK (forward_enabled IN (0, 1)),
    forward_address TEXT    NOT NULL DEFAULT 'localhost',
    forward_port    INTEGER NOT NULL DEFAULT 2368
                    CHECK (forward_port BETWEEN 1024 AND 65535),
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TRIGGER update_lidar_network_config_timestamp
    AFTER UPDATE ON lidar_network_config
    FOR EACH ROW
BEGIN
    UPDATE lidar_network_config
    SET updated_at = unixepoch()
    WHERE id = OLD.id;
END;
```

**Key fields:**

| Field            | Purpose                                                                                  |
| ---------------- | ---------------------------------------------------------------------------------------- |
| `interface_name` | Network interface name (e.g., `eth0`, `enp3s0`). Empty string = wildcard (all interfaces)|
| `bind_address`   | Explicit IP to bind (e.g., `192.168.1.100`). Empty string = derive from interface_name   |
| `udp_port`       | UDP port to listen on (default 2369). Restricted to unprivileged range                   |
| `receive_buffer` | OS socket receive buffer in bytes (default 4 MiB, max 64 MiB). Higher values reduce packet drops under burst load; 64 MiB ceiling is safe on Raspberry Pi 4 (1–8 GB RAM) |
| `sensor_model`   | Sensor identifier for parser selection. CHECK constraint: `LIKE 'hesai-%'`               |
| `forward_*`      | Packet forwarding target (mirrors `--lidar-forward-*` flags)                             |

**Bind address resolution:** When `interface_name` is set but `bind_address` is empty, the system resolves the interface's primary IPv4 address at bind time. When both are empty, the listener binds to `0.0.0.0` (wildcard). When `bind_address` is explicit, it is used directly regardless of `interface_name`.

### LiDAR Network Manager

A `LiDARNetworkManager` follows the `SerialPortManager` pattern — a hot-reload wrapper around the UDP listener with persistent API subscriptions and thread-safe configuration swaps.

```go
// LiDARNetworkManager wraps a UDPListener and enables hot-reloading of the
// network configuration. It delegates to the active listener via RWMutex and
// maintains persistent packet statistics across reloads.
type LiDARNetworkManager struct {
    mu       sync.RWMutex
    current  *network.UDPListener
    snapshot *NetworkConfigSnapshot
    closed   bool

    db      *db.DB
    factory UDPListenerFactory

    reloadMu sync.Mutex

    // Persistent stats survive listener reloads
    stats *network.PacketStats
}

// UDPListenerFactory constructs a UDPListener from the resolved bind address
// and configuration. Injected so tests can supply mock listeners.
// See internal/lidar/l1packets/network/listener.go for UDPListenerConfig.
type UDPListenerFactory func(cfg network.UDPListenerConfig) (*network.UDPListener, error)

// NetworkConfigSnapshot describes the active network configuration for API
// responses.
type NetworkConfigSnapshot struct {
    ConfigID      int    `json:"config_id,omitempty"`
    Name          string `json:"name,omitempty"`
    InterfaceName string `json:"interface_name"`
    BindAddress   string `json:"bind_address"`
    UDPPort       int    `json:"udp_port"`
    ReceiveBuffer int    `json:"receive_buffer"`
    Source        string `json:"source"` // "database", "cli-flags", "default"
}
```

**Reload strategy (mirrors serial pattern):**

1. Load enabled config from `lidar_network_config` table
2. Resolve bind address from interface name (if needed)
3. Compare resolved config against current snapshot
4. **Same address+port**: No-op — log "configuration unchanged"
5. **Different address or port**:
   a. Create new listener with factory (validate binding succeeds)
   b. Stop old listener (`Close()`)
   c. Swap under write lock
   d. Start new listener in background goroutine

**PacketStats persistence:** Unlike serial events, packet statistics are high-frequency (>1,000/sec) and must not be reset on reload. The manager owns a single `PacketStats` instance that is injected into each new listener, preserving throughput metrics across configuration changes.

### Network Diagnostics

The diagnostics subsystem provides three capabilities without requiring external tooling:

#### 1. Interface Enumeration

```go
// NetworkInterface describes a local network interface for the UI.
type NetworkInterface struct {
    Name       string   `json:"name"`        // e.g., "eth0"
    Addresses  []string `json:"addresses"`   // IPv4 and IPv6 CIDRs
    MAC        string   `json:"mac"`         // Hardware address
    Up         bool     `json:"up"`          // Interface is up
    Running    bool     `json:"running"`     // Interface has carrier
    MTU        int      `json:"mtu"`         // Maximum transmission unit
    IsLoopback bool     `json:"is_loopback"` // Loopback interface
}
```

Uses `net.Interfaces()` to enumerate NICs. Filters out loopback and down interfaces in the default response; include-all available via query parameter.

#### 2. Interface Detail with Gateway

For a selected interface, resolve additional network context:

```go
// NetworkInterfaceDetail extends NetworkInterface with routing information.
type NetworkInterfaceDetail struct {
    NetworkInterface
    Gateway    string `json:"gateway,omitempty"`    // Default gateway (from /proc/net/route on Linux)
    SubnetMask string `json:"subnet_mask,omitempty"`
}
```

Gateway resolution reads `/proc/net/route` on Linux (the target platform). On other platforms, this field is omitted.

#### 3. Traffic Probe

A non-destructive probe checks whether Hesai UDP traffic is arriving on a given interface and port:

```go
// TrafficProbeResult reports whether UDP traffic was detected.
type TrafficProbeResult struct {
    Port           int           `json:"port"`
    InterfaceName  string        `json:"interface_name"`
    BindAddress    string        `json:"bind_address"`
    PacketsFound   int           `json:"packets_found"`
    BytesReceived  int64         `json:"bytes_received"`
    SourceAddrs    []string      `json:"source_addresses"`
    Duration       time.Duration `json:"probe_duration_ms"`
    ListenerActive bool          `json:"listener_active"` // True if main listener is on this port
    Error          string        `json:"error,omitempty"`
}
```

**Probe behaviour:**

- Opens a temporary UDP socket on the specified interface:port for a short window (default 2 seconds)
- Counts arriving packets and records source IP addresses
- If the main listener is already bound to that port, reports from live `PacketStats` instead of opening a second socket (avoids `EADDRINUSE`)
- Returns packet count, byte count, and list of unique source addresses
- **Privacy note:** Only metadata is reported — no packet content inspection

**Active-listener warning:** If the traffic probe targets a port currently held by the `LiDARNetworkManager`, the API returns the live statistics from `PacketStats.GetLatestSnapshot()` with `listener_active: true`, rather than attempting to bind a second socket.

### API Endpoints

New endpoints under `/api/lidar/network/`, mirroring the `/api/serial/` pattern:

#### Configuration CRUD

| Method   | Path                            | Description                   |
| -------- | ------------------------------- | ----------------------------- |
| `GET`    | `/api/lidar/network/configs`    | List all network configs      |
| `POST`   | `/api/lidar/network/configs`    | Create a new config           |
| `GET`    | `/api/lidar/network/configs/:id`| Get config by ID              |
| `PUT`    | `/api/lidar/network/configs/:id`| Update config                 |
| `DELETE` | `/api/lidar/network/configs/:id`| Delete config                 |

#### Diagnostics & Control

| Method | Path                             | Description                                            |
| ------ | -------------------------------- | ------------------------------------------------------ |
| `GET`  | `/api/lidar/network/interfaces`  | Enumerate local network interfaces with IP/MAC/status  |
| `GET`  | `/api/lidar/network/interfaces/:name` | Interface detail with gateway and subnet info     |
| `POST` | `/api/lidar/network/probe`       | Traffic probe — check for UDP packets on port/interface |
| `POST` | `/api/lidar/network/reload`      | Apply enabled config (hot-reload listener)             |
| `GET`  | `/api/lidar/network/status`      | Current listener status + live packet stats            |

#### Request/Response Examples

**POST `/api/lidar/network/probe`**

```json
{
  "interface_name": "eth0",
  "port": 2369,
  "duration_ms": 2000
}
```

Response:

```json
{
  "port": 2369,
  "interface_name": "eth0",
  "bind_address": "192.168.1.100",
  "packets_found": 2847,
  "bytes_received": 3597054,
  "source_addresses": ["192.168.1.201"],
  "probe_duration_ms": 2000,
  "listener_active": false
}
```

**GET `/api/lidar/network/status`**

```json
{
  "active": true,
  "config": {
    "config_id": 1,
    "name": "Hesai eth0",
    "interface_name": "eth0",
    "bind_address": "192.168.1.100",
    "udp_port": 2369,
    "receive_buffer": 4194304,
    "source": "database"
  },
  "traffic": {
    "packets_per_sec": 1412.3,
    "mb_per_sec": 1.69,
    "points_per_sec": 564920.0,
    "dropped_recent": 0,
    "parse_enabled": true
  }
}
```

### Settings UI

A new settings page at `/settings/lidar-network` (under the `(constrained)` route group) extends the existing hardware settings pattern:

```
/settings
  ├── /serial          ← Radar serial configuration (existing)
  └── /lidar-network   ← LiDAR network configuration (new)
```

**Page sections:**

1. **Interface selector** — Dropdown populated from `GET /api/lidar/network/interfaces`. Shows name, IPv4 addresses, link state. Selection auto-populates the bind address field.

2. **Configuration form** — Name, interface, port, receive buffer, sensor model, forwarding options. Save creates/updates a `lidar_network_config` row.

3. **Traffic probe** — "Test Connection" button sends `POST /api/lidar/network/probe` for the selected interface and port. Displays packet count, source addresses, and throughput. Shows warning badge if probe finds no packets.

4. **Live status** — When a config is active, displays real-time packet stats from `GET /api/lidar/network/status`: packets/sec, MB/sec, points/sec, dropped count. Auto-refreshes on a 2-second interval.

5. **Reload control** — "Apply Configuration" button sends `POST /api/lidar/network/reload`. Displays success/failure result inline. Warns if changing interface/port while actively ingesting.

## Implementation Plan

### Phase 1: Database & API Foundation

| Task                                                  | Effort |
| ----------------------------------------------------- | ------ |
| Migration `000030_create_lidar_network_config`        | S      |
| Update `schema.sql` with new table                    | S      |
| `internal/db/lidar_network_config.go` — CRUD          | M      |
| `internal/db/lidar_network_config_test.go`            | M      |

### Phase 2: Network Diagnostics

| Task                                                  | Effort |
| ----------------------------------------------------- | ------ |
| `internal/api/network_diagnostics.go` — interface enum, gateway, probe | M      |
| `internal/api/network_diagnostics_test.go`            | M      |

### Phase 3: Hot-Reload Manager

| Task                                                  | Effort |
| ----------------------------------------------------- | ------ |
| `internal/api/lidar_network_reload.go` — LiDARNetworkManager | L      |
| `internal/api/lidar_network_reload_test.go`           | L      |
| Wire into `server.go` route registration              | S      |
| CLI flag migration: honour DB config over flags        | M      |

### Phase 4: Settings UI

| Task                                                  | Effort |
| ----------------------------------------------------- | ------ |
| `web/src/routes/(constrained)/settings/lidar-network/+page.svelte` | L      |
| `web/src/lib/api.ts` — lidar network API functions    | S      |
| Settings page link from `/settings`                   | S      |

### Phase 5: Integration & Testing

| Task                                                  | Effort |
| ----------------------------------------------------- | ------ |
| End-to-end test: probe → configure → reload → verify  | M      |
| Coverage targets: 90%+ across new files               | M      |

**Size key:** S = ½ day, M = 1 day, L = 2 days

## Open Questions

1. **Multiple listeners**: Should the system support binding to multiple interfaces simultaneously (e.g., primary + backup), or is single-config-enabled sufficient?
   - **Recommendation:** Single enabled config (matching serial pattern). Multi-listener adds complexity with minimal benefit for the current deployment model.

2. **Forwarding hot-reload**: Should forwarding targets (port 2368/2370) be independently hot-reloadable, or tied to the main listener config?
   - **Recommendation:** Include forwarding config in the same row. Forwarding changes require listener restart anyway (new forwarder goroutine).

3. **Privileged ports**: The schema restricts `udp_port` to 1024–65535. Should we support binding to ports below 1024 (e.g., port 443 for tunnel scenarios)?
   - **Recommendation:** Keep the unprivileged restriction. The Hesai default ports (2368/2369) are well above 1024, and privileged ports require root or `CAP_NET_BIND_SERVICE`.

4. **Interface-change detection**: Should the system detect interface state changes (link down/up, IP change via DHCP) and auto-reload?
   - **Recommendation:** Defer. Manual reload via API/UI is sufficient for v1. Interface monitoring (via netlink on Linux) is a natural extension for a future iteration.
