---
title: "Configure account linking (batch import)"
weight: 3
summary: "Configure OAuth for GitLab / Gitea / Bitbucket to import repositories in bulk."
---

Enable and configure these under **Admin → Settings**:

| Platform | Callback URL | Notes |
|---|---|---|
| GitLab | `<instance>/api/connections/gitlab/callback` | gitlab.com or self-hosted base URL; scopes `read_user read_api read_repository` |
| Gitea | `<instance>/api/connections/gitea/callback` | self-hosted base URL required; create an OAuth2 app |
| Bitbucket | `<instance>/api/connections/bitbucket/callback` | permissions `account` + `repository` |

GitHub reuses the [third-party login](oauth-login/) OAuth app, but needs repository read access (`repo`).

Once configured, users can link accounts under **Profile → Connected accounts** and batch-import repositories from the import dialog.
