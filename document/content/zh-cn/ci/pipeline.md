---
title: "流水线配置"
weight: 1
summary: ".gitdash.yml 语法、触发条件、Docker 步骤与超时。"
---

在仓库根目录放置 `.gitdash.yml`（或 `.gitdash/*.yml` 多个文件，各自独立触发）：

```yaml
on: [push, pull_request]
steps:
  - name: test
    image: golang:1.22
    run: go test ./...
  - name: build
    run: echo building
```

要点：

- `on:` 支持 `push` / `pull_request` / `manual` 等；多个流水线文件各自评估。
- `image:` 指定步骤运行的 Docker 镜像；省略时在宿主执行（需 `GITDASH_PIPELINE_EXEC=host`）。
- 支持并行分组、`timeout` / `job_timeout`、手动触发与延迟触发、取消、重跑、产物（artifacts）。
- 在仓库 **Settings → Pipeline** 打开开关。

仓库 **Pipeline** 页展示步骤 DAG、运行历史与日志。
