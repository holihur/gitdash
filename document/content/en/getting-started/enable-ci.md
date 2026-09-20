---
title: "Enable CI"
weight: 6
summary: "Add .gitdash.yml and run a pipeline on every push."
---

Add `.gitdash.yml` at the repository root:

```yaml
on: [push]
steps:
  - name: test
    run: go test ./...
```

Turn the switch on under repository **Settings → Pipeline**. Every push then produces a run on the **Pipeline** tab.

- Use `image:` in a step to run inside Docker, or `runs-on:` to target a self-hosted runner.
- Full syntax: [CI · Pipeline](../ci/pipeline/); runners: [CI · Runners](../ci/runners/).
