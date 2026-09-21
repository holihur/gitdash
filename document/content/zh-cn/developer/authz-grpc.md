---
title: "授权面 gRPC（AuthzService）"
weight: 1
summary: "把 SSH 鉴权收敛为 gRPC 服务，支持独立 SSH 网关与分机部署。"
---

`AuthzService` 是 Gitdash 的**授权面（control plane）**：把 SSH 服务在进程内直接调用的鉴权逻辑，收敛成一组稳定的一元 gRPC 接口。默认的 all-in-one 部署仍走进程内调用；设置 `GITDASH_ROLE=ssh` 后，同一二进制可作为**独立 SSH 网关**运行，鉴权全部经本服务完成。

它只做「决策」，不接触仓库数据。

## 定位与现状

```
                （可选）独立 SSH 网关
  GITDASH_ROLE=ssh │  gRPC: AuthorizePublicKey / IsIPBanned / CanRead / CanWrite / BranchProtection
                   ▼
        ┌───────────────────┐
        │  gitdash 主进程    │
        │  AuthzService      │  ← 本页描述的授权面
        │  + store（唯一权威）│
        └───────────────────┘
                ▲
                │ all-in-one：进程内直接调用（默认，行为不变）
        ┌───────┴───────┐
        │ internal/sshserver │
        └───────────────┘
```

- **默认（all-in-one）**：`internal/sshserver` 通过 `authz.StoreAuthorizer` 直接读 store，行为与拆分前一致；未设置 `GITDASH_ROLE=ssh` 时不启用网关模式。
- **分机部署**：设置 `GITDASH_ROLE=ssh`（或 `GITDASH_SSH_ONLY=1`）后，同一二进制以「仅 SSH 网关」角色运行，**不打开数据库、不启动 HTTP API**，全部鉴权经授权面 gRPC 完成。
- SSH 侧只依赖 `authz.Authorizer` 抽象，本地（store）/ 远端（gRPC）可无缝切换。

## 部署形态

### 形态 A：单机（all-in-one，默认）

HTTP API 与 SSH 在同一进程：SSH 经 `StoreAuthorizer` 直接读库。不设置
`GITDASH_ROLE` 即为此形态，行为与拆分前一致。

```
  浏览器 / git CLI
     │ HTTP :8080        │ SSH :2222
     ▼                   ▼
  ┌──────────────────────────────────────────┐
  │              gitdash 进程                  │
  │  HTTP API  +  内置 SSH（StoreAuthorizer）  │
  │  store（SQLite / PostgreSQL）＝ 唯一权威    │
  └──────────────────────────────────────────┘
     │
  GITDASH_DATA/repos（本地磁盘）
```

### 形态 B：分机（API + SSH 网关）

API 机器持有数据库与授权面；SSH 网关机器不连数据库，鉴权经 gRPC。两台机器
需共享仓库目录（`GITDASH_DATA/repos`）与其上的 push 事件 spool。

```
        API / 控制面机器                          SSH 网关机器
  ┌────────────────────────────┐        ┌────────────────────────────┐
  │  gitdash serve             │        │  gitdash serve             │
  │  GITDASH_GRPC_ADDR=:9090   │◄──gRPC─┤  GITDASH_ROLE=ssh          │
  │                            │ TLS +  │                            │
  │  ┌──────────────┐          │ Bearer │  ┌──────────────────────┐  │
  │  │  HTTP API    │          │ token  │  │ sshserver            │  │
  │  ├──────────────┤          │        │  │ (RemoteAuthorizer)   │  │
  │  │ 授权面 gRPC   │          │        │  └──────────┬───────────┘  │
  │  ├──────────────┤          │        │             │              │
  │  │ store（权威） │          │        │             │ git-upload/  │
  │  └──────────────┘          │        │             │ receive-pack │
  └─────────────┬──────────────┘        └─────────────┼──────────────┘
                │                                     │
                └────────► 共享存储 GITDASH_DATA/repos ◄────────┘
                           （NFS / 同一持久卷）
  浏览器 ──HTTP──► API 机器                    git CLI ──SSH──► 网关机器
```

> 网关机器只需：SSH 端口、`GITDASH_GRPC_ADDR/TOKEN`、共享的 `GITDASH_DATA`。
> 它不打开数据库；分支保护规则也经授权面获取。

## 启用与配置

授权面**默认不启动**。仅当显式设置 `GITDASH_GRPC_ADDR` 时才监听：

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `GITDASH_GRPC_ADDR` | 空（关闭） | gRPC 监听地址。为空则不启动授权面（老部署零影响）。建议 `127.0.0.1:9090` 或私网地址 |
| `GITDASH_GRPC_TOKEN` | 空 | 服务令牌。**设置 `GITDASH_GRPC_ADDR` 时必须提供**，否则进程启动即失败（`grpc authz: GITDASH_GRPC_TOKEN must be set`） |

```bash
GITDASH_GRPC_ADDR=127.0.0.1:9090 \
GITDASH_GRPC_TOKEN=$(head -c 32 /dev/urandom | base64) \
./gitdash serve
```

> 说明：地址不强制 loopback，由部署方按需选择；对外暴露时应置于私网或启用 TLS（见下）。

## 分机部署（SSH 网关）

让 API 与 SSH 跑在不同机器上：API 端开启授权面，SSH 端以网关角色启动。

API 端（主进程，权威数据源）：

```bash
GITDASH_GRPC_ADDR=0.0.0.0:9090 \
GITDASH_GRPC_TOKEN=$(head -c 32 /dev/urandom | base64) \
GITDASH_GRPC_TLS_CERT=/etc/gitdash/grpc.crt \
GITDASH_GRPC_TLS_KEY=/etc/gitdash/grpc.key \
./gitdash serve
```

SSH 网关端（仅 SSH，无数据库）：

```bash
GITDASH_ROLE=ssh \
GITDASH_DATA=/shared/gitdash-data \       # 与 API 共享仓库目录与 spool（共享存储）
GITDASH_SSH_ADDR=:2222 \
GITDASH_GRPC_ADDR=api.internal:9090 \
GITDASH_GRPC_TOKEN=<同 API 端令牌> \
GITDASH_GRPC_CA=/etc/gitdash/grpc-ca.crt \  # 启用 TLS 时提供 CA
./gitdash serve
```

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `GITDASH_ROLE` | 空 | 设为 `ssh` 进入「仅 SSH 网关」模式；不设则 all-in-one |
| `GITDASH_SSH_ONLY` | 空 | 设为 `1` 等价于 `GITDASH_ROLE=ssh`（兼容旧约定） |
| `GITDASH_GRPC_ADDR` | 空 | 网关端必填：授权面地址 |
| `GITDASH_GRPC_TOKEN` | 空 | 网关端必填：与 API 端一致的服务令牌 |
| `GITDASH_GRPC_CA` | 空 | 可选：设置后网关端启用 TLS 并用该 CA 校验服务端证书 |
| `GITDASH_GRPC_SERVER_NAME` | 空 | 可选：TLS SNI / 服务端证书主机名 |
| `GITDASH_GRPC_TLS_CERT` / `GITDASH_GRPC_TLS_KEY` | 空 | API 端可选：两者同时设置则授权面启用 TLS |

> 部署要求：SSH 机需能访问仓库目录（共享存储/NFS）；`post-receive` 写出的 push 事件 spool 由 API 端调度器消费；分支保护规则经授权面获取，SSH 机无需数据库连接。

## 服务契约

Proto：`backend/proto/authz/v1/authz.proto`，包 `gitdash.authz.v1`，服务 `AuthzService`。

| RPC | 请求 → 响应 | 语义（与现状一致） |
|---|---|---|
| `AuthorizePublicKey` | `{key_type, key_blob}` → `{authorized, username, reason}` | 见下 |
| `IsIPBanned` | `{ip}` → `{banned}` | 等价 `store.IsIPBanned`（管理员 IP/CIDR 黑名单） |
| `CanRead` | `{owner, repo, username}` → `{allowed}` | 等价 `store.CanRead`（读 = clone/fetch/archive，已含仓库封禁） |
| `CanWrite` | `{owner, repo, username}` → `{allowed}` | 等价 `store.CanWrite`（写 = push，已含仓库封禁） |
| `BranchProtection` | `{owner, repo, branch}` → `{protected, block_deletion, block_force_push}` | 等价 `store.GetBranchProtection`；无规则时 `protected=false`（调用方放行） |

### AuthorizePublicKey 判定顺序

与 `internal/sshserver` 的 `PublicKeyCallback` **逐行对齐**：

1. 遍历已登记公钥，按 `(key_type, key_blob)` **精确匹配**（不做前缀/模糊匹配）；
2. 命中且账号**未封禁** → `authorized=true`，返回 `username`；
3. 命中但账号**被封禁** → `authorized=false`，`reason="account is banned"`；
4. 未命中 → `authorized=false`，`reason="unknown public key"`。

> **安全要点**：匹配在**服务端**完成，绝不回传任何公钥列表——授权面不会变成信息泄漏点。

### 与现有调用的对应关系

| 现状（`internal/sshserver`） | 授权面 RPC |
|---|---|
| `PublicKeyCallback`：`st.PublicKeys()` 全表比对 + `st.IsUserBanned` | `AuthorizePublicKey` |
| `handleConn`：`st.IsIPBanned(host)` | `IsIPBanned` |
| `runGit`：`st.CanWrite(...)` | `CanWrite` |
| `runGit`：`st.CanRead(...)` | `CanRead` |
| `pre-receive` hook：`st.GetBranchProtection(...)` | `BranchProtection` |

## 安全模型

- **默认关闭**：不设 `GITDASH_GRPC_ADDR` 就没有监听。
- **强制服务令牌**：所有一元 RPC 经拦截器校验 `authorization: Bearer <GITDASH_GRPC_TOKEN>`；令牌缺失/不匹配返回 `UNAUTHENTICATED`。比较使用 `crypto/subtle.ConstantTimeCompare`（常数时间）。
- **无 reflection**：不注册 gRPC reflection，避免暴露接口面；客户端需自带 proto。
- **入站消息限制**：`MaxRecvMsgSize = 1 MiB`（授权请求都很小）。
- **不回传敏感数据**：仅返回布尔决策 + 命中的用户名/规则，不回传公钥清单。
- **传输加固（可选 TLS）**：API 端设置 `GITDASH_GRPC_TLS_CERT/KEY`、网关端设置 `GITDASH_GRPC_CA` 即启用 TLS。跨公网或不可信网络务必开启。

## 调用示例

### grpcurl（未开 reflection，需指定 proto）

```bash
grpcurl -plaintext \
  -H "authorization: Bearer $GITDASH_GRPC_TOKEN" \
  -import-path backend/proto -proto authz/v1/authz.proto \
  -d '{"ip":"10.0.0.1"}' \
  127.0.0.1:9090 gitdash.authz.v1.AuthzService/IsIPBanned
```

### Go 客户端

```go
conn, err := grpc.NewClient("127.0.0.1:9090",
    grpc.WithTransportCredentials(insecure.NewCredentials()))
if err != nil {
    return err
}
client := authzv1.NewAuthzServiceClient(conn)

ctx := metadata.AppendToOutgoingContext(context.Background(),
    "authorization", "Bearer "+os.Getenv("GITDASH_GRPC_TOKEN"))

resp, err := client.CanWrite(ctx, &authzv1.CanWriteRequest{
    Owner: "alice", Repo: "demo", Username: "bob",
})
```

## 代码生成

Proto 改动后用 `buf`（自带编译器，无需 `protoc`）重新生成：

```bash
task proto        # 安装固定版本插件 + buf generate
```

生成物：`backend/internal/grpcserver/authzv1/*.pb.go`（**已提交**）。

- 工具版本固定在 `Taskfile.yml`：`protoc-gen-go v1.36.12`、`protoc-gen-go-grpc v1.6.2`、`buf v1.47.2`
- buf 配置：`backend/buf.yaml`、`backend/buf.gen.yaml`
- 建议 CI 加一步 `task proto && git diff --exit-code`，防止生成物漂移

## 测试

### 白盒单测（同包）

```bash
cd backend && go test ./internal/grpcserver/...
```

用例覆盖：公钥命中/未登记/封禁、IP 黑名单命中与未命中、`CanRead`/`CanWrite` 的 owner/公开仓库/协作者分支，以及令牌缺失/错误返回 `UNAUTHENTICATED`。

### 黑盒测试（独立进程 + 真实网络）

`backend/tests/blackbox/` 是对授权面的**独立黑盒测试**：现场构建 Gitdash 二进制（或复用 `GITDASH_BIN`），在独立临时数据目录 + 随机端口上以子进程启动，只经 HTTP / gRPC 网络接口交互，不 import 任何服务端实现（仅用生成的 gRPC 客户端桩），因此验证的是 `main.go` 的真实接线（env → gRPC listener → store）。

```bash
cd backend && go test ./tests/blackbox/ -v
# 复用预构建二进制：
# GITDASH_BIN=/tmp/gitdash-server go test ./tests/blackbox/ -v
```

覆盖：令牌缺失/错误 → `UNAUTHENTICATED`；IP 黑名单（经管理端 API 添加后生效）；`CanRead`/`CanWrite`（owner / 陌生人 / 公开仓库）；`AuthorizePublicKey`（命中 / 未登记 / 封禁）。`-short` 模式下跳过（构建 + 起进程较重）。

## Roadmap

1. **第一阶段（已完成）**：新增默认关闭、带令牌校验的授权面，覆盖现有 SSH 的全部鉴权调用。
2. **独立 SSH 网关（已完成）**：`GITDASH_ROLE=ssh` 支持同二进制以仅 SSH 角色运行，鉴权（含分支保护规则）经授权面完成；all-in-one 行为保持不变。
3. **传输加固（已完成，可选启用）**：跨机器可通过 `GITDASH_GRPC_TLS_CERT/KEY` + `GITDASH_GRPC_CA` 启用 TLS；后续可升级为 mTLS + 服务身份。
4. **多实例/分区**：配合 `region`/IP 路由做横向扩展（见后续设计文档）。

## 非目标（本阶段）

- 不改变 all-in-one（API + 进程内 SSH）的默认行为
- 不引入数据面传输端点（仓库目录仍依赖共享存储）
