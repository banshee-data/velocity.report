#!/usr/bin/env python3
"""age-color.py — pipe stdin through this to colour lines by recency.

Each line is timestamped on arrival. A curses viewport repaints at 4 Hz,
shifting each line's colour as it ages through configurable thresholds.

Usage:
    command | python3 scripts/age-color.py [--thresholds 10,30,60,300,900]

Thresholds are in seconds. The default palette ages:
    < T0  bright white      (fresh)
    < T1  normal white
    < T2  cyan
    < T3  yellow
    < T4  red
    >=T4  dim red           (stale)

Press q or Ctrl-C to exit.
"""

import argparse
import curses
import sys
import threading
import time
from collections import deque
from dataclasses import dataclass, field


@dataclass
class Line:
    text: str
    ts: float = field(default_factory=time.monotonic)


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Colour stdin lines by age",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    p.add_argument(
        "--thresholds",
        default="10,30,60,300,900",
        help="Comma-separated age thresholds in seconds (default: 10,30,60,300,900)",
    )
    p.add_argument(
        "--buffer",
        type=int,
        default=2000,
        help="Max lines to keep in memory (default: 2000)",
    )
    return p.parse_args()


# Palette ordered from freshest to stalest.
# Tuple: (curses colour constant, background (-1 = default), extra attribute)
_PALETTE_SPEC = [
    ("WHITE", curses.A_BOLD),  # bracket 0: < T0
    ("WHITE", 0),  # bracket 1: < T1
    ("CYAN", 0),  # bracket 2: < T2
    ("YELLOW", 0),  # bracket 3: < T3
    ("RED", 0),  # bracket 4: < T4
    ("RED", curses.A_DIM),  # bracket 5: >= T4
]


def _init_pairs() -> list[tuple[int, int]]:
    """Initialise curses colour pairs. Returns list of (pair_number, extra_attr)."""
    pairs = []
    for i, (colour_name, extra) in enumerate(_PALETTE_SPEC):
        fg = getattr(curses, f"COLOR_{colour_name}", curses.COLOR_WHITE)
        curses.init_pair(i + 1, fg, -1)
        pairs.append((i + 1, extra))
    return pairs


def _bracket(age: float, thresholds: list[float]) -> int:
    for i, t in enumerate(thresholds):
        if age < t:
            return i
    return len(thresholds)  # one bracket beyond the last threshold


def _run(
    stdscr: "curses._CursesWindow",
    lines: deque,
    lock: threading.Lock,
    thresholds: list[float],
) -> None:
    curses.use_default_colors()
    curses.start_color()
    pairs = _init_pairs()

    stdscr.nodelay(True)
    curses.curs_set(0)

    while True:
        now = time.monotonic()
        h, w = stdscr.getmaxyx()

        with lock:
            # Show the most recent lines that fit on screen
            visible = list(lines)[-(h):]

        stdscr.erase()

        for row, line in enumerate(visible):
            if row >= h:
                break
            age = now - line.ts
            b = _bracket(age, thresholds)
            pair_num, extra_attr = pairs[b]
            attr = curses.color_pair(pair_num) | extra_attr

            text = line.text.rstrip("\n\r")
            # Truncate to avoid curses overflow on the last column
            if len(text) >= w:
                text = text[: w - 1]

            try:
                stdscr.addstr(row, 0, text, attr)
            except curses.error:
                pass

        stdscr.refresh()

        # 3 = Ctrl-C, ord('q') / ord('Q') = quit
        key = stdscr.getch()
        if key in (ord("q"), ord("Q"), 3):
            return

        time.sleep(0.25)  # 4 Hz repaint


def _reader(lines: deque, lock: threading.Lock, max_lines: int) -> None:
    try:
        for raw in sys.stdin:
            with lock:
                lines.append(Line(raw))
                while len(lines) > max_lines:
                    lines.popleft()
    except (KeyboardInterrupt, EOFError):
        pass


def main() -> None:
    args = parse_args()
    thresholds = [float(x.strip()) for x in args.thresholds.split(",")]

    lines: deque[Line] = deque()
    lock = threading.Lock()

    reader = threading.Thread(
        target=_reader, args=(lines, lock, args.buffer), daemon=True
    )
    reader.start()

    try:
        curses.wrapper(_run, lines, lock, thresholds)
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
