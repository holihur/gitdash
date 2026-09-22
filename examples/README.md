# gitdash examples

Small, runnable projects that demonstrate gitdash features against a live
instance.

| Example | What it shows | How to run |
|---|---|---|
| [`packages/`](packages/) | Publish to every private package registry (npm, pypi, composer, cargo, go, rubygems, maven, docker/OCI) plus the apt / yum / apk / brew / snap system repositories, with a `consume/` project next to each publisher | `python3 examples/packages/publish.py` |
| [`packages/system/`](packages/system/) | Native client configs to install from a gitdash-hosted apt / yum / apk repository (and brew formula / snap index) | see the README |
| [`packages/CONSUME.md`](packages/CONSUME.md) | Consume the private packages from each ecosystem (`.npmrc`, `pip.conf`, `.cargo/config.toml`, `Gemfile`, `settings.xml`, ...) | `python3 examples/packages/consume.py` |
| [`packages/e2e.py`](packages/e2e.py) | Both layers plus the native clients (npm / cargo / pip), skipping tools that are not installed | `python3 examples/packages/e2e.py` |

See the per-example README for the exact environment variables and the native
tooling commands (for copy-pasting into a real project).
