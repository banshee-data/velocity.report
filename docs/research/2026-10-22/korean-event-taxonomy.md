# South Korean digital-tachograph 11-event taxonomy

**Evidence:** Jin et al., *Journal of Korean Society of Transportation* (August 2026), as described in the October 22 briefing. Check the primary Korean definitions and vehicle-specific parameter tables before quoting regulatory cutoffs.

Eleven categories reported: speeding, prolonged speeding, rapid acceleration, rapid start, rapid deceleration, abrupt stop, abrupt left turn, abrupt right turn, abrupt U-turn, abrupt overtaking and abrupt lane change. Briefing examples: speeding 20 km/h above posted limit; prolonged speeding >3 minutes; class-specific deceleration examples 2.50 m/s² (bus), 3.89 m/s² (taxi/general), 2.22 m/s² (freight). These values are **not universal braking capabilities** or validated danger thresholds.

| Event family | Fixed roadside LiDAR feasibility | Key caveat |
|---|---|---|
| Speeding | High | Posted-limit provenance and passage-speed quality |
| Prolonged speeding | Not observable with short roadside window | Three-minute exposure requires long corridor or mobile telemetry |
| Rapid acceleration / deceleration | Moderate–high | Temporal smoothing and trajectory support |
| Rapid start / abrupt stop | Moderate | Must see the complete maneuver and relevant low-speed states |
| Left/right turn and U-turn | Moderate | Lane map, turn classification, observation coverage |
| Abrupt lane change | Moderate | Stable identity, lane boundaries and lateral motion |
| Abrupt overtaking | Lower | Must observe both road users and full maneuver |

Do not silently redefine a three-minute standard to fit a short LiDAR window. A shorter observable event would be a **different metric**. Preserve vehicle-class-dependent definitions and local jurisdiction. Research-only benchmark profile pending source verification.
