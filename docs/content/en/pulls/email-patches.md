---
title: "Email patches"
weight: 3
summary: "Receive git send-email patches and turn them into PRs."
---

Gitdash can turn an emailed patch series into a pull request — the
`git send-email` / `git format-patch` workflow. A contributor produces an mbox;
Gitdash applies it to a fresh branch and opens a PR.

## Usage

```sh
git format-patch -1 --stdout | \
  curl -X POST "https://gitdash.example.com/api/users/acme/repos/web/patches" \
       -H "Authorization: Bearer $GITDASH_PAT" \
       -H "Content-Type: text/plain" --data-binary @-
```

The server runs `git am --3way` on top of the target branch in a temporary
clone, pushes a new `patches/<timestamp>` branch and creates a normal PR whose
target is the repository's default branch (override with `?target=<branch>`).
The PR title comes from the first patch's `Subject:` (override with `?title=`);
the PR body lists every patch subject in the series.

## Submitting by email

The same flow is available by email, so `git send-email` works without a PAT:

```
git send-email --to="patches+acme+web@mail.example.com" *.patch
```

- The address format is `patches+<owner>+<repo>@<GITDASH_MAIL_REPLY_DOMAIN>`.
- The subject must contain `[PATCH` (e.g. `git format-patch` default).
- The sender (`From`) is matched to a user by email; that user must have a
  **verified email** and **write access** to the repository. Unknown senders get
  `400 unknown_sender`, missing permission `404`, unverified `403`.
- The endpoint is the shared-secret `POST /api/mail/inbound` from
  [reply by email](../email-replies/); MTA adapters should forward the raw mbox
  as `text`.

## API

| | |
| --- | --- |
| Method | `POST` |
| Path | `/api/users/{owner}/repos/{name}/patches` |
| Auth | PAT / session with write access to the repo |
| Body | mbox (`text/plain`, up to 20 MiB) |
| Query | `target` (default: repo default branch), `title` |

Responses:

- `201` with the created `PullRequest`.
- `400 empty_patch` — empty request body.
- `400 patch_failed` — `git am` could not apply the series (bad base, conflict, …).
- `401` — unauthenticated.
- `404` — no write access / repo not found.

## Notes

- Multiple patches concatenated into one mbox are applied in order; `git am`
  commits each with the author from the patch (committer is `gitdash`).
- A failed series is aborted and leaves no branch behind.
- The resulting PR is indistinguishable from one opened in the UI, so branch
  protection, CI and merge gates all apply.
- The first patch's commit metadata is preserved (author name/email); committer
  identity is the Gitdash service account.

## Example

```sh
# create two commits locally, then submit both as one series
git format-patch -2 --stdout > series.mbox
curl -X POST "https://gitdash.example.com/api/users/acme/repos/web/patches?title=Fix+widgets" \
     -H "Authorization: Bearer $GITDASH_PAT" \
     -H "Content-Type: text/plain" --data-binary @series.mbox
```
