# Remote host upgrade runbook

This runbook upgrades an already-installed `velocity.report` host over SSH
without using `velocity-ctl` or the removed legacy `velocity-deploy` tool.
It is written for an interactive VS Code agent running in Ask mode on the
target host.

## Goal

Replace the installed `velocity-report` binary, preserve the existing systemd
configuration, run any required database migrations, and verify the service
comes back healthy.

## Scope

This runbook assumes the host already has a working installation with these
paths:

- Binary: `/usr/local/bin/velocity-report`
- Service: `/etc/systemd/system/velocity-report.service`
- Data directory: `/var/lib/velocity-report`
- Main database: `/var/lib/velocity-report/sensor_data.db`
- Optional source/PDF checkout: `/opt/velocity-report`

Do not use this runbook for first-time installs. Do not overwrite the systemd
unit unless there is a deliberate service configuration change.

## Ask-Mode guardrails

Stop and ask before continuing if any of these are true:

- `velocity-report.service` is missing or points at paths other than the ones
  above.
- `systemctl is-active velocity-report.service` is not `active` before the
  upgrade.
- `velocity-report migrate --db-path /var/lib/velocity-report/sensor_data.db status`
  reports `Dirty: true`.
- `/opt/velocity-report` exists but `git status --short` is not clean.
- The only available build output was produced from a checkout whose
  [web/build/index.html](../../../web/build/index.html) is the stub page.
- The host cannot provide `sudo` for service stop/start, file install, and
  backup steps.

## Inputs

Decide these before you start:

- `TARGET_REF`: git tag, commit SHA, or branch to deploy.
- `NEW_BIN`: path to a vetted new binary on the host.
- Whether `/opt/velocity-report` also needs to move to `TARGET_REF` so PDF
  generation stays in sync with the Go binary.

Preferred source for `NEW_BIN`: a prebuilt `velocity-report-{version}-linux-arm64`
artifact already copied to the host. Building on-host is a fallback only.

## Reconnaissance

Paste this entire block into the SSH session first. Every command is read-only
and none require `sudo` (the final line tests whether passwordless `sudo` is
available but will never prompt). Copy the full output back so the agent can
analyse the current state before planning any changes.

```bash
echo "=== identity ==="
id
hostname
cat /etc/os-release 2>/dev/null | head -4

echo "=== platform ==="
uname -a
dpkg --print-architecture 2>/dev/null || arch

echo "=== paths ==="
export VR_BIN=/usr/local/bin/velocity-report
export VR_SVC=/etc/systemd/system/velocity-report.service
export VR_DB=/var/lib/velocity-report/sensor_data.db
ls -la "$VR_BIN" 2>/dev/null || echo "binary missing"
file "$VR_BIN" 2>/dev/null || true
"$VR_BIN" --version 2>/dev/null || echo "cannot run binary"

echo "=== service ==="
systemctl is-active velocity-report.service 2>/dev/null || echo "unknown"
systemctl is-enabled velocity-report.service 2>/dev/null || echo "unknown"
systemctl show velocity-report.service -p ExecStart -p WorkingDirectory -p User -p Environment -p EnvironmentFile 2>/dev/null || true
systemctl cat velocity-report.service 2>/dev/null || echo "cannot read unit"
VR_LISTEN_PORT="$(systemctl show velocity-report.service -p ExecStart --value 2>/dev/null | sed -n 's/.*--listen :\([0-9][0-9]*\).*/\1/p' | head -n1)"
echo "listen port: ${VR_LISTEN_PORT:-8080 (default)}"
echo "=== database ==="
ls -la "$VR_DB" 2>/dev/null || echo "no database"
stat --printf='%s bytes\n' "$VR_DB" 2>/dev/null || stat -f '%z bytes' "$VR_DB" 2>/dev/null || true
if [ -f "$VR_DB" ]; then
  "$VR_BIN" migrate --db-path "$VR_DB" status 2>/dev/null || echo "migrate status unavailable"
fi

echo "=== disk ==="
df -h /usr/local/bin /var/lib/velocity-report /opt/velocity-report /tmp 2>/dev/null || true

echo "=== opt checkout ==="
if [ -d /opt/velocity-report ]; then
  ls -ld /opt/velocity-report 2>/dev/null
  ls -la /opt/velocity-report/.git/HEAD 2>/dev/null || echo "not a git repo"
  git -C /opt/velocity-report log --oneline -1 2>/dev/null || true
  git -C /opt/velocity-report status --short 2>/dev/null && echo "(working tree clean)" || true
  ls -la /opt/velocity-report/.venv/bin/python 2>/dev/null || echo "no venv"
  ls -la /opt/velocity-report/config/tuning.defaults.json 2>/dev/null || echo "tuning config MISSING"
else
  echo "/opt/velocity-report does not exist"
fi

echo "=== sudo check ==="
sudo -n true 2>/dev/null && echo "passwordless sudo: yes" || echo "passwordless sudo: no"

echo "=== done ==="
```

Paste the output back to the agent. The agent should check for:

- Whether the binary exists and which version is installed.
- Whether the service is active and which user it runs as.
- Whether the database exists, its size, and whether migrations are clean
  (`Dirty: false`). A dirty migration is a guardrail: stop and ask.
- Whether `/opt/velocity-report` is present, clean, and at which commit.
  If `git status --short` printed any paths before `(working tree clean)`,
  the checkout is dirty: stop and ask.
- Whether [config/tuning.defaults.json](../../../config/tuning.defaults.json) exists in `/opt/velocity-report`.
  If missing, the checkout predates the config restructure and must be
  updated before the service can start.
- Directory ownership of `/opt/velocity-report`: determines whether later
  `git` and `make` steps need `sudo -u`.
- Whether `sudo` is available without a password (affects later steps).
- The listen port printed at the end of the service block.

## Preflight

After the agent has analysed the reconnaissance output and confirmed there are
no guardrail violations, set these variables for the remaining steps.

The `RUN_AS` helper avoids pointless `sudo -u` when the SSH user already is the
service user (common on single-user Raspberry Pi installs where the service
runs as `david` rather than the template's default `velocity`).

```bash
export VR_BIN=/usr/local/bin/velocity-report
export VR_SVC=/etc/systemd/system/velocity-report.service
export VR_DB=/var/lib/velocity-report/sensor_data.db

export SERVICE_USER=$(systemctl show velocity-report.service -p User --value 2>/dev/null)
export SERVICE_USER=${SERVICE_USER:-velocity}

if [ "$(id -un)" = "$SERVICE_USER" ]; then
  RUN_AS()  { "$@"; }
else
  RUN_AS()  { sudo -u "$SERVICE_USER" "$@"; }
fi

echo "SERVICE_USER=$SERVICE_USER  SSH_USER=$(id -un)  sudo-u needed: $([ "$(id -un)" = "$SERVICE_USER" ] && echo no || echo yes)"
```

If the service file shows a custom `--listen :PORT`, keep that port for the
HTTP verification step later. If no `--listen` flag is present, assume `8080`.

## Build and transfer (on the dev machine)

Run these on the **local Mac**, not the host. This builds the web frontend
into the Go binary and cross-compiles for linux/arm64, then copies the
artifact to the host.

### Build

```bash
# From the repo root on the dev machine
export TARGET_REF=<tag-or-sha>
git checkout "$TARGET_REF"
git status --short

# Build the real web frontend (not the stub)
make build-web

# Verify a real dashboard was built
if grep -q "Web Frontend Not Built" web/build/index.html 2>/dev/null; then
  echo "ERROR: stub web build — do not deploy this"; exit 1
fi

# Cross-compile for Raspberry Pi
make build-radar-linux

# Sanity-check the artifact (dev builds have datetime prefix + SHA suffix)
BINARY=$(ls -t *-velocity-report-*-linux-arm64-* 2>/dev/null | head -1)
if [ -z "$BINARY" ]; then
  echo "No matching linux-arm64 velocity-report binary found after build."
  exit 1
fi
file "$BINARY"
ls -lh "$BINARY"
```

The local output binary in the repo root is a dev-style versioned file such as
`20260407T142345Z-velocity-report-0.5.1.pre1-linux-arm64-a1b2c3d`.
A clean filename such as `velocity-report-0.5.1-linux-arm64` refers to a
release asset, not the local output of `make build-radar-linux`.

### Transfer to host

```bash
ssh radar.local 'mkdir -p /tmp/vr'
scp "$BINARY" radar.local:/tmp/vr/
```

## Prepare the new binary (on the host)

Paste this on the host to verify the transferred artifact:

```bash
shopt -s nullglob
candidates=(/tmp/vr/*velocity-report*)
files=()
for candidate in "${candidates[@]}"; do
  if [ -f "$candidate" ]; then
    files+=("$candidate")
  fi
done

if [ "${#files[@]}" -ne 1 ]; then
  echo "Expected exactly one transferred velocity-report binary in /tmp/vr, found ${#files[@]}." >&2
  printf 'Matches:\n' >&2
  printf '  %s\n' "${files[@]}" >&2
  exit 1
fi

export NEW_BIN="${files[0]}"
chmod +x "$NEW_BIN"
file "$NEW_BIN"
"$NEW_BIN" --version
echo "size: $(ls -lh "$NEW_BIN" | awk '{print $5}')"
```

On an ARM64 host, `file "$NEW_BIN"` should report an `ELF 64-bit` `ARM aarch64`
binary.

### Fallback: build on the host

Only use this path if no dev machine is available and the host already has a
suitable checkout plus the build toolchain. A plain `make build-radar-linux` on
a fresh clone can succeed with a stubbed dashboard, which is not a production
build.

```bash
export TARGET_REF=<tag-or-sha>
cd /path/to/velocity.report
git status --short
git checkout "$TARGET_REF"
test -f web/build/index.html
if grep -q "Web Frontend Not Built" web/build/index.html; then
  echo "Stub web build detected; stop and use a real artifact or build the web app first."
  exit 1
fi
make build-radar-linux
BINARY=$(ls -1t *-velocity-report-*-linux-arm64-* 2>/dev/null | head -1)
if [ -z "$BINARY" ]; then
  echo "No matching linux-arm64 velocity-report binary found after build."
  exit 1
fi
export NEW_BIN="$PWD/$BINARY"
"$NEW_BIN" --version
```

If [web/build/index.html](../../../web/build/index.html) is missing, you need a real web build first:

```bash
make install-web
make build-web
make build-radar-linux
```

## Optional: sync `/opt/velocity-report`

The canonical service template sets:

- `PDF_GENERATOR_DIR=/opt/velocity-report/tools/pdf-generator`
- `PDF_GENERATOR_PYTHON=/opt/velocity-report/.venv/bin/python`

If the running host uses that layout, upgrade `/opt/velocity-report` to the
same `TARGET_REF` when the release includes PDF generator changes.

If `/opt/velocity-report` is a clean git checkout:

```bash
cd /opt/velocity-report
RUN_AS git status --short
RUN_AS git fetch --tags --prune
RUN_AS git checkout "$TARGET_REF"
RUN_AS make install-python
```

If that checkout is dirty, stop and ask instead of force-resetting it.

## Backup and stop the service

Create a rollback point before replacing anything:

```bash
export TS=$(date +%Y%m%d-%H%M%S)
export BACKUP_DIR=/var/lib/velocity-report/backups/$TS

mkdir -p "$BACKUP_DIR" || sudo mkdir -p "$BACKUP_DIR"
cp "$VR_SVC" "$BACKUP_DIR/velocity-report.service" 2>/dev/null || sudo cp "$VR_SVC" "$BACKUP_DIR/velocity-report.service"
"$VR_BIN" --version > "$BACKUP_DIR/version.txt" 2>&1 || true

sudo systemctl stop velocity-report.service
systemctl is-active velocity-report.service || true

cp "$VR_BIN" "$BACKUP_DIR/velocity-report" 2>/dev/null || sudo cp "$VR_BIN" "$BACKUP_DIR/velocity-report"
if [ -f "$VR_DB" ]; then
  cp "$VR_DB" "$BACKUP_DIR/sensor_data.db" || sudo cp "$VR_DB" "$BACKUP_DIR/sensor_data.db"
fi
```

Do not continue until the service has actually stopped.

## Install and migrate

Install the new binary in place:

```bash
sudo install -o root -g root -m 0755 "$NEW_BIN" "$VR_BIN"
"$VR_BIN" --version
```

Check migration state before applying anything.

**Important:** `--db-path` must appear between `migrate` and the sub-command
(`status` / `up`). Go's flag parser stops at the first non-flag argument, so
`migrate status --db-path …` silently ignores `--db-path` and falls back to
`./sensor_data.db` in the current working directory. <!-- link-ignore -->

```bash
if [ -f "$VR_DB" ]; then
  RUN_AS "$VR_BIN" migrate --db-path "$VR_DB" status
fi
```

If `Dirty: true` appears, stop and recover the database before proceeding.

Apply migrations only when the database exists:

```bash
if [ -f "$VR_DB" ]; then
  RUN_AS "$VR_BIN" migrate --db-path "$VR_DB" up
  RUN_AS "$VR_BIN" migrate --db-path "$VR_DB" status
fi
```

`RUN_AS` is a no-op when the SSH user matches the service user, and
`sudo -u "$SERVICE_USER"` otherwise. Prefer it over `su - velocity` because
the service user is normally created with a non-login shell.

## Update service configuration

The binary requires a `--config` flag pointing at the
tuning defaults file. The old `ExecStart` line does not include this flag.

Check whether the service already has `--config`:

```bash
systemctl cat velocity-report.service | grep -q -- '--config' && echo "--config already present" || echo "--config MISSING — update needed"
```

If `--config` is missing, update the service file. This is the one step where
the systemd unit is deliberately modified:

```bash
sudo sed -i 's|ExecStart=/usr/local/bin/velocity-report --db-path /var/lib/velocity-report/sensor_data.db|ExecStart=/usr/local/bin/velocity-report --db-path /var/lib/velocity-report/sensor_data.db --config /opt/velocity-report/config/tuning.defaults.json|' "$VR_SVC"
sudo systemctl daemon-reload
systemctl cat velocity-report.service | grep ExecStart
```

Verify the `ExecStart` line now includes both `--db-path` and `--config`
before restarting.

## Pre-Restart verification

Before starting the service, verify that the three common startup failures
cannot occur. This block is read-only and safe to run at any time:

```bash
echo "=== config file ==="
ls -la /opt/velocity-report/config/tuning.defaults.json 2>/dev/null || echo "FAIL: tuning config missing"

echo "=== migration state ==="
"$VR_BIN" migrate --db-path "$VR_DB" status

echo "=== binary version ==="
"$VR_BIN" --version
```

All three must pass before restarting:

- **Config file exists**: the `--config` path in `ExecStart` must resolve.
  If missing, the `/opt/velocity-report` checkout is stale or the
  `git checkout` step failed (check permissions).
- **Migrations clean**: `Dirty: false` and the current version matches the
  latest migration the binary knows about. If the version is behind, run
  `migrate up` again. If `Dirty: true`, stop and recover.
- **Binary version**: confirms the installed binary is the expected release.

## Restart and verify

Bring the service back and verify both systemd and HTTP health:

```bash
sudo systemctl start velocity-report.service
sleep 2
systemctl is-active velocity-report.service
systemctl --no-pager --full status velocity-report.service
journalctl -u velocity-report.service -n 50 --no-pager
```

If the service listens on `8080`, verify the API:

```bash
curl -fsS http://127.0.0.1:8080/api/config >/dev/null
```

If the service uses a different `--listen` port, substitute that port.

Success means:

- `systemctl is-active velocity-report.service` returns `active`
- `journalctl` shows no crash loop or startup fatal
- the API responds on the configured local port
- a single transient `SQLITE_BUSY` immediately after migration is expected;
  if it repeats continuously, stop the service and check for stale WAL files

## Cleanup

Remove the transferred binary and any stray database files:

```bash
rm -f /tmp/vr/velocity-report-*
rm -f /tmp/vr/*-velocity-report-*
rmdir /tmp/vr 2>/dev/null || true
ls /opt/velocity-report/sensor_data.db* 2>/dev/null && echo "STRAY DB — delete it" || echo "clean"
```

If a stray `sensor_data.db` exists in `/opt/velocity-report/`, it was created
by a `migrate` invocation with incorrect `--db-path` flag ordering. Delete it.

## Rollback

If the new binary fails after install or migration, restore the backup you just
made. For schema-changing upgrades, restoring only the binary is usually not
enough; restore the pre-upgrade database too.

```bash
sudo systemctl stop velocity-report.service
sudo install -o root -g root -m 0755 "$BACKUP_DIR/velocity-report" "$VR_BIN"
sudo cp "$BACKUP_DIR/velocity-report.service" "$VR_SVC"
if [ -f "$BACKUP_DIR/sensor_data.db" ]; then
  sudo cp "$BACKUP_DIR/sensor_data.db" "$VR_DB"
  if [ "$(id -un)" != "$SERVICE_USER" ]; then
    sudo chown "$SERVICE_USER:$SERVICE_USER" "$VR_DB"
  fi
fi
sudo systemctl daemon-reload
sudo systemctl start velocity-report.service
systemctl is-active velocity-report.service
journalctl -u velocity-report.service -n 50 --no-pager
```

If `/opt/velocity-report` was also upgraded, roll that checkout back to the
previous ref before retrying report generation.

## Known pitfalls

Lessons learned from real upgrades:

### `migrate --db-path` flag ordering

Go's `flag.FlagSet` stops parsing at the first non-flag positional argument.
The migrate subcommand uses a secondary FlagSet, so `--db-path` must appear
**between** `migrate` and the sub-command (`status` / `up`):

```bash
# Correct — flag is parsed:
velocity-report migrate --db-path /var/lib/velocity-report/sensor_data.db up

# WRONG — flag is silently ignored, falls back to ./sensor_data.db:
velocity-report migrate up --db-path /var/lib/velocity-report/sensor_data.db
```

The wrong form creates a stray database in the current working directory and
reports migrations as up-to-date while the real database remains untouched.

### Tuning config required

The binary requires `--config` pointing at
[config/tuning.defaults.json](../../../config/tuning.defaults.json) (or the file must exist relative to the working
directory). Older binaries had no `--config` flag. Upgrades crossing this
boundary must update the `ExecStart` line in the systemd unit.

### `/opt/velocity-report` permissions

If the checkout was originally cloned or updated as `root`, some files under
`.git/` will be root-owned. This causes `git fetch` to fail with
`Permission denied` when run as the service user. Fix with:
`sudo chown -R david:david /opt/velocity-report`.

## Suggested agent prompt

Use this with a VS Code SSH agent in Ask mode. The agent does not have direct
command access: it analyses output you paste and gives you blocks to run.

```text
Open docs/radar/operations/remote-host-upgrade-runbook.md and follow it exactly.

You do NOT have direct terminal access. Your job is:
1. Give me the Reconnaissance block to paste. Analyse my output.
2. Ask any follow-up questions before proceeding (dirty checkout, missing
   binary, stub web build, dirty migration, missing tuning config, etc.).
3. For each subsequent step, give me a single copy-paste block. Wait for my
   output before moving on.
4. Minimise sudo usage — only request it for service stop/start, file install,
   and backup/restore. Never use sudo for read-only information gathering.
5. Stop and ask if any guardrail condition is triggered.
6. Read the Known Pitfalls section before generating any migrate commands.

Upgrade this host to TARGET_REF=<tag-or-sha> without using velocity-ctl
or the removed legacy velocity-deploy tool.
NEW_BIN is at <path-to-binary-on-host>.
Preserve the existing systemd service configuration unless a mismatch forces a
decision.
```
