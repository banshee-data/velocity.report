# macOS menu layout design

Active plan: [wireshark-menu-alignment.md](../../plans/wireshark-menu-alignment.md)

Defines the menu bar structure and keyboard shortcuts for VelocityVisualiser on macOS, aligned with Wireshark-style conventions for capture and playback workflows.

## Current VelocityVisualiser menu bar

| Menu           | Items                                                     |
| -------------- | --------------------------------------------------------- |
| **App Info**   | About VelocityReport.app                                  |
| **File**       | Connect/Disconnect (⇧⌘C), Open Recording... (⌘O)          |
| **Playback**   | Play/Pause (Space), Step (./,), Rate (]/[), Time Display  |
| **Overlays**   | Points (p), Boxes (b), Trails (t), Velocity (v), Grid (g) |
| **Labels**     | Label Selected Track (l), Classify submenu (1–9)          |
| **Annotation** | Open Annotation Window (⇧⌘A)                              |
| **Edit**       | Undo (⌘Z), Redo (⇧⌘Z)                                     |

## Single-Key shortcuts (immutable)

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

### In the Annotation window

The single keys above are bound app-wide with no modifier, so with the Annotation window in
front they are given that window's meaning rather than acting on a replay the operator is not
looking at. In the main window they are unchanged.

| Key      | In the Annotation window                           |
| -------- | -------------------------------------------------- |
| . / ,    | Next / previous frame of the pack                  |
| ] / [    | Larger / smaller brush                             |
| ⌘] / ⌘[  | Carry the mask forward / back                      |
| ⌘S       | Save points                                        |
| ⌘Z / ⇧⌘Z | Undo / redo a selection edit                       |
| Space    | Play/Pause the main view, which the window follows |

The routing is a focused scene value read by the menu commands. A bracket that reaches the
window both through its own binding and through the menu is applied once, by the key event's
timestamp. Undo and redo fall through to a text field that is being edited.

## Proposed new shortcuts (Wireshark-aligned)

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

## Target menu structure

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

## Implementation priority

### Phase 1: quick wins

Open Recent, Close Recording (⌘W), Go to Frame (⌘G), First/Last Frame
(⌘Home/⌘End), Keyboard Shortcuts reference.

### Phase 2: view enhancements

Zoom controls (⌘+/⌘-/⌘0), VRLOG File Properties, Preferences (⌘,),
Reset Layout.

### Phase 3: analysis features

Find Track (⌘F), Filter expression bar, Track statistics, Expert Info /
Quality Summary, Follow Track.

### Phase 4: export & polish

Export tracks as CSV/JSON, Copy track details (⌘C), Help links.

## Design principles (from Wireshark)

1. Consistent verb placement in menu items.
2. Ellipsis convention for items that open a dialogue.
3. Modifier key hierarchy: ⌘ primary, ⇧⌘ secondary, ⌥⌘ tertiary.
4. Standards compliance: ⌘O, ⌘W, ⌘,, ⌘Q.
5. No conflict with single-key shortcuts.
6. Time Display stays under Playback (more logical for replay).
7. Separation of concerns: View for display, Playback for temporal,
   Labels for annotation, Statistics for aggregates.
