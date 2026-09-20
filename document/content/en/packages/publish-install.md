---
title: "Publish and install"
weight: 1
summary: "Registry URLs and authentication for each ecosystem."
---

Supported ecosystems and basic usage:

- **npm**: `npm publish --registry <instance>/api/packages/<owner>/npm/`
- **composer**: point `repositories` at the instance.
- **pypi**: `twine upload --repository-url <instance>/api/packages/<owner>/pypi/`
- **rubygems**: `gem push --host <instance>/api/packages/<owner>/rubygems/`
- **Go modules**: `GOPROXY=<instance>/api/packages/<owner>/go`
- **cargo**: point the registry at the instance.
- **Maven**: configure `distributionManagement`.

Authentication: any username, PAT as the password.

See `docs/packages.md` and `examples/packages/` in the repository.
