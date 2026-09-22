---
title: "Set up SSH"
weight: 3
summary: "Generate a key pair, add the public key and clone over SSH."
---

SSH is the recommended way to access repositories.

## 1. Generate a key (if you have none)

```bash
ssh-keygen -t ed25519 -C "you@example.com"
```

## 2. Add the public key

Open the **SSH Keys** page and paste the contents of `~/.ssh/id_ed25519.pub`.

## 3. Test

```bash
ssh -T -p 2222 git@<your-instance>
```

## Clone URL

The repository page shows an SSH clone URL like:

```bash
git clone ssh://git@<host>:2222/<owner>/<repo>.git
```

> You can also use HTTPS with a personal access token (PAT): any username and the PAT as the password. Create one on the **Tokens** page.

## Next

- [First push](first-push/)
