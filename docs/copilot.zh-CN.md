# BYOK Copilot (MVP)

仓库级 AI copilot 功能。每个**会话**运行在独立的 Docker 容器中。LLM 采用
**自带密钥（BYOK, bring your own key）**：gitdash 不内置、不代理任何 LLM 密钥，
只会把你配置的 Anthropic 兼容密钥以环境变量注入容器。

> MVP 范围：会话是常驻容器，输出通过 `docker logs` 查看。
> 交互式对话 / 提交-推送集成是后续工作。

## 概念

- **BYOK 密钥** —— 用户级 Anthropic 兼容 API 密钥（`provider` 固定为
  `anthropic`；`base_url` 可用于 Anthropic 兼容端点 / 代理）。密钥只存库，
  **任何读接口都不会返回**明文；界面仅显示 `key_set = true`。
- **Copilot 会话** —— 绑定到某个 BYOK 密钥的仓库级实例。每个会话就是一个
  Docker 容器（`gitdash-copilot-<id>`），仓库克隆副本挂载在 `/workspace`。

## 工作原理

1. 在 **个人资料 → BYOK** 配置一个或多个 BYOK 密钥。
2. 在仓库的 **Copilot** 页创建会话：选择 BYOK 密钥，可选填写
   `image`、`prompt`（任务）和 `command`。
3. gitdash 把仓库克隆到 `<data>/copilots/{owner}/{repo}/ws-<id>` 并运行：

   ```text
   docker run -d --name gitdash-copilot-<id> \
     --workdir /workspace -v <workspace>:/workspace \
     --network <GITDASH_COPILOT_NETWORK|bridge> \
     --memory 512m --cpus 1.0 --pids-limit 128 \
     --security-opt no-new-privileges --cap-drop ALL \
     -e GITDASH_COPILOT_ID=<id> -e GITDASH_OWNER=<owner> \
     -e GITDASH_REPO=<repo> -e GITDASH_REF=<branch> -e GITDASH_TASK=<prompt> \
     -e LLM_APIKEY=<key> [-e LLM_BASE_URL=<url>] [-e LLM_MODEL=<model>] \
     <image> [sh -ec "<command>"]
   ```

   省略 `command` 时，镜像自身入口点运行，并应读取 `GITDASH_*` / `LLM_*` 环境变量。

4. 通过 `启动` / `停止` / `删除` 管理容器，日志来自 `docker logs`。

## 依赖

- 服务端需要 Docker CLI + 守护进程。
- agent `image` 必须本地存在或可拉取。可在每个会话指定 `image`，或设置服务端默认值：

  ```bash
  GITDASH_COPILOT_IMAGE=your/agent:latest
  ```

## 环境变量

| 变量 | 说明 |
| --- | --- |
| `GITDASH_COPILOT_IMAGE` | 会话未指定镜像时的默认镜像 |
| `GITDASH_COPILOT_NETWORK` | 容器网络（默认 `bridge`；`none` 会禁用 LLM 访问） |

注入每个容器的变量：`GITDASH_COPILOT_ID`、`GITDASH_OWNER`、`GITDASH_REPO`、
`GITDASH_REF`、`GITDASH_TASK`、`LLM_APIKEY`，以及可选的
`LLM_BASE_URL` / `LLM_MODEL`。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/me/byok` | 列出我的 BYOK 密钥（不含明文） |
| POST | `/api/me/byok` | 创建 BYOK 密钥 |
| PUT | `/api/me/byok/{id}` | 更新 BYOK 密钥（`api_key` 留空保留原密钥） |
| DELETE | `/api/me/byok/{id}` | 删除 BYOK 密钥 |
| GET | `/api/users/{owner}/repos/{name}/copilots` | 列出会话 |
| POST | `/api/users/{owner}/repos/{name}/copilots` | 创建并启动会话 |
| GET | `/api/users/{owner}/repos/{name}/copilots/{id}` | 会话详情 + 日志 |
| POST | `/api/users/{owner}/repos/{name}/copilots/{id}/start` | 启动会话 |
| POST | `/api/users/{owner}/repos/{name}/copilots/{id}/stop` | 停止会话 |
| DELETE | `/api/users/{owner}/repos/{name}/copilots/{id}` | 删除会话 + 工作区 |
