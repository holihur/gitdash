---
title: "Batch import"
weight: 3
summary: "After linking an account, select and import many repositories at once."
---

First [link a third-party account](../account/connected-accounts/) (GitHub / GitLab / Gitea / Bitbucket).

Then **Repositories → Import repository → From connected account**:

1. Choose a linked account.
2. Load the repositories it can access and filter as needed.
3. Select one or more repositories (select all / clear), and choose visibility.
4. Click **Import**; Gitdash creates and mirror-imports each repository asynchronously.

Notes:

- One failing repository does not affect the others; the result reports queued and skipped counts (already existing, invalid URL, …).
- Private repositories are accessed with the linked OAuth token, stored encrypted and used only for importing.
- At most 100 repositories per batch.
