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

### Prerequisites

1. Install Swift protobuf and gRPC plugins:

   ```bash
   brew install swift-protobuf grpc-swift
   ```

2. Generate Swift protobuf stubs:

   ```bash
   # From repository root
   make proto-gen-swift
   ```

3. **Create the Xcode project** (first-time setup):

   The source directory exists but the Xcode project needs to be created:

   a. Open Xcode
   b. File → New → Project
   c. Choose **macOS** → **App**
   d. Set:
   - Product Name: `VelocityVisualiser`
   - Interface: **SwiftUI**
   - Language: **Swift**
     e. Save location: `tools/visualiser-macos/` (choose this directory)
     f. In Xcode, delete the auto-generated source files
     g. Right-click project → Add Files to "VelocityVisualiser"
     h. Add the existing `VelocityVisualiser/` directory (with folders)

### Build with Xcode

1. Open `VelocityVisualiser.xcodeproj` in Xcode
2. Select the "VelocityVisualiser" scheme
3. Build and run (⌘R)

### Build from Command Line

```bash
cd tools/visualiser-macos
xcodebuild -project VelocityVisualiser.xcodeproj -scheme VelocityVisualiser -configuration Release
```

**Note:** The command-line build requires the Xcode project to be created first (see Prerequisites above).

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
       proto/velocity_visualizer/v1/visualizer.proto
```

## Related Documentation

- [Design Docs](../../docs/lidar/visualiser_macos/)
- [API Contract](../../docs/lidar/visualiser_macos/02-api-contracts.md)
- [Architecture](../../docs/lidar/visualiser_macos/03-architecture.md)
