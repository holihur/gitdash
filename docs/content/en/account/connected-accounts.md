---
title: "Connected accounts"
weight: 6
summary: "Link GitHub / GitLab / Gitea / Bitbucket to import repositories in bulk."
---

Under **Profile → Connected accounts** you can link third-party code hosting accounts:

- **GitHub**
- **GitLab** (gitlab.com or self-hosted)
- **Gitea / Forgejo**
- **Bitbucket**

Linking uses OAuth. Gitdash stores the access token (encrypted) and uses it only to **list and import your repositories**. You can unlink at any time; unlinking deletes the token.

## Prerequisite

An administrator must enable and configure the OAuth application for the platform under **Admin → Settings** (Client ID / Secret and callback URL). See [Admin · Connected accounts](../admin/connected-accounts/).

## After linking

Go to **Repositories → Import repository → From connected account**, select repositories and import them in bulk — see [Batch import](../repositories/batch-import/).
