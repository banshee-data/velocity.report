# macOS Menu Layout Design

Active plan: [wireshark-menu-alignment.md](../plans/wireshark-menu-alignment.md)

## Current VelocityVisualiser Menu Bar

| Menu         | Items                                                     |
| ------------ | --------------------------------------------------------- |
| **App Info** | About VelocityReport.app                                  |
| **File**     | Connect/Disconnect (⇧⌘C), Open Recording... (⌘O)          |
| **Playback** | Play/Pause (Space), Step (./,), Rate (]/[), Time Display  |
| **Overlays** | Points (p), Boxes (b), Trails (t), Velocity (v), Grid (g) |
| **Labels**   | Label Selected Track (l), Classify submenu (1–9)          |

## Single-Key Shortcuts (Immutable)

| Key   | Action                 |
| ----- | ---------------------- |
| Space | Play/Pause             |
| . / , | Step Forward/Back      |
| ] / [ | Increase/Decrease Rate |
| p     | Toggle Points          |
| b     | Toggle Boxes           |
| t     | Toggle Trails          |
| v     | Toggle Velocity        |
| g     | Toggle Grid            |
| l     | Label Selected Track   |
| 1–9   | Classify track         |

> Never change existing single-key shortcuts. They are optimised for rapid
> one-handed operation during replay review.

## Proposed New Shortcuts (Wireshark-Aligned)

| Shortcut | Action                | Wireshark Equivalent |
| -------- | --------------------- | -------------------- |
| ⌘G       | Go to Frame           | Go to Packet         |
| ⌘F       | Find Track            | Find Packet          |
| ⌘,       | Preferences           | Preferences          |
| ⌘+/⌘-/⌘0 | Zoom In/Out/Reset     | Zoom controls        |
| ⌘W       | Close Recording       | Close                |
| ⌘Home    | Jump to First Frame   | First Packet         |
| ⌘End     | Jump to Last Frame    | Last Packet          |
| ⌥→/⌥←    | Selection History Nav | History Nav          |

## Target Menu Structure

```
VelocityReport.app
├── About / Preferences (⌘,)
├── File
│   ├── Connect/Disconnect (⇧⌘C)
│   ├── Open Recording... (⌘O)
│   ├── Open Recent →
│   ├── Close Recording (⌘W)
│   └── Export Tracks...
├── Edit
│   ├── Find Track... (⌘F)
│   ├── Find Next (⌘G)
│   └── Copy Track Details (⌘C)
├── Playback
│   ├── Play/Pause, Step, Rate
│   ├── Go to Frame... (⌘G)
│   ├── First Frame (⌘Home) / Last Frame (⌘End)
│   └── Time Display modes
├── View (renamed from Overlays)
│   ├── Points, Boxes, Trails, Velocity, Grid
│   ├── Zoom In/Out/Reset
│   ├── Reset Layout
│   └── File Properties...
├── Labels (unchanged)
├── Statistics → Track Summary, Velocity Distribution, Quality
└── Help → Keyboard Shortcuts, User Guide, Release Notes
```

## Implementation Priority

### Phase 1 — Quick Wins

Open Recent, Close Recording (⌘W), Go to Frame (⌘G), First/Last Frame
(⌘Home/⌘End), Keyboard Shortcuts reference.

### Phase 2 — View Enhancements

Zoom controls (⌘+/⌘-/⌘0), VRLOG File Properties, Preferences (⌘,),
Reset Layout.

### Phase 3 — Analysis Features

Find Track (⌘F), Filter expression bar, Track statistics, Expert Info /
Quality Summary, Follow Track.

### Phase 4 — Export & Polish

Export tracks as CSV/JSON, Copy track details (⌘C), Help links.

## Design Principles (from Wireshark)

1. Consistent verb placement in menu items.
2. Ellipsis convention for items that open a dialogue.
3. Modifier key hierarchy: ⌘ primary, ⇧⌘ secondary, ⌥⌘ tertiary.
4. Standards compliance: ⌘O, ⌘W, ⌘,, ⌘Q.
5. No conflict with single-key shortcuts.
6. Time Display stays under Playback (more logical for replay).
7. Separation of concerns: View for display, Playback for temporal,
   Labels for annotation, Statistics for aggregates.
