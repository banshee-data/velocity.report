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

- macOS 14.0+ (Sonoma)
- Xcode 15.0+
- Apple Silicon or Intel Mac with Metal support

## Swift Package Dependencies

The Xcode project includes these package dependencies which are resolved automatically:

| Package | Version | Repository |
|---------|---------|------------|
| grpc-swift | 2.2.1+ | https://github.com/grpc/grpc-swift.git |
| grpc-swift-nio-transport | 2.4.1+ | https://github.com/grpc/grpc-swift-nio-transport.git |
| grpc-swift-protobuf | 2.1.2+ | https://github.com/grpc/grpc-swift-protobuf.git |

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
