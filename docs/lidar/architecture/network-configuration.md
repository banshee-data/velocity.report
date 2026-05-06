# LiDAR network configuration

- **Status:** Proposed
- **Related:** [`HESAI_PACKET_FORMAT.md`](../../../data/structures/HESAI_PACKET_FORMAT.md), [`LIDAR_ARCHITECTURE.md`](./LIDAR_ARCHITECTURE.md), [`networking.md`](../../radar/architecture/networking.md)

Architecture for interface-aware UDP binding, network diagnostics, hot-reload configuration, and a settings UI for the LiDAR sensor network layer.

## Overview & motivation

### Current state

The velocity.report LiDAR subsystem binds its UDP listener to a wildcard address (`:port`) at startup, accepting Hesai Pandar40P packets on whichever network interface they arrive. The listening port (default 2369), forwarding targets, and interface addresses are configured entirely through CLI flags on [cmd/radar](../../../cmd/radar) (the unified binary that hosts both radar and LiDAR subsystems). Changing any network parameter requires a process restart.

**Current CLI flags:**

```
-lidar-udp-port 2369              # Listening port
-lidar-udp-rcv-buf 4194304        # Socket receive buffer size in bytes
-lidar-forward                    # Enable packet forwarding
-lidar-forward-port 2368          # Forward destination port
-lidar-forward-addr localhost     # Forward destination address
-lidar-foreground-forward         # Enable foreground-only forwarding
-lidar-foreground-forward-port 2370
```

This model has three operational problems:

1. **No interface selection**: The listener binds to `0.0.0.0`, accepting packets from any NIC. On multi-homed hosts (e.g., Raspberry Pi with both eth0 for LiDAR and wlan0 for management), this is imprecise and prevents interface-specific diagnostics.
2. **No runtime diagnostics**: Operators cannot verify whether the Hesai sensor is reachable, which interface it is transmitting on, or whether UDP packets are actually arriving; without SSH access and `tcpdump`.
3. **No hot-reload**: Changing the listening port or interface requires restarting the service, interrupting data collection.

### Design goals

- **Interface-aware binding**: Bind the UDP listener to a specific network interface (by name or IP address), not just a wildcard port
- **Network diagnostics API**: Enumerate local interfaces, report IP/gateway info, and probe whether Hesai traffic is arriving on expected ports
- **Live traffic verification**: Confirm whether UDP packets are being received and ingested without external tooling
- **Hot-reload without restart**: Allow interface/port changes to take effect without stopping the process or losing existing subscriber connections
- **Consistent with serial pattern**: Follow the `SerialPortManager` hot-reload architecture established for radar serial configuration
- **Privacy-first**: No traffic content inspection; only metadata (packet counts, source IPs, port numbers)

## Hesai network topology

### Standard port assignments

| Port | Direction         | Protocol | Purpose                                           |
| ---- | ----------------- | -------- | ------------------------------------------------- |
| 2368 | Sensor → Host     | UDP      | Primary point cloud data (legacy default)         |
| 2369 | Sensor → Host     | UDP      | Point cloud data (velocity.report default listen) |
| 2370 | Host → Downstream | UDP      | Foreground-only re-stream (reconstructed packets) |

The Hesai Pandar40P broadcasts UDP packets at ~1,400 packets/sec (~10 Mbps) on its configured data port. The sensor expects a dedicated ethernet segment (typically `192.168.1.0/24`) with the host NIC on the same subnet.

### Typical deployment topology

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
                                     │ tailscale0: 100.x.x.x    │
                                     │   └─ Remote admin        │
                                     └──────────────────────────┘
```

## Proposed architecture

### Database schema

A new `lidar_network_config` table stores the network binding configuration, following the `radar_serial_config` pattern:

> **Source:** Migration in [internal/db/migrations/](../../../internal/db/migrations) (when implemented). Table `lidar_network_config` with columns: id, name, interface_name, bind_address, udp_port (default 2369, CHECK 1024–65535), receive_buffer (default 4 MiB, CHECK 64 KiB – 64 MiB), enabled, description, sensor_model (CHECK `LIKE 'hesai-%'`), forward_enabled, forward_address, forward_port, created_at, updated_at. An AFTER UPDATE trigger maintains `updated_at`.

**Key fields:**

| Field            | Purpose                                                                                                                                                                  |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `interface_name` | Network interface name (e.g., `eth0`, `enp3s0`). Empty string = wildcard (all interfaces)                                                                                |
| `bind_address`   | Explicit IP to bind (e.g., `192.168.1.100`). Empty string = derive from interface_name                                                                                   |
| `udp_port`       | UDP port to listen on (default 2369). Restricted to unprivileged range                                                                                                   |
| `receive_buffer` | OS socket receive buffer in bytes (default 4 MiB, max 64 MiB). Higher values reduce packet drops under burst load; 64 MiB ceiling is safe on Raspberry Pi 4 (1–8 GB RAM) |
| `sensor_model`   | Sensor identifier for parser selection. CHECK constraint: `LIKE 'hesai-%'`                                                                                               |
| `forward_*`      | Packet forwarding target (configured via `l1.forward_port` and `l1.foreground_forward_port` in the tuning config file)                                                   |

**Bind address resolution:** When `interface_name` is set but `bind_address` is empty, the system resolves the interface's primary IPv4 address at bind time. When both are empty, the listener binds to `0.0.0.0` (wildcard). When `bind_address` is explicit, it is used directly regardless of `interface_name`.

### LiDAR network manager

A `LiDARNetworkManager` follows the `SerialPortManager` pattern: a hot-reload wrapper around the UDP listener with persistent API subscriptions and thread-safe configuration swaps.

> **Source:** `internal/api/lidar_network_reload.go` (when implemented). `LiDARNetworkManager` holds the active `UDPListener` behind an RWMutex, a `NetworkConfigSnapshot` of the running config, a DB handle, and an injected `UDPListenerFactory` for testability. `PacketStats` are owned by the manager and persist across listener reloads. `NetworkConfigSnapshot` carries config_id, name, interface_name, bind_address, udp_port, receive_buffer, and source (`"database"`, `"cli-flags"`, or `"default"`).

**Reload strategy (mirrors serial pattern):**

1. Load enabled config from `lidar_network_config` table
2. Resolve bind address from interface name (if needed)
3. Compare resolved config against current snapshot
4. **Same address+port**: No-op; log "configuration unchanged"
5. **Different address or port**:
   a. Create new listener with factory (validate binding succeeds)
   b. Stop old listener (`Close()`)
   c. Swap under write lock
   d. Start new listener in background goroutine

**PacketStats persistence:** Unlike serial events, packet statistics are high-frequency (>1,000/sec) and must not be reset on reload. The manager owns a single `PacketStats` instance that is injected into each new listener, preserving throughput metrics across configuration changes.

### Network diagnostics

The diagnostics subsystem provides three capabilities without requiring external tooling:

#### 1. Interface enumeration

> **Source:** `internal/api/network_diagnostics.go` (when implemented). `NetworkInterface` struct with fields: Name, Addresses (IPv4/IPv6 CIDRs), MAC, Up, Running, MTU, and IsLoopback.

Uses `net.Interfaces()` to enumerate NICs. Filters out loopback and down interfaces in the default response; include-all available via query parameter.

#### 2. Interface detail with gateway

For a selected interface, resolve additional network context:

> **Source:** Same file. `NetworkInterfaceDetail` embeds `NetworkInterface` and adds Gateway (from `/proc/net/route` on Linux) and SubnetMask.

Gateway resolution reads `/proc/net/route` on Linux (the target platform). On other platforms, this field is omitted.

#### 3. Traffic probe

A non-destructive probe checks whether Hesai UDP traffic is arriving on a given interface and port:

> **Source:** Same file. `TrafficProbeResult` struct with fields: Port, InterfaceName, BindAddress, PacketsFound, BytesReceived, SourceAddrs, Duration, ListenerActive (true when the main listener already holds this port), and Error.

**Probe behaviour:**

- Opens a temporary UDP socket on the specified interface:port for a short window (default 2 seconds)
- Counts arriving packets and records source IP addresses
- If the main listener is already bound to that port, reports from live `PacketStats` instead of opening a second socket (avoids `EADDRINUSE`)
- Returns packet count, byte count, and list of unique source addresses
- **Privacy note:** Only metadata is reported; no packet content inspection

**Active-listener warning:** If the traffic probe targets a port currently held by the `LiDARNetworkManager`, the API returns the live statistics from `PacketStats.GetLatestSnapshot()` with `listener_active: true`, rather than attempting to bind a second socket.

### API endpoints

New endpoints under `/api/lidar/network/`, mirroring the `/api/serial/` pattern:

#### Configuration CRUD

| Method   | Path                             | Description              |
| -------- | -------------------------------- | ------------------------ |
| `GET`    | `/api/lidar/network/configs`     | List all network configs |
| `POST`   | `/api/lidar/network/configs`     | Create a new config      |
| `GET`    | `/api/lidar/network/configs/:id` | Get config by ID         |
| `PUT`    | `/api/lidar/network/configs/:id` | Update config            |
| `DELETE` | `/api/lidar/network/configs/:id` | Delete config            |

#### Diagnostics & control

| Method | Path                                  | Description                                            |
| ------ | ------------------------------------- | ------------------------------------------------------ |
| `GET`  | `/api/lidar/network/interfaces`       | Enumerate local network interfaces with IP/MAC/status  |
| `GET`  | `/api/lidar/network/interfaces/:name` | Interface detail with gateway and subnet info          |
| `POST` | `/api/lidar/network/probe`            | Traffic probe: check for UDP packets on port/interface |
| `POST` | `/api/lidar/network/reload`           | Apply enabled config (hot-reload listener)             |
| `GET`  | `/api/lidar/network/status`           | Current listener status + live packet stats            |

#### Request/Response examples

**POST `/api/lidar/network/probe`**

The probe request body contains:

| Field            | Type   | Purpose                        |
| ---------------- | ------ | ------------------------------ |
| `interface_name` | string | Network interface to probe     |
| `port`           | int    | UDP port to listen on          |
| `duration_ms`    | int    | Probe duration in milliseconds |

Response:

The probe response contains:

| Field               | Type     | Purpose                                 |
| ------------------- | -------- | --------------------------------------- |
| `port`              | int      | Probed UDP port                         |
| `interface_name`    | string   | Probed interface                        |
| `bind_address`      | string   | Resolved bind address                   |
| `packets_found`     | int      | Number of packets received              |
| `bytes_received`    | int      | Total bytes received                    |
| `source_addresses`  | string[] | Observed source IP addresses            |
| `probe_duration_ms` | int      | Actual probe duration in milliseconds   |
| `listener_active`   | bool     | Whether a persistent listener is active |

**GET `/api/lidar/network/status`**

The status response contains a top-level `active` boolean, a `config` object with the active listener configuration (`config_id`, `name`, `interface_name`, `bind_address`, `udp_port`, `receive_buffer`, `source`), and a `traffic` object with live statistics (`packets_per_sec`, `mb_per_sec`, `points_per_sec`, `dropped_recent`, `parse_enabled`).

### Settings UI

A new settings page at `/settings/lidar-network` (under the `(constrained)` route group) extends the existing hardware settings pattern:

```
/settings
  ├── /serial          ← Radar serial configuration (existing)
  └── /lidar-network   ← LiDAR network configuration (new)
```

**Page sections:**

1. **Interface selector**: Dropdown populated from `GET /api/lidar/network/interfaces`. Shows name, IPv4 addresses, link state. Selection auto-populates the bind address field.

2. **Configuration form**: Name, interface, port, receive buffer, sensor model, forwarding options. Save creates/updates a `lidar_network_config` row.

3. **Traffic probe**: "Test Connection" button sends `POST /api/lidar/network/probe` for the selected interface and port. Displays packet count, source addresses, and throughput. Shows warning badge if probe finds no packets.

4. **Live status**: When a config is active, displays real-time packet stats from `GET /api/lidar/network/status`: packets/sec, MB/sec, points/sec, dropped count. Auto-refreshes on a 2-second interval.

5. **Reload control**: "Apply Configuration" button sends `POST /api/lidar/network/reload`. Displays success/failure result inline. Warns if changing interface/port while actively ingesting.

## Implementation plan

### Phase 1: database & API foundation

| Task                                           | Effort |
| ---------------------------------------------- | ------ |
| Migration `000030_create_lidar_network_config` | S      |
| Update `schema.sql` with new table             | S      |
| `internal/db/lidar_network_config.go`: CRUD    | M      |
| `internal/db/lidar_network_config_test.go`     | M      |

### Phase 2: network diagnostics

| Task                                                                  | Effort |
| --------------------------------------------------------------------- | ------ |
| `internal/api/network_diagnostics.go`: interface enum, gateway, probe | M      |
| `internal/api/network_diagnostics_test.go`                            | M      |

### Phase 3: hot-reload manager

| Task                                                        | Effort |
| ----------------------------------------------------------- | ------ |
| `internal/api/lidar_network_reload.go`: LiDARNetworkManager | L      |
| `internal/api/lidar_network_reload_test.go`                 | L      |
| Wire into `server.go` route registration                    | S      |
| CLI flag migration: honour DB config over flags             | M      |

### Phase 4: settings UI

| Task                                                                           | Effort |
| ------------------------------------------------------------------------------ | ------ |
| `web/src/routes/(constrained)/settings/lidar-network/+page.svelte`             | L      |
| [web/src/lib/api.ts](../../../web/src/lib/api.ts): lidar network API functions | S      |
| Settings page link from `/settings`                                            | S      |

### Phase 5: integration & testing

| Task                                                  | Effort |
| ----------------------------------------------------- | ------ |
| Integration test: probe → configure → reload → verify | M      |
| Coverage targets: 90%+ across new files               | M      |

**Size key:** S = ½ day, M = 1 day, L = 2 days

## Resolved design questions

| Question                         | Resolution                                                                                                                                              |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Allow privileged ports (< 1024)? | No. Schema restricts `udp_port` to 1024–65535. Hesai defaults (2368/2369) are well above 1024; privileged ports require root or `CAP_NET_BIND_SERVICE`. |

## Open questions

1. **Multiple listeners**: Should the system support binding to multiple interfaces simultaneously (e.g., primary + backup), or is single-config-enabled sufficient?
   - **Recommendation:** Single enabled config (matching serial pattern). Multi-listener adds complexity with minimal benefit for the current deployment model.

2. **Forwarding hot-reload**: Should forwarding targets (port 2368/2370) be independently hot-reloadable, or tied to the main listener config?
   - **Recommendation:** Include forwarding config in the same row. Forwarding changes require listener restart anyway (new forwarder goroutine).

3. **Interface-change detection**: Should the system detect interface state changes (link down/up, IP change via DHCP) and auto-reload?
   - **Recommendation:** Defer. Manual reload via API/UI is sufficient for v1. Interface monitoring (via netlink on Linux) is a natural extension for a future iteration.
