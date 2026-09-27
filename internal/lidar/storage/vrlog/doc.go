// Package vrlog is the VRLOG 1.x container: typed, checksummed L4
// observation evidence on disk, and the durable capture frontier over it. It
// is the codec, writer and reader for the l4bobserve foreground-complete
// frame records, independent of the visualiser's FrameBundle. The legacy 0.5
// FrameBundle layout (header.json, index.bin, frames/) belongs to
// l9endpoints/recorder; each refuses the other's directories by name and
// magic, never by guessing from payloads.
//
// A container is a directory of immutable objects and one pointer:
//
//	manifest                  root: capture, extraction, profile, limits, commit policy
//	chunks/00000000.chunk     sealed whole-frame records (one batch)
//	chunks/00000000.index     per-chunk index, rebuildable from the chunk
//	generations/00000000.gen  commit chain: what each generation commits
//	current                   the latest published generation (replaced atomically)
//	summary                   closing summary, committed by the close generation
//	lock, reserve             the writer's lock and failure-marker reserve
//	quarantine/               uncommitted objects recovery moved aside
//
// Every object starts with the same 16-byte preamble (root magic, object
// kind, container version) and holds records in a fixed 48-byte envelope
// with a CRC32C over envelope and payload. Payloads are velocity.recording.v1
// protobuf messages. The layout is specified in data/structures/VRLOG_FORMAT.md.
//
// Evidence is exactly what the validated commit chain commits. The L4
// callback appends each frame synchronously through one ordered Writer; an
// append means accepted. A committer goroutine closes the open batch at the
// declared age or byte threshold and publishes it as a generation in four
// steps (seal; synchronise and rename the chunk and index; write, rename and
// synchronise the generation; replace and synchronise the pointer), and only
// then announces the durable frontier. A Follower reads a live capture
// through the frontier the writer announces; Open reads a finished or
// interrupted one through the pointer and the chain; Recover makes an
// interrupted one consistent without rewriting what was committed.
//
// Trust is layered, and each layer is checked before the next is used. The
// preamble and envelope reject a foreign or damaged file before any protobuf
// is parsed. Lengths and counts are checked against the manifest's limits,
// themselves bounded by this package's hard ceilings, before allocation. A
// generation must follow its predecessor's digest and account; a committed
// index must be the object its generation named; a chunk's SHA-256 is
// verified before any record in it is returned, and an index, being derived,
// never overrides a chunk digest. Decoded records must satisfy the
// manifest's profile.
//
// Process-crash behaviour is tested by killing the writer between every
// publication step. What a power loss can additionally undo depends on
// whether the filesystem and device honour fsync; that evidence (G-OBS-CRASH
// on target hardware) is outstanding, and so is any live default.
package vrlog
