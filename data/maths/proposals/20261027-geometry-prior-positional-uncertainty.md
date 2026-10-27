# Positional uncertainty for exported scene geometry

This note specifies how an exporter bounds the position of a polygon vertex it writes into a
[geometry-prior file](../../../docs/lidar/architecture/geometry-prior-service.md), which answers
that format's open question Q3. The bound is a conservative half-width under stated assumptions,
with the same linear, worst-case propagation as the
[physical reference records](../../../internal/lidar/annotation/physical.go). It is a statement
about how well a surface was measured, read as in
[Geometry priors: data purpose](../../../docs/geometry-priors-data-purpose.md).

- **Status:** Proposal Math (Not Active in Current Runtime); no exporter computes it yet
- **Layers:** L2 Frames (returns), L3 Grid (settled cells), L4 Perception (ground tiles, wall
  planes), prior export
- **Related:** [geometry-prior-service.md](../../../docs/lidar/architecture/geometry-prior-service.md)
  (§File format specification),
  [vector-scene-map.md](../../../docs/lidar/architecture/vector-scene-map.md)
  (§4.3 simplification, §5.5 provenance),
  [ground-plane-extraction.md](../../../docs/lidar/architecture/ground-plane-extraction.md)

## 1. What is being bounded

A vertex $v$ stands for a point on an observed surface: a corner of a ground tile, an end or top
of a wall plane, a corner of a footprint. The exporter writes two numbers per feature:

- `position_uncertainty_m`, a horizontal half-width $h$: the surface point lies within a disc of
  radius $h$ about the vertex's horizontal position;
- `vertical_uncertainty_m`, a vertical half-width $w$ for the feature's heights and `z_*` values.

Both hold under the assumptions the file records (§5). They are bounds, not standard deviations,
and they are not combined by root-sum-square: the largest terms are shared by every return of a
session (range bias, elevation calibration, the registration), so they do not average down with
the number of returns. That is the reason the physical reference records propagate linearly, and
it applies here with more force, because a wall plane is fitted to hundreds of returns that all
carry the same calibration.

## 2. One return

A return at slant range $r$, azimuth $\alpha$ and elevation $\phi$, with half-widths
$\delta r$, $\delta\alpha$ and $\delta\phi$ on each, moves by at most

$$
h_{\text{ret}} = \delta r \cos\phi + r\cos\phi\,\delta\alpha + r\,\lvert\sin\phi\rvert\,\delta\phi
\le \delta r + r\,(\delta\alpha + \delta\phi)
$$

horizontally and

$$
w_{\text{ret}} = \delta r\,\lvert\sin\phi\rvert + r\cos\phi\,\delta\phi
\le \delta r + r\,\delta\phi
$$

vertically, to first order in the angles. The simplified right-hand forms are what an exporter
uses; they cost little at street elevations, where $\lvert\phi\rvert$ is a few degrees.

The inputs differ by where the point came from:

| Source of the point          | $\delta r$                                                                                                 | $\delta\alpha$                                                              | $\delta\phi$                  |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- | ----------------------------- |
| Raw return (L2 frame)        | Stated range accuracy                                                                                      | Stated azimuth accuracy, or half the horizontal resolution if none          | Stated elevation calibration  |
| Settled background cell (L3) | Stated range accuracy plus $3\,s$, where $s$ is the cell's EMA of absolute deviation (`RangeSpreadMeters`) | One azimuth bin: the point sits at the bin's lower edge (`azimuth_bin_deg`) | The ring's stated calibration |

A settled cell's range is an average over returns that may come from different parts of a sloped
surface within one bin, which is why its spread enters $\delta r$. The factor 3 is an assumption:
it covers about 98% of a Gaussian spread measured as a mean absolute deviation, and the file
records it.

## 3. One vertex

A vertex is placed from returns, then simplified, rounded and, in a georeferenced file, moved by
the registration. Each step adds its own term. The horizontal form uses
$r\cos\phi = \rho$ and $r\,\lvert\sin\phi\rvert \le \rho$, which holds for
$\lvert\phi\rvert \le 45°$, well beyond any street geometry:

$$
h(v) = \delta r + \rho_v(\delta\alpha + \delta\phi)
+ \underbrace{\rho_v\,\Delta\alpha}_{\text{edge sampling}}
+ \rho_{95} + \varepsilon_{\text{LOD}} + \tfrac{\sqrt{2}}{2}\,q
+ \underbrace{\tau_h + \rho_v\,\delta\psi}_{\text{registration}} + g_v
$$

$$
w(v) = \delta r + r_v\,\delta\phi + \underbrace{r_v\,\Delta\phi_k}_{\text{edge sampling}}
+ \rho_{95} + \tfrac{1}{2}\,q
+ \underbrace{\tau_v + r_v\,\delta\theta}_{\text{registration}}
$$

| Term                                             | Meaning                                                                                                                                                                                                                          |
| ------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| $\rho_v$, $r_v$                                  | Horizontal and slant range from the sensor to the vertex, in the session that placed it                                                                                                                                          |
| $\rho_v\,\Delta\alpha$                           | An edge that ends between two neighbouring returns lies somewhere in the gap. $\Delta\alpha$ is the horizontal sampling step (the azimuth resolution for returns, the bin width for cells). Only for a vertex on a vertical edge |
| $r_v\,\Delta\phi_k$                              | The same for a top or bottom edge, with $\Delta\phi_k$ the elevation step between the ring that hit the surface and the next one, from the sensor's calibration. Only for `z_min` and `z_max`                                    |
| $\rho_{95}$                                      | The 95th percentile of absolute point-to-plane residuals of the fit the vertex lies on: measured, and it absorbs roughness and noise the stated accuracies miss                                                                  |
| $\varepsilon_{\text{LOD}}$                       | The Douglas-Peucker tolerance of the vertex's level of detail (vector-scene-map §4.3), since a simplified boundary may sit that far from the fitted one                                                                          |
| $q$                                              | The coordinate storage quantum in metres. Per axis the rounding error is at most $q/2$; horizontally, $\sqrt{2}\,q/2$                                                                                                            |
| $\tau_h$, $\tau_v$, $\delta\psi$, $\delta\theta$ | Half-widths of the accepted registration's translation, yaw and tilt. Zero in a sensor-local file, whose frame is the sensor's by definition                                                                                     |
| $g_v$                                            | For an `inferred` vertex, the distance along the boundary from the vertex to the nearest supporting return: a corner completed across an unseen stretch could lie anywhere along it. Zero for an observed vertex                 |

The feature's half-widths are the largest over its vertices:
$h = \max_v h(v)$ and $w = \max$ over its height members.

### Heights that are only lower bounds

A wall's top is bracketed only when the ring above the highest return on the wall is seen to pass
over it, returning from beyond or not at all. When the highest ring still hits the wall, the top
is above the sensor's field of view and `z_max` is a lower bound with no upper half-width. The
exporter then writes the highest observed return as `z_max`, sets `z_max_censored` to `true`, and
leaves `vertical_uncertainty_m` out rather than writing a number that would read as two-sided. The
same rule applies to `z_min` when the ground is not seen at the wall's foot.

## 4. A worked example

Illustrative inputs, not a calibration: a Pandar40P wall corner at $\rho_v = 20$ m from settled
cells, $\delta r = 0.02$ m (the manufacturer's stated range accuracy, not validated here), one
azimuth bin $\delta\alpha = \Delta\alpha = 0.2°$ (1,800 bins), an assumed elevation calibration
$\delta\phi = 0.05°$, $\rho_{95} = 0.03$ m, LOD 2 ($\varepsilon = 0.1$ m), $q = 0.01$ m, a
sensor-local file and an observed corner. In radians, $0.2° = 0.00349$ and $0.05° = 0.00087$.

| Term                                | Metres    |
| ----------------------------------- | --------- |
| $\delta r$                          | 0.020     |
| $\rho_v(\delta\alpha + \delta\phi)$ | 0.087     |
| $\rho_v\,\Delta\alpha$              | 0.070     |
| $\rho_{95}$                         | 0.030     |
| $\varepsilon_{\text{LOD}}$          | 0.100     |
| $\tfrac{\sqrt{2}}{2}\,q$            | 0.007     |
| **$h$**                             | **0.314** |

At LOD 3 ($\varepsilon = 0.02$ m) the same corner is bounded at 0.234 m, and at 5 m at LOD 3 it is
bounded at 0.116 m. A registration with $\tau_h = 0.05$ m and $\delta\psi = 0.1°$ adds 0.085 m at
20 m. Two things follow. The angular terms dominate at street range, so a corner position measured
from a kerbside sensor is not centimetre-level beyond a few metres, whatever the level of detail;
the scene map's finest level should be read as the finest the measurements support, not as a
promise of centimetres. And the bound grows linearly with range, which is why a merged feature
keeps the widest half-width of its parts: a merge cannot claim a nearer session's precision for
geometry a farther one contributed.

## 5. What the file records

The bound is only as good as its assumptions, so the file names them. A prior file carries an
`uncertainty` metadata member:

| Member                   | Example                  | Notes                                                                                                                              |
| ------------------------ | ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------- |
| `model`                  | `"linear_worst_case_v1"` | This note's estimator                                                                                                              |
| `range_accuracy_m`       | `0.02`                   | $\delta r$                                                                                                                         |
| `range_accuracy_source`  | `"manufacturer"`         | `"manufacturer"` or `"measured"`; a manufacturer's figure is not validated by the file                                             |
| `azimuth_accuracy_deg`   | `0.2`                    | $\delta\alpha$ as used                                                                                                             |
| `elevation_accuracy_deg` | `0.05`                   | $\delta\phi$, and its source in the same way as the range                                                                          |
| `cell_spread_factor`     | `3`                      | The multiplier on a settled cell's spread                                                                                          |
| `residual_quantile`      | `0.95`                   | The quantile behind $\rho_{95}$                                                                                                    |
| `registration`           | `null`                   | `null` in a sensor-local file; otherwise translation, yaw and tilt half-widths and the accepted registration record they come from |

A value the exporter cannot supply makes the bound unavailable: it omits
`position_uncertainty_m` and the feature fails the format's requirement, rather than carrying a
number built on a guess.

## 6. What this does not establish

- No term is measured here. The stated accuracies are the manufacturer's, the elevation
  calibration is assumed, and $\rho_{95}$ and the cell spread are only as representative as the
  session that produced them.
- The bound covers measurement and processing, not change: a surface that moved after the
  acquisition time is outside it, which is why every record carries that time.
- It says nothing about surfaces the sensor did not see. An unobserved side is a coverage gap,
  recorded as one, not a region the bound extends over.
- A validation would compare exported vertices against independently surveyed points at several
  ranges and check that the stated half-widths contain them; none has been run.
