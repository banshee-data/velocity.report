```
  _________________________ ________   ___________
 /   _____/\__    ___/\__  |   |    |  \_   _____/
 \_____  \   |    |    \   |   |    |   |    __/_
 /        \  |    |     \___   |    |___|        \
/ ________/  |____|    / ______|_______ \______  /
\/                     \/              \/      \/
```

Writing conventions for velocity.report. The single canonical reference for spelling, punctuation, and prose mechanics across all code, comments, documentation, and commit messages.

## British English

| Use            | Not            |
| -------------- | -------------- |
| analyse        | analyze        |
| behaviour      | behavior       |
| centre         | center         |
| colour         | color          |
| licence (noun) | license (noun) |
| metre          | meter          |
| neighbour      | neighbor       |
| organisation   | organization   |
| travelled      | traveled       |
| visualisation  | visualization  |

Applies to symbols, filenames, comments, documentation, and commit messages.

**Exception:** external dependencies, API names, or standards that require American spelling.

## Punctuation

### Colons and commas over dashes

Use a colon to introduce a consequence, explanation, or list. Use a comma for a natural pause. Reserve dashes for genuine parenthetical asides where commas would create ambiguity.

| Prefer                                         | Avoid                                           |
| ---------------------------------------------- | ----------------------------------------------- |
| The system records speed data: nothing else.   | The system records speed data — nothing else.   |
| Data stays local, by design.                   | Data stays local — by design.                   |
| Three principles: privacy, evidence, locality. | Three principles — privacy, evidence, locality. |

### Hyphens

Use hyphens only where structurally necessary:

- Compound adjectives before a noun: `privacy-preserving radar`, `local-first design`
- Established compounds: `open-source`, `real-time`
- Not after adverbs ending in -ly: `carefully designed system`
- Do not stack hyphens: restructure the sentence instead

### Em dashes

Avoid. When tempted, reach for:

1. A colon, if introducing an explanation or consequence
2. A comma, if the aside is short
3. Parentheses, if genuinely parenthetical
4. A full stop, if the thought deserves its own sentence

## General Mechanics

- **Oxford comma:** yes. `privacy, evidence, and locality`
- **Contractions:** allowed in documentation and comments. Not in legal or formal policy text.
- **Active voice:** prefer it. `The service records speeds` not `Speeds are recorded by the service`.
- **Sentence length:** short sentences do the work. Medium sentences explain. Long sentences earn their keep or get split.
- **Hedging:** do not. Say what is true. `The data do not support this` not `some might argue the data could potentially suggest otherwise`.

## Documentation Structure

Design documents and specifications describe _what_ a system should do and _why_. They are not repositories for implementation artefacts.

### Code blocks in documentation

If a block could be pasted into a source file and compiled, it is pre-built code and does not
belong in a design document. Replace it with prose, a field table, or a file reference.

**Banned code-block languages in docs:**

- Go (structs, interfaces, function bodies)
- SQL (DDL, migrations, queries)
- Protocol Buffers / gRPC definitions
- TypeScript, Svelte, Swift, Python
- JSON, JSONC, YAML, TOML, XML, CSV
- HTML, CSS, LaTeX
- Makefile targets (use prose or a target table)
- Dockerfile

**Allowed code-block languages:**

- `bash` / `sh`: operational commands, build steps, CLI examples
- `mermaid`: rendered diagrams
- `text`: ASCII diagrams, directory trees, plain-text tables
- Pseudocode (unfenced or fenced as `text`)

**Belongs in docs (any format):**

- Prose descriptions of behaviour and rationale
- ASCII diagrams (system boundaries, data flow, UI mockups)
- Tables: field schemas, API endpoints, error contracts, decision matrices
- Worked examples with brief inline values (not full payloads)
- Security considerations and threat mitigations
- Shell commands showing how to build, test, or deploy

**Rule of thumb:** if it could be copied into a source file and compiled, replace it with
a table or prose. Shell commands that show how to _use_ the system are fine.

### Length target

Specification and architecture documents should target 800 lines or fewer. Dense tables and design discussion justify going over; wandering prose and pre-built code do not.

## Dates and Timestamps

**Machine timestamps:** UTC ISO 8601 with trailing `Z`. Example: `2026-04-07T14:32:08Z`. Applies to build metadata, log output, generated files, persisted JSON, and git date attribution. Never use local time for machine-written timestamps.

**Human-readable dates:** `Month DD, YYYY` (e.g. `April 7, 2026`). Used in devlog headers and release notes. Derived from the UTC date of the event.

See `coding-standards.md` for the full rule.
