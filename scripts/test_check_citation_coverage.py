"""Exercise stable identifier coverage without relying on external services."""

import importlib.util
from pathlib import Path

SPEC = importlib.util.spec_from_file_location(
    "citation_coverage", Path(__file__).with_name("check-citation-coverage.py")
)
MOD = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MOD)


def test_published_entry_covers_preprint_forms_and_encoded_doi():
    bib = """@article{Example2020,
  doi = {10.1234/Example(1)},
  eprint = {2001.12345}
}
"""
    doc = """[Paper](https://doi.org/10.1234%2FEXAMPLE(1)).
https://arxiv.org/html/2001.12345v2
https://arxiv.org/pdf/2001.12345.pdf
arXiv:2001.12345
https://doi.org/10.48550/arXiv.2001.12345
- **References:** `Example2020`.
Inline `10.1234/Example(1)` keeps its Markdown delimiter out of the identifier.
"""
    assert MOD.validate(bib, {"plan.md": doc}) == []


def test_missing_identifiers_and_unknown_key_report_locations():
    problems = MOD.validate(
        "@article{Known2020,\n}\n",
        {
            "plan.md": "doi:10.1234/missing.\narXiv:2001.12345\n- **References:** `Unknown2020`."
        },
    )
    assert problems == [
        "plan.md:1: missing bibliography entry for doi:10.1234/missing",
        "plan.md:2: missing bibliography entry for arxiv:2001.12345",
        "plan.md:3: unknown bibliography key Unknown2020",
    ]


def test_duplicate_keys_are_rejected():
    assert MOD.validate("@misc{Same2020,\n}\n@article{Same2020,\n}\n", {}) == [
        "duplicate bibliography key: Same2020"
    ]


def test_preprint_and_publication_must_share_one_entry():
    bib = """@article{Published2020,
  eprint = {2001.12345}
}
@misc{Preprint2019,
  url = {https://arxiv.org/abs/2001.12345v1}
}
"""
    assert MOD.validate(bib, {}) == [
        "duplicate work arxiv:2001.12345: Published2020 and Preprint2019"
    ]
