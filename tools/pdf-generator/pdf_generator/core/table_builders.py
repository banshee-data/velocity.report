#!/usr/bin/env python3
"""Table building utilities for PDF report generation.

This module handles all PyLaTeX table creation for velocity reports, including:
- Statistics tables with time-series data
- Parameter tables (key-value pairs)
- Histogram distribution tables
- Two-column layout tables using supertabular
- Header and row formatting with custom fonts

The module is designed to work with PyLaTeX but is independent of document
generation logic, making tables reusable across different document types.
"""

from typing import Any, Dict, List, Optional

try:
    from pylatex import NoEscape, Center, Document
    from pylatex.table import Tabular
    from pylatex.utils import escape_latex

    HAVE_PYLATEX = True
except Exception:  # pragma: no cover
    HAVE_PYLATEX = False
    NoEscape = str
    Center = None
    Document = None
    Tabular = None

    def escape_latex(s: str) -> str:
        return s


from pdf_generator.core.stats_utils import (
    format_time,
    format_number,
    process_histogram,
    count_in_histogram_range,
    count_histogram_ge,
)
from pdf_generator.core.data_transformers import (
    MetricsNormalizer,
    extract_start_time_from_row,
    extract_count_from_row,
)


class StatsTableBuilder:
    """Builds statistics tables with time-series metrics.

    Creates LaTeX tables showing:
    - Start time (optional)
    - Count
    - Percentiles (p50, p85, p98)
    - Maximum speed

    Supports both single-table and two-column (supertabular) layouts.
    """

    def __init__(self):
        """Initialise stats table builder."""
        if not HAVE_PYLATEX:
            raise ImportError(
                "PyLaTeX is required for table generation. "
                "Install it with: pip install pylatex"
            )

        self.normalizer = MetricsNormalizer()

    def build_header(self, include_start_time: bool = True) -> List[str]:
        """Build table header cells with proper formatting.

        Args:
            include_start_time: Whether to include start time column

        Returns:
            List of NoEscape formatted header cells
        """
        header_cells = []

        if include_start_time:
            header_cells.append(
                NoEscape(r"\multicolumn{1}{l}{\sffamily\bfseries Start Time}")
            )

        header_cells.extend(
            [
                NoEscape(r"\multicolumn{1}{r}{\sffamily\bfseries Count}"),
                NoEscape(
                    r"\multicolumn{1}{r}{\sffamily\bfseries \shortstack{p50 \\ (mph)}}"
                ),
                NoEscape(
                    r"\multicolumn{1}{r}{\sffamily\bfseries \shortstack{p85 \\ (mph)}}"
                ),
                NoEscape(
                    r"\multicolumn{1}{r}{\sffamily\bfseries \shortstack{p98 \\ (mph)}}"
                ),
                NoEscape(
                    r"\multicolumn{1}{r}{\sffamily\bfseries \shortstack{Max \\ (mph)}}"
                ),
            ]
        )

        return header_cells

    def build_rows(
        self,
        stats: List[Dict[str, Any]],
        include_start_time: bool = True,
        tz_name: Optional[str] = None,
    ) -> List[List]:
        """Build table data rows.

        Args:
            stats: List of metric dictionaries
            include_start_time: Whether to include start time column
            tz_name: Timezone name for time formatting

        Returns:
            List of row data (each row is a list of cells)
        """
        rows = []

        for row in stats:
            row_data = []

            if include_start_time:
                # Extract and format start time
                start_val = extract_start_time_from_row(row, self.normalizer)
                formatted_time = format_time(start_val, tz_name)
                row_data.append(NoEscape(escape_latex(formatted_time)))

            # Extract metrics using normalizer
            cnt = extract_count_from_row(row, self.normalizer)
            p50 = self.normalizer.get_numeric(row, "p50")
            p85 = self.normalizer.get_numeric(row, "p85")
            p98 = self.normalizer.get_numeric(row, "p98")
            max_speed = self.normalizer.get_numeric(row, "max_speed")

            row_data.extend(
                [
                    NoEscape(escape_latex(str(int(cnt)))),
                    NoEscape(escape_latex(format_number(p50))),
                    NoEscape(escape_latex(format_number(p85))),
                    NoEscape(escape_latex(format_number(p98))),
                    NoEscape(escape_latex(format_number(max_speed))),
                ]
            )

            rows.append(row_data)

        return rows

    def build(
        self,
        stats: List[Dict[str, Any]],
        tz_name: Optional[str],
        units: str,
        caption: str,
        include_start_time: bool = True,
        center_table: bool = True,
    ) -> object:
        """Build complete statistics table.

        Args:
            stats: List of metric dictionaries
            tz_name: Timezone name for time formatting
            units: Units string (e.g., "mph")
            caption: Table caption text
            include_start_time: Whether to include start time column
            center_table: Whether to wrap table in Center environment

        Returns:
            PyLaTeX table object (Tabular or Center containing Tabular)
        """
        # Build column spec with monospace font for body columns
        if include_start_time:
            body_spec = ">{\\AtkinsonMono}l" + (">{\\AtkinsonMono}r" * 5)
        else:
            body_spec = ">{\\AtkinsonMono}r" * 5

        table = Tabular(body_spec)

        # Add header row
        header_cells = self.build_header(include_start_time)
        table.add_row(header_cells)
        table.add_hline()

        # Add data rows
        data_rows = self.build_rows(stats, include_start_time, tz_name)
        for row_data in data_rows:
            table.add_row(row_data)

        table.add_hline()

        # Wrap in Center if requested
        if center_table:
            centered = Center()
            centered.append(table)
        else:
            centered = table

        # Add caption if provided
        if caption and center_table:
            centered.append(NoEscape("\\par\\vspace{2pt}"))
            centered.append(
                NoEscape(
                    f"\\noindent\\makebox[\\linewidth]{{\\textbf{{\\small {caption}}}}}"
                )
            )

        return centered

    def build_twocolumn(
        self,
        doc: Document,
        stats: List[Dict[str, Any]],
        tz_name: Optional[str],
        units: str,
        caption: str,
    ) -> None:
        """Build statistics table for two-column layout using supertabular.

        This directly appends to the document rather than returning a table,
        since supertabular needs manual LaTeX environment control.

        Args:
            doc: PyLaTeX Document to append to
            stats: List of metric dictionaries
            tz_name: Timezone name for time formatting
            units: Units string (e.g., "mph")
            caption: Table caption text
        """
        # Build all data rows
        all_data_rows = self.build_rows(stats, include_start_time=True, tz_name=tz_name)

        # Build header cells
        header_cells = self.build_header(include_start_time=True)

        # Column spec
        body_spec = ">{\\AtkinsonMono}l" + (">{\\AtkinsonMono}r" * 5)

        # Start supertabular environment manually
        doc.append(NoEscape(f"\\begin{{supertabular}}{{{body_spec}}}"))

        # Add header row
        header_line = " & ".join(str(cell) for cell in header_cells) + r"\\"
        doc.append(NoEscape(header_line))
        doc.append(NoEscape("\\hline"))

        # Add all data rows
        for row_data in all_data_rows:
            row_line = " & ".join(str(cell) for cell in row_data) + r"\\"
            doc.append(NoEscape(row_line))

        doc.append(NoEscape("\\hline"))
        doc.append(NoEscape("\\end{supertabular}"))

        # Add caption
        if caption:
            doc.append(NoEscape("\\par\\vspace{2pt}"))
            doc.append(
                NoEscape(
                    f"\\noindent\\makebox[\\linewidth]{{\\textbf{{\\small {caption}}}}}"
                )
            )

        doc.append(NoEscape("\\par\\vspace{8pt}"))


class ParameterTableBuilder:
    """Builds parameter tables (key-value pairs).

    Creates simple two-column tables with:
    - Left column: Bold keys
    - Right column: Monospace values

    Used for configuration parameters, summary statistics, etc.
    """

    def __init__(self):
        """Initialise parameter table builder."""
        if not HAVE_PYLATEX:
            raise ImportError(
                "PyLaTeX is required for table generation. "
                "Install it with: pip install pylatex"
            )

    def build(self, entries: List[Dict[str, str]]) -> Tabular:
        """Build parameter table from key-value pairs.

        Args:
            entries: List of dicts with 'key' and 'value' fields

        Returns:
            PyLaTeX Tabular object
        """
        table = Tabular("ll")

        for e in entries:
            k = e.get("key", "")
            v = e.get("value", "")
            table.add_row(
                [
                    NoEscape(r"\textbf{" + escape_latex(k) + r":}"),
                    NoEscape(r"\AtkinsonMono{" + escape_latex(str(v)) + r"}"),
                ]
            )

        return table


class ComparisonSummaryTableBuilder:
    """Builds comparison summary tables for key metrics."""

    def __init__(self):
        """Initialise comparison summary table builder."""
        if not HAVE_PYLATEX:
            raise ImportError(
                "PyLaTeX is required for table generation. "
                "Install it with: pip install pylatex"
            )

    def build(
        self,
        entries: List[Dict[str, str]],
        primary_label: str,
        compare_label: str,
    ) -> Tabular:
        """Build comparison table for summary metrics.

        Args:
            entries: List of dicts with keys: label, primary, compare, change
            primary_label: Label for primary period column
            compare_label: Label for comparison period column

        Returns:
            PyLaTeX Tabular object
        """
        table = Tabular(
            ">{\\AtkinsonMono}l>{\\AtkinsonMono}r>{\\AtkinsonMono}r>{\\AtkinsonMono}r"
        )

        header_cells = [
            NoEscape(r"\multicolumn{1}{l}{\sffamily\bfseries Metric}"),
            NoEscape(
                r"\multicolumn{1}{r}{\sffamily\bfseries "
                + escape_latex(primary_label)
                + r"}"
            ),
            NoEscape(
                r"\multicolumn{1}{r}{\sffamily\bfseries "
                + escape_latex(compare_label)
                + r"}"
            ),
            NoEscape(r"\multicolumn{1}{r}{\sffamily\bfseries Change}"),
        ]
        table.add_row(header_cells)
        table.add_hline()

        for entry in entries:
            label = entry.get("label", "")
            primary = entry.get("primary", "--")
            compare = entry.get("compare", "--")
            change = entry.get("change", "--")
            table.add_row(
                [
                    NoEscape(escape_latex(label)),
                    NoEscape(escape_latex(str(primary))),
                    NoEscape(escape_latex(str(compare))),
                    NoEscape(escape_latex(str(change))),
                ]
            )

        return table


class HistogramTableBuilder:
    """Builds histogram distribution tables.

    Creates tables showing:
    - Velocity buckets (ranges)
    - Count per bucket
    - Percentage per bucket

    Supports both fixed-width buckets and adaptive bucketing based on data.
    """

    def __init__(self):
        """Initialise histogram table builder."""
        if not HAVE_PYLATEX:
            raise ImportError(
                "PyLaTeX is required for table generation. "
                "Install it with: pip install pylatex"
            )

    def build(
        self,
        histogram: Dict[str, int],
        units: str,
        cutoff: float = 5.0,
        bucket_size: float = 5.0,
        max_bucket: Optional[float] = None,
    ) -> Center:
        """Build histogram distribution table.

        Args:
            histogram: Dict mapping bucket labels to counts
            units: Units string (e.g., "mph")
            cutoff: Lower cutoff for first bucket
            bucket_size: Width of each bucket
            max_bucket: Maximum bucket value (for open-ended last bucket)

        Returns:
            PyLaTeX Center object containing table
        """
        # Process histogram to get numeric buckets
        _proc_max = max_bucket if max_bucket is not None else 50.0
        numeric_buckets, total, _ranges = process_histogram(
            histogram, cutoff, bucket_size, _proc_max
        )

        # Derive display ranges from actual histogram keys
        ranges = self._compute_ranges(numeric_buckets, bucket_size, max_bucket, _ranges)

        # Create centered container
        centered = Center()

        # Create table with monospace body and sans-serif header
        body_table = Tabular(">{\\AtkinsonMono}l>{\\AtkinsonMono}r>{\\AtkinsonMono}r")

        # Add header
        header_cells = [
            NoEscape(r"\multicolumn{1}{l}{\sffamily\bfseries Bucket}"),
            NoEscape(r"\multicolumn{1}{r}{\sffamily\bfseries Count}"),
            NoEscape(r"\multicolumn{1}{r}{\sffamily\bfseries Percent}"),
        ]
        body_table.add_row(header_cells)
        body_table.add_hline()

        # Add data rows
        if ranges:
            self._add_histogram_rows(
                body_table,
                numeric_buckets,
                total,
                ranges,
                cutoff,
                max_bucket,
                _proc_max,
            )
        else:
            # Fallback to original behavior
            self._add_histogram_rows_fallback(
                body_table, numeric_buckets, total, cutoff, _ranges, _proc_max
            )

        body_table.add_hline()
        centered.append(body_table)

        # Add caption
        centered.append(NoEscape("\\par\\vspace{2pt}"))
        centered.append(
            NoEscape(
                "\\noindent\\makebox[\\linewidth]{\\textbf{\\small Table 1: Velocity Distribution Data}}"
            )
        )

        return centered

    def _compute_ranges(
        self,
        numeric_buckets: Dict[float, int],
        bucket_size: float,
        max_bucket: Optional[float],
        fallback_ranges: List[tuple],
    ) -> List[tuple]:
        """Compute bucket ranges from actual data.

        The last bucket will always be shown as N+ where N is the start of the
        highest bucket with actual data, not max_bucket. This matches the chart behavior.
        """
        ranges = []
        inferred_bucket = float(bucket_size)

        if not numeric_buckets:
            return fallback_ranges

        try:
            sorted_keys = sorted(numeric_buckets.keys())

            # Infer bucket size from data
            if len(sorted_keys) > 1:
                diffs = [j - i for i, j in zip(sorted_keys[:-1], sorted_keys[1:])]
                positive_diffs = [d for d in diffs if d > 0]
                if positive_diffs:
                    inferred_bucket = float(min(positive_diffs))

            min_k = float(sorted_keys[0])
            max_k = float(sorted_keys[-1])

            # Guard against pathological zero bucket
            if inferred_bucket <= 0:
                inferred_bucket = float(bucket_size) or 5.0

            # Build ranges from min_k up to max_k (highest bucket with data)
            # Ignore max_bucket parameter - we only show buckets with actual data
            s = min_k
            while s <= max_k:
                ranges.append((s, s + inferred_bucket))
                s += inferred_bucket

        except Exception:
            return fallback_ranges

        return ranges

    def _add_histogram_rows(
        self,
        table: Tabular,
        numeric_buckets: Dict[float, int],
        total: int,
        ranges: List[tuple],
        cutoff: float,
        max_bucket: Optional[float],
        proc_max: float,
    ) -> None:
        """Add histogram data rows to table.

        The last bucket is always shown as N+ where N is the start of the highest
        bucket with actual data, matching the chart behavior.
        """
        first_start = ranges[0][0]

        # Below-cutoff row (only add if there's actually data below the first bucket)
        below_count = sum(v for k, v in numeric_buckets.items() if k < first_start)
        if below_count > 0:
            pct_below = (below_count / total * 100.0) if total > 0 else 0.0
            table.add_row(
                [
                    NoEscape(escape_latex(f"<{int(first_start)}")),
                    NoEscape(escape_latex(str(int(below_count)))),
                    NoEscape(escape_latex(f"{pct_below:.1f}%")),
                ]
            )

        # Bucket rows
        for idx, (a, b) in enumerate(ranges):
            is_last = idx == len(ranges) - 1

            if is_last:
                # Last bucket: render as "N+" where N is the start of this bucket
                # This matches the chart behavior (highest bucket with data as N+)
                label = f"{int(a)}+"
                cnt = count_histogram_ge(numeric_buckets, a)
                pct = (cnt / total * 100.0) if total > 0 else 0.0
            else:
                # Regular bucket: render as "A-B"
                cnt = count_in_histogram_range(numeric_buckets, a, b)
                pct = (cnt / total * 100.0) if total > 0 else 0.0
                label = f"{int(a)}-{int(b)}"

            table.add_row(
                [
                    NoEscape(escape_latex(label)),
                    NoEscape(escape_latex(str(int(cnt)))),
                    NoEscape(escape_latex(f"{pct:.1f}%")),
                ]
            )

    def _add_histogram_rows_fallback(
        self,
        table: Tabular,
        numeric_buckets: Dict[float, int],
        total: int,
        cutoff: float,
        ranges: List[tuple],
        proc_max: float,
    ) -> None:
        """Add histogram rows using fallback behavior."""
        # Below-cutoff row (only add if there's actually data below the cutoff)
        below_cutoff = sum(v for k, v in numeric_buckets.items() if k < cutoff)
        if below_cutoff > 0:
            pct_below = (below_cutoff / total * 100.0) if total > 0 else 0.0
            table.add_row(
                [
                    NoEscape(escape_latex(f"<{int(cutoff)}")),
                    NoEscape(escape_latex(str(int(below_cutoff)))),
                    NoEscape(escape_latex(f"{pct_below:.1f}%")),
                ]
            )

        # Bucket rows
        for a, b in ranges:
            cnt = count_in_histogram_range(numeric_buckets, a, b)
            pct = (cnt / total * 100.0) if total > 0 else 0.0
            label = f"{int(a)}-{int(b)}"
            table.add_row(
                [
                    NoEscape(escape_latex(label)),
                    NoEscape(escape_latex(str(int(cnt)))),
                    NoEscape(escape_latex(f"{pct:.1f}%")),
                ]
            )

        # Final open-ended bucket
        cnt_ge = count_histogram_ge(numeric_buckets, proc_max)
        pct_ge = (cnt_ge / total * 100.0) if total > 0 else 0.0
        table.add_row(
            [
                NoEscape(escape_latex(f"{int(proc_max)}+")),
                NoEscape(escape_latex(str(int(cnt_ge)))),
                NoEscape(escape_latex(f"{pct_ge:.1f}%")),
            ]
        )


# =============================================================================
# Convenience Functions
# =============================================================================


def create_stats_table(
    stats: List[Dict[str, Any]],
    tz_name: Optional[str],
    units: str,
    caption: str,
    include_start_time: bool = True,
    center_table: bool = True,
) -> object:
    """Create statistics table (convenience wrapper)."""
    builder = StatsTableBuilder()
    return builder.build(
        stats, tz_name, units, caption, include_start_time, center_table
    )


def create_param_table(entries: List[Dict[str, str]]) -> Tabular:
    """Create parameter table (convenience wrapper)."""
    builder = ParameterTableBuilder()
    return builder.build(entries)


def create_comparison_summary_table(
    entries: List[Dict[str, str]], primary_label: str, compare_label: str
) -> Tabular:
    """Create comparison summary table (convenience wrapper)."""
    builder = ComparisonSummaryTableBuilder()
    return builder.build(entries, primary_label, compare_label)


def create_histogram_table(
    histogram: Dict[str, int],
    units: str,
    cutoff: float = 5.0,
    bucket_size: float = 5.0,
    max_bucket: Optional[float] = None,
) -> Center:
    """Create histogram table (convenience wrapper)."""
    builder = HistogramTableBuilder()
    return builder.build(histogram, units, cutoff, bucket_size, max_bucket)


def create_histogram_comparison_table(
    histogram: Dict[str, int],
    compare_histogram: Dict[str, int],
    units: str,
    primary_label: str,
    compare_label: str,
    cutoff: float = 5.0,
    bucket_size: float = 5.0,
    max_bucket: Optional[float] = None,
) -> Center:
    """Create histogram comparison table."""
    if not HAVE_PYLATEX:
        raise ImportError(
            "PyLaTeX is required for table generation. "
            "Install it with: pip install pylatex"
        )

    _proc_max = max_bucket if max_bucket is not None else 50.0
    primary_buckets, primary_total, primary_ranges = process_histogram(
        histogram, cutoff, bucket_size, _proc_max
    )
    compare_buckets, compare_total, compare_ranges = process_histogram(
        compare_histogram, cutoff, bucket_size, _proc_max
    )

    combined_buckets = primary_buckets.copy()
    for key, value in compare_buckets.items():
        combined_buckets[key] = combined_buckets.get(key, 0) + value

    ranges = HistogramTableBuilder()._compute_ranges(
        combined_buckets,
        bucket_size,
        max_bucket,
        primary_ranges or compare_ranges,
    )

    centered = Center()
    body_table = Tabular(
        ">{\\AtkinsonMono}l>{\\AtkinsonMono}r>{\\AtkinsonMono}r>{\\AtkinsonMono}r>{\\AtkinsonMono}r>{\\AtkinsonMono}r"
    )

    header_cells = [
        NoEscape(r"\multicolumn{1}{l}{\sffamily\bfseries Bucket}"),
        NoEscape(
            r"\multicolumn{1}{r}{\sffamily\bfseries "
            + escape_latex(primary_label)
            + r" Count}"
        ),
        NoEscape(
            r"\multicolumn{1}{r}{\sffamily\bfseries "
            + escape_latex(primary_label)
            + r" Percent}"
        ),
        NoEscape(
            r"\multicolumn{1}{r}{\sffamily\bfseries "
            + escape_latex(compare_label)
            + r" Count}"
        ),
        NoEscape(
            r"\multicolumn{1}{r}{\sffamily\bfseries "
            + escape_latex(compare_label)
            + r" Percent}"
        ),
        NoEscape(r"\multicolumn{1}{r}{\sffamily\bfseries Change}"),
    ]
    body_table.add_row(header_cells)
    body_table.add_hline()

    def format_change(primary_value: int, compare_value: int) -> str:
        if primary_value == 0:
            return "--"
        change_pct = (compare_value - primary_value) / primary_value * 100.0
        return f"{change_pct:+.1f}%"

    if ranges:
        first_start = ranges[0][0]
        primary_below = sum(v for k, v in primary_buckets.items() if k < first_start)
        compare_below = sum(v for k, v in compare_buckets.items() if k < first_start)
        if primary_below > 0 or compare_below > 0:
            primary_pct = (
                primary_below / primary_total * 100.0 if primary_total > 0 else 0.0
            )
            compare_pct = (
                compare_below / compare_total * 100.0 if compare_total > 0 else 0.0
            )
            body_table.add_row(
                [
                    NoEscape(escape_latex(f"<{int(first_start)}")),
                    NoEscape(escape_latex(str(int(primary_below)))),
                    NoEscape(escape_latex(f"{primary_pct:.1f}%")),
                    NoEscape(escape_latex(str(int(compare_below)))),
                    NoEscape(escape_latex(f"{compare_pct:.1f}%")),
                    NoEscape(escape_latex(format_change(primary_below, compare_below))),
                ]
            )

        for idx, (a, b) in enumerate(ranges):
            is_last = idx == len(ranges) - 1
            if is_last:
                label = f"{int(a)}+"
                primary_count = count_histogram_ge(primary_buckets, a)
                compare_count = count_histogram_ge(compare_buckets, a)
            else:
                label = f"{int(a)}-{int(b)}"
                primary_count = count_in_histogram_range(primary_buckets, a, b)
                compare_count = count_in_histogram_range(compare_buckets, a, b)

            primary_pct = (
                primary_count / primary_total * 100.0 if primary_total > 0 else 0.0
            )
            compare_pct = (
                compare_count / compare_total * 100.0 if compare_total > 0 else 0.0
            )
            body_table.add_row(
                [
                    NoEscape(escape_latex(label)),
                    NoEscape(escape_latex(str(int(primary_count)))),
                    NoEscape(escape_latex(f"{primary_pct:.1f}%")),
                    NoEscape(escape_latex(str(int(compare_count)))),
                    NoEscape(escape_latex(f"{compare_pct:.1f}%")),
                    NoEscape(
                        escape_latex(format_change(primary_count, compare_count))
                    ),
                ]
            )

    body_table.add_hline()
    centered.append(body_table)
    centered.append(NoEscape("\\par\\vspace{2pt}"))
    centered.append(
        NoEscape(
            "\\noindent\\makebox[\\linewidth]{\\textbf{\\small Table 1: Velocity Distribution Comparison ("
            + escape_latex(units)
            + ")}}"
        )
    )
    return centered


def create_twocolumn_stats_table(
    doc: Document,
    stats: List[Dict[str, Any]],
    tz_name: Optional[str],
    units: str,
    caption: str,
) -> None:
    """Create two-column statistics table (convenience wrapper)."""
    builder = StatsTableBuilder()
    builder.build_twocolumn(doc, stats, tz_name, units, caption)
