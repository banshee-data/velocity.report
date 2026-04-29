# Role: Editorial

Shared mixin for agents in editorial roles (Florence, Terry, Ruth). Read this file for project-specific context that supplements the agent's persona.

## Shared Knowledge References

These files contain canonical project facts. Read them when needed rather than relying on memorised details:

- [TENETS.md](../../TENETS.md) — project constitution (Layer 0)
- [architecture.md](architecture.md) — tech stack, components, deployment
- [coding-standards.md](coding-standards.md) — British English, commit format, documentation rules

## Documentation Landscape

| Document                  | Audience     | Purpose                                                  |
| ------------------------- | ------------ | -------------------------------------------------------- |
| `README.md`               | Contributors | Project overview, quick start                            |
| `ARCHITECTURE.md`         | Developers   | Full system design                                       |
| `CONTRIBUTING.md`         | Contributors | Workflow and standards                                   |
| `cmd/radar/README.md`     | Developers   | Go server specifics                                      |
| `web/README.md`           | Developers   | Frontend specifics                                       |
| `public_html/src/guides/` | End users    | Setup and usage guides                                   |
| `docs/`                   | Internal     | Plans, backlogs, research notes                          |
| `config/CONFIG.md`        | Developers   | Tuning parameter documentation and maths cross-reference |

## Writing Style

See [STYLE.md](../STYLE.md) for British English spelling, punctuation conventions, and prose mechanics.

## Voice And Tone

The project speaks to neighbourhood change-makers — people who care about street safety and want evidence to support their advocacy. Documentation should be:

- Clear and direct — avoid jargon where plain language works
- Respectful of the reader's time
- Grounded in evidence, not marketing language
- Accessible to non-technical community members (in user-facing docs)
- Precise and technical (in developer-facing docs)

## Product Context

**velocity.report** exists to give communities measured evidence about traffic speeds on their streets — without surveillance, without cameras, without compromising anyone's privacy. The data serves people who walk, cycle, and play in those streets.

This context matters for editorial decisions: every piece of documentation should reinforce that this is a tool for community empowerment, not for enforcement or surveillance.
