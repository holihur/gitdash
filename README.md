# gitdash

一个最小的自托管 Git 服务 MVP（类似迷你 Gitea）：

- **Git SSH 服务**：内置 SSH server（默认 `:2222`），支持 `git clone` / `push` / `pull` / 上传归档
- **代码浏览**：网页端按分支 / 目录浏览仓库、查看文件内容、查看提交历史
- **仓库管理**：网页端创建 / 删除仓库
- **SSH Key 管理**：网页端增删公钥（CRUD），公钥即凭证
- **前端**：React + Vite + Tailwind + shadcn/ui 风格组件
- **后端**：Go 标准库 HTTP + `golang.org/x/crypto/ssh` + SQLite（modernc，纯 Go 无 CGO），git 操作直接调用系统 `git`

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
| `GITDASH_STATIC` | 自动探测 | 前端静态文件目录 |

### 2. 启动前端（开发）

```bash
cd frontend
npm install
npm run dev
# 打开 http://localhost:5173，/api 已代理到 :8080
```

### 3. 生产模式（单进程托管前端）

```bash
cd frontend && npm run build
cd ../backend && go build -o gitdash-server .
# 后端会自动探测 ../frontend/dist 或 ./static 目录并托管前端
./gitdash-server
```

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
