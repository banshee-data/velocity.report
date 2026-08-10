# Building VelocityVisualiser

## Quick start

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

## Swift package dependencies

The Xcode project includes these package dependencies which are resolved automatically:

| Package                  | Version | Repository                                           |
| ------------------------ | ------- | ---------------------------------------------------- |
| grpc-swift               | 2.2.1+  | https://github.com/grpc/grpc-swift.git               |
| grpc-swift-nio-transport | 2.4.1+  | https://github.com/grpc/grpc-swift-nio-transport.git |
| grpc-swift-protobuf      | 2.1.2+  | https://github.com/grpc/grpc-swift-protobuf.git      |

### First-Time setup

When opening the project for the first time, Xcode will fetch and build the Swift packages. This may take several minutes.

If packages don't resolve automatically:

1. File → Packages → Resolve Package Versions
2. File → Packages → Reset Package Caches
3. Clean build folder (⇧⌘K)
4. Build (⌘B)

## Testing end-to-end

### 1. Start the Go gRPC server

```bash
# Terminal 1
go run ./cmd/tools/visualiser-server -addr localhost:50051 -rate 10 -points 10000 -tracks 10
```

### 2. Launch the macOS app

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

See [Troubleshooting Guide](../../DEBUGGING.md#macos-visualiser-issues) for common issues
and solutions.

## Regenerating Protobuf stubs

When the protobuf schema changes:

```bash
# From repository root
make proto-gen
```

This generates both Go and Swift files. The Swift files are placed in:
`tools/visualiser-macos/VelocityVisualiser/gRPC/Generated/`

## Creating a release DMG

To package VelocityVisualiser.app into a versioned DMG for distribution:

```bash
# From repository root
make build-mac
make dmg-mac            # dev DMG: <DATETIME>-VelocityVisualiser-<DEV_VERSION>-<SHA>.dmg
make dmg-mac-release    # release DMG: VelocityVisualiser-<VERSION>.dmg
```

By default `dmg-mac` prepends a UTC timestamp and appends the short git SHA
to the filename (e.g. `20260407T142345Z-VelocityVisualiser-0.5.1.pre1-a1b2c3d.dmg`)
so that development builds are sortable and traceable. Use `dmg-mac-release`
to produce a clean release filename without the date or SHA.

Both targets build the app only if `VelocityVisualiser.app` is missing; this
lets the signed release path package the already-signed app without rebuilding
and invalidating the Developer ID signature.

The output DMG is written to `tools/visualiser-macos/build/`:

- `make dmg-mac` → `<DATETIME>-VelocityVisualiser-<DEV_VERSION>-<SHA>.dmg`
- `make dmg-mac-release` → `VelocityVisualiser-<VERSION>.dmg`

The DMG opens in a small Finder window with VelocityVisualiser.app on the left,
a `Getting Started.txt` guide in the centre, and an Applications shortcut on
the right for drag-and-drop installation. The layout is configured by
`scripts/create-dmg.sh`; Finder automation is best-effort and timeout-bounded
so local privacy prompts or headless runners cannot hang packaging. Override
the timeout with `DMG_LAYOUT_TIMEOUT_SECONDS=<seconds>`, or set `DMG_LAYOUT=0`
to skip Finder layout entirely. The version is read from the `VERSION` variable
in the Makefile.

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
make notarise-mac       # Submit release DMG, wait, staple
make verify-mac         # codesign + spctl + stapler checks
```

When running the steps manually, sign after the final build and package that
same app bundle. Do not run a build command between `make sign-mac` and
`make dmg-mac-release`; rebuilding replaces the signed app with a local
development signature. `make release-mac` handles the order correctly.

### One-Time Local Setup

1. **Install a Developer ID Application certificate** from the
   [Apple Developer portal](https://developer.apple.com/account/resources/certificates/list)
   into your login or System keychain.

   Generate the certificate request on the Mac that will sign the release, or
   import a `.p12` that contains both the certificate and its private key. A
   downloaded `.cer` alone is not enough unless Keychain can pair it with the
   private key from the original certificate-signing request.

   Verify that Keychain has a usable signing identity:

   ```bash
   security find-identity -v -p codesigning
   ```

   The output must include a `Developer ID Application: ...` identity. If more
   than one matching identity appears, pass the SHA-1 hash explicitly:

   ```bash
   make release-mac CODESIGN_IDENTITY=<SHA1_FROM_FIND_IDENTITY>
   ```

   On the first signing run, macOS may prompt for private-key access. Approve
   `codesign`; choosing **Always Allow** avoids repeated prompts during nested
   framework signing.

2. **Store notarisation credentials** (choose one):

   **Option A — Keychain profile** (recommended for local development):

   ```bash
   xcrun notarytool store-credentials "velocity-report" \
     --apple-id "<APPLE_ID>" --team-id "<TEAM_ID>" \
     --password "<APP-SPECIFIC-PASSWORD>"
   ```

   Check the profile without submitting a new build:

   ```bash
   xcrun notarytool history \
     --keychain-profile velocity-report \
     --keychain ~/Library/Keychains/login.keychain-db
   ```

   **Option B — App Store Connect API key** (recommended for CI):

   ```bash
   export NOTARY_KEY=/path/to/AuthKey_XXXX.p8
   export NOTARY_KEY_ID=XXXXXXXXXX
   export NOTARY_ISSUER=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
   ```

Keep all signing and notarisation credentials in Keychain, local environment
variables, or GitHub Actions secrets. Do not commit certificates, private keys,
notary profile exports, `.p8` API keys, or app-specific passwords to the repo.

### Verified Local Release Path

The first successful local notarised DMG run was completed on August 10, 2026
with this sequence:

```bash
security find-identity -v -p codesigning
xcrun notarytool history --keychain-profile velocity-report \
  --keychain ~/Library/Keychains/login.keychain-db
make build-mac
make sign-mac CODESIGN_IDENTITY=<Developer ID Application SHA1>
make dmg-mac-release
make notarise-mac
make verify-mac
```

Expected final verification:

```text
source=Notarized Developer ID
✓ Staple valid
✓ All verification checks passed
```

### Configuration

| Variable                     | Default                    | Description                                    |
| ---------------------------- | -------------------------- | ---------------------------------------------- |
| `CODESIGN_IDENTITY`          | `Developer ID Application` | Codesign identity name or SHA-1 hash           |
| `NOTARY_PROFILE`             | `velocity-report`          | Keychain profile for `notarytool`              |
| `NOTARY_KEYCHAIN`            | login keychain             | Keychain path used with `NOTARY_PROFILE`       |
| `NOTARY_KEY`                 | _(unset)_                  | Path to App Store Connect API key (.p8)        |
| `NOTARY_KEY_ID`              | _(unset)_                  | API key ID (used with `NOTARY_KEY`)            |
| `NOTARY_ISSUER`              | _(unset)_                  | API issuer UUID (used with `NOTARY_KEY`)       |
| `VISUALISER_NOTARY_DMG`      | release DMG path           | DMG path for notarise/verify targets           |
| `DMG_LAYOUT`                 | `1`                        | Set to `0` to skip Finder layout               |
| `DMG_LAYOUT_TIMEOUT_SECONDS` | `30`                       | Finder layout timeout before packaging goes on |

### CI Secrets

When configured, the `🍎 macOS CI` workflow signs and notarises
automatically on tagged releases. Required GitHub Actions secrets:

| Secret                       | Source                                                                | GitHub destination            |
| ---------------------------- | --------------------------------------------------------------------- | ----------------------------- |
| `MACOS_CERTIFICATE`          | Base64 export of a Developer ID Application `.p12` certificate bundle | Repository **Actions** secret |
| `MACOS_CERTIFICATE_PASSWORD` | Password chosen when exporting the `.p12` from Keychain Access        | Repository **Actions** secret |
| `NOTARY_KEY`                 | Contents of the App Store Connect API key file, `AuthKey_<KEY_ID>.p8` | Repository **Actions** secret |
| `NOTARY_KEY_ID`              | App Store Connect API key ID shown beside the downloaded `.p8` key    | Repository **Actions** secret |
| `NOTARY_ISSUER`              | App Store Connect issuer ID for the API key                           | Repository **Actions** secret |

The CI release job has two valid modes:

- With all five secrets configured, it signs the app with Developer ID,
  notarises the DMG, staples the ticket, verifies signatures, and uploads
  `VelocityVisualiser-dmg`.
- With none of the five secrets configured, it still produces a packaging
  smoke-test DMG, but renames the file and artifact with `UNSIGNED`.

Partial configuration is treated as an error. Populate these only in GitHub
repository **Settings → Secrets and variables → Actions**. Do not put their
values in workflow YAML, local docs, issue comments, or chat.

### Common Failure Modes

| Symptom                                                    | Cause                                      | Fix                                                                          |
| ---------------------------------------------------------- | ------------------------------------------ | ---------------------------------------------------------------------------- |
| `0 valid identities found`                                 | Certificate lacks matching private key     | Recreate from a CSR on this Mac or import `.p12`                             |
| `identity not found`                                       | Certificate not in searched keychain       | Install Developer ID Application identity or pass `CODESIGN_IDENTITY=<SHA1>` |
| `errSecInternalComponent`                                  | Keychain locked or restricted              | `security unlock-keychain login.keychain`                                    |
| `codesign` appears to hang                                 | Private-key access prompt is waiting       | Approve `codesign` in the macOS Keychain prompt                              |
| `Hardened Runtime not enabled`                             | Missing `--options runtime`                | Already set by `codesign-notarise.sh`                                        |
| `Unsigned nested code`                                     | Framework/dylib not signed                 | Script signs nested code inside-out                                          |
| `Invalid signature (code or signature have been modified)` | App modified after signing                 | Re-run `make sign-mac` before packaging                                      |
| `Notarisation credentials not found`                       | Missing profile/key                        | Run `store-credentials` or set env vars                                      |
| `Package Invalid` / rejected by Apple                      | Hardened Runtime issue or unsigned binary  | Check `xcrun notarytool log` for details                                     |
| DMG packaging stalls during Finder layout                  | Finder automation prompt or unavailable UI | Wait for timeout, set `DMG_LAYOUT=0`, or lower `DMG_LAYOUT_TIMEOUT_SECONDS`  |
