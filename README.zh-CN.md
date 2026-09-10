# gitdash

[![CI](https://github.com/holihur/gitdash/actions/workflows/ci.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/ci.yml)
[![Release](https://github.com/holihur/gitdash/actions/workflows/release.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/release.yml)
[![codecov](https://codecov.io/gh/holihur/gitdash/graphs/badge.svg?branch=main)](https://codecov.io/gh/holihur/gitdash)

English | 简体中文

一个最小的自托管 Git 服务 MVP（类似迷你 Gitea）：

- **用户系统**：注册 / 登录（bcrypt + 会话 token，7 天有效），仓库与 SSH Key 归属用户；资料邮箱支持设置，数据库层唯一（空值除外）
- **组织**：创建组织、管理成员（owner / member 角色）、可将仓库建到组织命名空间下
- **Issue 与标签**：仓库 issue 支持标签、里程碑；动态推送给关注者
- **项目看板**：仓库级看板项目，支持列、泳道与卡片（关联 issue 或文本便签），网页端拖拽流转 —— 详见 [docs/projects.zh-CN.md](docs/projects.zh-CN.md)
- **Pull Request**：基于 fork 的 PR，支持 squash 合并
- **Star 与 Fork**：一键 star / fork 仓库
- **镜像与导入**：从远端 URL 导入仓库，push 镜像到 GitHub/GitLab 等远端
- **Webhook**：仓库级 webhook，HMAC 签名推送
- **GPG Key**：上传 GPG 公钥验证提交签名
- **OAuth 登录**：GitHub OAuth 与通用 OIDC 登录（管理面板可配置）
- **管理面板**：管理员账号、设置（OAuth 提供方）、密码管理
- **发现（Explore）**：浏览公开仓库；仓库设置页可切换公开 / 私有
- **代码浏览**：网页端按分支 / 目录浏览仓库、查看文件内容、提交历史与 blame
- **私有包仓库**：为 npm、composer（PHP）、pypi（Python）、rubygems（Ruby）、Go modules、cargo（Rust）、Maven（Java）提供私有包发布与安装，按用户/组织命名空间隔离，PAT（Basic 认证）鉴权 —— 详见 [docs/packages.zh-CN.md](docs/packages.zh-CN.md)
- **关注与收件箱**：watch / unwatch 仓库；仓库的 issue / PR 动态（打开 / 关闭 / 重开 / 合并）推送到个人收件箱（未读角标 + 已读 / 删除管理）
- **CI 流水线 (MVP)**：仓库设置页可开启/关闭流水线；push 时按 `.gitdash.yml`（自定义 YAML DSL）定义的步骤在 Docker 容器中执行，逐步骤记录日志；任务默认进程内执行，也可走 Redis（asynq）持久化队列
- **自托管 Runner**：部署 `gitdash-runner` agent 主动连接服务端执行流水线；`.gitdash.yml` 用 `runs-on` 标签指定目标 agent（个人/组织 scope、工作区快照流、日志回传、取消、掉线检测），详见 [docs/runners.zh-CN.md](docs/runners.zh-CN.md)
- **Git SSH 服务**：内置 SSH server（默认 `:2222`），公钥绑定用户，支持 `git clone` / `push` / `pull`
- **SSH Key 管理**：网页端增删公钥（CRUD），公钥即用户凭证
- **自更新**：`gitdash update` 手动更新；可选后台自动更新（**默认关闭**）
- **前端**：React + Vite + Tailwind + shadcn/ui 风格组件
- **后端**：Go 标准库 HTTP + `golang.org/x/crypto/ssh` + SQLite（modernc，纯 Go 无 CGO）
- **单二进制发布**：GoReleaser 发布时前端已 embed 进二进制，下载即用
- **自动化测试**：`backend/tests/` 按功能覆盖（auth / repos / keys / browse / ssh git / updater / store / webui）；根目录 `tests/` 另有一套与后端完全隔离的黑盒 API 测试（pytest + requests + uv）

## 一键安装

```bash
curl -fsSL https://raw.githubusercontent.com/holihur/gitdash/main/install.sh | bash
gitdash serve
# 打开 http://localhost:8080（SSH :2222）
```

安装脚本支持环境变量：`GITDASH_VERSION`（指定版本）、`GITDASH_INSTALL_DIR`（安装目录）。

### Windows

```powershell
irm https://raw.githubusercontent.com/holihur/gitdash/main/install.ps1 | iex
gitdash serve
```

### macOS / Homebrew（macOS 与 Linux 均可）

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
nix-env -iA nixpkgs.gitdash   # 经 holihur/nur-packages（NUR）
```

### Arch Linux（AUR）

```bash
yay -S gitdash-bin
```

### Debian / Ubuntu（apt）

```bash
# 从 Releases 下载 .deb 后安装
curl -fsSLO https://github.com/holihur/gitdash/releases/latest/download/gitdash_linux_amd64.deb
sudo apt install ./gitdash_linux_amd64.deb
sudo systemctl enable --now gitdash
```

### RHEL / CentOS / Fedora（yum）

```bash
curl -fsSLO https://github.com/holihur/gitdash/releases/latest/download/gitdash_linux_amd64.rpm
sudo yum install ./gitdash_linux_amd64.rpm   # 或 dnf install
sudo systemctl enable --now gitdash
```

### Alpine（apk）

```bash
curl -fsSLO https://github.com/holihur/gitdash/releases/latest/download/gitdash_linux_amd64.apk
sudo apk add --allow-untrusted ./gitdash_linux_amd64.apk
```

> deb/rpm/apk 包内含 systemd 服务文件（Alpine 无 systemd，仅装二进制）并依赖系统 `git`。

> 以上包管理器渠道在 release 时自动发布；需要先建好对应仓库（homebrew-tap / scoop-bucket / nur-packages）并配置 `TAP_GITHUB_TOKEN`、`AUR_SSH_KEY` secrets，未配置时 GoReleaser 会自动跳过对应发布。

也可以直接到 [Releases](https://github.com/holihur/gitdash/releases) 下载对应平台的压缩包（前端已内嵌），或参考 [packaging/gitdash.service](packaging/gitdash.service) 用 systemd 部署、[packaging/com.gitdash.server.plist](packaging/com.gitdash.server.plist) 用 launchd 部署（macOS）、[packaging/gitdash.windows.md](packaging/gitdash.windows.md) 部署为 Windows 服务。

## 使用流程

1. 打开网页 → 注册账号（如 `alice`）
2. **SSH Keys** → 粘贴公钥（如 `~/.ssh/id_ed25519.pub`）
3. **仓库** → 新建仓库（如 `demo`，实际地址为 `alice/demo`）
4. 克隆并推送：

```bash
git clone ssh://git@<host>:2222/alice/demo.git
cd demo
echo "# demo" >> README.md
git add README.md && git commit -m "initial commit"
git push origin main
```

5. 回到网页即可浏览代码与提交历史。clone 地址也支持省略 owner 的单段形式（`ssh://git@<host>:2222/demo.git` 会解析为当前登录用户自己的仓库）。

## 快速开始（开发）

### 1. 启动后端

```bash
cd backend
go run .
# HTTP  :8080   Git SSH :2222   数据目录 ./data
```

环境变量（均可选）：

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `GITDASH_HTTP_ADDR` | `:8080` | Web / API 监听地址 |
| `GITDASH_SSH_ADDR` | `:2222` | Git SSH 监听地址 |
| `GITDASH_DATA` | `./data` | 数据目录（仓库、host key；SQLite 文件默认放这里） |
| `GITDASH_DB` | `./data/gitdash.db` | 数据库：SQLite 文件路径，或 `postgres://` 连接串切换 PostgreSQL（自动迁移 schema；不迁移已有 SQLite 数据） |
| `GITDASH_STATIC` | 自动探测 | 前端静态文件目录（开发模式覆盖 embed 资源） |
| `GITDASH_AUTO_UPDATE` | 关闭 | **自动更新默认关闭**，设为 `1`/`true`/`yes`/`on` 开启 |
| `GITDASH_AUTO_UPDATE_INTERVAL` | `24h` | 自动更新检查间隔（最小 1h） |
| `GITDASH_UPDATE_REPO` | `holihur/gitdash` | 更新源仓库（fork / 测试用） |
| `GITDASH_QUEUE` | `memory` | 流水线任务队列：`memory`（进程内 goroutine）或 `redis`/`asynq`（Redis 持久化队列） |
| `GITDASH_REDIS_ADDR` | `127.0.0.1:6379` | asynq 队列使用的 Redis 地址 |
| `GITDASH_REDIS_PASSWORD` / `GITDASH_REDIS_DB` | 空 / `0` | Redis 密码 / 数据库编号 |
| `GITDASH_QUEUE_CONCURRENCY` | `4` | asynq 队列工人并发数 |

Runner（自托管 CI agent）功能需要 Redis（`GITDASH_QUEUE=redis`）；WS 端点 `/api/runner/ws`，注册 `POST /api/runner/register`，管理 `GET/DELETE /api/runners`。

**修改监听地址**：

```bash
GITDASH_HTTP_ADDR=:9090 GITDASH_SSH_ADDR=:2322 gitdash serve
```

- systemd 方式：修改 `packaging/gitdash.service` 中对应的 `Environment=` 行后 `systemctl daemon-reload && systemctl restart gitdash`
- 注意：网页上显示的 clone 地址端口固定为 2222，SSH 端口改动后 clone 命令需手动调整

### 2. 启动前端（开发）

```bash
cd frontend
pnpm install
pnpm run dev
# 打开 http://localhost:5173，/api 已代理到 :8080
```

### 3. 生产模式（单二进制，内嵌前端）

```bash
bash scripts/embed-frontend.sh   # 构建前端并拷贝到 backend/internal/webui/dist
cd backend && go build -o gitdash .
./gitdash serve
./gitdash version
```

未执行 embed 时也能编译，此时走磁盘目录（`GITDASH_STATIC` / `./static` / `../frontend/dist`）。

## 更新 / 自动更新

```bash
# 手动更新到最新 release（校验 SHA256 后原子替换当前二进制）
gitdash update

# 自动更新：默认关闭，需显式开启；建议配合 systemd Restart=always
GITDASH_AUTO_UPDATE=1 gitdash serve
```

开启后进程会周期性检查 GitHub Releases，发现新版本即下载（校验 checksums.txt）、替换自身二进制并退出，由 systemd（`Restart=always`）拉起新版本。`dev` 编译版本不参与自动更新，但可手动 `gitdash update`。

## Docker 部署

```bash
docker compose up -d --build
# Web http://localhost:8080 ，Git SSH localhost:2222
```

- 数据（SQLite + bare 仓库 + SSH host key）持久化在 Docker volume `gitdash-data`（容器内 `/data`）。
- 监听端口 / 自动更新通过 `docker-compose.yml` 的 `environment` 调整。
- 也可直接构建镜像：`docker build -t gitdash .`，挂载 `-v gitdash-data:/data -p 8080:8080 -p 2222:2222`。

## 备份与恢复

```bash
# 在线备份（SQLite 一致性快照 + 仓库打包，保留最近 14 份）
bash scripts/backup.sh ./data ./backups
# KEEP=30 bash scripts/backup.sh   # 保留更多份
```

恢复：停掉服务，把备份解包回数据目录（`tar -xzf gitdash-backup-*.tar.gz -C <数据目录>`），再启动即可。
若宿主机没有 `sqlite3`，脚本会用直接拷贝兜底（WAL 模式建议先停服保证一致性）；Docker 内亦可 `docker compose exec gitdash bash` 执行同款脚本。

## 自动化测试

静态检查（golangci-lint，配置见 `backend/.golangci.yml`，CI 中自动执行）：

```bash
cd backend
golangci-lint run     # 安装：go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

后端集成测试（Go）：

```bash
cd backend
go test ./...          # 单元测试 + backend/tests/ 集成测试
bash scripts/e2e.sh    # 全链路冒烟（真实二进制：注册登录 -> ssh clone/push -> 浏览 -> 多用户隔离）
```

黑盒 API 测试（pytest + requests，依赖用 uv 管理，与后端源码/构建完全隔离）：

```bash
# 1) 单独构建被测二进制
(cd backend && go build -o /tmp/gitdash-server .)
# 2) 运行：夹具在独立临时数据目录 + 随机端口上启动全新实例，结束后销毁
(cd tests && GITDASH_BIN=/tmp/gitdash-server uv run pytest -v)
# 或指向一个已运行的实例：GITDASH_API_URL=http://127.0.0.1:8080 uv run pytest
```

黑盒 E2E UI 测试（TypeScript + Playwright + headless Chrome，独立在 `tests/ui/`，一个业务一个文件）：

```bash
task test:ui                              # 构建带内嵌前端的二进制并跑 UI 测试
(cd tests/ui && GITDASH_BIN=/tmp/gitdash-server-ui npx playwright test --grep @happy)
# 或指向一个已运行的实例：GITDASH_UI_URL=http://127.0.0.1:8080 npx playwright test
```

- `backend/tests/` 按功能划分：`auth`（注册/登录/会话）、`repos`（仓库 CRUD 与隔离）、`sshkeys`（公钥 CRUD 与绑定）、`browse`（tree/blob/commits）、`sshgit`（真实 SSH clone/push 与权限拒绝）、`updater`（版本比较/校验/解包）、`store`（schema 迁移）、`webui`（静态托管/SPA fallback/路径穿越防护）。CI 中全部自动执行。
- `tests/`（仓库根目录）为**独立、隔离**的纯黑盒接口自动化测试：不 import 后端代码、不执行 go 构建；用例随机命名、互不共享状态，覆盖 auth / repos / issues / orgs / pulls / webhooks / visibility / blame / ssh keys 的 happy path 与 bad path（400/401/404/409…）。详见 `tests/README.md`。
- `tests/ui/`（仓库根目录）为**独立、隔离**的纯黑盒 E2E UI 测试套件（Playwright + headless Chrome）：注册 → 建仓 → 代码浏览 → issue → star/fork/watch → 收件箱 → 协作者/Webhook/Release → PR 合并；UI 发现的问题记录在 `tests/ui/bug.md`。详见 `tests/ui/README.md`。

### 代码覆盖率

三个维度的覆盖率都在 CI 中上报 [Codecov](https://codecov.io/gh/holihur/gitdash)（见顶部徽章）：

| 维度 | 方式 | 本地运行 | 当前值（2026-09） |
| --- | --- | --- | --- |
| 单元测试（Go） | `go test -coverprofile` | `task coverage:go` | 9.2% statements |
| 黑盒 API（pytest） | `go build -cover` + `GOCOVERDIR` | `task coverage:blackbox` | 与下合并 |
| 黑盒 UI（Playwright） | 同 `GOCOVERDIR`，优雅停机落盘 | 与下合并 | 与下合并 |
| **黑盒合并（API+UI）** | `go tool covdata` | `task coverage:blackbox` | **58.7% statements** |

黑盒覆盖率原理：被测二进制用 `go build -cover` 构建，所有黑盒套件运行时通过 `GOCOVERDIR` 收集执行数据（服务端收到 SIGTERM 优雅退出以保证计数落盘），最后用 `go tool covdata percent / textfmt` 汇总。

## API 一览

认证类（公开）：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/auth/register` | 注册，返回会话 token |
| POST | `/api/auth/login` | 登录，返回会话 token |
| POST | `/api/auth/logout` | 登出（作废当前 token） |
| GET | `/api/me` | 当前用户 |
| GET | `/api/health` `/api/version` | 健康检查 / 版本 |

业务类（需 `Authorization: Bearer <token>`，token 来自注册/登录）：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/POST | `/api/repos` | 列出 / 创建自己的仓库 |
| GET/DELETE | `/api/repos/{name}` | 详情 / 删除 |
| GET | `/api/repos/{name}/branches` | 分支列表 |
| GET | `/api/repos/{name}/tree?ref=&path=` | 浏览目录 |
| GET | `/api/repos/{name}/blob?ref=&path=` | 文件内容 |
| GET | `/api/repos/{name}/commits?ref=` | 提交历史 |
| GET/POST | `/api/keys` | 列出 / 添加 SSH 公钥（绑定当前用户） |
| DELETE | `/api/keys/{id}` | 删除自己的公钥 |

仓库社交 / 收件箱（watch → 订阅仓库动态到收件箱）：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| PUT/DELETE | `/api/users/{owner}/repos/{name}/watch` | 关注 / 取消关注（返回 watch 数与状态） |
| GET | `/api/watched` | 我关注过的仓库列表 |
| GET | `/api/inbox` | 收件箱通知（最新在前） |
| GET | `/api/inbox/unread` | 未读数 |
| POST | `/api/inbox/read` `/api/inbox/read/{id}` | 全部 / 单条标为已读 |
| DELETE | `/api/inbox/{id}` | 删除单条通知 |

> MVP 注意：仓库为 owner-only（仅属主可读写）；HTTP 层请自行加 TLS（或置于反代之后）。

## CI 流水线 (MVP)

在仓库的 **流水线** 页开启（仅 owner）。此后每次 push 分支，gitdash 会读取该提交上的 `.gitdash.yml` 并在 Docker 容器中逐步执行（仓库工作区挂载在 `/workspace`，任一步骤失败即终止），也支持手动触发。运行日志保存在 `<data>/pipelines/{owner}/{repo}/`。

自定义 YAML DSL（受支持子集）：

```yaml
image: alpine:3.19   # 可选：每步运行所用镜像（省略时直接在宿主 sh 执行，需 GITDASH_PIPELINE_EXEC=host）
timeout: 10m         # 可选：单步超时（默认 10m，上限 1h）
env:                 # 可选：注入容器的环境变量 KEY=VALUE
  - CGO_ENABLED=0
runs-on: [docker]    # 可选：派发给匹配标签的远程 runner
steps:               # 必填：1..20 个步骤
  - name: build
    run: go build ./...
  - name: test
    run: |
      go test ./...
      go vet ./...
```

流水线 API：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/PUT | `/api/users/{owner}/repos/{name}/pipeline` | 查询 / 设置开关（PUT 仅 owner） |
| GET/POST | `/api/users/{owner}/repos/{name}/pipeline/runs` | 运行列表 / 手动触发（`{ref?}`） |
| GET | `/api/users/{owner}/repos/{name}/pipeline/runs/{id}` | 运行详情（含日志） |
| POST | `/api/users/{owner}/repos/{name}/pipeline/runs/{id}/cancel` | 取消远程 runner 执行的运行 |

## Runner（自托管 CI Agent）

不写 `runs-on` 时，流水线在服务端本地 Docker 中执行（内置，零配置）；设置 `GITDASH_PIPELINE_EXEC=host` 后还允许省略 `image` 的流水线直接在服务端宿主 `sh` 执行（无容器沙箱，需显式开启）。要在其他机器上执行流水线，部署 **agent**（`gitdash-runner`）：

1. 需要 Redis：服务端以 `GITDASH_QUEUE=redis` 启动（runner 调度、心跳与跨实例路由依赖 Redis）。
2. 签发一次性注册 token（10 分钟有效）：用户在 **个人设置 → Runner** 签发个人 scope；组织 owner 可为组织签发；站点管理员可在管理端签发全局 token（`POST /api/admin/runners/registration-token`）。
3. 在 agent 机器上：

```bash
gitdash-runner register -server http://gitdash.example:8080 \
  -name build-01 -labels docker,go1.22 -token <TOKEN>
gitdash-runner run   # 配置写入 ~/.gitdash-runner/config.json
```

4. 在 `.gitdash.yml` 中设置 `runs-on: [docker]`（标签须为 agent 标签的子集）。gitdash 选择标签匹配且负载最低的在线 agent，经其连接推送该提交的 `git archive` 快照（不下发任何 git 凭证），日志实时回传。无匹配 agent 时运行立即失败；agent 执行中掉线，运行记为 `failed (runner went offline)`（不静默重跑）。远程运行可在运行详情中取消。

安全模型：agent 在其宿主上执行仓库任意代码 —— 请将 agent 主机视为受信 CI 机器；agent 侧应用与内置执行一致的沙箱（默认禁外网、资源限制、拒绝 docker.sock 挂载）。注册即隔离：个人 runner 只会收到该用户仓库的任务，组织 runner 只会收到该组织仓库的任务。

## 升级说明

v0.2 起数据模型加入用户系统，旧版（≤ v0.1）`data` 目录中的 `repos` / `ssh_keys` 表会在启动时自动重置（磁盘上的 bare 仓库保留但需重新登记 / 迁移到用户名下）。

## CI / 发版

- **CI**（`.github/workflows/ci.yml`）：Go build/vet/test + `backend/tests/` 集成测试 + E2E 冒烟，前端 tsc + vite 构建。
- **发版**（`.github/workflows/release.yml`）：推送 tag 即自动发布：

```bash
git tag v0.2.0
git push origin v0.2.0
```

GoReleaser 会先构建前端并 embed，然后产出 linux / darwin / windows × amd64 / arm64 的压缩包与校验和，发布到 GitHub Releases。

本地验证发布配置：

```bash
goreleaser check
goreleaser release --snapshot --clean --skip=publish
```
