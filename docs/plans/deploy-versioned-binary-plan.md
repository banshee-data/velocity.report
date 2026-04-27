# Multi-call binary, versioned on disk, symlink-swap rollback

- **Document Version:** 1.1
- **Status:** Proposed
- **Layers:** Go binaries, image build, install scripts, release pipeline, sudoers
- **Related:** [deploy-nginx-removal-plan.md](./deploy-nginx-removal-plan.md), [deploy-distribution-packaging-plan.md](./deploy-distribution-packaging-plan.md), [cli-restructuring-plan.md](./cli-restructuring-plan.md)
- **Supersedes:** the multi-binary recommendation in §5 of [deploy-distribution-packaging-plan.md](./deploy-distribution-packaging-plan.md) and the relevant sections of [cli-restructuring-plan.md](./cli-restructuring-plan.md)

---

## Context

We ship two Go binaries today, `velocity-report` and `velocity-ctl`, that share most of the same Go runtime and embedded web build. The image also ships shell aliases for service lifecycle, `velocity-status`, `velocity-log`, `velocity-start`, `velocity-stop`, and `velocity-bounce`, plus the legacy `velocity-update` redirect stub. Rollback exists today via timestamped backups in `/var/lib/velocity-report/backups/`, but each rollback is a copy operation, not a fast atomic switch.

Folding every entry point into a single busybox-style binary gives us:

- One artifact to sign, hash, and ship. `release.json` shrinks per channel.
- Atomic upgrade and atomic rollback by symlink swap (one `renameat2(2)` in the kernel).
- N-version retention with bounded disk: keep the last 3 versions, prune the rest.
- Updates never write to `/usr/local/bin/`; the read-only-root story improves later.

## Proposed architecture

**Single binary**, with one public CLI and two explicit outside-the-binary surfaces:

1. **Binary CLI:** `velocity <namespace> ...` is the canonical command surface.
2. **Host lifecycle wrappers:** `velocity-status`, `velocity-log`, `velocity-start`, `velocity-stop`, and `velocity-bounce` stay as shell aliases around `systemctl` and `journalctl`.
3. **HTTP API:** the running service exposes `/api/...` endpoints, including the new `GET /api/version` build-identity endpoint.

`argv[0]` is used for compatibility only. `velocity` is the canonical executable name. `velocity-report` remains as the server-oriented compatibility alias because systemd and existing operator habits already depend on it. `velocity-ctl` should not survive as a promoted public alias: if a migration bridge is needed, it should be a short-lived redirect to the `device` namespace and it should disappear once the image, MOTD, sudoers, and docs all speak the new language.

### Architecture decision record

| Decision                 | Chosen direction                                                                            | Why                                                                                                                                                             |
| ------------------------ | ------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Public management naming | Use `device`, not `ctl`                                                                     | `ctl` describes an implementation bucket, not an operator task. `device` says what the commands are for: installed-version lifecycle on the Pi.                 |
| Alias budget             | Promote `velocity`; keep `velocity-report` for compatibility; do not promote `velocity-ctl` | The image already uses shell aliases for host lifecycle. The binary should not accumulate a second alias family unless compatibility forces a temporary bridge. |
| Host lifecycle boundary  | Keep service status, logs, start, stop, and restart outside the binary                      | These are host concerns, not application-domain namespaces. The image already wraps them cleanly with shell aliases.                                            |
| Version visibility       | Add `GET /api/version` and keep `velocity version`                                          | The API smoke test should read the running build identity directly, without shelling into the host or guessing from a file.                                     |
| Utility packaging        | Fold shipping utilities into namespaces                                                     | `cmd/sweep` already shares the runtime; operator-facing repair tools should live under one binary rather than spawning more release artifacts.                  |
| CLI shape                | One canonical command tree, aliases for compatibility only                                  | This is the DRY boundary: one parser per namespace, one help surface, one shipped artifact, multiple bounded compatibility forms.                               |

### System boundary diagram

```text
Operator, scripts, and systemd
        |
        +--> /usr/local/bin/velocity ----------------------+
        |                                                  |
        +--> /usr/local/bin/velocity-report ---------------+--> /opt/velocity-report/current/velocity
        |                                                  |           |
        |                                                  |           +--> dispatcher (argv[0], argv[1])
        |                                                  |                    |
        |                                                  |                    +--> serve namespace
        |                                                  |                    +--> device namespace
        |                                                  |                    +--> data namespace
        |                                                  |                    +--> report namespace
        |                                                  |                    +--> tune namespace
        |                                                  |                    +--> version/help
        |
        +--> shell aliases in /etc/profile.d/velocity-aliases.sh
        |       |
        |       +--> velocity-status, velocity-start, velocity-stop,
        |       |    velocity-bounce, velocity-log
        |       +--> systemctl and journalctl
        |
        +--> HTTP clients
                |
                +--> /api/version, /api/radar_stats, /api/config, /api/capabilities
                +--> /api/sites, /api/site_config_periods, /api/timeline
                +--> /api/generate_report, /api/reports/*, /api/db_stats
                +--> /api/charts/{timeseries,histogram,comparison}, /api/transit_worker, /command
```

### Overall CLI strategy

#### Rule of the surface

The user guide should promote three surfaces, not one blended pile:

| Surface                  | Purpose                                      | Promoted forms                                                                          |
| ------------------------ | -------------------------------------------- | --------------------------------------------------------------------------------------- |
| Binary CLI               | application and data operations              | `velocity ...`                                                                          |
| Shell lifecycle wrappers | host service lifecycle on the Pi             | `velocity-status`, `velocity-log`, `velocity-start`, `velocity-stop`, `velocity-bounce` |
| HTTP API                 | remote inspection and application automation | `/api/...`, plus `/command`                                                             |

The binary should not duplicate the shell lifecycle wrappers, and the shell aliases should not pretend to be the application CLI.

### Command surface and compatibility contract

#### Canonical rule

The project exposes one canonical binary surface after this lands: `velocity <namespace> ...`. Every other executable name is a compatibility bridge into that surface. We keep bridges because they reduce migration risk, not because they are separate interfaces.

#### Binary names and entry points

| Installed name    | Canonical status                              | Default behaviour                                           | Notes                                                      |
| ----------------- | --------------------------------------------- | ----------------------------------------------------------- | ---------------------------------------------------------- |
| `velocity`        | canonical                                     | print top-level help when no namespace is supplied          | preferred human and script entry point for new docs        |
| `velocity-report` | compatibility alias                           | start the radar server when invoked with no subcommand      | keeps the service file and current operator habits working |
| `velocity-ctl`    | transitional redirect only, if shipped at all | forward to `velocity device ...` with a deprecation warning | not part of the promoted user-guide surface                |

#### Namespaces to expose

| Namespace | Canonical invocation                                                                 | Compatibility forms                                                                    | Scope and policy                                                                                                                                                                                                                                              |
| --------- | ------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `serve`   | `velocity serve [server flags]`                                                      | `velocity-report [server flags]`, `velocity-report serve [server flags]`               | Reuse the current radar/server flags unchanged: `--listen`, `--db-path`, `--disable-radar`, LiDAR flags, PDF LaTeX flags, and transit-worker flags.                                                                                                           |
| `device`  | `velocity device <check\|upgrade\|rollback\|backup> [flags]`                         | transitional `velocity-ctl <command> [flags]`                                          | Own the installed-binary lifecycle only. `check` replaces the overloaded `upgrade --check` in the public shape, though a compatibility flag may remain during migration. `upgrade` keeps `--binary`, `--prerelease`, `--include-prereleases`, and `--config`. |
| `data`    | `velocity data migrate <up\|down\|status\|version\|force\|baseline\|detect> [flags]` | legacy `velocity-report migrate ...`                                                   | Reuse the existing migration CLI contract from `internal/db/migrate_cli.go`; do not invent a second migration parser.                                                                                                                                         |
| `data`    | `velocity data backfill <target> [flags]`                                            | legacy `velocity-report backfill ...`                                                  | Fold promoted operator-facing repair utilities into one namespace rather than shipping `velocity-report-backfill-*` binaries. Initial targets should cover `ring-elevations` and `lidar-run-config` if they remain operator-facing.                           |
| `report`  | `velocity report pdf --config <file> --db <file> [--output <dir>] [--version]`       | legacy `velocity-report pdf ...`                                                       | Preserve the current PDF flags and output contract.                                                                                                                                                                                                           |
| `tune`    | `velocity tune sweep [sweep flags]`                                                  | legacy `velocity-report sweep ...`                                                     | Fold `cmd/sweep` into the shipping binary. Preserve the current sweep flags in the first implementation: monitor, sensor, output, PCAP, mode, parameter ranges, seed, sampling, and tracking sweep flags.                                                     |
| `version` | `velocity version`                                                                   | `velocity-report --version`, `velocity-report -v`, transitional `velocity-ctl version` | Print the same build identity that backs `GET /api/version`: semantic version, git SHA, and build time.                                                                                                                                                       |
| `help`    | `velocity help [namespace]`                                                          | `velocity-report help [namespace]`, transitional `velocity-ctl --help`                 | All help text should describe the same underlying command tree, with compatibility bridges called out as temporary.                                                                                                                                           |

#### Outside the binary: host lifecycle wrappers

| Wrapper           | Backing command                                                 | Purpose                  |
| ----------------- | --------------------------------------------------------------- | ------------------------ |
| `velocity-status` | `systemctl status velocity-report.service`                      | service status           |
| `velocity-start`  | `sudo systemctl start velocity-report.service`                  | start the service        |
| `velocity-stop`   | `sudo systemctl stop velocity-report.service`                   | stop the service         |
| `velocity-bounce` | `sudo systemctl restart velocity-report.service`                | restart the service      |
| `velocity-log`    | `journalctl -u velocity-report.service -u nginx.service -f ...` | follow live service logs |

These wrappers stay outside the binary because they are host-admin affordances. The binary should not grow `status`, `start`, `stop`, `restart`, or `logs` namespaces that merely duplicate `systemctl` and `journalctl`.

#### Outside the binary: HTTP API families

| Family                  | Endpoints                                                                                    |
| ----------------------- | -------------------------------------------------------------------------------------------- |
| Identity and capability | `GET /api/version`, `GET /api/config`, `GET /api/capabilities`                               |
| Traffic data            | `GET /api/radar_stats`, `GET /api/timeline`, `GET /api/db_stats`                             |
| Site configuration      | `GET/POST /api/sites`, `GET/POST /api/site_config_periods`                                   |
| Reports                 | `POST /api/generate_report`, `GET/DELETE /api/reports/*`                                     |
| Charts                  | `GET/POST /api/charts/timeseries`, `GET /api/charts/histogram`, `GET /api/charts/comparison` |
| Control                 | `POST /command`, `GET/POST /api/transit_worker`                                              |

#### Not in scope for this plan

- `cmd/tools/*` does not automatically become part of the shipping CLI just because it exists.
- One-off developer helpers stay under `cmd/tools/` until there is an operator-facing reason to promote them.
- The DRY rule is: promote one parser once, then mount it under the dispatcher. Do not copy flags into parallel binaries.

### Failure registry

| Component                 | Failure mode                                                                           | Recovery                                                                                              |
| ------------------------- | -------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| Dispatcher                | Unknown alias or namespace name                                                        | Print one canonical help tree and exit non-zero; do not silently fall through to the server.          |
| `GET /api/version`        | Handler unavailable because the server is down                                         | Verification falls back to process-level checks; upgrades still gate on service restart health.       |
| `device` namespace        | Upgrade or rollback subcommand fails                                                   | Leave `current` unchanged, return a non-zero exit, and keep the running service on the prior version. |
| `tune` or `data backfill` | Utility-specific flag or runtime error                                                 | Fail only that invocation; the main service and release layout remain unaffected.                     |
| Compatibility bridges     | Script or service still uses `velocity-report` or a transitional `velocity-ctl` bridge | Compatibility wrappers forward into the new namespace tree while docs steer new usage to `velocity`.  |

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
└── velocity-report  -> /opt/velocity-report/current/velocity

/etc/profile.d/
└── velocity-aliases.sh            (service lifecycle wrappers)
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

- [image/stage-velocity/01-velocity-binaries/00-run.sh](../../image/stage-velocity/01-velocity-binaries/00-run.sh) — install the single `velocity` binary into `/opt/velocity-report/versions/<bake-version>/velocity`, create the `current` symlink, and create `/usr/local/bin/velocity` plus `/usr/local/bin/velocity-report`. Version comes from a build-time `-ldflags "-X main.Version=..."` baked at image time.
- [image/stage-velocity/03-velocity-config/files/velocity-aliases.sh](../../image/stage-velocity/03-velocity-config/files/velocity-aliases.sh) — keep the service lifecycle wrappers as the outside-the-binary admin surface.
- **Delete** the legacy stub `image/stage-velocity/01-velocity-binaries/files/velocity-update`.
- **Avoid** shipping `velocity-ctl` as a permanent symlink. If a migration bridge is required, make it a short-lived redirect wrapper to `velocity device ...`, not a third first-class entry point.
- [image/stage-velocity/03-velocity-config/files/velocity-report.service](../../image/stage-velocity/03-velocity-config/files/velocity-report.service) — `ExecStart=/usr/local/bin/velocity-report …` continues to work (resolves through the symlink chain). No service-file change required for upgrades to take effect on next restart.

## Sudo and privileges

The existing `/etc/sudoers.d/020_velocity-nopasswd` already grants the `pi` user `systemctl` actions for `velocity-report`, plus `velocity-ctl *` and `velocity-report migrate *`. In the new shape:

- Keep: the existing `systemctl` rules that power `velocity-status`, `velocity-start`, `velocity-stop`, and `velocity-bounce`.
- Add: `pi ALL=(root) NOPASSWD: /usr/local/bin/velocity device *`
- Add: `pi ALL=(root) NOPASSWD: /usr/local/bin/velocity data migrate *`
- Keep `velocity-report migrate *` only as a migration bridge while old docs and scripts still rely on it.
- Keep `velocity-ctl *` only if a temporary redirect wrapper is shipped. Remove it once the migration bridge goes away.
- **Do not** grant write access to `/opt/velocity-report/versions/` to anyone other than root. The `device` lifecycle path still runs under sudo and writes there as root.

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
- **Move** `cmd/radar/*.go` → `internal/cmd/radar/` (export `Main(args []string)`); mount the existing `cmd/velocity-ctl/` implementation under the public `device` namespace rather than exposing `ctl` in the user-facing CLI
- **Move** `cmd/sweep/` → `internal/cmd/sweep/` (export `Main(args []string)`), then mount it as the `sweep` applet
- **Promote** operator-facing backfill helpers under `cmd/tools/` into `internal/cmd/backfill/` targets such as `ring-elevations` and `lidar-run-config`, rather than shipping standalone backfill binaries
- **Edit** `Makefile` — replace `build-radar-local`, `build-radar-linux`, `build-ctl` with a single `build-velocity` target (linux-arm64 + local variants); update CI references
- **Edit** [internal/api/](../../internal/api) — add `GET /api/version` returning the build identity from [internal/version/version.go](../../internal/version/version.go)
- **Edit** [internal/ctl/manager.go](../../internal/ctl/manager.go) — versioned-dir layout, `current`/`previous` symlinks, `renameat2` swap, retention prune
- **Edit** [image/stage-velocity/01-velocity-binaries/00-run.sh](../../image/stage-velocity/01-velocity-binaries/00-run.sh) — install single binary into `versions/<v>/`, create symlinks
- **Edit** [image/stage-velocity/03-velocity-config/files/velocity-aliases.sh](../../image/stage-velocity/03-velocity-config/files/velocity-aliases.sh) only if the wrapper set needs renaming or expansion; keep host lifecycle outside the binary
- **Delete** `image/stage-velocity/01-velocity-binaries/files/velocity-update`
- **Edit** [image/stage-velocity/03-velocity-config/00-run.sh](../../image/stage-velocity/03-velocity-config/00-run.sh) — sudoers entries listed above
- **Edit** [public_html/src/\_data/release.json](../../public_html/src/_data/release.json) and [scripts/update-release-json.py](../../scripts/update-release-json.py) — single `velocity-linux-arm64` artifact per channel; drop the per-binary URL list
- **Edit** `.github/workflows/release.yml` (or equivalent) — single artifact + SHA256 <!-- link-ignore -->

## Verification

1. **Build**: `make build-velocity` produces one binary; `./velocity --help` lists `serve|device|data|report|tune|version|help`.
2. **Compatibility**: `./velocity-report --db-path test.db` still enters the server path; `./velocity-report report pdf --help` and `./velocity report pdf --help` describe the same operation.
3. **First install (image)**: burn image; verify `/opt/velocity-report/versions/<v>/velocity` exists, `current` symlink resolves, `/usr/local/bin/velocity` and `/usr/local/bin/velocity-report` work, the shell aliases are installed, the service starts, and `curl http://localhost/api/version` returns the baked build identity.
4. **Host lifecycle wrappers**: in an interactive shell, verify `velocity-status`, `velocity-log`, `velocity-start`, `velocity-stop`, and `velocity-bounce` resolve to the expected `systemctl` and `journalctl` actions.
5. **Upgrade**: stage a fake `release.json` pointing at a `0.5.1` artifact; run `sudo velocity device upgrade`; verify (a) `versions/0.5.1/` appears, (b) `current` flips, (c) service restarts cleanly, (d) `/api/version` reports `0.5.1`, and (e) `previous` points to `0.5.0`.
6. **Rollback**: `sudo velocity device rollback`; verify `current` points back to `0.5.0` after one symlink swap and service restart; `/api/version` reports `0.5.0`.
7. **Namespace parity**: verify `./velocity tune sweep --help` and the legacy `./velocity-report sweep --help` print the same sweep flag surface; verify `./velocity data backfill ring-elevations --help` and `./velocity data backfill lidar-run-config --help` are namespaced under one parser tree rather than separate binaries.
8. **Retention**: simulate three upgrades; confirm `versions/` holds exactly 3 entries.
9. **Migration failure path**: ship a `0.5.2` whose `migrate up` returns non-zero; confirm `current` is unchanged, `0.5.1` keeps running, and the error surfaces in `velocity device upgrade` exit code.
10. **Atomicity**: `strace -f -e renameat2 sudo velocity device upgrade` — observe a single `RENAME_EXCHANGE` swap.

## Sequencing

Land **after** [deploy-nginx-removal-plan.md](./deploy-nginx-removal-plan.md). The Go refactor and image layout shuffle benefit from a settled image stage. Touches `release.json`, install scripts, shell alias wrappers, and the existing device-management internals.
