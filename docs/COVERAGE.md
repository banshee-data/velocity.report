# Code coverage

This project uses [Codecov](https://codecov.io) to track code coverage across all
three main components: Go server, Python PDF generator, and Web frontend.

## Coverage badges

The README displays live coverage badges for each component:

- **Go Coverage**: Test coverage for `internal/` packages (~89% average)
- **Python Coverage**: Test coverage for the PDF generator in [tools/pdf-generator/](../tools/pdf-generator)
- **Web Coverage**: Test coverage for the Svelte web frontend in [web/src/](../web/src)

Each badge links to detailed coverage reports on Codecov.

## Running coverage locally

### All components

Generate coverage reports for all components at once:

```bash
make coverage
```

This will create HTML reports at:

- Go: [`coverage.html`](../coverage.html) <!-- link-ignore -->
- Python: [`tools/pdf-generator/htmlcov/index.html`](../tools/pdf-generator/htmlcov/index.html) <!-- link-ignore -->
- Web: [`web/coverage/lcov-report/index.html`](../web/coverage/lcov-report/index.html) <!-- link-ignore -->

### Individual components

**Go:**

```bash
make test-go-cov
open coverage.html  # macOS
xdg-open coverage.html  # Linux
```

**Python:**

```bash
make test-python-cov
open tools/pdf-generator/htmlcov/index.html  # macOS
xdg-open tools/pdf-generator/htmlcov/index.html  # Linux
```

**Web:**

```bash
make test-web-cov
open web/coverage/lcov-report/index.html  # macOS
xdg-open web/coverage/lcov-report/index.html  # Linux
```

## CI/CD integration

### GitHub actions

Coverage is automatically generated and uploaded to Codecov on every pull request:

- **Go CI** ([.github/workflows/go-ci.yml](../.github/workflows/go-ci.yml)): Runs tests with `-coverprofile=coverage.out`
- **Python CI** ([.github/workflows/python-ci.yml](../.github/workflows/python-ci.yml)): Uses `pytest-cov` to generate XML reports
- **Web CI** ([.github/workflows/web-ci.yml](../.github/workflows/web-ci.yml)): Uses Jest's built-in coverage to generate lcov reports

### Codecov configuration

The repository includes a [codecov.yml](../codecov.yml) configuration that:

- Defines separate flags for `go`, `python`, and `web` components
- Configures path-based coverage tracking
- Sets coverage thresholds
- Enables PR comments with coverage diffs

### Setting up codecov token

For CI to upload coverage data, a `CODECOV_TOKEN` secret must be configured:

1. Sign up/login to [Codecov](https://codecov.io) with the GitHub account that owns the repository
2. Add the repository to Codecov
3. Copy the upload token from the Codecov repository settings
4. Add it as a repository secret:
   - Go to GitHub repository → Settings → Secrets and variables → Actions
   - Create a new secret named `CODECOV_TOKEN` with the token value

## Coverage goals

The project aims for:

- **Go**: 80%+ coverage for core packages
- **Python**: 90%+ coverage (enforced via `pytest-cov`)
- **Web**: 90%+ coverage (enforced via Jest threshold in `jest.config.js`)

## Excluding files from coverage

### Go

Use the `//go:build ignore` build tag to exclude generated code from coverage.

### Python

Configure `.coveragerc` or use the inline `# pragma: no cover` comment to exclude specific lines or branches.

### Web

Configure exclusions in [web/jest.config.js](../web/jest.config.js) under `collectCoverageFrom`.

## Troubleshooting

### Coverage not uploading to codecov

1. Verify `CODECOV_TOKEN` is set in repository secrets
2. Check that the workflow has `permissions: write` for `pull-requests`
3. Review the CI logs for any Codecov upload errors

### Local coverage reports not generating

Ensure dependencies are installed:

```bash
# Go: No extra dependencies needed
go test -coverprofile=coverage.out ./...

# Python: pytest and pytest-cov
make install-python

# Web: Jest and coverage tools
make install-web
```

### Coverage percentage seems wrong

- **Go**: Make sure you're running tests with `-covermode=atomic` for accurate concurrency coverage
- **Python**: Check that `PYTHONPATH` includes the package root
- **Web**: Verify `collectCoverageFrom` patterns in `jest.config.js` are correct
