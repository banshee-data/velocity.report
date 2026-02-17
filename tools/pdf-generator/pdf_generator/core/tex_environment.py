#!/usr/bin/env python3
"""TeX environment configuration for development and production modes."""

from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Dict, Optional


@dataclass(frozen=True)
class TexEnvironment:
    """Resolved TeX environment paths and settings."""

    mode: str
    tex_root: Optional[str]
    compiler: str
    fmt_name: Optional[str]
    env_vars: Dict[str, str]


def resolve_tex_environment() -> TexEnvironment:
    """Resolve TeX compiler and environment settings for this process."""

    tex_root_raw = os.environ.get("VELOCITY_TEX_ROOT", "").strip()
    if not tex_root_raw:
        return TexEnvironment(
            mode="development",
            tex_root=None,
            compiler="xelatex",
            fmt_name=None,
            env_vars={},
        )

    tex_root = os.path.abspath(os.path.expanduser(tex_root_raw))
    bin_dir = os.path.join(tex_root, "bin")
    compiler = os.path.join(bin_dir, "xelatex")
    texmf_dist = os.path.join(tex_root, "texmf-dist")
    texmf_home = os.path.join(tex_root, "texmf")
    texmf_var = os.path.join(tex_root, "texmf-var")

    current_path = os.environ.get("PATH", "")
    env_vars = {
        "TEXMFHOME": texmf_home,
        "TEXMFDIST": texmf_dist,
        "TEXMFVAR": texmf_var,
        "TEXINPUTS": f"{os.path.join(texmf_dist, 'tex')}//{os.pathsep}",
        "TFMFONTS": f"{os.path.join(texmf_dist, 'fonts', 'tfm')}//{os.pathsep}",
        "PATH": (f"{bin_dir}{os.pathsep}{current_path}" if current_path else bin_dir),
    }

    fmt_dir = os.path.join(texmf_dist, "web2c", "xelatex")
    base_fmt_path = os.path.join(fmt_dir, "xelatex.fmt")
    if os.path.isfile(base_fmt_path):
        existing_formats = os.environ.get("TEXFORMATS", "")
        if existing_formats:
            env_vars["TEXFORMATS"] = f"{fmt_dir}{os.pathsep}{existing_formats}"
        else:
            env_vars["TEXFORMATS"] = f"{fmt_dir}{os.pathsep}"

    fmt_path = os.path.join(fmt_dir, "velocity-report.fmt")
    use_custom_fmt = os.environ.get("VELOCITY_USE_VELOCITY_FMT", "").strip().lower()
    enable_custom_fmt = use_custom_fmt in {"1", "true", "yes", "on"}

    fmt_name = (
        "velocity-report" if enable_custom_fmt and os.path.isfile(fmt_path) else None
    )
    if fmt_name and "TEXFORMATS" not in env_vars:
        existing_formats = os.environ.get("TEXFORMATS", "")
        if existing_formats:
            env_vars["TEXFORMATS"] = f"{fmt_dir}{os.pathsep}{existing_formats}"
        else:
            env_vars["TEXFORMATS"] = f"{fmt_dir}{os.pathsep}"

    return TexEnvironment(
        mode="production",
        tex_root=tex_root,
        compiler=compiler,
        fmt_name=fmt_name,
        env_vars=env_vars,
    )
