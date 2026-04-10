# Asset Naming Conventions

Canonical naming patterns for release artefacts, development builds, and
platform-specific binaries.

## Release Assets

Release assets follow the pattern:

```
{product}-{version}-{os}-{arch}{ext}
```

Examples: `velocity-report-0.5.2-linux-arm64`, `VelocityVisualiser-0.5.2-macos-arm64.dmg`.

## Development Assets

Development (nightly) assets include a timestamp and short SHA:

```
{datetime}-{product}-{devversion}-{os}-{arch}-{sha7}{ext}
```

## Product Names

| Product          | Asset name           | Case rule  |
| ---------------- | -------------------- | ---------- |
| Server           | `velocity-report`    | kebab-case |
| macOS Visualiser | `VelocityVisualiser` | PascalCase |
| Management CLI   | `velocity-ctl`       | kebab-case |

Active plan: [asset-naming-plan.md](../../plans/asset-naming-plan.md)
