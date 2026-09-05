# gitdash

[![CI](https://github.com/holihur/gitdash/actions/workflows/ci.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/ci.yml)
[![Release](https://github.com/holihur/gitdash/actions/workflows/release.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/release.yml)

[English](README.md) | [简体中文](README.zh-CN.md)

A minimal self-hosted Git service MVP (like a mini Gitea):

- **User system**: register / login (bcrypt + session token, valid for 7 days); repos and SSH keys belong to users
- **Watching & inbox**: watch / unwatch repos; repo issue / PR activity (opened / closed / reopened / merged) is pushed to your personal inbox (unread badge + read management)
- **CI pipeline (MVP)**: per-repo pipeline toggle in the web UI; on push, steps defined in `.gitdash.yml` (custom YAML DSL) run inside Docker containers with logs stored per run; jobs can be processed in-process (default) or via a Redis-backed asynq queue
- **Git SSH service**: built-in SSH server (default `:2222`), public keys bound to users, supports `git clone` / `push` / `pull`
- **Code browsing**: browse repos by branch / directory, view file contents and commit history on the web
- **SSH key management**: add / remove public keys via the web UI (CRUD); a public key acts as the user's credential
- **Self-update**: `gitdash update` for manual updates; optional background auto-update (**off by default**)
- **Frontend**: React + Vite + Tailwind + shadcn/ui-style components
- **Backend**: Go standard library HTTP + `golang.org/x/crypto/ssh` + SQLite (modernc, pure Go, no CGO)
- **Single-binary releases**: GoReleaser embeds the frontend into the binary at release time — download and run
- **Automated tests**: `backend/tests/` covers features (auth / repos / keys / browse / ssh git / updater / store / webui); the root `tests/` directory holds a fully isolated black-box API test suite (pytest + requests + uv)

## One-line Install

```bash
curl -fsSL https://raw.githubusercontent.com/holihur/gitdash/main/install.sh | bash
gitdash serve
# Open http://localhost:8080 (SSH :2222)
```

The install script supports environment variables: `GITDASH_VERSION` (pin a version) and `GITDASH_INSTALL_DIR` (install directory).

You can also download a platform archive from [Releases](https://github.com/holihur/gitdash/releases) (frontend embedded), or deploy with systemd via [packaging/gitdash.service](packaging/gitdash.service).

## Usage

1. Open the web UI → register an account (e.g. `alice`)
2. **SSH Keys** → paste a public key (e.g. `~/.ssh/id_ed25519.pub`)
3. **Repos** → create a repo (e.g. `demo`, actual path `alice/demo`)
4. Clone and push:

```bash
git clone ssh://git@<host>:2222/alice/demo.git
cd demo
echo "# demo" >> README.md
git add README.md && git commit -m "initial commit"
git push origin main
```

5. Return to the web UI to browse code and commit history. The clone URL also accepts a single-segment form without the owner (`ssh://git@<host>:2222/demo.git` resolves to the repo owned by the currently logged-in user).

## Quick Start (Development)

### 1. Start the backend

```bash
cd backend
go run .
# HTTP  :8080   Git SSH :2222   data dir ./data
```

Environment variables (all optional):

| Variable | Default | Description |
| --- | --- | --- |
| `GITDASH_HTTP_ADDR` | `:8080` | Web / API listen address |
| `GITDASH_SSH_ADDR` | `:2222` | Git SSH listen address |
| `GITDASH_DATA` | `./data` | Data directory (repos, host key; SQLite file lives here by default) |
| `GITDASH_DB` | `./data/gitdash.db` | Database: SQLite file path, or a `postgres://` URL to use PostgreSQL (schema auto-migrated; no data migration from existing SQLite files) |
| `GITDASH_STATIC` | auto-detect | Frontend static files directory (dev mode overrides embedded assets) |
| `GITDASH_AUTO_UPDATE` | off | **Auto-update is off by default**; set to `1`/`true`/`yes`/`on` to enable |
| `GITDASH_AUTO_UPDATE_INTERVAL` | `24h` | Auto-update check interval (minimum 1h) |
| `GITDASH_UPDATE_REPO` | `holihur/gitdash` | Source repo for updates (for forks / testing) |
| `GITDASH_QUEUE` | `memory` | Pipeline job queue: `memory` (in-process goroutines) or `redis`/`asynq` (durable Redis queue) |
| `GITDASH_REDIS_ADDR` | `127.0.0.1:6379` | Redis address for the asynq queue |
| `GITDASH_REDIS_PASSWORD` / `GITDASH_REDIS_DB` | empty / `0` | Redis auth / database index |
| `GITDASH_QUEUE_CONCURRENCY` | `4` | Worker concurrency for the asynq queue |

**Changing listen addresses**:

```bash
GITDASH_HTTP_ADDR=:9090 GITDASH_SSH_ADDR=:2322 gitdash serve
```

- For systemd: edit the corresponding `Environment=` lines in `packaging/gitdash.service`, then `systemctl daemon-reload && systemctl restart gitdash`
- Note: the clone URL shown in the web UI is hardcoded to port 2222; if you change the SSH port, adjust clone commands manually

### 2. Start the frontend (development)

```bash
cd frontend
pnpm install
pnpm run dev
# Open http://localhost:5173; /api is proxied to :8080
```

### 3. Production mode (single binary with embedded frontend)

```bash
bash scripts/embed-frontend.sh   # build the frontend and copy it into backend/internal/webui/dist
cd backend && go build -o gitdash .
./gitdash serve
./gitdash version
```

It also builds without embedding, in which case it serves from a disk directory (`GITDASH_STATIC` / `./static` / `../frontend/dist`).

## Update / Auto-update

```bash
# Manual update to the latest release (verifies SHA256, then atomically replaces the current binary)
gitdash update

# Auto-update: off by default; enable explicitly. Recommended together with systemd Restart=always
GITDASH_AUTO_UPDATE=1 gitdash serve
```

When enabled, the process periodically checks GitHub Releases; on a new version it downloads it (verifying checksums.txt), replaces its own binary, and exits so that systemd (`Restart=always`) starts the new version. `dev` builds are excluded from auto-update, but can be updated manually via `gitdash update`.

## Docker Deployment

```bash
docker compose up -d --build
# Web http://localhost:8080, Git SSH localhost:2222
```

- Data (SQLite + bare repos + SSH host key) is persisted in the Docker volume `gitdash-data` (`/data` inside the container).
- Listen ports / auto-update can be adjusted via `environment` in `docker-compose.yml`.
- You can also build the image directly: `docker build -t gitdash .`, then mount `-v gitdash-data:/data -p 8080:8080 -p 2222:2222`.

## Backup & Restore

```bash
# Online backup (consistent SQLite snapshot + repo archives, keeps the latest 14)
bash scripts/backup.sh ./data ./backups
# KEEP=30 bash scripts/backup.sh   # keep more copies
```

Restore: stop the service, unpack the backup into the data directory (`tar -xzf gitdash-backup-*.tar.gz -C <data dir>`), then start again.
If `sqlite3` is not available on the host, the script falls back to a straight file copy (in WAL mode, stop the server first for consistency); inside Docker you can also run the same script via `docker compose exec gitdash bash`.

## Automated Testing

Static checks (golangci-lint, config in `backend/.golangci.yml`, runs automatically in CI):

```bash
cd backend
golangci-lint run     # install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

Backend integration tests (Go):

```bash
cd backend
go test ./...          # unit tests + backend/tests/ integration tests
bash scripts/e2e.sh    # full-chain smoke test (real binary: register/login -> ssh clone/push -> browse -> multi-user isolation)
```

Black-box API tests (pytest + requests, dependencies managed with uv, fully isolated from backend source/builds):

```bash
# 1) Build the binary under test
(cd backend && go build -o /tmp/gitdash-server .)
# 2) Run: fixtures start a fresh instance on an isolated temp data dir + random port, then destroy it
(cd tests && GITDASH_BIN=/tmp/gitdash-server uv run pytest -v)
# Or point at an already-running instance: GITDASH_API_URL=http://127.0.0.1:8080 uv run pytest
```

- `backend/tests/` is split by feature: `auth` (register/login/session), `repos` (repo CRUD & isolation), `sshkeys` (public key CRUD & binding), `browse` (tree/blob/commits), `sshgit` (real SSH clone/push & permission denial), `updater` (version comparison/verification/extraction), `store` (schema migration), `webui` (static hosting/SPA fallback/path traversal protection). All run automatically in CI.
- `tests/` (repo root) is an **independent, isolated** pure black-box API test suite: it does not import backend code or run go builds; test cases use random names and share no state, covering happy path and bad path (400/401/404/409…) for auth / repos / issues / ssh keys. See `tests/README.md` for details.

## API Overview

Auth (public):

| Method | Path | Description |
| --- | --- | --- |
| POST | `/api/auth/register` | Register, returns a session token |
| POST | `/api/auth/login` | Login, returns a session token |
| POST | `/api/auth/logout` | Logout (invalidates the current token) |
| GET | `/api/me` | Current user |
| GET | `/api/health` `/api/version` | Health check / version |

Business (requires `Authorization: Bearer <token>`, token from register/login):

| Method | Path | Description |
| --- | --- | --- |
| GET/POST | `/api/repos` | List / create your own repos |
| GET/DELETE | `/api/repos/{name}` | Details / delete |
| GET | `/api/repos/{name}/branches` | Branch list |
| GET | `/api/repos/{name}/tree?ref=&path=` | Browse directory |
| GET | `/api/repos/{name}/blob?ref=&path=` | File content |
| GET | `/api/repos/{name}/commits?ref=` | Commit history |
| GET/POST | `/api/keys` | List / add SSH public keys (bound to the current user) |
| DELETE | `/api/keys/{id}` | Delete your own public key |

Repo social / inbox (watch → subscribe to repo activity in your inbox):

| Method | Path | Description |
| --- | --- | --- |
| PUT/DELETE | `/api/users/{owner}/repos/{name}/watch` | Watch / unwatch (returns watch count & status) |
| GET | `/api/watched` | List of repos you watch |
| GET | `/api/inbox` | Inbox notifications (newest first) |
| GET | `/api/inbox/unread` | Unread count |
| POST | `/api/inbox/read` `/api/inbox/read/{id}` | Mark all / one as read |
| DELETE | `/api/inbox/{id}` | Delete one notification |

> MVP note: repos are owner-only (only the owner can read/write); add TLS at the HTTP layer yourself (or place behind a reverse proxy).

## CI Pipeline (MVP)

Enable the pipeline in the repo's **Pipeline** tab (owner only). On every push to a branch, gitdash reads `.gitdash.yml` at the pushed commit and executes the steps in Docker containers (workspace mounted at `/workspace`); steps run in order and the first failure stops the run. Manual runs are available from the tab as well. Run logs are kept under `<data>/pipelines/{owner}/{repo}/`.

Custom YAML DSL (supported subset):

```yaml
image: alpine:3.19   # required: image for every step (POSIX sh required)
timeout: 10m         # optional: per-step timeout (default 10m, max 1h)
env:                 # optional: KEY=VALUE list injected into containers
  - CGO_ENABLED=0
steps:               # required: 1..20 steps
  - name: build
    run: go build ./...
  - name: test
    run: |
      go test ./...
      go vet ./...
```

Pipeline API:

| Method | Path | Description |
| --- | --- | --- |
| GET/PUT | `/api/users/{owner}/repos/{name}/pipeline` | Get / set enabled (PUT: owner only) |
| GET/POST | `/api/users/{owner}/repos/{name}/pipeline/runs` | List runs / trigger a manual run (`{ref?}`) |
| GET | `/api/users/{owner}/repos/{name}/pipeline/runs/{id}` | Run detail incl. log |

## Upgrade Notes

Since v0.2 the data model includes a user system; for older versions (≤ v0.1), the `repos` / `ssh_keys` tables in the `data` directory are automatically reset at startup (bare repos on disk are preserved but must be re-registered / migrated under a user).

## CI / Release

- **CI** (`.github/workflows/ci.yml`): Go build/vet/test + `backend/tests/` integration tests + E2E smoke test; frontend tsc + vite build.
- **Release** (`.github/workflows/release.yml`): pushing a tag triggers an automatic release:

```bash
git tag v0.2.0
git push origin v0.2.0
```

GoReleaser first builds and embeds the frontend, then produces archives and checksums for linux / darwin / windows × amd64 / arm64, published to GitHub Releases.

Verify the release config locally:

```bash
goreleaser check
goreleaser release --snapshot --clean --skip=publish
```
