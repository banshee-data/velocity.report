# macOS Local Server — Plan

- **Layers:** L10 Clients (macOS visualiser), Platform (deployment)
- **Canonical:** [velocity-visualiser-architecture.md](../ui/velocity-visualiser-architecture.md)
- **Feature:** Embed Go server binary in VelocityVisualiser.app with local server management and login-item boot hooks

---

## Problem

VelocityVisualiser currently requires a separate, manually started Go server
process to stream LiDAR data. On macOS, users must:

1. Download and place the Go binary somewhere on disk.
2. Launch it from the terminal with the right flags.
3. Know to restart it after reboot.
4. Remember which binary matches which app version.

This friction makes the macOS developer/analyst workflow unnecessarily
painful. The visualiser should be a self-contained application that manages
its own server lifecycle.

## Goal

A single VelocityVisualiser.app that:

- **Bundles** the Go server binary (ARM64) inside the app bundle.
- **Manages** the local server lifecycle: start, stop, restart from the
  menu bar, independent of the app's own quit/launch cycle.
- **Selects** between local bundled server and remote servers via the
  existing Server Manager UI (see [server-manager.md](server-manager.md)).
- **Optionally starts at login** via a macOS Login Item, with easy
  enable/disable from the app.

## Non-Goals

- Linux/Windows support — macOS only.
- Running the server as root or a system daemon — user-space only.
- Auto-updating the Go binary — the binary ships with the app version.
- Replacing the Raspberry Pi deployment model — this is for macOS
  analyst/developer use, not production sensor ingest.

---

## Architecture

### Binary Location

```
VelocityVisualiser.app/
└── Contents/
    ├── MacOS/
    │   └── VelocityVisualiser          (Swift app binary)
    ├── Resources/
    │   └── velocity-report-server      (Go binary, ARM64)
    ├── Library/
    │   └── LoginItems/
    │       └── VelocityServerHelper.app  (Phase 4, login item helper)
    └── Info.plist
```

The Go binary sits in `Contents/Resources/` — the standard location for
non-executable resources in a macOS app bundle. The app locates it at
runtime via `Bundle.main.url(forResource:)`.

### Data Directory

```
~/Library/Application Support/VelocityVisualiser/
├── sensor_data.db        (SQLite database)
├── server.log            (stdout/stderr capture)
└── server.pid            (PID file for lifecycle management)
```

This follows macOS conventions and keeps data out of the sandbox container
(requires the `com.apple.security.files.user-selected.read-write`
entitlement or a non-sandboxed helper — see Phase 4 trade-offs).

### Server Process Model

```
┌─────────────────────────┐
│   VelocityVisualiser    │
│   (Swift main process)  │
│                         │
│  ┌───────────────────┐  │     ┌──────────────────────────┐
│  │  ServerProcess    │──┼────▶│  velocity-report-server  │
│  │  Manager          │  │     │  (Go child process)      │
│  │                   │  │     │                          │
│  │  • start()        │  │     │  --listen :50051         │
│  │  • stop()         │  │     │  --grpc-only             │
│  │  • restart()      │  │     │  --db-path ~/Lib/...     │
│  │  • isRunning      │  │     │  --disable-radar         │
│  │  • pid / exitCode │  │     └──────────────────────────┘
│  └───────────────────┘  │
│                         │
│  ┌───────────────────┐  │
│  │  ServerManager    │──┼───── (existing, from server-manager.md)
│  │  (remote servers) │  │      Servers menu, persistence, etc.
│  └───────────────────┘  │
└─────────────────────────┘
```

The Go server runs as a child process of the Swift app. `ServerProcessManager`
owns the `Process` (née `NSTask`) handle. The process is independent of the
gRPC connection — the server can be running while the visualiser is not
streaming, and vice versa.

### Menu Structure

Extend the existing menu bar with a **Server** menu:

```
Server
├── Local Server
│   ├── Start Local Server        (⌃⌘S)
│   ├── Stop Local Server         (⌃⌘X)
│   ├── Restart Local Server      (⌃⌘R)
│   ├── ─────────
│   ├── ● Running on :50051            ← status indicator
│   ├── Open Data Folder...            ← reveals ~/Library/Application Support/
│   ├── View Server Log...             ← opens log in Console.app or TextEdit
│   └── ─────────
│       Start at Login    [✓]          ← toggle Login Item
├── ─────────
├── Servers                            ← existing server-manager.md menu
│   ├── Local                 ● (connected, auto-created)
│   ├── Office Pi             ○
│   ├── ─────────
│   ├── Add Server...         (⇧⌘N)
│   ├── Edit Servers...       (⇧⌘E)
│   └── Set Default           →
└── ─────────
    Connect to Local          (⇧⌘L)   ← shortcut: start server + connect
```

The "Connect to Local" shortcut is the common case: ensure the local server
is running, then connect the gRPC stream to it. One keyboard shortcut for
the entire workflow.

### Login Item Architecture (Phase 4)

macOS offers two mechanisms for starting at login:

| Mechanism              | Sandbox-safe | Requires helper app | User-visible                  |
| ---------------------- | ------------ | ------------------- | ----------------------------- |
| `SMAppService.mainApp` | Yes          | No                  | System Settings → Login Items |
| `SMAppService.agent()` | Yes          | Yes (helper bundle) | Hidden                        |

**Recommendation:** Use `SMAppService.mainApp` (macOS 13+). This registers
the _app itself_ as a Login Item. On login, macOS launches
VelocityVisualiser.app, which auto-starts its local server. The app can
run headless (no window) when launched as a Login Item by checking
`NSApp.activationPolicy`.

**User control:**

- Toggle via the "Start at Login" menu checkbox.
- Also visible in System Settings → General → Login Items.
- Easy to disable: uncheck in either location.

**Alternative (deferred):** A lightweight `VelocityServerHelper.app` in
`Contents/Library/LoginItems/` that runs only the Go server without the
full UI. More complex to build and sign, but avoids launching the full
app at login. Evaluate if users report the full-app approach is too heavy.

---

## Server Manager Integration

The existing [server-manager.md](server-manager.md) plan defines multi-server
connection management. This plan extends it:

1. **Auto-created "Local" entry:** On first launch, `ServerManager` seeds a
   "Local" server entry pointing at `localhost:50051`. This entry is marked
   as special (`isLocal: true`) and cannot be deleted or have its address
   edited.
2. **Connection logic:** When the user selects the "Local" server,
   `ServerProcessManager` ensures the Go process is running before the gRPC
   connection is attempted.
3. **Status integration:** The "Local" entry in the Servers menu shows process
   status (● Running / ○ Stopped) in addition to connection status.

---

## Entitlements

Current entitlements:

```xml
com.apple.security.app-sandbox              = true
com.apple.security.files.user-selected.read-only = true
com.apple.security.network.client           = true
```

Additional entitlements needed:

| Entitlement                                         | Why                                    |
| --------------------------------------------------- | -------------------------------------- |
| `com.apple.security.network.server`                 | Go binary listens on a port            |
| `com.apple.security.files.user-selected.read-write` | SQLite database in Application Support |

**Sandbox consideration:** The Go binary runs as a child process of the
sandboxed app. It inherits the sandbox. The `Application Support` directory
is accessible within the sandbox container
(`~/Library/Containers/com.velocity.visualiser/Data/Library/Application Support/`).
No entitlement escape is needed if we use the container path.

---

## Build Pipeline

### Makefile Changes

```makefile
# Build Go server for macOS ARM64, output to visualiser resources
build-server-for-mac:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
	go build -ldflags "..." \
	-o tools/visualiser-macos/VelocityVisualiser/Resources/velocity-report-server \
	./cmd/radar

# Existing build-mac target: add dependency
build-mac: build-server-for-mac
	xcodebuild ...
```

### Xcode Project Changes

- Add `velocity-report-server` to the Xcode project as a resource
  (Copy Bundle Resources build phase).
- Set "Skip Install" = Yes for the Go binary.
- Add a Run Script build phase that copies the Go binary from the build
  output, or reference the Makefile target.

### DMG Changes

No change — the Go binary ships inside the `.app` bundle, which is already
packaged into the DMG.

### Version Coupling

The Go binary version must match the app version. Both are stamped from the
same `VERSION` file and git SHA at build time. The app should verify the
embedded binary version on launch:

```swift
// velocity-report-server --version → "0.5.2-a1b2c3d"
// Compare with app's BuildInfo.gitSHA
```

Mismatches log a warning but do not block operation.

---

## Implementation Plan

### Phase 1 — ServerProcessManager (`S`)

1. Create `App/ServerProcessManager.swift` — `@Observable` class.
   - `start()` — locate binary via `Bundle.main`, launch `Process` with
     `--listen :50051 --grpc-only --db-path <container>/sensor_data.db
--disable-radar`.
   - `stop()` — `SIGTERM` → wait 5s → `SIGKILL`.
   - `restart()` — stop then start.
   - `isRunning: Bool` — poll via `Process.isRunning`.
   - `pid: Int32?`, `exitCode: Int32?` — observable state.
   - Capture stdout/stderr to `server.log` via `FileHandle`.
   - Write `server.pid` for external tooling.
2. Wire `AppState` to hold a `ServerProcessManager` instance.
3. Auto-start local server on app launch (before gRPC auto-connect).

**Depends on:** Go binary builds for macOS ARM64 (already exists:
`make build-radar-mac`).

### Phase 2 — Server Menu (`S`)

4. Add `CommandMenu("Server")` to `AppCommands`.
   - Start / Stop / Restart local server.
   - Status indicator (● Running / ○ Stopped / ⚠ Error).
   - "Open Data Folder..." → `NSWorkspace.shared.open()`.
   - "View Server Log..." → open `server.log` in Console.app.
5. Keyboard shortcuts: ⌃⌘S (start), ⌃⌘X (stop), ⌃⌘R (restart).
6. "Connect to Local" shortcut (⇧⌘L): start server if needed, then connect.

### Phase 3 — Build Pipeline (`S`)

7. Add `build-server-for-mac` Makefile target.
8. Add Go binary to Xcode project as a bundled resource.
9. Update `build-mac` to depend on `build-server-for-mac`.
10. Update `dmg-mac` and `dmg-mac-release` — no structural change needed
    since the binary is inside the `.app`.
11. Add version verification on app launch.

### Phase 4 — Login Item (`S`)

12. Add `SMAppService.mainApp` registration.
13. Add "Start at Login" toggle to Server menu.
14. On login-launch, detect launch reason and:
    - Start local server.
    - Suppress main window (set `NSApp.activationPolicy = .accessory`
      or check `ProcessInfo.processInfo.environment` for a launch flag).
    - Show menu bar status only.
15. Clicking the Dock icon or re-launching restores normal window behaviour.

**macOS version requirement:** macOS 13+ (Ventura) for `SMAppService`.
Current app minimum is macOS 14 — compatible.

### Phase 5 — Server Manager Integration (`S`)

16. Extend `ServerConfig` with `isLocal: Bool` flag.
17. Auto-create "Local" entry on first launch.
18. "Local" entry shows process status + connection status.
19. Selecting "Local" in Servers menu triggers `ServerProcessManager.start()`
    before gRPC connect.
20. Prevent editing/deleting the Local entry's host/port.

**Depends on:** [server-manager.md](server-manager.md) Phases 1–2.

---

## Failure Modes

| Failure                        | Detection                                             | Recovery                                                               |
| ------------------------------ | ----------------------------------------------------- | ---------------------------------------------------------------------- |
| Go binary missing from bundle  | `Bundle.main.url(forResource:)` returns nil           | Show alert: "Server binary not found. Reinstall VelocityVisualiser."   |
| Go binary crashes on start     | `Process.terminationHandler` fires with non-zero exit | Show error in Server menu status. Offer "View Server Log."             |
| Port already in use            | Go binary exits with bind error                       | Parse stderr, show alert: "Port 50051 already in use."                 |
| Database locked                | SQLite lock error in server log                       | Show in status. Suggest stopping other instances.                      |
| Stale PID file                 | PID file exists but process not running               | Remove stale PID, start normally.                                      |
| App killed without cleanup     | SIGKILL to app, server orphaned                       | On next launch, check PID file, kill orphan, restart.                  |
| Login item disabled externally | User disables in System Settings                      | Menu checkbox reflects actual state via `SMAppService.mainApp.status`. |

---

## Security Considerations

- **Binary provenance:** The Go binary is built from the same commit as the
  Swift app. Code-signed together as part of the app bundle.
- **No privilege escalation:** Server runs as the logged-in user, same
  as the app. No `sudo`, no daemon, no elevated privileges.
- **Network exposure:** Server listens on `localhost` only by default.
  The `--listen` flag is hardcoded to `:50051` (loopback).
  macOS firewall may prompt on first launch.
- **Sandbox:** Child process inherits the app sandbox. Database writes go
  to the sandbox container directory.

---

## Open Questions

1. **gRPC-only mode:** Does the Go server currently support a `--grpc-only`
   flag to skip HTTP API startup? If not, Phase 1 needs this flag added.
   Alternatively, start with both HTTP and gRPC and defer the flag.

2. **pcap dependency:** `build-radar-mac` links against libpcap. The
   bundled binary should use `build-radar-mac` (pcap available on macOS
   by default via Xcode CLT). Verify the resulting binary has no
   unexpected dylib dependencies (`otool -L`).

3. **Universal binary:** Should we ship both ARM64 and Intel Go binaries?
   The visualiser requires Apple Silicon (Metal). If the app already
   requires ARM64, a single-arch Go binary suffices.

4. **Database location:** Sandbox container path vs `~/Library/Application
Support/VelocityVisualiser/`. The sandbox container is safer but less
   discoverable. Start with the sandbox container; add "Open Data Folder"
   for discoverability.

5. **Server manager dependency:** Phases 1–4 can ship independently of
   the server-manager.md plan. Phase 5 requires server-manager Phases 1–2.
   Plan accordingly.

---

## Test Coverage

| Test                                                     | Type        |
| -------------------------------------------------------- | ----------- |
| `ServerProcessManager` start/stop/restart lifecycle      | Unit        |
| `ServerProcessManager` handles missing binary gracefully | Unit        |
| `ServerProcessManager` handles port-in-use error         | Unit        |
| `ServerProcessManager` cleans up stale PID on launch     | Unit        |
| Go binary `--version` output matches app version         | Integration |
| Go binary starts and accepts gRPC connection             | Integration |
| Login item toggle enables/disables `SMAppService`        | UI          |
| Server menu reflects running/stopped/error state         | UI          |
| "Connect to Local" starts server then connects gRPC      | Integration |
| DMG contains Go binary in `Contents/Resources/`          | Build       |
