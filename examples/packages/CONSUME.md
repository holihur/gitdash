# Consuming the private packages

Consumer-side examples for the gitdash [private package registry](https://holihur.github.io/gitdash/packages/publish-install/):
each ecosystem has a `consume/` project next to its publisher project, showing
how to point the native client at your instance and install the package.

Replace the placeholders in each file:

| Placeholder | Meaning | Example |
|---|---|---|
| `<host>` | instance host **with port** | `git.example.com:8080` |
| `<owner>` | user/org namespace the package was published under | `alice` |
| `<user>` | your username (for read auth) | `bob` |
| `<PAT>` | personal access token, `repo` scope | `gd_xxx` |

Read access: any authenticated user of the instance can install a package (if a
package is linked to a private repo, the repo's visibility applies).

## Verify without toolchains

`consume.py` lists a namespace's packages and pulls each one back through the
protocol the real client uses, then prints the install command. Only the Python
standard library is required:

```bash
export GITDASH_URL=http://127.0.0.1:8080
export GITDASH_USER=bob            # any user allowed to read
export GITDASH_PAT=<repo-scoped PAT>
export GITDASH_OWNER=alice         # namespace to consume (default: GITDASH_USER)

python3 examples/packages/consume.py
#   ok  cargo     cargo add hello-lib --registry gitdash
#   ok  composer  composer require example/hello-lib
#   ...
#   7/7 registries consumed
```

## Native clients

| Registry | Consumer project | Install |
|---|---|---|
| npm | [`npm/consume/`](npm/consume/) | `npm install` (with the `.npmrc` pointing at the registry) |
| pypi | [`pypi/consume/`](pypi/consume/) | `pip install hello-py --index-url ...` |
| composer | [`composer/consume/`](composer/consume/) | `composer install` |
| cargo | [`cargo/consume/`](cargo/consume/) | `cargo run` |
| go | [`go/consume/`](go/consume/) | `go run .` (with `GOPROXY` set) |
| rubygems | [`rubygems/consume/`](rubygems/consume/) | `bundle install` |
| maven | [`maven/consume/`](maven/consume/) | `mvn -s settings.xml dependency:resolve` |
| docker / OCI | [`docker/consume/`](docker/consume/) | `docker login <host>` then `docker build` |

Each directory has its own comments with the exact one-off setup (the
`GOPROXY`/`GOINSECURE` env, the `.cargo/config.toml` + `cargo login`, the
`~/.m2/settings.xml` server, ...).

> Plain-HTTP registries must be reachable over HTTPS or listed under
> `insecure-registries` in `/etc/docker/daemon.json`. `127.0.0.1` is always
> allowed over HTTP.
