---
title: "AI copilot"
weight: 9
summary: "Work with an AI agent in a repository checkout; changes are committed to a session branch."
---

A BYOK AI agent that can read, edit and run commands. After every turn Gitdash commits and pushes its changes to a `copilot/session-<id>` branch.

{{% notice warning %}}
**Copilot is disabled by default.** Its agent runtime executes shell commands directly on the host, so anyone with `write` access to any repository could otherwise escalate to remote code execution under the Gitdash service account. Set `GITDASH_COPILOT=1` (or `true`) to opt in, and only run it on instances where repository write access is limited to trusted users. When enabled, only repository write access is required to start a session.
{{% /notice %}}
