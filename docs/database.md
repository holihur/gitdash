# Database Backends (SQLite / PostgreSQL)

gitdash supports two storage backends behind the same `store.Store` API:

- **SQLite** (default) — pure-Go `modernc` driver, file under `GITDASH_DATA`
- **PostgreSQL** — set `GITDASH_DB=postgres://user:pass@host:5432/db?sslmode=disable`

Both are migrated identically with GORM `AutoMigrate` in `internal/store/migrate.go`.

## Shared invariants

- **Timestamps** are stored as RFC3339 UTC **strings** (`TEXT` / `varchar`), never native date types. Ordering and `<`/`>` comparisons are therefore lexicographic and behave the same on both backends.
- **Booleans** are `0/1` on SQLite and `boolean` on PostgreSQL; GORM serialises them transparently, including explicit `false` on insert (repo `private` uses a map insert to bypass zero-value elision).
- **Empty strings** are always `NOT NULL DEFAULT ''`; there is no `NULL` vs `''` ambiguity.
- **Upserts** use `ON CONFLICT ... DO UPDATE`, supported by both.
- **Partial unique index** on `users.email` (`WHERE email <> ''`) is supported by both; empty emails are not unique.
- **Unique-violation mapping**: `TranslateError: true` converts driver errors to `gorm.ErrDuplicatedKey`; `isUniqueErr` maps them to `store.ErrExists`/`ErrNotFound`.

## Known differences and how they are handled

| Area | SQLite | PostgreSQL | Handling in code |
| --- | --- | --- | --- |
| Write concurrency | single writer; `busy_timeout(5000)` + WAL | MVCC, row-level locks | short transactions; explicit `busy_timeout` pragma |
| `LIKE` case sensitivity | ASCII case-insensitive | case-sensitive | user-facing filters use `LOWER(col) LIKE <lowercased pattern>` (search, admin user list); prefix matches use lowercase keys |
| Lock/contention errors | `SQLITE_BUSY` retried by busy_timeout | lock waits / serialization failures | not retried explicitly; keep transactions short |
| Dialect-specific SQL | — | — | keep queries to the GORM/portable subset; raw SQL only where quoted identifiers (`"key"`) work on both |
| Quota enforcement race | process-wide `createMu` + `COUNT` | process-wide `createMu` + `COUNT` | multi-instance deployments can still over-admit; documented limitation (see `internal/store/quota.go`) |

## Where to look

- Backend selection: `store.Open` / `store.OpenDSN`
- Migration / schema: `internal/store/migrate.go`
- Error mapping: `isUniqueErr` (`internal/store/store.go`)
- PostgreSQL smoke test: `internal/store/pgsmoketest/` (runs in CI with a postgres service)

```bash
# PostgreSQL smoke test
GITDASH_DB="postgres://gitdash:pg@127.0.0.1:54329/gitdash?sslmode=disable" \
  go test ./internal/store/pgsmoketest/ -count=1 -v
```
