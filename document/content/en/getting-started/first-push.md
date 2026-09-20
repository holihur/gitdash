---
title: "First push"
weight: 4
summary: "Initialize a local repository and push it to gitdash."
---

```bash
git init
git add .
git commit -m "first commit"
git branch -M main
git remote add origin ssh://git@<host>:2222/<owner>/<repo>.git
git push -u origin main
```

After pushing:

- The default branch is updated (change it in repository settings).
- If pipelines are enabled, the push triggers the CI defined in `.gitdash.yml`.
- Watchers receive the activity in their inbox.

## Next

- [Issues and pull requests](first-issue-pr/)
- [Enable CI](enable-ci/)
