# Following review preparation

`make build-following-review` builds a stamped operator tool under `EXPERIMENT_TOOLS_DIR`.
It creates a new paired foreground recording, observation database, canonical following export
and one-minute native review files. It preserves all existing recordings and refuses an output
directory that already exists. All outputs belong on directly attached local storage.

```sh
following-review-prepare \
  -pcap /absolute/local/capture.pcapng -output /absolute/local/new-review \
  -site columbus-broadway -clip explicit-clip-id -binding explicit-parent-run-and-site-binding \
  -tuning config/experiments/weekend-following-baseline.json \
  -calibration /absolute/local/calibration.json -warmup 70 -port 2369 \
  -cohort-manifest config/cohorts/cohort-manifest.json \
  -annotation-root /absolute/local/annotations -eligibility-db /absolute/local/use-ledger.db
```

Calibration uses the `l4bobserve.Calibration` JSON contract, including sensor, source/destination
frames and the complete transform. Declare the actual coordinate relationship; the tool does
not infer it from a name or an accepted map pose. A known cohort source must match `-site`.
The policy purpose is fixed to annotation review, including for validation-cohort material;
this command has no tuning, candidate-scoring or experiment switches.
Its use-ledger entry records the tuning file's exact SHA-256 and the stamped producer build.

The warm-up prefix is processed but not recorded. Settling is required at its boundary, and
the rest of the source is recorded to EOF. Canonical evidence excludes samples outside the
recording's actual capture bounds. The complete database retains warm-up provenance.

Only `complete.json` signals that replay, analysis, canonical validation and review-file
publication all succeeded. It names every native file and its SHA-256. Open a matching minute
under **Inspector → Following / headway / DRAC evidence**. Review adjacent minutes when an
original sample interval crosses a file boundary, and save a separate audit for each file.
The canonical export and original recordings remain available for independent verification.

These are operator review artefacts, not physically qualified measurements. See the
[workflow](../../../docs/ui/visualiser/following-audit-workflow.md) and
[five-site readiness report](../../../docs/ui/visualiser/mesh-and-following-readiness-2026-11.md).
