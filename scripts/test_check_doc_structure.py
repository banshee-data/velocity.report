"""Check that document contracts inspect prose rather than example code."""

import importlib.util
from pathlib import Path

SPEC = importlib.util.spec_from_file_location(
    "doc_structure", Path(__file__).with_name("check-doc-structure.py")
)
MOD = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MOD)


def test_summary_after_multiline_metadata_and_examples_pass():
    text = """# Plan

- **Status:** Parked; retain its existing
  revisit trigger.

This plan describes the intended work.

## Approach

```markdown
# Example title
#### Not a real heading
```

~~~text
# Another example
~~~
<!--
#### Not a real heading either
-->
### Topic
"""
    assert MOD.validate(text) == []


def test_duplicate_title_and_skipped_heading_are_rejected():
    text = "# Plan\n\nSummary.\n\n- **Status:** Proposed\n\n### Skipped\n\n# Example"
    assert MOD.validate(text) == [
        "expected exactly one document title",
        "line 7: heading level skips a parent",
    ]


def test_metadata_cannot_substitute_for_summary():
    assert MOD.validate("# Plan\n\n- **Status:** Proposed\n\n## Goal") == [
        "missing opening narrative before the first section"
    ]


def test_research_requires_evidence_and_open_questions():
    text = "# Note\n\nA research assessment.\n\n- **Status:** Research\n\n## Interpretation"
    assert MOD.validate(text, research=True) == [
        "research assessment needs an Evidence section",
        "research assessment needs an Open questions section",
    ]
