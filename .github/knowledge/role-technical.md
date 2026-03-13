# Role: Technical

Shared mixin for agents in technical roles (Appius, Grace, Euler, Malory). Read this file for project-specific technical context that supplements the agent's persona.

## Shared Knowledge References

These files contain canonical project facts. Read them when needed rather than relying on memorised details:

- [TENETS.md](../TENETS.md) — project constitution (Layer 0)
- [architecture.md](architecture.md) — tech stack, data flow, deployment
- [build-and-test.md](build-and-test.md) — make targets, testing, dev servers
- [hardware.md](hardware.md) — radar/LIDAR specs, serial/UDP interfaces
- [coding-standards.md](coding-standards.md) — British English, commit format, paths

## Test Confidence

| Area                   | Confidence | Notes                                     |
| ---------------------- | ---------- | ----------------------------------------- |
| Radar serial parsing   | High       | Well-tested, production path              |
| Go API + transit logic | High       | Comprehensive unit + integration tests    |
| Python PDF generator   | High       | 532+ tests, good coverage                 |
| Web frontend           | Medium     | Growing test suite                        |
| LIDAR processing       | Medium     | Core algorithms tested, pipeline evolving |
| macOS visualiser       | Low        | Minimal automated tests                   |

## Code Review Standards

When reviewing code changes, verify:

1. `make format && make lint && make test` all pass
2. Changes respect the privacy guarantees (no PII collection, local-only storage)
3. New database operations use parameterised queries
4. British English in all new symbols, comments, and documentation
5. Documentation updated if functionality changed

## Security Awareness

All technical agents should be alert to:

- Input validation on system boundaries (API, serial, UDP, config files)
- No PII in logs, responses, or exports
- Path traversal in file operations
- LaTeX injection in PDF generation

For deeper security review, consult [security-surface.md](security-surface.md) and [security-checklist.md](security-checklist.md), or escalate to Malory.
