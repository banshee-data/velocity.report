# Troubleshooting

Status: Active
Purpose/Summary: 05-troubleshooting.

This guide covers common issues when building and running the macOS Visualiser.

---

## Build Errors

### "Unable to find module dependency: 'GRPCCore'"

**Cause:** Swift Package dependencies not resolved.

**Solution:**

1. Open `VelocityVisualiser.xcodeproj` in Xcode
2. Wait for package resolution (may take several minutes on first run)
3. If packages don't resolve automatically:
   - File → Packages → Resolve Package Versions
   - File → Packages → Reset Package Caches
4. Clean build folder (⇧⌘K) and rebuild (⌘B)

---

### "No such module 'SwiftProtobuf'"

**Cause:** Swift packages not properly cached.

**Solution:**

1. In Xcode: File → Packages → Reset Package Caches
2. File → Packages → Resolve Package Versions
3. Clean build folder (⇧⌘K)
4. Build (⌘B)

---

### Build succeeds but app crashes on launch

**Cause:** Metal device not available or shader compilation failure.

**Solution:**

1. Ensure running on Apple Silicon or Intel Mac with Metal support
2. Check Console.app for `MetalRenderer` error messages
3. Try running from Xcode to see detailed crash logs

---

## Connection Errors

### "Server unreachable" or connection timeout

**Cause:** Go gRPC server not running or wrong address.

**Solution:**

1. Start the server:
   ```bash
   go run ./cmd/tools/visualiser-server -addr localhost:50051
   ```
2. Verify the address in the app matches the server's `-addr` flag
3. Check for firewall blocking localhost connections

---

### Connection succeeds but no frames received

**Cause:** Stream not started or request parameters not matching.

**Solution:**

1. Check server logs for "StreamFrames started" message
2. Verify the app is requesting the correct sensor ID
3. Try restarting both server and client

---

## Rendering Issues

### Points not visible

**Cause:** Points toggle disabled or point buffer empty.

**Solution:**

1. Ensure "P" toggle is enabled in the toolbar
2. Check that server is sending `IncludePoints: true` in the stream
3. Verify point count in stats display is non-zero

---

### Trails appear corrupted or connected incorrectly

**Cause:** Trail segments being drawn as a single continuous line instead of separate trails.

**Solution:** This was a bug fixed in the renderer. Ensure you're running the latest build:

```bash
make build-mac
```

---

### Boxes not visible

**Cause:** Boxes toggle disabled or no tracks with bounding boxes.

**Solution:**

1. Ensure "B" toggle is enabled in the toolbar
2. Check that server is sending tracks with `BBoxLength/Width/Height` values

---

## Performance Issues

### Low frame rate (< 30 FPS)

**Cause:** Too many points or inefficient rendering.

**Solution:**

1. Reduce point count on server: `--points 5000`
2. Disable unused overlays (trails, velocity vectors)
3. Check Activity Monitor for GPU/CPU bottlenecks

---

### SwiftUI "AttributeGraph cycle detected" warnings

**Cause:** Complex objects changing rapidly being passed through SwiftUI view properties.

**Solution:** This is a known issue with high-frequency frame updates. The warnings are informational and don't affect functionality. The frame delivery architecture bypasses SwiftUI to avoid this.

---

## Development Setup

### Regenerating Protobuf Stubs

When the protobuf schema changes:

```bash
# From repository root
make proto-gen
```

This generates both Go and Swift files. The Swift files are placed in:
`tools/visualiser-macos/VelocityVisualiser/gRPC/Generated/`

---

### Package Dependencies

Required Swift Package dependencies:

| Package        | Version | Products                                      |
| -------------- | ------- | --------------------------------------------- |
| grpc-swift     | 2.0.0+  | GRPCCore, GRPCNIOTransportHTTP2, GRPCProtobuf |
| swift-protobuf | 1.28.0+ | SwiftProtobuf                                 |

These should be automatically resolved by Xcode when opening the project.

---

## Getting Help

If you encounter issues not covered here:

1. Check the server logs for error messages
2. Run the app from Xcode to see console output
3. Look for related issues in the repository
4. Include relevant logs when reporting problems
