# Velocity Visualiser for macOS

A native macOS application for visualising LiDAR point clouds, tracks, and debug overlays from the velocity.report tracking pipeline.

## Overview

This is a SwiftUI application that connects to the Go LiDAR pipeline via gRPC and renders:

- Point clouds (live and replay)
- Clusters with bounding boxes
- Tracks with IDs, velocities, and trails
- Debug overlays for algorithm tuning
- Labelling workflow for classifier training

## Requirements

- macOS 14.0+ (Sonoma)
- Apple Silicon (M1/M2/M3) or Intel with Metal support
- Xcode 15.0+

## Building

**Quick start (M0 - synthetic data):**

```bash
# Disable generated gRPC files (one-time)
./toggle-grpc-files.sh

# Build
make build-mac  # from repository root
```

**For full gRPC support, see [BUILDING.md](BUILDING.md)** for detailed instructions on adding Swift Package dependencies.

### Prerequisites

- macOS 15.0+ (Sonoma)
- Xcode 15.0+
- Command-line tools: `brew install swift-protobuf grpc-swift`

### First-Time Setup

If the Xcode project doesn't exist yet, follow the setup in [BUILDING.md](BUILDING.md#option-2-command-line-workaround).

## Usage

```bash
# From repository root
make build-mac

# Or with xcodebuild directly
cd tools/visualiser-macos
xcodebuild -project VelocityVisualiser.xcodeproj -scheme VelocityVisualiser -configuration Release
```

## Usage

### Connecting to Live Pipeline

1. Start the Go pipeline with gRPC enabled:

   ```bash
   velocity-report --grpc-enabled --grpc-addr localhost:50051
   ```

2. Launch VelocityVisualiser
3. Click "Connect" (or press ⌘⇧C)
4. The visualiser will start rendering live data

### Replaying Recorded Logs

1. File → Open Recording (⌘O)
2. Select a `.vrlog` directory
3. Use playback controls to navigate

### Labelling Tracks

1. Click on a track in the 3D view to select it
2. Use the Label panel (press L) to assign a class
3. Export labels via File → Export Labels (⌘E)

## Keyboard Shortcuts

| Action             | Shortcut |
| ------------------ | -------- |
| Connect/Disconnect | ⌘⇧C      |
| Pause/Play         | Space    |
| Step Forward       | .        |
| Step Backward      | ,        |
| Increase Rate      | ]        |
| Decrease Rate      | [        |
| Toggle Points      | P        |
| Toggle Boxes       | B        |
| Toggle Trails      | T        |
| Toggle Velocity    | V        |
| Toggle Debug       | D        |
| Reset Camera       | R        |
| Label Track        | L        |
| Export Labels      | ⌘E       |

## Architecture

```
VelocityVisualiser/
├── App/                 # Application entry and state
├── gRPC/                # gRPC client and protobuf decoding
├── Rendering/           # Metal renderer and shaders
├── UI/                  # SwiftUI views
├── Labelling/           # Label storage and export
└── Models/              # Swift data models
```

### Rendering Pipeline

1. gRPC client receives `FrameBundle` stream
2. Frames are decoded and pushed to render queue
3. Metal renderer draws:
   - Point cloud as point sprites
   - Boxes as instanced geometry
   - Trails as triangle strips
   - Overlays as 2D layer
4. SwiftUI displays Metal view + controls

## Configuration

The app stores preferences in `~/Library/Preferences/com.velocity.visualiser.plist`:

- `serverAddress`: Default server address (localhost:50051)
- `pointSize`: Point rendering size
- `trailDuration`: How long trails persist (seconds)
- `defaultOverlays`: Which overlays are enabled by default

## Development

### Running Tests

```bash
xcodebuild test -project VelocityVisualiser.xcodeproj -scheme VelocityVisualiser
```

### Regenerating Protobuf Stubs

When the protobuf schema changes:

```bash
# From repository root
protoc --swift_out=tools/visualiser-macos/VelocityVisualiser/gRPC/Generated \
       --grpc-swift_out=tools/visualiser-macos/VelocityVisualiser/gRPC/Generated \
       proto/velocity_visualiser/v1/visualiser.proto
```

## Related Documentation

- [Design Docs](../../docs/lidar/visualiser_macos/)
- [API Contract](../../docs/lidar/visualiser_macos/02-api-contracts.md)
- [Architecture](../../docs/lidar/visualiser_macos/03-architecture.md)
