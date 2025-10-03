"""Query data module for radar statistics analysis."""

from .api_client import RadarStatsClient, SUPPORTED_GROUPS
from .date_parser import parse_date_to_unix, parse_server_time, is_date_only

try:
    # latex_generator is legacy and may not be present in some branches; import if available
    from .latex_generator import stats_to_latex, generate_table_file  # type: ignore

    _HAS_LATEX = True
except Exception:  # pragma: no cover - environment dependent
    stats_to_latex = None
    generate_table_file = None
    _HAS_LATEX = False

__all__ = [
    "RadarStatsClient",
    "SUPPORTED_GROUPS",
    "parse_date_to_unix",
    "parse_server_time",
    "is_date_only",
]

if _HAS_LATEX:
    __all__.extend(["stats_to_latex", "generate_table_file"])
