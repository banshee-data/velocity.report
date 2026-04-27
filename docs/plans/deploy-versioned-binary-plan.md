# Multi-call binary, versioned on disk, symlink-swap rollback

- **Document Version:** 1.0
- **Status:** Proposed
- **Layers:** Go binaries, image build, install scripts, release pipeline, sudoers
- **Related:** [deploy-nginx-removal-plan.md](./deploy-nginx-removal-plan.md), [deploy-distribution-packaging-plan.md](./deploy-distribution-packaging-plan.md), [cli-restructuring-plan.md](./cli-restructuring-plan.md)
- **Supersedes:** the multi-binary recommendation in §5 of [deploy-distribution-packaging-plan.md](./deploy-distribution-packaging-plan.md) and the relevant sections of [cli-restructuring-plan.md](./cli-restructuring-plan.md)

---

## Context

We ship two Go binaries (`velocity-report`, `velocity-ctl`) that share most of the same Go runtime + embedded web build. Update logic lives in [internal/ctl/manager.go](../../internal/ctl/manager.go) and is invoked by a separate `velocity-ctl` process. Rollback exists today via timestamped backups in `/var/lib/velocity-report/backups/`, but each rollback is a copy operation, not a fast atomic switch.

Folding every entry point into a single busybox-style binary gives us:

- One artifact to sign, hash, and ship. `release.json` shrinks per channel.
- Atomic upgrade and atomic rollback by symlink swap (one `renameat2(2)` in the kernel).
- N-version retention with bounded disk: keep the last 3 versions, prune the rest.
- Updates never write to `/usr/local/bin/`; the read-only-root story improves later.

## Proposed architecture

**Single binary**, dispatched by `os.Args[0]`:

```go
// cmd/velocity/main.go
func main() {
    name := filepath.Base(os.Args[0])
    switch name {
    case "velocity-ctl":
        ctl.Main(os.Args[1:])
    case "velocity-report", "velocity":
        if len(os.Args) > 1 {
            switch os.Args[1] {
            case "ctl":     ctl.Main(os.Args[2:])
            case "migrate": migrate.Main(os.Args[2:])
            case "pdf":     pdf.Main(os.Args[2:])
            }
        }
        radar.Main(os.Args[1:]) // default: run the server
    }
}
```

`velocity --help` lists every applet, so the binary is self-documenting (`velocity ctl upgrade`, `velocity migrate up`, `velocity pdf`, `velocity serve`, etc.). `velocity-ctl` and `velocity-report` remain as symlinks for muscle memory and so that existing sudoers wildcards keep working.

## On-disk layout

```
/opt/velocity-report/
├── versions/
│   ├── 0.5.0/velocity            (real binary, mode 0755)
│   ├── 0.5.1/velocity
│   └── 0.6.0-pre.3/velocity
├── current  -> versions/0.5.1     (the active version symlink)
└── previous -> versions/0.5.0     (set by upgrade for one-shot rollback)

/usr/local/bin/
├── velocity         -> /opt/velocity-report/current/velocity
├── velocity-report  -> /opt/velocity-report/current/velocity
└── velocity-ctl     -> /opt/velocity-report/current/velocity
```

### Upgrade

1. Download `velocity` for the new version → `versions/<v>/velocity.partial`.
2. Verify SHA256 (already done in [internal/ctl/manager.go](../../internal/ctl/manager.go)).
3. `chmod 0755`, rename to `versions/<v>/velocity`.
4. Run `versions/<v>/velocity migrate up` (the new binary owns its migrations).
5. Atomically swap `current`: `ln -s versions/<v> current.new && renameat2(AT_FDCWD, "current.new", AT_FDCWD, "current", RENAME_EXCHANGE)` (Linux 3.15+, present on all supported Pi kernels). Go: `golang.org/x/sys/unix.Renameat2`.
6. Update `previous` to point at the prior version.
7. `systemctl restart velocity-report`.
8. After 60 s of healthy `/api/radar_stats`, prune all versions older than `previous`, keeping the last 3.

### Rollback

1. Read `previous`.
2. `renameat2` swap `current` → previous version.
3. `systemctl restart velocity-report`.
4. **Do not** auto-down-migrate the DB (see "Limitations").

## Image and install-script impact

- [image/stage-velocity/01-velocity-binaries/00-run.sh](../../image/stage-velocity/01-velocity-binaries/00-run.sh) — install the single `velocity` binary into `/opt/velocity-report/versions/<bake-version>/velocity`, create the `current` symlink, create the three `/usr/local/bin/*` symlinks. Version comes from a build-time `-ldflags "-X main.Version=..."` baked at image time.
- **Delete** the legacy stub `image/stage-velocity/01-velocity-binaries/files/velocity-update` (it currently just redirects to `velocity-ctl upgrade`).
- [image/stage-velocity/03-velocity-config/files/velocity-report.service](../../image/stage-velocity/03-velocity-config/files/velocity-report.service) — `ExecStart=/usr/local/bin/velocity-report …` continues to work (resolves through the symlink chain). No service-file change required for upgrades to take effect on next restart.

## Sudo and privileges

The existing `/etc/sudoers.d/020_velocity-nopasswd` already grants the `pi` user wildcard `velocity-ctl *`, `velocity-report migrate *`, and `systemctl …` for `velocity-report`. To stay tidy:

- Add: `pi ALL=(root) NOPASSWD: /usr/local/bin/velocity ctl *`
- Add: `pi ALL=(root) NOPASSWD: /usr/local/bin/velocity migrate *`
- Keep the existing wildcard rules so existing scripts and aliases keep working.
- **Do not** grant write access to `/opt/velocity-report/versions/` to anyone other than root. The `velocity-ctl` upgrade path runs under sudo and writes there as root. No change to that model.

## Migrations

- Embedded migrations stay forward-only. Every upgrade runs `migrate up` from the **new** binary before the symlink swap. If `migrate up` fails, the upgrade aborts, `current` stays pointing at the old version, and the running service keeps running uninterrupted.
- Rollback **does not** down-migrate. Schema changes that are forward-incompatible (a column drop, say) make rollback unsafe by definition. Document this; rely on the existing per-upgrade DB backup at `/var/lib/velocity-report/backups/<ts>.db` as the rescue path. The `rollback` subcommand prints a loud warning if the previous version's migration set is a strict subset of what's currently applied, and refuses to proceed without `--force`.

## Limitations and restrictions

- **Schema-rollback gap.** Symlink rollback is only safe across migration-compatible versions. The `--force` flag with a loud warning is the chosen safety valve.
- **Disk usage.** Each retained version is ~30–60 MB (Go binary + embedded web assets). Cap retention at 3 (`current`, `previous`, one cushion). Pruning runs at the end of every successful upgrade.
- **Atomic swap caveat.** `rename(2)` is atomic for files but historically not always atomic for directory symlinks under all kernels. `renameat2(…, RENAME_EXCHANGE)` (Linux 3.15+) is the correct primitive and is available on every supported Pi kernel.
- **systemd unit caching.** None — `ExecStart=/usr/local/bin/velocity-report` is resolved at exec time, so `systemctl restart` re-resolves the symlink chain.
- **Build matrix.** `make build-radar-local`, `make build-radar-linux`, `make build-ctl` collapse into a single `make build-velocity` (with linux-arm64 and local variants). [assets.go](../../assets.go)'s `embed.FS` directives stay as-is.
- **First install.** No special handling — the bake script writes a single version under `versions/`, and `current` is set; nothing different from a normal upgrade except `previous` is unset until the first upgrade runs.

## Files to change

- **New** `cmd/velocity/main.go` — argv[0] dispatcher
- **Move** `cmd/radar/*.go` → `internal/cmd/radar/` (export `Main(args []string)`); same shape for `cmd/velocity-ctl/` → `internal/cmd/ctl/`
- **Edit** `Makefile` — replace `build-radar-local`, `build-radar-linux`, `build-ctl` with a single `build-velocity` target (linux-arm64 + local variants); update CI references
- **Edit** [internal/ctl/manager.go](../../internal/ctl/manager.go) — versioned-dir layout, `current`/`previous` symlinks, `renameat2` swap, retention prune
- **Edit** [image/stage-velocity/01-velocity-binaries/00-run.sh](../../image/stage-velocity/01-velocity-binaries/00-run.sh) — install single binary into `versions/<v>/`, create symlinks
- **Delete** `image/stage-velocity/01-velocity-binaries/files/velocity-update`
- **Edit** [image/stage-velocity/03-velocity-config/00-run.sh](../../image/stage-velocity/03-velocity-config/00-run.sh) — sudoers entries listed above
- **Edit** [public_html/src/\_data/release.json](../../public_html/src/_data/release.json) and [scripts/update-release-json.py](../../scripts/update-release-json.py) — single `velocity-linux-arm64` artifact per channel; drop the per-binary URL list
- **Edit** `.github/workflows/release.yml` (or equivalent) — single artifact + SHA256 <!-- link-ignore -->

## Verification

1. **Build**: `make build-velocity` produces one binary; `./velocity --help` lists `ctl|migrate|pdf|serve` applets.
2. **Argv dispatch**: `ln -s velocity velocity-ctl && ./velocity-ctl status` exercises the same code path as `./velocity ctl status`.
3. **First install (image)**: burn image; verify `/opt/velocity-report/versions/<v>/velocity` exists, `current` symlink resolves, `/usr/local/bin/velocity-report` works, service starts.
4. **Upgrade**: stage a fake `release.json` pointing at a `0.5.1` artifact; run `sudo velocity-ctl upgrade`; verify (a) `versions/0.5.1/` appears, (b) `current` flips, (c) service restarts cleanly, (d) `/api/version` reports `0.5.1`, (e) `previous` points to `0.5.0`.
5. **Rollback**: `sudo velocity-ctl rollback`; verify `current` points back to `0.5.0` after one symlink swap and service restart; `/api/version` reports `0.5.0`.
6. **Retention**: simulate three upgrades; confirm `versions/` holds exactly 3 entries.
7. **Migration failure path**: ship a `0.5.2` whose `migrate up` returns non-zero; confirm `current` is unchanged, `0.5.1` keeps running, error surfaces in `velocity-ctl upgrade` exit code.
8. **Atomicity**: `strace -f -e renameat2 sudo velocity-ctl upgrade` — observe a single `RENAME_EXCHANGE` swap.

## Open questions

1. **`/api/version` endpoint.** Used in verification but does not yet exist in [internal/api/](../../internal/api). Either add a tiny handler returning the build-time `Version` string, or shell out to `velocity-report version` from the verification script. The handler is cheaper and makes Tailscale Serve smoke tests possible from any peer.
2. **Sweep and other utilities.** [deploy-distribution-packaging-plan.md](./deploy-distribution-packaging-plan.md) contemplates `velocity-report-sweep` and `velocity-report-backfill-rings` as standalone binaries. Decide whether they fold into `velocity sweep` / `velocity backfill-rings` applets, or remain separate. Recommendation: fold them; sweep already shares the runtime, and we want a single shipping artifact.

## Sequencing

Land **after** [deploy-nginx-removal-plan.md](./deploy-nginx-removal-plan.md). The Go refactor + image layout shuffle benefits from a settled image stage. Touches `release.json`, install scripts, and `velocity-ctl` internals.
