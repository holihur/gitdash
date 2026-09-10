# gitdash

[![CI](https://github.com/holihur/gitdash/actions/workflows/ci.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/ci.yml)
[![Release](https://github.com/holihur/gitdash/actions/workflows/release.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/release.yml)
[![codecov](https://codecov.io/gh/holihur/gitdash/graphs/badge.svg?branch=main)](https://codecov.io/gh/holihur/gitdash)

[English](README.md) | [简体中文](README.zh-CN.md)

A minimal self-hosted Git service MVP (like a mini Gitea):

- **User system**: register / login (bcrypt + session token, valid for 7 days); repos and SSH keys belong to users; profile email with uniqueness enforced (partially, empty allowed)
- **Organizations**: create orgs, manage members (owner / member roles), host repos under an org namespace
- **Issues & labels**: per-repo issues with labels & milestones; activity pushes to watchers' inboxes
- **Projects (kanban)**: per-repo kanban projects with columns, swimlanes and cards (issue-linked or text notes), drag & drop in the web UI — see [docs/projects.md](docs/projects.md)
- **Pull requests**: fork-based pull requests with squash merge and reviewer flow
- **Stars & forks**: star repos and fork them with one click
- **Repo mirroring & import**: import from a remote URL and push-mirror to GitHub/GitLab-like remotes
- **Webhooks**: per-repo webhooks with HMAC signature delivery
- **GPG keys**: upload GPG public keys to verify commit signatures
- **OAuth login**: GitHub OAuth, Google login and generic OIDC login (configurable in the admin panel)
- **Admin panel**: admin users, settings (OAuth providers), password management
- **Explore**: discover public repos; repo visibility (public / private) toggle in repo settings
- **Code browsing**: browse repos by branch / directory, view file contents, commit history and blame on the web
- **Private package registry**: publish & install packages for npm, composer (PHP), pypi (Python), rubygems (Ruby), Go modules, cargo (Rust) and Maven (Java) under user/org namespaces, authenticated with a PAT (Basic auth) — see [docs/packages.md](docs/packages.md)
- **Watching & inbox**: watch / unwatch repos; repo issue / PR activity (opened / closed / reopened / merged) is pushed to your personal inbox (unread badge + read / delete management)
- **CI pipeline (MVP)**: per-repo pipeline toggle in the web UI; on push, steps defined in `.gitdash.yml` (custom YAML DSL) run inside Docker containers with logs stored per run; jobs can be processed in-process (default) or via a Redis-backed asynq queue
- **Self-hosted runners**: deploy `gitdash-runner` agents that connect out to the server; `.gitdash.yml` can target them via `runs-on` labels (user/org scoping, workspace snapshot streaming, log streaming, cancel, offline detection) — see [docs/runners.md](docs/runners.md)
- **Git SSH service**: built-in SSH server (default `:2222`), public keys bound to users, supports `git clone` / `push` / `pull`
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

### Windows

```powershell
irm https://raw.githubusercontent.com/holihur/gitdash/main/install.ps1 | iex
gitdash serve
```

### macOS / Homebrew (macOS & Linux)

```bash
brew install holihur/tap/gitdash
```

### Windows / Scoop

```powershell
scoop bucket add gitdash https://github.com/holihur/scoop-bucket
scoop install gitdash/gitdash
```

### Nix

```bash
nix-env -iA nixpkgs.gitdash   # via holihur/nur-packages (NUR)
```

### Arch Linux (AUR)

```bash
yay -S gitdash-bin
```

### Debian / Ubuntu (apt)

```bash
# 从 Releases 下载 .deb 后安装
curl -fsSLO https://github.com/holihur/gitdash/releases/latest/download/gitdash_linux_amd64.deb
sudo apt install ./gitdash_linux_amd64.deb
sudo systemctl enable --now gitdash
```

### RHEL / CentOS / Fedora (yum)

```bash
curl -fsSLO https://github.com/holihur/gitdash/releases/latest/download/gitdash_linux_amd64.rpm
sudo yum install ./gitdash_linux_amd64.rpm   # 或 dnf install
sudo systemctl enable --now gitdash
```

### Alpine (apk)

```bash
curl -fsSLO https://github.com/holihur/gitdash/releases/latest/download/gitdash_linux_amd64.apk
sudo apk add --allow-untrusted ./gitdash_linux_amd64.apk
```

> deb/rpm/apk 包内含 systemd 服务文件（Alpine 无 systemd，仅装二进制）并依赖系统 `git`。

> Package-manager channels are published automatically on release; the tap/bucket/NUR repositories and the `TAP_GITHUB_TOKEN` / `AUR_SSH_KEY` secrets must exist (otherwise those publishers are skipped).

You can also download a platform archive from [Releases](https://github.com/holihur/gitdash/releases) (frontend embedded), deploy with systemd via [packaging/gitdash.service](packaging/gitdash.service), run as a launchd service on macOS via [packaging/com.gitdash.server.plist](packaging/com.gitdash.server.plist), or as a Windows service via [packaging/gitdash.windows.md](packaging/gitdash.windows.md).

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

Runner (self-hosted CI agent) support requires Redis (`GITDASH_QUEUE=redis`); WS endpoint `/api/runner/ws`, registration `POST /api/runner/register`, management `GET/DELETE /api/runners`.

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

Black-box E2E UI tests (TypeScript + Playwright + headless Chrome, isolated under `tests/ui/`, one spec file per business domain):

```bash
task test:ui                              # build embedded-frontend binary + run UI tests
(cd tests/ui && GITDASH_BIN=/tmp/gitdash-server-ui npx playwright test --grep @happy)
# Or point at an already-running instance: GITDASH_UI_URL=http://127.0.0.1:8080 npx playwright test
```

- `backend/tests/` is split by feature: `auth` (register/login/session), `repos` (repo CRUD & isolation), `sshkeys` (public key CRUD & binding), `browse` (tree/blob/commits), `sshgit` (real SSH clone/push & permission denial), `updater` (version comparison/verification/extraction), `store` (schema migration), `webui` (static hosting/SPA fallback/path traversal protection). All run automatically in CI.
- `tests/` (repo root) is an **independent, isolated** pure black-box API test suite: it does not import backend code or run go builds; test cases use random names and share no state, covering happy path and bad path (400/401/404/409…) for auth / repos / issues / orgs / pulls / webhooks / visibility / blame / ssh keys. See `tests/README.md` for details.
- `tests/ui/` (repo root) is an **independent, isolated** pure black-box E2E UI test suite (Playwright + headless Chrome): registration → repo creation → code browsing → issues → star/fork/watch → inbox → collabs/webhooks/releases → PR merge; UI findings are logged in `tests/ui/bug.md`. See `tests/ui/README.md` for details.

### Code Coverage

Three coverage dimensions, all reported to [Codecov](https://codecov.io/gh/holihur/gitdash) in CI (badges above):

| Dimension | Method | Run locally | Current (2026-09) |
| --- | --- | --- | --- |
| Unit tests (Go) | `go test -coverprofile` | `task coverage:go` | 9.2% statements |
| Black-box API (pytest) | `go build -cover` + `GOCOVERDIR` | `task coverage:blackbox` | merged below |
| Black-box UI (Playwright) | same `GOCOVERDIR`, graceful-shutdown flush | merged below | merged below |
| **Black-box merged (API + UI)** | `go tool covdata` | `task coverage:blackbox` | **58.7% statements** |

Black-box coverage works by building the server binary with `go build -cover`, running all black-box suites against it with `GOCOVERDIR` set (the server shuts down gracefully on SIGTERM so counters flush), then summarizing with `go tool covdata percent / textfmt`.

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

Enable the pipeline in the repo's **Pipeline** tab (owner only). gitdash then reads `.gitdash.yml` at the target commit and executes the steps in Docker containers (workspace mounted at `/workspace`); steps run in order and the first failure stops the run. Run logs are kept under `<data>/pipelines/{owner}/{repo}/`.

**Triggers** (automatic triggers are gated by `on:` in `.gitdash.yml`; without `on:` only `push` applies):

| Trigger | Event | Notes |
| --- | --- | --- |
| branch / tag push | `push` | pushes to `refs/heads/*` or `refs/tags/*`; a PR merge also pushes to the target branch |
| Pull Request | `pull_request` | PR opened/reopened, plus pushes to the source branch (synchronize, same-repo PRs only) |
| Schedule | `schedule` | cron list under `schedule:` (min hour dom month dow) from the default branch; scanned every 30s |
| External dispatch | `workflow_dispatch` | `POST .../pipeline/dispatch`, callable from external systems with a PAT and `inputs` |
| Manual | `manual` | UI "Run now" / `POST .../pipeline/runs` with an optional `ref` (branch/tag) or `sha`; never gated by `on` |

Example trigger config:

```yaml
on: [push, pull_request, schedule, workflow_dispatch]
schedule:
  - "0 2 * * *"   # every day at 02:00 UTC
```

External dispatch (requires `on: [..., workflow_dispatch]`): `POST /api/users/{owner}/repos/{name}/pipeline/dispatch` with `{ref?|sha?, inputs?}`; `inputs` are injected as `INPUT_<KEY>` env vars (lower precedence than repo-level/DSL env).

Repository-level environment variables can be configured under **Settings → Pipeline environment variables** (owner only). They are injected into the container / host environment of every run; on a key collision, the `env` block in `.gitdash.yml` takes precedence. Values are stored in plaintext in the database and are readable/writable by the repo owner only.

Custom YAML DSL (supported subset):

```yaml
image: alpine:3.19   # optional: image for every step (omit to run directly on the host, requires GITDASH_PIPELINE_EXEC=host)
timeout: 10m         # optional: per-step timeout (default 10m, max 1h)
on: [push, pull_request]  # optional: automatic trigger whitelist (default: push only)
schedule:            # optional: cron list (requires "schedule" in on)
  - "0 2 * * *"
env:                 # optional: KEY=VALUE list injected into containers
  - CGO_ENABLED=0
runs-on: [docker]    # optional: dispatch to a remote runner matching these labels
steps:               # required: 1..20 steps
  - name: build
    run: go build ./...
  - name: test
    run: |
      go test ./...
      go vet ./...
```

`when:` conditions can use `branch`, `tag`, `ref`, `event`, `sha` and `repo`.

Pipeline API:

| Method | Path | Description |
| --- | --- | --- |
| GET/PUT | `/api/users/{owner}/repos/{name}/pipeline` | Get / set enabled (PUT: owner only) |
| GET/POST | `/api/users/{owner}/repos/{name}/pipeline/runs` | List runs / manual trigger (`{ref?|sha?, inputs?}`) |
| POST | `/api/users/{owner}/repos/{name}/pipeline/runs/{id}/rerun` | Re-run an existing run (reuses its sha/ref/event) |
| POST | `/api/users/{owner}/repos/{name}/pipeline/dispatch` | External dispatch (requires `on: workflow_dispatch`; supports `inputs`) |
| GET | `/api/users/{owner}/repos/{name}/pipeline/runs/{id}` | Run detail incl. log |
| POST | `/api/users/{owner}/repos/{name}/pipeline/runs/{id}/cancel` | Cancel a remote-runner run |
| GET | `/api/users/{owner}/repos/{name}/env` | List repo-level pipeline environment variables |
| PUT | `/api/users/{owner}/repos/{name}/env` | Create / overwrite an environment variable (`{key, value}`) |
| DELETE | `/api/users/{owner}/repos/{name}/env/{key}` | Delete an environment variable |

## Runners (Self-hosted CI Agents)

Without `runs-on`, pipelines run on the server's local Docker (builtin, zero setup); set `GITDASH_PIPELINE_EXEC=host` to also allow pipelines that omit `image` to run directly on the server host via `sh` (no container sandbox — opt-in). To execute pipelines on other machines, deploy **agents** (`gitdash-runner`):

1. Requires Redis: start the server with `GITDASH_QUEUE=redis` (the runner hub uses Redis for dispatch, heartbeats and cross-instance routing).
2. Issue a one-time registration token (valid 10 minutes): users issue a personal-scope token in **Profile → Runners**; org owners issue org-scope tokens via the API; site admins can issue global tokens from the admin panel (`POST /api/admin/runners/registration-token`).
3. On the agent machine:

```bash
gitdash-runner register -server http://gitdash.example:8080 \
  -name build-01 -labels docker,go1.22 -token <TOKEN>
gitdash-runner run   # config in ~/.gitdash-runner/config.json
```

4. In `.gitdash.yml` set `runs-on: [docker]` (labels must be a subset of the agent's). gitdash picks the online agent with matching labels and the lowest load, streams a `git archive` snapshot of the pushed commit over the agent's connection (no git credentials are ever sent), and streams logs back. If no agent matches, the run fails immediately; if an agent goes offline mid-run, its run is marked `failed (runner went offline)` (no silent re-execution). Remote runs can be cancelled from the run detail view.

**Reverse mode** (gitdash behind NAT, public runner): the runner listens and the server dials out — for servers without a public address the runner can reach.

```bash
gitdash-runner register -server http://gitdash.internal:8080 \
  -name build-pub-01 -labels docker -token <TOKEN> \
  -reverse -url ws://runner.example.com:8443
gitdash-runner serve   # binds the url port; -listen to override, -tls-cert/-tls-key for TLS
```

The server dials with `Authorization: Bearer {name}:{sha256(secret)}` (it stores only the hash); a Redis leader lock keeps a single instance dialing each reverse runner. Use `wss://` (TLS) on public networks. See [docs/runners.md](docs/runners.md).

Security model: agents execute arbitrary repo code on their host — treat agent hosts as trusted CI machines; the same sandbox as builtin (no network by default, resource caps, docker.sock mounts rejected) is applied on the agent side. Registration is limited: a personal runner only ever receives that user's repos, an org runner only that org's repos.

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
