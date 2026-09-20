---
title: "Pipeline configuration"
weight: 1
summary: ".gitdash.yml syntax, triggers, Docker steps and timeouts."
---

Add `.gitdash.yml` at the repository root (or multiple files under `.gitdash/*.yml`, each evaluated on its own triggers):

```yaml
on: [push, pull_request]
steps:
  - name: test
    image: golang:1.22
    run: go test ./...
  - name: build
    run: echo building
```

Highlights:

- `on:` supports `push` / `pull_request` / `manual` and more; multiple pipeline files are evaluated independently.
- `image:` runs a step inside that Docker image; omit it to run on the host (requires `GITDASH_PIPELINE_EXEC=host`).
- Supports parallel groups, `timeout` / `job_timeout`, manual and delayed triggers, cancel, re-run and artifacts.
- Turn it on under repository **Settings → Pipeline**.

The repository **Pipeline** tab shows the step DAG, run history and logs.
