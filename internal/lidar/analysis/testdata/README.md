# Heading regression fixture

`baf20f02-heading.json` retains frames 1000 through 1199 and the two named track
outputs from the reported split-car case. It contains 200 frame records, 123
co-publications, and the frame-1026 collapse to approximately 0.11 by 0.08 metres.
Deleted publications remain present so tests can verify their exclusion.

The source PCAP, VRLOG header, and execution configuration are identified by hashes.
Raw packets and point clouds are not copied into the repository. Re-export using
`go run ./cmd/tools/heading-fixture` with the original VRLOG and PCAP paths; the
tool writes JSON to standard output and never changes either source.

These are recorded decisions, not physical pose ground truth. The same-object
identity is user-reported and has not been independently adjudicated. Co-publication
does not establish duplicate identity. Use this fixture for analysis regressions;
use the source PCAP to evaluate a changed perception pipeline. The old guard
path's measured numbers are not acceptance targets for the new axis path.

`heading-d2-ab-evidence.json` is a frozen diagnostic report from the later warmed A/B,
not an additional labelled fixture or a golden numerical acceptance target. It preserves
source/configuration/build provenance, missing terminal evidence, and repeat comparisons.
Capture timestamps in both files are signed 64-bit nanoseconds: do not round-trip them
through JavaScript numbers.
