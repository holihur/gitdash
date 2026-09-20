---
title: "Issue 与 Pull Request"
weight: 5
summary: "提 issue、开分支、发 PR、评审与合并。"
---

## 创建 Issue

进入仓库的 **Issues** 标签页，点击 **New issue**，填写标题与正文，可选标签、里程碑。

## 发起 Pull Request

1. 新建分支并提交改动：

   ```bash
   git checkout -b feature/x
   git commit -am "feat: x"
   git push -u origin feature/x
   ```

2. 在仓库页面发起 PR，选择 base（目标）与 head（来源）分支。
3. 添加评审人、描述，提交。
4. 评审通过后可按 **Squash merge** 等策略合并。

## 邮件补丁（可选）

gitdash 也支持 `git send-email` 风格的补丁与邮件回复流程，见 [PR · 邮件补丁](../pulls/email-patches/)。

## 下一步

- [开启 CI](enable-ci/)
