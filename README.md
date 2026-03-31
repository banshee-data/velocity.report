# velocity.report

**Measure velocity, not identity**

<div align="center">

[![CI](https://github.com/banshee-data/velocity.report/actions/workflows/go-ci.yml/badge.svg?branch=main)](https://github.com/banshee-data/velocity.report/actions/workflows/go-ci.yml)
[![Coverage](https://img.shields.io/codecov/c/github/banshee-data/velocity.report?label=coverage)](https://codecov.io/gh/banshee-data/velocity.report)
[![License](https://img.shields.io/github/license/banshee-data/velocity.report)](LICENSE)
[![Last commit](https://img.shields.io/github/last-commit/banshee-data/velocity.report)](https://github.com/banshee-data/velocity.report/commits/main)
[![Privacy](https://img.shields.io/badge/privacy-no%20PII%20by%20design-brightgreen)](.github/TENETS.md)
[![Discord](https://img.shields.io/discord/1387513267496419359?logo=discord&label=discord)](https://discord.gg/XXh6jXVFkt)
[![Release](https://img.shields.io/github/v/release/banshee-data/velocity.report?label=release)](https://github.com/banshee-data/velocity.report/releases/latest)
[![Sample report](https://img.shields.io/badge/sample-PDF%20report-blue)](https://banshee-data.com/velocity.reports/2026-01-19_velocity.report_Clarendon-Avenue-San-Francisco.pdf)

</div>

Street-level speed measurement for neighbourhood change-makers, researchers, and anyone learning what LiDAR can tell you about how traffic actually behaves. Privacy-preserving radar and LiDAR sensors collect evidence so communities can make the case for safer streets; without cameras, licence plates, or any personally identifiable information.

- 📊 Professional PDF reports ready for council meetings
- 🔒 No video, no plates, no PII — by design, not by promise
- 📡 Radar speed measurement and LiDAR object tracking (working toward [sensor fusion](docs/plans/lidar-l7-scene-plan.md))
- 🏠 Runs on a Raspberry Pi in your neighbourhood. Offline-first
- 🔒 Open source and auditable, because trust should be verifiable

```
                    ░░░         ░░░░░░                      ░░░         ░░░░░░
   ░░░░░    ░░░░░░░░▒▒▒    ░░░░░ :::. ░░   ░░░░░    ░░░░░░░░▒▒▒    ░░░░░ :::.
  ░▒▒▒▒▒░░░░▒▒▒▒▒▒▒▒▓▓▓░░░░▒▒▒▒▒ .:.  ▒▒  ░▒▒▒▒▒░░░░▒▒▒▒▒▒▒▒▓▓▓░░░░▒▒▒▒▒ .:.
░░▒▓▓▓▓▓▒▒▒▒▓▓▓▓▓▓▓▓███▒▒▒▒ .::.::: : ▓▓░░▒▓▓▓▓▓▒▒▒▒▓▓▓▓▓▓▓▓███▒▒▒▒ .::.:.: :
▒▒▓.  . ▓▓▓▓████ ... . ▓▓▓▓ :....:::::. ▒▒▓.  . ▓▓▓▓████ ... . ▓▓▓▓ :.::..::::
▓▓█ ..::████████ :.::: ████ :: :  : . ::▓▓█ ..::████████ :.::: ████ :: :: : ::
:.: : ::████:.:: : ::: ████ .: : .  ... :.: ::::████:.:: : ::: ████ .::: .:::.
.::  .  .   ::.:    .  .          .     .::  .  .   ::.: ::::  .          .
    .     .       .      ▄▀▀▀▀▀▀▄
                       ▄▄█▄▄▄  ▄▄▌
                         ▌▐▀▀▀█  ▌
                         ▐▀  ▄▄▀▀█
▀▀▀▀▀▀█████▀▀▀▀▀▀▀▀▀█▀▀█▀▄▓▀▀▀  ░▐▌▀▀▀▀▀▀█▀▀██▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
▄▄▄▀▀▀░░ ░ ░ ░▒▓▄▄▀▀▄▄█▄▓░   ░▄  ░█       ▀▀▄▄▀▀▄▄▓▒ ▒   ░ ░    ░  ░   ░  ░
 ░ ░░ ░ ░ ▒ ▄▄▀▀▄▄▀▀▄▀█░ ▄▄▀█▀░░ ░█           ▀▀▄▄▀▀▄▄  ▒ ░   ░   ░   ░ ░    ░
  ░  ░ ▒▄▄▀▀▄▄▀▀   █▀▀▓▀▀▌  ▐▌░ ░░▒▌              ▀▀▄▄▀▀▄▄   ▒ ░    ░      ░
░░ ▒▄▄▀▀▄▄▀▀       ▓  ▓  ▌  █▀▀▀▀▀▀█                  ▀▀▄▄▀▀▄▄   ░ ░   ░  ░  ░
▄▄▀▀▄▄▀▀           ▓▀▀▓▀▀▌ ▐▌░  ░  ▐▌                     ▀▀▄▄▀▀▄▄  ▒░ ▒
▄▄▀▀               ▓__▓__▌ █░  █▄░ ▐▌                         ▀▀▄▄▀▀▄▄   ░ ▒
                   ▓  ▓  ▌ █░ ▐▌ █░ █                             ▀▀▄▄▀▀▄▄   ░
▄▄▄▄   ▄▄▄▄▄▄   ▄▄▄▓▄ ▓  ▌▒▒▒▒▒  ▒▒▒▒                                 ▀▀▄▄▀▀▄▄
██▀  ▄█████▀  ▄█████▀  ▄███   ▄██ █▄  ▀████▄   ▀████▄   ▀████▄   ▀████▄   ██▄▄
▀  ▄█████▀  ▄█████▀  ▄████▀  ████ ███▌  ▀████▄   ▀████▄   ▀████▄   ▀████▄  ▀██
 ▄█████▀  ▄█████▀  ▄█████  ▄█████ █████   ▀████▄   ▀████▄   ▀████▄   ▀████▄  ▀
█████▀  ▄█████▀  ▄█████▀  ██████▌  ▀████    ▀████▄   ▀████▄   ▀████▄   ▀████▄
```

## Project Documents

- 🔭 [VISION.md](docs/VISION.md): where the project is heading
- 🎨 [DESIGN.md](docs/ui/DESIGN.md): frontend and visualisation design language
- ❓ [QUESTIONS.md](data/QUESTIONS.md): open research questions for the curious
- 🧭 [DECISIONS.md](docs/DECISIONS.md): why things are the way they are
- 🏗️ [ARCHITECTURE.md](ARCHITECTURE.md): system design, data flow, and component relationships
- 🧱 [COMMANDS.md](COMMANDS.md): every make target, catalogued
- 🌲 [MATRIX.md](data/structures/MATRIX.md): test and validation surface coverage
- 📋 [BACKLOG.md](docs/BACKLOG.md): work queue, prioritised and honest
- 🪵 [CHANGELOG.md](CHANGELOG.md): what changed and when
- 📓 [DEVLOG.md](docs/DEVLOG.md): engineering journal and working notes
- 🛠️ [TROUBLESHOOTING.md](TROUBLESHOOTING.md): when things go wrong, start here
- 🙋 [CONTRIBUTING.md](CONTRIBUTING.md): how to help
- 🤝 [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md): how we treat each other
- ⚖️ [LICENSE](LICENSE): Apache 2.0

## Why velocity.report?

Communities trying to make their streets safer face a familiar problem: everyone has an opinion about how fast cars go, and nobody has evidence. Council meetings run on anecdote. Speed surveys cost thousands and arrive months late. Meanwhile, someone's child is still crossing that road.

velocity.report exists to close the gap between _feeling unsafe_ and _proving it_.

The radar measures vehicle speeds. The LiDAR classifies and tracks objects. Both run independently today; [sensor fusion](docs/VISION.md) — combining them into a single corroborated record — is the next major milestone. No cameras, no licence plates, no surveillance infrastructure that a neighbourhood should never have to build in order to be heard. The data stays on a Raspberry Pi in someone's house. The reports are professional enough for a planning committee.

Evidence over opinion. Privacy over convenience. Community ownership over cloud dependency.

## What's Included

| Component            | Language            | What it does                                                                                                                                                                             |
| -------------------- | ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Go server**        | Go                  | Collects radar speed data and LiDAR point clouds independently, stores both in SQLite, serves the API. Runs as a systemd service on Raspberry Pi. → [cmd/](cmd/), [internal/](internal/) |
| **PDF generator**    | Python + LaTeX      | Turns speed data into professional reports with charts, statistics, and proper formatting. Ready for a council submission. → [tools/pdf-generator/](tools/pdf-generator/README.md)       |
| **Web frontend**     | Svelte + TypeScript | Data visualisation and interactive charts for recorded speed data. → [web/](web/README.md)                                                                                               |
| **macOS visualiser** | Swift + Metal       | Native 3D LiDAR point cloud viewer with object tracking, replay, and debug overlays. Apple Silicon. → [tools/visualiser-macos/](tools/visualiser-macos/README.md)                        |

## Quick Start

### Run the server and web frontend

```sh
git clone git@github.com:banshee-data/velocity.report.git
cd velocity.report
make build-web
make build-radar-local
```

If you have an existing SQLite database, place it at `./sensor_data.db`. For production, use `--db-path` to point elsewhere.

Now, start the server by running:

```sh
./velocity-report-local --disable-radar
```

Next, open [localhost:8080](http://localhost:8080) to see the dashboard.

### Launch the macOS LiDAR visualiser

Requires macOS 14+ and Metal support.

```sh
make dev-mac
```

Then open the Lidar Dashboard on [localhost:8081](http://localhost:8081) to start PCAP replay.

See [tools/visualiser-macos/README.md](tools/visualiser-macos/README.md) for replay mode, gRPC controls, and camera navigation.

## Privacy

The system records vehicle speed data. That is all it records.

- No cameras
- No licence plate recognition
- No video
- No personally identifiable information — by design, not by policy

The point is to measure traffic, not to start building a private surveillance habit. The data stays on a local device. Reports are generated locally. If PII reaches a log, a response body, or an export, the system has failed.

See [TENETS.md](.github/TENETS.md) for the full set of non-negotiable principles.

## Who It's For

- **Neighbourhood groups** measuring speed on their street, with evidence instead of guesswork
- **Community advocates** building a case for traffic calming, with data that survives a council meeting
- **Academics and researchers** studying street-level vehicle behaviour with LiDAR point clouds, tracking pipelines, and replayable datasets
- **Students and engineers** learning LiDAR perception — the pipeline is transparent, tuneable, and documented from raw UDP packets through to classified tracks
- **Before-and-after studies** showing whether traffic calming interventions actually work

## Architecture

```
   ┌──────────────────┐     ┌──────────────────────────┐     ┌──────────────────┐
   │     Sensors      │────►│  velocity.report Server  │◄───►│ SQLite Database  │
   │ (Radar / LiDAR)  │     │        (Go)              │     │ (sensor_data.db) │
   └──────────────────┘     └──────────────────────────┘     └──────────────────┘
                                  │              │
                       HTTP :8080 │              │ gRPC :50051
                   ┌──────────────┴─┐            │
                   │                │            │
                   ▼                ▼            ▼
        ┌──────────────┐ ┌───────────────┐ ┌─────────────────────┐
        │ Web Frontend │ │ PDF Generator │ │  VelocityVisualiser │
        │   (Svelte)   │ │  (Python/TeX) │ │ (macOS/Metal, gRPC) │
        └──────────────┘ └───────────────┘ └─────────────────────┘
```

For the full architecture — data flow, schema, and deployment model — see
For the full architecture — data flow, schema, and deployment model — see [ARCHITECTURE.md](ARCHITECTURE.md). Sensor fusion plans live in [VISION.md](docs/VISION.md).

## Development

Every commit should pass:

```sh
make format    # auto-fix all formatting
make lint      # check all formatting
make test      # run all test suites
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for prerequisites, dev environment setup, coding standards, and pull request workflow. All make targets are documented in [COMMANDS.md](COMMANDS.md).

## Deployment

The Go server runs as a systemd service on Raspberry Pi. See [public_html/src/guides/setup.md](public_html/src/guides/setup.md) for the complete setup guide and [cmd/deploy/README.md](cmd/deploy/README.md) for the deployment tool reference.

## Project Structure

```
velocity.report/
├── cmd/                  # Go CLI applications
│   ├── radar/            # Radar/LiDAR sensor service
│   ├── deploy/           # Deployment manager
│   ├── sweep/            # Parameter sweep utilities
│   └── tools/            # Utility tools (visualiser-server, gen-vrlog, pcap-analyse)
├── internal/             # Go internals (API, database, radar, LiDAR, monitoring)
├── web/                  # Svelte web frontend
├── tools/
│   ├── pdf-generator/    # Python PDF report generation
│   └── visualiser-macos/ # macOS LiDAR visualiser (Swift/Metal)
├── data/                 # Sample data, alignment, and analysis
├── docs/                 # Internal project documentation
├── public_html/          # Public documentation site (Eleventy)
├── config/               # Tuning parameters and configuration
├── proto/                # Protobuf definitions
└── scripts/              # Development shell scripts
```

## Contributing

Start with a small issue and read the nearby code before changing anything broad. It is the fastest route to understanding the project and the slowest route to producing an exciting new class of bug.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow, testing requirements, and pull request process.

## Licence

Apache License 2.0 — see [LICENSE](LICENSE).

## Community

[![join-us-on-discord](https://github.com/user-attachments/assets/fa329256-aee7-4751-b3c4-d35bdf9287f5)](https://discord.gg/XXh6jXVFkt)

Join the Discord to discuss the project, get help, and help make streets safer.
