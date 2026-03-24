# Multi-Sensor Capabilities API Plan

- **Status:** In Progress
- **Layers:** API, Frontend, cmd/radar
- **Canonical architecture:** `internal/api/server.go`, `cmd/radar/capabilities.go`

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
      "status": "receiving",
      "last_reading_at": "2026-03-24T06:45:12Z"
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
      "status": "receiving",
      "last_reading_at": "2026-03-24T06:45:12Z"
    },
    "ops243_rear": {
      "enabled": true,
      "status": "stale",
      "last_reading_at": "2026-03-23T02:11:44Z"
    }
  },
  "lidar": {
    "hesai": {
      "enabled": true,
      "status": "receiving",
      "last_reading_at": "2026-03-24T07:38:59Z",
      "sweep": true
    }
  }
}
```

### Why named objects over lists

| Concern | Named objects | Lists |
|---------|--------------|-------|
| Lookup by identity | `caps.radar["ops243_front"]` — O(1), stable key | Must scan by name field |
| Diffing across polls | Keys are stable — trivial Svelte keying | Index shifts on removal |
| Go type | `map[string]SensorStatus` — idiomatic | `[]SensorStatus` + Name field |
| Uniqueness | Structural — keys unique by definition | Must validate no duplicates |
| Ordering | Maps unordered (UI sorts by name) | Ordered but meaningless |

Named objects win on every axis relevant here.

### Field definitions

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | `bool` | Sensor channel was activated at startup |
| `status` | `string` | Runtime state: `disabled`, `starting`, `receiving`, `stale`, `error` |
| `last_reading_at` | `string \| null` | ISO 8601 timestamp of last data received; `null` = never |
| `sweep` | `bool` | (lidar only) Sweep/auto-tuner operational |

### State machine

```
disabled → starting → receiving ⇄ stale
                  ↘ error
```

### Empty map semantics

`{}` = no sensors of this class configured. A disabled sensor still appears in
the map with `"enabled": false`.

## 3. Go Types

```go
// SensorStatus is the per-sensor health snapshot.
type SensorStatus struct {
    Enabled       bool    `json:"enabled"`
    Status        string  `json:"status"`
    LastReadingAt *string `json:"last_reading_at"`
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

`map[string]T` marshals to `{}` when empty — clean, no `null` vs `[]` ambiguity.

## 4. Frontend Types

```typescript
interface SensorStatus {
    enabled: boolean;
    status: 'disabled' | 'starting' | 'receiving' | 'stale' | 'error';
    last_reading_at: string | null;
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
    Object.values($c.lidar).some(s => s.enabled)
);
```

## 5. Migration Path

| Old field | New location |
|-----------|-------------|
| `radar: true` | `radar.default.enabled = true` |
| `lidar.enabled` | `lidar.default.enabled` (or empty map) |
| `lidar.state` | `lidar.default.status` |
| `lidar_sweep` | `lidar.default.sweep` |

Frontend ships embedded in the binary — both sides change atomically. No
backwards-compatibility shim needed.

## 6. Sensor naming

Keys are stable, human-assigned identifiers. Today the single radar/lidar uses
`"default"`. Multi-sensor deployments use descriptive names:

- `"ops243_front"`, `"ops243_rear"` — by model and position
- `"hesai"`, `"hesai_kerb"` — by model and role

Names come from CLI flags or config at startup. The server rejects duplicate
names.

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

- [ ] Replace `Capabilities`, `LidarCapability` structs in `internal/api/server.go`
      with new `SensorStatus`, `LidarSensorStatus`, `Capabilities` types
- [ ] Update `showCapabilities` default in `internal/api/server_admin.go`
- [ ] Rewrite `capabilitiesProvider` in `cmd/radar/capabilities.go` to populate
      `map[string]SensorStatus` / `map[string]LidarSensorStatus`
- [ ] Update `internal/api/capabilities_test.go`
- [ ] Update `cmd/radar/capabilities_test.go`

### Frontend (Svelte/TypeScript)

- [ ] Update `Capabilities`, `LidarCapability` types in `web/src/lib/api.ts`
- [ ] Update default capabilities and derived stores in
      `web/src/lib/stores/capabilities.ts`
- [ ] Update layout gate in `web/src/routes/+layout.svelte` to use
      `Object.values($capabilities.lidar).some(s => s.enabled)`
- [ ] Update `web/src/lib/stores/capabilities.test.ts`

### Validation

- [ ] `make lint-go && make test-go`
- [ ] `make lint-web && make test-web`
- [ ] `make build-web`
