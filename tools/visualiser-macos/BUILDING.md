# Building VelocityVisualiser

## Quick Start

```bash
# From repository root
make build-mac

# Or with xcodebuild directly
cd tools/visualiser-macos
xcodebuild -project VelocityVisualiser.xcodeproj -scheme VelocityVisualiser -configuration Release build
```

The built app is located at:

```
tools/visualiser-macos/build/Build/Products/Release/VelocityVisualiser.app
```

## Requirements

- macOS 15.0+ (Sequoia) – matches the app deployment target (`@available(macOS 15.0, *)`)
- Xcode 16.0+ – required for the macOS 15 SDK and Swift 5.9+ used by grpc-swift 2.x (async/await)
- Apple Silicon or Intel Mac with Metal support

## Swift Package Dependencies

The Xcode project includes these package dependencies which are resolved automatically:

| Package                  | Version | Repository                                           |
| ------------------------ | ------- | ---------------------------------------------------- |
| grpc-swift               | 2.2.1+  | https://github.com/grpc/grpc-swift.git               |
| grpc-swift-nio-transport | 2.4.1+  | https://github.com/grpc/grpc-swift-nio-transport.git |
| grpc-swift-protobuf      | 2.1.2+  | https://github.com/grpc/grpc-swift-protobuf.git      |

### First-Time Setup

When opening the project for the first time, Xcode will fetch and build the Swift packages. This may take several minutes.

If packages don't resolve automatically:

1. File → Packages → Resolve Package Versions
2. File → Packages → Reset Package Caches
3. Clean build folder (⇧⌘K)
4. Build (⌘B)

## Testing End-to-End

### 1. Start the Go gRPC Server

```bash
# Terminal 1
go run ./cmd/tools/visualiser-server -addr localhost:50051 -rate 10 -points 10000 -tracks 10
```

### 2. Launch the macOS App

```bash
# Terminal 2 - or use make dev-mac for Xcode
open tools/visualiser-macos/build/Build/Products/Release/VelocityVisualiser.app
```

### 3. Connect

In the app:

- The server address defaults to `localhost:50051`
- Click "Connect" or press ⌘⇧C
- You should see point clouds and tracks streaming at the configured rate

## Troubleshooting

See [Troubleshooting Guide](../../docs/lidar/visualiser/05-troubleshooting.md) for common issues and solutions.

## Regenerating Protobuf Stubs

When the protobuf schema changes:

```bash
# From repository root
make proto-gen
```

This generates both Go and Swift files. The Swift files are placed in:
`tools/visualiser-macos/VelocityVisualiser/gRPC/Generated/`

## Creating a Release DMG

To package VelocityVisualiser.app into a versioned DMG for distribution:

```bash
# From repository root — builds the app then creates the DMG
make build-mac
make dmg-mac            # dev: VelocityVisualiser-<VERSION>+<SHA>.dmg
make dmg-mac-release    # release: VelocityVisualiser-<VERSION>.dmg
```

By default `dmg-mac` appends the short git SHA to the filename (e.g.
`VelocityVisualiser-0.5.0-pre14+a1b2c3d.dmg`) so that development builds
are easily distinguishable. Use `dmg-mac-release` (or pass `DMG_SUFFIX=`)
to produce a clean release filename without the SHA suffix.

The output DMG is written to `tools/visualiser-macos/build/`:

- `make dmg-mac` → `VelocityVisualiser-<VERSION>+<SHA>.dmg`
- `make dmg-mac-release` → `VelocityVisualiser-<VERSION>.dmg`

The DMG opens in a small Finder window with VelocityVisualiser.app on the
left, a `Getting Started.txt` guide in the centre, and an Applications
shortcut on the right for drag-and-drop installation. The layout is
configured by `scripts/create-dmg.sh`. The version is read from the
`VERSION` variable in the Makefile.

The Getting Started guide (`tools/visualiser-macos/Getting Started.txt`)
covers server setup, connecting the app, keyboard shortcuts, and basic
troubleshooting. Edit it in the repository and it will be included in the
next DMG build.

> **CI:** Tagged releases (`v*`) and manual workflow dispatches automatically
> produce the DMG as a downloadable artefact in the `🍎 macOS CI` workflow.
