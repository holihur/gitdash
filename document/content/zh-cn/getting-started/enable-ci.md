---
title: "开启 CI"
weight: 6
summary: "在仓库放置 .gitdash.yml，push 即触发流水线。"
---

在仓库根目录新增 `.gitdash.yml`：

```yaml
on: [push]
steps:
  - name: test
    run: go test ./...
```

在仓库 **Settings → Pipeline** 打开开关。之后每次 push 都会在 **Pipeline** 标签页产生一次运行记录。

- 需要 Docker 时可在步骤里指定 `image:`；也可用 `runs-on:` 指定自托管 runner。
- 完整语法见 [CI · 流水线](../ci/pipeline/)，runner 部署见 [CI · Runner](../ci/runners/)。
