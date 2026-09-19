# gitdash examples

Small, runnable projects that demonstrate gitdash features against a live
instance.

| Example | What it shows | How to run |
|---|---|---|
| [`packages/`](packages/) | Publish to every private package registry (npm, pypi, composer, cargo, go, rubygems, maven, docker/OCI) | `python3 examples/packages/publish.py` |
| [`packages/consume/`](packages/consume/) | Consume the private packages from each ecosystem (`.npmrc`, `pip.conf`, `.cargo/config.toml`, `Gemfile`, `settings.xml`, ...) | `python3 examples/packages/consume/consume.py` |

See the per-example README for the exact environment variables and the native
tooling commands (for copy-pasting into a real project).
