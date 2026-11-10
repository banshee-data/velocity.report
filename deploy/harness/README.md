# arrow-worker experiment harness

The implementation and [operator runbook](../../docs/platform/operations/experiment-harness.md)
cover the v0.5.2 mechanical runner. These files prepare a systemd installation;
installing and enabling the timer and the week-long NAS soak remain deployment work.

Use a dedicated clone with local `main` checked out. Install a stamped `velocity`
binary built through `make build-velocity`, Go at `go.mod`'s version, libpcap development
headers and Git. Populate the service user's Go module cache while connected. The
per-commit tool build uses `GOPROXY=off` and `GOTOOLCHAIN=local`; it never downloads
missing dependencies during a replay. The tool build uses embed stubs and does not
need Node at runtime. Building the complete `velocity` distribution still needs the
normal documentation and Typst build dependencies.

Installation, by an administrator on arrow-worker:

1. Install `velocity-harness.sysusers` as
   `/etc/sysusers.d/velocity-harness.conf`, then run `systemd-sysusers`.
2. With the captures and results mounts available, install
   `velocity-harness.tmpfiles` as `/etc/tmpfiles.d/velocity-harness.conf`, then
   run `systemd-tmpfiles --create`. Check the mounts before creating directories.
3. Place the dedicated clone at `/var/lib/velocity-harness/repo`, owned by
   `velocity-harness`, with local `main` checked out and a read-capable `origin`.
4. Copy `arrow-worker.example.json` to `/var/lib/velocity-harness/host.json` and
   check every path, corpus and capability. The example advertises S2 captures;
   add kirk captures and labels only after their paths and frozen references exist.
5. To publish, add a `publish` object shaped like `github-app.example.json` to
   the host profile. Install the repository-scoped App with contents and pull-request
   write, checks and commit-status read permissions. Give its RSA private key mode
   `0600`, owned by `velocity-harness`. Replace both IDs and the bot identity.
   No key, token or configured host profile belongs in Git.
6. Run `velocity experiments tick -host /var/lib/velocity-harness/host.json
-root /var/lib/velocity-harness/repo -dry-run` as the service user, then run one
   approved fixture with `-offline`. Review the output and its bounded note.
7. After the deployment review, install the service and timer in
   `/etc/systemd/system/`, run `systemctl daemon-reload`, and enable the timer
   with `systemctl enable --now velocity-harness.timer`.
8. Perform the acceptance soak: kill a replay through systemd, restart it, verify
   that done units retain their attempt numbers, then follow one approved spec
   through App-created draft, green checks, clear reviews and squash merge.
   Observe the timer for a week before declaring H5 complete.

The service opens no port, has no sudo access, reads captures read-only and kills
its entire process group on stop. It can write its own clone, cache and evidence
paths. App scope and the path guard constrain publication to the one repository's
new run notes. This deployment checklist is preparation, not evidence of a live
NAS installation or a completed security review.
