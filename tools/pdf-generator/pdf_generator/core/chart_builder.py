#!/usr/bin/env python3
"""Chart building utilities for velocity statistics visualization.

This module handles all matplotlib chart creation for velocity reports, including:
- Time-series plots with multiple percentile lines (P50/P85/P98/Max)
- Dual-axis charts (speed vs. count)
- Histogram generation
- Broken line handling for missing data
- Responsive bar width calculation

The module is designed to be independent of data sources and PDF generation,
making it reusable for other visualization contexts.
"""

from typing import Any, Dict, List, Optional, Tuple
from datetime import datetime
from zoneinfo import ZoneInfo

import numpy as np

try:
    import matplotlib
    import matplotlib.dates as mdates
    import matplotlib.pyplot as plt
    from matplotlib.patches import Patch

    HAVE_MATPLOTLIB = True
except Exception:  # pragma: no cover
    HAVE_MATPLOTLIB = False
    matplotlib = None
    mdates = None
    plt = None
    Patch = None

from pdf_generator.core.data_transformers import (
    MetricsNormalizer,
    extract_start_time_from_row,
    extract_count_from_row,
)
from pdf_generator.core.date_parser import parse_server_time
from pdf_generator.core.config_manager import (
    _colors_to_dict,
    _fonts_to_dict,
    _layout_to_dict,
    _debug_to_dict,
    DEFAULT_COLOR_CONFIG,
    DEFAULT_FONT_CONFIG,
    DEFAULT_LAYOUT_CONFIG,
    DEFAULT_DEBUG_CONFIG,
)


class TimeSeriesChartBuilder:
    """Builds time-series charts with percentile lines and count bars.

    Creates dual-axis charts showing:
    - Left axis: Speed percentiles (P50, P85, P98, Max) as line plots
    - Right axis: Sample counts as bar charts
    - Background highlighting for low-sample periods
    - Broken lines for missing/invalid data
    """

    def __init__(
        self,
        colors: Optional[Dict[str, str]] = None,
        fonts: Optional[Dict[str, int]] = None,
        layout: Optional[Dict[str, Any]] = None,
        debug: Optional[Dict[str, bool]] = None,
    ):
        """Initialise chart builder with styling configuration.

        Args:
            colors: Color palette dict (defaults to DEFAULT_COLORS)
            fonts: Font size dict (defaults to DEFAULT_FONTS)
            layout: Layout config dict (defaults to DEFAULT_LAYOUT)
            debug: Debug config dict (defaults to DEFAULT_DEBUG)
        """
        if not HAVE_MATPLOTLIB:
            raise ImportError(
                "matplotlib is required for chart generation. "
                "Install it with: pip install matplotlib"
            )

        self.colors = colors or _colors_to_dict(DEFAULT_COLOR_CONFIG)
        self.fonts = fonts or _fonts_to_dict(DEFAULT_FONT_CONFIG)
        self.layout = layout or _layout_to_dict(DEFAULT_LAYOUT_CONFIG)
        self.debug = debug or _debug_to_dict(DEFAULT_DEBUG_CONFIG)
        self.normalizer = MetricsNormalizer()

    def build(
        self,
        stats: List[Dict[str, Any]],
        title: str,
        units: str,
        tz_name: Optional[str] = None,
    ) -> object:
        """Build complete time-series chart from statistics data.

        Args:
            stats: List of metric dictionaries with time-series data
            title: Chart title
            units: Units string for Y-axis label (e.g., "mph")
            tz_name: Timezone name for X-axis (e.g., "US/Pacific")

        Returns:
            matplotlib Figure object
        """
        # Create figure with configured size
        fig, ax = plt.subplots(figsize=self.layout["chart_figsize"])

        # Handle empty data
        if not stats:
            ax.text(0.5, 0.5, "No data", ha="center", va="center")
            ax.set_title(title)
            return fig

        # Extract and convert data
        times, p50, p85, p98, mx, counts = self._extract_data(stats, tz_name)

        # Convert to masked arrays and handle missing data
        p50_a, p85_a, p98_a, mx_a = self._create_masked_arrays(
            p50, p85, p98, mx, counts
        )

        # Convert to float arrays for plotting
        p50_f = np.array(p50_a.filled(np.nan), dtype=float)
        p85_f = np.array(p85_a.filled(np.nan), dtype=float)
        p98_f = np.array(p98_a.filled(np.nan), dtype=float)
        mx_f = np.array(mx_a.filled(np.nan), dtype=float)

        # Debug output if enabled
        self._debug_output(times, counts, p50_f)

        # Plot percentile lines with broken line handling
        self._plot_percentile_lines(ax, times, p50_f, p85_f, p98_f, mx_f)

        # Configure left axis (speed)
        self._configure_speed_axis(ax, units)

        # Create right axis for counts and plot bars
        ax2 = ax.twinx()
        legend_data = self._plot_count_bars(ax2, times, counts)

        # Configure right axis (counts)
        self._configure_count_axis(ax2)

        # Merge and position legend
        self._create_legend(fig, ax, ax2, legend_data)

        # Format time axis
        self._format_time_axis(fig, ax, tz_name)

        # Apply final styling
        self._apply_final_styling(fig, ax, ax2)

        return fig

    def _extract_data(
        self,
        stats: List[Dict[str, Any]],
        tz_name: Optional[str],
    ) -> Tuple[
        List[datetime], List[float], List[float], List[float], List[float], List[int]
    ]:
        """Extract and convert data from stats dictionaries."""
        times = []
        p50 = []
        p85 = []
        p98 = []
        mx = []
        counts = []

        for row in stats:
            # Extract and parse start time
            st = extract_start_time_from_row(row, self.normalizer)
            try:
                t = parse_server_time(st)
            except Exception:
                continue  # Skip rows with bad time

            # Convert timezone if requested
            if tz_name:
                t = self._convert_timezone(t, tz_name)

            times.append(t)

            # Extract metrics using normalizer
            p50.append(self.normalizer.get_numeric(row, "p50"))
            p85.append(self.normalizer.get_numeric(row, "p85"))
            p98.append(self.normalizer.get_numeric(row, "p98"))
            mx.append(self.normalizer.get_numeric(row, "max_speed"))
            counts.append(extract_count_from_row(row, self.normalizer))

        return times, p50, p85, p98, mx, counts

    def _convert_timezone(self, t: datetime, tz_name: str) -> datetime:
        """Convert datetime to specified timezone."""
        try:
            tzobj = ZoneInfo(tz_name)
        except Exception:
            return t

        try:
            if getattr(t, "tzinfo", None) is not None:
                return t.astimezone(tzobj)
            else:
                # Naive datetime -> assume UTC then convert
                from datetime import timezone as _dt_timezone

                return t.replace(tzinfo=_dt_timezone.utc).astimezone(tzobj)
        except Exception:
            return t

    def _create_masked_arrays(
        self,
        p50: List[float],
        p85: List[float],
        p98: List[float],
        mx: List[float],
        counts: List[int],
    ) -> Tuple[
        np.ma.MaskedArray, np.ma.MaskedArray, np.ma.MaskedArray, np.ma.MaskedArray
    ]:
        """Create masked arrays handling invalid and low-count data."""
        # Convert to masked arrays
        p50_a = np.ma.masked_invalid(np.array(p50, dtype=float))
        p85_a = np.ma.masked_invalid(np.array(p85, dtype=float))
        p98_a = np.ma.masked_invalid(np.array(p98, dtype=float))
        mx_a = np.ma.masked_invalid(np.array(mx, dtype=float))

        # Mask low-count periods
        try:
            thresh = int(self.layout["count_missing_threshold"])
            zero_mask = np.array(counts) < thresh

            # Combine masks
            p50_a = np.ma.array(p50_a, mask=(np.ma.getmaskarray(p50_a) | zero_mask))
            p85_a = np.ma.array(p85_a, mask=(np.ma.getmaskarray(p85_a) | zero_mask))
            p98_a = np.ma.array(p98_a, mask=(np.ma.getmaskarray(p98_a) | zero_mask))
            mx_a = np.ma.array(mx_a, mask=(np.ma.getmaskarray(mx_a) | zero_mask))

            # Debug output
            if self.debug["plot_debug"]:
                import sys

                print(f"DEBUG_PLOT: missing_threshold={thresh}", file=sys.stderr)
                print(
                    f"DEBUG_PLOT: zero_mask_count={int(zero_mask.sum())}",
                    file=sys.stderr,
                )
        except Exception:
            pass

        return p50_a, p85_a, p98_a, mx_a

    def _debug_output(
        self, times: List[datetime], counts: List[int], p50_f: np.ndarray
    ) -> None:
        """Print debug information if enabled."""
        try:
            if self.debug["plot_debug"]:
                import sys

                print(f"DEBUG_PLOT: times(len)={len(times)}", file=sys.stderr)
                print(f"DEBUG_PLOT: counts={counts!r}", file=sys.stderr)
                print(f"DEBUG_PLOT: p50_f={p50_f.tolist()!r}", file=sys.stderr)
        except Exception:
            pass

    def _plot_percentile_lines(
        self,
        ax,
        times: List[datetime],
        p50_f: np.ndarray,
        p85_f: np.ndarray,
        p98_f: np.ndarray,
        mx_f: np.ndarray,
    ) -> None:
        """Plot percentile lines with broken line handling."""
        plot_broken = self._create_broken_line_plotter(times)

        # Plot each percentile line
        plot_broken(
            ax,
            times,
            p50_f,
            label="p50",
            marker="^",
            color=self.colors["p50"],
            linewidth=self.layout["line_width"],
            markersize=self.layout["marker_size"],
            markeredgewidth=self.layout["marker_edge_width"],
        )
        plot_broken(
            ax,
            times,
            p85_f,
            label="p85",
            marker="s",
            color=self.colors["p85"],
            linewidth=self.layout["line_width"],
            markersize=self.layout["marker_size"],
            markeredgewidth=self.layout["marker_edge_width"],
        )
        plot_broken(
            ax,
            times,
            p98_f,
            label="p98",
            marker="o",
            color=self.colors["p98"],
            linewidth=self.layout["line_width"],
            markersize=self.layout["marker_size"],
            markeredgewidth=self.layout["marker_edge_width"],
        )
        plot_broken(
            ax,
            times,
            mx_f,
            label="Max",
            marker="x",
            linestyle="--",
            color=self.colors["max"],
            linewidth=self.layout["line_width"],
            markersize=self.layout["marker_size"],
            markeredgewidth=self.layout["marker_edge_width"],
        )

    def _create_broken_line_plotter(self, times: List[datetime]):
        """Create a function to plot lines broken at gaps/NaN values."""

        def _plot_broken(ax, x, y_filled, label=None, **kwargs):
            """Plot series breaking line at masked/NaN values."""
            x_arr = np.array(x, dtype=object)
            valid_mask = np.isfinite(y_filled)

            if not np.any(valid_mask):
                return

            # Compute gap threshold
            gap_threshold = self._compute_gap_threshold(x_arr)

            # Build runs of contiguous valid points
            runs = self._build_runs(x_arr, valid_mask, gap_threshold)

            # Plot each run separately
            plotted_any = False
            for s, e in runs:
                x_seg = x_arr[s : e + 1]
                y_seg = y_filled[s : e + 1]
                seg_label = label if not plotted_any else None
                ax.plot(x_seg, y_seg, label=seg_label, **kwargs)
                plotted_any = True

        return _plot_broken

    def _compute_gap_threshold(self, x_arr: np.ndarray) -> Optional[float]:
        """Compute time gap threshold for breaking lines."""
        try:
            deltas = []
            for a, b in zip(x_arr[:-1], x_arr[1:]):
                if a is None or b is None:
                    continue
                try:
                    delta_s = (b - a).total_seconds()
                except Exception:
                    delta_s = float(
                        (np.datetime64(b) - np.datetime64(a)) / np.timedelta64(1, "s")
                    )
                if delta_s > 0:
                    deltas.append(delta_s)

            if deltas:
                base_delta = float(np.min(deltas))
                gap_threshold = base_delta * 2

                # Debug output
                if self.debug["plot_debug"]:
                    import sys

                    print(
                        f"DEBUG_PLOT: base_delta={base_delta} gap_threshold={gap_threshold}",
                        file=sys.stderr,
                    )
                return gap_threshold
        except Exception:
            pass

        return None

    def _build_runs(
        self,
        x_arr: np.ndarray,
        valid_mask: np.ndarray,
        gap_threshold: Optional[float],
    ) -> List[Tuple[int, int]]:
        """Build list of (start, end) index tuples for contiguous valid runs."""
        idx = np.where(valid_mask)[0]
        if len(idx) == 0:
            return []

        runs = []
        start = idx[0]
        prev = start

        for i in idx[1:]:
            split_on_gap = False

            # Check for large time gap
            if gap_threshold is not None and i > 0 and prev < len(x_arr):
                try:
                    try:
                        gap_s = (x_arr[i] - x_arr[prev]).total_seconds()
                    except Exception:
                        gap_s = float(
                            (np.datetime64(x_arr[i]) - np.datetime64(x_arr[prev]))
                            / np.timedelta64(1, "s")
                        )
                    if gap_s > gap_threshold:
                        split_on_gap = True
                except Exception:
                    pass

            # Start new run if non-contiguous or large gap
            if i != prev + 1 or split_on_gap:
                runs.append((start, prev))
                start = i
            prev = i

        runs.append((start, prev))
        return runs

    def _configure_speed_axis(self, ax, units: str) -> None:
        """Configure left Y-axis (speed)."""
        ax.set_ylabel(f"Velocity ({units})", fontsize=self.fonts["chart_axis_label"])
        ax.tick_params(
            axis="both", which="major", labelsize=self.fonts["chart_axis_tick"]
        )

        # Ensure axis starts at zero
        try:
            ax.set_ylim(bottom=0)
        except Exception:
            try:
                ymin, ymax = ax.get_ylim()
                ax.set_ylim(0, ymax)
            except Exception:
                pass

    def _plot_count_bars(
        self,
        ax2,
        times: List[datetime],
        counts: List[int],
    ) -> Optional[Tuple[str, str, float]]:
        """Plot count bars with low-sample highlighting.

        Returns:
            Tuple of (label, color, alpha) for low-sample legend entry, or None
        """
        # Compute max count and low-sample mask
        try:
            max_count = max(int(c) for c in counts) if counts else 0
        except Exception:
            max_count = 0

        try:
            low_mask = [
                (c is not None and int(c) < self.layout["low_sample_threshold"])
                for c in counts
            ]
        except Exception:
            low_mask = [False for _ in counts]

        # Compute top height for orange bars
        try:
            top = max(1, int(max_count * self.layout["count_axis_scale"]))
        except Exception:
            top = max_count if max_count > 0 else 1

        # Compute bar widths
        bar_width_bg, bar_width = self._compute_bar_widths(times)

        # Plot orange background bars for low-sample periods
        legend_data = None
        orange_heights = [top if m else 0 for m in low_mask]

        if any(orange_heights) and top > 0:
            ax2.bar(
                times,
                orange_heights,
                width=bar_width_bg,
                alpha=0.25,
                color=self.colors["low_sample"],
                zorder=0,
            )
            legend_data = (
                f"Low-sample (<{self.layout['low_sample_threshold']})",
                self.colors["low_sample"],
                0.25,
            )

        # Plot primary count bars
        ax2.bar(
            times,
            counts,
            width=bar_width,
            alpha=0.25,
            color=self.colors["count_bar"],
            label="Count",
            zorder=1,
        )

        # Set Y-axis limits
        try:
            ax2.set_ylim(0, top)
        except Exception:
            try:
                ymin, ymax = ax2.get_ylim()
                ax2.set_ylim(0, ymax * 1.4 if ymax > 0 else 1)
            except Exception:
                pass

        return legend_data

    def _compute_bar_widths(self, times: List[datetime]) -> Tuple[float, float]:
        """Compute responsive bar widths based on time bucket spacing."""
        try:
            if mdates is not None and len(times) > 1:
                x_nums = mdates.date2num(times)
                deltas = np.diff(x_nums)
                pos = deltas[deltas > 0]
                base = (
                    float(np.min(pos))
                    if pos.size > 0
                    else float(np.min(deltas) if deltas.size > 0 else 1.0)
                )
            else:
                # Fallback: compute from seconds
                x_arr = np.array(times, dtype=object)
                deltas_s = []
                for a, b in zip(x_arr[:-1], x_arr[1:]):
                    try:
                        ds = (b - a).total_seconds()
                    except Exception:
                        try:
                            ds = float(
                                (np.datetime64(b) - np.datetime64(a))
                                / np.timedelta64(1, "s")
                            )
                        except Exception:
                            ds = 86400.0
                    if ds > 0:
                        deltas_s.append(ds)
                base = (float(np.min(deltas_s)) / 86400.0) if deltas_s else 1.0
        except Exception:
            base = 1.0

        bar_width_bg = base * self.layout["bar_width_bg_fraction"]
        bar_width = base * self.layout["bar_width_fraction"]

        return bar_width_bg, bar_width

    def _configure_count_axis(self, ax2) -> None:
        """Configure right Y-axis (counts)."""
        ax2.set_ylabel("Count")
        try:
            ax2.tick_params(
                axis="both", which="major", labelsize=self.fonts["chart_axis_tick"]
            )
        except Exception:
            pass

    def _create_legend(
        self,
        fig,
        ax,
        ax2,
        legend_data: Optional[Tuple[str, str, float]],
    ) -> None:
        """Merge legends from both axes and position."""
        h1, l1 = ax.get_legend_handles_labels()
        h2, l2 = ax2.get_legend_handles_labels()

        if not (h1 or h2):
            return

        handles = h1 + h2
        labels = l1 + l2

        # Add low-sample patch if needed
        if legend_data is not None:
            try:
                lbl, col, alp = legend_data
                patch = Patch(facecolor=col, alpha=alp, edgecolor="none")
                handles.append(patch)
                labels.append(lbl)
            except Exception:
                pass

        try:
            # Horizontal legend below chart
            ncols = len(labels) if labels else 1
            leg = fig.legend(
                handles,
                labels,
                loc="lower center",
                bbox_to_anchor=(0.5, -0.12),
                ncol=ncols,
                framealpha=0.9,
                prop={"size": self.fonts["chart_legend"]},
            )

            # Style legend frame
            try:
                fr = leg.get_frame()
                fr.set_facecolor("white")
                fr.set_alpha(0.9)
                fr.set_edgecolor("#000000")
                fr.set_linewidth(1)
                leg.set_zorder(10)
                fr.set_zorder(11)
            except Exception:
                pass
        except Exception:
            # Fallback legend
            try:
                ax.legend(handles, labels, loc="lower right")
            except Exception:
                pass

    def _format_time_axis(self, fig, ax, tz_name: Optional[str]) -> None:
        """Format X-axis with date/time labels."""
        try:
            if mdates is None:
                return

            locator = mdates.AutoDateLocator()

            # Get timezone object if specified
            try:
                tzobj = ZoneInfo(tz_name) if tz_name else None
            except Exception:
                tzobj = None

            # Create formatter with timezone
            try:
                formatter = mdates.ConciseDateFormatter(locator, tz=tzobj)
            except TypeError:
                # Older matplotlib may not accept tz kwarg
                formatter = mdates.ConciseDateFormatter(locator)

            ax.xaxis.set_major_locator(locator)
            ax.xaxis.set_major_formatter(formatter)
            fig.autofmt_xdate()

            # Hide offset text
            try:
                ax.xaxis.get_offset_text().set_visible(False)
            except Exception:
                try:
                    ax.xaxis.set_offset_position("none")
                except Exception:
                    pass
        except Exception:
            pass

    def _apply_final_styling(self, fig, ax, ax2) -> None:
        """Apply final layout adjustments and styling."""
        # Tight layout with legend space
        try:
            fig.tight_layout(pad=0)
        except Exception:
            pass

        # Adjust subplot margins
        try:
            fig.subplots_adjust(
                left=self.layout["chart_left"],
                right=self.layout["chart_right"],
                top=self.layout["chart_top"],
                bottom=self.layout["chart_bottom"],
            )
        except Exception:
            pass


class HistogramChartBuilder:
    """Builds histogram charts for velocity distribution visualization."""

    def __init__(
        self,
        colors: Optional[Dict[str, str]] = None,
        fonts: Optional[Dict[str, int]] = None,
        layout: Optional[Dict[str, Any]] = None,
    ):
        """Initialise histogram builder with styling configuration.

        Args:
            colors: Color palette dict (defaults to DEFAULT_COLOR_CONFIG)
            fonts: Font size dict (defaults to DEFAULT_FONT_CONFIG)
            layout: Layout config dict (defaults to DEFAULT_LAYOUT_CONFIG)
        """
        if not HAVE_MATPLOTLIB:
            raise ImportError(
                "matplotlib is required for chart generation. "
                "Install it with: pip install matplotlib"
            )

        self.colors = colors or _colors_to_dict(DEFAULT_COLOR_CONFIG)
        self.fonts = fonts or _fonts_to_dict(DEFAULT_FONT_CONFIG)
        self.layout = layout or _layout_to_dict(DEFAULT_LAYOUT_CONFIG)

    def build(
        self,
        histogram: Dict[str, int],
        title: str,
        units: str,
        debug: bool = False,
    ) -> object:
        """Build histogram chart from bucket data.

        Args:
            histogram: Dict mapping bucket labels to counts
            title: Chart title
            units: Units string for X-axis label (e.g., "mph")
            debug: Enable debug output

        Returns:
            matplotlib Figure object
        """
        # Create figure
        fig, ax = plt.subplots(figsize=self.layout["histogram_figsize"])

        # Handle empty data
        if not histogram:
            ax.text(0.5, 0.5, "No histogram data", ha="center", va="center")
            ax.set_title(title)
            return fig

        # Sort and extract data
        try:
            sorted_items = sorted(histogram.items(), key=lambda x: float(x[0]))
        except Exception:
            sorted_items = sorted(histogram.items(), key=lambda x: str(x[0]))

        labels = [item[0] for item in sorted_items]
        counts = [item[1] for item in sorted_items]

        # Debug output
        if debug:
            total = sum(counts)
            print(f"DEBUG: histogram bins={len(labels)} total={total}")

        # Plot bars
        x = list(range(len(labels)))
        ax.bar(
            x, counts, alpha=0.7, color="steelblue", edgecolor="black", linewidth=0.5
        )

        # Configure axes and title
        ax.set_xlabel(f"Velocity ({units})", fontsize=self.fonts["histogram_label"])
        ax.set_ylabel("Count", fontsize=self.fonts["histogram_label"])
        ax.set_title(title, fontsize=self.fonts["histogram_title"])

        # Format X-axis labels
        formatted_labels = self._format_labels(labels)
        self._set_tick_labels(ax, x, formatted_labels)

        # Apply styling
        ax.tick_params(
            axis="both", which="major", labelsize=self.fonts["histogram_tick"]
        )

        # Layout adjustments
        try:
            fig.tight_layout(pad=0)
        except Exception:
            pass

        try:
            fig.subplots_adjust(left=0.02, right=0.985, top=0.96, bottom=0.08)
        except Exception:
            pass

        return fig

    def _format_labels(self, labels: List[str]) -> List[str]:
        """Format histogram labels to match table format (e.g., '5-10', '50+').

        Converts bucket start values to range labels:
        - Single values like '5', '10' → '5-10', '10-15', etc.
        - Detects bucket size from consecutive labels
        - Last bucket formatted as 'N+' (open-ended)
        - Non-numeric labels passed through unchanged
        """
        formatted = []

        # Try to parse labels as floats to detect ranges
        numeric_labels = []
        for lbl in labels:
            try:
                numeric_labels.append(float(lbl))
            except Exception:
                # Non-numeric label - pass through as-is
                formatted.append(str(lbl))
                continue

        # If we have numeric labels, convert to ranges
        if numeric_labels:
            # Detect bucket size from first two consecutive labels
            bucket_size = None
            if len(numeric_labels) >= 2:
                bucket_size = numeric_labels[1] - numeric_labels[0]

            for i, val in enumerate(numeric_labels):
                is_last = i == len(numeric_labels) - 1

                if is_last:
                    # Last bucket: format as "N+" (open-ended)
                    formatted.append(f"{int(val)}+")
                elif bucket_size:
                    # Regular bucket: format as "A-B"
                    next_val = val + bucket_size
                    formatted.append(f"{int(val)}-{int(next_val)}")
                else:
                    # Fallback: just show the value
                    formatted.append(f"{int(val)}")

        return formatted

    def _set_tick_labels(self, ax, x: List[int], formatted_labels: List[str]) -> None:
        """Set X-axis tick labels with responsive thinning."""
        if len(formatted_labels) <= 20:
            ax.set_xticks(x)
            ax.set_xticklabels(
                formatted_labels,
                rotation=45,
                ha="right",
                fontsize=self.fonts["histogram_tick"],
            )
        else:
            # Thin labels for dense histograms
            step = max(1, len(formatted_labels) // 15)
            tick_pos = x[::step]
            tick_labels = formatted_labels[::step]
            ax.set_xticks(tick_pos)
            ax.set_xticklabels(
                tick_labels,
                rotation=45,
                ha="right",
                fontsize=self.fonts["histogram_tick"],
            )
