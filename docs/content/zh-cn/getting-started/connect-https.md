---
title: "用 HTTPS 克隆"
weight: 4
summary: "用个人访问令牌（PAT）通过 HTTPS 克隆与推送，与 GitHub / GitLab 一致。"
---

除了 SSH，仓库还支持 **Git Smart HTTP**。克隆地址与常见 Git 托管一致：

```bash
git clone https://<host>/<owner>/<repo>.git
```

## 1. 创建个人访问令牌

打开 **Tokens** 页面，创建一个带 **repo** 作用域的 PAT 并复制（只显示一次）。

## 2. 用令牌克隆 / 推送

Git 询问凭据时，把 PAT 填在**密码**位置；用户名可任意（GitHub / GitLab 风格），
身份由令牌决定：

```bash
git clone https://<username>:<PAT>@<host>/<owner>/<repo>.git
```

或让 Git 交互提示，在密码处粘贴 PAT：

```bash
git clone https://<host>/<owner>/<repo>.git
# Username: 任意
# Password: <PAT>
```

远端配置好令牌后，推送（push）用法相同。

## 3. 免重复输入

可启用凭据助手，例如：

```bash
git config --global credential.helper store
```

这样令牌会以明文存入凭据文件，请像密码一样妥善保管；尽量使用**只读**或短期的 PAT。

## 访问规则

- **推送**要求写权限（所有者 / 协作者 / 组织成员）。
- **克隆 / 拉取**按仓库可见性：
  - `private` —— 仅自己与协作者；
  - `public` —— 任意已登录用户；
  - `anonymous` —— 任何人，无需登录。

## 说明

- 部署密钥（deploy key）仅支持 SSH；HTTPS 请使用 PAT。
- 令牌以 HTTP Basic 方式发送，生产环境务必使用 **HTTPS**。

## 下一步

- [首次推送](first-push/)
