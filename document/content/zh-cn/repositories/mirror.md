---
title: "推送镜像"
weight: 4
summary: "把仓库分支与标签自动 push 到第三方远端。"
---

在 **仓库 → Settings → Sync to remote** 配置镜像目标：

- **Remote URL**：目标远端地址（如 GitHub/GitLab）。
- **SSH 私钥**：SSH 远端需写入权限的 Deploy Key 私钥（加密存储）。

配置后，每次 push 到 Gitdash 都会自动推送到镜像远端；也可手动 **Sync now**。同步状态与错误可在设置里查看。
