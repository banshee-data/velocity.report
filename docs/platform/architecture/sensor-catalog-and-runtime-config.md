# Sensor catalog and runtime configuration

- **Status:** Proposed
- **Layers:** Cross-cutting (radar, LiDAR, API, database, settings UI)
- **Related:** [../../radar/architecture/serial-configuration-ui.md](../../radar/architecture/serial-configuration-ui.md), [../../lidar/architecture/network-configuration.md](../../lidar/architecture/network-configuration.md), [../../lidar/architecture/multi-model-ingestion-and-configuration.md](../../lidar/architecture/multi-model-ingestion-and-configuration.md)

Cross-cutting architecture for describing sensor models once while keeping host-specific runtime configuration in the correct radar and LiDAR tables.

## Problem

The current embedded catalog in `internal/api/sensor_models.json` is honest but narrow: it describes two radar models with serial-specific fields. That shape works while every entry is an OPS243 variant, but it starts to fray as soon as LiDAR models arrive with vendor-specific UDP defaults, telemetry ports, parser identities, and interface-sensitive runtime bindings.

The wrong response would be to keep bolting more optional fields onto the same flat record until every model carries a suitcase full of mostly-empty keys. The right response is to separate three concerns clearly: model identity, ingest defaults and constraints, and installation-specific runtime choices.

## ADR

### Decision

Adopt one shared **sensor catalog** for all supported hardware models, but keep **runtime configuration** in family-specific tables and managers.

The catalog answers: what sensor model is this, what family does it belong to, what capabilities does it expose, and what ingest profile does it expect?

Runtime configuration answers: how is this particular installation talking to that sensor on this machine today?

### Context

- Radar models currently need serial defaults such as baud rate and initialisation commands.
- LiDAR models such as Hesai, Velodyne, and Ouster need network-facing defaults such as data port, telemetry port, parser selection, receive-buffer guidance, and transport constraints.
- The repo already trends toward separate runtime configuration surfaces: [serial-configuration-ui.md](../../radar/architecture/serial-configuration-ui.md) for radar serial settings and [network-configuration.md](../../lidar/architecture/network-configuration.md) for LiDAR listener binding.
- Contributors need one durable answer for where sensor facts live and where operator choices live.

### Alternatives considered

| Option                                                                              | Outcome                                                                                       | Decision |
| ----------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- | -------- |
| Keep the current radar-only flat record and add more optional LiDAR fields          | Readable for no one; every new family increases sparsity and confusion                        | Rejected |
| Maintain separate radar and LiDAR model catalogs                                    | Duplicates concepts such as vendor, family, capabilities, and parser/ingest identity          | Rejected |
| Put host-specific interface names, bind addresses, and enabled flags in the catalog | Turns model metadata into installation state and makes embedded defaults look like live truth | Rejected |
| Shared catalog plus family-specific runtime config tables                           | Keeps model identity DRY and runtime state honest                                             | Selected |

### Consequences

- The current `sensor_models.json` concept should evolve into a shared sensor catalog rather than remain radar-only.
- `radar_serial_config` and `lidar_network_config` remain the correct homes for live operator settings.
- UI code can ask the catalog what configuration surface a model uses instead of inferring transport from family names.

## System boundary diagram

```text
                 +--------------------------------------+
                 | Shared sensor catalog                |
                 | embedded JSON / Go structs           |
                 | - model identity                     |
                 | - capabilities                       |
                 | - ingest defaults and constraints    |
                 +------------------+-------------------+
                                    |
                +-------------------+-------------------+
                |                                       |
                v                                       v
   +-----------------------------+         +-----------------------------+
   | Radar runtime config        |         | LiDAR runtime config        |
   | radar_serial_config         |         | lidar_network_config        |
   | - chosen port               |         | - interface / bind address  |
   | - chosen baud / parity      |         | - chosen UDP ports          |
   | - enabled row               |         | - receive buffer            |
   +--------------+--------------+         | - forwarding choices        |
                  |                        +--------------+--------------+
                  v                                       v
   +-----------------------------+         +-----------------------------+
   | SerialPortManager           |         | LiDARNetworkManager         |
   | current mux + reload        |         | current listener + reload   |
   +-----------------------------+         +-----------------------------+
```

## Catalog shape

### Top-level catalog entry

Use a common top-level entry with a tagged ingest specification rather than a single ever-growing struct.

| Field                 | Purpose                                                                                                     |
| --------------------- | ----------------------------------------------------------------------------------------------------------- |
| `slug`                | Stable model key such as `ops243-a`, `hesai-pandar40p`, `velodyne-vlp16`, `ouster-os1-64`                   |
| `family`              | High-level family such as `radar` or `lidar`                                                                |
| `vendor`              | Vendor name such as `omnipresense`, `hesai`, `velodyne`, `ouster`                                           |
| `display_name`        | Operator-facing model name                                                                                  |
| `description`         | Short narrative description                                                                                 |
| `capabilities`        | Curated capability tokens such as `speed`, `distance`, `point_cloud`, `dual_return`, `ptp_time`, `gps_time` |
| `runtime_config_kind` | Which runtime surface this model uses, for example `radar_serial` or `lidar_network`                        |
| `ingest`              | Tagged transport/defaults block                                                                             |

### Ingest variant: serial

Serial models should keep serial facts together instead of leaking them into unrelated models.

| Field                  | Purpose                                |
| ---------------------- | -------------------------------------- |
| `kind`                 | `serial`                               |
| `default_baud_rate`    | Model default used to seed new configs |
| `supported_baud_rates` | Allowed rates for UI validation        |
| `data_bits`            | Default data bits                      |
| `stop_bits`            | Default stop bits                      |
| `parity`               | Default parity                         |
| `init_commands`        | Model initialisation sequence          |

### Ingest variant: LiDAR UDP network

LiDAR models need parser and transport defaults, not host bindings.

| Field                      | Purpose                                              |
| -------------------------- | ---------------------------------------------------- |
| `kind`                     | `udp_lidar`                                          |
| `parser_key`               | Decoder identity used by the L1 parser factory       |
| `packet_format`            | Human-readable packet family or format version       |
| `default_data_port`        | Model default point-cloud port                       |
| `default_telemetry_port`   | Optional vendor telemetry or status port             |
| `default_receive_buffer`   | Recommended starting receive buffer                  |
| `receive_buffer_range`     | Safe lower/upper bounds                              |
| `supports_multicast`       | Whether multicast is a valid operating mode          |
| `supports_forwarding`      | Whether forwarding options make sense for this model |
| `expected_return_modes`    | Supported return mode identifiers                    |
| `expected_timestamp_modes` | Supported timestamp mode identifiers                 |

This shape works for Hesai, Velodyne, and Ouster because it describes the model's network expectations without pretending to know which NIC or IP address the current deployment should use.

## What stays out of the catalog

These belong in runtime configuration tables, not in the model catalog:

| Do not store in catalog            | Why                                                         |
| ---------------------------------- | ----------------------------------------------------------- |
| `interface_name`                   | Host-specific deployment choice                             |
| `bind_address`                     | Host-specific deployment choice                             |
| `enabled`                          | Live installation state                                     |
| config row `name`                  | Operator label, not model identity                          |
| `forward_enabled`                  | Live operator choice, even if the model supports forwarding |
| observed packet stats / source IPs | Runtime telemetry, not model metadata                       |
| hot-reload status                  | Manager state, not model metadata                           |

The catalog may define **defaults** and **constraints** for new configs. It must not become the live source of truth for an installation.

## Recommended runtime split

| Concern                                                                              | Canonical home                                       |
| ------------------------------------------------------------------------------------ | ---------------------------------------------------- |
| Sensor model identity and defaults                                                   | Shared sensor catalog                                |
| Radar serial port choices                                                            | `radar_serial_config`                                |
| LiDAR listener binding and forwarding choices                                        | `lidar_network_config`                               |
| LiDAR parser/profile overrides that are site-specific but not purely network-binding | LiDAR profile/config table layered above the catalog |

The practical rule for contributors is simple:

> A catalog entry describes the sensor model. A runtime config row describes how this host talks to that sensor.

## Migration path from the current radar-only shape

1. Rename the current concept from “sensor models for radar serial UI” to “shared sensor catalog”.
2. Replace radar-specific top-level booleans and baud fields with a common entry plus tagged ingest blocks.
3. Keep serving radar models from the existing API surface while adding LiDAR entries behind the same catalog contract.
4. Teach radar and LiDAR config UIs to resolve model defaults from the catalog, then persist operator choices into their own runtime tables.
5. Add vendor-specific LiDAR parser keys and defaults incrementally for Hesai, Velodyne, and Ouster.

## Failure registry

| Failure                                                    | Where it appears      | Effect                                                   | Recovery                                                                  |
| ---------------------------------------------------------- | --------------------- | -------------------------------------------------------- | ------------------------------------------------------------------------- |
| Catalog tries to carry host-specific fields                | Catalog schema/review | Embedded defaults start masquerading as live state       | Reject in review; move field to runtime config                            |
| One flat struct accumulates serial and network fields      | JSON/Go model layer   | Sparse unreadable records and invalid combinations       | Use tagged ingest variants                                                |
| LiDAR network config duplicates model defaults             | DB/API layer          | Drift between catalog and runtime surfaces               | Store model defaults once; runtime rows only override per installation    |
| Vendor-specific parser details leak into generic UI labels | API/UI layer          | Confusing operator experience                            | Keep parser identity in ingest spec, expose human display name separately |
| Family names become shorthand for transport                | Docs/code review      | Future non-serial radar or non-UDP LiDAR becomes awkward | Use `runtime_config_kind` / `ingest.kind` explicitly                      |

## Editorial guidance

Future docs should explain this with one clean distinction up front: the catalog is about the hardware model; runtime config is about the local installation. When examples are needed, show one serial radar and one networked LiDAR side by side rather than describing the design in the abstract until the cupboard starts rattling.
