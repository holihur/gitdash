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

完整列表见仓库 README 的环境变量章节。
