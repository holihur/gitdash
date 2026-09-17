# gitdash handover (CI / PR / email / search)

> For whoever picks up **#3** and **#4**. Summarises what landed, the remaining
> design, a code map, and the dev/test conventions. Chinese version:
> [handover.zh-CN.md](handover.zh-CN.md).
> Remotes: `origin` = GitHub, `gitdash` = self-hosted instance.

## 0. Starting point

- Base: `origin/main`. This round is 10 commits (`git log --oneline origin/main..HEAD`):
  split-god-pages refactor, CI secrets/cache/artifacts, and the five PR features
  (draft, suggestion, CODEOWNERS, auto-merge, merge queue).

## 1. Done

### 1.1 God-page refactor (pure extraction, `c694ebb`)
RepoIssues, RepoView, projects-board, settings-tab, code-tab split into focused
components/hooks. See the Chinese doc for the size table.

### 1.2 #1 CI: secrets + cache + artifacts
- **secrets** (`5408886`): `store/secrets.go` + `repo_secrets` (AES-256-GCM at rest);
  DSL `secrets: [NAME]`; injected as env and masked (`***`) in logs; API
  `GET/PUT/DELETE /users/{owner}/repos/{name}/secrets`; settings card.
- **cache** (`1c66911`): DSL `cache: { key, paths }`; restore/save in `pipeline/cache.go`;
  `data/cache/{owner}/{repo}/{key}` (runner: `GITDASH_RUNNER_CACHE`).
- **artifacts** (`3739a4e`): DSL `artifacts: { paths }`; archived to
  `data/artifacts/{owner}/{repo}/run-{id}`; list + tar.gz download endpoints.
  **Limitation:** remote runners do not upload artifacts yet.
- DSL parsing in `pipeline/dsl.go`; docs in `pipeline-docs.tsx`.

### 1.3 #2 PR features
| Item | Commit | Notes |
|---|---|---|
| Draft PR | `344be4e` | `draft` column; `POST /pulls/{n}/draft`; merging a draft → 409 |
| Suggestion | `643861d` | ` ```suggestion ` in inline comments; `POST /pulls/{n}/comments/{id}/apply` |
| CODEOWNERS | `ad1d49d` | `gitsvc/codeowners.go`; `require_codeowners` branch rule; `GET /pulls/{n}/codeowners` |
| Auto-merge | `b4665d1` | `auto_merge(_method)`; `POST /pulls/{n}/auto-merge`; triggered on review / pipeline success |
| Merge queue | `d880034` | `merge_queue` table; `merge_queue` branch rule; merge → 202 enqueue, serial processor |

Merge logic lives in `api/merge.go` (`mergeGateError` / `executeMerge` /
`tryAutoMerge` / `processMergeQueue`). **Add new merge gates only there.**
Pipeline success hook: `pipeline.SetRunSuccessHook`, wired in `main.go`.

### 1.4 Black-box tests
`tests/`: `test_pipeline_secrets.py`, `test_pipeline_host.py` (secrets/cache/artifacts
end-to-end), `test_codeowners.py`, `test_auto_merge.py`, `test_merge_queue.py`, plus
draft/suggestion cases in `test_pulls.py`.

## 2. TODO #3 — email as a first-class channel

**Read first:** activity email notifications **already exist** —
`notify.EmailHandler` (in `main.go`) sends to `store.NotifyRecipients` filtered by
`store.EmailTargets` (`users.notify_email = true`). SMTP is `notify/email.go`
(nil/no-op when unset). So only two things remain:

1. **Reply-by-email**: add `Message-ID` + `Reply-To: reply+<hmac-token>@<domain>` in
   `buildMessage`; add `POST /api/mail/inbound` (provider/MTA pipe, shared secret) that
   verifies the token, strips quoted text/signature, and calls
   `store.CreateComment(...)`. Token = HMAC over `owner|repo|kind|number|username`.
2. **Patch-by-email**: add `gitsvc.ApplyPatchSeries` (`git am` in a temp clone → push a
   `patches/<ts>` branch) and `POST /users/{owner}/repos/{name}/patches`, then open a PR
   via the existing `store.CreatePull`. Accept mbox/patch from `git send-email`.

Tests: `tests/test_mail_inbound.py`, `tests/test_patches.py`.

## 3. TODO #4 — global code search

- Today: repo-scoped `GET /users/{owner}/repos/{name}/search`; global search is
  repo-only (`GET /api/search`). Frontend: `lib/api/search.ts`, `pages/Explore.tsx`.
- MVP: `GET /api/search/code?q=&repo=&lang=&path=&limit=` with qualifiers
  `repo:` / `lang:` / `path:` / `symbol:`; enumerate **accessible** repos, run
  `git grep` on default branches with a worker pool + timeout + caps; return
  `{results:[{owner,repo,path,line,text}], truncated}`.
- Frontend: add `searchCode` + types, render a "Code" section in Explore, i18n en/zh.
- Later: incremental index (zoekt/trigram) under `internal/search/`, same API.
- Test: `tests/test_code_search.py` (qualifiers, permissions, private hidden).

## 4. Conventions

- **Backend**: Go stdlib HTTP + GORM. New tables: `store/models.go` + register in
  `store/migrate.go` `AutoMigrate`. New handlers: `api/<feature>.go`, routes in
  `api/api.go`, DTOs in `api/dto_*.go`, swagger annotations on handlers.
  **Struct conversions**: `store.Comment` = `Comment(commentRow)` and
  `BranchProtection(row)` require identical field sets — add fields on both sides.
  `PullRequest` is a hand-written DTO (`pullToDTO`).
- **Frontend**: React/TS/Tailwind. API in `lib/api/*`, types in `lib/api/types.ts`.
  **i18n: `locales/en.ts` and `locales/zh-CN.ts` must stay key-identical**
  (`src/test/locales.test.ts` enforces it); other locales are subsets.
- **OpenAPI**: run `task swagger` and commit `internal/api/docs/*` after API changes.
- **Commits**: Conventional Commits; one feature per commit; refactors separate.
  Workflow: implement → test → commit, then next item.

## 5. Commands

```bash
cd backend && go test ./... && go vet ./...
cd frontend && pnpm exec tsc --noEmit && pnpm test && pnpm build

# black-box API tests
cd backend && go build -o /tmp/gitdash-server-pytest .
cd tests && GITDASH_BIN=/tmp/gitdash-server-pytest .venv/bin/python -m pytest -q

task test        # Go + frontend + e2e smoke
task test:all    # + pytest + Playwright
```

> Without network, `uv run` may fail (mirror 403); use the existing
> `tests/.venv/bin/python -m pytest` directly.

## 6. Code map

| Concern | File |
|---|---|
| Pipeline DSL | `backend/internal/pipeline/dsl.go` |
| Pipeline run | `pipeline/pipeline.go` (`executeRun`), `executor.go` (`RunInWorkspace`) |
| secrets/cache/artifacts | `store/secrets.go`, `pipeline/{masking,cache,artifacts}.go`, `api/{secrets,artifacts}.go` |
| Remote runner | `runner/{proto,remote,hub}.go`, `cmd/gitdash-runner/main.go` |
| Merge gate/exec | `api/merge.go` |
| CODEOWNERS | `gitsvc/codeowners.go`, `api/codeowners.go` |
| Branch protection | `store/branchprotection.go`, `api/branchprotection.go` |
| Comments / inline | `store/comments.go`, `api/comments.go`, `frontend/src/components/diff-view.tsx` |
| Notifications | `store/notifications.go`, `notify/email.go`, `api/inbox.go` |
| Search | `api/search.go`, `frontend/src/lib/api/search.ts`, `pages/Explore.tsx` |
| Wiring | `backend/main.go` |

## 7. Gotchas

1. Remote runner ≠ local: artifacts are server-local only; cache uses runner env; secrets
   are sent over the WS (use TLS/wss).
2. Approvals are pinned to a commit SHA; a new head invalidates them (suggestions do too).
3. Merge queue blocks the whole line when the head entry can't pass; retried on events.
4. `AutoMigrate` adds columns/tables but never changes primary keys.
5. DSL parser is line-based; new nested blocks need a dedicated reader.
6. `Sender` is nil when SMTP is unset — guard every send.
7. Regenerate OpenAPI after API changes.
8. `push` events intentionally do not email.
