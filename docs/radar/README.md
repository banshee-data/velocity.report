# Radar processing

Radar ingest, local evidence, and reporting documentation for velocity.report.

## Existing behaviour

- [Serial configuration](serial-config-quickref.md): device setup and runtime adoption.
- [Transit deduplication](architecture/transit-deduplication.md): scalar radar sessionisation.
- [CLI guide](cli-comprehensive-guide.md): server and operator commands.

## Proposed FFT work

FFT recording and scene association are not implemented.

- [Stream exploration](operations/fft-stream-exploration.md): parser references, public-capture findings, and measurement limits.
- [Evidence contract](../../data/structures/RADAR_FFT_FORMAT.md): spectra, configuration, gaps, components, and scene associations.
- [v0.6.5 plan](../plans/radar-fft-parsing-plan.md): six delivery items and acceptance gates.
