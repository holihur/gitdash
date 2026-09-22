---
title: "Deploy keys"
weight: 12
summary: "Give a machine read or read-write access to a single repository."
---

A **deploy key** is an SSH public key that grants access to a single repository — useful for CI runners, build servers or read-only mirrors, without creating a user account or a personal access token.

Manage them under **Repository → Settings → Deploy keys** (repository owner only).

## Add a key

1. Generate a key pair on the machine: `ssh-keygen -t ed25519 -f deploy_key -N ""`.
2. Paste the contents of `deploy_key.pub` into the form, give it a **title**, and choose:
   - **Read-only** (default): clone/fetch only.
   - **Read & write**: also allows `git push`.
3. Click **Add deploy key**.

## Use it

```bash
GIT_SSH_COMMAND="ssh -i deploy_key" \
  git clone ssh://git@<host>:2222/<owner>/<repo>.git
```

Both `ssh://git@host:2222/owner/repo.git` and the short `ssh://git@host:2222/repo.git`
form work (the short form resolves to the bound repository).

## Scope and limits

- A deploy key can only access the repository it is bound to; other repositories are
  rejected even when its owner has access.
- Fingerprints are globally unique: the same key cannot be registered twice (use a
  separate key per repository).
- Deploy keys grant no web/API access — they only work over SSH.

## Revoke

Delete the key under **Repository → Settings → Deploy keys**. Access is removed immediately.
