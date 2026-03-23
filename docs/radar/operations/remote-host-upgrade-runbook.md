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

## Preflight

Run these commands first and keep the output in the terminal scrollback:

```bash
export VR_BIN=/usr/local/bin/velocity-report
export VR_SVC=/etc/systemd/system/velocity-report.service
export VR_DB=/var/lib/velocity-report/sensor_data.db

uname -a
file "$VR_BIN" || true
"$VR_BIN" --version || true
sudo systemctl cat velocity-report.service
sudo systemctl show velocity-report.service -p ExecStart -p WorkingDirectory -p User
sudo systemctl is-active velocity-report.service
df -h /usr/local/bin /var/lib/velocity-report /opt/velocity-report 2>/dev/null || true
```

If the service file shows a custom `--listen :PORT`, keep that port for the
HTTP verification step later. If no `--listen` flag is present, assume `8080`.

## Prepare the New Binary

### Preferred: use a prebuilt artifact

```bash
export NEW_BIN=/tmp/velocity-report-linux-arm64
test -x "$NEW_BIN"
file "$NEW_BIN"
"$NEW_BIN" --version
```

On an ARM64 host, `file "$NEW_BIN"` should report an `ELF 64-bit` `ARM aarch64`
binary.

### Fallback: build on the host

Only use this path if the host already has a suitable checkout plus the build
toolchain. A plain `make build-radar-linux` on a fresh clone can succeed with a
stubbed dashboard, which is not a production build.

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
SERVICE_USER=$(sudo systemctl show velocity-report.service -p User --value)
SERVICE_USER=${SERVICE_USER:-velocity}
cd /opt/velocity-report
sudo -u "$SERVICE_USER" git status --short
sudo -u "$SERVICE_USER" git fetch --tags --prune
sudo -u "$SERVICE_USER" git checkout "$TARGET_REF"
sudo make install-python
sudo chown -R "$SERVICE_USER:$SERVICE_USER" /opt/velocity-report/.venv
```

If that checkout is dirty, stop and ask instead of force-resetting it.

## Backup and Stop the Service

Create a rollback point before replacing anything:

```bash
export TS=$(date +%Y%m%d-%H%M%S)
export BACKUP_DIR=/var/lib/velocity-report/backups/$TS
export SERVICE_USER=$(sudo systemctl show velocity-report.service -p User --value)
export SERVICE_USER=${SERVICE_USER:-velocity}

sudo mkdir -p "$BACKUP_DIR"
sudo cp "$VR_SVC" "$BACKUP_DIR/velocity-report.service"
sudo sh -c "$VR_BIN --version > '$BACKUP_DIR/version.txt' 2>&1 || true"

sudo systemctl stop velocity-report.service
sudo systemctl is-active velocity-report.service || true

sudo cp "$VR_BIN" "$BACKUP_DIR/velocity-report"
if [ -f "$VR_DB" ]; then
  sudo cp "$VR_DB" "$BACKUP_DIR/sensor_data.db"
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
  sudo -u "$SERVICE_USER" "$VR_BIN" migrate status --db-path "$VR_DB"
fi
```

If `Dirty: true` appears, stop and recover the database before proceeding.

Apply migrations only when the database exists:

```bash
if [ -f "$VR_DB" ]; then
  sudo -u "$SERVICE_USER" "$VR_BIN" migrate up --db-path "$VR_DB"
  sudo -u "$SERVICE_USER" "$VR_BIN" migrate status --db-path "$VR_DB"
fi
```

Use `sudo -u "$SERVICE_USER"` rather than `su - velocity` because the service
user is normally created with a non-login shell.

## Restart and Verify

Bring the service back and verify both systemd and HTTP health:

```bash
sudo systemctl start velocity-report.service
sudo systemctl is-active velocity-report.service
sudo systemctl --no-pager --full status velocity-report.service
sudo journalctl -u velocity-report.service -n 50 --no-pager
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
  sudo cp "$BACKUP_DIR/sensor_data.db" "$VR_DB"
  sudo chown "$SERVICE_USER:$SERVICE_USER" "$VR_DB"
fi
sudo systemctl daemon-reload
sudo systemctl start velocity-report.service
sudo systemctl is-active velocity-report.service
sudo journalctl -u velocity-report.service -n 50 --no-pager
```

If `/opt/velocity-report` was also upgraded, roll that checkout back to the
previous ref before retrying report generation.

## Suggested Agent Prompt

Use this with a VS Code SSH agent in Ask mode:

```text
Open docs/radar/operations/remote-host-upgrade-runbook.md and follow it exactly.
Upgrade this host to TARGET_REF=<tag-or-sha> without using velocity-deploy.
Preserve the existing systemd service configuration unless a mismatch forces a
decision. Stop and ask before touching a dirty /opt/velocity-report checkout,
continuing with a dirty migration state, or building from a stubbed web build.
```
