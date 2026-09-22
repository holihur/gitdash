---
title: "Sign up and sign in"
weight: 1
summary: "Create an account, sign in, and use third-party login."
---

## Sign up

Open the instance home page and click **Register**:

1. Pick a username (4–32 chars: lowercase letters, digits, `_` or `-`, starting with a letter or digit).
2. Choose a password (at least 8 chars with 3 of: lowercase / uppercase / digit / symbol).
3. Optionally add an email (used for verification, notifications and password reset).

A profile repository `<username>/<username>` is created automatically (disable with `GITDASH_PROFILE_REPO=0`).

## Sign in

Sign in with username + password. If the administrator configured third-party login, the login page also offers:

- GitHub
- Google
- Generic OIDC (GitLab, Keycloak, Azure AD, …)

> On first third-party login, if a local account with the same name already exists, you must sign in to that local account first to complete the link. This prevents account takeover.

## Forgot password

Click **Forgot password** on the login page, enter your registered email and follow the link in the email. The administrator must configure SMTP first.

## Next

- [Create your first repository](first-repository/)
- [Set up SSH](connect-ssh/)
