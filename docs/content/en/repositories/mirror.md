---
title: "Push mirror"
weight: 4
summary: "Automatically push branches and tags to a third-party remote."
---

Configure a mirror target under **Repository → Settings → Sync to remote**:

- **Remote URL**: the target remote (e.g. GitHub/GitLab).
- **SSH private key**: for SSH targets, a deploy key with write access (stored encrypted).

Once configured, every push to Gitdash is mirrored automatically; you can also click **Sync now**. Sync status and errors are shown in the settings.
