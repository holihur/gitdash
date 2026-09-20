---
title: "配置第三方登录"
weight: 2
summary: "GitHub / Google / OIDC 登录的回调地址与 scope。"
---

为每个提供方创建 OAuth 应用，回调地址：

- GitHub：`<实例>/api/auth/github/callback`
- Google：`<实例>/api/auth/google/callback`
- OIDC：`<实例>/api/auth/oidc/callback`

填写 Client ID / Secret 后启用即可在登录页显示对应入口。
