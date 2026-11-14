# arrow-worker experiment harness

The implementation and [operator runbook](../../docs/platform/operations/experiment-harness.md)
cover the v0.5.2 mechanical runner. These files prepare a systemd installation;
installing and enabling the timer and the week-long NAS soak remain deployment work.

Use a dedicated clone with local `main` checked out. Install a stamped `velocity`
binary built through `make build-velocity`, Go at `go.mod`'s exact version, libpcap development
headers and Git. The systemd unit sets `GOTOOLCHAIN=local`, so the distribution's Go is not
enough when it is older: install the toolchain the module needs (on arrow-worker,
`/usr/local/go-1.27.1`, copied from the module cache's `golang.org/toolchain@…` directory) and
put its `bin` first on the unit's `PATH`. Refresh it whenever `go.mod` raises its version. Populate the service user's Go module cache while connected. The
per-commit tool build uses `GOPROXY=off` and `GOTOOLCHAIN=local`; it never downloads
missing dependencies during a replay. The tool build uses embed stubs and does not
need Node at runtime. Building the complete `velocity` distribution still needs the
normal documentation and Typst build dependencies.

Installation, by an administrator on arrow-worker:

1. Create `/etc/sysusers.d` if the image lacks it, install `velocity-harness.sysusers` as
   `/etc/sysusers.d/velocity-harness.conf`, then run `systemd-sysusers`. If `/var` is small
   (arrow-worker's is 2.3 GB), make `/var/lib/velocity-harness` a bind mount of a directory
   on a larger disk before anything else is created there.
2. With the captures and results mounts available, install
   `velocity-harness.tmpfiles` as `/etc/tmpfiles.d/velocity-harness.conf`, then
   run `systemd-tmpfiles --create`. Check the mounts before creating directories. On the NAS
   share the `fchmod` of `/mnt/results/velocity-harness` fails with EPERM and tmpfiles exits
   73; the directory still exists and that is expected.
3. Place the dedicated clone at `/var/lib/velocity-harness/repo`, owned by
   `velocity-harness`, with local `main` checked out and a read-capable `origin`. Copy the
   Go module cache (`<GOPATH>/pkg/mod`) to `/var/lib/velocity-harness/cache/go/pkg/mod`
   while connected: the builds run with `GOPROXY=off`. Install the stamped `velocity`
   at `/usr/local/bin/velocity`.
4. Copy `arrow-worker.example.json` to `/var/lib/velocity-harness/host.json` and
   check every path, corpus and capability. The example advertises S2 captures;
   the example also maps kirk0 at the capture root and advertises `pcap:kirk`.
   Verify those reads as the service user. Corpus-specific `capture_root` replaces both
   legacy path fields, so Kirk can sit beside `s2` without moving or copying captures.
   On macOS set it to the corresponding `/Volumes/...` directory in the local host profile.
   Labels and frozen scoring references remain separate prerequisites; advertise
   `labels:kirk` only when they are installed and verified. The NAS
   refuses `O_EXCL` and `chmod`, so the example keeps `evidence_root` there with
   `stage_attempts` and a local `build_root` ([runbook](../../docs/platform/operations/experiment-harness.md)).
5. Create the eligibility inputs the unit's `VELOCITY_*` variables name. The use ledger is a
   database at schema 70 or later: as the service user run
   `velocity data migrate --db-path /var/lib/velocity-harness/eligibility.db up` (without
   `--db-path` the command creates `sensor_data.db` in the current directory). The cohort
   manifest is the checked-in `config/cohorts/cohort-manifest.json`; development replays are
   refused for any capture it does not assign, and the S2 sites are unassigned until the
   owner assigns them.
6. To publish, add a `publish` object shaped like `github-app.example.json` to
   the host profile. Install the repository-scoped App with contents and pull-request
   write, checks and commit-status read permissions. Give its RSA private key mode
   `0600`, owned by `velocity-harness`. Replace both IDs and the bot identity.
   No key, token or configured host profile belongs in Git.
7. Run `velocity experiments tick -host /var/lib/velocity-harness/host.json
-root /var/lib/velocity-harness/repo -dry-run` as the service user, then run one
   approved fixture with `-offline`. Review the output and its bounded note.
8. After the deployment review, install the service and timer in
   `/etc/systemd/system/`, run `systemctl daemon-reload`, and enable the timer
   with `systemctl enable --now velocity-harness.timer`.
9. Perform the acceptance soak: kill a replay through systemd, restart it, verify
   that done units retain their attempt numbers, then follow one approved spec
   through App-created draft, green checks, clear reviews and squash merge.
   Observe the timer for a week before declaring H5 complete.

The service opens no port, has no sudo access, reads captures read-only and kills
its entire process group on stop. It can write its own clone, cache and evidence
paths. App scope and the path guard constrain publication to the one repository's
new run notes. This deployment checklist is preparation, not evidence of a live
NAS installation or a completed security review.

For read-only storage surveys, verified NAS archival, and guarded retirement of old
linked worktrees, see [worker storage hygiene](../../docs/platform/operations/worker-hygiene.md).
Keep the offline module cache and active service data out of cleanup batches.
