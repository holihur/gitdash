---
title: "Passkeys (WebAuthn)"
weight: 6
summary: "Passwordless sign-in with Touch ID, Windows Hello, security keys or a password manager."
---

Passkeys let you sign in without a password using the WebAuthn/FIDO2 standard.

## Register a passkey

1. Open **Profile → Passkeys**.
2. Click **Add passkey**, give it a name (e.g. `MacBook`, `YubiKey`).
3. Follow your browser's prompt (Touch ID, Windows Hello, a hardware security key or your password manager).

You can register multiple passkeys and remove them at any time from the same card.

## Sign in with a passkey

On the sign-in page click **Sign in with a passkey** and confirm with your authenticator.
Passkeys are discoverable, so you do not need to type a username first.

## Self-hosted deployment notes

WebAuthn binds credentials to a domain (the *Relying Party ID*) and an origin. gitdash derives
them from the request by default, which works for typical HTTPS deployments. Behind a reverse
proxy or on a non-standard domain, set:

| Variable | Description |
| --- | --- |
| `GITDASH_WEBAUTHN_RPID` | Relying Party ID, e.g. `git.example.com` (must be a domain, not an IP) |
| `GITDASH_WEBAUTHN_ORIGINS` | Comma-separated allowed origins, e.g. `https://git.example.com` |
| `GITDASH_WEBAUTHN_RP_NAME` | Display name shown in the browser prompt (default `gitdash`) |

Passkeys require a secure context: HTTPS, or `http://localhost` during local development.
