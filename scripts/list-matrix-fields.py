#!/usr/bin/env python3
"""List backend surface inventory for velocity.report.

Scans Go, Proto, Python, and Swift source files and prints a structured
inventory of HTTP endpoints, gRPC methods, DB tables/columns, pipeline
stages, tuning parameters, cmd/ entry points, and debug routes.

Two modes:
    python scripts/list-matrix-fields.py              # human-readable surface list
    python scripts/list-matrix-fields.py --checklist   # markdown checklist for LLM tracing
"""

from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass, field
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
    r"requests\.(get|post|put|delete)\(\s*" r'(?:f"[^"]*"|"[^"]*"|[a-zA-Z0-9_.]+)',
    re.IGNORECASE,
)

_PY_URL_RE = re.compile(
    r"(?:self\.(?:base_url|api_url)|base_url)\s*" r'(?:\+\s*f?"|=\s*f?")' r'([^"]+)"',
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


# ---------------------------------------------------------------------------
# Markdown checklist — LLM-consumable surface-tracing task list
# ---------------------------------------------------------------------------

# Surfaces to trace per item
_SURFACES = ["DB", "Web", "PDF", "Mac"]

# Section groupings for LLM context-window partitioning.
# Each group is a self-contained tracing task an LLM can handle in one request.
_SECTION_GROUPS: list[dict[str, object]] = [
    {
        "id": "http",
        "title": "HTTP API Surfaces",
        "sections": ["§1", "§2"],
        "focus": (
            "Trace every HTTP endpoint to determine which surfaces consume it. "
            "Check Go handler → DB calls, web/src fetch() calls, "
            "tools/pdf-generator API client, tools/visualiser-macos HTTP calls."
        ),
    },
    {
        "id": "grpc",
        "title": "gRPC + Proto Surfaces",
        "sections": ["§3", "§11"],
        "focus": (
            "Trace gRPC methods and FrameBundle proto fields. "
            "Almost all are Mac-only via StreamFrames. "
            "Check for any that also touch DB or Web."
        ),
    },
    {
        "id": "db",
        "title": "Database Schema Surfaces",
        "sections": ["§4", "§5"],
        "focus": (
            "Trace every table and column. DB is always ✅. "
            "Check which columns appear in HTTP JSON responses (Web), "
            "PDF generator queries (PDF), or gRPC/Swift calls (Mac). "
            "Flag deprecated columns as 🗑️."
        ),
    },
    {
        "id": "pipeline",
        "title": "Pipeline + Structs",
        "sections": ["§6", "§7", "§8", "§10"],
        "focus": (
            "Trace computed structs, comparison logic, live track fields, "
            "and classification pipeline. Many are in-memory only. "
            "Check if any field is persisted, returned via API, or sent via gRPC."
        ),
    },
    {
        "id": "config",
        "title": "Tuning + Entry Points + Debug",
        "sections": ["§9", "§12", "§13", "§14"],
        "focus": (
            "Trace tuning parameters (DB + Web only), ECharts endpoints "
            "(DB + embedded HTML), cmd/ binaries (which write to DB), "
            "and debug/admin routes (diagnostic only)."
        ),
    },
]


@dataclass
class ChecklistItem:
    """One item that needs surface marks traced."""

    id: str  # e.g. "§1.003"
    section: str  # e.g. "§1"
    section_title: str
    label: str  # human-readable: "GET /events"
    source_file: str  # e.g. "internal/api/server.go"
    handler: str  # e.g. "s.listEvents"
    trace_hint: str  # brief guidance for the LLM


def _build_checklist(inv: MatrixInventory, root: Path) -> list[ChecklistItem]:
    """Build the full list of items that need surface-mark tracing."""
    items: list[ChecklistItem] = []
    seq = 0

    def _add(
        section: str,
        title: str,
        label: str,
        source: str,
        handler: str,
        hint: str,
    ) -> None:
        nonlocal seq
        seq += 1
        items.append(
            ChecklistItem(
                id=f"{section}.{seq:03d}",
                section=section,
                section_title=title,
                label=label,
                source_file=source,
                handler=handler,
                trace_hint=hint,
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
            f"handler `{ep.handler}` → check DB calls, web/src fetch, PDF api_client, Mac HTTP",
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
            f"handler `{ep.handler}` → check DB calls, web/src fetch, Mac appendingPathComponent",
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
            f"Go impl in internal/lidar/grpc/ → DB reads? Mac: search Swift for `{m.name}`",
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
            f"DB=✅ always. Web: handlers SELECT from `{t.name}`? PDF: api_client? Mac: gRPC?",
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
                f"DB=✅. Check JSON serialisation of `{col}`, PDF usage, gRPC/Mac exposure. Flag 🗑️ if deprecated.",
            )

    # §6 Computed structs
    seq = 0
    sec, stitle = "§6", "Go Structs — Computed but Not Persisted"
    for s in inv.computed_structs:
        _add(
            sec,
            stitle,
            f"{s.name} ({len(s.fields)} fields)",
            s.file,
            "",
            "in-memory struct — check if any field is persisted, returned via HTTP, or sent via gRPC",
        )

    # §7 Compare functions
    seq = 0
    sec, stitle = "§7", "Comparison Logic — No Triggering Endpoint"
    for f in inv.compare_functions:
        _add(
            sec,
            stitle,
            f"{f.name}()",
            f.file,
            f.name,
            "check DB read/write, check if any HTTP endpoint calls it",
        )

    # §8 Live track fields
    seq = 0
    sec, stitle = "§8", "Live Track Fields — Fully Wired (Reference)"
    for field_name in inv.live_track_fields:
        _add(
            sec,
            stitle,
            f"TrackedObject.{field_name}",
            "internal/lidar/l5tracks/tracking.go",
            "",
            f"check lidar_tracks/lidar_track_obs for `{field_name}`, JSON responses, gRPC FrameBundle",
        )

    # §9 Tuning params
    seq = 0
    sec, stitle = "§9", "Tuning Parameters"
    for p in inv.tuning_params:
        _add(
            sec,
            stitle,
            f"{p.go_field} (json:{p.json_key})",
            "internal/config/tuning.go",
            "",
            "DB: params_json in lidar_analysis_runs? Web: GET /api/lidar/params? PDF/Mac: —",
        )

    # §10 Classification
    seq = 0
    sec, stitle = "§10", "Classification Pipeline"
    for s in inv.classification:
        _add(
            sec,
            stitle,
            f"{s.name} ({len(s.fields)} items)",
            s.file,
            "",
            "check lidar_tracks.object_class, track API JSON, gRPC FrameBundle classification",
        )

    # §11 Proto field groups
    seq = 0
    sec, stitle = "§11", "FrameBundle — macOS-Only Proto Fields"
    for g in inv.proto_field_groups:
        _add(
            sec,
            stitle,
            f"message {g.message} ({len(g.fields)} fields)",
            g.file,
            "",
            "Mac=✅ via StreamFrames. Check if also persisted to DB or sent via HTTP.",
        )

    # §12 ECharts
    seq = 0
    sec, stitle = "§12", "ECharts Dashboard Endpoints"
    for ep in inv.echart_endpoints:
        _add(
            sec,
            stitle,
            f"{ep.method} {ep.path}",
            ep.file,
            ep.handler,
            "DB: SQLite chart query? Web=✅ embedded ECharts at /debug/lidar/*. PDF/Mac: —",
        )

    # §13 cmd entries
    seq = 0
    sec, stitle = "§13", "cmd/ Entry Points"
    for e in inv.cmd_entries:
        _add(
            sec,
            stitle,
            f"binary: {e.binary}",
            e.location,
            "main()",
            "does this binary write to production SQLite?",
        )

    # §14 Debug routes
    seq = 0
    sec, stitle = "§14", "Debug / Admin Routes"
    for r in inv.debug_routes:
        _add(
            sec,
            stitle,
            r.path,
            r.file,
            r.handler,
            "diagnostic only — mark DB only if it queries SQLite (db-stats, tailsql, backup)",
        )

    return items


def print_markdown_checklist(items: list[ChecklistItem]) -> None:
    """Print a markdown checklist partitioned by surface-area groups."""
    print("# Surface Tracing Checklist")
    print()
    print("Generated by `scripts/list-matrix-fields.py --checklist`.")
    print(f"**{len(items)} items** across **{len(_SECTION_GROUPS)} task groups**.")
    print()
    print("## How to use this checklist")
    print()
    print("Each **Task Group** below is sized for one LLM context window.")
    print("For each item, trace its exposure across four surfaces:")
    print()
    print("| Mark | Meaning |")
    print("|------|---------|")
    print("| ✅ | Fully wired to this surface |")
    print("| 📋 | Planned — not yet implemented |")
    print("| 🔶 | Partially wired (explain in notes) |")
    print("| 🗑️ | Deprecated — to be removed |")
    print("| — | Not applicable to this surface |")
    print()
    print(
        "**Surfaces:** DB (SQLite), Web (Svelte UI :8080), "
        "PDF (Python LaTeX generator), Mac (Metal visualiser via gRPC)"
    )
    print()

    for group in _SECTION_GROUPS:
        group_sections = group["sections"]
        group_items = [i for i in items if i.section in group_sections]
        if not group_items:
            continue

        print("---")
        print()
        print(f"## Task Group: {group['title']}")
        print()
        print(f"**Sections:** {', '.join(str(s) for s in group_sections)}  ")
        print(f"**Items:** {len(group_items)}  ")
        print(f"**Focus:** {group['focus']}")
        print()

        current_section = ""
        for item in group_items:
            if item.section != current_section:
                current_section = item.section
                print(f"### {item.section} {item.section_title}")
                print()
                print("| ID | Item | Source | DB | Web | PDF | Mac | Notes |")
                print("|---|---|---|---|---|---|---|---|")

            handler_str = f" → `{item.handler}`" if item.handler else ""
            print(
                f"| {item.id} | `{item.label}` | "
                f"`{item.source_file}`{handler_str} | | | | | "
                f"{item.trace_hint} |"
            )

        print()


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------


def main() -> None:
    parser = argparse.ArgumentParser(
        description="List backend surface inventory for velocity.report",
    )
    parser.add_argument(
        "--checklist",
        action="store_true",
        help="Output a markdown checklist for LLM-guided surface tracing",
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

    if args.checklist:
        checklist_items = _build_checklist(inv, root)
        print_markdown_checklist(checklist_items)
    else:
        print_text_report(inv)


if __name__ == "__main__":
    main()
