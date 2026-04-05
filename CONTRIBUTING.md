# Contributing

Thanks for contributing to Gorkbot.

## Development Flow

1. Create a feature branch from `main`.
2. Keep changes scoped and production-safe.
3. Run local checks before opening a PR.

Recommended checks:

```bash
go test ./...
go vet ./...
bash scripts/release_checklist.sh
```

## Coding Standards

- Use idiomatic Go and run `gofmt -w` before commit.
- Prefer explicit error handling over implicit fallthrough.
- Keep app wiring in `cmd/` and `internal/`; reusable logic belongs in `pkg/`.
- Never commit secrets, tokens, or local credential files.

## Commit Format

Use conventional commits:

```text
feat(scope): summary
fix(scope): summary
refactor(scope): summary
test(scope): summary
docs(scope): summary
chore(scope): summary
```

## Pull Request Requirements

- Clear summary of problem and solution.
- List affected paths.
- Include test evidence.
- Include config/migration notes for behavior changes.

## Release Safety

- Public tags must match `VERSION` public version (`v<public-version>`).
- Use [docs/RELEASE_OPERATIONS.md](docs/RELEASE_OPERATIONS.md) for tagging and release workflow details.
