// Package labelset stores reference labels against the returns of a capture
// rather than the points of one run's export.
//
// An annotation pack cites sample_id + point_index, and its version 1 sidecar
// held its labels that way. The pack is a run's foreground with the run's frame numbering, so the same
// capture replayed again, even with the same settings, gives a pack with a
// different digest and the labels no longer apply. A label set instead cites
// returns by (packet key, block, channel). The packet key is the first 64 bits
// of SHA-256 over the UDP payload: the bytes the sensor sent, which survive
// clipping, merging, PCAP/PCAPNG conversion and re-stamping unchanged. A label
// set therefore applies to every run of a capture and to every file that holds
// its packets, which Bind verifies rather than assumes.
//
// Annotation stays downstream of foreground separation. Every mask names the
// foreground extraction (its Domain) it was made on, and the label set records
// that extraction's returns window by window, so a scorer can tell a return the
// labeller never saw from one it saw and left out.
//
// History is a journal of deltas, one line per save, rather than a snapshot
// per save. The head files are the current state in one canonical encoding,
// and the state digest over them pins a revision. Only the head is published;
// the journal is local working history.
package labelset
