# Portable LiDAR and IMU timing

This design gives a backpack or cargo-bike recorder a shared acquisition clock without adding
odometry to the Pi. It separates documented component capabilities, proposed acceptance limits,
and measurements still required on the actual rig.

- **Status:** Proposed; documentation only, no hardware validation or purchase
- **Layers:** L1 Packets, L2 Frames, capture provenance, offline trajectory generation
- **Related:** [route capture plan](../../plans/lidar-route-capture-plan.md), [backpack experiment](../../../data/experiments/try/backpack-motion-spectrum-and-stabiliser.md), [GPS ingest](gps-ethernet-parsing.md)

## Decision

Prototype a GNSS receiver with exposed PPS and serial time, a small MCU that records PPS and IMU
data-ready edges, and a raw six-axis IMU on the LiDAR's rigid plate. Feed the same PPS reference
and correctly associated serial seconds to the Pandar40P. The Pi records raw packets and timing
evidence; a workstation reconstructs the clocks, calibrates the sensors, and estimates motion.

The concrete candidate is SparkFun MAX-M10S GPS-18037, Raspberry Pi Pico (RP2040, without
wireless),
and Adafruit LSM6DSOX #4438. Budget **US $120–145 for the complete timing/IMU add-on**, excluding
tax, shipping, tools, and labour. This assumes the owned LiDAR has a usable connection box and
harness. The IMU board is $11.95; a complete synchronised recorder does not cost $11.95.

Keep host-arrival timestamps and offline alignment as the immediately available fallback. Accept
that fallback only where its measured uncertainty passes the motion-dependent gate below;
otherwise retain the capture for geometry experiments or use LiDAR-only processing with its own
quality gate. A GNSS-free shared local clock is a promising second hardware mode, conditional on
the sensor accepting the proposed epoch messages. PTP is useful with a verified timing-capable
NIC, but does not remove the need to map IMU acquisition time.

## What the repository does today

Code inspected at PR #580 head `104b0613b6ebc47b0ca8ed62795a920c56104733`:

| Path                                                                                                                                                            | Observed behaviour                                                                                            | Consequence for route capture                                                                                     |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| [parse/config.go](../../../internal/lidar/l1packets/parse/config.go), `ConfigureTimestampMode`                                                                  | Defaults to `system`; accepts `system`, `gps`, `internal`, and `lidar`                                        | An environment setting is not proof of synchronisation; `ptp` is not a recognised setting here                    |
| [parse/extract.go](../../../internal/lidar/l1packets/parse/extract.go), `resolvePacketTime`                                                                     | System mode calls `time.Now()` during parsing; native `lidar` mode combines packet date and microseconds      | Live default time includes socket, queue, and parser latency                                                      |
| Same function                                                                                                                                                   | GPS/PTP enum paths add the microsecond field to parser boot time, with a static-value fallback to system time | This does not establish a UTC epoch; treating a within-second field as elapsed boot time also mishandles rollover |
| [network/pcap.go](../../../internal/lidar/l1packets/network/pcap.go) and [network/pcap_realtime.go](../../../internal/lidar/l1packets/network/pcap_realtime.go) | Both supply PCAP time through `SetPacketTime`; the parser gives that override precedence over every mode      | Selecting native mode alone does not preserve native acquisition time during replay                               |
| `blockToPoints` in `extract.go`                                                                                                                                 | Adds channel firetime to one packet time; `blockIdx` does not contribute to point time                        | Repeated blocks receive the same channel time; block and return timing must be reconstructed before deskew        |
| [l2frames/frame_builder.go](../../../internal/lidar/l2frames/frame_builder.go)                                                                                  | Copies these point times into frames                                                                          | The `Point.Timestamp` acquisition-time comment describes intent, not a validated guarantee                        |

For scale, Appendix B of the [Pandar40P manual][hesai] gives a single-return block separation of
55.56 µs: nine missing separations span about 500 µs. Channel correction alone cannot repair this.
Validate packet reference, block order, channel offsets, and dual-return pairing against the
matching vendor decoder and actual firmware. Do not patch the moving case by changing the
fixed-sensor default silently.

Implementation prerequisite: retain **both** packet arrival and native sensor time, with source,
validity, and clock-segment identifiers. Offline point-time reconstruction must be invariant to
replay pacing and PCAP arrival perturbations. Date normalisation by `time.Date`, a plausible year,
or increasing timestamps must not turn invalid sensor time into trusted UTC. These are backlog
requirements; this design changes no parser or runtime behaviour.

## Five different things called synchronisation

| Term                  | What it establishes                                                  | What it does not establish                                 |
| --------------------- | -------------------------------------------------------------------- | ---------------------------------------------------------- |
| Clock phase and rate  | The relationship between ticking clocks                              | Which calendar second a pulse names                        |
| Epoch and time scale  | The meaning of seconds: session time, UTC, GPS, or PTP/TAI           | When a sample was physically acquired                      |
| Acquisition timing    | The effective time of the measurement, including known sensor delays | The sensor's mounting transform                            |
| Mechanical extrinsics | Rotation and translation between LiDAR and IMU, including lever arm  | Clock offset or drift                                      |
| Scan rotation phase   | Rotor azimuth at a reference instant                                 | Clock lock, acquisition accuracy, or extrinsic calibration |

A FIFO timestamp, data-ready edge, completed SPI read, USB delivery, UDP receipt, and PCAP record
are distinct events. Record their meanings explicitly. FIFO batching and network transport can
be slow without corrupting acquisition time if the original times and sample identities survive.
An interrupt-handler timestamp includes interrupt latency; a hardware edge capture avoids that
part of the delay. Neither removes internal filtering or analogue response delay.

## Pandar40P applicability and electrical boundary

The [402-en-241220 manual][hesai] applies to software ≥2.10.8, sensor firmware ≥4.3.44a, and
controller firmware ≥4.53. It is evidence about **Pandar40P**, not Pandar40 or an unidentified
second-hand unit. Read the label, firmware versions, and box revision before using its pinout.

| Manual location         | Relevant specification                                                                                                                                                      |
| ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| §2.3, printed pp. 28–29 | GPS socket JST SM06B-SRSS-TB; mate SHR-06V-S-B. Pin 1: TTL 3.3/5 V PPS input; 2: 5 V output; 3/5: ground; 4: RS232 serial input; 6: reserved. View/orientation as Figure 13 |
| §2.2, printed pp. 23–24 | Lemo FGG.2T.316.CLAC75Z: NMEA pin 9, PPS pin 10; Phoenix is a separate variant. PPS period 1 s ±50 µs, width ≥1 ms, recommended 10–100 ms                                   |
| §4.2.3                  | GPS accepts GPRMC/GPGGA; selectable baud. PTP profiles include 1588v2 and 802.1AS; lock threshold 1–100 µs                                                                  |
| §1.5                    | Sensor: 9–48 V, typical 18 W excluding accessories; PTP accuracy ≤1 µs and constant-temperature holdover drift ≤1 µs/s                                                      |

Use the box's 9600-baud interface as the initial bench configuration. Verify numbering by the
drawing and continuity with power disconnected; cable colours alone are insufficient. The box's
5 V output is not a documented current budget for the proposed electronics. Leave it unconnected
when powering the add-on separately, with a deliberate common signal ground and no back-feed.
Verify the box's own power-input rating and polarity; the sensor's 9–48 V rating is not proof
that every connection box or accessory accepts that range.

The MAX-M10S breakout needs **3.3 V**, including its UART side. Put an RS232 line driver between
TTL serial output and the box; an I²C level shifter is not an RS232 converter. PPS bypasses that
converter. Check fan-out, rise time, polarity, grounding, and cable loading at both receiving
pins. Use a 3.3 V buffer if needed and measure its differential delay. Do not feed a 5 V or RS232
signal into Pico GPIO. Provide strain relief, protection, and a fused power branch on either rig.

The sensor's advertised clock lock is one contribution to the budget. It says nothing about
MCU edge capture, IMU group delay, sample association, or offline reconstruction. Check the actual
unit and its documentation if any model, firmware, connector, or voltage differs; no automatic
firmware upgrade is part of this proposal.

## Architecture comparison

| Option                      | Acquisition clock chain                                                        | Cost/power and integration judgement                                                                                                        | Decision                                              |
| --------------------------- | ------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| Shared GNSS PPS + time      | GNSS edge to LiDAR and MCU; MCU captures IMU data-ready; serial labels seconds | About $120–145 complete add-on; reserve 1 W. Requires harness, MCU firmware, epoch validation, and filter-delay work                        | Preferred outdoor baseline for both rigs              |
| Local PPS + synthetic epoch | MCU hardware generates PPS and matching serial time; same MCU captures IMU     | About $57–82 using the same build without the $62.45 GNSS/antenna pair; reserve 0.7 W. Oscillator and epoch-emulation tests required        | Conditional GNSS-free hardware fallback               |
| Ethernet PTP                | Master PHC to LiDAR; separate proven mapping from MCU/IMU to PHC               | Pi 4 software PTP adds little hardware but uncertain jitter; replacing compute/NIC costs more and is not priced as an owned part            | Bench comparison, not baseline                        |
| Arrival/offline alignment   | Native LiDAR clock and IMU clock related using arrival evidence and motion     | $0 additional timing hardware when streams already exist; otherwise IMU/logger still needed. Highest uncertainty and offline fitting effort | Measured fallback; never assumed millisecond-accurate |

### GNSS baseline: label the same edge at both sensors

```text
GNSS antenna -> MAX-M10S
                 PPS -----------------------> Pandar40P PPS input
                  +-------------------------> MCU edge capture
                 serial time -> MCU validation/relay -> RS232 -> LiDAR NMEA input
LSM6DSOX gyro/accel data-ready ---------------> MCU edge capture
LSM6DSOX SPI samples/FIFO -------------------> MCU -> USB -> Pi recorder
Pandar40P raw UDP -----------------------------------------> Pi recorder
Pi: PCAP bundle + clock/configuration evidence -> workstation processing
```

Use the [MAX-M10S integration manual][m10-manual], UBX-20053088 R05 (April 28, 2026), for
MAX-M10S-00B-01 / ROM SPG 5.10. SparkFun also links an older R01 for -00 / SPG 5.00; read
`UBX-MON-VER` and use the matching interface description before configuring a board. Save its
configuration at each boot; battery-backed settings are not permanent provisioning.

Proposed configuration is continuous operation, one navigation epoch per second, and a 1 Hz
rising PPS aligned to UTC with a 10 ms pulse. Disable any unlocked pulse mode that looks identical
to locked output. Save `UBX-TIM-TP` and UTC validity/accuracy evidence from `UBX-NAV-PVT`,
including
`validDate`, `validTime`, `fullyResolved`, and confirmation flags where available. The manual says
`TIM-TP` names the **next** pulse: associating it with the previous edge is a whole-second error.
Time validity and position validity remain separate; a position fix LED is not sufficient.

PPS supplies phase, not a date. In the [Hesai timing sequence][hesai], the current-second NMEA
sentence follows its PPS rise, ends after its fall and ≥100 ms before the next rise; the sensor
uses the preceding message plus one second at the next boundary. Use RMC for its date, and test
midnight explicitly. GGA alone needs a separate date source. Do not assume a `GNRMC` talker is
accepted where the manual specifies `GPRMC`.

Configure compatible RMC output where possible. Otherwise the MCU relays a checksum-validated
sentence for the identified second, converting talker/checksum if necessary, while preserving the
original bytes in the log. Schedule within the verified window; at 9600 baud, 84 bytes with
8N1 framing occupy 87.5 ms. Drop a late or ambiguous message, record the failure, and degrade
quality rather than repeat last second's time. Retiming serial delivery does not retime PPS.
Never manufacture a valid position flag or coordinates to persuade the LiDAR to lock.

For each continuous clock segment fit `t_reference = a × tick_MCU + b` from PPS pairs. Check
held-out
edges and permit slowly varying rate only with measured support. Map IMU events through that
fit, subtract the calibrated effective sensor delay, then interpolate motion at reconstructed
LiDAR point times. Retain raw counts, fit coefficients, validity interval, and residuals so the
workstation can repeat or reject the fit.

### IMU acquisition: data-ready is not an external trigger

The [LSM6DSOX datasheet][imu-ds] (DS12814 Rev 4) specifies raw gyro/accelerometer output,
programmable
data-ready routing, FIFO timestamps at 25 µs resolution, and DEN stamping. The
[Adafruit board][imu-pins] exposes SPI and both interrupt pins. Proposed first setup: 416 Hz
gyro and accelerometer, gyro data-ready on INT1, accelerometer data-ready on INT2, SPI burst reads,
and a recorded filter configuration. Test 833 Hz and wider ranges for bike shocks; log clipping.
These are experiment settings, not an established useful bandwidth.

[ST AN5272 Rev 5][imu-an] §4.3 describes availability interrupts: pulsed mode gives a 75 µs pulse;
latched mode clears on a data read. Use pulsed edges, separate sensor identities, and hardware
capture. A latched line left unread can hide later samples. Block-data-update protects byte pairs,
not an arbitrarily slow multi-axis read. Validate SPI completion before overwrite, or reconstruct
FIFO records using tags, sample counts, and timestamps; never assign the FIFO-read time to all
its contents. Count overruns and missed edges and invalidate affected intervals.

The Pico's [RP2040 PIO][rp2040] permits deterministic pin sampling. Implement a common counter
domain for PPS, gyro, and accelerometer edge capture using PIO/DMA; do not advertise an ordinary
GPIO callback as hardware capture. Prove counter alignment, sampling quantisation, wrap extension,
and overflow handling under USB load. If that implementation cannot meet its bound, choose an MCU
with timer input-capture channels; the $4 board is a cost candidate, not working firmware.

Neither data-ready nor DEN promises that a conversion occurred on the external edge. DEN can tag
output; using it as a trigger without checking the configured mode changes the meaning of time.
For comparison, the [TDK ICM-42688-P][icm-ds] FSYNC facility records a delta from an FSYNC edge to
an ODR event and can put that indication in the FIFO. It is not a command to acquire all axes
at PPS. Its INT2/FSYNC/CLKIN pin has alternative functions; an external sample clock and a frame
tag are different designs. The exact breakout must expose the required pin. No unverified
ICM-42688 module is substituted into this BOM.

For FIFO operation, establish a second mapping from the IMU's own counter to MCU edge time using
unambiguous sample IDs. Do not reconstruct a long batch from nominal ODR alone: the IMU oscillator
can drift independently of the MCU. Save the actual association and its uncertainty.

Analogue and digital filters delay effective gyro and acceleration measurements, potentially by
different amounts and by more than the clock target. Record ODR, bandwidth, filter registers,
power mode, temperature, and settling exclusions. Measure phase versus frequency over the motion
band. Subtract a fixed delay only where that model fits; retain the residual as uncertainty.
A change of filters or ODR invalidates the old delay calibration. Good PPS residuals do not
measure this delay.

### GNSS-free shared timing

A crystal-clocked MCU can generate a periodic hardware edge and a serial RMC epoch for the LiDAR,
while timestamping the IMU in that same local clock. Absolute UTC is unnecessary for relative
deskew. Oscillator error still affects seconds and speed scale, so measure frequency versus
temperature and supply voltage; do not use a Linux sleep loop to generate PPS.

Use a supported, deliberately synthetic calendar origin plus a unique session/boot identifier.
Record `epoch_kind=session`, unknown UTC offset, measured rate error, and separate GNSS positioning
if a phone provides it. Never publish the synthetic date as capture UTC or treat dummy location
fields as a fix. Test whether the physical LiDAR accepts date/time in RMC with invalid navigation
status and no position. The manual does not establish that compatibility; if it refuses, this
mode remains unavailable. PPS alone with the LiDAR's unrelated free-running epoch is not the
proposed mode.

Test start-up, missing messages, midnight, restart, and the input pulse-period tolerance before
adopting it. Switching between GNSS and local timing starts a new clock segment; do not splice
epochs silently. In local mode, absolute time may drift while relative timing stays useful, but
only measured LiDAR-to-MCU residuals justify that claim.

### PTP and the Raspberry Pi

Treat the current plan's **Pi 4 Model B** as a software-timestamp platform. The inspected
[Raspberry Pi 6.12 GENET driver][genet] uses generic timestamp reporting; installing `ptp4l` does
not create a hardware clock. CM4 has different Ethernet hardware, and Pi 5 has RP1 with a Cadence
MAC ([RP1 manual][rp1]) and a [PTP driver][macb]. Hardware capability, kernel support, exposed PHC
pins, and a working application are separate checks. Generic USB Ethernet adapters are not an
assumed substitute.

On the actual recorder save model, NIC, kernel, driver, `ethtool -T <interface>`, `/dev/ptp*`, and
the mapping from interface to PHC. Require hardware transmit/receive timestamp capability and a
working PHC for a hardware claim, then test two-way offsets under sustained LiDAR and storage
load. Keep a direct link initially; switches add delay/asymmetry unless handled and measured.

Match LiDAR firmware, PTP profile, transport, domain, and delay mechanism with
[linuxptp][ptp4l]. A local master can distribute session time without GNSS. Mapping the IMU still
requires a common hardware edge captured by MCU and PHC, or a validated clock-transfer method.
Do not assume Pi 5 exposes a usable PHC PPS output or external timestamp input on its header.
The inspected driver capability table has zero external timestamp channels, periodic-output
channels, and pins; its PPS flag alone does not establish a physical PPS output.
System GPIO interrupts and USB clock exchanges add their own uncertainty. A GNSS PPS shared
with the MCU and a verified master input is another option, but reintroduces GNSS wiring.

Follow [linuxptp time-scale handling][phc2sys]: a hardware PTP clock commonly follows the
continuous
PTP/TAI scale while Linux system time follows UTC. Record `ptpTimescale`, `currentUtcOffset`, its
validity, leap flags, master identity, and servo state. Confirm the LiDAR's emitted date/time
against a known epoch: do not blindly subtract a remembered 18 or 37 seconds. GPS and TAI use
different origins/offsets, and UTC has leap discontinuities. Avoid mixing a smeared host clock
with an unsmeared source without an explicit mapping.

PTP mode does not supply the GPS packet stream used in GPS mode. Record PTP status and master
changes through the supported control interface and master logs, alongside the captured traffic;
absence of a GPS packet in this mode is not proof of capture loss.

### Measured arrival and offline fallback

Preserve native packet time, PCAP time and timestamp precision/type, raw IMU counters if present,
host monotonic read times, and periodic monotonic-to-realtime pairs. Fit offset **and** rate per
continuous segment. Separate fixed acquisition-to-delivery latency from variable queueing;
minimum-delay methods cannot identify an unknown constant sensor latency by themselves.

Use varied, non-periodic rotations about multiple axes at the start, during a safe pause, and at
the end. Compare LiDAR registration-derived angular rates with filtered gyro data. Fit on one
interval, assess on others, report confidence/multiple peaks, and reject weak excitation. A slow
single-axis wiggle neither observes the full mounting transform nor separates offset from filter
phase or clock drift. Periodic walking introduces ambiguous correlation peaks. LiDAR scan-rate
sampling also cannot establish sub-millisecond accuracy merely by interpolating a broad peak.

An unknown or changing timestamp error makes a capture timing-ineligible. Keep its raw data, but
do not admit it to speed aggregates by replacing uncertainty with zero. Existing car captures
remain useful for LiDAR-only experiments; they cannot retrospectively supply missing IMU timing.

## Derive the timing gate from motion

For a static world point at range `R`, small timing error `δt`, rig translation speed `v`, and
rig angular speed `ω` in radians/s, a conservative first-order bound is:

`e_position ≤ (|v| + R |ω|) |δt| = S |δt|`.

Here `ω` is rig motion, not the spinning rotor rate. For accelerating motion, evaluate the maximum
`S` over the uncertainty interval or include second-order terms. Lever-arm motion belongs in the
pose model. Moving-target distortion, range noise, extrinsic error, and odometry error have
additional budgets; an ego IMU does not make deskew exact.

Illustrative envelopes, **not measurements of either rig**:

| Regime                 |     R |        v |      ω | Error at 1 ms | Timing limit for 5 cm |
| ---------------------- | ----: | -------: | -----: | ------------: | --------------------: |
| Gentle walking/sway    |  20 m |  1.4 m/s |  30°/s |       1.19 cm |               4.21 ms |
| Fast backpack turn     |  50 m |  1.4 m/s | 180°/s |      15.85 cm |              0.315 ms |
| Bike turn at 25 km/h   |  50 m | 6.94 m/s |  90°/s |       8.55 cm |              0.585 ms |
| Rough bike, long range | 100 m | 6.94 m/s | 180°/s |      32.11 cm |              0.156 ms |

For two positions separated by `T`, independently bounded timing-induced position errors give
`e_speed ≤ (S1 u1 + S2 u2) / T`. This is conservative; a constant correlated offset can cancel
under steady motion, while changing yaw, acceleration, and jitter prevent that cancellation.
Do not use the bound as a complete tracker accuracy model. Mis-timed ego velocity also contributes
approximately `|a_ego| u`; angular acceleration and the rotating lever arm need corresponding
terms.

Propose a 5 cm timing-only geometry allowance and 0.2 m/s timing-only speed allowance. For equal
uncertainty `u`, require `u ≤ min(0.05 / S, 0.2 T / (S1 + S2))`. At `T=0.1 s`, the bike-turn case
allows about **117 µs**, but the rough-bike case allows only **31 µs**. Longer validated tracking
windows may relax the conservative differencing bound; they do not excuse a bad clock.

A **100 µs combined residual target** is a useful prototype goal for moderate motion, not a
universal pass mark. For equal endpoint bounds it gives 0.171 m/s timing error in the bike-turn
example, and 0.642 m/s in the rough-bike example. Reduce accepted range/motion, improve timing,
or reject observations when the dynamic gate is tighter.

| Contribution after corrections                             | Proposed absolute allocation |
| ---------------------------------------------------------- | ---------------------------: |
| LiDAR alignment to reference edge                          |                        15 µs |
| Wiring, buffer skew, and MCU edge sampling                 |                         5 µs |
| IMU event/sample association and timestamp quantisation    |                        15 µs |
| Residual IMU filter/acquisition-delay model                |                        40 µs |
| Clock fit/interpolation and between-edge drift             |                        20 µs |
| LiDAR packet/block/channel reconstruction and quantisation |                         5 µs |
| Arithmetic sum, not an independence/RSS assumption         |                   **100 µs** |

These allocations are engineering targets, not component guarantees. Cheap IMU filter-delay
variation may exceed its allocation. Whole-second ambiguity, missed samples, and clock resets
are invalid states, not small additive noise. Report median, p95, p99.9, observed maximum,
measurement uncertainty, and coverage. Percentiles alone cannot certify every accepted point;
the gate needs a conservative validated bound and an explicit policy for uncovered conditions.

## Loss, resets, and provenance

Use states `uninitialised`, `acquiring`, `locked`, `holdover`, `local`, and `invalid`, recording
the independent validity of relative timing, UTC epoch, and position. Loss of GNSS need not mean
immediate loss of relative timing, but continuing pulses do not prove continued lock either.

Bound holdover as `u(t) ≥ u0 + |relative_frequency_error| × elapsed`, with temperature and model
margin. An illustrative 20 ppm relative error accumulates 1.2 ms in a minute. Do not transfer
a constant-temperature LiDAR PTP specification to an MCU, a moving GNSS receiver, or the whole
outdoor rig. Reject when the uncertainty exceeds the motion gate; reacquisition starts a fresh
fit and requires a measured settling interval. Preserve steps rather than smoothing them away.

Record enough to reproduce every timestamp:

| Evidence                | Required content                                                                                                                                                                               |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Identity/configuration  | Sensor serial/model/firmware; angle/firetime files and hashes; return/trigger/rotation settings; board revisions; MCU firmware; IMU register dump; Pi/kernel/NIC and capture timestamp source  |
| Clock observations      | Raw PPS edge counters and sequence; raw serial messages/checksums and association; IMU counters/edges/sample IDs; native LiDAR date/microseconds; arrival times; lock and validity transitions |
| Mapping                 | Clock and boot IDs; time scale/epoch; offset/rate coefficients, fit windows, residuals, UTC conversion source/leap policy; holdover age and uncertainty                                        |
| Acquisition/calibration | Effective-time convention; filter/group-delay model and valid bandwidth/temperature; extrinsic transform/lever arm and uncertainty; calibration version                                        |
| Failures                | FIFO/USB/UDP loss, out-of-order records, missing pulses, late RMC, saturation, power brownouts, resets, rate changes, rejection reason and affected interval                                   |

Extend wrapping counters only with sequence continuity; distinguish wrap from reboot. LSM6DSOX's
32-bit 25 µs counter wraps after about 29.8 h; a 32-bit MCU counter at 1 MHz wraps after about
71.6 min. Segment on ambiguous gaps, native time jumps, host clock steps, receiver resets,
firmware changes, or uncertain leap handling. GPS week rollover and RMC's two-digit year require
a validated date window. Exercise UTC midnight, month/year transitions, and leap fixtures before
using `UnixNano`; it cannot independently represent a leap second. Keep continuous session time
for integration and a versioned mapping to UTC. Mark unsupported leap intervals invalid.

The PCAP bundle retains raw LiDAR and explicit IMU/timing UDP records, plus a versioned session
manifest. Repeat configuration and clock identity at file rotation so a five-minute file is not
orphaned. Verify that locally generated UDP actually reaches the capture interface; a send to the
Pi's own address may stay on loopback. Preserve per-interface timestamps if using PCAPNG and test
that replay dispatches sidecar streams separately from LiDAR packets. Port 10110 can carry Hesai
GPS packets or plain NMEA: payload type/source, not port alone, must select the decoder.

## Cost, power, and work

Public single-unit US prices checked September 23, 2026. Parts below are candidates; availability,
tax, and shipping must be checked at purchase time. No purchase is authorised by this design.

| Item                                                 |              Price | Basis                                                                    |
| ---------------------------------------------------- | -----------------: | ------------------------------------------------------------------------ |
| [SparkFun MAX-M10S GPS-18037][gnss-price]            |             $45.95 | Listed board price; PPS/UART exposed, no antenna included                |
| [Adafruit LSM6DSOX #4438][imu-price]                 |             $11.95 | Listed breakout price; use raw SPI and exposed interrupts                |
| [Raspberry Pi Pico][pico-price]                      |              $4.00 | Manufacturer starting price for original Pico; headers extra             |
| [SparkFun GPS-14986 SMA antenna, 3 m][antenna-price] |             $16.50 | Listed price; antenna and cable, intended for the MAX-M10S guide's setup |
| [SparkFun BOB-11189 RS232 transceiver][rs232-price]  |              $7.26 | Listed price; verify populated chip/revision, operate TTL side at 3.3 V  |
| [Pololu D24V5F5 #2843][power-price]                  |              $8.95 | Listed 5 V, 500 mA add-on supply; 5.1–36 V input, not a Pi/LiDAR supply  |
| Harness, JST mate/contacts, headers, USB lead        |             $10–20 | Design allowance, not a supplier quote                                   |
| PPS buffer/protection, fuse, decoupling              |              $5–10 | Design allowance                                                         |
| Enclosure, plate attachment, strain relief           |             $10–20 | Design allowance                                                         |
| Total                                                | **$119.61–144.61** | $94.61 listed parts plus $25–50 allowances                               |

Use the owned battery's measured voltage envelope, including full charge and transients, to
choose the converter; the proposed Pololu input limit is below the LiDAR's maximum. Feed Pico
VSYS from the regulated branch and the small 3.3 V loads from a checked regulator budget. Follow
the Pico power-path rules when USB is also connected. Fuse and protect reverse polarity; the
Pololu board does not provide reverse-voltage protection. A missing Hesai box or proprietary
cable is an unpriced dependency and can dominate these savings.

The [MAX-M10S manual][m10-manual] quotes 25 mW continuous tracking at module level; that excludes
the practical antenna, LEDs, and conversion losses. [ST][imu-ds] quotes 0.55 mA for the IMU in
combo high-performance mode, under its specified conditions. Its typical gyro noise density is
3.8 mdps/√Hz and acceleration noise is 70 µg/√Hz at ±2 g; those are not bias-stability guarantees.
Measure stationary bias/Allan behaviour, temperature sensitivity, and vibration sensitivity.

Estimate 0.3–0.8 W for the assembled add-on and reserve **1 W** until measured. This includes an
MCU allowance, active antenna, line driver, buffers, and supply losses; it is not a vendor power
rating. One watt for two hours costs 2 Wh. Battery runtime uses usable pack Wh divided by measured
whole-rig battery power, including Pi/storage, connection box, conversion losses, and start-up
peaks. The already-owned LiDAR, Pi, and battery are excluded from the add-on price, not from power.

The existing under-$50 IMU aspiration is met by the sensor board. The under-$500 LiDAR-plus-IMU
aspiration excludes timing accessories; if $500 is a hard complete-kit limit and a used LiDAR
costs $450, this baseline does **not** fit, even before Pi, battery, and mounting costs.

Allow roughly 2–4 engineering days for harness and firmware bring-up, 3–5 for capture/parser
contracts and offline mapping, and 3–5 for bench/field qualification, assuming suitable instruments
can be borrowed. These are planning estimates; filter-delay or firmware incompatibility can add
weeks. The GNSS-free variant adds epoch-emulation testing; hardware PTP adds NIC/driver and
IMU-to-PHC integration. Parts cost is the inexpensive part of this experiment.

## Reproducible qualification

Run the timing phase in the [backpack experiment][experiment] before attributing an improvement
to inertial deskew. Retain raw records, firmware/configuration hashes, analysis scripts, and
instrument model/calibration/uncertainty with the result. All steps are proposed, not performed.

1. **Inventory and electrical check.** Photograph labels and connector orientation, export versions
   and settings, and verify power/levels without connecting incompatible inputs. Scope PPS at the
   receiver, MCU, and LiDAR pins together; decode serial after the RS232 conversion. Check width,
   period, skew, message deadlines, and second labels, including midnight and deliberate ±1 s
   errors.
2. **Known timing edges.** Split a pulse generator's non-periodic test sequence into capture inputs
   and a reference logic analyser. Sweep edge phase against MCU sampling and IMU ODR. Use a
   separately measured reference; comparing two counters driven by the same error is insufficient.
   Check overflow/loss detection and held-out mapping residuals under simultaneous SPI, USB, CPU,
   Ethernet, and storage load. Do not inject test pulses into undocumented LiDAR pins.
3. **Native LiDAR timing.** Compare packet/block/channel times against the matching vendor decoder,
   single and dual returns, second boundaries, and known rotor/target geometry. Replay identical
   native packets with perturbed PCAP arrivals and at different speeds: reconstructed times must
   stay identical. Observe a moving target or instrumented rotation to test acquisition phase;
   a PPS trace alone cannot reveal the LiDAR's internal timestamp error.
4. **Sensor delay and extrinsics.** Put the rigid plate on an encoder-instrumented rotating
   fixture;
   timestamp encoder edges on the reference clock and observe fixed planes with LiDAR. Use varied
   axes and frequencies, reverse direction, and estimate extrinsics separately. Fit delay on one
   sequence and validate on another. Repeat IMU ODR/filter settings, temperature, and acceleration
   excitation. Report frequency-dependent residuals and bias, not just best correlation.
5. **Loss and discontinuities.** Remove antenna reception, then PPS, then serial independently for
   1, 10, and 60 s. Log which clocks continue; restore them and measure steps/settling. Reset MCU,
   IMU, receiver, and LiDAR separately. Test counter wraps, dropped batches, overnight epoch
   fixtures, and host realtime steps. Verify automatic quality rejection and no cross-reset fit.
6. **Field and power.** Capture a twenty-minute stand, walking turns, and repeat bike passes over
   smooth and rough surfaces. At start/end use a safe calibration area. Log battery-side watts,
   start-up peaks, usable Wh, temperatures, and packet/sample loss. Replay LiDAR-only, validated
   shared timing, and arrival-aligned variants on the same data. Inject ±0.1, ±0.5, ±1, and ±5 ms
   offsets and a 20 ppm drift to test sensitivity and gate response. Confirm speed with the
   [fixed-radar experiment](../../../data/experiments/try/route-speed-accuracy-vs-fixed-radar.md).

Report clock residuals, absolute-epoch correctness, effective acquisition-time uncertainty,
point-to-plane residuals versus range/motion, speed bias/scatter, and retained/rejected coverage
by timing state. Accept only the measured motion/range/temperature envelope. A radar agreement
result is useful corroboration, not an independent proof of microsecond timing.

Before assembly, unresolved facts are the physical LiDAR/box revision and firmware, available
connector/harness, actual Pi/NIC, battery voltage/current headroom, MAX-M10S firmware, IMU signal
path, and available reference instruments. Until those checks and the experiment pass, the
architecture is a costed prototype proposal and its accuracy is unverified.

[hesai]: https://www.hesaitech.com/wp-content/uploads/2025/02/Pandar40P_User_Manual_402-en-241220.pdf
[m10-manual]: https://content.u-blox.com/sites/default/files/MAX-M10S_IntegrationManual_UBX-20053088.pdf
[gnss-price]: https://www.sparkfun.com/sparkfun-gnss-receiver-breakout-max-m10s-qwiic.html
[imu-price]: https://www.adafruit.com/product/4438
[imu-ds]: https://www.st.com/resource/en/datasheet/lsm6dsox.pdf
[imu-an]: https://www.st.com/resource/en/application_note/an5272-lsm6dsox-alwayson-3d-accelerometer-and-3d-gyroscope-stmicroelectronics.pdf
[imu-pins]: https://learn.adafruit.com/lsm6dsox-and-ism330dhc-6-dof-imu/pinouts
[icm-ds]: https://product.tdk.com/system/files/dam/doc/product/sensor/mortion-inertial/imu/data_sheet/ds-000347-icm-42688-p-v1.6.pdf
[pico-price]: https://www.raspberrypi.com/products/raspberry-pi-pico/
[rp2040]: https://datasheets.raspberrypi.com/rp2040/rp2040-datasheet.pdf
[rp1]: https://datasheets.raspberrypi.com/rp1/rp1-peripherals.pdf
[antenna-price]: https://www.sparkfun.com/gps-gnss-magnetic-mount-antenna-3m-sma.html
[rs232-price]: https://www.sparkfun.com/sparkfun-transceiver-breakout-max3232.html
[power-price]: https://www.pololu.com/product/2843
[genet]: https://github.com/raspberrypi/linux/blob/rpi-6.12.y/drivers/net/ethernet/broadcom/genet/bcmgenet.c
[macb]: https://github.com/raspberrypi/linux/blob/rpi-6.12.y/drivers/net/ethernet/cadence/macb_ptp.c
[ptp4l]: https://www.linuxptp.org/documentation/ptp4l/
[phc2sys]: https://www.linuxptp.org/documentation/phc2sys/
[experiment]: ../../../data/experiments/try/backpack-motion-spectrum-and-stabiliser.md
