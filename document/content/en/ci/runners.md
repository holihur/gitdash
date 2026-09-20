---
title: "Self-hosted runners"
weight: 2
summary: "Deploy gitdash-runner and target it with runs-on."
---

Deploy a self-hosted runner when you need more compute or a private environment:

1. Create a registration token on the **Runners** page (user, organization or global scope).
2. Run `gitdash-runner` on the target machine, using the token to connect.
3. Use `runs-on:` in `.gitdash.yml` to target matching labels.

Supports streaming workspace snapshots and logs, cancellation and offline detection.

See `docs/runners.md` in the repository.
