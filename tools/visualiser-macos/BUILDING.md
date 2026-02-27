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

## Signing, Notarisation, and Distribution

To distribute the DMG outside the App Store, the `.app` must be codesigned
with a **Developer ID Application** certificate and the DMG must be
**notarised** by Apple. The `release-mac` target automates the full pipeline:

```bash
# Full pipeline: build → sign → DMG → notarise → verify
make release-mac
```

Individual steps can also be run separately:

```bash
make build-mac          # Build the .app
make sign-mac           # Codesign with Developer ID (Hardened Runtime)
make dmg-mac-release    # Package into DMG
make notarise-mac DMG_SUFFIX=   # Submit, wait, staple
make verify-mac   DMG_SUFFIX=   # codesign + spctl + stapler checks
```

### One-Time Local Setup

1. **Install a Developer ID Application certificate** from the
   [Apple Developer portal](https://developer.apple.com/account/resources/certificates/list)
   into your login keychain.

2. **Store notarisation credentials** (choose one):

   **Option A — Keychain profile** (recommended for local development):

   ```bash
   xcrun notarytool store-credentials "velocity-report" \
     --apple-id "<APPLE_ID>" --team-id "<TEAM_ID>" \
     --password "<APP-SPECIFIC-PASSWORD>"
   ```

   **Option B — App Store Connect API key** (recommended for CI):

   ```bash
   export NOTARY_KEY=/path/to/AuthKey_XXXX.p8
   export NOTARY_KEY_ID=XXXXXXXXXX
   export NOTARY_ISSUER=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
   ```

### Configuration

| Variable            | Default                    | Description                              |
| ------------------- | -------------------------- | ---------------------------------------- |
| `CODESIGN_IDENTITY` | `Developer ID Application` | Codesign identity name                   |
| `NOTARY_PROFILE`    | `velocity-report`          | Keychain profile for `notarytool`        |
| `NOTARY_KEY`        | _(unset)_                  | Path to App Store Connect API key (.p8)  |
| `NOTARY_KEY_ID`     | _(unset)_                  | API key ID (used with `NOTARY_KEY`)      |
| `NOTARY_ISSUER`     | _(unset)_                  | API issuer UUID (used with `NOTARY_KEY`) |

### CI Secrets

When configured, the `🍎 macOS CI` workflow signs and notarises
automatically on tagged releases. Required GitHub Actions secrets:

| Secret                       | Description                                     |
| ---------------------------- | ----------------------------------------------- |
| `MACOS_CERTIFICATE`          | Base64-encoded Developer ID `.p12` certificate  |
| `MACOS_CERTIFICATE_PASSWORD` | Password for the `.p12` file                    |
| `NOTARY_KEY`                 | Contents of the App Store Connect API key (.p8) |
| `NOTARY_KEY_ID`              | API key ID                                      |
| `NOTARY_ISSUER`              | API issuer UUID                                 |

If the secrets are not set, CI still produces an unsigned DMG artefact.

### Common Failure Modes

| Symptom                                                    | Cause                                     | Fix                                         |
| ---------------------------------------------------------- | ----------------------------------------- | ------------------------------------------- |
| `identity not found`                                       | Certificate not in keychain               | Install Developer ID cert from Apple portal |
| `errSecInternalComponent`                                  | Keychain locked or restricted             | `security unlock-keychain login.keychain`   |
| `Hardened Runtime not enabled`                             | Missing `--options runtime`               | Already set by `codesign-notarise.sh`       |
| `Unsigned nested code`                                     | Framework/dylib not signed                | Script signs nested code inside-out         |
| `Invalid signature (code or signature have been modified)` | App modified after signing                | Re-run `make sign-mac` before packaging     |
| `Notarisation credentials not found`                       | Missing profile/key                       | Run `store-credentials` or set env vars     |
| `Package Invalid` / rejected by Apple                      | Hardened Runtime issue or unsigned binary | Check `xcrun notarytool log` for details    |
