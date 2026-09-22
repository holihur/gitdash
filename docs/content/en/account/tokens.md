---
title: "Personal access tokens (PAT)"
weight: 4
summary: "Create API/HTTPS tokens with scopes and IP restrictions."
---

Create PATs on the **Tokens** page:

- **Scopes**: `repo` (repository read/write), `inbox`, `keys`, and more — grant the minimum you need.
- **CIDR allow-list**: optional source IP restriction.
- **Expiry**: optional; the token stops working when it expires.

Use a PAT for:

- HTTPS clone/push: any username, PAT as the password.
- API calls: `Authorization: Bearer <PAT>`.
- Package registries (npm/pypi/…): Basic auth with the PAT as the password.
- `gitdash-cli` login.

A token is shown only once at creation time — store it safely.
