---
title: "首次推送"
weight: 4
summary: "初始化本地仓库并推送到 gitdash。"
---

```bash
git init
git add .
git commit -m "first commit"
git branch -M main
git remote add origin ssh://git@<host>:2222/<owner>/<repo>.git
git push -u origin main
```

推送后：

- 仓库默认分支会更新（可在仓库设置里修改）。
- 若仓库启用了流水线，push 会触发 `.gitdash.yml` 中定义的 CI。
- 关注该仓库的用户会在收件箱收到动态。

## 下一步

- [Issue 与 Pull Request](first-issue-pr/)
- [开启 CI](enable-ci/)
