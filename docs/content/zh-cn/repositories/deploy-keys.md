---
title: "部署密钥"
weight: 12
summary: "让一台机器只读或读写访问单个仓库。"
---

**部署密钥（deploy key）** 是只授权单个仓库访问的 SSH 公钥，适合 CI runner、构建机或只读镜像使用，无需创建用户账号或 Personal Access Token。

在 **仓库 → Settings → Deploy keys** 管理（仅仓库 owner）。

## 添加

1. 在目标机器生成密钥对：`ssh-keygen -t ed25519 -f deploy_key -N ""`。
2. 把 `deploy_key.pub` 的内容粘贴到表单，填写**标题**，并选择：
   - **只读**（默认）：仅 clone/fetch。
   - **读写**：同时允许 `git push`。
3. 点击**添加部署密钥**。

## 使用

```bash
GIT_SSH_COMMAND="ssh -i deploy_key" \
  git clone ssh://git@<host>:2222/<owner>/<repo>.git
```

`ssh://git@host:2222/owner/repo.git` 与简写 `ssh://git@host:2222/repo.git` 都可用
（简写会解析到绑定的仓库）。

## 范围与限制

- 部署密钥只能访问绑定的那个仓库；即使其属主有权限，访问其他仓库也会被拒绝。
- 指纹全局唯一：同一把钥匙不能重复注册（每个仓库应使用独立密钥）。
- 部署密钥不授予网页/API 访问，仅用于 SSH。

## 撤销

在 **仓库 → Settings → Deploy keys** 删除即可，权限立即失效。
