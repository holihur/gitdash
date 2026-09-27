---
title: "CI pipelines"
weight: 7
summary: "Define steps in .gitdash.yml and run them in Docker or on self-hosted runners."
---

Per-repository CI: triggered by push, with logs stored per run.

{{% notice warning %}}
**The built-in executor is disabled by default.** To run pipelines on the
gitdash host you must explicitly opt in:

```bash
GITDASH_PIPELINE_EXEC=docker      # or host (no container sandbox)
GITDASH_DISABLE_REGISTRATION=1    # required while docker/host mode is on
GITDASH_PIPELINE_IMAGES=alpine:3.19,golang:1.22   # required image allowlist
```

The image allowlist **denies all images when unset** (default-deny); host mode
(`image` omitted) is the only exception. Leave `GITDASH_PIPELINE_EXEC` unset to
keep the executor off. Self-hosted runners are unaffected.
{{% /notice %}}
