# Platform Hub

Stable documentation for shared codebase structure and cross-cutting development
concerns. Anything that is not specific to a single sensor or to the user interface
lives here.

| Subdirectory                   | Contents                                                                          |
| ------------------------------ | --------------------------------------------------------------------------------- |
| [architecture/](architecture/) | Go package design, DB schema shape, logging model, ID conventions, metrics naming |
| [operations/](operations/)     | Deployment, release lifecycle, documentation standards, tooling, reporting        |

## Scope

This hub owns topics where the lasting value is a structural property of the shared
codebase **or** a development methodology that applies regardless of which sensor or
UI surface is being changed:

### Codebase structure

- Go package boundaries and import rules
- Database schema design (two-package SQL boundary)
- Track Description Language (TDL)
- Structured logging model
- Typed UUID prefix conventions
- Platform library design (TicTacTail)

### Deployment and release

- Single-binary distribution packaging
- Raspberry Pi imager
- Schema migrations
- Release lifecycle and backward-compatibility management

### Development methodology

- Documentation and plan hygiene standards
- Metrics naming and observability contracts
- Code coverage methodology
- Data science reproducibility principles
- Python tooling and venv management
- Code-formatting standards
- PDF reporting infrastructure
- AI agent architecture
