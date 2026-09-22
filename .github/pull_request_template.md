## Summary

<!-- What does this change and why? Link the issue: Closes # -->

## Changes

-

## Testing

- [ ] `cd backend && golangci-lint run && go vet ./... && go test ./...`
- [ ] `cd backend && go test -race ./...`
- [ ] black-box: `cd tests && uv run pytest` (SQLite; CI also runs PostgreSQL)
- [ ] frontend: `cd frontend && pnpm lint && pnpm test && pnpm build`
- [ ] UI changed? `cd tests/ui && npx playwright test`

## Checklist

- [ ] Docs updated (`README.md` / `README.zh-CN.md` / `docs/`) where behavior changed
- [ ] No secrets / tokens / private keys committed
- [ ] Works on both SQLite and PostgreSQL (see `docs/content/en/self-hosting/database.md`)
- [ ] Backwards compatible (migrations additive, no data loss)
- [ ] New/changed backend error codes have a frontend `errors.*` entry (contract test covers it)
