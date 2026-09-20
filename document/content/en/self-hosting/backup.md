---
title: "Backup & restore"
weight: 4
summary: "Consistent snapshots, verification, retention and scheduled backups."
---

```bash
gitdash backup   # consistent backup (DB + repos + webhook spool + SSH host key)
gitdash restore  # restore (--dry-run verifies)
```

Supports a retention policy (`--keep`) and scheduled background backups (`GITDASH_BACKUP_DIR`).
