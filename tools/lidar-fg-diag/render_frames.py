#!/usr/bin/env python3
"""Draw foreground classification from `lidar-fg-diag` frame dumps.

`lidar-fg-diag vrlog` and `lidar-fg-diag replay` write each frame as a packed
record file with a JSON sidecar. This draws them, so foreground extraction can
be looked at rather than inferred from counts:

    render_frames.py top DUMP[,DUMP...] FRAME OUT.png [--limits -60,60,-60,60]
    render_frames.py side BEFORE AFTER OUT.png [--count 10]
    render_frames.py view BEFORE AFTER OUT.png --frames 100,110,120

`top` draws frames from above, one panel per dump. With an oracle label in the
dump, as `lidar-fg-diag replay -oracle` writes, colour says whether foreground
landed on a moving return or a persistent surface. `side` draws the moving
tracks of AFTER from the side, in both dumps, in the track's own frame. `view`
draws the same frames of two dumps from one camera.
"""

import argparse
import glob
import json
import os
import sys

import matplotlib

matplotlib.use("Agg")

import matplotlib.pyplot as plt  # noqa: E402
import numpy as np  # noqa: E402
from matplotlib.patches import Rectangle  # noqa: E402
from matplotlib.transforms import Affine2D  # noqa: E402

RECORD = np.dtype(
    [
        ("x", "<f4"),
        ("y", "<f4"),
        ("z", "<f4"),
        ("range", "<f4"),
        ("az", "<f4"),
        ("avg", "<f4"),
        ("spread", "<f4"),
        ("win", "<f4"),
        ("base", "<f4"),
        ("locked", "<f4"),
        ("seen", "<u4"),
        ("ring", "u1"),
        ("intensity", "u1"),
        ("pub", "u1"),
        ("l3", "u1"),
        ("code", "u1"),
        ("label", "u1"),
        ("_", "V2"),
    ]
)
STATIC, TRANSIENT = 1, 2
BACKGROUND = "#14161f"
GREY = "#6a7080"
GREEN = "#30ff60"


def load(directory, index):
    base = os.path.join(directory, f"frame_{index:06d}")
    with open(base + ".json") as f:
        meta = json.load(f)
    return np.fromfile(base + ".bin", dtype=RECORD), meta


def indices(directory):
    names = glob.glob(os.path.join(directory, "frame_*.bin"))
    return sorted(int(os.path.basename(n)[6:12]) for n in names)


def plain_axes(ax):
    ax.set_facecolor(BACKGROUND)
    ax.tick_params(labelsize=6, colors="#888888")


def top(args):
    dirs = args.dumps.split(",")
    limits = [float(v) for v in args.limits.split(",")]
    fig, axes = plt.subplots(
        1,
        len(dirs),
        figsize=(9 * len(dirs), 9),
        facecolor=BACKGROUND,
        dpi=100,
        squeeze=False,
    )
    for ax, directory in zip(axes[0], dirs):
        points, meta = load(directory, args.frame)
        fg, label = points["pub"] > 0, points["label"]
        plain_axes(ax)
        rest = ~fg & (label != TRANSIENT)
        ax.scatter(
            points["x"][rest], points["y"][rest], s=0.5, c="#3f4656", linewidths=0
        )
        handles = []
        for mask, colour, name in (
            (fg & (label == TRANSIENT), GREEN, "foreground on a moving return"),
            (fg & (label == STATIC), "#ff5030", "foreground on a persistent surface"),
            (fg & (label == 0), "#ffe020", "foreground, unlabelled"),
            (~fg & (label == TRANSIENT), "#20c0ff", "moving return missed"),
        ):
            ax.scatter(
                points["x"][mask], points["y"][mask], s=3, c=colour, linewidths=0
            )
            handles.append(
                plt.Line2D(
                    [],
                    [],
                    marker="o",
                    ls="",
                    color=colour,
                    label=f"{name}: {int(mask.sum())}",
                )
            )
        for track in meta.get("tracks") or []:
            transform = Affine2D().rotate_around(track["x"], track["y"], track["yaw"])
            corner = (track["x"] - track["length"] / 2, track["y"] - track["width"] / 2)
            ax.add_patch(
                Rectangle(
                    corner,
                    track["length"],
                    track["width"],
                    fill=False,
                    ec="#ff50ff",
                    lw=0.6,
                    transform=transform + ax.transData,
                )
            )
        ax.legend(
            handles=handles,
            fontsize=7,
            loc="lower right",
            facecolor="#222222",
            labelcolor="w",
        )
        ax.set_xlim(*limits[:2])
        ax.set_ylim(*limits[2:])
        ax.set_aspect("equal")
        title = f"{os.path.basename(directory)} t={meta['seconds']:.1f}s tracks={len(meta.get('tracks') or [])}"
        ax.set_title(title, color="w", fontsize=9)
    plt.tight_layout()
    fig.savefig(args.out)


def side(args):
    frames = sorted(set(indices(args.before)) & set(indices(args.after)))
    chosen = []
    for index in frames:
        _, meta = load(args.after, index)
        for track in meta.get("tracks") or []:
            speed = np.hypot(track["vx"], track["vy"])
            if speed > 2 and track["misses"] == 0 and track["length"] > 2.5:
                chosen.append((index, track, speed))
    chosen = chosen[:: max(1, len(chosen) // args.count)][: args.count]
    if not chosen:
        sys.exit("no moving track in the frames both dumps hold")
    fig, axes = plt.subplots(
        len(chosen),
        2,
        figsize=(15, 2.3 * len(chosen)),
        facecolor=BACKGROUND,
        dpi=90,
        squeeze=False,
    )
    for row, (index, track, speed) in zip(axes, chosen):
        heading = np.arctan2(track["vy"], track["vx"])
        cos, sin = np.cos(heading), np.sin(heading)
        for ax, directory in zip(row, (args.before, args.after)):
            points, meta = load(directory, index)
            dx, dy = points["x"] - track["x"], points["y"] - track["y"]
            along, across = dx * cos + dy * sin, -dx * sin + dy * cos
            near = (abs(along) < track["length"] / 2 + 3) & (
                abs(across) < track["width"] / 2 + 2.5
            )
            fg = points["pub"][near] > 0
            plain_axes(ax)
            ax.scatter(
                along[near][~fg], points["z"][near][~fg], s=4, c=GREY, linewidths=0
            )
            ax.scatter(
                along[near][fg], points["z"][near][fg], s=5, c=GREEN, linewidths=0
            )
            ax.set_aspect("equal")
            title = (
                f"{os.path.basename(directory)} t={meta['seconds']:.1f}s "
                f"{speed:.1f} m/s, foreground {int(fg.sum())} of {int(near.sum())}"
            )
            ax.set_title(title, color="w", fontsize=7)
    plt.tight_layout()
    fig.savefig(args.out)


def project(points, eye, target, focal=1.1):
    eye, target = np.array(eye, float), np.array(target, float)
    forward = target - eye
    forward /= np.linalg.norm(forward)
    right = np.cross(forward, [0, 0, 1.0])
    right /= np.linalg.norm(right)
    up = np.cross(right, forward)
    relative = np.stack(
        [points["x"] - eye[0], points["y"] - eye[1], points["z"] - eye[2]], 1
    )
    depth = relative @ forward
    front = depth > 1
    return (
        (relative @ right)[front] / depth[front] * focal,
        (relative @ up)[front] / depth[front] * focal,
        front,
    )


def view(args):
    frames = [int(v) for v in args.frames.split(",")]
    eye = [float(v) for v in args.eye.split(",")]
    target = [float(v) for v in args.target.split(",")]
    fig, axes = plt.subplots(
        len(frames),
        2,
        figsize=(18, 5.6 * len(frames)),
        facecolor=BACKGROUND,
        dpi=90,
        squeeze=False,
    )
    for row, index in zip(axes, frames):
        for ax, directory in zip(row, (args.before, args.after)):
            points, meta = load(directory, index)
            x, y, front = project(points, eye, target)
            fg = points["pub"][front] > 0
            ax.set_facecolor("#1a1b2a")
            ax.scatter(x[~fg], y[~fg], s=0.35, c="#8a8fa3", linewidths=0)
            ax.scatter(x[fg], y[fg], s=1.6, c="#35d24a", linewidths=0)
            ax.set_xlim(-1.1, 1.1)
            ax.set_ylim(-0.62, 0.62)
            ax.axis("off")
            ax.set_title(
                f"{os.path.basename(directory)} t={meta['seconds']:.1f}s, foreground {meta['fg']}",
                color="w",
                fontsize=10,
            )
    plt.tight_layout()
    fig.savefig(args.out)


def main():
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawTextHelpFormatter
    )
    commands = parser.add_subparsers(dest="command", required=True)

    p = commands.add_parser("top")
    p.add_argument("dumps")
    p.add_argument("frame", type=int)
    p.add_argument("out")
    p.add_argument("--limits", default="-60,60,-60,60")
    p.set_defaults(run=top)

    p = commands.add_parser("side")
    p.add_argument("before")
    p.add_argument("after")
    p.add_argument("out")
    p.add_argument("--count", type=int, default=10)
    p.set_defaults(run=side)

    p = commands.add_parser("view")
    p.add_argument("before")
    p.add_argument("after")
    p.add_argument("out")
    p.add_argument("--frames", required=True)
    p.add_argument("--eye", default="-40,-55,28")
    p.add_argument("--target", default="5,-25,-2")
    p.set_defaults(run=view)

    args = parser.parse_args()
    args.run(args)


if __name__ == "__main__":
    main()
