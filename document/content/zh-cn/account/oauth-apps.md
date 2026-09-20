---
title: "OAuth 应用"
weight: 7
summary: "把 gitdash 作为 OAuth 2.0 授权服务器，签发访问令牌给第三方应用。"
---

在 **OAuth Apps** 页面注册第三方应用：

- 配置名称、主页、回调地址，获得 `client_id` 与 `client_secret`。
- 支持授权码流程与设备码流程（RFC 8628）。
- 可签发 `repo` / `inbox` / `keys` 范围的访问令牌。
- 在「已授权应用」列表可随时撤销授权。

详见仓库的 OAuth 2.0 provider 文档。
