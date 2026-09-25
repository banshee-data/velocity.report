// Package vrlog is the VRLOG 1.x container: typed, checksummed L4
// observation evidence on disk. It is the codec and offline reader for the
// l4bobserve foreground-complete frame records, independent of the
// visualiser's FrameBundle. The legacy 0.5 FrameBundle layout (header.json,
// index.bin, frames/) belongs to l9endpoints/recorder; each refuses the
// other's directories by name and magic, never by guessing from payloads.
//
// A container is a directory of immutable objects:
//
//	manifest                 root: capture, extraction, profile, limits
//	chunks/00000000.chunk    sealed whole-frame records
//	chunks/00000000.index    per-chunk index, rebuildable from the chunk
//	summary                  optional; written by a clean close
//
// Every object starts with the same 16-byte preamble (root magic, object
// kind, container version) and holds records in a fixed 48-byte envelope
// with a CRC32C over envelope and payload. Payloads are velocity.recording.v1
// protobuf messages. The layout is specified in data/structures/VRLOG_FORMAT.md.
//
// Trust is layered, and each layer is checked before the next is used. The
// preamble and envelope reject a foreign or damaged file before any protobuf
// is parsed. Lengths and counts are checked against the manifest's limits,
// themselves bounded by this package's hard ceilings, before allocation. A
// chunk's SHA-256 is verified before any record in it is returned, and an
// index, being derived, never overrides a chunk digest. Decoded records must
// satisfy the manifest's profile.
//
// This is delivery phase 1 of the VRLOG plan: codec, sealed-chunk writer and
// offline reader. Commit generations, the durable frontier and recovery are
// phase 2; the reader's chunk catalogue is the seam where they attach.
package vrlog
