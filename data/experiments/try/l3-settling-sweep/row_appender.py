"""CSV writer that reopens its file for every row.

The sweep drivers used to hold one append handle for the whole run. On
2026-09-18 a git pre-commit hook (mixed-line-ending) rewrote results.csv while
l3_extended_sweep_b was running; the driver kept appending to the unlinked old
file, so 344 completed runs never reached results.csv (their raw reports
survived). Opening the file per row means a replaced file is simply picked up
on the next row. The header is written only when the file is missing or empty,
so a replaced file that still has its header is never given a second one.
"""

import csv
from pathlib import Path


class RowAppender:
    def __init__(self, path, fieldnames):
        self.path = Path(path)
        self.fieldnames = fieldnames

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False

    def writeheader(self):
        if not self.path.exists() or self.path.stat().st_size == 0:
            self._append(None)

    def writerow(self, row):
        self.writeheader()
        self._append(row)

    def _append(self, row):
        with self.path.open("a", newline="") as f:
            writer = csv.DictWriter(f, fieldnames=self.fieldnames)
            if row is None:
                writer.writeheader()
            else:
                writer.writerow(row)
