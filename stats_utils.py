"""Compatibility shim for tests that import stats_utils from the project root.

This file re-exports symbols from the real implementation under
internal.report.query_data.stats_utils so tests can continue importing
`stats_utils` without changing import paths.
"""

from internal.report.query_data.stats_utils import *  # noqa: F401,F403
