# gitdash

[![CI](https://github.com/holihur/gitdash/actions/workflows/ci.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/ci.yml)
[![Release](https://github.com/holihur/gitdash/actions/workflows/release.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/release.yml)
[![codecov](https://codecov.io/gh/holihur/gitdash/graphs/badge.svg?branch=main)](https://codecov.io/gh/holihur/gitdash)

English | 简体中文

一个最小的自托管 Git 服务 MVP（类似迷你 Gitea）：

- **用户系统**：注册 / 登录（bcrypt + 会话 token，7 天有效），仓库与 SSH Key 归属用户；资料邮箱支持设置，数据库层唯一（空值除外）
- **组织**：创建组织、管理成员（owner / member 角色）、可将仓库建到组织命名空间下，并提供公开的组织主页与关注（follow）
- **Issue 与标签**：仓库 issue 支持标签、里程碑，编辑 / 删除、关键词与状态搜索、置顶；动态推送给关注者
- **项目看板**：仓库级看板项目，支持列、泳道与卡片（关联 issue 或文本便签），网页端拖拽流转 —— 详见 [docs/projects.zh-CN.md](docs/projects.zh-CN.md)
- **Pull Request**：基于 fork 的 PR，支持 squash 合并
- **Star 与 Fork**：一键 star / fork 仓库
- **镜像与导入**：从远端 URL 导入仓库，push 镜像到 GitHub/GitLab 等远端
- **Webhook**：仓库级出站 webhook，支持按事件类型订阅（push / issue / PR / 评论 / 分支标签 / Release / 流水线 / fork / star / watch），HMAC 签名推送，经任务队列异步派发并按退避重试；另提供**入站 webhook** token，外部系统可凭其创建 issue
- **GPG Key**：上传 GPG 公钥验证提交签名
- **OAuth 登录**：GitHub OAuth、Google 登录与通用 OIDC 登录（管理面板可配置）
- **OAuth 2.0 提供方**：gitdash 可作为 OAuth 2.0 授权服务器——注册第三方应用、跑授权码流程、签发 `repo`/`inbox`/`keys` 访问令牌（在「OAuth Apps」管理），见 [OAuth 2.0 提供方](#oauth-20-提供方applications)
- **CLI（`gitdash-cli`）**：`gh`/`glab` 风格命令行客户端（仓库 / issue / PR / copilot），支持 PAT 或 OAuth 2.0 设备流登录，见 [CLI](#cli-gitdash-cli)
- **管理面板**：管理员账号、设置（OAuth 提供方）、密码管理、用户/仓库/组织封禁与 IP/CIDR 黑名单
- **发现（Explore）**：浏览公开仓库；支持按标签筛选与关键词搜索；仓库设置页可切换公开 / 私有
- **仓库设置**：owner 可设置默认分支（决定浏览/HEAD 分支）、开启或关闭 issue 功能，并管理可见性与模版标记；支持一键 **`git gc`**（仓库维护）打包松散对象、回收磁盘空间
- **仓库标签（topics）**：owner 可为仓库管理标签（最多 20 个），在仓库页展示、用于 Explore 筛选与搜索
- **代码浏览与网页编辑**：网页端按分支 / 目录浏览仓库、查看文件内容、提交历史与 blame；可新建 / 编辑 / 删除文件与目录，在提交记录页撤销某次提交（生成反向提交），并在代码页比较任意两个分支 / 标签 / 提交的差异
- **私有包仓库**：为 npm、composer（PHP）、pypi（Python）、rubygems（Ruby）、Go modules、cargo（Rust）、Maven（Java）以及 Docker/OCI 镜像提供私有发布与安装，按用户/组织命名空间隔离，PAT（Basic 认证）鉴权 —— 详见 [docs/packages.zh-CN.md](docs/packages.zh-CN.md) 及可直接运行的 [examples/packages](examples/packages/)
- **关注与收件箱**：watch / unwatch 仓库；仓库的 issue / PR 动态（打开 / 关闭 / 重开 / 合并）推送到个人收件箱（未读角标 + 已读 / 删除管理）
- **CI 流水线 (MVP)**：仓库设置页可开启/关闭流水线；push 时按 `.gitdash.yml` 或 `.gitdash/*.yml`（自定义 YAML DSL）定义的步骤在 Docker 容器中执行，逐步骤记录日志；每个仓库可放多个独立流水线文件，按各自的 `on:` 规则分别触发；任务默认进程内执行，也可走 Redis（asynq）持久化队列
- **BYOK Copilot**：与运行在仓库检出副本中的独立 agent 运行时（由 `deps/agent` 子模块构建的 `agent` 二进制）对话；它能读、改、执行命令，每轮结束后 gitdash 会把改动提交并推送到 `copilot/session-<id>` 分支；关联 issue 的会话在 agent 推送后自动开 PR（正文 `Closes #N`），网页端与 `gitdash-cli copilot fix` 均可触发——详见 [docs/copilot.zh-CN.md](docs/copilot.zh-CN.md)
- **用户头像**：上传 / 移除头像（PNG/JPEG/GIF/WebP，最大 2MB）；显示在顶栏、用户主页与个人资料页，未设置时回退为首字母
- **同名仓库**：用户首次创建（注册 / 管理端 / OAuth）时自动创建公开的 `<用户名>/<用户名>` 仓库并初始化 README；创建组织时同样自动初始化公开的 `<组织名>/<组织名>` 仓库（`GITDASH_PROFILE_REPO=0` 关闭以上行为）
- **结构化日志与链路追踪**：基于 `log/slog` 的日志（级别 + text/JSON 格式），支持滚动文件输出（`GITDASH_LOG_FILE`）；通过 OTLP 导出 OpenTelemetry trace（`OTEL_EXPORTER_OTLP_ENDPOINT`）
- **流水线可视化**：流水线页渲染 `.gitdash.yml` 的步骤 DAG（含并行组）
- **Markdown 支持 Mermaid**：` ```mermaid ` 代码块渲染为图表（按需懒加载）
- **自托管 Runner**：部署 `gitdash-runner` agent 主动连接服务端执行流水线；`.gitdash.yml` 用 `runs-on` 标签指定目标 agent（个人/组织 scope、工作区快照流、日志回传、取消、掉线检测），详见 [docs/runners.zh-CN.md](docs/runners.zh-CN.md)
- **Git SSH 服务**：内置 SSH server（默认 `:2222`），公钥绑定用户，支持 `git clone` / `push` / `pull`
- **SSH Key 管理**：网页端增删公钥（CRUD），公钥即用户凭证
- **自更新**：`gitdash update` 手动更新；可选后台自动更新（**默认关闭**）
- **备份与恢复**：`gitdash backup` / `gitdash restore`（SQLite 一致性快照 + 仓库 + webhook spool + SSH host key），归档校验（`restore --dry-run`）、保留份数（`--keep`），以及可选定时后台备份（`GITDASH_BACKUP_DIR`）——见 [备份与恢复](#备份与恢复)
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

如果只需要命令行客户端（`gitdash-cli`），在命令后追加 `cli`：

```bash
curl -fsSL https://raw.githubusercontent.com/holihur/gitdash/main/install.sh | bash -s -- cli
gitdash-cli login
# 浏览器完成授权；也可加 --method pat 使用 PAT
```

其他组件：`runner`（自托管 CI runner）、`agent`（copilot 运行时）。

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
| `GITDASH_BACKUP_DIR` | 空（关闭） | 在 `serve` 模式启用定时后台备份；归档输出目录 |
| `GITDASH_BACKUP_INTERVAL` | `24h` | 自动备份间隔（最小 1m） |
| `GITDASH_BACKUP_KEEP` | `14` | 自动备份保留的最新份数 |
| `GITDASH_QUEUE` | `memory` | 流水线任务队列：`memory`（进程内 goroutine）或 `redis`/`asynq`（Redis 持久化队列） |
| `GITDASH_REDIS_ADDR` | `127.0.0.1:6379` | asynq 队列使用的 Redis 地址 |
| `GITDASH_REDIS_PASSWORD` / `GITDASH_REDIS_DB` | 空 / `0` | Redis 密码 / 数据库编号 |
| `GITDASH_QUEUE_CONCURRENCY` | `4` | asynq 队列工人并发数 |
| `GITDASH_PROFILE_REPO` | `1` | 用户 / 组织首次创建时自动创建公开的 `<名称>/<名称>` 仓库（`0` 关闭） |
| `GITDASH_COPILOT_AGENT_BIN` | gitdash 同目录的 `agent` / PATH | copilot 会话使用的 agent 运行时路径（见 [docs/copilot.zh-CN.md](docs/copilot.zh-CN.md)） |
| `GITDASH_COPILOT_AGENT_URL` | 空 | 使用已在运行的 agent（`http://host:port`），而不是每会话拉起一个 |
| `GITDASH_LLM_ALLOW_HOSTS` | 空 | 逗号分隔的 `host` 或 `host:port` 白名单，放行 LLM 端点的 SSRF 拦截（私有网关 / 本地 Ollama）；仅作用于 BYOK/copilot |
| `GITDASH_LOG_LEVEL` | `info` | 日志级别：`debug` / `info` / `warn` / `error` |
| `GITDASH_LOG_FORMAT` | `text` | 日志格式：`text` 或 `json` |
| `GITDASH_LOG_FILE` | 空 | 启用滚动文件日志（路径）；空则仅输出 stderr |
| `GITDASH_LOG_MAX_SIZE_MB` | `100` | 单个日志文件达到该大小后滚动 |
| `GITDASH_LOG_MAX_BACKUPS` | `7` | 保留的历史日志文件数 |
| `GITDASH_LOG_MAX_AGE_DAYS` | `28` | 历史日志文件最长保留天数 |
| `GITDASH_LOG_COMPRESS` | `true` | 压缩历史日志文件（gzip） |
| `GITDASH_SLOW_SQL_MS` | `200` | 查询超过该毫秒数时打 `slow sql` warn（含 SQL 与耗时）；`0` 关闭 |
| `GITDASH_SLOW_API_MS` | `1000` | 请求超过该毫秒数时打 `slow api` warn 并累加 `gitdash_http_slow_requests_total` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 空 | 启用 OpenTelemetry trace 导出（OTLP/HTTP，如 `http://localhost:4318`） |
| `GITDASH_ADMIN_PASSWORD` | 空（关闭） | 设置后在**首次启动**创建管理员并启用 `/admin` 管理面板 |
| `GITDASH_ADMIN_USER` | `admin` | 管理员用户名（配合上者） |
| `GITDASH_DISABLE_REGISTRATION` | 关闭 | 设为 `1` 关闭公开注册（改为邀请所需用户） |
| `GITDASH_SMTP_HOST` | 空（关闭） | SMTP 主机；设置后启用邮件通知/邮箱验证 |
| `GITDASH_SMTP_PORT` | `587` | SMTP 端口 |
| `GITDASH_SMTP_USER` / `GITDASH_SMTP_PASS` | 空 | SMTP 账号 / 密码 |
| `GITDASH_SMTP_FROM` | SMTP 用户名 | 发件人地址 |
| `GITDASH_EMAIL_PUSH` | 关闭 | 设为 `1` 时 push 事件也发送邮件通知 |
| `GITDASH_MAIL_REPLY_DOMAIN` | 空（关闭） | 邮件回复（reply-by-email）的 `Reply-To` / `Message-ID` 域名（见 `docs/email-replies.zh-CN.md`） |
| `GITDASH_MAIL_SECRET` | `GITDASH_SECRET_KEY` | 邮件回复 token 的 HMAC 密钥 |
| `GITDASH_MAIL_INBOUND_SECRET` | `GITDASH_MAIL_SECRET` | `POST /api/mail/inbound` 要求的共享密钥 |
| `GITDASH_TRUSTED_PROXIES` | 空（仅回环） | 信任 `X-Forwarded-For` 的反代 IP/CIDR 列表（逗号分隔）。反代**必须重写/剥离**外部传入的 `X-Forwarded-For`（而非追加客户端头部），否则客户端可伪造最左 IP，绕过 PAT IP 白名单 / 登录限流 |
| `GITDASH_SECURE_COOKIES` | 关闭 | 反代终止 TLS 时设为 `1`，让会话/管理 cookie 带 `Secure` |
| `GITDASH_TLS_CERT` / `GITDASH_TLS_KEY` | 空 | 内置 HTTPS 证书/私钥路径 |
| `GITDASH_ACME_DOMAINS` | 空 | 逗号分隔域名；设置后用 ACME 自动申请证书（另见 `GITDASH_ACME_EMAIL`） |
| `GITDASH_PIPELINE_VOLUMES_DIR` | 空（禁止） | CI 允许挂载的宿主目录；未设置则禁止流水线挂载宿主卷 |
| `GITDASH_SSH_KNOWN_HOSTS` | 空（accept-new） | `known_hosts` 文件路径；设置后导入/镜像的 SSH 使用 `StrictHostKeyChecking=yes`（固定 host key，防首次连接 MITM） |
| `GITDASH_OAUTH_TOKEN_TTL` | `2160h`（90 天） | OAuth / 设备流 access token 有效期；`0` 表示永不过期（不建议） |
| `GITDASH_METRICS_TOKEN` | 空 | 设置后 `/metrics` 需 `Authorization: Bearer <token>` |

> 用 deb/rpm 安装时，服务通过 `EnvironmentFile=-/etc/gitdash/gitdash.env` 读取上述可选变量（包安装脚本会生成带注释的示例文件，权限 0600）。把 `GITDASH_ADMIN_PASSWORD` 等写进去后执行 `sudo systemctl restart gitdash` 即生效。

管理面板：设置 `GITDASH_ADMIN_PASSWORD`（可选 `GITDASH_ADMIN_USER`）后在**无管理员**的首次启动时创建，访问 `http://<host>:8080/admin` 登录，可管理用户、全局 Runner、OAuth/OIDC 登录配置、用户/仓库/组织封禁以及 IP/CIDR 黑名单（命中黑名单的地址会被 HTTP 与 SSH 同时拒绝）。

Runner（自托管 CI agent）功能需要 Redis（`GITDASH_QUEUE=redis`）；WS 端点 `/api/runner/ws`，注册 `POST /api/runner/register`，管理 `GET/DELETE /api/runners`。

**修改监听地址**：

```bash
GITDASH_HTTP_ADDR=:9090 GITDASH_SSH_ADDR=:2322 gitdash serve
```

- systemd 方式：把变量写进 `/etc/gitdash/gitdash.env`（由包安装生成，或 `systemctl edit gitdash` 加 `EnvironmentFile=`），然后 `systemctl daemon-reload && systemctl restart gitdash`
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

### 内置命令（推荐）

```bash
# 生成一致性备份（SQLite VACUUM INTO 快照 + 仓库 + webhook spool + SSH host key）
gitdash backup -d ./backups -k 14     # -d 输出目录，-k 保留最新 N 份（不传 -k 则不清理）
gitdash backup -o /tmp/snap.tar.gz    # 指定输出文件

# 列出已有备份（新→旧）
gitdash backup -d ./backups --list

# 只读校验归档，不改动数据（gzip/tar 完整性 + 路径安全）
gitdash restore ./backups/gitdash-backup-*.tar.gz --dry-run

# 恢复（数据目录非空需 --force；拒绝路径穿越）
gitdash restore ./backups/gitdash-backup-*.tar.gz --force
```

### 定时（自动）备份

设置 `GITDASH_BACKUP_DIR` 即在 `serve` 模式启用后台备份：

```bash
GITDASH_BACKUP_DIR=/var/backups/gitdash \
GITDASH_BACKUP_INTERVAL=24h \
GITDASH_BACKUP_KEEP=14 \
gitdash serve
```

启动时先备份一次，之后每隔 `GITDASH_BACKUP_INTERVAL`（默认 `24h`，最小 `1m`）备份一次，保留最新 `GITDASH_BACKUP_KEEP`（默认 `14`）份。全程在线，无需停服。若使用 `GITDASH_DB=postgres://...`，归档只包含 `GITDASH_DATA` 下的文件，数据库需自行 `pg_dump`。

### Shell 脚本

```bash
# 在线备份（SQLite 一致性快照 + 仓库打包，保留最近 14 份）
bash scripts/backup.sh ./data ./backups
# KEEP=30 bash scripts/backup.sh   # 保留更多份
```

手动恢复：停掉服务，把备份解包回数据目录（`tar -xzf gitdash-backup-*.tar.gz -C <数据目录>`），再启动即可。
若宿主机没有 `sqlite3`，脚本会用直接拷贝兜底（WAL 模式建议先停服保证一致性）；Docker 内亦可 `docker compose exec gitdash bash` 执行同款脚本。

## 自动化测试

静态检查（golangci-lint，配置见 `backend/.golangci.yml`，CI 中自动执行）：

```bash
cd backend
golangci-lint run     # 安装：go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
                      # 需使用 Go >= 1.26 构建的版本（旧版会报 "Go language version used to build golangci-lint is lower than the targeted Go version"）；CI 固定 v2.13.2
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
| DELETE | `/api/me` | 注销账号（校验密码/MFA 后彻底删除全部数据） |
| GET | `/api/health` `/api/health/live` `/api/version` | 就绪检查（ping DB，数据库不可用返回 503）/ 存活检查 / 版本 |

业务类（需 `Authorization: Bearer <token>`，token 来自注册/登录）：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/POST | `/api/repos` | 列出 / 创建自己的仓库 |
| GET/DELETE | `/api/repos/{name}` | 详情 / 删除 |
| GET | `/api/repos/{name}/branches` | 分支列表 |
| GET | `/api/repos/{name}/tree?ref=&path=` | 浏览目录 |
| GET | `/api/repos/{name}/blob?ref=&path=` | 文件内容 |
| GET | `/api/repos/{name}/commits?ref=` | 提交历史 |
| POST | `/api/users/{owner}/repos/{name}/commits` | 创建提交（批量文件变更） |
| POST | `/api/users/{owner}/repos/{name}/commits/{sha}/revert` | 撤销提交（在指定分支生成反向提交） |
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

## OAuth 2.0 提供方（应用）

gitdash 可作为 **OAuth 2.0 授权服务器**（授权码流程），让第三方应用以用户身份、用与 PAT
相同的 scope（`repo`/`inbox`/`keys`）访问 API。在网页 **OAuth Apps** 页注册管理应用。

### 流程

1. `POST /api/applications` 注册应用，得到 `client_id` 与 `client_secret`（secret 只显示一次，丢失可重置）。
2. 浏览器跳转到 `GET /login/oauth/authorize?client_id=…&redirect_uri=…&scope=repo&state=…&response_type=code`；`redirect_uri` 必须与注册的回调地址完全一致。
3. 用户批准后回跳 `redirect_uri?code=…&state=…`。
4. `POST /login/oauth/access_token`（表单）带 `grant_type=authorization_code&client_id&client_secret&code&redirect_uri` 换取 `access_token`。
5. 之后以 `Authorization: Bearer <access_token>` 调用受保护接口。

授权码一次性、10 分钟过期；access token 不透明、只存 sha256，复用现有一套 PAT 校验/scope/过期机制；
可在 **OAuth Apps → Authorized Apps** 撤销单个授权（删除应用会撤销其全部 token）。

### 设备流（CLI 用，RFC 8628）

`gitdash-cli` 用内置第一方公开客户端（`client_id=gitdash-cli`）跑设备流，用户无需注册应用或粘贴密钥：

1. `POST /login/oauth/device/code`（`client_id=gitdash-cli`）→ `device_code`、`user_code`、`verification_uri_complete`。
2. 用户打开 `verification_uri_complete` 并批准。
3. 客户端轮询 `POST /login/oauth/access_token`（`grant_type=urn:ietf:params:oauth:grant-type:device_code`），等待期间返回 `authorization_pending`，批准后返回 `access_token`。

### 接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/POST | `/api/applications` | 列出 / 注册 OAuth 应用 |
| DELETE | `/api/applications/{id}` | 删除应用（级联撤销其 token） |
| POST | `/api/applications/{id}/reset_secret` | 重置 `client_secret` |
| GET | `/api/applications/authorizations` | 列出已签发的授权 |
| DELETE | `/api/applications/authorizations/{id}` | 撤销单个授权 |
| GET/POST | `/login/oauth/authorize` | 授权确认页 / 批准或拒绝 |
| POST | `/login/oauth/access_token` | 用 `code`（或 `device_code`）换 `access_token` |
| POST | `/login/oauth/device/code` | 启动设备流，返回 `device_code` + `user_code` |
| GET/POST | `/login/oauth/device` | 设备流验证/确认页 |

## CLI（gitdash-cli）

`gh`/`glab` 风格的命令行客户端，管理仓库、issue 与 PR，支持 **PAT** 或 **OAuth 2.0 设备流**登录。

发布归档会附带 `gitdash-cli`。一键安装：

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/holihur/gitdash/main/install.sh | bash -s -- cli

# Windows（PowerShell）
& $([ScriptBlock]::Create((irm https://raw.githubusercontent.com/holihur/gitdash/main/install.ps1))) cli
```

源码构建：`task cli`（或 `cd backend && go build -o gitdash-cli ./cmd/gitdash-cli`）。

```bash
gitdash-cli login                      # 交互式：浏览器设备流（默认）或 PAT
gitdash-cli login --host http://localhost:8080 --method pat
gitdash-cli me

# 常用命令
gitdash-cli repo list
gitdash-cli repo create --private demo
gitdash-cli repo delete alice/demo --yes   # 删除仓库（不加 --yes 会交互确认）
gitdash-cli issue list alice/demo
gitdash-cli issue create alice/demo --title "Bug" --body "..."
gitdash-cli issue delete alice/demo 1
gitdash-cli pr list alice/demo
gitdash-cli pr create alice/demo --title "Fix" --head feature --base main

# 看板项目（默认列 To Do / In Progress / Done）
gitdash-cli project create alice/demo --name "Roadmap"
gitdash-cli project list alice/demo
gitdash-cli project delete alice/demo 6

# AI copilot：让 agent 修 issue 并自动开 PR（等价于网页端「用 Copilot 修复」）
gitdash-cli copilot fix alice/demo 14                  # 修 issue #14，收尾自动开 PR
gitdash-cli copilot fix alice/demo 14 --detach         # 只建会话，稍后再跑
gitdash-cli copilot list alice/demo                    # 列出会话及关联 issue/PR
gitdash-cli copilot run alice/demo 3 --text "顺便更新 changelog"
```

`copilot fix` 会创建一个关联 issue 的会话，驱动 agent 在仓库检出中工作，推送后由 gitdash 自动开出一个正文 `Closes #N` 的 PR；多个 BYOK 密钥时用 `--byok <名称|id>` 指定。

凭据存于 `~/.config/gitdash/config.json`（0600）；`--host`/`--token` 与 `GITDASH_HOST`/`GITDASH_TOKEN` 可覆盖；`--json` 输出原始 JSON。

任意命令或子命令都支持 `--help`（无需登录即可查看）：

```bash
gitdash-cli project create --help          # 子命令用法
gitdash-cli repo --help                     # 命令总览
```

### Agent Skill（供 Claude Code / opencode / pi 使用）

`gitdash-cli` 内置一份 [Agent Skill](https://agentskills.io/specification)（`SKILL.md`），告诉 AI 编码代理如何用本 CLI 操作 gitdash：

```bash
gitdash-cli skill install              # 装到 ~/.claude/skills 与 ~/.agents/skills
gitdash-cli skill install --target claude
gitdash-cli skill install --target agents
gitdash-cli skill install --project    # 装到当前项目的 ./.claude/skills 与 ./.agents/skills
gitdash-cli skill show                 # 打印 skill 内容
```

Claude Code 读 `~/.claude/skills/gitdash-cli/SKILL.md`；opencode 自动加载 `~/.claude/skills` 与 `~/.agents/skills`；pi 读 `~/.agents/skills`。一次 `gitdash-cli skill install` 三者都能用。

## CI 流水线 (MVP)

在仓库的 **流水线** 页开启（仅 owner）。此后 gitdash 会读取目标提交上的 `.gitdash.yml` 并在 Docker 容器中逐步执行（仓库工作区挂载在 `/workspace`，任一步骤失败即终止）。运行日志保存在 `<data>/pipelines/{owner}/{repo}/`。

**触发方式**（由 `.gitdash.yml` 的 `on:` 控制自动触发；省略 `on` 时仅 `push`）：

| 触发 | 事件名 | 说明 |
| --- | --- | --- |
| 分支 / tag push | `push` | push 到 `refs/heads/*` 或 `refs/tags/*`；PR 合并到目标分支也会触发 |
| Pull Request | `pull_request` | PR opened/reopened，以及向源分支 push（synchronize，仅同仓库 PR） |
| 定时 | `schedule` | `schedule:` 列 cron（分 时 日 月 周），基于默认分支的 DSL；每 30s 扫描一次 |
| 外部 dispatch | `workflow_dispatch` | `POST .../pipeline/dispatch`，可用 PAT 从外部系统触发并传 `inputs` |
| 手动 | `manual` | UI「立即运行」/ `POST .../pipeline/runs`（可指定 `ref` 分支/tag 或 `sha`），不受 `on` 限制 |

`.gitdash.yml` 触发配置示例：

```yaml
on: [push, pull_request, schedule, workflow_dispatch]
schedule:
  - "0 2 * * *"   # 每天 02:00（UTC）
```

外部 dispatch（需 `on` 含 `workflow_dispatch`）：`POST /api/users/{owner}/repos/{name}/pipeline/dispatch`，body `{ref?|sha?, inputs?}`；`inputs` 注入为 `INPUT_<KEY>` 环境变量（优先级低于仓库级/DSL env）。

仓库 **设置 → 流水线环境变量** 可配置仓库级环境变量（仅 owner），它们会自动注入每次运行的容器 / host 执行环境；同名时 `.gitdash.yml` 里的 `env` 覆盖仓库变量。变量值以明文存储在数据库中，仅仓库 owner 可读写。

自定义 YAML DSL（受支持子集）：

```yaml
image: alpine:3.19   # 可选：每步运行所用镜像（省略时直接在宿主 sh 执行，需 GITDASH_PIPELINE_EXEC=host）
timeout: 10m         # 可选：单步超时（默认 10m，上限 1h）
on: [push, pull_request]  # 可选：自动触发事件白名单（默认仅 push）
schedule:            # 可选：cron 列表（需 on 含 schedule）
  - "0 2 * * *"
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

`when:` 条件步骤可用变量：`branch`、`tag`、`ref`、`event`、`sha`、`repo`。

流水线 API：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/PUT | `/api/users/{owner}/repos/{name}/pipeline` | 查询 / 设置开关（PUT 仅 owner） |
| GET/POST | `/api/users/{owner}/repos/{name}/pipeline/runs` | 运行列表 / 手动触发（`{ref?|sha?, inputs?}`） |
| POST | `/api/users/{owner}/repos/{name}/pipeline/runs/{id}/rerun` | 重跑一次既有运行（复用其 sha/ref/event） |
| POST | `/api/users/{owner}/repos/{name}/pipeline/dispatch` | 外部 dispatch 触发（需 `on: workflow_dispatch`，支持 `inputs`） |
| GET | `/api/users/{owner}/repos/{name}/pipeline/runs/{id}` | 运行详情（含日志） |
| POST | `/api/users/{owner}/repos/{name}/pipeline/runs/{id}/cancel` | 取消远程 runner 执行的运行 |
| GET | `/api/users/{owner}/repos/{name}/env` | 列出仓库级流水线环境变量 |
| PUT | `/api/users/{owner}/repos/{name}/env` | 新增 / 覆盖环境变量（`{key, value}`） |
| DELETE | `/api/users/{owner}/repos/{name}/env/{key}` | 删除环境变量 |

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

**反向连接模式**（gitdash 在内网、runner 在公网）：runner 监听端口，由 gitdash 服务端主动拨号，适用于服务端无公网地址、runner 无法回连的场景。

```bash
gitdash-runner register -server http://gitdash.internal:8080 \
  -name build-pub-01 -labels docker -token <TOKEN> \
  -reverse -url ws://runner.example.com:8443
gitdash-runner serve   # 监听 url 端口；-listen 覆盖，-tls-cert/-tls-key 直接启用 TLS
```

服务端以 `Authorization: Bearer {name}:{sha256(secret)}` 拨号认证（只存 hash）；多实例经 Redis 选主锁保证只有一个实例拨号。公网部署请用 `wss://`（TLS）或仅在受信网络使用 `ws://`。详见 [docs/runners.zh-CN.md](docs/runners.zh-CN.md)。

安全模型：agent 在其宿主上执行仓库任意代码 —— 请将 agent 主机视为受信 CI 机器；agent 侧应用与内置执行一致的沙箱（默认禁外网、资源限制、拒绝 docker.sock 挂载）。注册即隔离：个人 runner 只会收到该用户仓库的任务，组织 runner 只会收到该组织仓库的任务。

## 升级说明

v0.2 起数据模型加入用户系统，旧版（≤ v0.1）`data` 目录中的 `repos` / `ssh_keys` 表会在启动时自动重置（磁盘上的 bare 仓库保留但需重新登记 / 迁移到用户名下）。

## CI / 发版

- **CI**（`.github/workflows/ci.yml`）：push/PR 只跑快速路径——Go lint/build/vet/单测 + `backend/tests/` 集成测试 + E2E 冒烟 + 黑盒 API 测试，以及前端 tsc/vite 构建；重任务（race 检测 + 基准、Playwright 全量 UI、PostgreSQL 黑盒、Docker 镜像构建、`govulncheck`、`pnpm audit`）放在夜间（18:00 UTC）或手动 **workflow_dispatch** 触发。
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
