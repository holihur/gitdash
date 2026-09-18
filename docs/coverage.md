# Coverage & Defect-Hunting Roadmap

> The point of coverage is to find defects. Numbers are targets, not the goal:
> every newly-covered endpoint/line is a place where a bug can hide. This file
> tracks the staged plan and the defects found so far.

## Metrics

| Metric | How it is measured | Current | Target |
|---|---|---|---|
| Go unit coverage | `go test ./... -covermode=atomic -coverprofile=…` (Codecov `unittests`) | ~30% | 80% |
| Black-box API endpoint coverage | pytest with `GITDASH_ROUTE_COVERAGE_FILE` + `scripts/route-coverage.py` | **306/306 (100%)** | 100% |
| UI (Playwright) endpoint coverage | `scripts/ui-api-coverage.py` (frontend-called endpoints × route hits) | 101/192 (52.6%) | 100% of the UI-reachable surface |

The endpoint metrics are *route-hit* coverage: every registered `http.ServeMux`
pattern that receives at least one request. `internal/api/routecov.go` wraps the
mux to record the inventory (`route<TAB>pattern`) and the first hit of each
pattern (`hit<TAB>pattern`).

## Scope of "UI endpoint coverage 100%"

Not all 306 routes have a web UI entry point (package registries, Docker `/v2/`,
SSH, OAuth callbacks, the admin panel, git protocol endpoints, …). The surface is
therefore defined **from the frontend source**: `scripts/ui-api-coverage.py`
statically extracts every endpoint the UI can call (192 today) and matches it
structurally against the recorded route hits, so literal segments match route
wildcards. The 100% target is against that UI-reachable set, not the full
inventory. The same run flags UI-called paths that match **no** registered route
(frontend/backend mismatches).

## How to measure locally

```bash
# unit
cd backend && go test ./... -covermode=atomic -coverprofile=/tmp/unit.out
go tool cover -func=/tmp/unit.out | tail -1

# black-box API endpoint coverage
cd backend && go build -cover -o /tmp/gitdash-server .
cd tests && GITDASH_BIN=/tmp/gitdash-server \
  GITDASH_ROUTE_COVERAGE_FILE=/tmp/routes-api.txt .venv/bin/python -m pytest -q
python3 scripts/route-coverage.py /tmp/routes-api.txt --min 100

# UI endpoint coverage (needs the embedded frontend in the binary)
(cd frontend && pnpm run build)
rm -rf backend/internal/webui/dist && mkdir -p backend/internal/webui/dist
cp -r frontend/dist/. backend/internal/webui/dist/ && touch backend/internal/webui/dist/.gitkeep
(cd backend && go build -cover -o /tmp/gitdash-server-ui .)
cd tests/ui && GITDASH_BIN=/tmp/gitdash-server-ui \
  GITDASH_ROUTE_COVERAGE_FILE=/tmp/routes-ui.txt npx playwright test
python3 scripts/ui-api-coverage.py --src frontend/src --coverage /tmp/routes-ui.txt --list-missing
```

Or all of it at once: `bash scripts/coverage-blackbox.sh`.

## Staged plan

1. **Stage 1 — instrumentation + API endpoint coverage (done).**
   Route inventory/recorder, checker script, CI gate at 100% for black-box API
   tests; probe untested endpoints and fix what they expose.
2. **Stage 2 — UI endpoint coverage (in progress).** The UI-reachable surface is
extracted from the frontend (192 endpoints); Playwright covers 101 (52.6%) today.
Add page flows in batches (repo settings, projects, pipeline, pulls, copilot,
deletes) until the surface is 100%; CI reports it non-blocking meanwhile.
3. **Stage 3 — Go unit coverage ramp.** Raise unit coverage package by package
   (gitsvc, store, api first), adding regression tests for every defect found.
4. **Stage 4 — ratchet.** Make the Codecov project status blocking at 80%, and
   make the UI endpoint coverage gate blocking for the UI subset.

## Defects found

| # | Area | Symptom | Status |
|---|---|---|---|
| 1 | Admin auth | `POST /api/admin/password` did not invalidate other admin sessions (user password change does) | fixed (`store.DeleteAdminSessionsExcept`) |
| 2 | Packages | `GET /api/packages/{owner}` (all types) always returned `[]`: `ListPackages` filtered `type = ''` | fixed |
| 3 | Composer | `GET /api/packages/composer/{owner}/p2/{vendor}/{name}` returned `packages` as a nested **array** instead of an object keyed by `vendor/name`, breaking Composer 2 clients | fixed |
| 4 | UI tests | `repos.spec.ts` looked for the clone command on the repo list card, but it moved into the repo header SSH dropdown | fixed (test) |
| 5 | UI tests | `explore.spec.ts` used the old search placeholder (`…users…` → `…users, code…`) | fixed (test) |
| 6 | CI flake | `test_pipeline_host.py::test_host_cache_reuse` intermittently got `429 too_many_runs` under full-suite load — investigate active-run accounting race | open |
| 7 | Runners | Admin global registration token returns `scope: ""` while user/org return `user:x` / `org:x`; ambiguous but matches the internal `"" = global` convention | noted |
