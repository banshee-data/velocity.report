#!/usr/bin/env python3
"""Unit tests for tex_environment.py."""

import os
import stat
import tempfile
import unittest
from unittest.mock import patch

from pdf_generator.core.tex_environment import resolve_tex_environment


class TestTexEnvironment(unittest.TestCase):
    """Tests for TeX environment resolution."""

    @patch.dict(os.environ, {"VELOCITY_TEX_ROOT": ""}, clear=False)
    def test_resolve_development_mode_when_env_unset(self):
        """Unset VELOCITY_TEX_ROOT should resolve to development mode."""
        env = resolve_tex_environment()

        self.assertEqual(env.mode, "development")
        self.assertIsNone(env.tex_root)
        self.assertEqual(env.compiler, "xelatex")
        self.assertIsNone(env.fmt_name)
        self.assertEqual(env.env_vars, {})

    def test_resolve_production_mode_without_fmt(self):
        """Setting VELOCITY_TEX_ROOT should resolve production paths."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            bin_dir = os.path.join(tmp_dir, "bin")
            texmf_dist = os.path.join(tmp_dir, "texmf-dist")
            texmf_var = os.path.join(tmp_dir, "texmf-var")
            texmf_home = os.path.join(tmp_dir, "texmf")
            os.makedirs(bin_dir, exist_ok=True)
            os.makedirs(texmf_dist, exist_ok=True)
            os.makedirs(texmf_var, exist_ok=True)
            os.makedirs(texmf_home, exist_ok=True)
            compiler_path = os.path.join(bin_dir, "xelatex")
            with open(compiler_path, "w", encoding="utf-8") as handle:
                handle.write("#!/bin/sh\nexit 0\n")
            os.chmod(compiler_path, stat.S_IRWXU)

            with patch.dict(
                os.environ,
                {
                    "VELOCITY_TEX_ROOT": tmp_dir,
                    "PATH": "/usr/bin:/bin",
                },
                clear=False,
            ):
                env = resolve_tex_environment()

            self.assertEqual(env.mode, "production")
            self.assertEqual(env.tex_root, os.path.abspath(tmp_dir))
            self.assertEqual(env.compiler, compiler_path)
            self.assertIsNone(env.fmt_name)
            self.assertEqual(env.env_vars["TEXMFHOME"], texmf_home)
            self.assertEqual(env.env_vars["TEXMFDIST"], texmf_dist)
            self.assertEqual(env.env_vars["TEXMFVAR"], texmf_var)
            self.assertTrue(env.env_vars["PATH"].startswith(f"{bin_dir}{os.pathsep}"))

    def test_resolve_production_mode_with_fmt_opt_in(self):
        """Custom format should be detected only when explicitly enabled."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            fmt_dir = os.path.join(tmp_dir, "texmf-dist", "web2c", "xelatex")
            os.makedirs(fmt_dir, exist_ok=True)
            os.makedirs(os.path.join(tmp_dir, "bin"), exist_ok=True)
            with open(
                os.path.join(tmp_dir, "bin", "xelatex"), "w", encoding="utf-8"
            ) as handle:
                handle.write("#!/bin/sh\nexit 0\n")
            os.chmod(os.path.join(tmp_dir, "bin", "xelatex"), stat.S_IRWXU)
            with open(
                os.path.join(fmt_dir, "velocity-report.fmt"), "w", encoding="utf-8"
            ) as handle:
                handle.write("fmt\n")

            with patch.dict(
                os.environ,
                {
                    "VELOCITY_TEX_ROOT": tmp_dir,
                    "VELOCITY_USE_VELOCITY_FMT": "1",
                    "PATH": "/usr/bin:/bin",
                    "TEXFORMATS": "/existing/formats",
                },
                clear=False,
            ):
                env = resolve_tex_environment()

            self.assertEqual(env.mode, "production")
            self.assertEqual(env.fmt_name, "velocity-report")
            self.assertIn("TEXFORMATS", env.env_vars)
            self.assertTrue(env.env_vars["TEXFORMATS"].startswith(fmt_dir))
            self.assertIn("/existing/formats", env.env_vars["TEXFORMATS"])

    def test_resolve_production_mode_with_fmt_opt_out_default(self):
        """Custom format should be ignored unless explicitly enabled."""
        with tempfile.TemporaryDirectory() as tmp_dir:
            fmt_dir = os.path.join(tmp_dir, "texmf-dist", "web2c", "xelatex")
            os.makedirs(fmt_dir, exist_ok=True)
            os.makedirs(os.path.join(tmp_dir, "bin"), exist_ok=True)
            with open(
                os.path.join(tmp_dir, "bin", "xelatex"), "w", encoding="utf-8"
            ) as handle:
                handle.write("#!/bin/sh\nexit 0\n")
            os.chmod(os.path.join(tmp_dir, "bin", "xelatex"), stat.S_IRWXU)
            with open(
                os.path.join(fmt_dir, "velocity-report.fmt"), "w", encoding="utf-8"
            ) as handle:
                handle.write("fmt\n")

            with patch.dict(
                os.environ,
                {
                    "VELOCITY_TEX_ROOT": tmp_dir,
                    "VELOCITY_USE_VELOCITY_FMT": "",
                    "PATH": "/usr/bin:/bin",
                },
                clear=False,
            ):
                env = resolve_tex_environment()

            self.assertEqual(env.mode, "production")
            self.assertIsNone(env.fmt_name)
            self.assertNotIn("TEXFORMATS", env.env_vars)


if __name__ == "__main__":
    unittest.main()
