---
title: "自托管 Runner"
weight: 2
summary: "部署 gitdash-runner，用 runs-on 定向任务。"
---

当需要更强的算力或私有环境时，部署自托管 runner：

1. 在 **Runners** 页创建注册令牌（用户级/组织级/全局）。
2. 在目标机器运行 `gitdash-runner`，使用令牌连接服务器。
3. 在 `.gitdash.yml` 用 `runs-on:` 指定标签，任务会调度到匹配的 runner。

支持工作区快照流式传输、日志流式回传、取消与离线检测。

详见仓库文档 `docs/runners.md`。
