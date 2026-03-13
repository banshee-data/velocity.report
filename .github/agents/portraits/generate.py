#!/usr/bin/env python3
"""Generate agent portrait thumbnails from source images.

Downloads each source image, crops to the defined region, and resizes
to 120x120 JPEG.  Run from anywhere — paths are relative to this script.

    .venv/bin/python .github/agents/portraits/generate.py          # all
    .venv/bin/python .github/agents/portraits/generate.py grace    # one
    .venv/bin/python .github/agents/portraits/generate.py euler ruth  # several

Requires: Pillow (pip install Pillow)
"""

import hashlib
import os
import sys
import time
import urllib.request
from io import BytesIO

from PIL import Image

DIR = os.path.dirname(os.path.abspath(__file__))
SIZE = 120

# ── Source images ──────────────────────────────────────────────────────
#
# Wikimedia Commons thumbnails are built from the filename using an MD5
# hash path.  The helper below constructs the URL automatically.
#
# Each entry: (name, source_url, crop_box, thumb_width)
#   crop_box is (left, top, right, bottom) on the thumbnail at the
#   given thumb_width.

PORTRAITS = [
    {
        "name": "euler",
        "file": "Leonhard_Euler.jpg",
        "source": "wikimedia",
        "width": 1920,
        "crop": (336, 240, 1584, 1488),
        "licence": "Public domain",
    },
    {
        "name": "grace",
        "file": "Commodore_Grace_M._Hopper,_USN_(covered).jpg",
        "source": "wikimedia",
        "width": 1920,
        "crop": (816, 384, 1680, 1248),
        "licence": "Public domain (US government work)",
    },
    {
        "name": "appius",
        "file": "Musei_vaticani,_braccio_chiaramonti,_busto_02.JPG",
        "source": "wikimedia",
        "width": 1920,
        "crop": (230, 38, 1651, 1574),
        "licence": "Public domain",
    },
    {
        "name": "malory",
        "file": None,
        "source": "https://live.staticflickr.com/3874/14569194548_55a59de923_b.jpg",
        "width": None,
        "crop": (260, 256, 518, 512),
        "licence": "CC BY-NC-ND 2.0",
    },
    {
        "name": "flo",
        "file": "Florence_Nightingale_(H_Hering_NPG_x82368).jpg",
        "source": "wikimedia",
        "width": 1920,
        "crop": (288, 144, 1632, 1536),
        "licence": "Public domain",
    },
    {
        "name": "terry",
        "file": "10.12.12TerryPratchettByLuigiNovi1.jpg",
        "source": "wikimedia",
        "width": 1920,
        "crop": (432, 192, 1488, 1248),
        "licence": "CC BY 3.0",
    },
    {
        "name": "ruth",
        "file": "Ruth_Bader_Ginsburg_2016_portrait.jpg",
        "source": "wikimedia",
        "width": 1920,
        "crop": (576, 158, 1464, 1046),
        "licence": "Public domain (US government work)",
    },
]


# ── Helpers ────────────────────────────────────────────────────────────


def wikimedia_thumb_url(filename: str, width: int) -> str:
    """Build a Wikimedia Commons thumbnail URL from the filename."""
    md5 = hashlib.md5(filename.encode()).hexdigest()
    encoded = (
        filename.replace(" ", "_")
        .replace(",", "%2C")
        .replace("(", "%28")
        .replace(")", "%29")
    )
    return (
        f"https://upload.wikimedia.org/wikipedia/commons/thumb/"
        f"{md5[0]}/{md5[:2]}/{encoded}/{width}px-{encoded}"
    )


def download(url: str) -> Image.Image:
    """Download an image and return a PIL Image."""
    req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
    data = urllib.request.urlopen(req, timeout=30).read()  # noqa: S310
    return Image.open(BytesIO(data))


def get_url(portrait: dict) -> str:
    """Resolve the download URL for a portrait entry."""
    if portrait["source"] == "wikimedia":
        return wikimedia_thumb_url(portrait["file"], portrait["width"])
    return portrait["source"]


# ── Main ───────────────────────────────────────────────────────────────

NAMES = [p["name"] for p in PORTRAITS]


def main() -> None:
    targets = sys.argv[1:]
    if targets:
        unknown = [t for t in targets if t not in NAMES]
        if unknown:
            print(f"Unknown: {', '.join(unknown)}.  Valid: {', '.join(NAMES)}")
            sys.exit(2)
    else:
        targets = NAMES

    ok, fail = 0, 0

    for p in PORTRAITS:
        name = p["name"]
        if name not in targets:
            continue

        url = get_url(p)
        out = os.path.join(DIR, f"{name}.jpg")

        print(f"{name}: ", end="", flush=True)
        try:
            img = download(url)
        except Exception as exc:
            print(f"FAILED ({exc})")
            fail += 1
            continue

        cropped = img.crop(p["crop"])
        cropped = cropped.resize((SIZE, SIZE), Image.LANCZOS)
        if cropped.mode == "RGBA":
            cropped = cropped.convert("RGB")
        cropped.save(out, "JPEG", quality=90)

        box = p["crop"]
        print(f"{img.size[0]}x{img.size[1]} -> crop {box} -> {SIZE}x{SIZE}")
        ok += 1

        # Polite delay between Wikimedia requests
        if p["source"] == "wikimedia" and ok < len(targets):
            time.sleep(2)

    print(f"\nDone: {ok} succeeded, {fail} failed.")
    if fail:
        sys.exit(1)


if __name__ == "__main__":
    main()
