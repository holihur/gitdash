---
title: "Pages (static hosting)"
weight: 11
summary: "Publish a static website from a repository branch or directory."
---

Gitdash can host a static website directly from a repository. Pages is **disabled by default** and enabled per repository.

## Enable

1. Open **Repository → Settings → Pages**.
2. Choose the **source branch** (defaults to the repository default branch) and the **source directory** (defaults to the repository root, e.g. `public` for a prebuilt site).
3. Click **Enable Pages**.

The site is served at:

```
https://<host>/pages/<owner>/<repo>/
```

## Routing

- `/` and any path without a file extension fall back to `index.html` in that directory.
- If a file is missing, `404.html` from the source root is served when present.
- Content types and a short cache header are set automatically; `HEAD` is supported.

## Access control

- **Public** repositories: the site is public.
- **Private** repositories: visitors need read access (session cookie or personal access token). Requests without access return `404`, so a private Pages site does not leak its existence.

## Security

A Pages site is user-generated HTML/JS, so it is served under a restrictive
`Content-Security-Policy: sandbox` (no same-origin access). Scripts still run, but the
page cannot read the main application's cookies/local storage, and credentialed
same-origin requests are not sent.

> Tip: to publish this documentation site, point Pages at the branch/directory that
> contains `docs/public`, or use the prebuilt `gitdash-docs_<version>.tar.gz` artifact
> attached to each release.
