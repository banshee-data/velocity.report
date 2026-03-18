# sticker-generator

Cairo-based bumper sticker renderer for road-safety campaigns.

Generates printable sticker designs as **SVG** (vector) or **PNG** (bitmap)
from a simple configuration.  Two built-in designs ship with the tool:

| Design | Description |
| --- | --- |
| `20-is-plenty` | 20 mph campaign sticker — amber numeral on deep navy |
| `speed-kills` | Safety-awareness sticker — white text on safety red |

---

## Requirements

- **Python 3.11+**
- **pycairo 1.20+** — Python bindings for the Cairo graphics library
- **Cairo system libraries** — must be installed before pycairo

### Installing system libraries

**Debian / Ubuntu / Raspberry Pi OS:**

```bash
sudo apt install libcairo2-dev
```

**macOS (Homebrew):**

```bash
brew install cairo
```

**Fedora / RHEL:**

```bash
sudo dnf install cairo-devel
```

### Installing pycairo

Use the shared project venv (recommended):

```bash
make install-python        # sets up .venv
.venv/bin/pip install pycairo
```

Or install directly:

```bash
pip install pycairo
```

---

## Usage

### Generate all built-in designs (PNG)

```bash
python3 tools/sticker-generator/sticker_generator.py
```

Writes `20-is-plenty.png` and `speed-kills.png` to the current directory.

### Choose format and output directory

```bash
python3 sticker_generator.py --output svg --outdir /tmp/stickers
```

### Render a single built-in design

```bash
python3 sticker_generator.py --design speed-kills --output png
```

### Load a custom JSON config

```bash
python3 sticker_generator.py --config configs/20-is-plenty.json --output svg
```

---

## Configuration reference

All parameters are optional unless marked **required**.

### `StickerConfig` (top-level)

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `name` | string | `"sticker"` | Filename stem for the output file |
| `width_mm` | float | `305.0` | Physical width in millimetres |
| `height_mm` | float | `76.0` | Physical height in millimetres |
| `dpi` | int | `150` | Pixel density for PNG output (no effect on SVG) |
| `background_colour` | string | `"#ffffff"` | Background fill (`#RRGGBB` or `#RRGGBBAA`) |
| `border_colour` | string\|null | `null` | Border stroke colour; `null` = no border |
| `border_width_mm` | float | `1.5` | Border stroke width in millimetres |
| `blocks` | array | `[]` | List of `TextBlock` objects |
| `cool_s_decorations` | array | `[]` | List of `CoolSDecoration` objects |

### `TextBlock`

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `text` | string | **required** | The text to render |
| `font_size_pt` | float | `36.0` | Font size in typographic points |
| `bold` | bool | `false` | Use bold weight |
| `italic` | bool | `false` | Use italic style |
| `colour` | string | `"#000000"` | Text fill colour |
| `x_frac` | float | `0.5` | Horizontal anchor as fraction of sticker width (0–1) |
| `y_frac` | float | `0.5` | Vertical baseline as fraction of sticker height (0–1) |
| `anchor` | string | `"centre"` | `"left"`, `"centre"`, or `"right"` |

### `CoolSDecoration`

| Key | Type | Default | Description |
| --- | --- | --- | --- |
| `x_frac` | float | `0.1` | Horizontal centre as fraction of sticker width |
| `y_frac` | float | `0.5` | Vertical centre as fraction of sticker height |
| `size_frac` | float | `0.65` | Height of the S as a fraction of sticker height |
| `colour` | string | `"#000000"` | Stroke colour |
| `stroke_width_frac` | float | `0.048` | Stroke width as a fraction of the S height |

---

## Creating a custom design

### Via JSON

Copy an existing config and edit it:

```bash
cp configs/20-is-plenty.json configs/my-sticker.json
```

Edit `configs/my-sticker.json`, then render:

```bash
python3 sticker_generator.py --config configs/my-sticker.json --output svg
```

### Layout tip — "IS PLENTY" alignment rule

In the `20-is-plenty` design, the "IS PLENTY" text block uses the same
`x_frac` as the "20" numeral.  This keeps both text elements in a single
typographic column.  Adjust both `x_frac` values together when repositioning
the text group.

### Via Python

```python
from sticker_generator import StickerConfig, TextBlock, CoolSDecoration, render_sticker

cfg = StickerConfig(
    name="slow-down",
    width_mm=305,
    height_mm=76,
    background_colour="#003366",
    border_colour="#ffffff",
    border_width_mm=2.0,
    blocks=[
        TextBlock(
            text="SLOW DOWN",
            font_size_pt=66,
            bold=True,
            colour="#ffcc00",
            x_frac=0.55,
            y_frac=0.45,
            anchor="centre",
        ),
        TextBlock(
            text="it's 20 mph here",
            font_size_pt=18,
            italic=True,
            colour="#aaccff",
            x_frac=0.55,
            y_frac=0.75,
            anchor="centre",
        ),
    ],
    cool_s_decorations=[
        CoolSDecoration(
            x_frac=0.09,
            y_frac=0.50,
            size_frac=0.70,
            colour="#ffcc00",
        ),
    ],
)

render_sticker(cfg, "slow-down.png", output_format="png")
render_sticker(cfg, "slow-down.svg", output_format="svg")
```

---

## Design functions reference

| Function | Purpose |
| --- | --- |
| `draw_background(ctx, w, h, colour)` | Fill the canvas with a solid colour |
| `draw_border(ctx, w, h, colour, stroke_w)` | Draw a rectangular border |
| `draw_text_block(ctx, block, w, h)` | Render a `TextBlock` with proportional placement |
| `draw_cool_s(ctx, cx, cy, size, colour)` | Draw the classic cool-S doodle |
| `proportional_x(canvas_w, frac, offset)` | Compute absolute x from fraction |
| `proportional_y(canvas_h, frac, offset)` | Compute absolute y from fraction |
| `render_sticker(cfg, path, format)` | Orchestrate full render to SVG or PNG |

---

## The cool S

The cool-S path function draws the classic geometric S doodle (also called
Super S or graffiti S) using Cairo strokes.

**Construction:**

1. Six short horizontal lines: three at the top, three at the bottom.
2. A vertical stroke closes the **right side** of the top group.
3. A diagonal stroke crosses from the lower-right of the top group to the
   upper-left of the bottom group (the characteristic S crossbar).
4. A vertical stroke closes the **left side** of the bottom group.

All proportions scale with the `size` parameter, so it renders correctly at
any dimension.

---

## Makefile targets

```bash
make generate-stickers            # render both designs as PNG in /tmp/stickers
make generate-stickers-svg        # render both designs as SVG in /tmp/stickers
```
