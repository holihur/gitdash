---
title: "Import a repository"
weight: 2
summary: "Mirror-import from any git URL: http(s), ssh or git."
---

**Repositories → Import repository → Import by URL**:

- **Repository URL**: `https://`, `ssh://`, `git://` (private networks only) and scp-like (`git@host:owner/repo.git`).
- **Name**: optional; inferred from the URL by default.
- **Private**: whether the new repository is private (default yes).
- **SSH private key**: for private sources, the private key of a read-only deploy key (used only for this import, never stored).

The import runs asynchronously (mirror clone, keeping all branches and tags). Track `import_status` on the repository page. When done, the source is shown on the repository page.

> Security: import URLs are validated against SSRF. Loopback/private/link-local addresses are blocked by default; self-hosted private networks can opt in with `GITDASH_SSRF_ALLOW_PRIVATE=1`.
