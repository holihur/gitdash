# BYOK Copilot

仓库级 AI copilot。每个**会话**是仓库的一个检出副本，由独立的 **agent 运行时**
经一套很小的 HTTP+SSE 协议（[`copilot-api/1`](../deps/agent/docs/copilot-api.md)）驱动。
LLM 采用**自带密钥（BYOK, bring your own key）**：gitdash 不内置、不代理任何 LLM 密钥，
只把你配置的 Anthropic 兼容密钥以环境变量传给 agent 进程。

> gitdash 与 agent 解耦：gitdash 只说 `copilot-api/1`。agent 可替换，二进制路径 /
> 端点均可配置。

## 概念

- **BYOK 密钥** —— 用户级 Anthropic 兼容 API 密钥（`provider` 固定为
  `anthropic`；`base_url` 可用于 Anthropic 兼容端点 / 代理）。密钥只存库，
  **任何读接口都不会返回**明文；界面仅显示 `key_set = true`。
- **Copilot 会话** —— 绑定到某个 BYOK 密钥与一个工作区
  （`<data>/copilots/{owner}/{repo}/ws-<id>`），检出在 `copilot/session-<id>` 分支。
- **agent 运行时** —— 由 [`deps/agent`](../deps/agent) 子模块构建的 `agent` 二进制，
  每会话以 `agent -C <工作区> -api-addr 127.0.0.1:<port>` 启动。它提供聊天 API + Web UI，
  并受限在工作区内的 `shell` 与 `read`/`write`/`edit` 工具执行"思考-行动-观察"循环。

## 工作原理

1. 在 **个人资料 → BYOK** 配置一个或多个 BYOK 密钥。
2. 在仓库的 **Copilot** 页创建会话：选择 BYOK 密钥，可附加额外指令。
3. 打开会话：浏览器连到 gitdash 的 WebSocket，gitdash 把消息转发到 agent 的
   `POST /api/chat`，并把 `delta` / `tool_*` 事件流式回传；历史经 `GET /api/messages` 回放。
4. **闭环**：每轮结束（`done`）后，gitdash 在工作区执行
   `git add -A && git commit && git push origin HEAD:refs/heads/copilot/session-<id>`，
   分支随即出现在 gitdash 中，可评审 / 提 PR。每轮开始前 gitdash 会 fetch，并在工作区
   干净时把会话分支 rebase 到默认分支之上，让仓库侧的新提交也进入 agent 视野。

## 依赖

- `agent` 二进制。它由 `deps/agent` 子模块构建，随 gitdash 一起发布（安装脚本会一并安装）。
  本地开发用 `task agent`（或在 `deps/agent` 下 `go build -o agent ./cmd/agent`）。
- 若二进制不在 gitdash 同目录 / PATH 中，设置 `GITDASH_COPILOT_AGENT_BIN`。
- 想使用已在运行的 agent（而非每会话拉起），设置 `GITDASH_COPILOT_AGENT_URL`
  （如 `http://127.0.0.1:8790`）。它是单实例共享运行时，主要用于测试。
- 不需要 Docker。

## 环境变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `GITDASH_COPILOT_AGENT_BIN` | gitdash 同目录的 `agent`，否则 PATH | agent 运行时二进制 |
| `GITDASH_COPILOT_AGENT_URL` | 空 | 外部 agent 基地址（不再拉起进程） |

注入每个 agent 进程：`LLM_API_KEY`、`LLM_BASE_URL`、`LLM_MODEL`（来自 BYOK）。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/me/byok` | 列出我的 BYOK 密钥（不含明文） |
| POST | `/api/me/byok` | 创建 BYOK 密钥 |
| PUT | `/api/me/byok/{id}` | 更新 BYOK 密钥（`api_key` 留空保留原密钥） |
| DELETE | `/api/me/byok/{id}` | 删除 BYOK 密钥 |
| GET | `/api/users/{owner}/repos/{name}/copilots` | 列出会话 |
| POST | `/api/users/{owner}/repos/{name}/copilots` | 创建会话 |
| GET | `/api/users/{owner}/repos/{name}/copilots/{id}` | 会话详情 |
| GET | `/api/users/{owner}/repos/{name}/copilots/{id}/messages` | 对话历史 |
| GET | `/api/users/{owner}/repos/{name}/copilots/{id}/chat` | 双向聊天（WebSocket） |
| POST | `/api/users/{owner}/repos/{name}/copilots/{id}/stop` | 取消当前轮次 |
| DELETE | `/api/users/{owner}/repos/{name}/copilots/{id}` | 删除会话 + 工作区 |

### 聊天 WebSocket

客户端 → 服务端：`{"type":"user","text":"…"}` / `{"type":"cancel"}`。

服务端 → 客户端：`{"type":"history","messages":[…]}`、`{"type":"status","status":"idle|running"}`、
`{"type":"delta","text":"…"}`、`{"type":"tool_start","name","input"}`、
`{"type":"tool_end","name","result","is_error"}`、`{"type":"done","text"}`、
`{"type":"error","error"}`。
