# Multi-Sensor Capabilities API Plan

- **Status:** Complete
- **Layers:** API, Frontend, cmd/radar
- **Canonical:** [web-frontend-consolidation.md](../ui/web-frontend-consolidation.md)

Redesign `/api/capabilities` to support multiple named sensors per class,
future-proofing for deployments with more than one radar or LiDAR unit.

## 1. Current Format

```json
{
  "radar": true,
  "lidar": { "enabled": false, "state": "disabled" },
  "lidar_sweep": false
}
```

Flat structure — one boolean for radar, one object for LiDAR. No room for a
second sensor of either class without a breaking change.

## 2. Target Format

Two top-level keys — `radar` and `lidar` — each a **named object**
(keys are stable, human-assigned sensor names). No `_sensors` suffix so path
access stays light: `$.lidar.hesai.enabled`.

### Single-sensor deployment (today)

```json
{
  "radar": {
    "default": {
      "enabled": true,
      "status": "receiving"
    }
  },
  "lidar": {}
}
```

### Multi-sensor deployment (future)

```json
{
  "radar": {
    "ops243_front": {
      "enabled": true,
      "status": "receiving"
    },
    "ops243_rear": {
      "enabled": true,
      "status": "stale"
    }
  },
  "lidar": {
    "hesai": {
      "enabled": true,
      "status": "ready",
      "sweep": true
    }
  }
}
```

### Why named objects over lists

| Concern              | Named objects                                   | Lists                         |
| -------------------- | ----------------------------------------------- | ----------------------------- |
| Lookup by identity   | `caps.radar["ops243_front"]` — O(1), stable key | Must scan by name field       |
| Diffing across polls | Keys are stable — trivial Svelte keying         | Index shifts on removal       |
| Go type              | `map[string]SensorStatus` — idiomatic           | `[]SensorStatus` + Name field |
| Uniqueness           | Structural — keys unique by definition          | Must validate no duplicates   |
| Ordering             | Maps unordered (UI sorts by name)               | Ordered but meaningless       |

Named objects win on every axis relevant here.

### Field definitions

| Field     | Type     | Description                                                                   |
| --------- | -------- | ----------------------------------------------------------------------------- |
| `enabled` | `bool`   | Sensor channel was activated at startup                                       |
| `status`  | `string` | Runtime state: `disabled`, `starting`, `ready`, `receiving`, `stale`, `error` |
| `sweep`   | `bool`   | (lidar only) Sweep/auto-tuner operational                                     |

### State machine

```
disabled → starting → ready → receiving ⇄ stale
                  ↘ error
```

### Empty map semantics

`{}` = no sensors of this class are currently configured or active for this
sensor class. Providers may omit disabled sensors from the map, so a missing
entry is treated the same as a disabled or unconfigured sensor.

## 3. Go Types

```go
// SensorStatus is the per-sensor health snapshot.
type SensorStatus struct {
    Enabled bool   `json:"enabled"`
    Status  string `json:"status"`
}

// LidarSensorStatus extends SensorStatus with lidar-specific fields.
type LidarSensorStatus struct {
    SensorStatus
    Sweep bool `json:"sweep"`
}

// Capabilities is the JSON shape returned by /api/capabilities.
type Capabilities struct {
    Radar map[string]SensorStatus      `json:"radar"`
    Lidar map[string]LidarSensorStatus `json:"lidar"`
}
```

Non-nil empty `map[string]T` values marshal to `{}`. The handler normalises
provider nil maps to empty maps before encoding, so the public contract never
emits `null` for `radar` or `lidar`.

## 4. Frontend Types

```typescript
interface SensorStatus {
  enabled: boolean;
  status: "disabled" | "starting" | "ready" | "receiving" | "stale" | "error";
}

interface LidarSensorStatus extends SensorStatus {
  sweep: boolean;
}

interface Capabilities {
  radar: Record<string, SensorStatus>;
  lidar: Record<string, LidarSensorStatus>;
}
```

### Convenience derivations

```typescript
const anyLidarEnabled = derived(capabilities, ($c) =>
  Object.values($c.lidar).some((s) => s.enabled),
);
```

## 5. Migration Path

| Old field       | New location                           |
| --------------- | -------------------------------------- |
| `radar: true`   | `radar.default.enabled = true`         |
| `lidar.enabled` | `lidar.default.enabled` (or empty map) |
| `lidar.state`   | `lidar.default.status`                 |
| `lidar_sweep`   | `lidar.default.sweep`                  |

Frontend ships embedded in the binary — both sides change atomically. No
backwards-compatibility shim needed.

## 6. Sensor naming

Keys are stable, human-assigned identifiers. Today the single radar/lidar uses
`"default"`. Multi-sensor deployments use descriptive names:

- `"ops243_front"`, `"ops243_rear"` — by model and position
- `"hesai"`, `"hesai_kerb"` — by model and role

Current runtime providers emit the single built-in sensor key `"default"`.
Future multi-sensor CLI/config work should assign these names at startup and
validate uniqueness before constructing the capabilities maps. The response
shape already enforces unique keys structurally, but named-sensor input parsing
and duplicate-name validation are not implemented in this PR.

## 7. Future extensibility

A third sensor class (thermal, ultrasonic) is just another top-level key:

```json
{
  "radar": { ... },
  "lidar": { ... },
  "thermal": { ... }
}
```

Each class gets its own extended status type if it has class-specific fields.

## 8. Implementation Checklist

### Backend (Go)

- [x] Replace `Capabilities`, `LidarCapability` structs in `internal/api/server.go`
      with new `SensorStatus`, `LidarSensorStatus`, `Capabilities` types
- [x] Update `showCapabilities` default in `internal/api/server_admin.go`
- [x] Rewrite `capabilitiesProvider` in `internal/cmd/server/capabilities.go` to populate
      `map[string]SensorStatus` / `map[string]LidarSensorStatus`
- [x] Update `internal/api/capabilities_test.go`
- [x] Update `internal/cmd/server/capabilities_test.go`

### Frontend (Svelte/TypeScript)

- [x] Update `Capabilities`, `LidarCapability` types in `web/src/lib/api.ts`
- [x] Update default capabilities and derived stores in
      `web/src/lib/stores/capabilities.ts`
- [x] Update layout gate in `web/src/routes/+layout.svelte` to use
      `Object.values($capabilities.lidar).some(s => s.enabled)`
- [x] Update `web/src/lib/stores/capabilities.test.ts`

### Validation

- [x] `make lint-go && make test-go`
- [x] `make lint-web && make test-web`
- [x] `make build-web`
