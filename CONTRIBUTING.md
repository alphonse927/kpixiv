# Contributing

Thank you for contributing to KPixiv. This guide covers the development
workflow and conventions.

## Development setup

Requirements:

- Go 1.26+
- `golangci-lint`
- Fyne build dependencies (see the CI workflow in `.github/workflows` for a
  full list)

Build both binaries into the workspace:

```bash
make build
```

Install a symlinked dev build that mirrors the installed layout:

```bash
make dev-install
```

## Validation

Code is not complete until it builds and passes every check:

```bash
go build ./...
go vet ./...
golangci-lint run ./...
go test ./...
```

Changes that affect behavior should be validated at runtime (for example a
`kpixivctl` command should be run against a scratch `--config` before it is
considered done).

## Conventions

- Follow the project style guide in the repository root: small packages,
  clear naming, explicit error handling, stdlib first.
- Keep commits focused on a single logical change.
- Match the existing commit style (`feat:`, `fix:`, `refactor:`, `docs:`,
  `test:`, `ci:`, `clean:`).
- Run `gofmt` (or `golangci-lint`) before committing.
- Write tests for new logic; keep them free of network and destructive
  filesystem operations.

## Documentation

- Behavioral discoveries about Pixiv APIs belong in `docs/research/`.
- CLI changes belong in `docs/cli.md`.
- Release notes are generated from the git log; keep commit messages
  descriptive.

## Releasing

Versioning is tag-driven. See the **Versioning** and **Cutting a Release**
sections of the project documentation for the exact process.
