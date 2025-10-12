#!/usr/bin/env python3
"""Data transformation and normalization utilities.

This module provides utilities for normalizing field names from API responses,
handling schema variations across different data sources and API versions.
"""

from typing import Any, Dict, List, Optional, Union
import numpy as np


# =============================================================================
# Field Alias Mappings
# =============================================================================

FIELD_ALIASES: Dict[str, List[str]] = {
    # Speed percentiles
    "p50": ["p50", "P50Speed", "p50speed", "p50_speed"],
    "p85": ["p85", "P85Speed", "p85speed", "p85_speed"],
    "p98": ["p98", "P98Speed", "p98speed", "p98_speed"],
    # Max speed
    "max_speed": ["max_speed", "MaxSpeed", "maxspeed", "max"],
    # Time fields
    "start_time": ["start_time", "StartTime", "starttime", "start_time_utc"],
    # Count
    "count": ["Count", "cnt", "count"],
}


# =============================================================================
# MetricsNormalizer Class
# =============================================================================


class MetricsNormalizer:
    """Normalizes field names from API responses to a consistent schema.

    This class handles the variation in field naming conventions across different
    API sources (radar_objects vs radar_data_transits) and ensures downstream
    code can use consistent field names.

    Example:
        normalizer = MetricsNormalizer()
        row = {"P50Speed": 25.5, "Count": 42, "StartTime": "2024-01-01T00:00:00Z"}
        normalized = normalizer.normalize(row)
        # normalized = {"p50": 25.5, "count": 42, "start_time": "2024-01-01T00:00:00Z"}
    """

    def __init__(self, aliases: Optional[Dict[str, List[str]]] = None):
        """Initialize the normalizer with field aliases.

        Args:
            aliases: Optional custom alias mapping. If None, uses FIELD_ALIASES.
        """
        self.aliases = aliases or FIELD_ALIASES

    def get_value(self, row: Dict[str, Any], field: str, default: Any = None) -> Any:
        """Get field value trying all known aliases.

        Args:
            row: Data row dictionary
            field: Normalized field name (e.g., 'p50', 'count')
            default: Default value if field not found

        Returns:
            Field value or default if not found
        """
        if field not in self.aliases:
            # Field has no known aliases, try direct lookup
            return row.get(field, default)

        # Try all aliases in order
        for alias in self.aliases[field]:
            if alias in row and row[alias] is not None:
                return row[alias]

        return default

    def normalize(self, row: Dict[str, Any]) -> Dict[str, Any]:
        """Normalize a data row to use consistent field names.

        Args:
            row: Original data row with potentially varied field names

        Returns:
            New dictionary with normalized field names
        """
        normalized = {}

        # Copy all original fields
        for key, value in row.items():
            normalized[key] = value

        # Add normalized fields
        for field in self.aliases.keys():
            value = self.get_value(row, field)
            if value is not None:
                normalized[field] = value

        return normalized

    def get_numeric(
        self, row: Dict[str, Any], field: str, default: float = np.nan
    ) -> float:
        """Get a numeric field value trying all known aliases.

        Args:
            row: Data row dictionary
            field: Normalized field name
            default: Default value if field not found or not numeric

        Returns:
            Float value or default
        """
        value = self.get_value(row, field)
        if value is None:
            return default

        try:
            return float(value)
        except (ValueError, TypeError):
            return default


# =============================================================================
# Helper Functions
# =============================================================================


def extract_metrics_from_row(
    row: Dict[str, Any], normalizer: Optional[MetricsNormalizer] = None
) -> Dict[str, float]:
    """Extract speed metrics from a data row using normalization.

    Args:
        row: Data row from API response
        normalizer: Optional normalizer instance (creates default if None)

    Returns:
        Dict with keys: p50, p85, p98, max_speed (all as floats)
    """
    if normalizer is None:
        normalizer = MetricsNormalizer()

    return {
        "p50": normalizer.get_numeric(row, "p50"),
        "p85": normalizer.get_numeric(row, "p85"),
        "p98": normalizer.get_numeric(row, "p98"),
        "max_speed": normalizer.get_numeric(row, "max_speed"),
    }


def extract_count_from_row(
    row: Dict[str, Any], normalizer: Optional[MetricsNormalizer] = None
) -> int:
    """Extract count value from a data row using normalization.

    Args:
        row: Data row from API response
        normalizer: Optional normalizer instance (creates default if None)

    Returns:
        Integer count or 0 if not found
    """
    if normalizer is None:
        normalizer = MetricsNormalizer()

    count_val = normalizer.get_value(row, "count", 0)
    try:
        return int(count_val)
    except (ValueError, TypeError):
        return 0


def extract_start_time_from_row(
    row: Dict[str, Any], normalizer: Optional[MetricsNormalizer] = None
) -> Optional[str]:
    """Extract start_time value from a data row using normalization.

    Args:
        row: Data row from API response
        normalizer: Optional normalizer instance (creates default if None)

    Returns:
        Start time string or None if not found
    """
    if normalizer is None:
        normalizer = MetricsNormalizer()

    return normalizer.get_value(row, "start_time")


# =============================================================================
# Batch Processing Functions
# =============================================================================


def normalize_metrics_list(
    metrics: List[Dict[str, Any]], normalizer: Optional[MetricsNormalizer] = None
) -> List[Dict[str, Any]]:
    """Normalize a list of metric rows.

    Args:
        metrics: List of data rows from API
        normalizer: Optional normalizer instance (creates default if None)

    Returns:
        List of normalized rows
    """
    if normalizer is None:
        normalizer = MetricsNormalizer()

    return [normalizer.normalize(row) for row in metrics]


def extract_metrics_arrays(
    metrics: List[Dict[str, Any]], normalizer: Optional[MetricsNormalizer] = None
) -> Dict[str, List[float]]:
    """Extract metric arrays from a list of rows for plotting.

    Args:
        metrics: List of data rows from API
        normalizer: Optional normalizer instance (creates default if None)

    Returns:
        Dict with keys p50, p85, p98, max_speed, each mapping to a list of floats
    """
    if normalizer is None:
        normalizer = MetricsNormalizer()

    p50_list = []
    p85_list = []
    p98_list = []
    max_list = []

    for row in metrics:
        p50_list.append(normalizer.get_numeric(row, "p50"))
        p85_list.append(normalizer.get_numeric(row, "p85"))
        p98_list.append(normalizer.get_numeric(row, "p98"))
        max_list.append(normalizer.get_numeric(row, "max_speed"))

    return {
        "p50": p50_list,
        "p85": p85_list,
        "p98": p98_list,
        "max_speed": max_list,
    }
