# gitdash

[![CI](https://github.com/holihur/gitdash/actions/workflows/ci.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/ci.yml)
[![Release](https://github.com/holihur/gitdash/actions/workflows/release.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/release.yml)
[![codecov](https://codecov.io/gh/holihur/gitdash/graphs/badge.svg?branch=main)](https://codecov.io/gh/holihur/gitdash)

[English](README.md) | [简体中文](README.zh-CN.md)

A minimal self-hosted Git service MVP (like a mini Gitea):

- **User system**: register / login (bcrypt + session token, valid for 7 days); repos and SSH keys belong to users; profile email with uniqueness enforced (partially, empty allowed)
- **Organizations**: create orgs, manage members (owner / member roles), host repos under an org namespace, and follow orgs from a public organization profile page
- **Issues & labels**: per-repo issues with labels & milestones, edit / delete, keyword + state search, and pin-to-top; activity pushes to watchers' inboxes
- **Projects (kanban)**: per-repo kanban projects with columns, swimlanes and cards (issue-linked or text notes), drag & drop in the web UI — see the [Projects docs](https://holihur.github.io/gitdash/projects/kanban/)
- **Pull requests**: fork-based pull requests with squash merge and reviewer flow
- **Stars & forks**: star repos and fork them with one click
- **Repo mirroring & import**: import from a remote URL and push-mirror to GitHub/GitLab-like remotes
- **Connected accounts & batch import**: link GitHub, GitLab, Gitea/Forgejo or Bitbucket accounts (OAuth) and import selected repositories in bulk from the import dialog
- **Webhooks**: per-repo outbound webhooks with per-event subscriptions (push / issues / pull requests / comments / branches & tags / releases / pipeline / fork / star / watch) and HMAC signature delivery, dispatched asynchronously through the job queue with backoff retries; plus an **incoming webhook** token that lets external systems create issues
- **GPG keys**: upload GPG public keys to verify commit signatures
- **OAuth login**: GitHub OAuth, Google login and generic OIDC login (configurable in the admin panel)
- **OAuth 2.0 provider**: gitdash can act as an OAuth 2.0 authorization server — register third-party apps, run the authorization-code flow, and issue `repo`/`inbox`/`keys` access tokens (managed in **OAuth Apps**) — see [OAuth 2.0 provider](#oauth-20-provider-applications)
- **CLI (`gitdash-cli`)**: a `gh`/`glab`-style command-line client (repo / issue / PR / copilot) that logs in with a PAT or the OAuth 2.0 device flow — see [CLI](#cli-gitdash-cli)
- **Language stats**: an async job analyzes each repository's code composition after every push (or web commit) to the default branch — the repo list shows the primary language and the Code tab shows the top 5 languages with percentages; per-file-extension detection (Go, C/C++, C#, Java, Python, Rust, TypeScript, ...) skips vendored/generated files and docs, admins can turn the feature off in **Admin → Sign-in & API access**, and customize per-language colors in **Admin → Language colors**
- **Admin panel**: admin users, settings (OAuth providers), password management, user/repo/org bans and an IP/CIDR blacklist
- **User feedback**: admins can enable a floating feedback button (bottom-right); submitted messages are filed as issues in a configured repository via a GitHub/Gitea-compatible API (Admin panel → User feedback)
- **Onboarding & docs**: a getting-started checklist on the home page, plus a login-page/header entry to an independently deployed multilingual [Hugo documentation site](docs/) (English primary, `GITDASH_DOCS_URL` or Admin → Documentation site)
- **Explore**: discover public repos; repo visibility (public / private) toggle in repo settings; filter by tag and free-text search
- **Repo settings**: owner-managed default branch (drives the browsed/HEAD branch), issue tracker on/off toggle, visibility and template flags; one-click **`git gc`** (repository maintenance) to pack loose objects and reclaim disk space
- **Repo tags (topics)**: owner-managed labels per repository (up to 20), shown on repo pages and used to filter/search Explore
- **Code browsing & web editing**: browse repos by branch / directory, view file contents, commit history and blame on the web; create / edit / delete files and folders, revert a commit (creates an inverse commit) from the Commits tab, and compare any two branches / tags / commits from the Code tab
- **Private package registry**: publish & install packages for npm, composer (PHP), pypi (Python), rubygems (Ruby), Go modules, cargo (Rust), Maven (Java) and Docker/OCI images under user/org namespaces, authenticated with a PAT (Basic auth) — see the [packages docs](https://holihur.github.io/gitdash/packages/publish-install/) and the runnable [examples/packages](examples/packages/)
- **Watching & inbox**: watch / unwatch repos; repo issue / PR activity (opened / closed / reopened / merged) is pushed to your personal inbox (unread badge + read / delete management)
- **CI pipeline (MVP)**: per-repo pipeline toggle in the web UI; on push, steps defined in `.gitdash.yml` or `.gitdash/*.yml` (custom YAML DSL) run inside Docker containers with logs stored per run; multiple independent pipeline files per repo are each evaluated and triggered on their own `on:` rules; jobs can be processed in-process (default) or via a Redis-backed asynq queue
- **BYOK copilot**: chat with a standalone agent runtime (the `agent` binary built from the `deps/agent` submodule) inside a checkout of the repo; it can read, edit and run commands, and gitdash commits + pushes its changes to a `copilot/session-<id>` branch after every turn; sessions linked to an issue auto-open a pull request (`Closes #N`) once the agent pushes, from the web UI or `gitdash-cli copilot fix` — see [docs/copilot.md](docs/copilot.md)
- **User avatars**: upload / remove a profile picture (PNG/JPEG/GIF/WebP, max 2MB); shown in the header, user page and profile, with an initials fallback
- **Profile repo**: a public `<username>/<username>` repo is created when an account is first created (register / admin / OAuth), initialized with a README; creating an organization likewise initializes a public `<org>/<org>` repo (`GITDASH_PROFILE_REPO=0` disables both)
- **Structured logging & tracing**: `log/slog`-based logs with levels + text/JSON format, optional rotating file output (`GITDASH_LOG_FILE`), and OpenTelemetry tracing via OTLP (`OTEL_EXPORTER_OTLP_ENDPOINT`)
- **Pipeline visualization**: the Pipeline tab renders the `.gitdash.yml` step DAG (parallel groups included)
- **Mermaid in markdown**: ` ```mermaid ` code blocks render as diagrams (lazily loaded)
- **Self-hosted runners**: deploy `gitdash-runner` agents that connect out to the server; `.gitdash.yml` can target them via `runs-on` labels (user/org scoping, workspace snapshot streaming, log streaming, cancel, offline detection) — see the [runner docs](https://holihur.github.io/gitdash/ci/runners/)
- **Git SSH service**: built-in SSH server (default `:2222`), public keys bound to users, supports `git clone` / `push` / `pull`
- **SSH key management**: add / remove public keys via the web UI (CRUD); a public key acts as the user's credential
- **Self-update**: `gitdash update` for manual updates; optional background auto-update (**off by default**)
- **Backup & restore**: `gitdash backup` / `gitdash restore` (consistent SQLite snapshot + repos + webhook spool + SSH host key), archive verification (`restore --dry-run`), retention (`--keep`), and optional scheduled background backups (`GITDASH_BACKUP_DIR`) — see [Backup & Restore](#backup--restore)
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

Need the command-line client (`gitdash-cli`) instead of the server? Append `cli`:

```bash
curl -fsSL https://raw.githubusercontent.com/holihur/gitdash/main/install.sh | bash -s -- cli
gitdash-cli login
# Open your browser, or use --method pat
```

Other components: `runner` (self-hosted CI runner) and `agent` (copilot runtime).

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
| `GITDASH_BACKUP_DIR` | empty (off) | Enable scheduled background backups in `serve` mode; output directory for the archives |
| `GITDASH_BACKUP_INTERVAL` | `24h` | Auto-backup interval (minimum 1m) |
| `GITDASH_BACKUP_KEEP` | `14` | Number of newest auto-backups to retain |
| `GITDASH_QUEUE` | `memory` | Pipeline job queue: `memory` (in-process goroutines) or `redis`/`asynq` (durable Redis queue) |
| `GITDASH_REDIS_ADDR` | `127.0.0.1:6379` | Redis address for the asynq queue |
| `GITDASH_REDIS_PASSWORD` / `GITDASH_REDIS_DB` | empty / `0` | Redis auth / database index |
| `GITDASH_QUEUE_CONCURRENCY` | `4` | Worker concurrency for the asynq queue |
| `GITDASH_CODE_SEARCH` | `bleve` | Code search backend: `bleve` (**default**, embedded full-text index under `$GITDASH_DATA/code-index`, rebuilt asynchronously and **incrementally** after pushes; identifier-aware (camelCase/underscore/symbol split) + CJK + smart-case; only each repo's default branch is indexed; **eventually consistent**—while a repo is being indexed a search returns no rows for it and `indexing:true`), `grep` (explicit live `git grep`, no index), or `remote` (delegate to a dedicated index worker via `GITDASH_SEARCH_URL`) |
| `GITDASH_SEARCH_URL` | empty | Base URL of a remote code-index service (required by `GITDASH_CODE_SEARCH=remote`), e.g. `http://127.0.0.1:8090` |
| `GITDASH_SEARCH_TOKEN` | empty | Shared bearer token for the internal `/internal/codesearch` endpoint (set on both the worker and the API node) |
| `GITDASH_ROLE=codeindex` | off | Run a dedicated index worker: consume the `gitdash:codeindex` asynq queue, own the Bleve index, and serve the internal search endpoint. Requires `GITDASH_QUEUE=redis` and shared `GITDASH_DATA`/DB with the API node |
| `GITDASH_SEARCH_LISTEN` | `127.0.0.1:8090` | Internal search endpoint listen address (index worker only) |
| `GITDASH_CODE_INDEX_CONCURRENCY` | `1` | Index rebuild worker concurrency (index worker only) |
| `GITDASH_CODE_INDEX_CONSUME` | `1` | Set to `0` so this node only produces `gitdash:codeindex` tasks (redis mode; the index worker consumes them) |
| `GITDASH_PROFILE_REPO` | `1` | Auto-create a public `<name>/<name>` repo when an account or organization is first created (`0` disables) |
| `GITDASH_COPILOT_AGENT_BIN` | `agent` next to gitdash / in PATH | Path to the agent runtime for copilot sessions (see the [copilot docs](https://holihur.github.io/gitdash/copilot/sessions/)) |
| `GITDASH_COPILOT_AGENT_URL` | empty | Use an already-running agent (`http://host:port`) instead of spawning one per session |
| `GITDASH_LLM_ALLOW_HOSTS` | empty | Comma-separated `host` or `host:port` allowed to bypass the LLM SSRF guard (private gateway / local Ollama); scoped to BYOK/copilot only |
| `GITDASH_LOG_LEVEL` | `info` | Log level: `debug` / `info` / `warn` / `error` |
| `GITDASH_LOG_FORMAT` | `text` | Log format: `text` or `json` |
| `GITDASH_LOG_FILE` | empty | Enable rotating file logging (path); empty logs to stderr only |
| `GITDASH_LOG_MAX_SIZE_MB` | `100` | Rotate a log file once it reaches this size |
| `GITDASH_LOG_MAX_BACKUPS` | `7` | Number of rotated files to keep |
| `GITDASH_LOG_MAX_AGE_DAYS` | `28` | Maximum age of rotated files |
| `GITDASH_LOG_COMPRESS` | `true` | gzip rotated log files |
| `GITDASH_SLOW_SQL_MS` | `200` | Log a `slow sql` warning (with SQL + duration) when a query exceeds this many ms; `0` disables |
| `GITDASH_SLOW_API_MS` | `1000` | Log a `slow api` warning and bump `gitdash_http_slow_requests_total` when a request exceeds this many ms |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | Enable OpenTelemetry trace export (OTLP/HTTP, e.g. `http://localhost:4318`) |
| `GITDASH_ADMIN_PASSWORD` | empty (off) | Set to create an admin on **first boot** and enable the `/admin` panel |
| `GITDASH_ADMIN_USER` | `admin` | Admin username (used with the above) |
| `GITDASH_DISABLE_REGISTRATION` | off | Set to `1` to close public sign-up (invite the users you need instead) |
| `GITDASH_SMTP_HOST` | empty (off) | SMTP host; enables email notifications / address verification |
| `GITDASH_SMTP_PORT` | `587` | SMTP port |
| `GITDASH_SMTP_USER` / `GITDASH_SMTP_PASS` | empty | SMTP username / password |
| `GITDASH_SMTP_FROM` | SMTP user | From address |
| `GITDASH_EMAIL_PUSH` | off | Set to `1` to also send email notifications for `push` events |
| `GITDASH_MAIL_REPLY_DOMAIN` | empty (off) | Domain used for reply-by-email `Reply-To` / `Message-ID` (see the [email replies docs](https://holihur.github.io/gitdash/pulls/email-replies/)) |
| `GITDASH_MAIL_SECRET` | `GITDASH_SECRET_KEY` | HMAC secret for reply-by-email tokens |
| `GITDASH_MAIL_INBOUND_SECRET` | `GITDASH_MAIL_SECRET` | Shared secret required by `POST /api/mail/inbound` |
| `GITDASH_TRUSTED_PROXIES` | empty (loopback only) | Comma-separated proxy IP/CIDR allowed to set `X-Forwarded-For`. The proxy **must strip/overwrite** the incoming `X-Forwarded-For` (not append client headers), otherwise a client can spoof the left-most IP and bypass PAT IP allow-lists / login rate limits |
| `GITDASH_SECURE_COOKIES` | off | Set to `1` behind a TLS-terminating proxy so session/admin cookies get `Secure` |
| `GITDASH_TLS_CERT` / `GITDASH_TLS_KEY` | empty | Built-in HTTPS certificate/key paths |
| `GITDASH_ACME_DOMAINS` | empty | Comma-separated domains; obtain certificates automatically via ACME (see `GITDASH_ACME_EMAIL`) |
| `GITDASH_PIPELINE_VOLUMES_DIR` | empty (denied) | Host directory CI may mount; unset denies all host volume mounts |
| `GITDASH_SSH_KNOWN_HOSTS` | empty (accept-new) | Path to a `known_hosts` file; when set, import/push-mirror SSH uses `StrictHostKeyChecking=yes` (pins host keys, blocks first-connect MITM) |
| `GITDASH_OAUTH_TOKEN_TTL` | `2160h` (90 days) | Lifetime of OAuth / device-flow access tokens; `0` disables expiry (not recommended) |
| `GITDASH_METRICS_TOKEN` | empty | When set, `/metrics` requires `Authorization: Bearer <token>` |

> With deb/rpm packages the service reads these optional variables from `EnvironmentFile=-/etc/gitdash/gitdash.env` (the package post-install script creates a commented sample, mode 0600). Add `GITDASH_ADMIN_PASSWORD` etc. there, then `sudo systemctl restart gitdash`.

Admin panel: set `GITDASH_ADMIN_PASSWORD` (optionally `GITDASH_ADMIN_USER`); it is created on the first boot when no admin exists. Log in at `http://<host>:8080/admin` to manage users, global runners, OAuth/OIDC login settings, user/repo/org bans and an IP/CIDR blacklist (blacklisted addresses are rejected for both HTTP and SSH).

Runner (self-hosted CI agent) support requires Redis (`GITDASH_QUEUE=redis`); WS endpoint `/api/runner/ws`, registration `POST /api/runner/register`, management `GET/DELETE /api/runners`.

**Changing listen addresses**:

```bash
GITDASH_HTTP_ADDR=:9090 GITDASH_SSH_ADDR=:2322 gitdash serve
```

- For systemd: put variables in `/etc/gitdash/gitdash.env` (created by the package post-install, or add an `EnvironmentFile=` via `systemctl edit gitdash`), then `systemctl daemon-reload && systemctl restart gitdash`
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

The bundled `docker-compose.yml` runs a **production-ish** stack: **PostgreSQL** for the
database and **Redis** for the task queue / self-hosted runners (no SQLite, no in-process
memory queue).

```bash
docker compose up -d --build
# Web http://localhost:8080, Git SSH localhost:2222
```

What the compose file sets for the `gitdash` service:

```yaml
GITDASH_DB: postgres://gitdash:gitdash@postgres:5432/gitdash?sslmode=disable
GITDASH_QUEUE: redis
GITDASH_REDIS_ADDR: redis:6379
```

- `postgres` and `redis` are separate services with healthchecks; `gitdash` waits for both (`depends_on: condition: service_healthy`).
- Persistent volumes: `gitdash-data` (`/data`: bare repos, webhook spool, SSH host key, package blobs, copilot workspaces), `gitdash-pg` (PostgreSQL data), `gitdash-redis` (AOF). **Change the default `POSTGRES_PASSWORD` before exposing this to anyone.**
- The database schema is auto-migrated on startup. Backups: only `GITDASH_DATA` files are archived automatically — dump PostgreSQL with `pg_dump` (e.g. `docker compose exec postgres pg_dump -U gitdash gitdash > backup.sql`).
- Listen ports / auto-update can be adjusted via `environment` in `docker-compose.yml`.
- You can also build the image directly: `docker build -t gitdash .`, then run it against your own PostgreSQL + Redis via the `GITDASH_*` variables above.

> To run SQLite + in-process queue instead (single-node, no external deps), drop the `postgres`/`redis` services and unset `GITDASH_DB` / `GITDASH_QUEUE`.

## Backup & Restore

### Built-in CLI (recommended)

```bash
# Write a consistent backup (SQLite VACUUM INTO snapshot + repos + webhook spool + SSH host key)
gitdash backup -d ./backups -k 14     # -d output dir, -k keep newest N (omit -k to disable pruning)
gitdash backup -o /tmp/snap.tar.gz    # explicit output file

# List existing backups (newest first)
gitdash backup -d ./backups --list

# Verify an archive without touching data (gzip/tar integrity + path safety)
gitdash restore ./backups/gitdash-backup-*.tar.gz --dry-run

# Restore (refuses a non-empty data dir unless --force; rejects path traversal)
gitdash restore ./backups/gitdash-backup-*.tar.gz --force
```

### Scheduled (automatic) backups

Set `GITDASH_BACKUP_DIR` to enable a background backup loop in `serve` mode:

```bash
GITDASH_BACKUP_DIR=/var/backups/gitdash \
GITDASH_BACKUP_INTERVAL=24h \
GITDASH_BACKUP_KEEP=14 \
gitdash serve
```

The first backup runs at startup, then every `GITDASH_BACKUP_INTERVAL` (default `24h`, minimum `1m`); the newest `GITDASH_BACKUP_KEEP` (default `14`) archives are retained. Backups run online without stopping the service. With `GITDASH_DB=postgres://...` only the files under `GITDASH_DATA` are archived — run `pg_dump` separately.

### Shell script

```bash
# Online backup (consistent SQLite snapshot + repo archives, keeps the latest 14)
bash scripts/backup.sh ./data ./backups
# KEEP=30 bash scripts/backup.sh   # keep more copies
```

Restore manually: stop the service, unpack the backup into the data directory (`tar -xzf gitdash-backup-*.tar.gz -C <data dir>`), then start again.
If `sqlite3` is not available on the host, the script falls back to a straight file copy (in WAL mode, stop the server first for consistency); inside Docker you can also run the same script via `docker compose exec gitdash bash`.

## Automated Testing

Static checks (golangci-lint, config in `backend/.golangci.yml`, runs automatically in CI):

```bash
cd backend
golangci-lint run     # install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
                      # must be built with Go >= 1.26 (older binaries fail with "Go language version used to build golangci-lint is lower than the targeted Go version"); CI pins v2.13.2
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
| DELETE | `/api/me` | Delete own account (password/MFA confirmation, wipes all data) |
| GET | `/api/health` `/api/health/live` `/api/version` | Readiness (pings DB, 503 when down) / liveness / version |

Business (requires `Authorization: Bearer <token>`, token from register/login):

| Method | Path | Description |
| --- | --- | --- |
| GET/POST | `/api/repos` | List / create your own repos |
| GET/DELETE | `/api/repos/{name}` | Details / delete |
| GET | `/api/repos/{name}/branches` | Branch list |
| GET | `/api/repos/{name}/tree?ref=&path=` | Browse directory |
| GET | `/api/repos/{name}/blob?ref=&path=` | File content |
| GET | `/api/repos/{name}/commits?ref=` | Commit history |
| POST | `/api/users/{owner}/repos/{name}/commits` | Create a commit (batch file changes) |
| POST | `/api/users/{owner}/repos/{name}/commits/{sha}/revert` | Revert a commit (creates an inverse commit on a branch) |
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

## OAuth 2.0 provider (applications)

gitdash can act as an **OAuth 2.0 authorization server** (authorization-code flow), so third-party
apps can act on behalf of a user with the same scopes as personal access tokens
(`repo`, `inbox`, `keys`). Register and manage apps in the web UI under **OAuth Apps**,
or via the API below.

### Flow

1. Register an app (`POST /api/applications`) — you get a `client_id` and a `client_secret` (the secret is shown only once; reset it if lost).
2. Send the user to `GET /login/oauth/authorize?client_id=…&redirect_uri=…&scope=repo&state=…&response_type=code`. `redirect_uri` must exactly match the registered callback URL.
3. After the user approves, gitdash redirects back to `redirect_uri?code=…&state=…`.
4. Exchange the code: `POST /login/oauth/access_token` (form-encoded) with `grant_type=authorization_code`, `client_id`, `client_secret`, `code`, `redirect_uri`. The response is `{"access_token":"…","token_type":"bearer","scope":"repo"}`.
5. Call protected endpoints with `Authorization: Bearer <access_token>`.

Authorization codes are single-use and expire after 10 minutes. Access tokens are opaque,
sha256-hashed at rest, and reuse the existing PAT validation / scope / expiry machinery;
revoke individual grants (or delete an app to revoke all its tokens) from **OAuth Apps → Authorized Apps**.

### Endpoints

| Method | Path | Description |
| --- | --- | --- |
| GET/POST | `/api/applications` | List / register OAuth apps |
| DELETE | `/api/applications/{id}` | Delete an app (revokes all its tokens) |
| POST | `/api/applications/{id}/reset_secret` | Rotate `client_secret` |
| GET | `/api/applications/authorizations` | List issued tokens (grants) |
| DELETE | `/api/applications/authorizations/{id}` | Revoke one grant |
| GET/POST | `/login/oauth/authorize` | Consent page / approve-or-deny |
| POST | `/login/oauth/access_token` | Exchange `code` (or `device_code`) for `access_token` |
| POST | `/login/oauth/device/code` | Start device flow (RFC 8628), returns `device_code` + `user_code` |
| GET/POST | `/login/oauth/device` | Device-flow verification/consent page |

### Device flow (for CLIs)

`gitdash-cli` uses the OAuth 2.0 **device flow** with a built-in first-party client
(`client_id=gitdash-cli`), so users never register an app or paste a secret:

1. `POST /login/oauth/device/code` with `client_id=gitdash-cli` → `device_code`, `user_code`, `verification_uri_complete`.
2. The user opens `verification_uri_complete` and approves.
3. The client polls `POST /login/oauth/access_token` with `grant_type=urn:ietf:params:oauth:grant-type:device_code` until it receives an `access_token` (`authorization_pending` while waiting).

## CLI (gitdash-cli)

A small command-line client (`gh`/`glab`-style) for repos, issues and pull requests.
It authenticates with a **PAT** or the **OAuth 2.0 device flow**.

### Install

The release archives ship a `gitdash-cli` binary alongside `gitdash`.

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/holihur/gitdash/main/install.sh | bash -s -- cli

# Windows (PowerShell)
& $([ScriptBlock]::Create((irm https://raw.githubusercontent.com/holihur/gitdash/main/install.ps1))) cli
```

From source:

```bash
task cli                       # → /tmp/gitdash-cli
# or:
cd backend && go build -o gitdash-cli ./cmd/gitdash-cli
```

### Login

```bash
gitdash-cli login                            # interactive: browser (device flow) or PAT
gitdash-cli login --host http://localhost:8080 --method pat
gitdash-cli me
gitdash-cli logout
```

Credentials are stored in `~/.config/gitdash/config.json` (mode 0600). `--host` / `--token`
flags and the `GITDASH_HOST` / `GITDASH_TOKEN` environment variables override the file. Add
`--json` for raw output.

### Commands

| Command | Description |
| --- | --- |
| `gitdash-cli login` | Authenticate (device flow or PAT) |
| `gitdash-cli logout` | Remove stored credentials |
| `gitdash-cli me` | Show the authenticated user |
| `gitdash-cli repo list` | List your repositories |
| `gitdash-cli repo create [--private=false] [--description D] <name>` | Create a repository |
| `gitdash-cli repo delete <owner/repo> [--yes]` | Delete a repository (permanent; prompts unless `--yes`) |
| `gitdash-cli issue list <owner/repo>` | List issues |
| `gitdash-cli issue create <owner/repo> --title T [--body B]` | Create an issue |
| `gitdash-cli issue delete <owner/repo> <n>` | Delete an issue |
| `gitdash-cli issue fix <owner/repo> <n> [--byok NAME] [--detach]` | Alias of `copilot fix` |
| `gitdash-cli copilot list <owner/repo>` | List AI copilot sessions |
| `gitdash-cli copilot create <owner/repo> [--byok NAME] [--issue N] [--prompt P]` | Create a copilot session |
| `gitdash-cli copilot run <owner/repo> <id> [--text M]` | Drive a session and stream the agent's work |
| `gitdash-cli copilot fix <owner/repo> <n> [--byok NAME] [--instructions P] [--detach]` | Have the agent fix an issue; auto-opens a PR |
| `gitdash-cli pr list <owner/repo>` | List pull requests |
| `gitdash-cli pr create <owner/repo> --title T --head H --base B [--body B]` | Open a pull request |
| `gitdash-cli project list <owner/repo>` | List kanban projects in a repository |
| `gitdash-cli project create <owner/repo> --name N [--description D]` | Create a kanban project |
| `gitdash-cli project delete <owner/repo> <project-id>` | Delete a project and its columns/swimlanes/cards |
| `gitdash-cli skill show` | Print the embedded Agent Skill |
| `gitdash-cli skill install` | Install the Agent Skill for Claude Code / opencode / pi |

Run `gitdash-cli <command> --help` (or `gitdash-cli <command> <subcommand> --help`) for
usage; help works without being logged in.

The copilot commands are the headless equivalent of the web UI's **Fix with Copilot**
flow: `copilot fix` creates a session linked to an issue, runs the agent against a
checkout of the repo, and — once the agent pushes — gitdash opens a pull request whose
body closes the issue.

```bash
gitdash-cli copilot fix alice/demo 14                 # fix issue #14, auto-open a PR
gitdash-cli copilot fix alice/demo 14 --detach        # only create the session
gitdash-cli copilot list alice/demo                   # sessions + linked issue/PR
gitdash-cli copilot run alice/demo 3 --text "also update the changelog"
```

### Agent skill (for Claude Code / opencode / pi)

`gitdash-cli` ships an [Agent Skill](https://agentskills.io/specification) (`SKILL.md`) that
teaches AI coding agents how to drive the CLI. Install it into the standard skill
directories:

```bash
gitdash-cli skill install              # ~/.claude/skills + ~/.agents/skills
gitdash-cli skill install --target claude
gitdash-cli skill install --target agents
gitdash-cli skill install --project    # ./.claude/skills + ./.agents/skills
gitdash-cli skill show                 # print the skill
```

Claude Code loads `~/.claude/skills/gitdash-cli/SKILL.md`; opencode auto-loads both
`~/.claude/skills` and `~/.agents/skills`; pi loads `~/.agents/skills`. So a single
`gitdash-cli skill install` makes the CLI usable by all three.

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

The server dials with `Authorization: Bearer {name}:{sha256(secret)}` (it stores only the hash); a Redis leader lock keeps a single instance dialing each reverse runner. Use `wss://` (TLS) on public networks. See the [runner docs](https://holihur.github.io/gitdash/ci/runners/).

Security model: agents execute arbitrary repo code on their host — treat agent hosts as trusted CI machines; the same sandbox as builtin (no network by default, resource caps, docker.sock mounts rejected) is applied on the agent side. Registration is limited: a personal runner only ever receives that user's repos, an org runner only that org's repos.

## Upgrade Notes

Since v0.2 the data model includes a user system; for older versions (≤ v0.1), the `repos` / `ssh_keys` tables in the `data` directory are automatically reset at startup (bare repos on disk are preserved but must be re-registered / migrated under a user).

## CI / Release

- **CI** (`.github/workflows/ci.yml`): push/PR runs the fast path — Go lint/build/vet/unit tests + `backend/tests/` integration tests + E2E smoke + black-box API tests, and frontend tsc/vite build; heavy jobs (race detector + benchmarks, full Playwright UI suite, PostgreSQL black-box run, Docker image build, `govulncheck`, `pnpm audit`) run nightly (18:00 UTC) or via **workflow_dispatch**.
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
