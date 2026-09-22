---
title: "导入仓库"
weight: 2
summary: "从任意 git URL 镜像导入，支持 http(s)/ssh/git。"
---

**Repositories → Import repository → 通过 URL 导入**：

- **Repository URL**：支持 `https://`、`ssh://`、`git://`（仅内网）与 scp-like（`git@host:owner/repo.git`）。
- **Name**：可选，缺省从 URL 推断。
- **Private**：是否私有，默认是。
- **SSH 私钥**：私有仓库可用只读 Deploy Key 的私钥（仅本次导入使用，不落库）。

导入为异步任务（镜像克隆，保留全部分支与标签），可在仓库页查看 `import_status`。导入完成后，来源信息会显示在仓库页。

> 安全：导入地址会做 SSRF 校验，默认禁止回环/私有/链路本地地址；自托管内网可用 `GITDASH_SSRF_ALLOW_PRIVATE=1` 放开。
