---
title: "Configure third-party login"
weight: 2
summary: "GitHub / Google / OIDC callback URLs and scopes."
---

Create an OAuth application for each provider. Callback URLs:

- GitHub: `<instance>/api/auth/github/callback`
- Google: `<instance>/api/auth/google/callback`
- OIDC: `<instance>/api/auth/oidc/callback`

Fill in the Client ID / Secret and enable it; the matching entry then appears on the login page.
