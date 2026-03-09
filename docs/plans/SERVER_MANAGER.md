# Server Manager — Plan

**Layers:** L10 Clients (macOS visualiser)

> **Feature:** Multi-server connection management for VelocityVisualiser
>
> **Goal:** Allow users to configure, save, and switch between multiple
> velocity.report servers, with clear connection status feedback and a default
> server that auto-connects on launch.

---

## Current State

- **Hardcoded address:** `serverAddress` defaults to `"localhost:50051"` in
  `AppState` — no UI to change it.
- **Auto-connect:** App connects to `serverAddress` 500 ms after launch.
- **Status indicator:** Small coloured circle (`ConnectionStatusView`) —
  green when connected, red on error, grey when disconnected. Hover tooltip
  shows the address or error message.
- **Connect button:** `ConnectionButtonView` toggles connect/disconnect with
  play/stop icon and "Connect" / "Disconnect" / "Connecting..." text.
- **No persistence:** Server address is not saved between sessions.
- **Single server only:** No concept of a server list or switching.

---

## Requirements

### 1. Server Configuration Model

A saved server entry contains:

| Field           | Type    | Required | Notes                                   |
| --------------- | ------- | -------- | --------------------------------------- |
| `id`            | UUID    | auto     | Stable identity for persistence         |
| `name`          | String  | yes      | User-friendly label (e.g. "Office Pi")  |
| `host`          | String  | yes      | Hostname or IP address                  |
| `port`          | UInt16  | yes      | gRPC port (default: 50051)              |
| `isDefault`     | Bool    | no       | Auto-connect on launch (exactly one)    |
| `lastConnected` | Date?   | auto     | Timestamp of last successful connection |
| `notes`         | String? | no       | User notes (e.g. "South Rd sensor")     |

Derived: `address` → `"\(host):\(port)"`

### 2. Server List Menu

Add a **Servers** `CommandMenu` to the menu bar:

```
Servers
├── Office Pi          ● (connected)     ← green dot if active
├── Home Test          ○                 ← grey dot if saved but inactive
├── Lab Sensor         ○
├── ─────────
├── Add Server...      (⇧⌘N)
├── Edit Servers...    (⇧⌘E)            ← opens settings panel
├── ─────────
└── Set Default        →
    ├── ✓ Office Pi
    ├──   Home Test
    └──   Lab Sensor
```

Clicking a server name connects to it (disconnecting from the current server
first if needed).

### 3. Connection Status Callout

Improve the existing `ConnectionStatusView` to provide richer feedback:

#### 3a. Toast / Callout on Connect & Disconnect

When the connection state changes, show a brief transient callout (2–3 seconds)
overlaid on the main view:

- **Connected:** `"Connected to Office Pi (192.168.1.42:50051)"` — green
  background, checkmark icon.
- **Disconnected:** `"Disconnected from Office Pi"` — grey background, xmark
  icon.
- **Error:** `"Connection failed: cannot reach 192.168.1.42:50051"` — red
  background, exclamationmark icon.

Use SwiftUI `.overlay()` with animation, auto-dismiss via `Task.sleep`.

#### 3b. Status Indicator with Hover Detail

Replace the plain circle with a richer status badge in the toolbar:

- **Connected:** Green dot + server name (truncated to ~15 chars).
- **Connecting:** Spinner + "Connecting...".
- **Disconnected:** Grey dot + "No server".
- **Error:** Red dot + "Connection error".

On hover (`.help()` or popover), show:

```
Server:  Office Pi
Address: 192.168.1.42:50051
Status:  Connected
Uptime:  2h 14m
Frames:  12,847
```

### 4. Add / Edit Server Panel

A sheet (modal) or settings tab with:

- **Name** text field
- **Host** text field (with placeholder "e.g. 192.168.1.42 or raspberrypi.local")
- **Port** stepper/text field (default 50051)
- **Notes** text field (optional)
- **Test Connection** button — attempts a quick connect/disconnect, shows
  ✅ reachable or ❌ unreachable with latency
- **Set as Default** toggle
- **Save** / **Cancel** buttons
- **Delete** button (with confirmation, disabled if only one server)

### 5. Persistence

Use `UserDefaults` (or a JSON file in `~/Library/Application Support/VelocityVisualiser/`)
to store the server list. Persist on every change (add, edit, delete, set default,
update `lastConnected`).

Suggested key: `savedServers` — array of `Codable` `ServerConfig` structs.

On first launch with no saved servers, auto-create a default entry:
`{ name: "Local", host: "localhost", port: 50051, isDefault: true }`.

### 6. Default Server & Auto-Connect

- Exactly one server can be marked `isDefault`.
- On app launch, auto-connect to the default server (preserve current 500 ms
  delay behaviour).
- If no default is set, show the server list panel instead of auto-connecting.
- Changing the default does **not** disconnect the current session.

---

## UI Placement

| Component                | Location                                          |
| ------------------------ | ------------------------------------------------- |
| Server list menu         | `CommandMenu("Servers")` in menu bar              |
| Connection status badge  | Toolbar, replacing current `ConnectionStatusView` |
| Connection toast/callout | Overlay on `MetalView` (top-centre)               |
| Add/Edit server sheet    | Modal sheet on `ContentView`                      |
| Server list panel        | Popover from status badge, or sheet               |

---

## Implementation Plan

### Phase 1 — Model & Persistence

1. Create `ServerConfig` struct (`Codable`, `Identifiable`, `Equatable`)
2. Create `ServerManager` class (`@Observable` / `ObservableObject`)
   - `@Published var servers: [ServerConfig]`
   - `@Published var activeServerId: UUID?`
   - `@Published var defaultServerId: UUID?`
   - Load/save from `UserDefaults`
   - Methods: `add`, `update`, `delete`, `setDefault`, `setActive`
3. Seed default "Local" entry on first launch
4. Wire `AppState.serverAddress` to read from `ServerManager.activeServer`

### Phase 2 — Server Menu

5. Add `CommandMenu("Servers")` to `AppCommands`
   - List saved servers with status dot
   - Click to connect
   - "Add Server..." opens sheet
   - "Edit Servers..." opens settings
   - "Set Default" submenu
6. Add keyboard shortcuts (⇧⌘N for Add, ⇧⌘E for Edit)

### Phase 3 — Add / Edit UI

7. Create `ServerEditorView` (sheet)
   - Name, host, port, notes fields
   - Test Connection button (gRPC connect → immediate disconnect)
   - Save / Cancel / Delete actions
8. Create `ServerListView` (for Edit Servers)
   - Table/list of all saved servers
   - Edit, delete, reorder
   - Set default toggle per row

### Phase 4 — Connection Status Callout

9. Create `ConnectionToastView` — transient overlay
   - Triggered by `AppState.isConnected` changes
   - Auto-dismiss after 2.5 seconds
   - Animated slide-in/fade-out
10. Enhance `ConnectionStatusView`
    - Show server name + dot
    - Popover on click with full details (address, uptime, frames)

### Phase 5 — Polish

11. Handle edge cases:
    - Delete the currently connected server → disconnect first
    - Delete the default server → prompt to set a new default
    - Edit the currently connected server's address → disconnect + reconnect
    - No servers saved (all deleted) → re-seed "Local" default
12. Add `lastConnected` timestamp updates on successful connection
13. Accessibility: VoiceOver labels for status badge and toast

---

## Data Flow

```
┌──────────────┐      ┌──────────────┐      ┌─────────────────┐
│ ServerManager│─────▶│   AppState   │─────▶│ VisualiserClient│
│              │      │              │      │                 │
│ servers[]    │      │ serverAddress│      │ address         │
│ activeId     │─set─▶│ connect()    │─────▶│ connect()       │
│ defaultId    │      │ disconnect() │      │ disconnect()    │
│              │      │              │      │                 │
│ UserDefaults │      │ isConnected  │◀─────│ delegate        │
│ (persistence)│      │ isConnecting │      │ callbacks       │
└──────────────┘      │ connError    │      └─────────────────┘
                      └──────────────┘
                            │
                      ┌─────▼──────┐
                      │   Views    │
                      │            │
                      │ StatusBadge│
                      │ Toast      │
                      │ ServerMenu │
                      │ EditorSheet│
                      └────────────┘
```

---

## File Plan

| File                              | Purpose                                 |
| --------------------------------- | --------------------------------------- |
| `App/ServerConfig.swift`          | `ServerConfig` model struct             |
| `App/ServerManager.swift`         | Server list management + persistence    |
| `UI/ServerEditorView.swift`       | Add / Edit server sheet                 |
| `UI/ServerListView.swift`         | Server list panel (Edit Servers)        |
| `UI/ConnectionToastView.swift`    | Transient connection status overlay     |
| `UI/ConnectionStatusView.swift`   | Enhanced status badge (modify existing) |
| `App/VelocityVisualiserApp.swift` | Add Servers CommandMenu                 |
| `App/AppState.swift`              | Wire to ServerManager                   |

---

## Keyboard Shortcuts

| Shortcut | Action                        |
| -------- | ----------------------------- |
| ⇧⌘N      | Add Server...                 |
| ⇧⌘E      | Edit Servers...               |
| ⇧⌘C      | Connect/Disconnect (existing) |

---

## Test Coverage

| Test                                         | Type  |
| -------------------------------------------- | ----- |
| `ServerConfig` Codable round-trip            | Unit  |
| `ServerManager` add/edit/delete/setDefault   | Unit  |
| `ServerManager` persistence (UserDefaults)   | Unit  |
| `ServerManager` first-launch seeding         | Unit  |
| Address derivation (`host:port`)             | Unit  |
| Only one default at a time                   | Unit  |
| Delete active server triggers disconnect     | Unit  |
| `ConnectionToastView` renders for each state | UI    |
| `ServerEditorView` field validation          | UI    |
| Test Connection success/failure              | Integ |

---

## Open Questions

1. **Persistence format:** `UserDefaults` (simplest) vs JSON file in
   `Application Support` (more portable, visible)? Recommendation: start with
   `UserDefaults`, migrate later if needed.
2. **Server discovery:** Should we support mDNS/Bonjour to auto-discover
   velocity.report servers on the local network? Useful but adds complexity —
   defer to a future phase.
3. **Multiple simultaneous connections:** Currently single-server only. Is there
   a use case for viewing two servers side-by-side? Defer unless requested.
4. **Secure transport:** Visualiser↔server gRPC traffic MUST use TLS by default,
   ideally mutual TLS (mTLS) with certificate validation. Plaintext/insecure
   connections, if supported at all, SHOULD be an explicit, opt-in per-server
   debug mode (for trusted lab networks only), clearly marked as insecure in
   the UI rather than the default configuration.
