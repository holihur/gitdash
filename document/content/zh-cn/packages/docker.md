---
title: "Docker / OCI 镜像"
weight: 2
summary: "推拉私有 Docker 镜像。"
---

```bash
docker login <实例>
docker tag myimage <实例>/<owner>/myimage:latest
docker push <实例>/<owner>/myimage:latest
```

使用 PAT 作为登录口令。镜像 blob 带访问控制，私有仓库不会泄漏。
