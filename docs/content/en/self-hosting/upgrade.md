---
title: "Upgrade"
weight: 5
summary: "Manual updates and optional auto-update."
---

```bash
gitdash update
```

Optional background auto-update is off by default. Back up before upgrading.

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
