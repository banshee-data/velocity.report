# PCAP read in place, October 2026

- **Status:** Complete. Reading PCAP packets on the replay's own goroutine cuts a replay's CPU by about 10 % and halves the Go scheduler's share, with byte-identical output, and shortens a replay from local disk by about 3 %. Reading captures from the NAS rather than the internal SSD makes a replay twice as long, which outweighs either.
- **Scope:** B0 replays of three S2 sites on main against the in-place read and a batched reader, captures from the NAS; and 3rd-folsom from the internal SSD against the NAS, and the three builds from the SSD. 300 s scored, balanced order, on the Mac.
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
- **From the NAS, wall time does not move.** Reading from the NAS is what sets a replay's wall
  time there, so a change to the read path cannot show (see
  [Where the captures are read from](#where-the-captures-are-read-from)). The one larger difference,
  at embarcadero-bryant, comes from one slow main run (252 s against 226 s), not from the build.
- **Byte-identical.** kirk0 (default replay, with evidence) and 3rd-folsom (300 s) on both builds
  wrote identical tracking baselines, first and repeat, identical frame counts and source digests,
  and on kirk0 identical rows in `lidar_observations` and `lidar_track_estimates`.

### A batched reader instead

A reader goroutine that hands packets over in batches of 256 would keep the read beside the replay
while waking it once per batch. That was built
(`e81bd2164`), checked byte-identical on kirk0 and 3rd-folsom, and timed against main in the same
way, AB-BA at the same three sites.

| Site               | CPU, main → batched (s) | Repeat replay wall, main → batched (s) | Scheduler share, main → batched |
| ------------------ | ----------------------- | -------------------------------------- | ------------------------------- |
| 3rd-folsom         | 220 → 200 (0.91)        | 76 → 72 (0.96)                         | 59 % → 48 %                     |
| pierce-haight      | 230 → 209 (0.91)        | 105 → 105 (1.00)                       | 60 % → 47 %                     |
| embarcadero-bryant | 219 → 197 (0.90)        | 78 → 76 (0.98)                         | 58 % → 46 %                     |

From the NAS it saves the same CPU as the in-place read and does not shorten a replay either;
3rd-folsom's 0.96 comes from one slow main run (79.5 s against 71.8 s). Its scheduler share stays
higher, since its goroutine still parks and wakes once per batch.

### Where the captures are read from

All the runs above read their captures from the NAS. A sampled replay spent much of its time with
threads blocked in `read`, although the NAS delivers 61 to 81 MB/s to `dd` at 4 KiB to 1 MiB reads.
So 3rd-folsom's four captures were copied to the internal SSD (byte-identical) and the same main
build replayed from each place, local, NAS, NAS, local:

| Captures from | Whole run (s) | Repeat replay wall (s) | Repeat replay CPU (s) |
| ------------- | ------------: | ---------------------: | --------------------: |
| Internal SSD  |    88.9, 87.2 |             41.9, 41.9 |            95.6, 95.6 |
| NAS           |  191.5, 181.4 |             68.9, 70.6 |            95.9, 96.2 |

The work is the same and the replay from the SSD finishes in about half the time: it keeps 2.3
cores busy against 1.4 from the NAS. Then the three builds from the SSD, in the order main, in
place, batched, batched, in place, main:

| Build    | Whole run (s) | Repeat replay wall (s) | Whole-run CPU (s) |
| -------- | ------------: | ---------------------: | ----------------: |
| main     |    87.7, 88.0 |             41.7, 41.8 |      211.4, 211.4 |
| in place |    84.9, 84.8 |             40.7, 40.7 |      188.8, 189.4 |
| batched  |    83.5, 83.1 |             40.5, 39.8 |      192.6, 192.3 |

From local disk both changes do shorten a replay: the in-place read by about 3 %, the batched reader
by about 4 %, for 11 % and 9 % less CPU. The batched reader's extra percent of wall time costs a
goroutine and a percent or two more CPU, so it was reverted and the simpler in-place read kept.

## Interpretation

Reading packets in place is a small, safe gain: about a tenth of every replay's CPU, two fifths of
its system time, and about 3 % of its wall time when the captures are local. It also removes a
goroutine and a 1,000-packet buffer from every replay. The CPU matters most when replays run side
by side, as a corpus campaign or the worker queue does.

Where the captures live matters far more. From the NAS a replay takes twice as long as from the
internal SSD, for the same work. For a corpus run on this Mac, copying each case's captures to the
internal disk first (and deleting them after) would roughly halve the wall time; the S2 first
segments are about 0.7 GB per case, a whole case 3 to 6 GB. The USB LiDAR volume is no substitute:
it read at 40 MB/s, below the NAS.

## Limitations

- One machine, 300 s windows, two runs per build and site, B0 only. The shadow and A2 run the same
  read path, so the saving should carry over, but they were not measured.
- The SSD comparison is one site with two runs per condition; the size of the NAS penalty will
  vary with the network and the NAS's load.
- The live UDP path is unaffected and was not measured.

## Provenance

| Item           | Value                                                                                                                                                                                     |
| -------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Builds         | main `901b250eeb8baadbd73e5a855db5bf4f36822fe0`; in place `29b6df0c9346c43fe9e99fd021d6bf1cc2da82f3`; batched `e81bd2164` (reverted); all stamped                                         |
| Parameter hash | `sha256:fd35b0b28fc1…` (B0) on both builds                                                                                                                                                |
| Source digests | 3rd-folsom `1d19feceed93…`, pierce-haight `c9b3b0f8d588…`, embarcadero-bryant `3c55a2292c59…`                                                                                             |
| Raw outputs    | Run script, per-run logs, `time` output, CPU profiles and `throughput-summary.json` and `throughput-batch-summary.json` on the LiDAR volume under `velocity-campaign/pcap-read-20261005/` |
