---
title: "会话与修复 Issue"
weight: 2
summary: "创建会话、对话、关联 issue 自动开 PR。"
---

在仓库 **Copilot** 标签页新建会话：

- 可选关联一个 issue；Agent 推送后会自动开一个 PR（正文含 `Closes #N`）。
- 在对话中让 Agent 检查或修改仓库。
- 也可用 `gitdash-cli copilot fix <owner/repo> <issue>` 触发。

详见仓库文档 `docs/copilot.md`。
