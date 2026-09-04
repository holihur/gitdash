# gitdash

[![CI](https://github.com/holihur/gitdash/actions/workflows/ci.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/ci.yml)
[![Release](https://github.com/holihur/gitdash/actions/workflows/release.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/release.yml)

一个最小的自托管 Git 服务 MVP（类似迷你 Gitea）：

- **用户系统**：注册 / 登录（bcrypt + 会话 token，7 天有效），仓库与 SSH Key 归属用户
- **Git SSH 服务**：内置 SSH server（默认 `:2222`），公钥绑定用户，支持 `git clone` / `push` / `pull`
- **代码浏览**：网页端按分支 / 目录浏览仓库、查看文件内容、查看提交历史
- **SSH Key 管理**：网页端增删公钥（CRUD），公钥即用户凭证
- **自更新**：`gitdash update` 手动更新；可选后台自动更新（**默认关闭**）
- **前端**：React + Vite + Tailwind + shadcn/ui 风格组件
- **后端**：Go 标准库 HTTP + `golang.org/x/crypto/ssh` + SQLite（modernc，纯 Go 无 CGO）
- **单二进制发布**：GoReleaser 发布时前端已 embed 进二进制，下载即用
- **自动化测试**：`backend/tests/` 按功能覆盖（auth / repos / keys / browse / ssh git / updater / store / webui）

## 一键安装

```bash
curl -fsSL https://raw.githubusercontent.com/holihur/gitdash/main/install.sh | bash
gitdash serve
# 打开 http://localhost:8080（SSH :2222）
```

安装脚本支持环境变量：`GITDASH_VERSION`（指定版本）、`GITDASH_INSTALL_DIR`（安装目录）。

也可以直接到 [Releases](https://github.com/holihur/gitdash/releases) 下载对应平台的压缩包（前端已内嵌），或参考 [packaging/gitdash.service](packaging/gitdash.service) 用 systemd 部署。

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
| `GITDASH_DATA` | `./data` | 数据目录（SQLite、仓库、host key） |
| `GITDASH_STATIC` | 自动探测 | 前端静态文件目录（开发模式覆盖 embed 资源） |
| `GITDASH_AUTO_UPDATE` | 关闭 | **自动更新默认关闭**，设为 `1`/`true`/`yes`/`on` 开启 |
| `GITDASH_AUTO_UPDATE_INTERVAL` | `24h` | 自动更新检查间隔（最小 1h） |
| `GITDASH_UPDATE_REPO` | `holihur/gitdash` | 更新源仓库（fork / 测试用） |

**修改监听地址**：

```bash
GITDASH_HTTP_ADDR=:9090 GITDASH_SSH_ADDR=:2322 gitdash serve
```

- systemd 方式：修改 `packaging/gitdash.service` 中对应的 `Environment=` 行后 `systemctl daemon-reload && systemctl restart gitdash`
- 注意：网页上显示的 clone 地址端口固定为 2222，SSH 端口改动后 clone 命令需手动调整

### 2. 启动前端（开发）

```bash
cd frontend
npm install
npm run dev
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

## 自动化测试

```bash
cd backend
go test ./...          # 单元测试 + backend/tests/ 集成测试
bash scripts/e2e.sh    # 全链路冒烟（真实二进制：注册登录 -> ssh clone/push -> 浏览 -> 多用户隔离）
```

`backend/tests/` 按功能划分：`auth`（注册/登录/会话）、`repos`（仓库 CRUD 与隔离）、`sshkeys`（公钥 CRUD 与绑定）、`browse`（tree/blob/commits）、`sshgit`（真实 SSH clone/push 与权限拒绝）、`updater`（版本比较/校验/解包）、`store`（schema 迁移）、`webui`（静态托管/SPA fallback/路径穿越防护）。CI 中全部自动执行。

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

> MVP 注意：仓库为 owner-only（仅属主可读写）；HTTP 层请自行加 TLS（或置于反代之后）。

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
