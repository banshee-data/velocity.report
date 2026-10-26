# Radar FFT evidence contract

- **Status:** Proposed; no FFT recorder, parser, or scene association is implemented.
- **Target:** v0.6.5
- **Plan:** [Radar FFT parsing and scene evidence](../../docs/plans/radar-fft-parsing-plan.md)
- **Evidence:** [FFT stream exploration](../../docs/radar/operations/fft-stream-exploration.md)

This contract describes the proposed records for OmniPreSense Doppler spectra,
their derived motion components, and their optional association with positioned
LiDAR tracks. Field names are design vocabulary, not a shipped API or migration.

## Measurement boundary

The OPS243-A measures radial velocity. An FFT bin has neither target range nor
bearing. A spectral component has no vehicle identity until an association supplies
evidence for one. Multiple vehicles can share a component, and one vehicle can
produce several components.

Keep acquisition evidence immutable. Processing versions may derive new components
or associations without replacing the original spectrum. Preserve unresolved and
ambiguous observations explicitly. Never insert FFT frames into the scalar speed
population used by radar transit statistics.

## Record relationships

| Record                | Identity and relationship                         | Purpose                                              |
| --------------------- | ------------------------------------------------- | ---------------------------------------------------- |
| Capture epoch         | Sensor, capture, connection epoch                 | Bounds continuity across reconnects and clock resets |
| Configuration epoch   | Capture epoch and effective source sequence       | Freezes decoding and bin calibration                 |
| Placement revision    | Sensor, site frame, validity interval             | Places the radar independently of the LiDAR          |
| Spectral frame        | Capture epoch and host source sequence            | Preserves one measured spectrum                      |
| Gap                   | Capture epoch and source interval                 | Records missing, rejected, or unavailable evidence   |
| Component observation | Processing run and spectral frame                 | Describes one grouped spectral lobe                  |
| Component track       | Processing run and ordered component observations | Describes temporal motion evidence                   |
| Scene association     | Processing run, component, LiDAR run and track    | Retains candidate matches and positional provenance  |

Use the platform's capture, deployment, run, and identifier conventions when they
land. These are logical record names: this proposal does not create a competing
capture catalogue or prescribe final table names.

## Capture and configuration epochs

| Field                                 | Type / unit                | Required semantics                                                        |
| ------------------------------------- | -------------------------- | ------------------------------------------------------------------------- |
| `schema_version`                      | Version                    | Version of this evidence contract                                         |
| `sensor_id`, `capture_id`, `epoch_id` | Identifiers                | Stable source identity; epoch changes at continuity boundaries            |
| `product`, `firmware_version`         | Text                       | Observed model and firmware, unknown explicitly                           |
| `configuration_id`                    | Identifier                 | Immutable configuration snapshot                                          |
| `effective_sequence`                  | Unsigned integer           | First host source sequence governed by this configuration                 |
| `domain`                              | Enum                       | Doppler speed; FMCW range is a distinct future domain                     |
| `sample_rate_hz`                      | Positive number            | ADC sample rate, distinct from spectrum arrival cadence                   |
| `sample_count`, `fft_size`            | Positive integers          | Acquired samples and transformed samples, including padding               |
| `bin_layout`                          | Enum                       | Full unshifted, full shifted, or explicitly documented subset             |
| `first_bin`, `bin_count`              | Integers                   | Native bin coverage; never infer missing bins as zero                     |
| `speed_per_bin_mps`                   | Optional positive number   | Observed firmware calibration with provenance                             |
| `carrier_frequency_hz`                | Optional positive number   | Alternative physical conversion input, with provenance                    |
| `direction_convention`                | Enum                       | Calibrated mapping to positive receding, negative approaching, or unknown |
| `window`, `scale`, `quantity`         | Descriptors                | Known window, scaling, and amplitude/power meaning; unknown allowed       |
| `filter_settings`                     | Configuration reference    | Device output filters and peak reporting settings                         |
| `source_payload_ref`                  | Digest and byte reference  | Original configuration replies, including malformed replies               |
| `processing_identity`                 | Build and parameter digest | Parser/converter used to interpret configuration                          |

Unknown calibration permits a bin-index plot but suppresses a physical speed axis.
Unknown direction permits signed frequency but suppresses inbound/outbound labels.
Do not silently apply the published sample's 0.0607 m/s scale to another setup.

## Spectral frame

| Field                                        | Type / unit                  | Required semantics                                                  |
| -------------------------------------------- | ---------------------------- | ------------------------------------------------------------------- |
| `frame_id`, `epoch_id`, `configuration_id`   | Identifiers                  | Immutable evidence and decoding references                          |
| `source_sequence`                            | Unsigned integer             | Host-assigned before fan-out; not proof of device completeness      |
| `device_sequence`                            | Optional integer             | Device sequence only when the firmware actually provides one        |
| `device_time_raw`                            | Optional native value        | Original clock value, including its native unit                     |
| `acquisition_start_ns`, `acquisition_end_ns` | Optional Unix ns             | Measured or justified acquisition interval                          |
| `received_at_ns`                             | Unix ns                      | Host receive time, never substituted for acquisition time           |
| `clock_segment_id`                           | Optional identifier          | Mapping from device time into the comparison clock                  |
| `time_uncertainty_ns`                        | Optional nonnegative integer | Unknown differs from zero                                           |
| `time_basis`                                 | Enum                         | Device mapped, reconstructed with provenance, or receive only       |
| `origin`                                     | Enum                         | Device FFT or locally computed from an identified I/Q pair          |
| `values`                                     | Numeric array                | Full available native bins; no display normalisation in evidence    |
| `quantity`, `value_encoding`                 | Descriptors                  | Magnitude, power, or documented complex representation              |
| `iq_pair_ref`                                | Optional evidence reference  | Both raw buffers when FFT is locally computed                       |
| `source_payload_ref`                         | Digest and byte reference    | Preserved original message                                          |
| `quality_flags`                              | Set                          | Invalid scale, timing unknown, clipping known, or incomplete source |

Array shape, finite values, declared lengths, supported sizes, and configured byte
limits are validated before interpretation. Reject incompatible representations
with a recorded reason. Do not guess that historical complex pairs and current
flat magnitude arrays have identical semantics.

An FFT magnitude frame has no recoverable phase. Recomputed I/Q spectra retain
their own window and scaling identity; they need not numerically reproduce the
firmware FFT. Pair separate I and Q messages only with established ordering and
continuity; a gap, reset, or unmatched buffer invalidates the pair.

## Gap and continuity record

| Field                                        | Type               | Required semantics                                                                                       |
| -------------------------------------------- | ------------------ | -------------------------------------------------------------------------------------------------------- |
| `epoch_id`, `sequence_start`, `sequence_end` | Source references  | Affected host sequence interval when known                                                               |
| `time_start_ns`, `time_end_ns`               | Optional times     | Known interval only; no invented duration                                                                |
| `reason`                                     | Enum               | Queue overflow, invalid payload, size limit, transport error, reconnect, clock reset, or storage failure |
| `known_lost_count`                           | Optional integer   | Known count, not an estimate of losses upstream of the host                                              |
| `completeness`                               | Enum               | Known complete, known incomplete, or unknown                                                             |
| `source_payload_ref`                         | Optional reference | Rejected bytes retained under the capture policy                                                         |

A host sequence detects losses after sequencing. UART/device-side losses without
a device counter may remain unknown. Missing intervals stay visible in replay
and waterfall plots. Never interpolate them into acquired spectra.

## Derived component records

| Field                                           | Type / unit         | Required semantics                                             |
| ----------------------------------------------- | ------------------- | -------------------------------------------------------------- |
| `component_id`, `frame_id`, `processing_run_id` | Identifiers         | Derivation identity and source evidence                        |
| `bin_start`, `bin_end`, `peak_bin`              | Bin coordinates     | Grouped lobe extent and peak, with wrap semantics stated       |
| `radial_speed_mps`, `speed_uncertainty_mps`     | Optional numbers    | Calibrated estimate and uncertainty method                     |
| `peak_value`, `integrated_value`                | Native quantity     | Comparable only under compatible quantity/scaling              |
| `noise_estimate`, `prominence`                  | Optional numbers    | Estimator and units recorded; not automatically calibrated SNR |
| `component_track_id`                            | Optional identifier | Temporal spectral identity, never a vehicle-count identifier   |
| `quality_flags`, `ambiguity`                    | Descriptors         | Merged, unresolved, crossing, or insufficient support          |
| `method_id`, `parameters_digest`                | References          | Versioned peak grouping and temporal tracking method           |

Keep both signs of Doppler frequency. Do not discard one sign merely because
the opposite sign has greater amplitude: opposing vehicles may occupy both.
Zero-padding refines interpolation, not the information available to resolve
neighbouring targets. Near-zero returns cannot enumerate stationary vehicles.

## Placement and scene association

A placement revision carries the radar origin and orientation in the site frame,
pose covariance when known, beam model and its source, validity interval, and
revision identifier. Use Gerardus's sensor-to-site pose conventions. Register the
radar separately or supply a calibrated rigid radar-to-LiDAR transform.

| Field                                                | Type / unit                    | Required semantics                                                    |
| ---------------------------------------------------- | ------------------------------ | --------------------------------------------------------------------- |
| `association_id`, `processing_run_id`                | Identifiers                    | Immutable association derivation                                      |
| `component_ref`                                      | Evidence reference             | Frame component or bounded component-track interval                   |
| `lidar_run_id`, `lidar_track_id`, `estimate_version` | Optional identifiers           | Exact positioned estimate being compared                              |
| `placement_revision`, `scene_model_revision`         | References                     | Radar geometry and external Gerardus model used                       |
| `clock_segment_id`, `comparison_time_ns`             | References                     | Qualified shared comparison instant                                   |
| `predicted_radial_speed_mps`, `residual_mps`         | Optional numbers               | LiDAR projection and observed-minus-predicted residual                |
| `state`                                              | Enum                           | Unassigned, candidate, associated, ambiguous, rejected, or predicted  |
| `score`, `score_method`                              | Optional value and method      | Association score; probability only after calibration                 |
| `position_site_m`, `position_covariance`             | Optional vector and covariance | Position supplied by identified LiDAR estimate or explicit prediction |
| `position_source`                                    | Enum                           | None, LiDAR observation, LiDAR estimate, or coasting prediction       |
| `candidate_refs`, `reason`                           | References and text            | Retained alternatives and suppression explanation                     |

Store candidates as separate rows so one component can be compatible with several
vehicles. Unknown position remains absent. Coasting predictions carry age and
growing uncertainty; they are not new positional observations.

For a stationary radar at r and a LiDAR track at p with velocity v in the same
site frame, predicted receding radial speed is v · (p − r) / |p − r|.
Suppress projection at zero separation or invalid clock/pose intervals. Compare
radial speed directly; dividing by a near-zero viewing-angle cosine is unstable.
Moving-radar compensation is outside this first contract.

## Persistence, replay, and map output

Use compressed, checksummed chunks for original payloads and spectra, with a
SQLite time/source index. Atomically publish completed chunks and recover an
interrupted final chunk using explicit incomplete status. Derived records reference
immutable source digests and processing identities. Retention removes payloads
only under an explicit policy and leaves their metadata marked unavailable.

At 1024 float32 bins and 10 spectra/s, values alone occupy 3.54 GB/day before
compression and metadata. This is sizing arithmetic, not a measured capture rate.
Select chunk encoding, retention, and default cadence after the hardware gate.

| Evidence available                          | Permitted map presentation                                         |
| ------------------------------------------- | ------------------------------------------------------------------ |
| Spectrum and placement                      | Sensor marker, approximate beam footprint, linked waterfall        |
| Spectrum, lanes, and explicit motion priors | Candidate regions labelled as inferred                             |
| Qualified LiDAR association                 | Track position with spectral evidence, residual, and uncertainty   |
| Unmatched component                         | Unresolved coverage region; no fabricated point or lane assignment |
| Lost LiDAR association                      | Bounded prediction, uncertainty and age visible, then unresolved   |

Keep spectra on the time–velocity axes. A heatmap painted across a beam is a
coverage or hypothesis display, not a measured spatial distribution. Per-track
associations and detailed spectra remain local operator evidence; public export
requires a separate reviewed contract under the existing privacy rules.

## Compatibility and unresolved decisions

The device parser must be qualified against actual model/firmware captures before
the representation is frozen. Unknown layouts are preserved but not interpreted.
No database migration, public endpoint, VRLOG extension, binary chunk encoding, or
default-on fusion is established by this document. The plan owns those delivery
decisions and their acceptance gates.
