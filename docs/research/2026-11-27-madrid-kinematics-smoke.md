# Madrid v2 kinematics smoke test (2026-11-27)

The test asks whether this release can serve as an external reference for position-derived
speed and coarse longitudinal acceleration.

- **Status:** One-session exploratory benchmark. No estimator, release gate, safety threshold,
  or production metric changed.

## Source and reproduction

The Madrid [study](https://doi.org/10.1038/s41597-026-08900-x) cites two Zenodo records.
[18493475](https://zenodo.org/records/18493475) is v1.0; its newer-version link goes to
[24137291](https://zenodo.org/records/24137291), v2, released 11 November 2026.
Both records are CC BY 4.0. File equivalence across versions has not been checked.

Download only the v2 session file [Participant07.csv](https://zenodo.org/records/24137291/files/Participant07.csv?download=1)
outside the checkout. Its 21,409,437 bytes match deposited MD5 `6a5e42f16f808ddeb1f91bae76bac4b8`
and pinned SHA-256 `01f4a14840a9745f97b0a3bd6401ff1453967d27830ac392a9095438993361d2`.
Run the [offline script](../../scripts/lidar_madrid_kinematics_smoke.py) from the repository root
(Python 3, NumPy 2 or later; evaluated with NumPy 2.3.5):

```sh
python scripts/lidar_madrid_kinematics_smoke.py /path/to/Participant07.csv
```

Only the two elapsed-time columns, GNSS positions, CAN speed and brake input are used.
The script rejects an unmatched file and prints aggregate diagnostics, never coordinates or
row-level output. A cloud rerun produced identical aggregate output on the pinned file.

## Evidence

- The 152,124 merged rows span 737.616 seconds on the column labelled GPS time and 765.720
  seconds on the eye-tracking time column. The difference is 28.103 seconds before rounding
  each span; the eye-tracking clock has two gaps above 100 ms. Median row intervals are 4.69
  and 5.00 ms, respectively. Exactly 48.3% of adjacent coordinate pairs repeat; 56.8% of CAN
  speed readings repeat. The paper describes a 10 Hz native GNSS chipset and a merged roughly
  200 Hz time base. These rows are not independent GNSS fixes, and the export does not identify
  which rows are native fixes.
- Raw GNSS fields are outside decimal-degree bounds. Dividing by 60 produces coordinates in the
  broad Madrid area; interpreting the values as signed NMEA degrees-and-minutes does not.
  Total arcminutes and CAN speed in km/h are working assumptions, not independently documented
  unit calibration. The 10 Hz GNSS chord sum is 4.334 km versus 4.838 km for integrated CAN
  speed, with three seconds trimmed from each GNSS edge.
- On 4,887 moving evaluation points (unshifted CAN speed above 2 m/s), the 1-second centred
  position chord gives a mean GPS-minus-CAN bias of −0.988 m/s and a p95 absolute error of
  1.843 m/s. The same row-mapped moving support gives 0.988 m/s MAE with the GPS-labelled clock
  and 1.293 m/s with the eye-tracking clock. Bias ranges from −0.413 m/s in the 2 to <5 m/s CAN
  band to −1.776 m/s in the 16 to <21 m/s band. Do not fit or adopt a correction from one drive.
- A CAN query-time scan (−2 to +2 seconds in 0.1-second steps, holding the support fixed) found
  quarterwise minima of −0.1, 0, −0.1 and 0 seconds, improving RMSE by at most 0.006 m/s.
  This is not evidence of a clock correction or an independent bound on channel latency.
- Centred 1-second differences of the 1-second GNSS chord speed and of CAN speed disagree by
  0.155 m/s² MAE and 0.391 m/s² p95 absolute error. At 606 moving points above a descriptive
  brake-input threshold of 10 native units, their median derivatives were −1.371 and −1.515
  m/s². Brake input is not a physical onset label, and CAN-speed derivatives are not independent
  acceleration or jerk measurements.

## Open questions

Use v2 as an offline diagnostic candidate, not as a calibrated physical reference. Verify native
fix timing, unit metadata and reference provenance; repeat across sessions with predeclared
windows and independent acceleration evidence before qualifying derivative accuracy.
A single instrumented car provides no paired-vehicle gap, leader association or DRAC truth and
does not validate the fixed LiDAR sensor. See the [behaviour analytics plan](../plans/lidar-behaviour-analytics-plan.md)
and [benchmark planning PR #1322](https://github.com/banshee-data/velocity.report/pull/1322).
