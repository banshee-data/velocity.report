# Remote Host Upgrade Runbook

This runbook upgrades an already-installed `velocity.report` host over SSH
without using `velocity-deploy` or legacy setup scripts. It is written for an
interactive VS Code agent running in Ask mode on the target host.

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

## Ask-Mode Guardrails

Stop and ask before continuing if any of these are true:

- `velocity-report.service` is missing or points at paths other than the ones
  above.
- `systemctl is-active velocity-report.service` is not `active` before the
  upgrade.
- `velocity-report migrate status --db-path /var/lib/velocity-report/sensor_data.db`
  reports `Dirty: true`.
- `/opt/velocity-report` exists but `git status --short` is not clean.
- The only available build output was produced from a checkout whose
  `web/build/index.html` is the stub page.
- The host cannot provide `sudo` for service stop/start, file install, and
  backup steps.

## Inputs

Decide these before you start:

- `TARGET_REF`: git tag, commit SHA, or branch to deploy.
- `NEW_BIN`: path to a vetted new binary on the host.
- Whether `/opt/velocity-report` also needs to move to `TARGET_REF` so PDF
  generation stays in sync with the Go binary.

Preferred source for `NEW_BIN`: a prebuilt `velocity-report-linux-arm64`
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
echo "listen port: $(systemctl show velocity-report.service -p ExecStart --value 2>/dev/null | grep -oP '(?<=--listen :)\d+' || echo '8080 (default)')"

echo "=== database ==="
ls -la "$VR_DB" 2>/dev/null || echo "no database"
stat --printf='%s bytes\n' "$VR_DB" 2>/dev/null || stat -f '%z bytes' "$VR_DB" 2>/dev/null || true
if [ -f "$VR_DB" ]; then
  "$VR_BIN" migrate status --db-path "$VR_DB" 2>/dev/null || echo "migrate status unavailable"
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
  (`Dirty: false`). A dirty migration is a guardrail — stop and ask.
- Whether `/opt/velocity-report` is present, clean, and at which commit.
  If `git status --short` printed any paths before `(working tree clean)`,
  the checkout is dirty — stop and ask.
- Directory ownership of `/opt/velocity-report` — determines whether later
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

## Build and Transfer (on the dev machine)

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

# Sanity-check the artifact
file velocity-report-linux-arm64
ls -lh velocity-report-linux-arm64
```

The output binary is `velocity-report-linux-arm64` in the repo root.

### Transfer to host

```bash
ssh radar.local 'mkdir -p /tmp/vr'
scp velocity-report-linux-arm64 radar.local:/tmp/vr/
```

## Prepare the New Binary (on the host)

Paste this on the host to verify the transferred artifact:

```bash
export NEW_BIN=/tmp/vr/velocity-report-linux-arm64
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
export NEW_BIN="$PWD/velocity-report-linux-arm64"
"$NEW_BIN" --version
```

If `web/build/index.html` is missing, you need a real web build first:

```bash
make install-web
make build-web
make build-radar-linux
```

## Optional: Sync `/opt/velocity-report`

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
make install-python
if [ "$(id -un)" != "$SERVICE_USER" ]; then
  sudo chown -R "$SERVICE_USER:$SERVICE_USER" /opt/velocity-report/.venv
fi
```

If that checkout is dirty, stop and ask instead of force-resetting it.

## Backup and Stop the Service

Create a rollback point before replacing anything:

```bash
export TS=$(date +%Y%m%d-%H%M%S)
export BACKUP_DIR=/var/lib/velocity-report/backups/$TS

mkdir -p "$BACKUP_DIR"
cp "$VR_SVC" "$BACKUP_DIR/velocity-report.service" 2>/dev/null || sudo cp "$VR_SVC" "$BACKUP_DIR/velocity-report.service"
"$VR_BIN" --version > "$BACKUP_DIR/version.txt" 2>&1 || true

sudo systemctl stop velocity-report.service
systemctl is-active velocity-report.service || true

cp "$VR_BIN" "$BACKUP_DIR/velocity-report" 2>/dev/null || sudo cp "$VR_BIN" "$BACKUP_DIR/velocity-report"
if [ -f "$VR_DB" ]; then
  cp "$VR_DB" "$BACKUP_DIR/sensor_data.db"
fi
```

Do not continue until the service has actually stopped.

## Install and Migrate

Install the new binary in place:

```bash
sudo install -o root -g root -m 0755 "$NEW_BIN" "$VR_BIN"
"$VR_BIN" --version
```

Check migration state before applying anything:

```bash
if [ -f "$VR_DB" ]; then
  RUN_AS "$VR_BIN" migrate status --db-path "$VR_DB"
fi
```

If `Dirty: true` appears, stop and recover the database before proceeding.

Apply migrations only when the database exists:

```bash
if [ -f "$VR_DB" ]; then
  RUN_AS "$VR_BIN" migrate up --db-path "$VR_DB"
  RUN_AS "$VR_BIN" migrate status --db-path "$VR_DB"
fi
```

`RUN_AS` is a no-op when the SSH user matches the service user, and
`sudo -u "$SERVICE_USER"` otherwise. Prefer it over `su - velocity` because
the service user is normally created with a non-login shell.

## Restart and Verify

Bring the service back and verify both systemd and HTTP health:

```bash
sudo systemctl start velocity-report.service
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

## Rollback

If the new binary fails after install or migration, restore the backup you just
made. For schema-changing upgrades, restoring only the binary is usually not
enough; restore the pre-upgrade database too.

```bash
sudo systemctl stop velocity-report.service
sudo install -o root -g root -m 0755 "$BACKUP_DIR/velocity-report" "$VR_BIN"
sudo cp "$BACKUP_DIR/velocity-report.service" "$VR_SVC"
if [ -f "$BACKUP_DIR/sensor_data.db" ]; then
  cp "$BACKUP_DIR/sensor_data.db" "$VR_DB"
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

## Suggested Agent Prompt

Use this with a VS Code SSH agent in Ask mode. The agent does not have direct
command access — it analyses output you paste and gives you blocks to run.

```text
Open docs/radar/operations/remote-host-upgrade-runbook.md and follow it exactly.

You do NOT have direct terminal access. Your job is:
1. Give me the Reconnaissance block to paste. Analyse my output.
2. Ask any follow-up questions before proceeding (dirty checkout, missing
   binary, stub web build, dirty migration, etc.).
3. For each subsequent step, give me a single copy-paste block. Wait for my
   output before moving on.
4. Minimise sudo usage — only request it for service stop/start, file install,
   and backup/restore. Never use sudo for read-only information gathering.
5. Stop and ask if any guardrail condition is triggered.

Upgrade this host to TARGET_REF=<tag-or-sha> without using velocity-deploy.
NEW_BIN is at <path-to-binary-on-host>.
Preserve the existing systemd service configuration unless a mismatch forces a
decision.
```
