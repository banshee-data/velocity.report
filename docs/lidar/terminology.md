# LiDAR Terminology

Core terms used across the LiDAR tracking system.

| Term                   | Definition                                                                                                                                                   |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Point**              | A single 3D measurement from the LiDAR sensor (x, y, z, intensity, timestamp).                                                                               |
| **Cluster**            | A group of spatially-proximate foreground points identified by DBSCAN, representing a potential object.                                                      |
| **Track**              | A temporally-linked sequence of clusters representing a moving object across frames, maintained by the Kalman-filter tracker.                                |
| **Observation**        | A single cluster-to-track association at one point in time (one frame's measurement of a track).                                                             |
| **Scene**              | A named collection of reference ground-truth labels for a specific sensor environment (installation, angle, location). Used for evaluating tracking quality. |
| **Run** (Analysis Run) | A single processing pass over a data source (live or PCAP) with fixed parameters, producing tracks that can be compared against a scene's ground truth.      |
| **Sweep**              | A batch execution that varies parameter combinations, running one analysis per combination and collecting metrics for comparison.                            |
| **Auto-Tune**          | An iterative sweep that narrows parameter bounds across rounds, converging on optimal parameters via objective scoring.                                      |
