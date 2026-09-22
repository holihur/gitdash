---
title: "备份与恢复"
weight: 4
summary: "一致性快照、校验、保留策略与定时备份。"
---

```bash
gitdash backup   # 生成一致性备份（DB + 仓库 + webhook spool + SSH host key）
gitdash restore  # 恢复（--dry-run 可校验）
```

支持保留策略 `--keep` 与定时后台备份 `GITDASH_BACKUP_DIR`。
