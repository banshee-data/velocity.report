# OPS243-A FFT stream exploration

This investigation examines what radar FFT parsing can show about simultaneous
vehicles and how its evidence could be presented against Gerardus vector geometry.
It reviews existing code and one manufacturer capture; it is not a physical
validation of multi-vehicle tracking.

- **Contract:** [Radar FFT evidence](../../../data/structures/RADAR_FFT_FORMAT.md)
- **Plan:** [Implementation and gates](../../plans/radar-fft-parsing-plan.md)

## Sources and parsing findings

| Source                                                                                                                                     | Finding                                                                                                                         |
| ------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------- |
| [Manufacturer Python plotter](https://github.com/omnipresense/python_plotter/blob/80cd43154626395c5b3def498fe4823258fe7ca1/plotter.py)     | Parses a flat FFT array; separately mean-centres and windows I/Q before a local complex FFT                                     |
| [ROS driver](https://github.com/SCU-RSL-ROS/radar_omnipresense/blob/db1f290aa622c4b6e586ee479245f2ba1177bf80/src/radar_publisher.cpp)      | Explicitly rejects OF and OR despite older documentation advertising FFT support                                                |
| [Rolling-buffer processor](https://github.com/omnipresense/rolling_buffer/blob/e40383f73e378189a44b1f231af46d56e839bcb7/rolling_buffer.py) | Demonstrates conversion and peak processing; mirrored-frequency winner selection is unsuitable for independent opposing targets |
| [AN-010-AF API](https://omnipresense.com/wp-content/uploads/2026/09/AN-010-AF_API_Interface.pdf)                                           | Documents OF/Of for Doppler FFT output, distinct FMCW commands, sample/FFT settings, and low-baud UART limitations              |
| [AN-011 cosine error](https://omnipresense.com/wp-content/uploads/2023/03/AN-011-B_Cosine-Error_2303.pdf)                                  | Establishes why viewing angle changes measured radial speed                                                                     |
| [AN-021 vehicle classification](https://omnipresense.com/wp-content/uploads/2025/05/AN-021-B_Vehicle-Classification_250521.pdf)            | Describes post-event magnitude/speed summaries; these do not provide target coordinates                                         |

These are implementation references, not a recommendation to run their sensor
initialisation sequences on a deployed unit. Direction conventions and array
representations require actual model/firmware captures before a parser is frozen.

## Public capture check

The source is [Radar-Datasets](https://github.com/omnipresense/Radar-Datasets/tree/92f02dde39e994df22e71d7b7f9049103dd3920e),
specifically Bike_Inbound_231107_1254 and its published OPS243-A configuration.
The pinned tree and the path Bike_Inbound_231107_1254/Bike_Inbound_231107_1254.txt
identify the analysed payload.

Four ordered I/Q pairs were present, each with 1024 samples per channel. The
archived text uses tab-separated array entries; analysis normalised delimiters
for this archived sample only. A live JSON parser must not silently repair
arbitrary malformed input. The published configuration gives 10 ksps, 1024-point
FFT, and 0.0607 m/s per bin.

For the second I/Q pair, analysis subtracted each channel's mean, applied a
1024-sample Hann window, formed I + iQ, and computed an unpadded complex FFT.
Magnitudes were examined in native bin order, with the upper half interpreted as
negative frequencies. No tuning arms or pipeline changes were run.

| Observation                               | Result                                                                                    |
| ----------------------------------------- | ----------------------------------------------------------------------------------------- |
| Dominant lobe in the second pair          | Native bin 931, signed bin −93                                                            |
| Speed using the published bin calibration | −5.6451 m/s                                                                               |
| Nearby device speed report                | +5.58, +5.64, and +5.52 m/s                                                               |
| Interpretation                            | Adjacent reported bins are consistent with one lobe; they do not establish three vehicles |
| Sign                                      | Raw I + iQ convention differs from the nearby positive inbound report                     |

The capture supplies no reliable timestamp per raw buffer and no independent
multi-vehicle truth. The nearby speed report is not proven to refer to precisely
the same acquisition window. This supports a parser and plotting illustration,
not target-count accuracy, timing accuracy, direction calibration, or tracking
validation. The calculation does not establish numerical equivalence with the
firmware's window or magnitude scaling. Raw data and generated plots remain local
outputs; this report retains the method and findings without importing them.

## What to plot and learn

| View                                      | Useful evidence                                          | Limit                                                 |
| ----------------------------------------- | -------------------------------------------------------- | ----------------------------------------------------- |
| Magnitude versus signed radial speed      | Strong and weak motion components                        | One object can generate multiple peaks                |
| Time–speed waterfall                      | Entry, persistence, merging, crossing, and disappearance | Equal radial speeds can merge and identities can swap |
| Grouped lobe tracks                       | Temporal motion candidates beyond the strongest speed    | Component count is not vehicle count                  |
| Waterfall with projected LiDAR velocities | Which tracks explain spectral energy                     | Needs qualified clocks and poses                      |
| Spatial residual summary                  | Where associated speeds disagree or coverage weakens     | Populate only from qualified associations             |

Doppler slope alone is not longitudinal acceleration: a fixed-speed vehicle's
radial speed changes with its line of sight. Amplitude is not target range.
Near-zero clutter cannot enumerate parked or stationary vehicles. Zero-padding
changes bin interpolation, not the underlying observation duration or information.

## Scene overlay and lost positions

For a stationary radar origin r, track position p and track velocity v in one
qualified site frame, predict receding radial speed as v · (p − r) / |p − r|.
Compare the spectrum against this projection, retaining multiple plausible
tracks when they share a radial speed.

The vector scene supplies lanes, terrain, structures and registration context.
Radar-only output can place a sensor and approximate beam on the map, linked to
a spectrum. A positioned LiDAR association can carry radar evidence at the
track's position. An unmatched peak remains unresolved in the coverage region;
lane priors alone do not measure its position.

A lost LiDAR track can retain a bounded coasting prediction with growing
uncertainty. Radar can constrain radial motion but cannot supply an absolute
starting range or guarantee continued identity. Suppress unsupported positions.
Gerardus accepted poses are not yet persisted into this repository's recording
frame: see [the L7 status](../../plans/lidar-l7-scene-plan.md).

## Storage and ingest implications

A plain FFT JSON object currently reaches configuration handling in
[parse.go](../../../internal/serialmux/parse.go), and
[handlers.go](../../../internal/serialmux/handlers.go) keeps its latest array in
memory rather than a history. The serial fan-out's one-message buffers can skip
lines without recording the loss. Both need explicit treatment before a
waterfall can be interpreted as continuous evidence.

The fallback transport is 19,200 baud: approximately 1,920 bytes/s with 8N1.
At 115,200 baud the corresponding ceiling is 11,520 bytes/s, before application
overhead. ADC sample rate is not serial report cadence. Confirm which physical
transport is deployed and measure loss and sustainable output rate.

At 1024 float32 values and 10 spectra/s, array values alone require 3.538944 GB/day
before compression and metadata. This is arithmetic, not a measured file size
or supported rate. Use immutable compressed chunks with a time/source index,
configuration and clock epochs, and separate derived components and associations.

The next evidence should be synchronised spectra and positioned LiDAR tracks
for isolated passes, opposing traffic, similar-speed followers, and occlusion.
Freeze the protocol and thresholds first, then measure separability, ambiguity,
transport loss and Pi cost before making counting or fusion claims.
