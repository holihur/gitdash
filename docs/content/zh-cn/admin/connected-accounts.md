---
title: "配置账号绑定（批量导入）"
weight: 3
summary: "为 GitLab / Gitea / Bitbucket 配置 OAuth，支持批量导入仓库。"
---

在 **Admin → 设置** 启用并配置：

| 平台 | 回调地址 | 备注 |
|---|---|---|
| GitLab | `<实例>/api/connections/gitlab/callback` | 可填 gitlab.com 或自建 Base URL；scope：`read_user read_api read_repository` |
| Gitea | `<实例>/api/connections/gitea/callback` | 必填自建 Base URL；创建 OAuth2 应用 |
| Bitbucket | `<实例>/api/connections/bitbucket/callback` | 权限：`account` + `repository` |

GitHub 复用 [第三方登录](oauth-login/) 的 OAuth 应用，但需要仓库读取权限（`repo`）。

配置完成后，用户即可在 **Profile → 已绑定账号** 绑定，并在导入弹窗中批量导入仓库。
