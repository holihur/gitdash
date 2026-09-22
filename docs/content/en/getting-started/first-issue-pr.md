---
title: "Issues and pull requests"
weight: 5
summary: "File an issue, branch, open a PR, review and merge."
---

## Create an issue

Open the repository **Issues** tab, click **New issue**, add a title and body, and optionally labels and a milestone.

## Open a pull request

1. Create a branch and commit:

   ```bash
   git checkout -b feature/x
   git commit -am "feat: x"
   git push -u origin feature/x
   ```

2. Open a PR on the repository page, choosing base (target) and head (source).
3. Add reviewers and a description, then submit.
4. Once approved, merge (for example with **Squash merge**).

## Email patches (optional)

Gitdash also supports a `git send-email`-style patch and email reply workflow — see [PR · Email patches](../pulls/email-patches/).

## Next

- [Enable CI](enable-ci/)
