---
title: "OAuth apps"
weight: 7
summary: "Use gitdash as an OAuth 2.0 authorization server for third-party apps."
---

Register third-party applications on the **OAuth Apps** page:

- Configure name, homepage and callback URL, and receive a `client_id` and `client_secret`.
- Supports the authorization-code flow and the device-code flow (RFC 8628).
- Issues access tokens scoped to `repo` / `inbox` / `keys`.
- Revoke any grant from the authorized-applications list.

See the repository's OAuth 2.0 provider documentation for details.
