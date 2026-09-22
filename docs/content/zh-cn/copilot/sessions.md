---
title: "会话与修复 Issue"
weight: 2
summary: "创建会话、对话、关联 issue 自动开 PR。"
---

仓库级 AI copilot。每个**会话**是仓库的一个检出副本，由独立的 **agent 运行时**
经一套很小的 HTTP+SSE 协议（`copilot-api/1`）驱动。
LLM 采用**自带密钥（BYOK, bring your own key）**：Gitdash 不内置、不代理任何 LLM 密钥，
只把你配置的 Anthropic 兼容密钥以环境变量传给 agent 进程。

> Gitdash 与 agent 解耦：Gitdash 只说 `copilot-api/1`。agent 可替换，二进制路径 /
> 端点均可配置。

## 概念

- **BYOK 密钥** —— 用户级 LLM 密钥，带一个 `provider` 预设（`anthropic` / `compatible` /
  `ollama`，见下）；`base_url` 指向 Anthropic 兼容端点（官方、网关或本地 Ollama）。
  密钥只存库，**任何读接口都不会返回**明文；界面仅显示 `key_set = true`。
- **Copilot 会话** —— 绑定到某个 BYOK 密钥与一个工作区
  （`<data>/copilots/{owner}/{repo}/ws-<id>`），检出在 `copilot/session-<id>` 分支；
  可关联一个 issue（`issue_number`），闭环后自动开 PR。
- **agent 运行时** —— 由 `deps/agent` 子模块构建的 `agent` 二进制，
  每会话以 `agent -C <工作区> -api-addr 127.0.0.1:<port>` 启动。它提供聊天 API + Web UI，
  并受限在工作区内的 `shell` 与 `read`/`write`/`edit` 工具执行"思考-行动-观察"循环。

## 供应商预设

agent 的 LLM 客户端只说 **Anthropic Messages 兼容协议**（`POST {base_url}/v1/messages`），
因此所有预设都必须指向 Anthropic 兼容端点：

| provider | 默认 base_url | 需要密钥 | 说明 |
| --- | --- | --- | --- |
| `anthropic` | `https://api.anthropic.com` | 是 | Anthropic 官方端点 |
| `compatible` | 无（必填） | 是 | 任意 Anthropic 兼容网关（LiteLLM / one-api / 自建），可转接 OpenAI / vLLM / Ollama |
| `ollama` | `http://127.0.0.1:11434` | 否 | 本地 Ollama（需其 Anthropic 兼容 `/v1/messages` 端点），无密钥时由服务端补占位密钥 |

在 **个人资料 → BYOK** 里可直接点击「测试连接」发一次最小请求验证 provider / base_url /
model / 密钥是否可用（`POST /api/me/byok/test`）。

## 工作原理

1. 在 **个人资料 → BYOK** 配置一个或多个 BYOK 密钥。
2. 在仓库的 **Copilot** 页创建会话：选择 BYOK 密钥，可附加额外指令。
3. 打开会话：浏览器连到 Gitdash 的 WebSocket，Gitdash 把消息转发到 agent 的
   `POST /api/chat`，并把 `delta` / `tool_*` 事件流式回传；历史经 `GET /api/messages` 回放。
4. **闭环**：每轮结束（`done`）后，Gitdash 在工作区执行
   `git add -A && git commit && git push origin HEAD:refs/heads/copilot/session-<id>`，
   分支随即出现在 Gitdash 中，可评审 / 提 PR。每轮开始前 Gitdash 会 fetch，并在工作区
   干净时把会话分支 rebase 到默认分支之上，让仓库侧的新提交也进入 agent 视野。
5. **issue → PR**：会话关联了 issue 时，推送成功后 Gitdash 自动开一个 PR
   （标题 `fix: <issue 标题>`，正文 `Closes #N`，源分支为会话分支，目标为默认分支），
   并在会话上记录 `pr_number`（幂等：已有同源分支的 open PR 则复用）。入口有两个：
   网页端 issue 详情页的「用 Copilot 修复」，或 CLI `gitdash-cli copilot fix`。

## 依赖

- `agent` 二进制。它由 `deps/agent` 子模块构建，随 Gitdash 一起发布（安装脚本会一并安装）。
  本地开发用 `task agent`（或在 `deps/agent` 下 `go build -o agent ./cmd/agent`）。
- 若二进制不在 Gitdash 同目录 / PATH 中，设置 `GITDASH_COPILOT_AGENT_BIN`。
- 想使用已在运行的 agent（而非每会话拉起），设置 `GITDASH_COPILOT_AGENT_URL`
  （如 `http://127.0.0.1:8790`）。它是单实例共享运行时，主要用于测试。
- 不需要 Docker。

## 环境变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `GITDASH_COPILOT_AGENT_BIN` | Gitdash 同目录的 `agent`，否则 PATH | agent 运行时二进制 |
| `GITDASH_COPILOT_AGENT_URL` | 空 | 外部 agent 基地址（不再拉起进程） |
| `GITDASH_LLM_ALLOW_HOSTS` | 空 | 逗号分隔的 `host` 或 `host:port` 白名单，放行 LLM 端点的 SSRF 拦截（如私有网关或本地 Ollama）。仅作用于 BYOK/copilot，不影响 webhook/导入的 SSRF 防护；`GITDASH_SSRF_ALLOW_PRIVATE=1` 可全局放开私有网段。 |

注入每个 agent 进程：`LLM_API_KEY_FILE`（短生命周期的 0600 临时文件，避免密钥出现在
进程环境/`/proc/<pid>/environ` 中）、`LLM_BASE_URL`、`LLM_MODEL`、`LLM_PROVIDER`、
`LLM_AUTH_STYLE`（均来自所选 BYOK 预设；`ollama` 无密钥时使用占位密钥）。agent 也直接接受
`LLM_API_KEY`。

## CLI

```bash
gitdash-cli copilot fix <owner/repo> <issue-number> [--byok <名称|id>] [--instructions <文本>]
gitdash-cli copilot fix <owner/repo> <issue-number> --detach   # 只建会话
gitdash-cli copilot list <owner/repo>
gitdash-cli copilot run <owner/repo> <session-id> --text "..."
```

`copilot fix` 等价于网页端「用 Copilot 修复」：创建关联 issue 的会话、通过 WebSocket
驱动 agent 并在 `done` 后打印自动开出的 PR；`issue fix` 是它的别名。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/me/byok` | 列出我的 BYOK 密钥（不含明文） |
| POST | `/api/me/byok` | 创建 BYOK 密钥（`ollama` 可不带 `api_key`） |
| POST | `/api/me/byok/test` | 测试连接（`api_key` 可留空并回退到 `id` 对应密钥） |
| PUT | `/api/me/byok/{id}` | 更新 BYOK 密钥（`api_key` 留空保留原密钥） |
| DELETE | `/api/me/byok/{id}` | 删除 BYOK 密钥 |
| GET | `/api/users/{owner}/repos/{name}/copilots` | 列出会话 |
| POST | `/api/users/{owner}/repos/{name}/copilots` | 创建会话（可带 `issue_number` 关联 issue） |
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
