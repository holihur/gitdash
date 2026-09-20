---
title: "Email replies"
weight: 4
summary: "Comment on issues / PRs by replying to notification emails."
---

Gitdash can turn an email reply to a notification into a comment on the matching
issue / pull request. It is **stateless**: the routing information is carried in a
signed `Reply-To` address, so no server-side token table is needed.

## How it works

1. When `GITDASH_MAIL_REPLY_DOMAIN` is set and a mail secret is configured, every
   notification email gets two extra headers:

   ```
   Message-ID: <gitdash.pull.acme.web.42.9f1c...@mail.example.com>
   Reply-To:   reply+<payload>.<sig>@mail.example.com
   ```

   The token is generated **per recipient**, so replying authenticates that user.

2. The recipient replies. The mail provider (or an MTA pipe) POSTs the parsed
   message to gitdash:

   ```
   POST /api/mail/inbound
   X-Gitdash-Mail-Secret: <shared secret>
   Content-Type: application/json

   {"to": "reply+<payload>.<sig>@mail.example.com",
    "from": "alice@example.com",
    "subject": "Re: [acme/web#42] Fix the thing",
    "text": "Looks good to me!\n\n> quoted previous message\n-- \nAlice"}
   ```

3. Gitdash verifies the token, cleans the body (drops `>` quotes, `-- `
   signatures and `On ... wrote:` attribution lines), creates the comment as the
   token's user, and fans out the usual inbox / webhook notifications.

## Threading & idempotency

Every comment is assigned a `Message-ID` when it is created, and the notification
email for that comment reuses the same value — so replies thread under the original
notification in any mail client. The MTA adapter should forward the reply's own
`Message-ID` and its `In-Reply-To` / `References` headers:

```json
{"to": "reply+<payload>.<sig>@mail.example.com",
 "from": "alice@example.com",
 "subject": "Re: [acme/web#42] Fix the thing",
 "text": "Looks good to me!",
 "message_id": "<reply-123@mail.example.com>",
 "in_reply_to": "<gitdash.issue.acme.web.42.9f1c@mail.example.com>",
 "references": "<gitdash...@mail.example.com>"}
```

Gitdash stores both on the comment (`message_id` / `in_reply_to`, visible in the
comments API) and uses `message_id` as an idempotency key: redelivering the same
message returns the existing comment (`200`) instead of creating a duplicate. When
`in_reply_to` is absent, the last token of `references` is used.

## Token format

- `payload = base64url(owner|repo|kind|number|username|exp)`, no padding.
- `sig = base64url(HMAC-SHA256(secret, payload)[:10])`, no padding.
- `exp` is a Unix timestamp; tokens are valid for 90 days.
- `kind` is `issue` or `pull`.

Only the holder of the secret can mint a token, and the address is only sent to
the intended recipient. As with any email-based flow, a forwarded email forwards
the ability to comment.

## Configuration

| Env var | Purpose |
| --- | --- |
| `GITDASH_MAIL_REPLY_DOMAIN` | Domain used in `Reply-To` / `Message-ID`. Unset = feature off. |
| `GITDASH_MAIL_SECRET` | HMAC key for reply tokens. Falls back to `GITDASH_SECRET_KEY`. Empty = no `Reply-To`. |
| `GITDASH_MAIL_INBOUND_SECRET` | Shared secret required by `POST /api/mail/inbound`. Falls back to `GITDASH_MAIL_SECRET`. Empty = endpoint disabled (401). |

The inbound endpoint is **not** behind user auth; it is meant to be called only by
the mail provider / MTA pipe. Keep it on a private network or behind your
provider's signature verification if possible.

## Mailgun inbound example

Create a route that stores the message and forwards it, or use a webhook that
maps Mailgun's fields to Gitdash's payload:

```python
# pseudo-handler: POST https://gitdash.example.com/api/mail/inbound
requests.post(url, headers={"X-Gitdash-Mail-Secret": SECRET}, json={
    "to": body["recipient"],
    "from": body["sender"],
    "subject": body["subject"],
    "text": body["stripped-text"] or body["body-plain"],
})
```

## Postfix `aliases` pipe example

Deliver the whole `reply+...@` domain to a script via a Postfix pipe transport:

```
# /etc/postfix/virtual
reply+@mail.example.com   gitdash-inbound
```

```ini
# /etc/postfix/master.cf
gitdash-inbound unix  -  n  n  -  -  pipe
  flags=DRhu user=gitdash argv=/usr/local/bin/gitdash-inbound.py ${recipient} ${sender}
```

The script parses the message (e.g. Python's `email` module), reads the plain
text body, and POSTs `{to, from, subject, text}` to `/api/mail/inbound`. Keep the
shared secret in a root-owned file and pass it via the header.

## Security notes

- The endpoint rejects requests without the shared secret (`401`), tokens with a
  bad signature or past `exp` (`400`), and unknown repositories (`404`).
- Only `issue` and `pull` targets are accepted; inline comments are not.
- Banned repos / orgs are rejected like everywhere else.
- Bodies are capped at 10000 characters after cleanup.
