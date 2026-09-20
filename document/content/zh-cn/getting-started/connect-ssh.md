---
title: "配置 SSH"
weight: 3
summary: "生成密钥并把公钥添加到 gitdash，用 SSH 克隆与推送。"
---

推荐使用 SSH 访问仓库。

## 1. 生成密钥（若还没有）

```bash
ssh-keygen -t ed25519 -C "you@example.com"
```

## 2. 添加公钥

打开 **SSH Keys** 页面，粘贴 `~/.ssh/id_ed25519.pub` 的内容，保存。

## 3. 测试

```bash
ssh -T -p 2222 git@<你的实例地址>
```

## 克隆地址

仓库页面会显示 SSH 克隆地址，形如：

```bash
git clone ssh://git@<host>:2222/<owner>/<repo>.git
```

> 也可以使用 HTTPS + 个人访问令牌（PAT）：用户名任意，口令填 PAT。PAT 在 **Tokens** 页面创建。

## 下一步

- [首次推送](first-push/)
