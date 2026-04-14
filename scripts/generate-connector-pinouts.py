#!/usr/bin/env python3
"""Generate face-on connector pinout SVGs from radar-wiring.yml.

Reads the wiring harness YAML and produces SVG (+ PNG) connector face
views with colored pins for:

  - M12 4-pin (sensor cable)
  - DE-9 9-pin (serial HAT)

Pin assignments and wire colors come from the YAML so the connector
diagrams stay consistent with the WireViz harness.

Usage:
    python3 scripts/generate-connector-pinouts.py
    make wiring   # calls this automatically
"""

import math
import shutil
import subprocess
import sys
from pathlib import Path

try:
    import yaml
except ImportError:
    sys.exit("pyyaml required (pip install pyyaml)")


# ── Wire color definitions (IEC 60757 codes used by WireViz) ──────────
#   code → (fill, stroke, text-on-fill, display name)

COLORS = {
    "BK": ("#222222", "#000000", "#ffffff", "Black"),
    "BN": ("#8B4513", "#6B3410", "#ffffff", "Brown"),
    "BG": ("#8B4513", "#6B3410", "#ffffff", "Brown"),
    "RD": ("#DC2626", "#B91C1C", "#ffffff", "Red"),
    "OG": ("#EA580C", "#C2410C", "#ffffff", "Orange"),
    "YE": ("#EAB308", "#A16207", "#000000", "Yellow"),
    "GN": ("#16A34A", "#15803D", "#ffffff", "Green"),
    "BU": ("#2563EB", "#1D4ED8", "#ffffff", "Blue"),
    "VT": ("#7C3AED", "#6D28D9", "#ffffff", "Violet"),
    "GY": ("#9CA3AF", "#6B7280", "#000000", "Grey"),
    "WH": ("#F3F4F6", "#9CA3AF", "#000000", "White"),
    "PK": ("#EC4899", "#DB2777", "#ffffff", "Pink"),
}

NC = ("#D1D5DB", "#9CA3AF", "#6B7280", "n/c")  # not connected

REPO = Path(__file__).resolve().parent.parent
WIRING_YAML = REPO / "docs" / "platform" / "hardware" / "radar-wiring.yml"
OUT_DIR = REPO / "docs" / "platform" / "hardware"

FONT = '-apple-system, system-ui, "Segoe UI", sans-serif'


# ── YAML parsing ──────────────────────────────────────────────────────


def parse_wiring():
    """Extract pin assignments and wire colors from the harness YAML."""
    with open(WIRING_YAML) as f:
        data = yaml.safe_load(f)

    cons = data["connectors"]
    cabs = data["cables"]
    conns = data["connections"]

    def find_connections(*names):
        """Find all connection groups routing through all named endpoints."""
        results = []
        for grp in conns:
            eps = {}
            for ep in grp:
                for k, v in ep.items():
                    eps[k] = v
            if all(n in eps for n in names):
                results.append(eps)
        return results

    # ── M12 cable pin data ────────────────────────────────────────────
    m12_wire_colors = cabs["M12 cable"]["colors"]
    ops_label_map = dict(zip(cons["OPS7243"]["pins"], cons["OPS7243"]["pinlabels"]))
    m12_pins = {}
    for grp in find_connections("OPS7243", "M12 cable"):
        for ops_pin, wire in zip(grp["OPS7243"], grp["M12 cable"]):
            if wire not in m12_pins:
                m12_pins[wire] = (
                    m12_wire_colors[wire - 1],
                    ops_label_map.get(ops_pin, f"Pin {ops_pin}"),
                )

    # ── DE-9 pin data ─────────────────────────────────────────────────
    de9_labels = cons["HAT serial"]["pinlabels"]
    de9_connected = {}
    for cable_name in cabs:
        cable_colors = cabs[cable_name]["colors"]
        for grp in find_connections(cable_name, "HAT serial"):
            for wire, pin in zip(grp[cable_name], grp["HAT serial"]):
                de9_connected[pin] = cable_colors[wire - 1]

    return m12_pins, de9_labels, de9_connected


# ── SVG helpers ───────────────────────────────────────────────────────


def _color(code):
    """Return (fill, stroke, text, name) for a wire color code."""
    return COLORS.get(code, NC) if code else NC


# ── M12 4-pin connector SVG ──────────────────────────────────────────


def m12_svg(pins):
    """Face-on M12 A-coded 4-pin female connector."""
    W, H = 280, 230
    CX, CY = 140, 108
    BODY = 56
    PCIRC = 27
    DOT = 12

    # M12 A-coded pin angles (CW from 12 o'clock, looking at female face)
    ANGLES = {1: 45, 2: 135, 3: 225, 4: 315}

    s = []
    s.append(
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}">\n'
        f"<style>\n"
        f"  text {{ font-family: {FONT}; }}\n"
        f"</style>\n"
        f'<rect width="{W}" height="{H}" fill="white" rx="8"/>'
    )

    # Title
    s.append(
        f'<text x="{CX}" y="24" text-anchor="middle" font-size="13"'
        f' font-weight="600" fill="#1F2937">M12 4-pin · sensor cable</text>'
    )
    s.append(
        f'<text x="{CX}" y="40" text-anchor="middle" font-size="10"'
        f' fill="#9CA3AF">Female face (looking at cable end)</text>'
    )

    # Connector body
    s.append(
        f'<circle cx="{CX}" cy="{CY}" r="{BODY}"'
        f' fill="#E5E7EB" stroke="#9CA3AF" stroke-width="2.5"/>'
    )

    # Key notch at 12 o'clock
    ky = CY - BODY
    s.append(
        f'<rect x="{CX - 6}" y="{ky - 1}" width="12" height="10"'
        f' rx="2" fill="#9CA3AF"/>'
    )
    s.append(
        f'<rect x="{CX - 4}" y="{ky}" width="8" height="8"' f' rx="1" fill="white"/>'
    )

    # Pins and labels
    for pn in (1, 2, 3, 4):
        a = math.radians(ANGLES[pn])
        px = CX + PCIRC * math.sin(a)
        py = CY - PCIRC * math.cos(a)

        code, signal = pins.get(pn, (None, ""))
        fill, stroke, tc, name = _color(code)

        # Colored dot
        s.append(
            f'<circle cx="{px:.1f}" cy="{py:.1f}" r="{DOT}"'
            f' fill="{fill}" stroke="{stroke}" stroke-width="1.5"/>'
        )
        # Pin number
        s.append(
            f'<text x="{px:.1f}" y="{py + 4:.1f}" text-anchor="middle"'
            f' font-size="11" font-weight="700" fill="{tc}">{pn}</text>'
        )

        if not signal:
            continue

        # Leader line + label outside the body
        lr = BODY + 16
        lx = CX + lr * math.sin(a)
        ly = CY - lr * math.cos(a)
        ex = CX + (BODY + 3) * math.sin(a)
        ey = CY - (BODY + 3) * math.cos(a)
        anchor = "start" if pn in (1, 2) else "end"

        s.append(
            f'<line x1="{ex:.1f}" y1="{ey:.1f}"'
            f' x2="{lx:.1f}" y2="{ly:.1f}"'
            f' stroke="#D1D5DB" stroke-width="0.75"/>'
        )
        s.append(
            f'<text x="{lx:.1f}" y="{ly + 1:.1f}" text-anchor="{anchor}"'
            f' font-size="11" fill="#374151">{signal}</text>'
        )
        s.append(
            f'<text x="{lx:.1f}" y="{ly + 13:.1f}" text-anchor="{anchor}"'
            f' font-size="9" fill="#9CA3AF">{name} wire</text>'
        )

    s.append("</svg>")
    return "\n".join(s)


# ── DE-9 connector SVG ───────────────────────────────────────────────


def de9_svg(labels, connected):
    """Face-on DE-9 male connector with colored connected pins."""
    W, H = 340, 195
    CX, CY = 170, 82
    PIN_SP = 32
    ROW_SP = 30
    DOT = 10

    # D-shell dimensions
    sw = 5 * PIN_SP + 40
    sh = ROW_SP + 44
    sl = CX - sw / 2
    sr = CX + sw / 2
    st = CY - sh / 2
    sb = CY + sh / 2
    taper = 10
    r = 14

    s = []
    s.append(
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}">\n'
        f"<style>\n"
        f"  text {{ font-family: {FONT}; }}\n"
        f"</style>\n"
        f'<rect width="{W}" height="{H}" fill="white" rx="8"/>'
    )

    # Title
    s.append(
        f'<text x="{CX}" y="22" text-anchor="middle" font-size="13"'
        f' font-weight="600" fill="#1F2937">DE-9 · serial HAT</text>'
    )
    s.append(
        f'<text x="{CX}" y="38" text-anchor="middle" font-size="10"'
        f' fill="#9CA3AF">Male face (looking at HAT connector)</text>'
    )

    # D-shell outline (trapezoid with rounded corners, wider at top)
    s.append(
        f'<path d="'
        f"M {sl + r},{st} L {sr - r},{st} "
        f"Q {sr},{st} {sr},{st + r} "
        f"L {sr - taper},{sb - r} "
        f"Q {sr - taper},{sb} {sr - taper - r},{sb} "
        f"L {sl + taper + r},{sb} "
        f"Q {sl + taper},{sb} {sl + taper},{sb - r} "
        f"L {sl},{st + r} "
        f'Q {sl},{st} {sl + r},{st} Z"'
        f' fill="#E5E7EB" stroke="#9CA3AF" stroke-width="2"/>'
    )

    def pin_xy(n):
        if n <= 5:
            return CX + (n - 3) * PIN_SP, CY - ROW_SP / 2
        return CX + (n - 7.5) * PIN_SP, CY + ROW_SP / 2

    # All 9 pins
    for pn in range(1, 10):
        px, py = pin_xy(pn)
        cc = connected.get(pn)
        fill, stroke, tc, _ = _color(cc)

        s.append(
            f'<circle cx="{px:.1f}" cy="{py:.1f}" r="{DOT}"'
            f' fill="{fill}" stroke="{stroke}" stroke-width="1.5"/>'
        )
        s.append(
            f'<text x="{px:.1f}" y="{py + 3.5:.1f}" text-anchor="middle"'
            f' font-size="9" font-weight="700" fill="{tc}">{pn}</text>'
        )

    # Labels for connected pins below the shell
    label_y = sb + 16
    for pn in sorted(connected):
        px, py = pin_xy(pn)
        cc = connected[pn]
        fill, _, _, name = _color(cc)
        label = labels[pn - 1] if pn <= len(labels) else ""

        # Leader line
        s.append(
            f'<line x1="{px:.1f}" y1="{py + DOT + 2:.1f}"'
            f' x2="{px:.1f}" y2="{label_y - 12:.1f}"'
            f' stroke="#D1D5DB" stroke-width="0.75"/>'
        )
        # Small color dot
        s.append(f'<circle cx="{px:.1f}" cy="{label_y - 6:.1f}" r="4" fill="{fill}"/>')
        # Signal name
        s.append(
            f'<text x="{px:.1f}" y="{label_y + 6:.1f}" text-anchor="middle"'
            f' font-size="10" font-weight="600" fill="#374151">{label}</text>'
        )
        # Wire color
        s.append(
            f'<text x="{px:.1f}" y="{label_y + 18:.1f}" text-anchor="middle"'
            f' font-size="9" fill="#9CA3AF">{name}</text>'
        )

    s.append("</svg>")
    return "\n".join(s)


# ── PNG conversion ────────────────────────────────────────────────────


def svg_to_png(svg_path, png_path, scale=2):
    """Convert SVG to PNG. Tries rsvg-convert, then cairosvg."""
    if shutil.which("rsvg-convert"):
        subprocess.run(
            [
                "rsvg-convert",
                "-z",
                str(scale),
                "-o",
                str(png_path),
                str(svg_path),
            ],
            check=True,
        )
        return True

    try:
        import cairosvg

        cairosvg.svg2png(url=str(svg_path), write_to=str(png_path), scale=scale)
        return True
    except ImportError:
        pass

    return False


# ── Main ──────────────────────────────────────────────────────────────


def main():
    m12_pins, de9_labels, de9_connected = parse_wiring()

    m12_svg_path = OUT_DIR / "m12-pinout.svg"
    de9_svg_path = OUT_DIR / "de9-pinout.svg"

    m12_svg_path.write_text(m12_svg(m12_pins))
    de9_svg_path.write_text(de9_svg(de9_labels, de9_connected))

    # Convert to PNG for WireViz embedding
    png_ok = True
    for svg_path in (m12_svg_path, de9_svg_path):
        png_path = svg_path.with_suffix(".png")
        if not svg_to_png(svg_path, png_path):
            png_ok = False

    for p in (m12_svg_path, de9_svg_path):
        print(f"  {p.relative_to(REPO)}")
        png = p.with_suffix(".png")
        if png.exists():
            print(f"  {png.relative_to(REPO)}")

    if not png_ok:
        print(
            "  ⚠ PNG conversion failed; install librsvg (brew install librsvg)",
            file=sys.stderr,
        )


if __name__ == "__main__":
    main()
