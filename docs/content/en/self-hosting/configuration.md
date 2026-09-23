---
title: "Configuration"
weight: 2
summary: "Common environment variables: ports, data dir, secret, proxies, SSRF, docs."
---

Common environment variables:

| Variable | Description |
|---|---|
| `GITDASH_DATA` | Data directory |
| `GITDASH_HTTP_ADDR` / `GITDASH_SSH_ADDR` | Listen addresses |
| `GITDASH_ADMIN_USER` / `GITDASH_ADMIN_PASSWORD` | Create the administrator on first boot |
| `GITDASH_SECRET_KEY` | Encryption key (credentials/tokens) |
| `GITDASH_DOCS_URL` | Documentation site URL (login-page/header entry) |
| `GITDASH_SSRF_ALLOW_PRIVATE` | Allow imports/mirrors to reach private networks |
| `GITDASH_TRUSTED_PROXIES` | Trusted reverse-proxy addresses |
| `GITDASH_CODE_SEARCH` | Code search backend: `bleve` (**default**, embedded full-text index, rebuilt incrementally; identifier-aware + CJK + smart-case; default branch only; eventually consistent—`indexing:true` while rebuilding), `grep` (explicit live `git grep`), or `remote` |
| `GITDASH_SEARCH_URL` / `GITDASH_SEARCH_TOKEN` | Remote index service URL / shared bearer token (required by `GITDASH_CODE_SEARCH=remote` on the API node, and by `GITDASH_ROLE=codeindex` on the worker) |
| `GITDASH_ROLE=codeindex` | Run a dedicated index worker: consumes the `gitdash:codeindex` asynq queue, owns the index, and serves the internal search endpoint (needs `GITDASH_QUEUE=redis` and shared data/DB) |
| `GITDASH_SEARCH_LISTEN` / `GITDASH_CODE_INDEX_CONCURRENCY` / `GITDASH_CODE_INDEX_CONSUME` | Worker search listen address / index concurrency / set `0` on the API node to only produce index tasks |

See the repository README for the full list.

### Split code-search deployment

Bleve is single-process (one writer), so a dedicated index worker owns the index and the
API node delegates searches to it:

```bash
# Index worker (owns the index; consume the asynq queue)
GITDASH_ROLE=codeindex GITDASH_QUEUE=redis GITDASH_REDIS_ADDR=redis:6379 \
GITDASH_SEARCH_TOKEN=$TOKEN GITDASH_SEARCH_LISTEN=0.0.0.0:8090 gitdash serve

# API node (produce index tasks, search via the worker; no local index)
GITDASH_QUEUE=redis GITDASH_REDIS_ADDR=redis:6379 \
GITDASH_CODE_SEARCH=remote GITDASH_SEARCH_URL=http://index-worker:8090 \
GITDASH_SEARCH_TOKEN=$TOKEN gitdash serve
```

Both processes must share `GITDASH_DATA` (repos) and the database. Without Redis, code
indexing runs in-process (`GITDASH_CODE_SEARCH=bleve`) and `remote` is unnecessary.
