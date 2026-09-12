# Pi performance benchmark runbook

This runbook captures the `pi` row of the performance matrix defined in
[performance-regression-testing.md](performance-regression-testing.md). It is the only cell
that can answer "fast enough for a 10 Hz sensor" — a Mac or a hosted runner cannot stand in for
it, and no automation runs it yet.

- **Status:** Manual only. Every Pi baseline in the repository as of this writing was captured
  by hand following these steps.
- **Scope:** Capturing and refreshing `pi` baselines for the gated profiles (`full`,
  `l3-only`) against the committed golden capture (`kirk0.pcapng`).
- **Related:** [Performance regression testing](performance-regression-testing.md),
  [Remote host upgrade runbook](../../radar/operations/remote-host-upgrade-runbook.md) (same
  transfer conventions, different payload).

## Goal

Produce `baseline-kirk0-{full,l3-only}-pi.json`, captured on the actual deployment target, and
bring them back into the repository so `make test-perf-all` can gate future changes against
real device performance instead of asserting it.

## Scope

This runbook does not build or install a production `velocity-report` release. It builds one
tool, `lidar-bench`, and runs it against one committed capture. It does not touch the device's
running service, its database, or its systemd units.

## Guardrails

Stop and ask before continuing if any of these are true:

- The Pi is a live deployment currently monitoring traffic. Building and benchmarking is CPU
  and I/O intensive; do this on a spare or lab device, or during a confirmed maintenance
  window.
- The device has less than ~1 GB free — the toolchain, checkout, and capture together need
  headroom beyond the ~290 MB of golden captures themselves.
- `go build -tags pcap ./...` does not succeed on the dev machine first. Do not carry a broken
  build to the device to debug there; the Pi is slow for iteration and the point of this
  runbook is a clean measurement, not a build fix session.

## Inputs

Decide these before starting:

- `PI_HOST`: hostname or Tailscale name of the target device (e.g. `velocity-pi`).
- `PI_USER`: SSH user on the device.
- Which profiles to capture: at minimum both gated profiles, `full` and `l3-only`. Add
  `detect` only if a per-layer cost curve is wanted; it is not gated and not required.

## Prerequisites (on the device)

The project pins Go 1.26.5 in `go.mod`. Raspberry Pi OS's `apt` repository typically ships an
older toolchain, so install from the official tarball rather than `apt install golang`.

```bash
curl -LO https://go.dev/dl/go1.26.5.linux-arm64.tar.gz
```

```bash
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.26.5.linux-arm64.tar.gz
```

Add `/usr/local/go/bin` to `PATH` if it is not already (check `go version`).

`lidar-bench` needs the `pcap` build tag, which needs libpcap headers at build time:

```bash
sudo apt-get install -y libpcap-dev
```

If transferring via a git clone rather than `rsync` (see below), also install Git LFS — the
golden captures are LFS objects, and a plain clone without it fetches pointer files, not the
capture:

```bash
sudo apt-get install -y git git-lfs
```

## Transfer (from the dev machine)

Two options. Prefer direct transfer: it is faster and does not touch the 7+ GB of full commit
history a clone would otherwise pull.

### Direct transfer (recommended)

```bash
rsync -avz --progress --exclude .git /path/to/velocity.report/ "$PI_USER@$PI_HOST:~/velocity.report/"
```

### Shallow clone (if the Pi has GitHub access and the dev machine does not have direct access

to it)

Only two files in the repository are tracked in LFS (`kirk0.pcapng` and `lidar_20Hz.pcapng`,
~290 MB together), so a shallow clone stays reasonably light:

```bash
git clone --depth 1 --branch <branch> https://github.com/banshee-data/velocity.report.git
```

```bash
cd velocity.report && git lfs pull --include="internal/lidar/perf/pcap/kirk0.pcapng"
```

## Build (on the device)

```bash
cd ~/velocity.report && go build -tags pcap -o lidar-bench ./cmd/tools/lidar-bench
```

There is currently no cross-compiled or statically-linked `lidar-bench` artifact — unlike the
`velocity` server binary, which ships via the hermetic `build-radar-static` Docker pipeline.
Building on-device with `apt`'s `libpcap-dev` is the only path today. See
[Known gaps](#known-gaps) below.

## Confirm the matrix cell

`PERF_HOST_CLASS` auto-detects to `pi` on Linux/aarch64, so no override is normally needed.
Confirm it anyway before capturing — the whole point of the matrix is that a baseline
captured under the wrong label is worse than no baseline:

```bash
make perf-policy
```

Expect `host class: pi`, threshold `0.20`, repeats `5`. If the device reports something else
(a different architecture, or `CI=true` set in its environment from some other job), pass
`PERF_HOST_CLASS=pi` explicitly to every command below rather than proceeding on a
misdetected cell.

## Capture

```bash
make perf-baseline-all
```

This writes `internal/lidar/perf/baseline/baseline-kirk0-{full,l3-only}-pi.json`, each the
median of 5 runs. Capturing `detect` as well, if wanted:

```bash
make perf-baseline PROFILE=detect
```

### Read the result before trusting it

The frame budget check does not need a baseline and is the number that matters most on this
device specifically — it is the only cell that answers "fast enough":

- Open the written JSON and check `metrics.frame_budget.frames_over_pct`. Above
  `PERF_MAX_OVER_BUDGET_PCT` (default 1.0) means the pipeline is not keeping up with a 10 Hz
  sensor on this hardware, full stop, independent of any comparison.
- Check `repeat_spread` — if the 5 runs disagree widely, something else was running on the
  device during capture (background apt processes, thermal throttling, a live sensor feed).
  Recapture on an otherwise idle device; a baseline taken under load measures the load, not
  the code. This is a _measured_ risk, not a hypothetical one: two consecutive `l3-only` runs
  on a busy workstation this project's `mac` baseline differed by 3.5% in foreground point
  count alone, purely from settling drift under load.

## Bring the baselines back

```bash
scp "$PI_USER@$PI_HOST:~/velocity.report/internal/lidar/perf/baseline/baseline-kirk0-*-pi.json" internal/lidar/perf/baseline/
```

Commit them like any other change to the gate — per
[performance-regression-testing.md](performance-regression-testing.md#capturing-and-recapturing),
a baseline is a claim about what the code costs, and moving or adding one silently is a way to
retire evidence without saying so. Say in the commit message what device (model, storage,
cooling) produced it.

## Ongoing use

Once the `pi` baselines exist, `make test-perf-all` on the device gates future changes against
them at the class's own threshold (20%) rather than only the absolute frame budget:

```bash
make test-perf-all
```

Re-run this after any change to L3–L6 hot-path code, tuning defaults, or profile boundaries,
before believing a change is safe on the deployment target. A change that passes on `mac` or
`ci` has not been shown safe on the Pi; those cells answer a narrower question.

## Known gaps

- **No automation.** Nothing runs this on a schedule. The device must be reachable and free,
  and a person must run the steps above. Wiring this into CI would need a self-hosted runner
  attached to a Pi, or a webhook-triggered job — tracked in the backlog, not built.
- **No dependency-free `lidar-bench` binary.** `build-radar-static` (the hermetic
  zig+musl+libpcap.a Docker pipeline) produces the `velocity` server binary only. Extending it
  to also emit a statically-linked `lidar-bench` would remove the `apt install libpcap-dev`
  step and produce a binary with the same linking characteristics as what actually ships —
  closer to a production-representative benchmark than a native `go build` against the
  system's glibc and dynamically-linked libpcap. Small, bounded addition; not done because it
  was not the blocking gap. Tracked in the backlog.
- **One capture, one site.** `kirk0.pcapng` is the only real golden capture in the repository
  (`lidar_20Hz.pcapng` is degenerate — one frame). The "gold standard" file list in
  [performance-regression-testing.md](performance-regression-testing.md#gold-standard-pcap-files)
  names captures (`high-density.pcapng`, `pedestrian-focus.pcapng`, etc.) that do not exist;
  that section is aspirational. A Pi result from this runbook describes one site's traffic mix
  only.
