# Wireshark Menu Alignment Plan

**Layers:** L10 Client (macOS visualiser)

> **Purpose:** Compare VelocityVisualiser menu structure with Wireshark's proven
> menu design patterns to identify gaps, adopt relevant conventions, and align
> keyboard shortcuts where appropriate.
>
> **Source:** [`wireshark_main_window.ui`](https://github.com/wireshark/wireshark/blob/96fed9f3402169440cc662336827e46da6839d05/ui/qt/wireshark_main_window.ui)
> (Qt XML, Wireshark 4.x)

---

## Menu Structure Comparison

### Wireshark Menu Bar

| Menu           | Key Sections                                                                      |
| -------------- | --------------------------------------------------------------------------------- |
| **File**       | Open, Open Recent, Merge, Close, Save, Save As, File Set, Export, Print, Quit     |
| **Edit**       | Copy, Find, Mark/Unmark, Ignore, Time References, Time Shift, Preferences         |
| **View**       | Toolbars, Full Screen, Panes, Time Display Format, Name Resolution, Zoom, Colours |
| **Go**         | Go to Packet, Next/Prev Packet, First/Last, Conversation Nav, Auto Scroll         |
| **Capture**    | Options, Start, Stop, Restart, Capture Filters, Refresh Interfaces                |
| **Analyze**    | Display Filters, Apply/Prepare Filter, Follow Stream, Decode As, Expert Info      |
| **Statistics** | File Properties, Protocol Hierarchy, Conversations, Endpoints, I/O Graphs, Plots  |
| **Telephony**  | VoIP Calls, RTP, SCTP, SIP, GSM, LTE, MTP3 (protocol-specific)                    |
| **Wireless**   | Bluetooth, WLAN Traffic                                                           |
| **Tools**      | Firewall ACL, Credentials, MAC Lookup, TLS Keylog                                 |
| **Help**       | User Guide, Manual Pages, Website, FAQ, Wiki, Release Notes, About                |

### VelocityVisualiser Menu Bar (current)

| Menu         | Items                                                                         |
| ------------ | ----------------------------------------------------------------------------- |
| **App Info** | About VelocityReport.app                                                      |
| **File**     | Connect/Disconnect (⇧⌘C), Open Recording... (⌘O)                              |
| **Playback** | Play/Pause (Space), Step Fwd (.), Step Back (,), Rate +/- (]/[), Time Display |
| **Overlays** | Points (p), Boxes (b), Trails (t), Velocity (v), Grid (g)                     |
| **Labels**   | Label Selected Track (l), Classify submenu (1–9)                              |

---

## Alignment Matrix

Each Wireshark menu item is mapped to a VelocityVisualiser equivalent (existing
or proposed). Categories:

- ✅ **Already have** — feature exists in VelocityVisualiser
- 🟡 **Should add** — relevant feature, should implement
- 🔵 **Consider** — might be useful, needs evaluation
- ⬜ **Not applicable** — Wireshark-specific, no VelocityVisualiser equivalent

### File Menu

| Wireshark Item            | VV Equivalent          | Status | Notes                                          |
| ------------------------- | ---------------------- | ------ | ---------------------------------------------- |
| Open                      | Open Recording... (⌘O) | ✅     | Opens VRLOG directory                          |
| Open Recent               | —                      | 🟡     | Submenu of recently opened VRLOGs              |
| Merge                     | —                      | ⬜     | No packet merging concept                      |
| Import from Hex Dump      | —                      | ⬜     | Protocol-specific                              |
| Close                     | —                      | 🟡     | Close current VRLOG / disconnect               |
| Save / Save As            | —                      | 🔵     | Export current recording? (read-only replayer) |
| File Set                  | —                      | 🔵     | Navigate between related VRLOGs                |
| Export Packet Dissections | —                      | 🔵     | Export tracks as CSV/JSON                      |
| Export Packet Bytes       | —                      | ⬜     | Byte-level export N/A                          |
| Export Objects            | —                      | ⬜     | Protocol-specific                              |
| Print                     | —                      | 🔵     | Print current view / generate PDF?             |
| Quit                      | (⌘Q — system)          | ✅     | macOS provides automatically                   |

### Edit Menu

| Wireshark Item     | VV Equivalent            | Status | Notes                                         |
| ------------------ | ------------------------ | ------ | --------------------------------------------- |
| Copy (submenu)     | —                        | 🔵     | Copy track details / point data to clipboard  |
| Find Packet (⌘F)   | —                        | 🟡     | Find track by ID / velocity range             |
| Find Next/Previous | —                        | 🔵     | Navigate between search results               |
| Mark/Unmark        | Label Selected Track (l) | ✅     | Labelling system serves similar purpose       |
| Ignore/Unignore    | —                        | 🔵     | Hide tracks from view?                        |
| Set Time Reference | —                        | 🔵     | Mark a frame as t=0 reference                 |
| Time Shift         | —                        | ⬜     | Not applicable for replay                     |
| Packet Comments    | —                        | 🔵     | Annotate tracks / frames                      |
| Preferences (⌘,)   | —                        | 🟡     | Settings panel (connection, display defaults) |

### View Menu

| Wireshark Item          | VV Equivalent          | Status | Notes                                        |
| ----------------------- | ---------------------- | ------ | -------------------------------------------- |
| Main Toolbar            | (always visible)       | ✅     | Toolbar with playback controls               |
| Filter Toolbar          | —                      | 🔵     | Filter tracks by velocity / quality flags    |
| Full Screen             | (system ⌃⌘F)           | ✅     | macOS provides automatically                 |
| Packet List             | Track list panel       | ✅     | Side panel with track details                |
| Packet Details          | Track inspector        | ✅     | Selected track properties                    |
| Packet Bytes            | —                      | ⬜     | Raw bytes not applicable                     |
| **Time Display Format** | **Time Display modes** | ✅     | Elapsed / Remaining / Frames (Playback menu) |
| Name Resolution         | —                      | ⬜     | Network-specific                             |
| Zoom In/Out/Normal      | —                      | 🟡     | Camera zoom controls; could add ⌘+/⌘-/⌘0     |
| Expand/Collapse All     | —                      | 🔵     | Expand/collapse track detail sections        |
| Colorize Packet List    | —                      | 🔵     | Colour tracks by classification label        |
| Coloring Rules          | —                      | 🔵     | Custom colouring rules for tracks            |
| Reset Layout            | —                      | 🔵     | Reset camera / panel layout to defaults      |
| Internals > Shortcuts   | —                      | 🟡     | Show keyboard shortcut reference             |

### Go Menu

| Wireshark Item            | VV Equivalent       | Status | Notes                                         |
| ------------------------- | ------------------- | ------ | --------------------------------------------- |
| Go to Packet (⌘G)         | —                   | 🟡     | Go to frame number (seekbar provides partial) |
| Next/Previous Packet      | Step Fwd/Back (./,) | ✅     | Frame stepping                                |
| First/Last Packet         | —                   | 🟡     | Jump to first/last frame (Home/End keys?)     |
| Next/Prev in Conversation | —                   | 🔵     | Next/prev frame where selected track appears  |
| Auto Scroll               | —                   | ⬜     | Live capture auto-scroll (N/A for replay)     |
| History Navigation        | —                   | 🔵     | Selection history (⌥←/⌥→)                     |

### Capture Menu

| Wireshark Item     | VV Equivalent            | Status | Notes                             |
| ------------------ | ------------------------ | ------ | --------------------------------- |
| Start/Stop         | Connect/Disconnect (⇧⌘C) | ✅     | Analogous: start/stop data stream |
| Restart            | —                        | 🔵     | Reconnect to server               |
| Options            | —                        | 🟡     | Connection settings (host, port)  |
| Capture Filters    | —                        | ⬜     | BPF filters N/A                   |
| Refresh Interfaces | —                        | ⬜     | Network interfaces N/A            |

### Analyze Menu

| Wireshark Item          | VV Equivalent | Status | Notes                                          |
| ----------------------- | ------------- | ------ | ---------------------------------------------- |
| Display Filters         | —             | 🟡     | Filter expression builder for tracks           |
| Apply/Prepare as Filter | —             | 🔵     | Apply track property as filter                 |
| Follow Stream           | —             | 🔵     | Follow a track across frames (highlight trail) |
| Decode As               | —             | ⬜     | Protocol-specific                              |
| Expert Info             | —             | 🔵     | Quality flag summary / anomaly report          |

### Statistics Menu

| Wireshark Item          | VV Equivalent | Status | Notes                                        |
| ----------------------- | ------------- | ------ | -------------------------------------------- |
| Capture File Properties | —             | 🟡     | VRLOG metadata: duration, frames, site info  |
| Protocol Hierarchy      | —             | ⬜     | Protocol-specific                            |
| Conversations           | —             | 🔵     | Track summary: count, avg velocity, duration |
| Endpoints               | —             | ⬜     | Network endpoints N/A                        |
| Packet Lengths          | —             | 🔵     | Point count per frame distribution           |
| I/O Graphs              | —             | 🔵     | Tracks/frame over time, velocity histogram   |
| Plots                   | —             | 🔵     | Live plotting of track properties            |

### Help Menu

| Wireshark Item     | VV Equivalent            | Status | Notes                         |
| ------------------ | ------------------------ | ------ | ----------------------------- |
| User Guide (F1)    | —                        | 🔵     | Link to docs site             |
| Keyboard Shortcuts | —                        | 🟡     | Show shortcut reference sheet |
| Website            | —                        | 🔵     | Open velocity.report website  |
| Release Notes      | —                        | 🔵     | Show CHANGELOG                |
| About              | About VelocityReport.app | ✅     | Already implemented           |

---

## Keyboard Shortcut Alignment

### Current VelocityVisualiser Shortcuts (single-key, keep as-is)

| Key   | Action               | Wireshark Equivalent |
| ----- | -------------------- | -------------------- |
| Space | Play/Pause           | —                    |
| .     | Step Forward         | —                    |
| ,     | Step Backward        | —                    |
| ]     | Increase Rate        | —                    |
| [     | Decrease Rate        | —                    |
| p     | Toggle Points        | —                    |
| b     | Toggle Boxes         | —                    |
| t     | Toggle Trails        | —                    |
| v     | Toggle Velocity      | —                    |
| g     | Toggle Grid          | —                    |
| l     | Label Selected Track | —                    |
| 1–9   | Classify track       | —                    |

> **Rule:** Never change existing single-key shortcuts. They are optimised for
> rapid one-handed operation during replay review.

### Proposed New Shortcuts (multi-key combos, Wireshark-aligned)

| Shortcut | Proposed Action               | Wireshark Equivalent                 |
| -------- | ----------------------------- | ------------------------------------ |
| ⌘O       | Open Recording (exists)       | Open (⌘O)                            |
| ⇧⌘C      | Connect/Disconnect (exists)   | — (Capture Start is ⌘E in Wireshark) |
| ⌘G       | Go to Frame                   | Go to Packet (⌘G)                    |
| ⌘F       | Find Track                    | Find Packet (⌘F)                     |
| ⌘,       | Preferences                   | Preferences (⌘,)                     |
| ⌘+       | Zoom In                       | Zoom In (⌘+)                         |
| ⌘-       | Zoom Out                      | Zoom Out (⌘-)                        |
| ⌘0       | Reset Zoom                    | Normal Size (⌘0)                     |
| ⌘W       | Close Recording               | Close (⌘W)                           |
| ⌘Home    | Jump to First Frame           | First Packet (⌘Home)                 |
| ⌘End     | Jump to Last Frame            | Last Packet (⌘End)                   |
| ⌥→       | Next in Selection History     | Next Packet In History (⌥→)          |
| ⌥←       | Previous in Selection History | Previous Packet In History (⌥←)      |

---

## Implementation Priority

### Phase 1 — Quick Wins (low effort, high value)

1. **Open Recent** submenu — track last 5–10 opened VRLOGs
2. **Close Recording** (⌘W) — unload current VRLOG, return to connection view
3. **Go to Frame** (⌘G) — input field to jump to frame number
4. **First/Last Frame** (⌘Home / ⌘End) — jump to boundaries
5. **Keyboard Shortcuts reference** — show shortcut sheet in Help menu

### Phase 2 — View Enhancements

6. **Zoom controls** (⌘+, ⌘-, ⌘0) — camera zoom via keyboard
7. **VRLOG File Properties** — show metadata panel (duration, frames, site)
8. **Preferences** (⌘,) — connection settings, display defaults, overlay colours
9. **Reset Layout** — reset camera position and panel visibility

### Phase 3 — Analysis Features

10. **Find Track** (⌘F) — search by track ID, velocity range, quality flags
11. **Filter expression bar** — filter visible tracks by properties
12. **Track statistics** — count, velocity distribution, duration histogram
13. **Expert Info / Quality Summary** — aggregate quality flag overview
14. **Follow Track** — highlight all frames containing selected track

### Phase 4 — Export & Polish

15. **Export tracks as CSV/JSON** — selected or all tracks
16. **Copy track details** — clipboard support for track properties
17. **Help > User Guide** — link to documentation site
18. **Help > Release Notes** — show CHANGELOG content

---

## Menu Structure Proposal (post-alignment)

```
VelocityReport.app
├── About VelocityReport.app
├── Preferences...          (⌘,)     [Phase 2]
│
├── File
│   ├── Connect/Disconnect  (⇧⌘C)
│   ├── Open Recording...   (⌘O)
│   ├── Open Recent         →        [Phase 1]
│   ├── ─────────
│   ├── Close Recording     (⌘W)     [Phase 1]
│   ├── ─────────
│   └── Export Tracks...              [Phase 4]
│
├── Edit
│   ├── Find Track...       (⌘F)     [Phase 3]
│   ├── Find Next           (⌘G)     [Phase 3]
│   ├── ─────────
│   └── Copy Track Details  (⌘C)     [Phase 4]
│
├── Playback
│   ├── Play/Pause          (Space)
│   ├── Step Forward        (.)
│   ├── Step Backward       (,)
│   ├── ─────────
│   ├── Increase Rate       (])
│   ├── Decrease Rate       ([)
│   ├── ─────────
│   ├── Go to Frame...      (⌘G)     [Phase 1]
│   ├── First Frame         (⌘Home)  [Phase 1]
│   ├── Last Frame          (⌘End)   [Phase 1]
│   ├── ─────────
│   ├── Elapsed Time
│   ├── Remaining Time
│   └── Frame Index
│
├── View (renamed from Overlays)
│   ├── Points              (p)
│   ├── Boxes               (b)
│   ├── Trails              (t)
│   ├── Velocity            (v)
│   ├── Grid                (g)
│   ├── ─────────
│   ├── Zoom In             (⌘+)     [Phase 2]
│   ├── Zoom Out            (⌘-)     [Phase 2]
│   ├── Reset Zoom          (⌘0)     [Phase 2]
│   ├── Reset Layout                  [Phase 2]
│   ├── ─────────
│   └── File Properties...            [Phase 2]
│
├── Labels
│   ├── Label Selected      (l)
│   ├── ─────────
│   └── Classify            →
│       ├── (label 1)       (1)
│       ├── (label 2)       (2)
│       └── ...             (3–9)
│
├── Statistics                         [Phase 3]
│   ├── Track Summary
│   ├── Velocity Distribution
│   ├── Quality Flag Summary
│   └── Frame Point Counts
│
└── Help
    ├── Keyboard Shortcuts             [Phase 1]
    ├── ─────────
    ├── User Guide                     [Phase 4]
    ├── Release Notes                  [Phase 4]
    └── About VelocityReport.app
```

---

## Design Principles (adopted from Wireshark)

1. **Consistent verb placement** — action verbs in menu items (Go to, Find, Export)
2. **Ellipsis convention** — items that open a dialogue end with "..." (e.g. "Find Track...")
3. **Modifier key hierarchy** — ⌘ for primary, ⇧⌘ for secondary, ⌥⌘ for tertiary
4. **Standards compliance** — ⌘O (Open), ⌘W (Close), ⌘, (Preferences), ⌘Q (Quit)
5. **No conflict with single-key shortcuts** — all new shortcuts use modifier keys
6. **Time Display in View** — Wireshark places time format under View; VelocityVisualiser
   keeps it under Playback (more logical for replay context)
7. **Separation of concerns** — View for display options, Playback for temporal navigation,
   Labels for annotation, Statistics for aggregate analysis
