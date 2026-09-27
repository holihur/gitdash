---
title: "Upgrade"
weight: 5
summary: "Manual updates and optional auto-update."
---

```bash
gitdash update
```

Optional background auto-update is off by default. Back up before upgrading.

## Database migrations

Schema changes are tracked in a `schema_meta` table. Destructive migrations
(legacy `DROP TABLE` / `DROP INDEX`) are **refused by default** and require
explicit confirmation:

```bash
gitdash backup                                   # take a snapshot first
GITDASH_ALLOW_DESTRUCTIVE_MIGRATION=1 gitdash serve
```

Use `GITDASH_MIGRATE_DRY_RUN=1 gitdash serve` to only print the pending
destructive steps and exit without changing anything.

## Release integrity

Official releases ship `checksums.txt` plus a detached `checksums.txt.minisig`
signature. The updater always verifies SHA256, and verifies the minisign
signature of `checksums.txt` whenever a public key is available:

- official binaries embed the maintainer public key at build time;
- self-built binaries can set `GITDASH_UPDATE_MINISIGN_PUBKEY` (base64 public
  key) or `GITDASH_UPDATE_REQUIRE_SIGNATURE=1` to require a valid signature.

The `install.sh` / `install.ps1` scripts download `checksums.txt` and verify the
SHA256 of the archive before installing. Set `GITDASH_MINISIGN_PUBKEY` to also
require the minisign signature (needs the `minisign` tool).
