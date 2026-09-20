---
title: "Configuration"
weight: 2
summary: "Common environment variables: ports, data dir, secret, proxies, SSRF, docs."
---

Common environment variables:

| Variable | Description |
|---|---|
| `GITDASH_DATA` | Data directory |
| `GITDASH_HTTP_ADDR` / `GITDASH_SSH_ADDR` | Listen addresses |
| `GITDASH_ADMIN_USER` / `GITDASH_ADMIN_PASSWORD` | Create the administrator on first boot |
| `GITDASH_SECRET_KEY` | Encryption key (credentials/tokens) |
| `GITDASH_DOCS_URL` | Documentation site URL (login-page/header entry) |
| `GITDASH_SSRF_ALLOW_PRIVATE` | Allow imports/mirrors to reach private networks |
| `GITDASH_TRUSTED_PROXIES` | Trusted reverse-proxy addresses |

See the repository README for the full list.
