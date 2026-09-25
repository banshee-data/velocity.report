// Package l4bobserve owns track-independent L4 detection evidence. It does not
// select object faces, correct positions, or assign detections to tracks.
//
// Two record families share its identities (SourceID, CalibrationID,
// ObservationID) and differ in what they can prove. Each declares an evidence
// Profile: a named CapabilitySet. A reader states the capabilities its
// experiment needs and calls Require; the typed MissingCapabilitiesError names
// everything absent, so a request for full evidence fails instead of quietly
// running on less.
//
//   - Record / DetectionObservation (profile reduced-cluster-sample) is the
//     schema-1 JSON record stored in SQLite: one accepted cluster, its summary
//     and a capped, content-seeded retained sample. There are no frame
//     records, empty frames, gaps, membership or lineage. It remains readable
//     and is the fidelity oracle for older corpora.
//
//   - FrameRecord (profile foreground-complete) is one record per frame the
//     extractor received, in a dense source sequence, including empty,
//     unsettled, suppressed and failed frames. Its retained domain is every
//     L3-labelled foreground return before ground/height filtering, voxel
//     reduction and the DBSCAN input cap, in source order, with float64 XYZ
//     exactly as L4 computed them, acquisition-time offsets, intensity,
//     channel, source ordinal and packet locator. Clusters refer to sorted,
//     unique indices in that domain; the unassigned points complete an exact
//     partition, each with its rejection reason. Completeness, processing
//     disposition and payload availability are separate fields, and stage
//     counts distinguish unknown from zero. GapRecord makes missing intervals
//     explicit; StreamValidator checks order, coverage and each record.
//
// ExtractionBuilder is the in-memory producer. The pipeline reports each stage
// to a FrameDraft; lineage is carried by a source ordinal stamped on each
// l4perception.WorldPoint before the first filter, so every stage's output is
// traced back to the domain rather than matched by coordinates. The package is
// free of storage: a durable binary container maps these types later.
package l4bobserve
