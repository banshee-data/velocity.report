# Error Surface Voice

Consistent, humane voice across all user-facing error messages — Go server,
web frontend, Python tools, and shell scripts.

## Principles

1. **Sentence case** — error messages start with a capital, do not end with a full stop.
2. **Name the problem, not the user** — "Cannot open database" rather than "You provided an invalid path".
3. **Include a next step** — where possible, tell the reader what to try.
4. **Diagnostic hints** use ` — try X` suffix or `\nTry: X` on a new line.

## Scope

The voice audit covers every user-visible string: HTTP error responses,
CLI `log.Fatalf` messages, web UI toast/error components, PDF generator
warnings, and shell script output.
