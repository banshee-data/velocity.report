# Role: Editorial

Shared mixin for agents in editorial roles (Florence, Terry, Ruth). Read this file for project-specific context that supplements the agent's persona.

## Shared Knowledge References

These files contain canonical project facts. Read them when needed rather than relying on memorised details:

- [TENETS.md](../TENETS.md) — project constitution (Layer 0)
- [architecture.md](architecture.md) — tech stack, components, deployment
- [coding-standards.md](coding-standards.md) — British English, commit format, documentation rules

## Documentation Landscape

| Document                        | Audience     | Purpose                         |
| ------------------------------- | ------------ | ------------------------------- |
| `README.md`                     | Contributors | Project overview, quick start   |
| `ARCHITECTURE.md`               | Developers   | Full system design              |
| `CONTRIBUTING.md`               | Contributors | Workflow and standards          |
| `cmd/radar/README.md`           | Developers   | Go server specifics             |
| `tools/pdf-generator/README.md` | Developers   | Python tooling                  |
| `web/README.md`                 | Developers   | Frontend specifics              |
| `public_html/src/guides/`       | End users    | Setup and usage guides          |
| `docs/`                         | Internal     | Plans, backlogs, research notes |
| `config/README.md`              | Developers   | Tuning parameter documentation  |
| `config/README.maths.md`        | Researchers  | Mathematical foundations        |

## British English Enforcement

British English is mandatory across all documentation, comments, and user-facing text:

| Use            | Not            |
| -------------- | -------------- |
| analyse        | analyze        |
| colour         | color          |
| centre         | center         |
| neighbour      | neighbor       |
| organisation   | organization   |
| licence (noun) | license (noun) |
| travelled      | traveled       |
| behaviour      | behavior       |
| visualisation  | visualization  |

**Exception:** External dependencies, API names, or standards that require American spelling.

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
