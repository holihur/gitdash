# gitdash

[![CI](https://github.com/holihur/gitdash/actions/workflows/ci.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/ci.yml)
[![Release](https://github.com/holihur/gitdash/actions/workflows/release.yml/badge.svg)](https://github.com/holihur/gitdash/actions/workflows/release.yml)

一个最小的自托管 Git 服务 MVP（类似迷你 Gitea）：

- **Git SSH 服务**：内置 SSH server（默认 `:2222`），支持 `git clone` / `push` / `pull` / 上传归档
- **代码浏览**：网页端按分支 / 目录浏览仓库、查看文件内容、查看提交历史
- **仓库管理**：网页端创建 / 删除仓库
- **SSH Key 管理**：网页端增删公钥（CRUD），公钥即凭证
- **前端**：React + Vite + Tailwind + shadcn/ui 风格组件
- **后端**：Go 标准库 HTTP + `golang.org/x/crypto/ssh` + SQLite（modernc，纯 Go 无 CGO），git 操作直接调用系统 `git`
- **单二进制发布**：GoReleaser 发布时前端已 embed 进二进制，下载即用

## 一键安装

```bash
curl -fsSL https://raw.githubusercontent.com/holihur/gitdash/main/install.sh | bash
gitdash serve
# 打开 http://localhost:8080（默认 token: dev，SSH :2222）
```

安装脚本支持环境变量：`GITDASH_VERSION`（指定版本）、`GITDASH_INSTALL_DIR`（安装目录）。

也可以直接到 [Releases](https://github.com/holihur/gitdash/releases) 下载对应平台的压缩包，解压即用（前端已内嵌，无需额外静态文件）。

systemd 部署参考 [packaging/gitdash.service](packaging/gitdash.service)。

## 快速开始

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
| `GITDASH_TOKEN` | `dev` | Web API Bearer Token |
| `GITDASH_STATIC` | 自动探测 | 前端静态文件目录（开发模式覆盖 embed 资源） |

**修改监听地址**：

```bash
GITDASH_HTTP_ADDR=:9090 GITDASH_SSH_ADDR=:2322 gitdash serve
# 或
GITDASH_HTTP_ADDR=127.0.0.1:8080 GITDASH_SSH_ADDR=127.0.0.1:2222 gitdash serve
```

- `GITDASH_HTTP_ADDR` / `GITDASH_SSH_ADDR` 均为 Go `net.Listen` 地址，支持 `host:port`、`:port`（监听所有网卡）
- systemd 方式：修改 `packaging/gitdash.service` 中对应的 `Environment=` 行后 `systemctl daemon-reload && systemctl restart gitdash`
- 注意：网页上显示的 clone 地址端口固定为 2222，SSH 端口改动后 clone 命令需手动调整（如 `ssh://git@host:2322/demo.git`）

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

## 使用流程

1. 打开网页 → **SSH Keys** → 粘贴你的公钥（如 `~/.ssh/id_ed25519.pub`）
2. **仓库** → 新建仓库（如 `demo`）
3. 克隆并推送：

```bash
git clone ssh://git@<host>:2222/demo.git
cd demo
echo "# demo" >> README.md
git add README.md && git commit -m "initial commit"
git push origin main
```

4. 回到网页即可浏览代码与提交历史。

## CI / 发版

- **CI**（`.github/workflows/ci.yml`）：Go build/vet/test + E2E 冒烟测试（真实走一遍建仓库 → 加公钥 → SSH clone/push → 代码浏览 API），前端 tsc + vite 构建。
- **发版**（`.github/workflows/release.yml`）：推送 tag 即自动发布：

```bash
git tag v0.1.0
git push origin v0.1.0
```

GoReleaser 会先构建前端并 embed，然后产出 linux / darwin / windows × amd64 / arm64 的压缩包与校验和，发布到 GitHub Releases。

本地验证发布配置：

```bash
goreleaser check
goreleaser release --snapshot --clean --skip=publish
```

## API 一览（需 `Authorization: Bearer <token>`）

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET/POST | `/api/repos` | 列出 / 创建仓库 |
| GET/DELETE | `/api/repos/{name}` | 详情 / 删除 |
| GET | `/api/repos/{name}/branches` | 分支列表 |
| GET | `/api/repos/{name}/tree?ref=&path=` | 浏览目录 |
| GET | `/api/repos/{name}/blob?ref=&path=` | 文件内容 |
| GET | `/api/repos/{name}/commits?ref=` | 提交历史 |
| GET/POST | `/api/keys` | 列出 / 添加 SSH 公钥 |
| DELETE | `/api/keys/{id}` | 删除公钥 |

> MVP 注意：所有注册公钥对**所有仓库**拥有读写权限，鉴权模型请自行加强后再用于生产。
