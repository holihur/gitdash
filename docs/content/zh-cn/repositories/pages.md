---
title: "Pages（静态网站托管）"
weight: 11
summary: "从仓库分支/目录发布静态网站。"
---

Gitdash 可以直接从仓库托管静态网站。Pages **默认关闭**，按仓库开启。

## 开启

1. 打开 **仓库 → Settings → Pages**。
2. 选择**源分支**（默认仓库默认分支）与**源目录**（默认仓库根，例如构建产物所在的 `public`）。
3. 点击**开启 Pages**。

站点访问地址：

```
https://<host>/pages/<owner>/<repo>/
```

## 路由

- `/` 或不带扩展名的路径会回退到该目录下的 `index.html`。
- 文件不存在时，若源根目录有 `404.html` 则用它作为 404 页面。
- 自动设置 Content-Type 与短缓存；支持 `HEAD`。

## 访问控制

- **公开**仓库：站点公开。
- **私有**仓库：访问者需要读权限（会话 cookie 或 Personal Access Token）。无权限请求返回 `404`，不会泄露私有站点是否存在。

## 安全

Pages 内容属于用户生成的 HTML/JS，因此统一以严格的 `Content-Security-Policy: sandbox`
提供服务（无同源权限）。脚本仍可运行，但页面无法读取主应用的 cookie/localStorage，
也不会携带凭据发起同源请求。

> 提示：要发布本站文档，可将 Pages 指向包含 `docs/public` 的分支/目录，或使用每次
> release 附带的 `gitdash-docs_<version>.tar.gz` 制品。
