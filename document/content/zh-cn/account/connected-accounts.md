---
title: "绑定第三方账号"
weight: 6
summary: "绑定 GitHub / GitLab / Gitea / Bitbucket，用于批量导入仓库。"
---

在 **Profile → 已绑定账号** 可以绑定第三方代码托管平台账号：

- **GitHub**
- **GitLab**（gitlab.com 或自建）
- **Gitea / Forgejo**
- **Bitbucket**

绑定使用 OAuth 授权，gitdash 会保存访问令牌（加密存储），仅用于**列出并导入你的仓库**。随时可以解绑，解绑会删除令牌。

## 前提

对应平台需要管理员在 **Admin → 设置** 中启用并配置 OAuth 应用（Client ID / Secret 与回调地址），详见 [管理 · 第三方账号绑定](../admin/connected-accounts/)。

## 绑定后

前往 **Repositories → Import repository → 从已绑定账号导入**，勾选仓库即可批量导入，见 [批量导入](../repositories/batch-import/)。
