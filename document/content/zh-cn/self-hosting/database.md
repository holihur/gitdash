---
title: "数据库"
weight: 3
summary: "默认 SQLite，可切换到 PostgreSQL。"
---

默认使用嵌入式 SQLite（纯 Go，无 CGO）。可通过 `GITDASH_DATABASE_URL` 切换到 PostgreSQL。详见仓库文档 `docs/database.md`。
