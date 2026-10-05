# PCAP read in place, October 2026

- **Status:** Complete. Reading PCAP packets on the replay's own goroutine cuts a replay's CPU by about 10 % and halves the Go scheduler's share, with byte-identical output. Neither it nor a batched reader shortens a replay: the per-frame pipeline, not packet ingest, is the critical path.
- **Scope:** B0 replays of three S2 sites on main against the in-place read and against a batched reader, 300 s scored, AB-BA order, on the Mac.
- **Related:** [PCAP analysis mode](pcap-analysis-mode.md), [October near-edge campaign](near-edge-campaign-2026-10.md).

The CPU profiles taken for the near-edge update-cost work showed that about half of every replay's
samples were Go scheduler work, not perception: about a quarter in one path, a channel receive
waking the goroutine blocked on sending (`runtime.recv` → `goready` → `wakep` →
`pthread_cond_signal`). The channel is gopacket's: `PacketSource.Packets()` reads on a goroutine
of its own and hands each packet over a 1,000-slot buffer. A replay consumes packets more slowly
than that goroutine reads them, so the buffer stays full and every receive wakes the reader, once
per packet.

`ReadPCAPFile` and `CountPCAPPackets` now read with `nextPacket`, which calls `NextPacket` on the
caller's goroutine and applies the reader goroutine's error rules exactly: temporary errors and
`EAGAIN` retry at once, EOF and the other unrecoverable errors end the source, anything else waits
5 ms and retries. The real-time replay path keeps the channel, since it sleeps between packets.

## Method

| Item     | Value                                                                                                                                                                    |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Builds   | main `901b250ee` and the in-place read `29b6df0c9` (main plus this change), each built from a clean checkout with its git SHA stamped                                    |
| Runs     | `lidar-state-estimation-baseline -case <site> -duration 300 -warmup 70`, default configuration (B0), no evidence database, captures read from the NAS, `-cpuprofile-dir` |
| Sites    | 3rd-folsom, pierce-haight and embarcadero-bryant: the October campaign's timing sites                                                                                    |
| Order    | Per site main, in place, in place, main, so a linear drift cancels                                                                                                       |
| Measures | `/usr/bin/time -l` user, system and wall seconds for the whole run (capture digests, first replay, repeat replay); the repeat replay's CPU profile                       |

## Findings

Means of the two runs per build. The scheduler share is the profile's flat time in
`pthread_cond_signal`, `usleep`, `pthread_cond_wait`, `pthread_kill`, `kevent`, `semasleep` and
`semawakeup`, as a share of the repeat replay's CPU.

| Site               | Build    | Wall (s) | CPU (s) | User / system (s) | Repeat CPU (s) | Scheduler share |
| ------------------ | -------- | -------: | ------: | ----------------- | -------------: | --------------: |
| 3rd-folsom         | main     |      198 |     221 | 195 / 26          |             97 |            59 % |
| 3rd-folsom         | in place |      199 |     198 | 181 / 16          |             87 |            34 % |
| pierce-haight      | main     |      259 |     231 | 201 / 29          |             99 |            59 % |
| pierce-haight      | in place |      260 |     206 | 188 / 18          |             89 |            34 % |
| embarcadero-bryant | main     |      239 |     220 | 193 / 27          |             94 |            57 % |
| embarcadero-bryant | in place |      230 |     197 | 180 / 17          |             85 |            34 % |

- **CPU falls by 10 % at every site,** for the whole run and for the repeat replay alone. The two
  runs of each build agree to within 1 %. System time falls by about 40 %: the wake-ups were
  system calls.
- **The scheduler's share falls from between 57 % and 59 % to 34 %.** What remains is the pipeline's
  other handoffs (frame assembly to the frame callback), which this change does not touch.
- **Wall time does not move.** The likely reason: the reader goroutine used to run beside the
  pipeline, and now its read sits on the pipeline's own goroutine, so the scheduler time saved is
  roughly the read time added to the critical path. This run does not isolate that. The one larger difference, at embarcadero-bryant, comes from one
  slow main run (252 s against 226 s), not from the build.
- **Byte-identical.** kirk0 (default replay, with evidence) and 3rd-folsom (300 s) on both builds
  wrote identical tracking baselines, first and repeat, identical frame counts and source digests,
  and on kirk0 identical rows in `lidar_observations` and `lidar_track_estimates`.

### A batched reader instead

If the read now sat on the replay's critical path, a reader goroutine that hands packets over in
batches of 256 would keep it in parallel while waking the replay once per batch. That was built
(`e81bd2164`), checked byte-identical on kirk0 and 3rd-folsom, and timed against main in the same
way, AB-BA at the same three sites.

| Site               | CPU, main → batched (s) | Repeat replay wall, main → batched (s) | Scheduler share, main → batched |
| ------------------ | ----------------------- | -------------------------------------- | ------------------------------- |
| 3rd-folsom         | 220 → 200 (0.91)        | 76 → 72 (0.96)                         | 59 % → 48 %                     |
| pierce-haight      | 230 → 209 (0.91)        | 105 → 105 (1.00)                       | 60 % → 47 %                     |
| embarcadero-bryant | 219 → 197 (0.90)        | 78 → 76 (0.98)                         | 58 % → 46 %                     |

It saves the same CPU as the in-place read and does not shorten a replay either; 3rd-folsom's
0.96 comes from one slow main run (79.5 s against 71.8 s). Its scheduler share stays higher, since
its goroutine still parks and wakes once per batch. It was reverted, and the simpler in-place read
kept.

## Interpretation

A single replay is no faster. What the change buys is CPU: about a tenth of every replay's, and
two fifths of its system time. That matters when replays run side by side, as a corpus campaign or
the worker queue does, and on a machine with fewer cores. It also removes a goroutine and a
1,000-packet buffer from every replay.

Taking the read onto the replay's goroutine and moving it off again both left wall time where it
was. So reading and parsing packets are not on a replay's critical path: they have slack. The
critical path is the frame callback, which runs L3 to L5 for each assembled frame on a goroutine
of its own. A faster single replay needs work there, in the background grid, clustering and
tracking per frame, not in packet ingest.

## Limitations

- One machine, 300 s windows, two runs per build and site, B0 only. The shadow and A2 run the same
  read path, so the saving should carry over, but they were not measured.
- Captures were read from the NAS over SMB. A local disk would change the read cost on the
  critical path, and so possibly the wall-time result.
- The live UDP path is unaffected and was not measured.

## Provenance

| Item           | Value                                                                                                                                                                                     |
| -------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Builds         | main `901b250eeb8baadbd73e5a855db5bf4f36822fe0`; in place `29b6df0c9346c43fe9e99fd021d6bf1cc2da82f3`; batched `e81bd2164` (reverted); all stamped                                         |
| Parameter hash | `sha256:fd35b0b28fc1…` (B0) on both builds                                                                                                                                                |
| Source digests | 3rd-folsom `1d19feceed93…`, pierce-haight `c9b3b0f8d588…`, embarcadero-bryant `3c55a2292c59…`                                                                                             |
| Raw outputs    | Run script, per-run logs, `time` output, CPU profiles and `throughput-summary.json` and `throughput-batch-summary.json` on the LiDAR volume under `velocity-campaign/pcap-read-20261005/` |
