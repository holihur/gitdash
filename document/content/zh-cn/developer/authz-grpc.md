---
title: "授权面 gRPC（AuthzService）"
weight: 1
summary: "把 SSH 鉴权收敛为 gRPC 服务，供未来的独立 SSH 网关调用。"
---

`AuthzService` 是 Gitdash 的**授权面（control plane）**：把当前 SSH 服务在进程内直接调用的鉴权逻辑，收敛成一组稳定的一元 gRPC 接口，供后续**独立的 SSH 网关**通过 gRPC 调用。

它只做「决策」，不接触仓库数据，也**不改变现有进程内 SSH / API / hook 的任何行为**。

## 定位与现状

```
        （未来）独立 SSH 网关
                │  gRPC: AuthorizePublicKey / IsIPBanned / CanRead / CanWrite
                ▼
        ┌───────────────────┐
        │  gitdash 主进程    │
        │  AuthzService      │  ← 本页描述的授权面
        │  + store（唯一权威）│
        └───────────────────┘
                ▲
                │ 进程内直接调用（现状，未改动）
        ┌───────┴───────┐
        │ internal/sshserver │
        └───────────────┘
```

- **现状（第一阶段）**：授权面是**纯新增的旁路服务**，默认关闭，还没有任何消费者。现有 `internal/sshserver` 仍然直接调 `store`，行为逐字节不变。
- **目标**：契约先冻结；后续拆出独立 `gitdash-ssh` 时，只需「换 client + 移进程」，不再重新设计授权。

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

> 说明：地址不强制 loopback，由部署方按需选择；对外暴露时应置于私网或加 mTLS（见「安全模型 / Roadmap」）。

## 服务契约

Proto：`backend/proto/authz/v1/authz.proto`，包 `gitdash.authz.v1`，服务 `AuthzService`。

| RPC | 请求 → 响应 | 语义（与现状一致） |
|---|---|---|
| `AuthorizePublicKey` | `{key_type, key_blob}` → `{authorized, username, reason}` | 见下 |
| `IsIPBanned` | `{ip}` → `{banned}` | 等价 `store.IsIPBanned`（管理员 IP/CIDR 黑名单） |
| `CanRead` | `{owner, repo, username}` → `{allowed}` | 等价 `store.CanRead`（读 = clone/fetch/archive，已含仓库封禁） |
| `CanWrite` | `{owner, repo, username}` → `{allowed}` | 等价 `store.CanWrite`（写 = push，已含仓库封禁） |

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

## 安全模型

- **默认关闭**：不设 `GITDASH_GRPC_ADDR` 就没有监听。
- **强制服务令牌**：所有一元 RPC 经拦截器校验 `authorization: Bearer <GITDASH_GRPC_TOKEN>`；令牌缺失/不匹配返回 `UNAUTHENTICATED`。比较使用 `crypto/subtle.ConstantTimeCompare`（常数时间）。
- **无 reflection**：不注册 gRPC reflection，避免暴露接口面；客户端需自带 proto。
- **入站消息限制**：`MaxRecvMsgSize = 1 MiB`（授权请求都很小）。
- **不回传敏感数据**：仅返回布尔决策 + 命中的用户名，不回传公钥/规则明细。

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

1. **本阶段（已完成）**：新增默认关闭、带令牌校验的授权面，覆盖现有 SSH 的全部鉴权调用；其他不动。
2. **独立 SSH 网关**：把 `internal/sshserver` 抽成独立二进制，鉴权改为调用本服务；`git export`/数据面按部署形态选择（共享存储 / 无状态代理）。
3. **传输加固**：跨机器时改 mTLS + 服务身份，替代共享令牌。
4. **多实例/分区**：配合 `region`/IP 路由做横向扩展（见后续设计文档）。

## 非目标（本阶段）

- 不拆分 SSH 进程、不改 `internal/sshserver`
- 不改 API / hook / 前端 / 克隆地址
- 不引入数据面传输端点
