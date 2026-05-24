#!/usr/bin/env python3
"""Serial-harness CLI — exercise the velocity.report serial API directly.

This tool exists to isolate UI bugs from backend bugs when serial-port
discovery or testing misbehaves. Every subcommand makes one HTTP call
against the running Go server (default http://localhost:8080) and prints
a human-readable summary; pass --json for machine output.

Subcommands:
  devices    GET /api/serial/devices                   — pretty-print the merged device list
  diagnose   GET /api/serial/devices?diagnostic=true   — primary vs supplemental vs /dev/ contents
  configs    GET /api/serial/configs                   — show stored serial configurations
  models     GET /api/serial/models                    — list supported sensor models
  test PORT  POST /api/serial/test                     — open a port, send "??", report result
  reload     POST /api/serial/reload                   — hot-reload the active serial config
  smoke      Full create → get → update → delete cycle on a throwaway config

Stdlib only: urllib.request, json, argparse. Runs anywhere python3 lives.

Examples:
  python3 tools/serial-harness.py diagnose
  python3 tools/serial-harness.py --host http://velocity.local:8080 diagnose
  python3 tools/serial-harness.py test /dev/ttySC0 --baud 19200
  python3 tools/serial-harness.py --json devices | jq '.[].port_path'
"""

from __future__ import annotations

import argparse
import json
import re
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Any

# macOS exposes pseudo-terminals in two flavours:
#   masters: /dev/tty<letter><hex>  — e.g. ttyp0, ttyqf, ttyw9 (5 chars total)
#   slaves:  /dev/ttys<digits>      — e.g. ttys000, ttys044
# Neither is a serial port. We suppress them from the "MISSING FROM API"
# highlight because flagging hundreds of them would bury the real signal.
# Note: the lowercase ttys<N> pattern is macOS-specific; Linux serial UARTs
# use uppercase ttyS<N> which our backend regex matches correctly.
MACOS_PTY_PATTERN = re.compile(r"^(tty[a-z][0-9a-f]|ttys[0-9]+)$")

# Linux virtual consoles: /dev/tty0..tty63. Also never serial ports.
LINUX_VC_PATTERN = re.compile(r"^tty[0-9]+$")


# Exit codes — used by make/CI to distinguish "transport broken" from
# "API responded but reported failure".
EXIT_OK = 0
EXIT_API_FAILURE = 1  # API returned success:false, or assertion failed
EXIT_TRANSPORT = 2  # network/HTTP-level error
EXIT_BAD_USAGE = 64  # argparse / shell input error


# ---------------------------------------------------------------------------
# HTTP plumbing
# ---------------------------------------------------------------------------


@dataclass
class Client:
    """Tiny urllib wrapper. One Client per invocation; no connection pooling."""

    host: str
    timeout: float

    def _request(self, method: str, path: str, body: Any = None) -> tuple[int, Any]:
        url = self.host.rstrip("/") + path
        data = None
        headers = {"Accept": "application/json"}
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(url, data=data, method=method, headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                raw = resp.read()
                code = resp.getcode()
        except urllib.error.HTTPError as e:
            # 4xx/5xx — body may still be useful (e.g. our JSON error envelope)
            raw = e.read()
            code = e.code
        except urllib.error.URLError as e:
            print(f"ERROR: transport failure for {url}: {e}", file=sys.stderr)
            sys.exit(EXIT_TRANSPORT)

        if not raw:
            return code, None
        try:
            return code, json.loads(raw)
        except json.JSONDecodeError:
            return code, raw.decode("utf-8", errors="replace")

    def get(self, path: str) -> tuple[int, Any]:
        return self._request("GET", path)

    def post(self, path: str, body: Any = None) -> tuple[int, Any]:
        return self._request("POST", path, body)

    def put(self, path: str, body: Any) -> tuple[int, Any]:
        return self._request("PUT", path, body)

    def delete(self, path: str) -> tuple[int, Any]:
        return self._request("DELETE", path)


# ---------------------------------------------------------------------------
# Formatting helpers
# ---------------------------------------------------------------------------


def _print_json(obj: Any) -> None:
    json.dump(obj, sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")


def _print_list(label: str, items: list[str], empty: str = "(none)") -> None:
    if not items:
        print(f"{label:32s} {empty}")
        return
    print(f"{label:32s} {items[0]}")
    for extra in items[1:]:
        print(f"{'':32s} {extra}")


# ---------------------------------------------------------------------------
# Subcommands
# ---------------------------------------------------------------------------


def cmd_devices(client: Client, args: argparse.Namespace) -> int:
    code, body = client.get("/api/serial/devices")
    if code != 200 or not isinstance(body, list):
        print(f"ERROR: /api/serial/devices returned {code}: {body}", file=sys.stderr)
        return EXIT_API_FAILURE
    if args.json:
        _print_json(body)
        return EXIT_OK
    if not body:
        print(
            "(no devices returned — every port is either configured or undiscoverable)"
        )
        return EXIT_OK
    print(f"Discovered {len(body)} device(s):")
    for d in body:
        print(f"  {d.get('port_path', '?'):30s}  {d.get('friendly_name', '')}")
    return EXIT_OK


def cmd_diagnose(client: Client, args: argparse.Namespace) -> int:
    code, body = client.get("/api/serial/devices?diagnostic=true")
    if code != 200 or not isinstance(body, dict):
        print(f"ERROR: diagnostic mode returned {code}: {body}", file=sys.stderr)
        return EXIT_API_FAILURE
    if args.json:
        _print_json(body)
        return EXIT_OK

    diag = body.get("diagnostic") or {}
    devices = body.get("devices") or []

    print("=== Serial enumeration breakdown ===\n")
    _print_list(
        f"Enumeration ({diag.get('enumeration_source', '?')}):",
        diag.get("enumerated_ports", []),
    )
    if diag.get("enumeration_error"):
        print(f"  └─ enumeration error: {diag['enumeration_error']}")
    _print_list("Supplemental scan:", diag.get("supplemental_ports", []))
    for err in diag.get("supplemental_errors", []) or []:
        print(f"  └─ supplemental error: {err}")
    _print_list("Configured (excluded from list):", diag.get("configured_ports", []))
    _print_list("Raw /dev/ tty*/serial* entries:", diag.get("dev_dir_listing", []))

    # Cross-check: which /dev/ entries did NOT make it into either the primary
    # enumeration or the supplemental list? Those are the "missing from API"
    # — usually because no scan pattern matched. This is the headline output.
    # We skip obvious non-serial entries (macOS PTYs, Linux virtual consoles)
    # because flagging hundreds of them as "missing" would bury the signal.
    seen_paths = set(diag.get("enumerated_ports", []) or [])
    seen_paths.update(diag.get("supplemental_ports", []) or [])
    seen_paths.update(diag.get("configured_ports", []) or [])
    missing = []
    suppressed = 0
    for name in diag.get("dev_dir_listing", []) or []:
        full = f"/dev/{name}"
        if full in seen_paths:
            continue
        if (
            MACOS_PTY_PATTERN.match(name)
            or LINUX_VC_PATTERN.match(name)
            or name == "tty"
        ):
            suppressed += 1
            continue
        missing.append(full)
    print()
    if missing:
        print("MISSING FROM API:")
        for m in missing:
            print(
                f"  {m}  (matched no scan pattern — consider broadening the regex set)"
            )
    else:
        print("All /dev/ entries accounted for in enumeration, scan, or configs.")
    if suppressed:
        print(
            f"  (suppressed {suppressed} non-serial entries: macOS PTYs / Linux virtual consoles)"
        )

    print(f"\nMerged device list: {len(devices)} entry(ies)")
    for d in devices:
        print(f"  {d.get('port_path', '?'):30s}  {d.get('friendly_name', '')}")
    return EXIT_OK


def cmd_configs(client: Client, args: argparse.Namespace) -> int:
    code, body = client.get("/api/serial/configs")
    if code != 200:
        print(f"ERROR: /api/serial/configs returned {code}: {body}", file=sys.stderr)
        return EXIT_API_FAILURE
    if args.json:
        _print_json(body)
        return EXIT_OK
    if not body:
        print("(no serial configurations stored)")
        return EXIT_OK
    for cfg in body:
        enabled = "enabled" if cfg.get("enabled") else "disabled"
        print(
            f"  [{cfg.get('id'):>3}] {cfg.get('name'):30s} {cfg.get('port_path'):30s} "
            f"{cfg.get('baud_rate'):>6} baud  {enabled}"
        )
    return EXIT_OK


def cmd_models(client: Client, args: argparse.Namespace) -> int:
    code, body = client.get("/api/serial/models")
    if code != 200:
        print(f"ERROR: /api/serial/models returned {code}: {body}", file=sys.stderr)
        return EXIT_API_FAILURE
    if args.json:
        _print_json(body)
        return EXIT_OK
    for m in body:
        caps = []
        if m.get("has_doppler"):
            caps.append("Doppler")
        if m.get("has_fmcw"):
            caps.append("FMCW")
        if m.get("has_distance"):
            caps.append("Distance")
        print(f"  {m.get('slug'):12s}  {m.get('display_name')}")
        print(
            f"  {'':12s}  default {m.get('default_baud_rate')} baud; capabilities: {', '.join(caps) or '(none)'}"
        )
        if m.get("supported_baud_rates"):
            print(f"  {'':12s}  supported baud: {m['supported_baud_rates']}")
    return EXIT_OK


def cmd_test(client: Client, args: argparse.Namespace) -> int:
    payload = {
        "port_path": args.port,
        "baud_rate": args.baud,
        "data_bits": 8,
        "stop_bits": 1,
        "parity": "N",
        "timeout_seconds": args.timeout_seconds,
        "auto_correct_baud": args.auto_correct,
    }
    code, body = client.post("/api/serial/test", payload)
    if args.json:
        _print_json({"http_status": code, "response": body})
        # Even in JSON mode, exit non-zero on test failure so shell pipelines compose.
        if not (isinstance(body, dict) and body.get("success")):
            return EXIT_API_FAILURE
        return EXIT_OK

    if code != 200 or not isinstance(body, dict):
        print(f"ERROR: /api/serial/test returned {code}: {body}", file=sys.stderr)
        return EXIT_API_FAILURE

    if body.get("success"):
        bytes_in = body.get("bytes_received", 0)
        dur = body.get("test_duration_ms", 0)
        print(f"PASS: bytes={bytes_in} duration={dur}ms")
        for r in body.get("raw_responses") or []:
            preview = r.get("response", "")
            if len(preview) > 120:
                preview = preview[:120] + "..."
            print(f"  {r.get('command')} → {preview}")
        if body.get("message"):
            print(f"  message: {body['message']}")
        return EXIT_OK

    print(f"FAIL: {body.get('error', 'unknown error')}")
    if body.get("message"):
        print(f"  message: {body['message']}")
    if body.get("suggestion"):
        print(f"  suggestion: {body['suggestion']}")
    return EXIT_API_FAILURE


def cmd_reload(client: Client, args: argparse.Namespace) -> int:
    code, body = client.post("/api/serial/reload")
    if args.json:
        _print_json({"http_status": code, "response": body})
        if code != 200:
            return EXIT_API_FAILURE
        return EXIT_OK
    if code == 503:
        print("SKIP: serial manager not installed on this server (HTTP 503)")
        print(f"  body: {body}")
        return EXIT_OK
    if code != 200:
        print(f"ERROR: /api/serial/reload returned {code}: {body}", file=sys.stderr)
        return EXIT_API_FAILURE
    print(f"OK: {body.get('message', '(no message)')}")
    cfg = body.get("config") or {}
    if cfg:
        print(
            f"  active: {cfg.get('name')} @ {cfg.get('port_path')} ({cfg.get('source')})"
        )
    return EXIT_OK


def cmd_smoke(client: Client, args: argparse.Namespace) -> int:
    """End-to-end CRUD against a throwaway config. Cleans up on success and failure."""
    name = f"harness-smoke-{int(time.time())}"
    payload = {
        "name": name,
        "port_path": args.smoke_port,
        "baud_rate": 19200,
        "data_bits": 8,
        "stop_bits": 1,
        "parity": "N",
        "enabled": False,
        "description": "created by tools/serial-harness.py smoke; safe to delete",
        "sensor_model": "ops243-a",
    }

    created_id = None
    failures: list[str] = []

    def step(label: str, ok: bool, detail: str = "") -> None:
        mark = "✓" if ok else "✗"
        line = f"  {mark} {label}"
        if detail:
            line += f"  ({detail})"
        print(line)
        if not ok:
            failures.append(label)

    try:
        code, body = client.post("/api/serial/configs", payload)
        step("create config", code == 201 and isinstance(body, dict), f"HTTP {code}")
        if isinstance(body, dict):
            created_id = body.get("id")

        if created_id is not None:
            code, body = client.get(f"/api/serial/configs/{created_id}")
            step("get config", code == 200, f"HTTP {code}")

            update = dict(payload, description="updated by smoke test")
            code, body = client.put(f"/api/serial/configs/{created_id}", update)
            step("update config", code == 200, f"HTTP {code}")

        # Don't actually open the port here — caller may not have hardware.
        # The dedicated `test` subcommand exercises that path.
        code, body = client.get("/api/serial/devices")
        step("list devices", code == 200 and isinstance(body, list), f"HTTP {code}")

    finally:
        # Cleanup must run even if a step failed mid-cycle.
        if created_id is not None:
            code, _ = client.delete(f"/api/serial/configs/{created_id}")
            step("delete config", code == 204, f"HTTP {code}")

    if failures:
        print(f"\nFAIL: {len(failures)} step(s) failed: {', '.join(failures)}")
        return EXIT_API_FAILURE
    print("\nALL PASS")
    return EXIT_OK


# ---------------------------------------------------------------------------
# Argparse wiring
# ---------------------------------------------------------------------------


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="serial-harness",
        description=__doc__.strip().splitlines()[0],
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="See file header for full subcommand list and examples.",
    )
    p.add_argument(
        "--host",
        default="http://localhost:8080",
        help="base URL of the velocity.report server (default: %(default)s)",
    )
    p.add_argument(
        "--timeout",
        type=float,
        default=15.0,
        help="HTTP timeout in seconds (default: %(default)s)",
    )
    p.add_argument(
        "--json",
        action="store_true",
        help="emit raw JSON instead of the formatted human output",
    )

    sub = p.add_subparsers(dest="cmd", required=True)

    sub.add_parser("devices", help="list discovered serial devices (merged)")
    sub.add_parser(
        "diagnose", help="diagnostic breakdown of enumeration vs supplemental vs /dev/"
    )
    sub.add_parser("configs", help="list stored serial configurations")
    sub.add_parser("models", help="list supported sensor models")

    t = sub.add_parser("test", help="open a port and probe with '??'")
    t.add_argument("port", help="port path, e.g. /dev/ttySC0")
    t.add_argument(
        "--baud", type=int, default=19200, help="baud rate (default: %(default)s)"
    )
    t.add_argument(
        "--timeout-seconds",
        type=int,
        default=5,
        help="port read timeout in seconds (default: %(default)s)",
    )
    t.add_argument(
        "--auto-correct",
        action="store_true",
        help="ask the device its baud rate via I? and report mismatches",
    )

    sub.add_parser("reload", help="POST /api/serial/reload to swap the live config")

    s = sub.add_parser("smoke", help="end-to-end CRUD against a throwaway config")
    # Port path must start with /dev/tty or /dev/serial to pass server-side
    # validation, but doesn't need to be a real device — we never open it.
    s.add_argument(
        "--smoke-port",
        default="/dev/tty-harness-smoke",
        help="port_path field for the throwaway config (must start "
        "with /dev/tty or /dev/serial; default: %(default)s)",
    )

    return p


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    client = Client(host=args.host, timeout=args.timeout)

    dispatch = {
        "devices": cmd_devices,
        "diagnose": cmd_diagnose,
        "configs": cmd_configs,
        "models": cmd_models,
        "test": cmd_test,
        "reload": cmd_reload,
        "smoke": cmd_smoke,
    }
    handler = dispatch.get(args.cmd)
    if handler is None:
        parser.error(f"unknown subcommand: {args.cmd}")
        return EXIT_BAD_USAGE
    return handler(client, args)


if __name__ == "__main__":
    sys.exit(main())
