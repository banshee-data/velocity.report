#!/usr/bin/env python3
"""List all fields needed to regenerate BACKEND_SURFACE_MATRIX.md.

Scans Go, Proto, Python, and Swift source files and prints a structured
inventory of HTTP endpoints, gRPC methods, DB tables/columns, pipeline
stages, tuning parameters, cmd/ entry points, and debug routes.

Usage:
    python scripts/list-matrix-fields.py           # from repo root
    python scripts/list-matrix-fields.py --json     # machine-readable output
    python scripts/list-matrix-fields.py --coverage # gap analysis
    python scripts/list-matrix-fields.py --trace    # generate LLM tracing checklist
    python scripts/list-matrix-fields.py --trace-continue  # resume from checklist
    python scripts/list-matrix-fields.py --trace-status    # show checklist progress
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import asdict, dataclass, field
from pathlib import Path


# ---------------------------------------------------------------------------
# Data classes
# ---------------------------------------------------------------------------


@dataclass
class Endpoint:
    method: str
    path: str
    handler: str
    file: str


@dataclass
class GRPCMethod:
    name: str
    request: str
    response: str
    streaming: bool
    file: str


@dataclass
class DBTable:
    name: str
    columns: list[str]
    file: str


@dataclass
class PipelineStage:
    package: str
    file: str
    description: str


@dataclass
class TuningParam:
    go_field: str
    json_key: str
    go_type: str


@dataclass
class CmdEntry:
    binary: str
    location: str


@dataclass
class ExternalConsumer:
    file: str
    endpoints: list[str]


@dataclass
class DebugRoute:
    path: str
    handler: str
    file: str


@dataclass
class GoStruct:
    package: str
    file: str
    name: str
    fields: list[str]


@dataclass
class GoFunction:
    package: str
    file: str
    name: str


@dataclass
class ProtoFieldGroup:
    message: str
    group: str
    fields: list[str]
    file: str


@dataclass
class MatrixInventory:
    radar_http: list[Endpoint] = field(default_factory=list)
    lidar_http: list[Endpoint] = field(default_factory=list)
    grpc_methods: list[GRPCMethod] = field(default_factory=list)
    db_tables: list[DBTable] = field(default_factory=list)
    pipeline_stages: list[PipelineStage] = field(default_factory=list)
    tuning_params: list[TuningParam] = field(default_factory=list)
    cmd_entries: list[CmdEntry] = field(default_factory=list)
    pdf_consumers: list[ExternalConsumer] = field(default_factory=list)
    mac_http_consumers: list[ExternalConsumer] = field(default_factory=list)
    debug_routes: list[DebugRoute] = field(default_factory=list)
    computed_structs: list[GoStruct] = field(default_factory=list)
    compare_functions: list[GoFunction] = field(default_factory=list)
    live_track_fields: list[str] = field(default_factory=list)
    classification: list[GoStruct] = field(default_factory=list)
    proto_field_groups: list[ProtoFieldGroup] = field(default_factory=list)
    echart_endpoints: list[Endpoint] = field(default_factory=list)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _read(path: Path) -> str:
    """Read a file, returning empty string if missing."""
    try:
        return path.read_text(encoding="utf-8", errors="replace")
    except FileNotFoundError:
        return ""


def _rel(path: Path, root: Path) -> str:
    """Return path relative to root."""
    try:
        return str(path.relative_to(root))
    except ValueError:
        return str(path)


# ---------------------------------------------------------------------------
# §1  Radar / Main Server HTTP endpoints
# ---------------------------------------------------------------------------

_HANDLE_FUNC_RE = re.compile(
    r's\.mux\.HandleFunc\(\s*"([^"]+)"\s*,\s*(\S+?)\s*\)',
)


def extract_radar_http(root: Path) -> list[Endpoint]:
    src = root / "internal" / "api" / "server.go"
    text = _read(src)
    results: list[Endpoint] = []
    for m in _HANDLE_FUNC_RE.finditer(text):
        path, handler = m.group(1), m.group(2)
        if path.startswith("/debug") or path.startswith("/app") or path == "/":
            continue
        if path == "/favicon.ico":
            continue
        results.append(
            Endpoint(
                method="*",
                path=path,
                handler=handler,
                file=_rel(src, root),
            )
        )
    return results


# ---------------------------------------------------------------------------
# §2  LiDAR Monitor HTTP endpoints
# ---------------------------------------------------------------------------

# Route struct literals: {"<optional METHOD> <PATH>", <HANDLER>}
_ROUTE_STRUCT_RE = re.compile(
    r'\{\s*"((?:(?:GET|POST|PUT|DELETE|PATCH)\s+)?[^"]+)"\s*,'
    r"\s*([a-zA-Z0-9_.()]+)\s*\}",
)

# mux.HandleFunc("<PATH>", <HANDLER>)
_MUX_HANDLE_RE = re.compile(
    r'mux\.HandleFunc\(\s*"([^"]+)"\s*,\s*(\S+?)\s*\)',
)


def _split_method_path(raw: str) -> tuple[str, str]:
    """Split 'GET /foo' into ('GET', '/foo') or ('*', '/foo')."""
    parts = raw.strip().split(None, 1)
    if len(parts) == 2 and parts[0] in {"GET", "POST", "PUT", "DELETE", "PATCH"}:
        return parts[0], parts[1]
    return "*", raw.strip()


def extract_lidar_http(root: Path) -> list[Endpoint]:
    files = [
        root / "internal" / "lidar" / "monitor" / "webserver.go",
        root / "internal" / "lidar" / "monitor" / "track_api.go",
        root / "internal" / "lidar" / "monitor" / "run_track_api.go",
        root / "internal" / "api" / "lidar_labels.go",
    ]
    results: list[Endpoint] = []
    seen: set[str] = set()
    for src in files:
        text = _read(src)
        rel = _rel(src, root)
        for regex in (_ROUTE_STRUCT_RE, _MUX_HANDLE_RE):
            for m in regex.finditer(text):
                raw, handler = m.group(1), m.group(2)
                method, path = _split_method_path(raw)
                if path.startswith("/debug"):
                    continue
                if path == "/":
                    continue
                key = f"{method} {path}"
                if key not in seen:
                    seen.add(key)
                    results.append(
                        Endpoint(
                            method=method,
                            path=path,
                            handler=handler,
                            file=rel,
                        )
                    )
    return results


# ---------------------------------------------------------------------------
# §3  gRPC service methods
# ---------------------------------------------------------------------------

_RPC_RE = re.compile(
    r"rpc\s+(\w+)\s*\(\s*(\w+)\s*\)\s*returns\s*\(\s*(stream\s+)?(\w+)\s*\)\s*;",
)


def extract_grpc(root: Path) -> list[GRPCMethod]:
    src = root / "proto" / "velocity_visualiser" / "v1" / "visualiser.proto"
    text = _read(src)
    results: list[GRPCMethod] = []
    for m in _RPC_RE.finditer(text):
        results.append(
            GRPCMethod(
                name=m.group(1),
                request=m.group(2),
                response=m.group(4),
                streaming=bool(m.group(3)),
                file=_rel(src, root),
            )
        )
    return results


# ---------------------------------------------------------------------------
# §4-5  Database tables and columns
# ---------------------------------------------------------------------------

_CREATE_TABLE_RE = re.compile(
    r'CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?["`]?(\w+)["`]?\s*\(',
    re.IGNORECASE,
)

_COLUMN_RE = re.compile(
    r'^\s*[,]?\s*["`]?(\w+)["`]?\s+'
    r"(INTEGER|TEXT|REAL|BLOB|BOOLEAN|DATETIME|TIMESTAMP|VARCHAR|NUMERIC)",
    re.IGNORECASE | re.MULTILINE,
)


def extract_db_tables(root: Path) -> list[DBTable]:
    schema = root / "internal" / "db" / "schema.sql"
    text = _read(schema)
    if not text:
        return []

    # Split on CREATE TABLE to process each table block
    blocks = re.split(r"(?=CREATE\s+TABLE)", text, flags=re.IGNORECASE)
    results: list[DBTable] = []
    for block in blocks:
        tm = _CREATE_TABLE_RE.match(block)
        if not tm:
            continue
        table_name = tm.group(1)
        # Find the parenthesised body
        paren_start = block.find("(")
        if paren_start == -1:
            continue
        # Extract columns from the body
        body = block[paren_start:]
        columns = [
            cm.group(1)
            for cm in _COLUMN_RE.finditer(body)
            if cm.group(1).upper()
            not in {
                "PRIMARY",
                "UNIQUE",
                "FOREIGN",
                "CHECK",
                "CONSTRAINT",
                "CREATE",
                "INDEX",
                "TABLE",
                "NOT",
                "NULL",
                "DEFAULT",
            }
        ]
        results.append(
            DBTable(
                name=table_name,
                columns=columns,
                file=_rel(schema, root),
            )
        )

    # Also scan migrations for additional tables
    migrations_dir = root / "internal" / "db" / "migrations"
    if migrations_dir.is_dir():
        for mig in sorted(migrations_dir.glob("*.up.sql")):
            mig_text = _read(mig)
            for tm in _CREATE_TABLE_RE.finditer(mig_text):
                table_name = tm.group(1)
                if any(t.name == table_name for t in results):
                    continue
                results.append(
                    DBTable(
                        name=table_name,
                        columns=[],
                        file=_rel(mig, root),
                    )
                )

    return results


# ---------------------------------------------------------------------------
# §6  Pipeline stages
# ---------------------------------------------------------------------------

_PIPELINE_DIRS = [
    ("l2frames", "L2 Frame Builder"),
    ("l3grid", "L3 Background + Foreground"),
    ("l4perception", "L4 Clustering (DBSCAN)"),
    ("l5tracks", "L5 Tracking (Kalman)"),
    ("l6eval", "L6 Evaluation"),
    ("l6objects", "L6 Objects / Quality"),
    ("adapters", "L6 Ground-Truth Adapter"),
]


def extract_pipeline_stages(root: Path) -> list[PipelineStage]:
    results: list[PipelineStage] = []
    base = root / "internal" / "lidar"
    for dirname, desc in _PIPELINE_DIRS:
        pkg_dir = base / dirname
        if pkg_dir.is_dir():
            go_files = sorted(pkg_dir.glob("*.go"))
            go_files = [f for f in go_files if not f.name.endswith("_test.go")]
            for gf in go_files:
                results.append(
                    PipelineStage(
                        package=dirname,
                        file=_rel(gf, root),
                        description=desc,
                    )
                )
    return results


# ---------------------------------------------------------------------------
# §10  Tuning parameters
# ---------------------------------------------------------------------------

_TUNING_FIELD_RE = re.compile(
    r'(\w+)\s+\*(\w+)\s+`json:"(\w+)(?:,omitempty)?"`',
)


def extract_tuning_params(root: Path) -> list[TuningParam]:
    src = root / "internal" / "config" / "tuning.go"
    text = _read(src)
    results: list[TuningParam] = []
    for m in _TUNING_FIELD_RE.finditer(text):
        results.append(
            TuningParam(
                go_field=m.group(1),
                json_key=m.group(3),
                go_type=m.group(2),
            )
        )
    return results


# ---------------------------------------------------------------------------
# §16  cmd/ entry points
# ---------------------------------------------------------------------------


def extract_cmd_entries(root: Path) -> list[CmdEntry]:
    """Find cmd/ entry points by looking for func main() declarations."""
    cmd_dir = root / "cmd"
    if not cmd_dir.is_dir():
        return []
    results: list[CmdEntry] = []
    for go_file in sorted(cmd_dir.rglob("*.go")):
        if go_file.name.endswith("_test.go"):
            continue
        text = _read(go_file)
        if "func main()" not in text:
            continue
        rel = _rel(go_file, root)
        # Derive binary name: use grandparent if under cmd/tools/<name>/
        parts = go_file.relative_to(cmd_dir).parts
        if len(parts) >= 2 and parts[0] == "tools":
            binary = parts[1]  # e.g. gen-vrlog
        elif len(parts) >= 2:
            binary = parts[0]  # e.g. radar, sweep
        else:
            binary = go_file.stem
        results.append(CmdEntry(binary=binary, location=rel))
    # Deduplicate by binary name (keep first)
    seen: set[str] = set()
    deduped: list[CmdEntry] = []
    for e in results:
        if e.binary not in seen:
            seen.add(e.binary)
            deduped.append(e)
    return deduped


# ---------------------------------------------------------------------------
# §11  PDF generator consumers (Python)
# ---------------------------------------------------------------------------

_PY_REQUEST_RE = re.compile(
    r"requests\.(get|post|put|delete)\(\s*"
    r'(?:f"[^"]*"|"[^"]*"|[a-zA-Z0-9_.]+)',
    re.IGNORECASE,
)

_PY_URL_RE = re.compile(
    r"(?:self\.(?:base_url|api_url)|base_url)\s*"
    r'(?:\+\s*f?"|=\s*f?")'
    r'([^"]+)"',
)

_PY_FSTRING_URL_RE = re.compile(
    r'f"[^"]*(/api/[^"{}]*(?:\{[^}]*\}[^"{}]*)*)[^"]*"',
)


def extract_pdf_consumers(root: Path) -> list[ExternalConsumer]:
    pdf_dir = root / "tools" / "pdf-generator" / "pdf_generator" / "core"
    if not pdf_dir.is_dir():
        return []
    results: list[ExternalConsumer] = []
    for py_file in sorted(pdf_dir.glob("*.py")):
        if py_file.name.startswith("__"):
            continue
        text = _read(py_file)
        endpoints: list[str] = []
        for m in _PY_FSTRING_URL_RE.finditer(text):
            ep = m.group(1).strip()
            if ep and ep not in endpoints:
                endpoints.append(ep)
        # Also look for url assignments with /api/ paths
        for m in _PY_URL_RE.finditer(text):
            ep = m.group(1).strip()
            if "/api/" in ep and ep not in endpoints:
                endpoints.append(ep)
        if endpoints:
            results.append(
                ExternalConsumer(
                    file=_rel(py_file, root),
                    endpoints=endpoints,
                )
            )
    return results


# ---------------------------------------------------------------------------
# §12  macOS visualiser HTTP consumers (Swift)
# ---------------------------------------------------------------------------

_SWIFT_URL_RE = re.compile(
    r'appendingPathComponent\("(api/[^"]+)"\)',
)


def extract_mac_http_consumers(root: Path) -> list[ExternalConsumer]:
    labelling_dir = (
        root / "tools" / "visualiser-macos" / "VelocityVisualiser" / "Labelling"
    )
    grpc_dir = root / "tools" / "visualiser-macos" / "VelocityVisualiser" / "gRPC"
    results: list[ExternalConsumer] = []
    for search_dir in (labelling_dir, grpc_dir):
        if not search_dir.is_dir():
            continue
        for swift_file in sorted(search_dir.glob("*.swift")):
            text = _read(swift_file)
            endpoints: list[str] = []
            for m in _SWIFT_URL_RE.finditer(text):
                ep = "/" + m.group(1).lstrip("/")
                if ep not in endpoints:
                    endpoints.append(ep)
            if endpoints:
                results.append(
                    ExternalConsumer(
                        file=_rel(swift_file, root),
                        endpoints=endpoints,
                    )
                )
    return results


# ---------------------------------------------------------------------------
# §18  Debug routes
# ---------------------------------------------------------------------------


def extract_debug_routes(root: Path) -> list[DebugRoute]:
    files = [
        root / "internal" / "lidar" / "monitor" / "webserver.go",
        root / "internal" / "api" / "server.go",
    ]
    results: list[DebugRoute] = []
    for src in files:
        text = _read(src)
        rel = _rel(src, root)
        for regex in (_ROUTE_STRUCT_RE, _MUX_HANDLE_RE):
            for m in regex.finditer(text):
                raw, handler = m.group(1), m.group(2)
                _, path = _split_method_path(raw)
                if "/debug" in path:
                    results.append(
                        DebugRoute(
                            path=path,
                            handler=handler,
                            file=rel,
                        )
                    )

    # Radar server admin routes registered via tsweb Debugger in db.go
    # and serialmux.go — these use debug.Handle() not mux.HandleFunc()
    _TSWEB_ROUTES = [
        ("/debug/pprof/*", "tsweb.Debugger", "internal/db/db.go"),
        ("/debug/db-stats", "db.AttachAdminRoutes", "internal/db/db.go"),
        ("/debug/backup", "db.AttachAdminRoutes", "internal/db/db.go"),
        ("/debug/tailsql/*", "db.AttachAdminRoutes", "internal/db/db.go"),
        (
            "/debug/send-command",
            "serialmux.AttachAdminRoutes",
            "internal/serialmux/serialmux.go",
        ),
        (
            "/debug/tail",
            "serialmux.AttachAdminRoutes",
            "internal/serialmux/serialmux.go",
        ),
    ]
    for path, handler, file in _TSWEB_ROUTES:
        results.append(DebugRoute(path=path, handler=handler, file=file))

    # Static/SPA routes on radar server (not /debug/ but still admin-like)
    _STATIC_ROUTES = [
        ("/favicon.ico", "staticHandler", "internal/api/server.go"),
        ("/app/*", "SPA handler", "internal/api/server.go"),
        ("/", "redirect → /app/", "internal/api/server.go"),
    ]
    for path, handler, file in _STATIC_ROUTES:
        results.append(DebugRoute(path=path, handler=handler, file=file))

    return results


# ---------------------------------------------------------------------------
# §7  Computed-but-not-persisted structs
# ---------------------------------------------------------------------------

_STRUCT_RE = re.compile(
    r"type\s+(\w+)\s+struct\s*\{([^}]*)\}",
    re.DOTALL,
)

_STRUCT_FIELD_RE = re.compile(
    r"^\s+(\w+)\s+",
    re.MULTILINE,
)

# File → struct names of interest
_COMPUTED_STRUCT_TARGETS: list[tuple[str, list[str]]] = [
    (
        "internal/lidar/l6objects/quality.go",
        [
            "NoiseCoverageMetrics",
            "TrainingDatasetSummary",
            "RunStatistics",
            "TrackQualityMetrics",
        ],
    ),
    (
        "internal/lidar/l6objects/features.go",
        ["TrackFeatures", "ClusterFeatures"],
    ),
    (
        "internal/lidar/l3grid/foreground.go",
        ["FrameMetrics"],
    ),
    (
        "internal/lidar/l5tracks/tracking.go",
        ["TrackAlignmentMetrics"],
    ),
    (
        "internal/lidar/sweep/runner.go",
        ["ComboResult"],
    ),
]


def _extract_struct_fields(text: str, name: str) -> list[str]:
    """Extract field names from a Go struct definition."""
    pattern = re.compile(
        rf"type\s+{re.escape(name)}\s+struct\s*\{{([^}}]*)\}}",
        re.DOTALL,
    )
    m = pattern.search(text)
    if not m:
        return []
    body = m.group(1)
    fields: list[str] = []
    for line in body.splitlines():
        line = line.strip()
        if not line or line.startswith("//") or line.startswith("*"):
            continue
        # Embedded struct (single word, capitalised, no type following)
        parts = line.split()
        if len(parts) >= 2 and parts[0][0].isupper():
            fields.append(parts[0])
        elif len(parts) == 1 and parts[0][0].isupper():
            fields.append(f"[embed:{parts[0]}]")
    return fields


def extract_computed_structs(root: Path) -> list[GoStruct]:
    results: list[GoStruct] = []
    for rel_path, struct_names in _COMPUTED_STRUCT_TARGETS:
        src = root / rel_path
        text = _read(src)
        pkg = rel_path.rsplit("/", 1)[-1].replace(".go", "")
        for name in struct_names:
            fields = _extract_struct_fields(text, name)
            results.append(
                GoStruct(
                    package=pkg,
                    file=rel_path,
                    name=name,
                    fields=fields,
                )
            )
    return results


# ---------------------------------------------------------------------------
# §8  Comparison logic functions
# ---------------------------------------------------------------------------

_FUNC_RE = re.compile(r"func\s+(?:\(\w+\s+\*?\w+\)\s+)?(\w+)\(")


def extract_compare_functions(root: Path) -> list[GoFunction]:
    src = root / "internal" / "lidar" / "storage" / "sqlite" / "analysis_run_compare.go"
    text = _read(src)
    if not text:
        return []
    results: list[GoFunction] = []
    for m in _FUNC_RE.finditer(text):
        results.append(
            GoFunction(
                package="sqlite",
                file=_rel(src, root),
                name=m.group(1),
            )
        )
    return results


# ---------------------------------------------------------------------------
# §9  Live track fields (from TrackedObject struct)
# ---------------------------------------------------------------------------


def extract_live_track_fields(root: Path) -> list[str]:
    src = root / "internal" / "lidar" / "l5tracks" / "tracking.go"
    text = _read(src)
    return _extract_struct_fields(text, "TrackedObject")


# ---------------------------------------------------------------------------
# §13  Classification pipeline
# ---------------------------------------------------------------------------

_CONST_BLOCK_RE = re.compile(
    r"const\s*\(\s*(.*?)\s*\)",
    re.DOTALL,
)
_CLASS_CONST_RE = re.compile(
    r'Class\w+\s+ObjectClass\s*=\s*"(\w+)"',
)


def extract_classification(root: Path) -> list[GoStruct]:
    src = root / "internal" / "lidar" / "l6objects" / "classification.go"
    text = _read(src)
    if not text:
        return []
    rel = _rel(src, root)
    results: list[GoStruct] = []

    # ObjectClass constants
    classes: list[str] = []
    for m in _CONST_BLOCK_RE.finditer(text):
        for cm in _CLASS_CONST_RE.finditer(m.group(1)):
            classes.append(cm.group(1))
    if classes:
        results.append(
            GoStruct(package="l6objects", file=rel, name="ObjectClass", fields=classes)
        )

    for struct_name in [
        "ClassificationResult",
        "ClassificationFeatures",
        "TrackClassifier",
    ]:
        fields = _extract_struct_fields(text, struct_name)
        results.append(
            GoStruct(package="l6objects", file=rel, name=struct_name, fields=fields)
        )

    return results


# ---------------------------------------------------------------------------
# §14  FrameBundle proto fields
# ---------------------------------------------------------------------------

_PROTO_MSG_RE = re.compile(
    r"message\s+(\w+)\s*\{(.*?)\}",
    re.DOTALL,
)
_PROTO_FIELD_RE = re.compile(
    r"^\s+(?:repeated\s+)?(\w+)\s+(\w+)\s*=\s*\d+;",
    re.MULTILINE,
)


def extract_proto_fields(root: Path) -> list[ProtoFieldGroup]:
    src = root / "proto" / "velocity_visualiser" / "v1" / "visualiser.proto"
    text = _read(src)
    if not text:
        return []
    rel = _rel(src, root)
    results: list[ProtoFieldGroup] = []
    for m in _PROTO_MSG_RE.finditer(text):
        msg_name = m.group(1)
        body = m.group(2)
        fields = [fm.group(2) for fm in _PROTO_FIELD_RE.finditer(body)]
        if fields:
            results.append(
                ProtoFieldGroup(
                    message=msg_name,
                    group=msg_name,
                    fields=fields,
                    file=rel,
                )
            )
    return results


# ---------------------------------------------------------------------------
# §15  ECharts dashboard endpoints (subset of §2 chart/ routes)
# ---------------------------------------------------------------------------


def extract_echart_endpoints(lidar_http: list[Endpoint]) -> list[Endpoint]:
    return [ep for ep in lidar_http if "/chart/" in ep.path]


# ---------------------------------------------------------------------------
# Reporting
# ---------------------------------------------------------------------------


def print_text_report(inv: MatrixInventory) -> None:
    """Print human-readable summary."""

    def _header(title: str, section: str) -> None:
        print(f"\n{'=' * 72}")
        print(f"  {section}  {title}")
        print(f"{'=' * 72}")

    _header("HTTP Endpoints — Radar / Main Server", "§1")
    for ep in inv.radar_http:
        print(f"  {ep.method:6s} {ep.path:<45s}  → {ep.handler}  ({ep.file})")
    print(f"  Total: {len(inv.radar_http)}")

    _header("HTTP Endpoints — LiDAR Monitor", "§2")
    for ep in inv.lidar_http:
        print(f"  {ep.method:6s} {ep.path:<45s}  → {ep.handler}  ({ep.file})")
    print(f"  Total: {len(inv.lidar_http)}")

    _header("gRPC Service Methods", "§3")
    for m in inv.grpc_methods:
        stream = " (stream)" if m.streaming else ""
        print(f"  rpc {m.name}({m.request}) → {m.response}{stream}")
    print(f"  Total: {len(inv.grpc_methods)}")

    _header("Database Tables & Columns", "§4-5")
    total_cols = 0
    for t in inv.db_tables:
        print(f"  {t.name} ({len(t.columns)} columns)  ({t.file})")
        for col in t.columns:
            print(f"    - {col}")
        total_cols += len(t.columns)
    print(f"  Tables: {len(inv.db_tables)}  Columns: {total_cols}")

    _header("Pipeline Stages", "§6")
    seen_pkg: set[str] = set()
    for s in inv.pipeline_stages:
        if s.package not in seen_pkg:
            seen_pkg.add(s.package)
            print(f"  {s.package}: {s.description}")
        print(f"    {s.file}")

    _header("Tuning Parameters", "§10")
    for p in inv.tuning_params:
        print(f"  {p.go_field:<40s}  json:{p.json_key:<35s}  *{p.go_type}")
    print(f"  Total: {len(inv.tuning_params)}")

    _header("cmd/ Entry Points", "§16")
    for e in inv.cmd_entries:
        print(f"  {e.binary:<25s}  {e.location}")
    print(f"  Total: {len(inv.cmd_entries)}")

    _header("PDF Generator Consumers (Python)", "§11")
    for c in inv.pdf_consumers:
        print(f"  {c.file}")
        for ep in c.endpoints:
            print(f"    → {ep}")

    _header("macOS Visualiser HTTP Consumers (Swift)", "§12")
    for c in inv.mac_http_consumers:
        print(f"  {c.file}")
        for ep in c.endpoints:
            print(f"    → {ep}")

    _header("Debug / Admin Routes", "§18")
    for r in inv.debug_routes:
        print(f"  {r.path:<50s}  → {r.handler}  ({r.file})")
    print(f"  Total: {len(inv.debug_routes)}")

    _header("Computed-but-Not-Persisted Structs", "§7")
    for s in inv.computed_structs:
        print(f"  {s.name} ({len(s.fields)} fields)  ({s.file})")
        for f in s.fields:
            print(f"    - {f}")

    _header("Comparison Logic Functions", "§8")
    for f in inv.compare_functions:
        print(f"  {f.name}()  ({f.file})")

    _header("Live Track Fields (TrackedObject)", "§9")
    for f in inv.live_track_fields:
        print(f"  - {f}")
    print(f"  Total: {len(inv.live_track_fields)}")

    _header("Classification Pipeline", "§13")
    for s in inv.classification:
        print(f"  {s.name} ({len(s.fields)} items)  ({s.file})")
        for f in s.fields:
            print(f"    - {f}")

    _header("Proto Messages (FrameBundle + sub-messages)", "§14")
    for g in inv.proto_field_groups:
        print(f"  {g.message} ({len(g.fields)} fields)")
        for f in g.fields:
            print(f"    - {f}")

    _header("ECharts Dashboard Endpoints", "§15")
    for ep in inv.echart_endpoints:
        print(f"  {ep.method:6s} {ep.path:<45s}  → {ep.handler}")
    print(f"  Total: {len(inv.echart_endpoints)}")

    print(f"\n{'=' * 72}")
    print("  TOTALS")
    print(f"{'=' * 72}")
    print(f"  §1  Radar HTTP endpoints:    {len(inv.radar_http)}")
    print(f"  §2  LiDAR HTTP endpoints:    {len(inv.lidar_http)}")
    print(f"  §3  gRPC methods:            {len(inv.grpc_methods)}")
    print(f"  §4  DB tables:               {len(inv.db_tables)}")
    print(f"  §5  DB columns:              {total_cols}")
    print(f"  §6  Pipeline stage files:    {len(inv.pipeline_stages)}")
    print(f"  §7  Computed structs:         {len(inv.computed_structs)}")
    print(f"  §8  Compare functions:        {len(inv.compare_functions)}")
    print(f"  §9  Live track fields:        {len(inv.live_track_fields)}")
    print(f"  §10 Tuning parameters:       {len(inv.tuning_params)}")
    print(f"  §13 Classification items:     {len(inv.classification)}")
    print(f"  §14 Proto messages:           {len(inv.proto_field_groups)}")
    print(f"  §15 ECharts endpoints:        {len(inv.echart_endpoints)}")
    print(f"  §16 cmd/ entry points:       {len(inv.cmd_entries)}")
    print(f"  §18 Debug routes:            {len(inv.debug_routes)}")


def print_json_report(inv: MatrixInventory) -> None:
    """Print machine-readable JSON."""
    print(json.dumps(asdict(inv), indent=2))


# ---------------------------------------------------------------------------
# Generation hints — what the matrix needs that requires human audit
# ---------------------------------------------------------------------------

GENERATION_HINTS: dict[str, dict[str, object]] = {
    "§1": {
        "title": "HTTP API Endpoints — Radar / Main Server",
        "auto_extractable": True,
        "matrix_rows": 14,
        "notes": (
            "Script extracts individual HandleFunc paths. Matrix groups "
            "multi-method endpoints (e.g. GET/PUT/DEL on one row). "
            "Surface marks (DB/Web/PDF/Mac) require tracing handler → "
            "DB calls and checking which frontends call each endpoint."
        ),
        "source_files": ["internal/api/server.go"],
        "matrix_columns": [
            "Folder",
            "File",
            "Endpoint",
            "DB",
            "Web",
            "PDF",
            "Mac",
        ],
    },
    "§2": {
        "title": "HTTP API Endpoints — LiDAR Monitor",
        "auto_extractable": True,
        "matrix_rows": 77,
        "notes": (
            "Script finds ~67 routes from route structs and HandleFunc. "
            "Gap: run_track_api.go sub-routes dispatched via URL path "
            "parsing (runs/{id}/tracks, .../label, .../flags, .../compare, "
            ".../labelling-progress) are not captured as separate endpoints. "
            "Also missed-regions and sweeps history routes."
        ),
        "source_files": [
            "internal/lidar/monitor/webserver.go",
            "internal/lidar/monitor/track_api.go",
            "internal/lidar/monitor/run_track_api.go",
            "internal/api/lidar_labels.go",
        ],
        "matrix_columns": [
            "Layer",
            "File",
            "Endpoint",
            "DB",
            "Web",
            "PDF",
            "Mac",
        ],
    },
    "§3": {
        "title": "gRPC Service — macOS Visualiser",
        "auto_extractable": True,
        "matrix_rows": 9,
        "notes": "Exact match. Layer grouping (Streaming/Playback/Debug/Recording) "
        "is manual curation.",
        "source_files": [
            "proto/velocity_visualiser/v1/visualiser.proto",
        ],
    },
    "§4": {
        "title": "Database Tables",
        "auto_extractable": True,
        "matrix_rows": 22,
        "notes": (
            "Script finds ~26 tables (includes migration-only tables). "
            "Matrix has 22 curated rows with Layer grouping and Notes column. "
            "Surface marks need checking: which tables are queried by Web "
            "handlers, PDF generator, Mac gRPC."
        ),
        "source_files": ["internal/db/schema.sql"],
    },
    "§5": {
        "title": "Database Fields — All Columns",
        "auto_extractable": True,
        "matrix_rows": "~300",
        "notes": (
            "Script extracts column names and types from schema.sql. "
            "Matrix adds PK/FK/STORED/UNIQUE type annotations and "
            "surface marks per column. The 🗑️ deprecated marks and 🔶 "
            "partially-wired marks require code-path tracing."
        ),
        "source_files": ["internal/db/schema.sql"],
    },
    "§6": {
        "title": "Pipeline Stages",
        "auto_extractable": "partial",
        "matrix_rows": 13,
        "notes": (
            "Script lists all .go files in l2-l6 dirs (~47 files). "
            "Matrix has 13 curated rows with specific stage descriptions "
            "and field counts. Curation needed: pick the primary file for "
            "each pipeline stage, write the stage description, determine "
            "surface marks."
        ),
        "curated_stages": [
            ("l2frames", "frame_builder.go", "L2 Frame Builder (UDP → point clouds)"),
            (
                "l3grid",
                "background.go",
                "L3 Background Grid (foreground/background)",
            ),
            ("l3grid", "foreground.go", "L3 FrameMetrics (foreground fraction)"),
            ("l4perception", "dbscan.go", "L4 Clustering (DBSCAN → world clusters)"),
            ("l5tracks", "tracking.go", "L5 Tracking (Kalman → tracked objects)"),
            (
                "l5tracks",
                "tracking.go",
                "L5 TrackingMetrics (fragmentation, jitter)",
            ),
            ("adapters", "ground_truth.go", "L6 Evaluation (quality metrics)"),
            ("l6objects", "quality.go", "L6 RunStatistics (12 fields)"),
            ("l6objects", "quality.go", "L6 TrackQualityMetrics (8 fields)"),
            ("l6objects", "quality.go", "L6 NoiseCoverageMetrics (7 fields)"),
            ("l6objects", "quality.go", "L6 TrainingDatasetSummary (7 fields)"),
            ("l6objects", "features.go", "L6 TrackFeatures (20 features)"),
            ("l6objects", "features.go", "L6 ClusterFeatures (10 features)"),
        ],
        "source_dirs": [
            "internal/lidar/l2frames",
            "internal/lidar/l3grid",
            "internal/lidar/l4perception",
            "internal/lidar/l5tracks",
            "internal/lidar/l6eval",
            "internal/lidar/l6objects",
            "internal/lidar/adapters",
        ],
    },
    "§7": {
        "title": "Go Structs — Computed but Not Persisted",
        "auto_extractable": True,
        "matrix_rows": 7,
        "notes": (
            "Script now extracts struct definitions and field counts. "
            "Notes column needs human review: why it's not persisted, "
            "links to remediation plans."
        ),
    },
    "§8": {
        "title": "Comparison Logic — No Triggering Endpoint",
        "auto_extractable": True,
        "matrix_rows": 4,
        "notes": (
            "Script extracts function names from analysis_run_compare.go. "
            "Matrix also includes is_split_candidate and is_merge_candidate "
            "flags which are DB column flags, not functions."
        ),
        "extra_items": [
            "is_split_candidate flag (written but not triggerable)",
            "is_merge_candidate flag (written but not triggerable)",
        ],
    },
    "§9": {
        "title": "Live Track Fields — Fully Wired (Reference)",
        "auto_extractable": True,
        "matrix_rows": 16,
        "notes": (
            "Script extracts TrackedObject fields. Matrix has 16 curated "
            "rows showing only the fields that flow through all applicable "
            "surfaces (a subset of the full struct). The selection of "
            "which fields are 'fully wired' requires human verification."
        ),
    },
    "§10": {
        "title": "Tuning Parameters",
        "auto_extractable": True,
        "matrix_rows": 4,
        "notes": (
            "Script extracts 45 individual json-tagged params. Matrix "
            "groups them: L3 Background (8), L4 Perception (3), L5 "
            "Tracker (14), plus defaults row. Grouping is by Go struct "
            "nesting in tuning.go."
        ),
    },
    "§11": {
        "title": "PDF Generator — Python Surfaces",
        "auto_extractable": "partial",
        "matrix_rows": 6,
        "notes": (
            "Script finds endpoint URLs from api_client.py. Matrix has "
            "6 curated rows covering api_client.py, chart_builder.py, "
            "document_builder.py, and map_utils.py — some rows describe "
            "components rather than specific HTTP calls."
        ),
    },
    "§12": {
        "title": "macOS Visualiser — Swift Surfaces",
        "auto_extractable": "partial",
        "matrix_rows": 6,
        "notes": (
            "Script finds appendingPathComponent() URLs. Matrix has 6 "
            "curated rows describing consumer roles (StreamFrames "
            "subscriber, playback controls, point cloud rendering, etc) "
            "not just HTTP paths."
        ),
    },
    "§13": {
        "title": "Classification Pipeline — Fully Wired (Reference)",
        "auto_extractable": True,
        "matrix_rows": 4,
        "notes": (
            "Script now extracts ObjectClass constants, ClassificationResult, "
            "ClassificationFeatures, TrackClassifier structs. Notes paragraph "
            "about data flow requires human curation."
        ),
    },
    "§14": {
        "title": "FrameBundle — macOS-Only Proto Fields",
        "auto_extractable": True,
        "matrix_rows": 12,
        "notes": (
            "Script extracts all proto message definitions. Matrix groups "
            "FrameBundle sub-messages into 12 logical field groups with "
            "field counts. The grouping and field-count aggregation need "
            "curation: e.g. 'Point cloud (x/y/z/i/c)' = 7 fields."
        ),
    },
    "§15": {
        "title": "ECharts Dashboard Endpoints",
        "auto_extractable": True,
        "matrix_rows": 5,
        "notes": (
            "Subset of §2 endpoints matching /chart/ paths. Script "
            "extracts these from the LiDAR HTTP routes. Matrix adds "
            "'Data Source' and 'Method' columns."
        ),
    },
    "§16": {
        "title": "cmd/ Entry Points",
        "auto_extractable": True,
        "matrix_rows": 11,
        "notes": ("Exact match. The 'Consumers' description column is human-curated."),
    },
    "§17": {
        "title": "Speed Percentile Columns — Design Debt",
        "auto_extractable": False,
        "matrix_rows": 0,
        "notes": (
            "Narrative section — no table to extract. Describes 6 "
            "deprecated columns (p50/p85/p95 on lidar_tracks + "
            "lidar_run_tracks) and links to migration/removal plans."
        ),
    },
    "§18": {
        "title": "Debug / Admin Routes",
        "auto_extractable": True,
        "matrix_rows": 19,
        "notes": (
            "Script now combines: LiDAR monitor /debug/ routes from "
            "route structs, radar server routes from db.AttachAdminRoutes "
            "(tsweb debugger: pprof, db-stats, backup, tailsql) and "
            "serialmux (send-command, tail). Plus static/SPA routes."
        ),
    },
    "summary": {
        "title": "Summary + Gap Summary Tables",
        "auto_extractable": "partial",
        "notes": (
            "Counts-by-surface table is derivable by counting surface "
            "marks across all section tables. Gap summary table requires "
            "human analysis: identifying schema columns never written, "
            "fields live-only (Mac but not DB), structs computed but not "
            "persisted, transient pipeline metrics, logic with no "
            "triggering endpoint, deprecated columns."
        ),
    },
    "surface_marks": {
        "title": "Surface Marks (✅/📋/🔶/🗑️/—)",
        "auto_extractable": False,
        "notes": (
            "The core value of the matrix. Determining which surface "
            "consumes each item requires tracing code paths:\n"
            "  DB: handler → SQL INSERT/UPDATE/SELECT\n"
            "  Web: Svelte fetch() calls → API endpoint → response fields\n"
            "  PDF: Python api_client.py → endpoint → data used in LaTeX\n"
            "  Mac: Swift appendingPathComponent / gRPC proto fields\n"
            "This is fundamentally a human audit task."
        ),
    },
}


def print_coverage_report(inv: MatrixInventory) -> None:
    """Print a gap analysis comparing script output to matrix requirements."""
    total_cols = sum(len(t.columns) for t in inv.db_tables)

    sections = [
        ("§1", "Radar HTTP endpoints", len(inv.radar_http), 14),
        ("§2", "LiDAR HTTP endpoints", len(inv.lidar_http), 77),
        ("§3", "gRPC methods", len(inv.grpc_methods), 9),
        ("§4", "DB tables", len(inv.db_tables), 22),
        ("§5", "DB columns", total_cols, 300),
        ("§6", "Pipeline stages (raw files)", len(inv.pipeline_stages), 13),
        ("§7", "Computed structs", len(inv.computed_structs), 7),
        ("§8", "Compare functions", len(inv.compare_functions), 4),
        ("§9", "Live track fields", len(inv.live_track_fields), 16),
        ("§10", "Tuning params (individual)", len(inv.tuning_params), 4),
        ("§11", "PDF consumers", len(inv.pdf_consumers), 6),
        ("§12", "Mac consumers", len(inv.mac_http_consumers), 6),
        ("§13", "Classification items", len(inv.classification), 4),
        ("§14", "Proto messages", len(inv.proto_field_groups), 12),
        ("§15", "ECharts endpoints", len(inv.echart_endpoints), 5),
        ("§16", "cmd/ entry points", len(inv.cmd_entries), 11),
        ("§17", "Speed percentile debt", 0, 0),
        ("§18", "Debug/admin routes", len(inv.debug_routes), 19),
    ]

    print("BACKEND_SURFACE_MATRIX.md — Coverage Analysis")
    print("=" * 72)
    print()
    print(f"{'Section':<6} {'Description':<30} {'Found':>6} {'Matrix':>7}  Status")
    print(f"{'-' * 6:<6} {'-' * 30:<30} {'-' * 6:>6} {'-' * 7:>7}  {'-' * 20}")

    fully_covered = 0
    partially_covered = 0
    not_covered = 0

    for ref, desc, found, matrix_rows in sections:
        if matrix_rows == 0:
            status = "narrative (no table)"
            partially_covered += 1
        elif found >= matrix_rows:
            status = "✅ fully extractable"
            fully_covered += 1
        elif found > 0:
            pct = found * 100 // matrix_rows
            status = f"🔶 {pct}% extractable"
            partially_covered += 1
        else:
            status = "❌ not extractable"
            not_covered += 1

        print(f"{ref:<6} {desc:<30} {found:>6} {matrix_rows:>7}  {status}")

    print()
    print("=" * 72)
    print(f"  Fully extractable:   {fully_covered}/18 sections")
    print(f"  Partially:           {partially_covered}/18 sections")
    print(f"  Not extractable:     {not_covered}/18 sections")
    print()
    print("IMPORTANT: Counts above measure raw extraction only.")
    print("The matrix also needs:")
    print("  • Surface marks (✅/📋/🔶/🗑️/—) — requires code-path tracing")
    print("  • Layer/Group column — human curation")
    print("  • Notes column — human curation")
    print("  • Summary + Gap summary tables — derivable from section data")
    print()

    print("GENERATION HINTS PER SECTION")
    print("=" * 72)
    for ref, hint in GENERATION_HINTS.items():
        auto = hint.get("auto_extractable", False)
        if auto is True:
            badge = "AUTO"
        elif auto == "partial":
            badge = "PARTIAL"
        else:
            badge = "MANUAL"
        print(f"\n  [{badge:>7}] {ref}: {hint['title']}")
        notes = hint.get("notes", "")
        if notes:
            for line in notes.split("\n"):
                print(f"           {line.strip()}")


# ---------------------------------------------------------------------------
# Tracing checklist — LLM-guided surface-mark audit
# ---------------------------------------------------------------------------

# Default checklist path, relative to repo root
_CHECKLIST_FILENAME = "data/structures/.matrix-trace-checklist.json"

# Surfaces to trace per item
_SURFACES = ["DB", "Web", "PDF", "Mac"]

# Valid mark values an LLM can assign
_VALID_MARKS = {"✅", "📋", "🔶", "🗑️", "—", "?"}


@dataclass
class TraceItem:
    """One row in the matrix that needs surface marks traced."""

    id: str  # e.g. "§1.03"
    section: str  # e.g. "§1"
    section_title: str
    label: str  # human-readable: "GET /events"
    source_file: str  # e.g. "internal/api/server.go"
    handler: str  # e.g. "s.listEvents"
    surfaces: dict[str, str]  # {"DB": "?", "Web": "?", "PDF": "?", "Mac": "?"}
    notes: str  # LLM fills this in during tracing
    status: str  # "pending" | "done" | "skip"

    # Tracing guidance for the LLM
    trace_instructions: str


def _build_trace_items(inv: MatrixInventory, root: Path) -> list[TraceItem]:
    """Build the full list of items that need surface-mark tracing."""
    items: list[TraceItem] = []
    seq = 0

    def _add(
        section: str,
        title: str,
        label: str,
        source: str,
        handler: str,
        instructions: str,
    ) -> None:
        nonlocal seq
        seq += 1
        items.append(
            TraceItem(
                id=f"{section}.{seq:03d}",
                section=section,
                section_title=title,
                label=label,
                source_file=source,
                handler=handler,
                surfaces={s: "?" for s in _SURFACES},
                notes="",
                status="pending",
                trace_instructions=instructions,
            )
        )

    # §1 Radar HTTP
    sec, stitle = "§1", "HTTP API Endpoints — Radar / Main Server"
    for ep in inv.radar_http:
        _add(
            sec,
            stitle,
            f"{ep.method} {ep.path}",
            ep.file,
            ep.handler,
            (
                f"1. Read handler `{ep.handler}` in `{ep.file}`.\n"
                f"2. DB: does it call any SQL query/insert/update on the DB? "
                f"Check for db.Query/db.Exec/store.* calls.\n"
                f"3. Web: search web/src for fetch calls to `{ep.path}`.\n"
                f"4. PDF: search tools/pdf-generator for `{ep.path}`.\n"
                f"5. Mac: search tools/visualiser-macos for `{ep.path}`."
            ),
        )

    # §2 LiDAR HTTP
    seq = 0
    sec, stitle = "§2", "HTTP API Endpoints — LiDAR Monitor"
    for ep in inv.lidar_http:
        _add(
            sec,
            stitle,
            f"{ep.method} {ep.path}",
            ep.file,
            ep.handler,
            (
                f"1. Read handler `{ep.handler}` in `{ep.file}`.\n"
                f"2. DB: does it read/write SQLite? Check for store.* or "
                f"db.* calls in the handler chain.\n"
                f"3. Web: search web/src for fetch to `{ep.path}` or the "
                f"path prefix.\n"
                f"4. PDF: search tools/pdf-generator for this path.\n"
                f"5. Mac: search tools/visualiser-macos for this path or "
                f"appendingPathComponent containing it."
            ),
        )

    # §3 gRPC
    seq = 0
    sec, stitle = "§3", "gRPC Service — macOS Visualiser"
    for m in inv.grpc_methods:
        _add(
            sec,
            stitle,
            f"rpc {m.name}({m.request}) → {m.response}",
            m.file,
            m.name,
            (
                f"1. Find the Go implementation of `{m.name}` in the gRPC "
                f"server (internal/lidar/grpc/).\n"
                f"2. DB: does the implementation read from SQLite?\n"
                f"3. Web: gRPC methods are not called from Web (mark —).\n"
                f"4. PDF: gRPC methods are not called from PDF (mark —).\n"
                f"5. Mac: search Swift code for `{m.name}` call."
            ),
        )

    # §4 DB tables
    seq = 0
    sec, stitle = "§4", "Database Tables"
    for t in inv.db_tables:
        _add(
            sec,
            stitle,
            f"table: {t.name}",
            t.file,
            "",
            (
                f"1. DB: mark ✅ (it's a table, always in DB).\n"
                f"2. Web: grep Go handlers for SELECT/INSERT on "
                f"`{t.name}`. If any handler returns data from this "
                f"table via HTTP, mark ✅.\n"
                f"3. PDF: check if Python api_client.py fetches data "
                f"that ultimately comes from `{t.name}`.\n"
                f"4. Mac: check if gRPC or Swift HTTP calls fetch "
                f"data from `{t.name}`."
            ),
        )

    # §5 DB columns
    seq = 0
    sec, stitle = "§5", "Database Fields — All Columns"
    for t in inv.db_tables:
        for col in t.columns:
            _add(
                sec,
                stitle,
                f"{t.name}.{col}",
                t.file,
                "",
                (
                    f"1. DB: mark ✅ (column exists in schema).\n"
                    f"2. Web: is `{col}` included in any JSON response "
                    f"from an HTTP handler that queries `{t.name}`? "
                    f"Check the Go struct → JSON serialisation.\n"
                    f"3. PDF: is `{col}` used in the PDF report data "
                    f"pipeline?\n"
                    f"4. Mac: is `{col}` sent via gRPC or HTTP to Mac?\n"
                    f"5. Check if this column is deprecated (🗑️) or "
                    f"partially wired (🔶) — look for TODOs or zero "
                    f"writes."
                ),
            )

    # §7 Computed structs
    seq = 0
    sec, stitle = "§7", "Go Structs — Computed but Not Persisted"
    for s in inv.computed_structs:
        _add(
            sec,
            stitle,
            f"{s.name} ({len(s.fields)} fields)",
            s.file,
            "",
            (
                f"1. Read struct `{s.name}` in `{s.file}`.\n"
                f"2. DB: is any field of this struct written to SQLite "
                f"anywhere? Search for INSERT/UPDATE containing these "
                f"field names.\n"
                f"3. Web: is this struct returned in any HTTP response?\n"
                f"4. PDF/Mac: is it consumed downstream?\n"
                f"5. If no surface consumes it, mark all as — and note "
                f"why in notes (e.g. 'in-memory only')."
            ),
        )

    # §8 Compare functions
    seq = 0
    sec, stitle = "§8", "Comparison Logic — No Triggering Endpoint"
    for f in inv.compare_functions:
        _add(
            sec,
            stitle,
            f"{f.name}()",
            f.file,
            f.name,
            (
                f"1. Read `{f.name}` in `{f.file}`.\n"
                f"2. DB: does it read/write DB?\n"
                f"3. Web: is there an HTTP endpoint that calls it?\n"
                f"4. If no endpoint triggers it, note 'no triggering "
                f"endpoint' and mark Web as 📋."
            ),
        )

    # §9 Live track fields
    seq = 0
    sec, stitle = "§9", "Live Track Fields — Fully Wired (Reference)"
    for field_name in inv.live_track_fields:
        _add(
            sec,
            stitle,
            f"TrackedObject.{field_name}",
            "internal/lidar/l5tracks/tracking.go",
            "",
            (
                f"1. Check if `{field_name}` is stored in lidar_tracks "
                f"or lidar_track_obs DB table.\n"
                f"2. Check if it appears in JSON responses from track "
                f"API endpoints.\n"
                f"3. Check if it's sent via gRPC FrameBundle to Mac.\n"
                f"4. Mark ✅ for each surface that fully wires this field."
            ),
        )

    # §10 Tuning params
    seq = 0
    sec, stitle = "§10", "Tuning Parameters"
    for p in inv.tuning_params:
        _add(
            sec,
            stitle,
            f"{p.go_field} (json:{p.json_key})",
            "internal/config/tuning.go",
            "",
            (
                f"1. DB: is `{p.json_key}` stored in params_json in "
                f"lidar_analysis_runs or lidar_sweeps?\n"
                f"2. Web: is it returned by GET /api/lidar/params?\n"
                f"3. PDF/Mac: not applicable (mark —)."
            ),
        )

    # §13 Classification
    seq = 0
    sec, stitle = "§13", "Classification Pipeline"
    for s in inv.classification:
        _add(
            sec,
            stitle,
            f"{s.name} ({len(s.fields)} items)",
            s.file,
            "",
            (
                f"1. Read `{s.name}` usage in the codebase.\n"
                f"2. DB: is it persisted in lidar_tracks.object_class / "
                f"object_confidence?\n"
                f"3. Web: does the track API return classification data?\n"
                f"4. Mac: does gRPC FrameBundle include classification?"
            ),
        )

    # §14 Proto field groups
    seq = 0
    sec, stitle = "§14", "FrameBundle — macOS-Only Proto Fields"
    for g in inv.proto_field_groups:
        _add(
            sec,
            stitle,
            f"message {g.message} ({len(g.fields)} fields)",
            g.file,
            "",
            (
                f"1. These are proto fields in `{g.message}`.\n"
                f"2. Mac: mark ✅ — all proto fields are consumed by "
                f"the macOS visualiser via StreamFrames.\n"
                f"3. DB/Web/PDF: check if any field is also persisted "
                f"or sent via HTTP. Most FrameBundle sub-messages are "
                f"Mac-only (mark —)."
            ),
        )

    # §15 ECharts
    seq = 0
    sec, stitle = "§15", "ECharts Dashboard Endpoints"
    for ep in inv.echart_endpoints:
        _add(
            sec,
            stitle,
            f"{ep.method} {ep.path}",
            ep.file,
            ep.handler,
            (
                f"1. Read handler `{ep.handler}`.\n"
                f"2. DB: does it query SQLite for chart data?\n"
                f"3. Web: these serve embedded ECharts HTML dashboards "
                f"at /debug/lidar/*, not the Svelte SPA. Mark ✅.\n"
                f"4. PDF/Mac: mark —."
            ),
        )

    # §16 cmd entries
    seq = 0
    sec, stitle = "§16", "cmd/ Entry Points"
    for e in inv.cmd_entries:
        _add(
            sec,
            stitle,
            f"binary: {e.binary}",
            e.location,
            "main()",
            (
                f"1. Read main() in `{e.location}`.\n"
                f"2. Does this binary write to the production SQLite DB?\n"
                f"3. Describe what this binary does in the notes field."
            ),
        )

    # §18 Debug routes
    seq = 0
    sec, stitle = "§18", "Debug / Admin Routes"
    for r in inv.debug_routes:
        _add(
            sec,
            stitle,
            r.path,
            r.file,
            r.handler,
            (
                "1. Debug routes serve diagnostic pages.\n"
                "2. These are not consumed by Web/PDF/Mac frontends.\n"
                "3. Mark DB only if it queries the database (e.g. "
                "db-stats, backup, tailsql)."
            ),
        )

    return items


def _checklist_path(root: Path) -> Path:
    return root / _CHECKLIST_FILENAME


def _save_checklist(path: Path, items: list[TraceItem]) -> None:
    data = {
        "version": 1,
        "description": (
            "Surface-mark tracing checklist for BACKEND_SURFACE_MATRIX.md. "
            "Each item needs DB/Web/PDF/Mac surface marks determined by "
            "reading the source code. Use --trace-continue to resume."
        ),
        "instructions_for_llm": (
            "Process items in order. For each item with status='pending':\n"
            "1. Read the source file and handler listed.\n"
            "2. Follow the trace_instructions to determine each surface mark.\n"
            "3. Set each surface to one of: ✅ 📋 🔶 🗑️ — ?\n"
            "4. Add any relevant notes.\n"
            "5. Set status to 'done'.\n"
            "6. Save the checklist file after each batch.\n"
            "7. If you hit your context limit, stop and report progress. "
            "The next session will pick up from where you left off using "
            "--trace-continue.\n\n"
            "IMPORTANT: Work section by section. Complete all items in one "
            "section before moving to the next. This keeps output coherent "
            "and allows partial matrix regeneration."
        ),
        "total_items": len(items),
        "sections": _summarise_sections(items),
        "items": [asdict(item) for item in items],
    }
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2, ensure_ascii=False), encoding="utf-8")


def _load_checklist(path: Path) -> list[TraceItem]:
    text = path.read_text(encoding="utf-8")
    data = json.loads(text)
    items: list[TraceItem] = []
    for d in data["items"]:
        items.append(
            TraceItem(
                id=d["id"],
                section=d["section"],
                section_title=d["section_title"],
                label=d["label"],
                source_file=d["source_file"],
                handler=d["handler"],
                surfaces=d["surfaces"],
                notes=d["notes"],
                status=d["status"],
                trace_instructions=d["trace_instructions"],
            )
        )
    return items


def _summarise_sections(items: list[TraceItem]) -> list[dict[str, object]]:
    """Per-section summary for quick LLM orientation."""
    sections: dict[str, dict[str, int]] = {}
    for item in items:
        if item.section not in sections:
            sections[item.section] = {"total": 0, "pending": 0, "done": 0, "skip": 0}
        sections[item.section]["total"] += 1
        sections[item.section][item.status] += 1

    return [
        {
            "section": sec,
            "title": next((i.section_title for i in items if i.section == sec), ""),
            **counts,
        }
        for sec, counts in sections.items()
    ]


def print_trace_status(items: list[TraceItem]) -> None:
    """Print progress summary for the checklist."""
    total = len(items)
    done = sum(1 for i in items if i.status == "done")
    skip = sum(1 for i in items if i.status == "skip")
    pending = total - done - skip

    print("MATRIX TRACING CHECKLIST — Progress")
    print("=" * 72)
    print(f"  Total items:   {total}")
    print(f"  Done:          {done}  ({done * 100 // total}%)")
    print(f"  Skipped:       {skip}")
    print(f"  Pending:       {pending}")
    print()

    # Per-section breakdown
    print(f"  {'Section':<6} {'Title':<42} {'Done':>5} {'Pend':>5} {'Skip':>5}")
    print(f"  {'-' * 6} {'-' * 42} {'-' * 5} {'-' * 5} {'-' * 5}")
    for s in _summarise_sections(items):
        print(
            f"  {s['section']:<6} {str(s['title']):<42} "
            f"{s['done']:>5} {s['pending']:>5} {s['skip']:>5}"
        )

    if pending == 0:
        print()
        print("  ✅ ALL ITEMS TRACED — checklist complete.")
        print("  Run with --trace-emit to generate the matrix markdown.")
    else:
        # Find next pending section
        next_item = next((i for i in items if i.status == "pending"), None)
        if next_item:
            print()
            print(f"  Next: {next_item.id} — {next_item.label}")
            print(f"  Section: {next_item.section} {next_item.section_title}")


def print_trace_next_batch(items: list[TraceItem], batch_size: int = 30) -> None:
    """Print the next batch of pending items for an LLM to process."""
    pending = [i for i in items if i.status == "pending"]
    if not pending:
        print("All items have been traced. No pending items remain.")
        return

    batch = pending[:batch_size]
    current_section = batch[0].section

    print(f"TRACE BATCH — {len(batch)} items starting at {batch[0].id}")
    print("=" * 72)
    print()
    print("For each item below:")
    print("  1. Read the source_file and follow trace_instructions")
    print("  2. Set each surface mark in the surfaces dict")
    print("  3. Add notes if relevant")
    print("  4. Set status to 'done'")
    print(f"  5. Save changes to {_CHECKLIST_FILENAME}")
    print()

    for item in batch:
        if item.section != current_section:
            print(f"\n{'─' * 72}")
            print(f"  SECTION CHANGE: {item.section} {item.section_title}")
            print(f"{'─' * 72}")
            current_section = item.section

        print(f"\n  [{item.id}] {item.label}")
        print(f"    File:    {item.source_file}")
        if item.handler:
            print(f"    Handler: {item.handler}")
        print(f"    Surfaces: {item.surfaces}")
        print("    Instructions:")
        for line in item.trace_instructions.split("\n"):
            print(f"      {line}")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main() -> None:
    parser = argparse.ArgumentParser(
        description="List all fields needed to regenerate BACKEND_SURFACE_MATRIX.md",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Output as JSON instead of human-readable text",
    )
    parser.add_argument(
        "--coverage",
        action="store_true",
        help="Show coverage analysis: what the script can vs. cannot extract",
    )
    parser.add_argument(
        "--trace",
        action="store_true",
        help="Generate a fresh tracing checklist for LLM-guided audit",
    )
    parser.add_argument(
        "--trace-continue",
        action="store_true",
        help="Show next pending batch from existing checklist",
    )
    parser.add_argument(
        "--trace-status",
        action="store_true",
        help="Show progress summary of the tracing checklist",
    )
    parser.add_argument(
        "--batch-size",
        type=int,
        default=30,
        help="Number of items per --trace-continue batch (default: 30)",
    )
    parser.add_argument(
        "--root",
        type=Path,
        default=None,
        help="Repository root (default: auto-detect from script location)",
    )
    args = parser.parse_args()

    if args.root:
        root = args.root.resolve()
    else:
        # Script lives in scripts/ — root is one level up
        root = Path(__file__).resolve().parent.parent

    if not (root / "go.mod").exists():
        print(
            f"Error: {root} does not look like the repo root (no go.mod)",
            file=sys.stderr,
        )
        sys.exit(1)

    inv = MatrixInventory(
        radar_http=extract_radar_http(root),
        lidar_http=extract_lidar_http(root),
        grpc_methods=extract_grpc(root),
        db_tables=extract_db_tables(root),
        pipeline_stages=extract_pipeline_stages(root),
        tuning_params=extract_tuning_params(root),
        cmd_entries=extract_cmd_entries(root),
        pdf_consumers=extract_pdf_consumers(root),
        mac_http_consumers=extract_mac_http_consumers(root),
        debug_routes=extract_debug_routes(root),
        computed_structs=extract_computed_structs(root),
        compare_functions=extract_compare_functions(root),
        live_track_fields=extract_live_track_fields(root),
        classification=extract_classification(root),
        proto_field_groups=extract_proto_fields(root),
    )
    inv.echart_endpoints = extract_echart_endpoints(inv.lidar_http)

    if args.json:
        print_json_report(inv)
    elif args.coverage:
        print_coverage_report(inv)
    elif args.trace:
        checklist = _checklist_path(root)
        if checklist.exists():
            print(
                f"Checklist already exists at {checklist}",
                file=sys.stderr,
            )
            print(
                "Use --trace-continue to resume, or delete the file to start fresh.",
                file=sys.stderr,
            )
            sys.exit(1)
        trace_items = _build_trace_items(inv, root)
        _save_checklist(checklist, trace_items)
        print(f"Created tracing checklist: {checklist}")
        print(f"  Total items: {len(trace_items)}")
        print()
        print_trace_status(trace_items)
        print()
        print("To begin tracing, run:")
        print("  python scripts/list-matrix-fields.py --trace-continue")
        print()
        print("Or have an LLM read and edit the checklist directly:")
        print(f"  {checklist}")
    elif args.trace_continue:
        checklist = _checklist_path(root)
        if not checklist.exists():
            print(
                f"No checklist found at {checklist}",
                file=sys.stderr,
            )
            print("Run with --trace first to generate one.", file=sys.stderr)
            sys.exit(1)
        trace_items = _load_checklist(checklist)
        print_trace_status(trace_items)
        print()
        print_trace_next_batch(trace_items, batch_size=args.batch_size)
    elif args.trace_status:
        checklist = _checklist_path(root)
        if not checklist.exists():
            print(
                f"No checklist found at {checklist}",
                file=sys.stderr,
            )
            sys.exit(1)
        trace_items = _load_checklist(checklist)
        print_trace_status(trace_items)
    else:
        print_text_report(inv)


if __name__ == "__main__":
    main()
