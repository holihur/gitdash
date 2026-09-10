# Runner 使用指南（自托管 CI Agent）

自托管 runner 让 gitdash 的 CI 流水线不再局限于服务端本机 Docker：你可以在任意机器上部署
`gitdash-runner` agent，它主动连接 gitdash 服务端领取任务，在本地容器中执行 `.gitdash.yml`（`gitdash-runner run -exec host` 可切换为无 Docker 的宿主执行，无容器沙箱，需显式开启）
定义的流水线，并把日志、状态实时回传。

## 架构

```
┌──────────────┐   WS 长连接（agent 主动外连，无需开放入站端口）   ┌────────────────┐
│ gitdash-runner│ ◀──────────────────────────────────────────────▶ │  gitdash 服务端  │
│  (docker)     │   派发任务 / 回传日志与状态 / 心跳 / 取消          │  Redis Hub      │
└──────────────┘                                                  └────────────────┘
```

- agent → 服务端只需**出站**连接（`/api/runner/ws`），agent 可在 NAT/防火墙之后
- 服务端经 Redis pub/sub 定向派发（`gitdash:runner:{name}`），支持服务端多实例横向扩展
- 任务工作区由服务端用 `git archive` 打快照后经 agent 的连接推送，**不下发任何 git 凭证**
- 日志统一落盘在服务端 `<data>/pipelines/{owner}/{repo}/run-{id}.log`，前端查看与内置执行一致

## 前置条件

| 组件 | 要求 |
| --- | --- |
| 服务端 | `GITDASH_QUEUE=redis`（runner 功能依赖 Redis：派发、心跳、跨实例路由） |
| agent 机器 | 已安装 Docker（`docker` CLI + daemon 可用）；能出站访问服务端 HTTP 端口 |
| 网络 | 服务端与 agent 之间：WS（默认 80/443/自定义端口，经反代时需支持 Upgrade） |

未配置 Redis 时，服务端 runner 相关接口返回 503，仓库不写 `runs-on` 的流水线仍可在服务端本地执行。

## 第一步：签发一次性注册 token（10 分钟有效）

按作用域（scope）分三级，签发入口不同：

| Scope | 谁能签发 | 入口 | 作用范围 |
| --- | --- | --- | --- |
| `user:{owner}` | 用户本人 | **个人设置 → Runner → 签发注册 token** | 仅该用户名下仓库 |
| `org:{org}` | 组织 owner | API `POST /api/runners/registration-token`（`{"scope":"org","org":"<org名>"}`） | 仅该组织仓库 |
| 全局 | 站点管理员 | 管理端 `POST /api/admin/runners/registration-token` | 任意仓库 |

要点：
- token 是**一次性**的：注册成功即失效（重复使用返回 403）
- 有效期 10 分钟，过期返回 403，重新签发即可
- agent 与 token 的 scope 绑定终身：个人 runner 永远只会收到该用户仓库的任务

## 第二步：注册 agent

在 runner 机器上构建或获取 `gitdash-runner` 二进制（release 压缩包自带，或
`cd backend && go build -o gitdash-runner ./cmd/gitdash-runner`）：

```bash
gitdash-runner register \
  -server http://gitdash.example:8080 \
  -name build-01 \
  -labels docker,go1.22 \
  -token <一次性注册token>
```

- `-name`：全局唯一，`2-64` 位字母数字与 `._-`；重名返回 409
- `-labels`：逗号分隔，供 `.gitdash.yml` 的 `runs-on` 匹配（如 `docker,go1.22`）
- 成功后服务端返回一次 secret，写入 `~/.gitdash-runner/config.json`（权限 0600）

## 第三步：启动

```bash
gitdash-runner run
```

行为：
- 自动重连（断线 5s 后重试），重连后自动恢复在线状态
- 每 10s 心跳；服务端 30s 无心跳即判定 offline，其执行中的运行记为
  `failed (runner went offline)`（**不静默重跑**，避免双执行）
- 默认并发 2 个任务，可在配置文件中改 `concurrency`
- 每个任务：接收工作区快照（tar.gz）→ 解包到临时目录 → 用统一 parser 解析 DSL →
  在沙箱容器中按步骤执行 → 流式回传日志 → 回传最终状态

## 第四步：在流水线中使用

仓库根目录 `.gitdash.yml`：

```yaml
image: alpine:3.19
timeout: 10m
env:
  - CGO_ENABLED=0
runs-on: [docker]        # ← 新增：目标 runner 标签（须为 agent 标签的子集）
steps:
  - name: build
    run: echo build
```

- **不写 `runs-on`**：走服务端本地 Docker（builtin，行为与之前完全一致，向后兼容）
- **写 `runs-on`**：只派发给标签匹配且在线的 agent，取当前负载最低者
- 无匹配在线 agent：运行立即记 `failed (no online runner matches labels [...])`，
  不会无限排队——修好 agent 或改标签后手动重新触发
- 远程运行可在**运行详情**中点击「取消」，agent 会杀掉对应容器
- 运行详情会显示执行它的 runner 名

`runs-on` 支持两种写法：

```yaml
runs-on: [docker, go1.22]   # 内联（推荐）
# 或
runs-on:                     # 块列表
  - docker
  - go1.22
```

## 宿主执行模式（无 Docker）

目标机器没有 Docker 也能跑流水线：`.gitdash.yml` **省略 `image`**，步骤直接在宿主 `sh -ec`
中执行（工作区解包到临时目录，`GITDASH_REPO` / `GITDASH_REF` / `GITDASH_SHA` / `CI=1`
与仓库级环境变量、cfg `env` 一并注入，`volumes` 被忽略；同 key 时 cfg `env` 覆盖仓库变量）。

开启方式（**默认关闭**，两侧独立开启）：

```bash
# 服务端：允许本机 builtin 执行 image 为空的流水线
GITDASH_PIPELINE_EXEC=host gitdash serve

# agent：允许该 runner 执行 image 为空的流水线
gitdash-runner run -exec host
```

流水线写法（省略 `image`，其余键不变）：

```yaml
env:
  - GREETING=hello
steps:
  - name: build
    run: go build ./...
  - name: test
    run: go test ./...
```

未开启时：省略 `image` 的流水线记 `failed (host execution is disabled ...)`，
错误信息会提示开启方法；写了 `image` 的流水线不受影响（仍走 Docker 沙箱）。

注意事项：

- **无容器沙箱**：没有网络隔离、资源限制与 capability 降权——`env`、`timeout` 生效，
  但步骤代码对宿主文件系统/网络有完整访问权，等同把执行机交给该 scope 的仓库所有者
- 仅在你完全受信的机器上开启；不要用 root 跑 agent 的 host 模式
- 需要 POSIX `sh`（Linux/macOS 天然满足；Windows agent 不支持 host 模式）
- 黑盒测试用例见 `tests/test_pipeline_host.py`

## 管理 Runner

- 用户：个人设置 → Runner 卡片：在线状态（绿点）、标签、签发 token、删除
- 组织 owner：可在 Runner 列表中看到并删除 `org:` scope 的 runner
- 站点管理员：`GET /api/admin/runners`（全量）、`DELETE /api/admin/runners/{name}`、全局 token 签发
- 删除 runner 立即生效：该 agent 的后续心跳/任务会被拒绝（凭证随删除失效）

## 多实例与运维

- 服务端可多实例 + 负载均衡：agent 对连接无粘性要求（重连任意实例均会重新注册），
  派发经 Redis 定向到持有该 agent 连接的实例
- agent 掉线不自动重跑：如需重试，在运行详情手动触发（MVP 语义，保证日志与状态一致）
- 排查：
  - 服务端日志搜 `runner audit:`（CONNECT / REGISTER / DISPATCH / OFFLINE / DELETE）
  - agent 无日志回传：确认 agent 机器 `docker version` 可用
  - 运行卡 pending：无匹配 runner 时应已 failed；若 pending 说明队列（asynq）未消费，检查 Redis

## 安全模型（务必阅读）

1. **agent = 受信 CI 机器**：agent 在其宿主上执行仓库的任意代码，等同把宿主交给该 scope
   的仓库所有者。不要把 agent 部署在有敏感数据的机器上，也不要用 root 运行。
2. **scope 隔离**：个人/组织 runner 只会收到对应 scope 仓库的任务；全局 runner 由管理员签发，
   慎用。
3. **凭证最小化**：agent 只持有一个长期 secret（服务端仅存 sha256，可删除即时失效）；
   任务代码经快照流下发，不含任何 git 凭证。
4. **沙箱一致**：agent 侧执行与内置执行共用同一套参数——默认禁外网（`GITDASH_PIPELINE_NETWORK`
   可显式放开）、512MB 内存 / 1 CPU / 128 pids、`no-new-privileges`、丢弃全部 capabilities、
   拒绝挂载 `/var/run/docker.sock`。
   **host 模式绕过全部沙箱**（见「宿主执行模式」节），仅在受信机器上开启。
5. **注册 token 面窄**：一次性 + 10 分钟过期，泄露影响有限。

## 环境变量速查

| 变量 | 端 | 说明 |
| --- | --- | --- |
| `GITDASH_QUEUE=redis` | 服务端 | 启用 runner 功能（必填） |
| `GITDASH_REDIS_ADDR` / `_PASSWORD` / `_DB` | 服务端 | Redis 连接 |
| `GITDASH_PIPELINE_NETWORK` | 服务端/agent | 容器网络：默认 `none`，显式指定则用该值 |
| `GITDASH_PIPELINE_EXEC=host` | 服务端 | 允许 builtin 执行省略 `image` 的流水线（宿主 `sh`，无沙箱，默认关闭） |
| `gitdash-runner run -exec host` | agent | 允许该 runner 执行省略 `image` 的流水线（宿主 `sh`，无沙箱，默认关闭） |
| `GITDASH_PIPELINE_DEFAULT_TIMEOUT` | 服务端/agent | 单步默认超时（默认 10m，上限 1h） |
| config.json `concurrency` | agent | 并发任务数（默认 2，无对应环境变量） |
