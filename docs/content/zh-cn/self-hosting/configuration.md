---
title: "配置"
weight: 2
summary: "常用环境变量：端口、数据目录、密钥、代理、SSRF、文档地址。"
---

常用环境变量：

| 变量 | 说明 |
|---|---|
| `GITDASH_DATA` | 数据目录 |
| `GITDASH_HTTP_ADDR` / `GITDASH_SSH_ADDR` | 监听地址 |
| `GITDASH_ADMIN_USER` / `GITDASH_ADMIN_PASSWORD` | 首次启动创建管理员 |
| `GITDASH_SECRET_KEY` | 加密密钥（凭据/令牌加密） |
| `GITDASH_DOCS_URL` | 文档站地址（登录页/页头入口） |
| `GITDASH_SSRF_ALLOW_PRIVATE` | 允许导入/镜像访问内网 |
| `GITDASH_TRUSTED_PROXIES` | 受信反代地址 |
| `GITDASH_CODE_SEARCH` | 代码搜索后端：`bleve`（**默认**，嵌入式全文索引，增量重建；标识符感知 + CJK + 智能大小写；仅默认分支；最终一致——重建中返回 `indexing:true`）、`grep`（显式实时 `git grep`）或 `remote` |
| `GITDASH_SEARCH_URL` / `GITDASH_SEARCH_TOKEN` | 远程索引服务地址 / 共享 Bearer token（API 节点用 `remote` 时必填，worker 侧也要配置） |
| `GITDASH_ROLE=codeindex` | 以独立索引 worker 运行：消费 `gitdash:codeindex` asynq 队列、持有索引并提供内部检索端点（需 `GITDASH_QUEUE=redis` 并共享数据/数据库） |
| `GITDASH_SEARCH_LISTEN` / `GITDASH_CODE_INDEX_CONCURRENCY` / `GITDASH_CODE_INDEX_CONSUME` | worker 检索监听地址 / 索引并发 / API 节点设为 `0` 表示只生产索引任务 |

完整列表见仓库 README 的环境变量章节。

### 代码搜索拆服务部署

Bleve 为单进程独占（单写者），因此由独立 worker 持有索引，API 节点把检索委托给它：

```bash
# 索引 worker（持有索引，消费 asynq 队列）
GITDASH_ROLE=codeindex GITDASH_QUEUE=redis GITDASH_REDIS_ADDR=redis:6379 \
GITDASH_SEARCH_TOKEN=$TOKEN GITDASH_SEARCH_LISTEN=0.0.0.0:8090 gitdash serve

# API 节点（只生产索引任务，检索走 worker，不打开本地索引）
GITDASH_QUEUE=redis GITDASH_REDIS_ADDR=redis:6379 \
GITDASH_CODE_SEARCH=remote GITDASH_SEARCH_URL=http://index-worker:8090 \
GITDASH_SEARCH_TOKEN=$TOKEN gitdash serve
```

两个进程需共享 `GITDASH_DATA`（仓库目录）与数据库。没有 Redis 时，代码索引在进程内运行
（`GITDASH_CODE_SEARCH=bleve`），无需 `remote`。
