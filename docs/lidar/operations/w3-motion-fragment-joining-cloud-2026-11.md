# W3 motion-supported fragment joining

- **Status:** Default-off implementation and cloud screen; W3 qualification remains open
- **Spec:** [l4clus0008](../../../data/experiments/l4clus/l4clus0008.md)
- **Layers:** L4 Perception, pipeline with prior L5 snapshots

## Question and implementation

Can a recent, well-constrained moving body's predicted footprint support joining
current fragments without enlarging DBSCAN's neighbourhood or recovering noise?
This follows the failed minimum-density and fixed-radius alternatives.

The optional `l4.dbscan_xy_v1.fragment_join` block is absent from defaults.
Startup and offline replay pass the same settings to the callback. Before current
association, a confirmed rigid vehicle with at least eight observations and
observed support is projected to one common capture time. The configured bounds
are 0.2 s age, 2 m/s minimum speed and 0.75 m maximum position standard deviation.
The position covariance uses the full CV projection, the tracker's current process-noise law (including the coupled option), and its largest eigenvalue;
weak prior directions are not replaced by a fixed isotropic certainty.

L4 considers only accepted components and the complete post-cap member stream.
Each member must be inside one predicted footprint, with 0.15 m margin. Any
intersection with a second eligible body makes ownership ambiguous. Components
within a 2 m gap may join under that one body. Existing shape filters still apply;
a rejected group restores all its original components, including their IDs.
Noise and already-rejected components are not recovered. The same partition
feeds detections, immutable observation membership and association. Source
ordinals and processed members survive; evidence samples remain capped separately.

`fragment_join` in the corpus summary counts clustered frames, frames with joins
and components joined, including warm-up. Counts are deterministic diagnostics,
not physical accuracy. No default, wider input cap, minimum density or foreground
classifier changes are made.

## Validation and evidence

Synthetic tests cover genuine separated fragments, ambiguous neighbouring bodies,
missing membership, excessive gaps, non-finite/duplicate priors and shape-filter
rejection. They verify no input mutation, canonical joined identity, full return
lineage, noise preservation and observation-tap consistency. Prior tests refuse
stale, tentative, coasting, stopped, pedestrian and uncertain tracks.

Real-pcap results and the predeclared rule are recorded after the runs. Missing
labels or independent references mean insufficient evidence, not acceptance.
No physical, corpus-wide or Pi throughput claim follows from synthetic joining.

## Provenance

Base `1ec1ccea5947ec8f72d155b337ed86d066f342c4`.
The pre-run rule was committed before any candidate replay. Kirk0 source SHA-256
`2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a`.
Dataset revision `37ef1befc1876ad860800beea1bbeaa6c9078da9`.
Raw evidence and logs stay outside Git.

The first development launch was stopped before reading candidate results when
review found that the hint covariance projection omitted process noise. The
implementation now reads the actual tracker noise configuration, including
through instrumentation, rejects priors beyond `MaxPredictDt`, and has a
boundary regression that the noise-free calculation would incorrectly admit.
The final screen uses fresh outputs and a single corrected build for both arms;
the stopped launch supplies no acceptance evidence.
