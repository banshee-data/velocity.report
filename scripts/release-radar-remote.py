#!/usr/bin/env python3
"""Deploy a release-built velocity-report binary to a remote host over SSH."""

from __future__ import annotations

import argparse
import shlex
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
DEFAULT_LOCAL_BINARY = REPO_ROOT / "image" / "velocity-binaries" / "velocity-report"
DEFAULT_HOST = "velocity.local"
DEFAULT_USER = "pi"
DEFAULT_REMOTE_TEMP_DIR = "/tmp/up"
DEFAULT_REMOTE_BIN = "/usr/local/bin/velocity-report"
DEFAULT_REMOTE_DB_PATH = "/var/lib/velocity-report/sensor_data.db"
DEFAULT_REMOTE_SERVICE = "velocity-report.service"
STAGED_BINARY_NAME = "velocity-report"


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Copy a release-built velocity-report binary to a remote host, swap it "
            "in with a timestamped backup, run migrations, and restart the service."
        )
    )
    parser.add_argument("--host", default=DEFAULT_HOST, help="remote host name")
    parser.add_argument("--user", default=DEFAULT_USER, help="remote SSH user")
    parser.add_argument(
        "--local-binary",
        default=str(DEFAULT_LOCAL_BINARY),
        help="path to the local velocity-report binary to deploy",
    )
    parser.add_argument(
        "--remote-temp-dir",
        default=DEFAULT_REMOTE_TEMP_DIR,
        help="remote staging directory used before the binary swap",
    )
    parser.add_argument(
        "--remote-binary",
        default=DEFAULT_REMOTE_BIN,
        help="installed velocity-report binary path on the remote host",
    )
    parser.add_argument(
        "--remote-db-path",
        default=DEFAULT_REMOTE_DB_PATH,
        help="SQLite database path passed to migrate up on the remote host",
    )
    parser.add_argument(
        "--service",
        default=DEFAULT_REMOTE_SERVICE,
        help="systemd service name to stop and start during deployment",
    )
    return parser.parse_args(argv)


def validate_local_binary(path: Path) -> Path:
    resolved = path.expanduser().resolve()
    if not resolved.is_file():
        raise FileNotFoundError(f"local binary not found: {resolved}")
    if resolved.stat().st_size == 0:
        raise ValueError(f"local binary is empty: {resolved}")
    return resolved


def remote_target(args: argparse.Namespace) -> str:
    return f"{args.user}@{args.host}"


def staged_remote_path(remote_temp_dir: str) -> str:
    trimmed = remote_temp_dir.rstrip("/") or "/"
    if trimmed == "/":
        return f"/{STAGED_BINARY_NAME}"
    return f"{trimmed}/{STAGED_BINARY_NAME}"


def build_remote_deploy_script() -> str:
    return """set -euo pipefail

TMP_DIR="$1"
REMOTE_BIN="$2"
SERVICE="$3"
DB_PATH="$4"
STAGED_PATH="$5"
STAMP="$(date -u +"%Y%m%dT%H%M%SZ")"
BACKUP_PATH="${REMOTE_BIN}.${STAMP}.bak"

rollback() {
    status=$?
    trap - EXIT
    if [[ "$status" -eq 0 ]]; then
        exit 0
    fi

    echo "Remote deploy failed; attempting rollback..." >&2
    if sudo test -f "$BACKUP_PATH"; then
        sudo mv "$BACKUP_PATH" "$REMOTE_BIN"
        sudo systemctl start "$SERVICE" || true
        echo "Rollback restored $REMOTE_BIN from $BACKUP_PATH" >&2
    fi
    exit "$status"
}
trap rollback EXIT

mkdir -p "$TMP_DIR"
if [[ ! -f "$STAGED_PATH" ]]; then
    echo "Staged binary not found: $STAGED_PATH" >&2
    exit 1
fi

sudo systemctl stop "$SERVICE"
if sudo test -f "$REMOTE_BIN"; then
    sudo mv "$REMOTE_BIN" "$BACKUP_PATH"
fi

sudo install -o root -g root -m 755 "$STAGED_PATH" "$REMOTE_BIN"
rm -f "$STAGED_PATH"

"$REMOTE_BIN" version
sudo "$REMOTE_BIN" migrate --db-path "$DB_PATH" up
sudo systemctl start "$SERVICE"
sudo systemctl is-active --quiet "$SERVICE"

trap - EXIT
echo "Deploy complete."
echo "Installed binary: $REMOTE_BIN"
if [[ -f "$BACKUP_PATH" ]]; then
    echo "Backup binary: $BACKUP_PATH"
fi
"""


def format_command(command: list[str]) -> str:
    return " ".join(shlex.quote(part) for part in command)


def run_command(command: list[str], *, input_text: str | None = None) -> None:
    print(f"+ {format_command(command)}")
    subprocess.run(command, check=True, text=True, input=input_text)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(sys.argv[1:] if argv is None else argv)

    try:
        local_binary = validate_local_binary(Path(args.local_binary))
        ssh_target = remote_target(args)
        staged_path = staged_remote_path(args.remote_temp_dir)

        run_command(["ssh", ssh_target, "mkdir", "-p", args.remote_temp_dir])
        run_command(["scp", str(local_binary), f"{ssh_target}:{staged_path}"])
        run_command(
            [
                "ssh",
                ssh_target,
                "bash",
                "-s",
                "--",
                args.remote_temp_dir,
                args.remote_binary,
                args.service,
                args.remote_db_path,
                staged_path,
            ],
            input_text=build_remote_deploy_script(),
        )
    except (FileNotFoundError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    except subprocess.CalledProcessError as exc:
        print(
            f"error: command failed with exit code {exc.returncode}: {format_command(exc.cmd)}",
            file=sys.stderr,
        )
        return exc.returncode or 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
