---
title: "个人访问令牌（PAT）"
weight: 4
summary: "创建、限制范围与来源的 API/HTTPS 令牌。"
---

在 **Tokens** 页面创建 PAT：

- **Scopes**：`repo`（仓库读写）、`inbox`（收件箱）、`keys`（密钥）等，按需最小授权。
- **CIDR 白名单**：可选，限制令牌可用来源 IP。
- **过期时间**：可选，到期自动失效。

PAT 用于：

- HTTPS 克隆/推送：用户名任意，口令填 PAT。
- API 调用：`Authorization: Bearer <PAT>`。
- 包仓库（npm/pypi/...）认证：Basic 认证，口令填 PAT。
- `gitdash-cli` 登录。

令牌只在创建时显示一次，请妥善保存。
