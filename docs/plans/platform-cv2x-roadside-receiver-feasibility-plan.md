# C-V2X roadside receiver feasibility plan

Evaluate whether velocity.report can obtain independent, short-duration vehicle acceleration and braking measurements from direct 5.9 GHz C-V2X safety broadcasts for less than $500 in hardware. This is a **research and purchasing decision**, not approval to introduce radio-derived vehicle traces into the product or publish captures.

- **Status:** Proposed, research only; no verified live receiver, purchased hardware, or roadside V2X observations
- **Layers:** External sensor research, L8 behaviour validation, local offline tools; no runtime pipeline or storage contract changes authorised
- **Target:** Unscheduled feasibility spike; promote to a release only after technical, legal, privacy, and empirical gates pass
- **Companion plans:** [behaviour analytics](lidar-behaviour-analytics-plan.md), [state estimation](lidar-state-estimation-plan.md), [metrics-first research](platform-data-science-metrics-first-plan.md)
- **Related:** [project tenets](../../TENETS.md), [identifiability analysis](../platform/architecture/identifiability-analysis.md), [clock and time domains](lidar-clock-abstraction-and-time-domain-model-plan.md), [radar FFT research](radar-fft-parsing-plan.md), [repository documentation](../README.md)

## Goal and non-goals

Establish whether a receive-only hobbyist installation can detect real Basic Safety Messages (BSMs), reconstruct the SAE J2735 motion fields correctly, and add enough observations to validate LiDAR-derived longitudinal acceleration and braking events at a **single** roadside site.

Success requires a complete evidence chain, not merely an antenna receiving energy at 5.9 GHz. Show the effect on physics-derived measurements and the fraction of traffic actually represented. Compare against the option of a consented vehicle with an IMU/GNSS/OBD logger or public connected-vehicle sample data.

Not goals: intercepting cellular telematics; reconstructing commuting patterns; persistent vehicle/device identification; collecting VINs or licence plates; multi-site re-identification; implementing transmitters; and replacing LiDAR/radar with a biased sample of equipped cars. This plan does not approve unattended RF logging on public roads or changes to published schemas.

## Executive decisions and stopping rules

1. **Start at $0:** reproduce the [srsRAN C-V2X test vector](https://docs.srsran.com/projects/4g/en/latest/app_notes/source/cv2x/source/index.html) and separately parse [USDOT J2735 sample messages](https://github.com/usdot-fhwa-stol/j2735decoder). Document executable commit SHAs, environment, hashes, and outputs in a human-readable research report, not committed raw outputs.
2. **Two independent procurement tracks:** an open SDR front end and a supported, complete PC5 module/OBU. An inexpensive LGA modem with no firmware or host receive API is **not** a functioning receiver.
3. **No purchase merely because a board tunes to 5.9 GHz.** Require a named path from antenna to valid J2735 payload; a clock/timing plan; and a credible end-to-end test source.
4. **Gate any public-road interception and retention** on protocol-specific legal review and an explicit privacy design consistent with [Tenet 1](../../TENETS.md#1-privacy-above-all).
5. **Stop if deployment value is too low:** a working decoder with effectively no messages from ambient traffic is not a viable traffic-safety sensor. The standard's nominal 10 Hz BSM rate does not establish deployment penetration or capture yield.
6. **Do not schedule this against the 05x physical-measurement gates.** It is exploratory external reference work, not the basis for a release claim.

## Current state and corrections to earlier discussion

- The owned RTL2832U / R820T-class receiver is not directly usable: tuning stops far below 5.9 GHz and practical instantaneous bandwidth is only a few MHz. A pigtail or higher-gain antenna cannot fix either limitation. A downconverter does not fix the bandwidth deficit.
- The 2021 Chrysler Pacifica is **not a proven BSM transmitter**. Uconnect/cellular or cloud hazard-alert support does not imply direct SAE J2735 PC5 transmission. Its VIN/trim would need a documented radio capability check; use known-compatible authorised test equipment rather than assuming positive data.
- In the United States, 5.895–5.925 GHz is the ITS band. DSRC/IEEE 802.11p and LTE C-V2X PC5 are different PHY/MAC systems. The [FCC's 47 CFR 90.350](https://www.law.cornell.edu/cfr/text/47/90.350) requires incumbent DSRC stations to cease operations in that band no later than **14 December 2026**. Under-$100 Atheros/ath9k experiments are for legacy DSRC, **not** modern C-V2X.
- Nuand **original bladeRF x40 / x115** (BRF-40 / BRF-115) tune only to 3.8 GHz and cannot receive 5.9 GHz directly; do not conflate them with **bladeRF 2.0 micro xA4** (BRFM-XA4), which covers 47 MHz–6 GHz.
- The [published srsRAN application note](https://docs.srsran.com/projects/4g/en/latest/app_notes/source/cv2x/source/index.html) proves reception and decoding of LTE sidelink control and transport blocks from commercial C-V2X hardware with a **USRP B210 + GPSDO**. It **does not** demonstrate extraction of real J2735 braking telemetry from general traffic, HackRF, macOS, or a virtual machine.
- Neither a functional $50 AG15/SIM8100 board nor a complete sub-$500 PC5 receiver has been demonstrated. Earlier $10–18 Alibaba module pricing, used-board pricing, firmware availability, and prospective clocks were not traced to a dated supplier quotation.
- Sidelink reception is not identical to ordinary LTE cellular reception; neither an LTE dongle nor a Wi-Fi adapter becomes a PC5 receiver solely because the frequency overlaps.

## Measurement value and validity

| Field / metric            | SAE J2735 BSM source            | Validation use                                                    | Important caveat                                                                     |
| ------------------------- | ------------------------------- | ----------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Speed                     | `coreData.speed`                | Compare instantaneous LiDAR speed and differentiated acceleration | Check unavailable/reserved encodings and unit conversion                             |
| Longitudinal acceleration | `coreData.accelSet.long`        | Peak deceleration, onset, magnitude, duration                     | Vehicle-reported, possibly filtered or biased; not independent physical ground truth |
| Lateral acceleration      | `coreData.accelSet.lat`         | Cornering/turn dynamics                                           | Depends on axes/orientation and quality                                              |
| Yaw rate                  | `coreData.accelSet.yaw`         | Heading-change comparison                                         | Synchronisation and offset can dominate                                              |
| Brake status              | `coreData.brakes`               | Braking-event co-occurrence                                       | Status bits are not deceleration or pedal force; decode enumerations                 |
| Heading                   | `coreData.heading`              | Temporary association with a local LiDAR episode                  | Directional and positional ambiguity                                                 |
| Position                  | `coreData.lat`, `coreData.long` | Temporary alignment in an authorised test only                    | Highly identifying when combined with precise time                                   |
| Temporary message ID      | `coreData.id`                   | At most in-memory same-encounter continuity                       | Can support tracking; never persist/export                                           |

The BSM Part I core typically includes the fields above. Optional extensions, certificates and implementations vary. Validate J2735 edition, scaling, unavailable sentinels and coordinate reference in fixtures, not in an ad-hoc parser. Normal BSMs are not expected to broadcast VINs or licence plates; **do not assert their absence from every extended message, upper-layer packet, or arbitrary capture**.

Braking methodology: declare an event rule over the vehicle-reported longitudinal acceleration, report sample count, peak, duration and missing-frame fraction, and quantify time alignment error against LiDAR. Contrast a threshold on deceleration with the actual brake-applied bit: hard slowing can be regenerative, braking or coasting depending on vehicle; brake application does not establish emergency behaviour. Do not publish a judgement about an individual driver.

A single site cannot estimate a road-network trajectory or multi-site travel time from these messages. The key study question is **what fraction of all LiDAR-counted transits has an independently received, validated BSM observation**, stratified by traffic volume/time. Report equipped-subset bias and confidence intervals. A null receive result must distinguish absent transmissions, limited line of sight, antenna loss, receiver faults, channel mismatch and decoder failure.

## The signal chain and evidence boundaries

```text
Receive-only 5.9 GHz antenna (50 ohm, appropriate band and polarisation)
  -> low-loss pigtail / filters / optional LNA
  -> SDR or C-V2X modem RF input
  -> (SDR route) I/Q + sample timestamp + clock discipline
  -> GNSS time/reference acquisition and sidelink synchronisation
  -> LTE Release 14 PSCCH/PSSCH PHY decoding + MAC transport blocks
  -> link/network/security envelope: ITS / WSMP / IEEE 1609.2 as applicable
  -> SAE J2735 MessageFrame ASN.1 UPER decoder
  -> BSM validity + unit conversion + measurement provenance
  -> aggregate, site-local braking/acceleration statistics; no identifier storage
```

A dedicated PC5 module replaces much of the RF/baseband chain but may still require firmware, configured GNSS, ITS networking/security libraries and an SDK for received packet access. The presence of USB, UART or PCIe pads does **not** guarantee a message API. Security-layer parsing and trust checks may be separate tasks; receiving a signed message and verifying it are distinct capabilities.

[The srsRAN 4G C-V2X note](https://docs.srsran.com/projects/4g/en/latest/app_notes/source/cv2x/source/index.html) states Release 14 sidelink physical-layer support from release 20.04 and demonstrates `pssch_ue`. It describes a 5.920 GHz, 10 MHz (50 PRB), **11.52 MS/s** test using a B210 GPSDO. Its public transmit example carried random/synthetic contents; successful `/tmp/pssch.pcap` creation only establishes PHY/MAC transport block capture. Prove the subsequent message framing separately.

### Synchronisation and RF sample representation

A frequency-accurate **10 MHz reference** disciplines the RF/sample clock. A **1 PPS and GNSS time/epoch** establish absolute frame timing. These are related but not interchangeable. The documented srsRAN experiment explicitly acquires timing from GNSS; software-only recovery in unaided, asynchronous ambient PC5 reception is **unproven**, not a promised cheap substitute. Determine how the chosen radio, driver and decoder receive both clock and timestamp information, and whether a file can carry the necessary metadata.

At 11.52 MS/s complex I/Q, raw payload alone is approximately:

| Representation                      | Approximate sustained throughput | Approximate hourly volume |
| ----------------------------------- | -------------------------------: | ------------------------: |
| 8-bit I + 8-bit Q (complex int8)    |                       23.04 MB/s |                  82.94 GB |
| 16-bit I + 16-bit Q (complex int16) |                       46.08 MB/s |                 165.89 GB |
| 20 MS/s complex int8                |                          40 MB/s |                    144 GB |

Figures exclude metadata, filesystem overhead and compression and use decimal GB. Clock alignment, CPU utilisation, lost samples, USB transfer errors, format (int8 vs int16), and precise sample-rate conversion must all be measured. A MacBook Pro can process offline captures; **real-time VM USB pass-through is not yet qualified**. Prefer native macOS RF capture with offline decode under ARM Linux (or a proven streaming bridge) if USB scheduling causes loss.

## Hardware candidates, parts, and purchase gate

Indicative one-unit USD amounts below are **not quotes for a tested working decoder**. Vendor pages may change. Mark unsupported prices as TBD. All SDR totals exclude any GPSDO until a viable timing strategy is validated.

| Route                     | Manufacturer / part number                             | RF capability and interface                                     | Price evidence                                         | Assessment                                                                |
| ------------------------- | ------------------------------------------------------ | --------------------------------------------------------------- | ------------------------------------------------------ | ------------------------------------------------------------------------- |
| Already owned RTL-SDR     | RTL2832U, typically R820T/R828D tuner                  | Too low in frequency and bandwidth                              | Owned                                                  | Reject for direct PC5                                                     |
| HackRF Pro                | Great Scott Gadgets; SparkFun **WRL-29523**            | 100 kHz–6 GHz; up to 20 MS/s complex int8; USB; SMA clock input | SparkFun lists **$400**, availability to confirm       | Candidate for 10 MHz proof of concept; no known plug-and-play PC5 decoder |
| HackRF One                | Great Scott Gadgets; historical SparkFun **WRL-13001** | 1 MHz–6 GHz; 20 MS/s class; USB; 8-bit                          | New/used pricing TBD, older design                     | Candidate only if well-priced and tested                                  |
| bladeRF 2.0 micro xA4     | Nuand **BRFM-XA4**                                     | 47 MHz–6 GHz; up to 61.44 MS/s, 2x2; USB 3; reference-clock PLL | Nuand **$540** new                                     | Strong candidate if used below cap; total new >$500                       |
| bladeRF 2.0 micro xA9     | Nuand **BRFM-XA9**                                     | Same V2X front-end range, larger FPGA                           | Nuand **$860** new                                     | Exclude on cost                                                           |
| Original bladeRF x40/x115 | Nuand **BRF-40**, **BRF-115**                          | 300 MHz–3.8 GHz                                                 | Irrelevant                                             | Reject; cannot directly tune 5.9 GHz                                      |
| USRP B210 + GPSDO         | Ettus **B210**, GPSDO **783454-01** (confirm revision) | UHD SDR + integrated GPS discipline                             | Prior estimated $2,387 + $1,502, requote               | Published reproducibility reference, far over cap                         |
| Quectel module            | **AG15**                                               | Release 14 PC5 Band 47, LGA, host and GNSS interfaces           | Written quote and firmware terms needed                | Potentially lowest-cost supported modem                                   |
| Quectel newer module      | **AG18**                                               | PC5 product family, details variant-dependent                   | Written quote needed                                   | Ask whether replacing AG15 for new designs                                |
| SIMCom module             | **SIM8100**                                            | Historical Qualcomm MDM9150, PC5-only Release 14 LGA            | Written quote and firmware needed                      | Historical/reverse-engineering candidate                                  |
| Integrated PC5 OBU        | Unex **OBU-352UC**                                     | Packaged V2X receiver/OBU + ITS stack and SDK; FAKRA-Z          | Written evaluation quote needed                        | Higher-cost functionality reference                                       |
| Legacy DSRC               | Atheros **AR9280**, Compex **WLE200NX**                | 5.9 GHz 802.11p with specific patched Linux drivers; U.FL       | Used/retail total potentially <$100, **not validated** | Legacy-only, no C-V2X PC5                                                 |

**Vendor and implementation sources:** [HackRF Pro](https://hackrf.readthedocs.io/en/latest/hackrf_pro.html), [SparkFun catalogue](https://www.sparkfun.com/iot-wireless/rf.html), [Nuand BRFM-XA4](https://www.nuand.com/product/bladerf-xa4/), [Nuand original radio](https://www.nuand.com/bladeRF-brief.pdf), [Quectel automotive brochure](https://www.quectel.com/content/uploads/2025/03/Quectel_Product_Brochure_V8.3.pdf), [AG15 hardware design](https://forums.quectel.com/uploads/short-url/jMruIlww2pTpuzjwmDiUSEivvWW.pdf), [SIM8100](https://www.simcom.com/product/SIM8100.html), [SIMCom chipset roadmap](https://www.amstron.es/wp-content/uploads/2019/06/AMSTRON-SIMCom-Product-Roadmap_201906.pdf), [Unex OBU-352UC](https://unex.com.tw/en/docs/obu-352-information-sheet/).

### Candidate bills of materials

| HackRF Pro, SDR research route                           |               Estimate |
| -------------------------------------------------------- | ---------------------: |
| WRL-29523                                                | $400, observed listing |
| 5.9 GHz 50 ohm antenna (e.g. Taoglas TD.10.5113)         |       $20–35, estimate |
| Compatible SMA cable/adapters and short low-loss pigtail |       $10–25, estimate |
| MacBook Pro and Linux VM                                 |                  Owned |
| External timing system / receiver changes                |          TBD, excluded |
| **Pre-timing subtotal**                                  |           **$430–460** |

A used BRFM-XA4 only beats this if condition, shipping, reference inputs and actual **5.9 GHz reception** are confirmed. The Nuand xA4 ships with a USB 3 cable, but antenna, clock and enclosure choices require a separate bill of materials.

| Bare C-V2X module prototype                                              | Price status                                               |
| ------------------------------------------------------------------------ | ---------------------------------------------------------- |
| AG15 / AG18 / SIM8100, exact regional SKU and hardware revision          | Quote required                                             |
| Vendor EVB or custom PCB, socket/assembly for LGA                        | Quote required                                             |
| Correct power rails, start/reset pins, ESD, USB 2/3 or PCIe host wiring  | Design required                                            |
| 5.9 GHz RF matching, primary/diversity ports, matching coax and antennas | RF review required                                         |
| GNSS receiver/antenna, time signal, firmware provisioning                | Device-specific                                            |
| SDK/ITS stack licence, firmware image, support entitlement               | **Critical unknown**                                       |
| Board assembly, shipping, tools and measurement access                   | Quote required                                             |
| **Total**                                                                | **Not established; $50 complete prototype is speculative** |

For repeatable antenna swapping, standardise on **50 ohm SMA-female bulkhead** ports and SMA-male 5.85–5.925 GHz antennas. Verify actual connector sex, U.FL versus MHF4, and FAKRA-Z-to-SMA adapter fit per module. Taoglas **TD.10.5113** is an initial terminal dipole candidate; **TD.59.6H31** is an alternative outdoor/N-type candidate. At 5.9 GHz minimise lossy coax and check gain, VSWR, polarisation, placement, weather sealing, and exposure to strong adjacent-band sources.

No quoted price in this document is an accepted procurement authority.

## Research software and exact reproducibility targets

### R0: srsRAN PHY reproduction

- Repository: [srsran/srsRAN_4G](https://github.com/srsran/srsRAN_4G), AGPL-3.0.
- Primary evidence: [C-V2X Signalling application note](https://docs.srsran.com/projects/4g/en/latest/app_notes/source/cv2x/source/index.html).
- Example binary: `lib/examples/pssch_ue`.
- Sample vector: `lib/src/phy/phch/test/signal_sidelink_cmw500_f5.92e9_s11.52e6_50prb_0offset_1ms.dat`.
- Expected documented test-vector outcome: one decoded SCI, one decoded TB and a PCAP written to `/tmp/pssch.pcap`.
- Documented live invocation: `./lib/examples/pssch_ue -a clock=gpsdo`. **Do not run live until supported radio and timebase exist.**

First pin a tested Git commit, compiler, dependency versions and sample file hash. Reproduce with offline data on Linux before radio purchases. If the current repository no longer builds the historical example, identify the minimal historical tag and record the difference; **do not claim test success from the published screenshot**.

### R1: J2735 upper-layer fixture reproduction

- Repository: [usdot-fhwa-stol/j2735decoder](https://github.com/usdot-fhwa-stol/j2735decoder).
- ASN.1 package: [usdot-fhwa-stol/j2735_202409](https://github.com/usdot-fhwa-stol/j2735_202409).
- Inputs: bundled synthetic/sample J2735 UPER hex BSMs; 2016 versus 2024 schema compatibility should be tested.
- Independent unit fixtures must validate signed longitudinal acceleration scaling, negative values, reserved/unavailable sentinel values, brake bits, time and heading.
- Keep decoded test payloads that are _synthetic_ and hand-authored only. No real vehicle captures in Git.

### R2: transport blocks into BSMs

Show how decoded MAC transport blocks reach their intended ITS networking and security payloads. Determine applicable IEEE 1609.2, 1609.3/WSMP and regional networking conventions rather than assuming every transport block starts with J2735 UPER. Open-source [V2X Hub](https://github.com/usdot-fhwa-OPS/V2X-Hub) is another reference for ITS application integration. Demonstrate a valid end-to-end **synthetic** J2735 BSM frame before field recording.

### R3: software-defined radio adapter feasibility

Assess file-based sample conversion, clock epoch sidecar metadata, sample gaps, channel plan and format before porting live drivers. HackRF and bladeRF integrations are **not documented successes** of `pssch_ue`; replacing UHD does not remove the GPS timing dependency. Prototype offline file support before spending effort on a low-level real-time RF adapter.

### R4: operating-system and VM boundary

Use ARM64 Linux VM on Apple Silicon where dependency compatibility permits; otherwise use a matching Linux machine/container without claiming support. Run USB radio acquisition on a proven host if a VM drops samples. Record data loss, thermal throttling, CPU utilisation, wall-clock timing and decode performance. Raw packet/IQ artefacts remain outside the checkout under the [repository's no-results rule](../../AGENTS.md#keep-experiment-findings-in-git-and-raw-outputs-local).

## Vendor procurement questions and firmware feasibility

Ask manufacturer (Quectel, SIMCom) and authorised distributor in writing, for a **single-unit, receive-only PC5 Band 47 evaluation**:

1. Exact orderable module and EVB SKU; product lifecycle, region, modem chipset and firmware revision.
2. Unit/EVB price for 1, 10 and 100 units, shipping, MOQ, lead time, return/support policy.
3. Whether factory firmware boots as a standalone passive PC5 receiver without SIM or carrier service.
4. Whether the host can receive **arbitrary third-party BSM/ITS payload bytes**, with example API, tooling, OS, and sample code; clarify whether output is raw sidelink TB, secured ITS message or already decoded J2735.
5. Whether SDK, reference design, firmware, ITS stack, signing/security components or feature unlocks require NDA, fees or provisioning.
6. GNSS antenna/time inputs and whether the vendor solution supplies its own PPS/clock discipline.
7. RF connector or pad requirements, dual-antenna necessity, power rails, surge/ESD protection and certified antenna choices.
8. Whether passive monitoring has documented limits on channels, message types, security, or throughput.
9. A **receive demonstration** including one valid BSM, with unambiguous provenance; use consented test traffic.

Do **not** buy marketplace LGA samples without evidence of firmware image and legal entitlement, host API, boot and pinout, and one practical test path. A reverse-engineering task is appropriate only after acquiring a functional module and documented rights. Prioritise a **host-side USB/QMI/UART API investigation** over reverse-engineering modem DSP firmware; replacing proprietary Qualcomm baseband code would almost certainly exceed this research budget. Do not bypass authentication, licensing or access controls.

## Privacy, legal review and storage boundary

The project has stronger self-imposed restrictions than mere statutory minimums: [Tenet 1](../../TENETS.md#1-privacy-above-all) forbids personally identifiable data and traffic observation uploads. The [identifiability analysis](../platform/architecture/identifiability-analysis.md) explains why timestamp composition and repeated small-cohort publication can re-identify vehicles even without VINs.

- V2X BSMs commonly contain transient 4-byte message IDs and precise time/location. Radio/security headers and pseudonymous signing credentials can be linkable across messages. A temporary identifier is still an identifier, and hashing it without rotation does not anonymise it.
- Direct safety messages ordinarily do not contain a VIN or registration plate in their core, but arbitrary optional extensions and captured envelopes require inspection; never promise universal absence.
- Receive-only RF spectrum observation, decoding messages, storing payloads and publicly redistributing them are **four different legal decisions**. Hobbyist status grants no blanket exception. Review [18 USC §§ 2510–2511](https://uscode.house.gov/view.xhtml?path=/prelim@title18/part1/chapter119&edition=prelim), [47 USC § 605](https://uscode.house.gov/view.xhtml?req=granuleid:USC-prelim-title47-section605&num=0&edition=prelim), relevant [California Penal Code § 631](https://leginfo.legislature.ca.gov/faces/codes_displaySection.xhtml?lawCode=PEN&sectionNum=631.), and FCC rules with qualified counsel for actual intended collection and redistribution. This is not a legal opinion.
- Until review: use only synthetic vectors, owned/consented source transmissions, and receiver-only bench tests. In particular do not commit RF I/Q, arbitrary third-party PCAPs or identifiable J2735 captures to Git, Hugging Face, the site's public datasets, CI artefacts, telemetry, debug logs or issue attachments.
- Proposed privacy architecture for any later authorised trial: memory-only local packet decode, no persistent ID/certificate/VIN/coordinate/trajectory storage, local in-memory ephemeral episode joins if explicitly approved, short-lived high-resolution timing, output only non-identifying aggregate event counts and distributions with cohort suppression and documented aggregation windows.
- Do not silently add an RF side channel to LiDAR tracks. The data-contract review must explicitly ensure that the raw BSM message and provenance fields cannot leak into L4 VRLOG, later track packs, public Scenes or report exports.
- Even aggregated metrics require validity review: a handful of equipped vehicles can create a misleading, potentially revealing "braking rate". Suppress small cohorts and disclose equipped-vehicle denominator uncertainty.

## Six independent agent assignments and acceptance contracts

### A1: supported modem and vendor quotes (P0)

Deliver a comparison of AG15, AG18, SIM8100, Unex OBU-352UC and any currently orderable successor. Include exact SKU, EVB part number, quote evidence, firmware/SDK licence, host receive API, GNSS/antenna details, supplier, lead time and lifecycle. Send the nine questions above, preserving responses and dates in a written non-confidential report.

**Pass:** vendor documentation or an authorised demo that receives a real or consented BSM and exposes its application payload on Linux. **Fail:** silicon-only listing, no working firmware, no receive API, or unbounded licensing/support cost.

### A2: offline srsRAN reproduction (P0)

Pin the source revision and tools; reproduce `pssch_ue` on the published vector; report sample digest, decoded SCI/TB counts, PCAP inspection and exact command/environment. Audit PHY assumptions on GNSS epoch, reference frequency, input file framing and sample type. Determine whether a Linux ARM64 VM can reproduce it without claiming live RF support.

**Pass:** independently repeatable documented output from the vector. **Fail:** builds without a decodable vector or version-specific unsupported features.

### A3: <$500 SDR and timing qualification (P0)

Check WRL-29523, WRL-13001 and BRFM-XA4, explicitly rejecting BRF-40/115. Price an exact antenna/pigtail/host BOM. Confirm timebase interface, live I/Q recording of 11.52 MS/s for representative intervals, no overruns, input format, channel frequency and actionable GNSS timing strategy. Compare known-good offline and actual live captures where authorised.

**Pass:** sample-complete 5.920 GHz recordings with independently measured timing provenance and a decoder input path. **Fail:** insufficient usable channel bandwidth, no timing strategy, unknown sample loss or >$500 unavoidable initial hardware.

### A4: J2735 and behaviour metric fixtures (P1)

Reproduce USDOT BSM fixtures; add an independent validity/unit conversion contract and testable event definitions. Design a comparator for LiDAR acceleration, onset lag, peak error, brake bit co-occurrence, yaw/heading and uncertainty. Explicitly separate observation count from distinct equipped-vehicle count.

**Pass:** edition-pinned golden fixtures for valid, invalid, reserved and missing fields; numerical agreement with expected SI values and documentation of the event rule.

### A5: custom PCB / firmware feasibility and <$50 claim (P1)

For the best supported module, document pinout, power, USB, RF ports, GNSS, carrier dimensions and SDK; obtain PCB assembly costs for 1/10/100 units. Review host-driver interfaces first. State what can be done with shipped firmware, what requires vendor documents, and what is not feasible without controlled/proprietary code.

**Pass:** a full priced BOM and lawful operational firmware path with a credible one-unit and volume estimate. **Fail:** imaginary low-price part numbers or a design that assumes unavailable firmware.

### A6: observation yield, selection bias and privacy (P1, gated)

After legal and hardware approval, use consented transmissions or an approved receive-only field methodology. Record local receiver coverage and decode success statistics without keeping identifiable paths. Compare number of BSM-bearing passages to LiDAR observed passages with confidence intervals and traffic strata. Test whether measured braking improves any existing L8 validity result.

**Pass:** reproducible denominator-aware estimate of information gained, bias and uncertainty; written privacy/retention decision and explicit deployment recommendation. **Fail:** unmeasured penetration, inability to establish independent association without vehicle tracking, or any required policy exception not approved.

## Evidence gates and decision matrix

| Gate | Evidence required                                                                      | Purchase/release consequence                                |
| ---- | -------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| G0   | Valid USDOT synthetic J2735 BSM into typed SI fields                                   | No hardware needed                                          |
| G1   | srsRAN bundled sidelink I/Q vector to transport block and PCAP                         | No hardware needed                                          |
| G2   | Transport-block-to-J2735 synthetic end-to-end fixture, security envelope accounted for | Defines missing upper layers                                |
| G3   | Written modem SDK/EVB quote **or** validated SDR timing + sample format path           | Enables at-most-$500 candidate selection                    |
| G4   | Known-source live C-V2X BSM extraction with RF/timing and software versions recorded   | Proves hardware path, not traffic coverage                  |
| G5   | Legal authorisation and privacy design for a specific field trial                      | Required before observing third-party payloads persistently |
| G6   | Yield + bias + speed/acceleration comparison against LiDAR                             | Determines whether product integration is justified         |

A gate report must separate **Demonstrated** (locally reproduced), **Primary documented** (manufacturer/standards), **Third-party demonstrated** (published experiment), **Vendor claimed** (unverified quotation), **Estimated** (cost/volume model), and **Open** (no evidence). Do not promote lower-confidence evidence to implementation claims.

## Unresolved issues ledger

| ID  | Question                                                                                                                        | Owner             | Blocker           |
| --- | ------------------------------------------------------------------------------------------------------------------------------- | ----------------- | ----------------- |
| U1  | How does the proposed receiver recover GNSS frame epoch/PPS, and can it operate without an RF GPSDO?                            | A2/A3             | Live decoder      |
| U2  | Does `pssch_ue` at a pinned revision ingest arbitrary int8/int16 I/Q files and handle sample format conversions?                | A2/A3             | Low-cost SDR      |
| U3  | Can an AG15/AG18/SIM8100 ship with licensed firmware and receive arbitrary third-party PC5 messages over a documented host API? | A1                | Bare modem        |
| U4  | Is the complete first module EVB plus antennas, power, GNSS and SDK within the cap?                                             | A1/A5             | Budget            |
| U5  | Which IEEE 1609.2/1609.3, WSMP or other ITS layers are necessary for actual BSM parsing?                                        | A4                | End-to-end parser |
| U6  | Is the 2021 Pacifica a direct V2X transmitter? No affirmative evidence collected; identify verified test source.                | A1/A6             | Test design       |
| U7  | What percent of LiDAR passages broadcast usable messages at a chosen site?                                                      | A6                | Scientific value  |
| U8  | What permissions, privacy controls and retention restrictions apply to each capture type?                                       | A6 + legal review | Field eligibility |
| U9  | Does a Linux VM preserve repeatable live streaming/timestamps on the MacBook?                                                   | A2/A3             | Host integration  |
| U10 | Are advertised $10–18 module lots genuine, functional, provisionable and purchasable at quantity one?                           | A1/A5             | <$100 objective   |
| U11 | How to compare acceleration latency and noise when LiDAR observations and BSM report different physical quantities?             | A4/A6             | Validation        |
| U12 | Do capture antennas, RF front-end and coverage enable receiver operation without inconsistent false negatives?                  | A3/A6             | Yield             |

## Research ordering and ownership

1. **In parallel: A2** (offline PHY vector) and **A1** (vendor modem/firmware). Both can advance without hardware purchase.
2. **In parallel: A4** (upper layer and metrics) and **A3** (offline sample/clock/antenna BOM). Publish evidence and resolve U1/U2/U5.
3. After G0–G3, choose one **known-good, receive-only** candidate and seek explicit purchase approval. Do not conflate a research exploration with a deployable feature.
4. A5 refines manufacturing only when a modem is proven viable. A6 runs only after legal/privacy review and a known authorised test source.
5. Promote to product work only if the observed equipped sample adds reliable behaviour validation without breaching core tenets.

## Source and claims ledger

These references support documented hardware capabilities and public examples. They do not establish the unverified experiments in this plan.

| Source                                                                                                                          | Claim supported                                                                | Evidence class / remaining gap                        |
| ------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ | ----------------------------------------------------- |
| [srsRAN C-V2X note](https://docs.srsran.com/projects/4g/en/latest/app_notes/source/cv2x/source/index.html)                      | Release 14 PHY, B210 GPSDO, 5.920 GHz, 11.52 MS/s, known vector and decoded TB | Third-party demonstrated; not J2735 live braking      |
| [srsRAN 4G GitHub](https://github.com/srsran/srsRAN_4G)                                                                         | Accessible open-source PHY code, AGPL-3.0                                      | Source available; fresh build not reproduced          |
| [USDOT J2735 decoder](https://github.com/usdot-fhwa-stol/j2735decoder)                                                          | Python decoder with BSM/MAP/SPaT fixtures                                      | Primary source; local fixture test pending            |
| [USDOT 2024 ASN.1](https://github.com/usdot-fhwa-stol/j2735_202409)                                                             | J2735 edition-specific bindings                                                | Primary source; edition compatibility pending         |
| [USDOT V2X Hub](https://github.com/usdot-fhwa-OPS/V2X-Hub)                                                                      | Open ITS/application-layer reference                                           | Primary source; suitability pending                   |
| [HackRF Pro docs](https://hackrf.readthedocs.io/en/latest/hackrf_pro.html)                                                      | Frequency range, 20 MS/s, 8-bit, SMA clock                                     | Manufacturer documented; no PC5 demo                  |
| [SparkFun RF catalogue](https://www.sparkfun.com/iot-wireless/rf.html)                                                          | WRL-29523 $400 listing                                                         | Retail price, stock/total cost dynamic                |
| [Nuand xA4](https://www.nuand.com/product/bladerf-xa4/)                                                                         | BRFM-XA4 $540 and 47 MHz–6 GHz                                                 | Manufacturer documented, no live PC5 demo             |
| [Nuand original bladeRF brief](https://www.nuand.com/bladeRF-brief.pdf)                                                         | x40/x115 maximum 3.8 GHz                                                       | Manufacturer documented; reject originals             |
| [Quectel automotive brochure](https://www.quectel.com/content/uploads/2025/03/Quectel_Product_Brochure_V8.3.pdf)                | AG15/AG18 Band 47, interfaces, dimensions                                      | Manufacturer documented; host API unknown             |
| [Quectel AG15 hardware design](https://forums.quectel.com/uploads/short-url/jMruIlww2pTpuzjwmDiUSEivvWW.pdf)                    | LGA pads, power and host interfaces                                            | Manufacturer design; firmware not proven              |
| [SIMCom SIM8100](https://www.simcom.com/product/SIM8100.html)                                                                   | Product and dated hardware-design listing                                      | Manufacturer historical; orderability unknown         |
| [SIMCom roadmap](https://www.amstron.es/wp-content/uploads/2019/06/AMSTRON-SIMCom-Product-Roadmap_201906.pdf)                   | Historical MDM9150 / PC5-only concept                                          | Historic roadmap, not procurement evidence            |
| [Unex OBU-352UC](https://unex.com.tw/en/docs/obu-352-information-sheet/)                                                        | SDK-enabled integrated OBU reference                                           | Manufacturer documented; quote required               |
| [47 CFR 90.350](https://www.law.cornell.edu/cfr/text/47/90.350)                                                                 | Legacy DSRC cessation by 14 December 2026                                      | Regulatory text; other legal review pending           |
| [Connected Vehicle Pilot open data](https://data.transportation.gov/stories/s/Connected-Vehicle-Pilot-CVP-Open-Data/hr8h-ufhq/) | Source of external traffic safety messages                                     | Dataset reference; field provenance/rights to inspect |
| [Project tenets](../../TENETS.md)                                                                                               | Local-first, no identifying vehicle data or tracking                           | Binding project policy                                |
| [Identifiability analysis](../platform/architecture/identifiability-analysis.md)                                                | Site/time anonymity and composition risks                                      | Internal analysis; no safe blanket time rule          |

## Checklist

### Complete in this PR

- [x] Correct RF-band and original-versus-2.0 bladeRF model mismatch.
- [x] Record exact orderable hardware identifiers where documented and distinguish quote-required parts.
- [x] Capture PHY, clocking, ITS security/networking and J2735 decode as separate validation stages.
- [x] Specify cost ceilings, test gates, independent agent outputs, privacy constraints and source confidence.

### Outstanding research, not implemented by this PR

- [ ] A1: manufacturer quotes and proven receive API.
- [ ] A2: reproduce pinned offline srsRAN vector.
- [ ] A3: prove low-cost RF sample/timing path.
- [ ] A4: decode independent J2735 golden fixtures and benchmark metrics.
- [ ] A5: prove module firmware/PCB/BOM feasibility.
- [ ] A6: authorise and measure field yield, uncertainty and privacy.
- [ ] Select hardware only after G0–G3 and owner purchase approval.

### Accepted residuals

- No identified production Pacifica BSM broadcast.
- Current lack of a working under-$500 live C-V2X receiver.
- The risk that radio deployment yield is too low to justify any product integration.
