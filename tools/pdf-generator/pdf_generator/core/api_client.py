"""HTTP client for querying the radar stats API."""

import requests
from typing import Dict, List, Tuple, Optional, Any


# Server supported aggregation groups (in seconds)
SUPPORTED_GROUPS = {
    "15m": 15 * 60,
    "30m": 30 * 60,
    "1h": 60 * 60,
    "2h": 2 * 60 * 60,
    "3h": 3 * 60 * 60,
    "4h": 4 * 60 * 60,
    "6h": 6 * 60 * 60,
    "8h": 8 * 60 * 60,
    "12h": 12 * 60 * 60,
    "24h": 24 * 60 * 60,
}


class RadarStatsClient:
    """Client for querying radar statistics from the API."""

    def __init__(self, base_url: str = "http://localhost:8080"):
        """Initialise the client.

        Args:
            base_url: Base URL of the radar stats API
        """
        self.base_url = base_url.rstrip("/")
        self.api_url = f"{self.base_url}/api/radar_stats"

    def get_stats(
        self,
        start_ts: int,
        end_ts: int,
        group: str = "1h",
        units: str = "mph",
        source: str = "radar_objects",
        model_version: Optional[str] = None,
        timezone: Optional[str] = None,
        min_speed: Optional[float] = None,
        compute_histogram: bool = False,
        hist_bucket_size: Optional[float] = None,
        hist_max: Optional[float] = None,
    ) -> Tuple[List[Dict[str, Any]], Dict[str, int], requests.Response]:
        """Query radar statistics from the API.

        Args:
            start_ts: Start timestamp (unix seconds)
            end_ts: End timestamp (unix seconds)
            group: Aggregation period (15m, 30m, 1h, etc.)
            units: Speed units (mph, kph, etc.)
            source: Data source (radar_objects or radar_data_transits)
            model_version: Transit model version to request (for radar_data_transits)
            timezone: Timezone for StartTime conversion
            min_speed: Minimum speed filter
            compute_histogram: Whether to request histogram data
            hist_bucket_size: Histogram bucket size in display units
            hist_max: Maximum speed for histogram

        Returns:
            Tuple of (metrics list, histogram dict, response object)

        Raises:
            requests.HTTPError: If the API request fails
        """
        params = {
            "start": start_ts,
            "end": end_ts,
            "group": group,
            "units": units,
            "source": source,
        }
        if model_version:
            params["model_version"] = model_version
        if timezone:
            params["timezone"] = timezone
        if min_speed is not None:
            params["min_speed"] = min_speed
        if compute_histogram:
            params["compute_histogram"] = "true"
            if hist_bucket_size is not None:
                params["hist_bucket_size"] = hist_bucket_size
            if hist_max is not None:
                params["hist_max"] = hist_max

        resp = requests.get(self.api_url, params=params)
        resp.raise_for_status()
        payload = resp.json()

        # Extract metrics and histogram from payload
        metrics = payload.get("metrics", []) if isinstance(payload, dict) else payload
        histogram = payload.get("histogram", {}) if isinstance(payload, dict) else {}

        return metrics, histogram, resp
