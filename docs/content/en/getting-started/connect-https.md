---
title: "Clone over HTTPS"
weight: 4
summary: "Use a personal access token (PAT) to clone and push over HTTPS, GitHub/GitLab style."
---

Besides SSH, repositories are served over **Git Smart HTTP**. A clone URL looks
like any other Git host:

```bash
git clone https://<host>/<owner>/<repo>.git
```

## 1. Create a personal access token

Open the **Tokens** page and create a PAT with the **repo** scope. Copy it — it
is shown only once.

## 2. Clone / push with the token

When Git asks for credentials, use the PAT as the **password**. The username can
be anything (GitHub/GitLab style) — the token identifies you:

```bash
git clone https://<username>:<PAT>@<host>/<owner>/<repo>.git
```

or let Git prompt and paste the PAT when asked:

```bash
git clone https://<host>/<owner>/<repo>.git
# Username: anything
# Password: <PAT>
```

Push works the same way once the remote is configured with the token.

## 3. Avoid retyping the token

Use a credential helper, e.g.:

```bash
git config --global credential.helper store
```

Your shell history / credential file now contains the token — treat it like a
password. Prefer a **read-only** or short-lived PAT where possible.

## Access rules

- **Push** requires write access (owner, collaborator or org member).
- **Clone/fetch** follows the repository visibility:
  - `private` — only you and collaborators;
  - `public` — any signed-in user;
  - `anonymous` — anyone, no sign-in required.

## Notes

- Deploy keys are SSH-only; over HTTPS use a PAT.
- The token is sent as HTTP Basic auth, so always use **HTTPS** in production.

## Next

- [First push](first-push/)
