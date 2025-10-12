#!/usr/bin/env python3
"""Chart saving utilities for PDF export.

This module handles saving matplotlib figures to PDF format with:
- Tight bounding box calculation
- Automatic size constraints (min/max width)
- Proportional scaling
- Clean resource management

Designed to produce PDF charts that integrate well with LaTeX documents.
"""

from typing import Optional

try:
    import matplotlib.pyplot as plt

    HAVE_MATPLOTLIB = True
except Exception:  # pragma: no cover
    HAVE_MATPLOTLIB = False
    plt = None

from pdf_generator.core.config_manager import DEFAULT_LAYOUT_CONFIG


class ChartSaver:
    """Handles saving matplotlib figures to PDF format.

    Features:
    - Tight bounding box calculation for minimal whitespace
    - Size constraints to prevent extreme dimensions
    - Automatic figure cleanup
    - Error handling with graceful fallbacks
    """

    def __init__(
        self,
        min_width_in: Optional[float] = None,
        max_width_in: Optional[float] = None,
    ):
        """Initialize chart saver with size constraints.

        Args:
            min_width_in: Minimum PDF width in inches (prevents tiny charts)
            max_width_in: Maximum PDF width in inches (prevents huge charts)
        """
        if not HAVE_MATPLOTLIB:
            raise ImportError(
                "matplotlib is required for chart saving. "
                "Install it with: pip install matplotlib"
            )

        self.min_width_in = min_width_in or DEFAULT_LAYOUT_CONFIG.min_chart_width_in
        self.max_width_in = max_width_in or DEFAULT_LAYOUT_CONFIG.max_chart_width_in

    def save_as_pdf(
        self,
        fig,
        output_path: str,
        close_fig: bool = True,
    ) -> bool:
        """Save matplotlib figure as PDF with tight bounds.

        Args:
            fig: matplotlib Figure object
            output_path: Output PDF file path
            close_fig: Whether to close figure after saving

        Returns:
            True if successful, False otherwise
        """
        try:
            # Resize figure based on tight bounding box
            self._resize_figure_to_tight_bounds(fig)

            # Save with tight bbox and no padding
            fig.savefig(output_path, bbox_inches="tight", pad_inches=0.0)
        except Exception:
            # Fallback: simple save without tight bounds
            try:
                fig.savefig(output_path)
            except Exception:
                return False

        # Clean up figure if requested
        if close_fig:
            self._close_figure(fig)

        return True

    def _resize_figure_to_tight_bounds(self, fig) -> None:
        """Resize figure based on tight bounding box with size constraints."""
        try:
            # Force render to get accurate bounds
            fig.canvas.draw()
            renderer = fig.canvas.get_renderer()

            try:
                tight_bbox = fig.get_tightbbox(renderer)
            except Exception:
                tight_bbox = None

            if tight_bbox is None:
                return

            # Convert from display units (points) to inches
            dpi = self._get_dpi(fig)
            width_in = tight_bbox.width / dpi
            height_in = tight_bbox.height / dpi

            # Validate dimensions
            if width_in <= 0 or height_in <= 0:
                return

            # Apply size constraints with proportional scaling
            width_in, height_in = self._apply_size_constraints(width_in, height_in)

            # Update figure size
            fig.set_size_inches(width_in, height_in)
        except Exception:
            # If anything fails, keep original figure size
            pass

    def _get_dpi(self, fig) -> float:
        """Get figure DPI, with fallback to default."""
        if hasattr(fig, "dpi"):
            return fig.dpi

        if plt is not None:
            return plt.rcParams.get("figure.dpi", 72)

        return 72  # Standard default

    def _apply_size_constraints(
        self,
        width_in: float,
        height_in: float,
    ) -> tuple[float, float]:
        """Apply min/max width constraints with proportional height scaling."""
        if width_in < self.min_width_in:
            # Scale up proportionally
            scale = self.min_width_in / width_in
            width_in = self.min_width_in
            height_in = height_in * scale
        elif width_in > self.max_width_in:
            # Scale down proportionally
            scale = self.max_width_in / width_in
            width_in = self.max_width_in
            height_in = height_in * scale

        return width_in, height_in

    def _close_figure(self, fig) -> None:
        """Close figure and free resources."""
        try:
            if plt is not None:
                plt.close(fig)
        except Exception:
            # Ignore close failures (e.g., matplotlib not fully available)
            pass


def save_chart_as_pdf(
    fig,
    output_path: str,
    close_fig: bool = True,
) -> bool:
    """Convenience function to save chart as PDF.

    This provides a simple functional interface matching the original API.

    Args:
        fig: matplotlib Figure object
        output_path: Output PDF file path
        close_fig: Whether to close figure after saving

    Returns:
        True if successful, False otherwise
    """
    try:
        saver = ChartSaver()
        return saver.save_as_pdf(fig, output_path, close_fig)
    except Exception:
        return False
