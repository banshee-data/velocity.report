---
name: svelte-ux-review
description: Review a Svelte component or page for opportunities to use svelte-ux components instead of hand-rolled HTML. Produces a replacement table for designer review.
---

# Workflow: svelte-ux-review

Review Svelte files for UI elements that could be replaced with `svelte-ux` library components. Produces a structured replacement table for **designer review before any changes are made**.

Invoke with `#svelte-ux-review` in Copilot Chat, optionally specifying a file or directory.

## Design Principle

> Use `svelte-ux` components wherever a suitable one exists. Hand-rolled HTML for standard UI patterns (buttons, dialogs, form fields, tables, toggles, notifications) creates maintenance burden and visual inconsistency. The library is the canonical source of truth for interactive UI primitives in this project.

## Available svelte-ux Components (v2)

### Already in use

Button, Card, DateRangeField, Dialog, Header, Notification, NumberStepper, ProgressCircle, SelectField, Switch, Table, TextField, ToggleGroup, ToggleOption

### Available but unused — check for opportunities

AppBar, AppLayout, Avatar, Backdrop, Badge, Breadcrumb, ButtonGroup, Checkbox, Code, Collapse, CopyButton, Drawer, EmptyMessage, ErrorNotification, ExpansionPanel, Field, Form, Icon, Input, Kbd, ListItem, Menu, MenuButton, MenuItem, MultiSelect, MultiSelectField, Overflow, Paginate, Pagination, Popover, Progress, Radio, RangeField, RangeSlider, ScrollContainer, SectionDivider, Shine, Step, Steps, Tabs, Tab, Timeline, TimelineEvent, Toggle, ToggleButton, Tooltip, TreeList

## Procedure

### Step 1 — Identify scope

If a file path is given, read that file. If a directory is given, list `.svelte` files in it. If nothing is given, scan `web/src/routes/` for all page components.

### Step 2 — Scan for replaceable patterns

For each file, look for:

| Hand-rolled pattern                            | Candidate svelte-ux component         |
| ---------------------------------------------- | ------------------------------------- |
| `<input type="checkbox"`                       | `Checkbox` or `Switch`                |
| `<input type="radio"`                          | `Radio`                               |
| `<input type="text"` or `<input type="number"` | `TextField` or `NumberStepper`        |
| `<select>`                                     | `SelectField` or `MultiSelectField`   |
| `<table>` with manual thead/tbody              | `Table`                               |
| `<button` (not from svelte-ux)                 | `Button`                              |
| Modal/overlay with manual backdrop             | `Dialog` or `Drawer`                  |
| Toast/alert with manual styling                | `Notification` or `ErrorNotification` |
| Tab navigation with manual state               | `Tabs` + `Tab`                        |
| Accordion/collapsible sections                 | `ExpansionPanel` or `Collapse`        |
| Breadcrumb navigation                          | `Breadcrumb`                          |
| Tooltip via `title=` attribute                 | `Tooltip`                             |
| Progress bar `<div>` with width %              | `Progress` or `ProgressCircle`        |
| Empty state placeholder                        | `EmptyMessage`                        |
| Step/wizard indicators                         | `Steps` + `Step`                      |
| Pagination controls                            | `Pagination`                          |
| Badge/count indicators                         | `Badge`                               |
| Copy-to-clipboard buttons                      | `CopyButton`                          |
| Keyboard shortcut hints                        | `Kbd`                                 |
| Icon with raw `<svg>`                          | `Icon` (with mdi path)                |

### Step 3 — Produce the replacement table

Output a markdown table with columns:

| File                | Line(s) | Current                   | Suggested  | Impact | Notes       |
| ------------------- | ------- | ------------------------- | ---------- | ------ | ----------- |
| path/to/file.svelte | 42–48   | `<input type="checkbox">` | `Checkbox` | Low    | Simple swap |

**Impact levels:**

- **Low** — drop-in replacement, no logic change
- **Medium** — needs prop mapping or event handler adjustment
- **High** — structural change, affects layout or parent component

### Step 4 — Flag for designer review

End the output with:

> **Designer review required.** The replacements above change visual appearance and interaction patterns. Apply only after design approval.

Do NOT make any file edits. This workflow is advisory only.

### Step 5 — If asked to apply

If the user explicitly asks to apply changes after reviewing the table:

1. Apply only **Low** impact changes unless the user confirms others.
2. Import the component from `svelte-ux`.
3. Preserve all existing behaviour (event handlers, bindings, accessibility attributes).
4. Run `make format-web && make lint-web` after changes.
