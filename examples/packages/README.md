# Package registry examples

Runnable examples for the gitdash [private package registry](../../docs/packages.md):
one minimal project per ecosystem plus a stdlib-only script that publishes each
one to a live instance and verifies the download.

## Quick start (no extra toolchains)

The script talks to the registry over plain HTTP with a personal access token,
so it works without `npm` / `cargo` / `mvn` / ... installed. It is also a handy
manual end-to-end test of a running server.

1. Create a **PAT** with the `repo` scope: *Settings → Keys → PAT*.
2. Run:

```bash
export GITDASH_URL=http://127.0.0.1:8080   # your instance
export GITDASH_USER=alice                  # owner (user or org) to publish under
export GITDASH_PAT=gd_xxx                  # the PAT from step 1

python3 examples/packages/publish.py
```

Expected output:

```
publishing examples to http://127.0.0.1:8080 as alice
  ok  npm
  ok  pypi
  ok  composer
  ok  cargo
  ok  go
  ok  rubygems
  ok  maven

7/7 registries passed
```

Every run publishes a unique `*-<hex>` name so it can be repeated safely. The
Docker / OCI registry is not covered by the script (it needs a real image
client); use the example below.

## Native tooling

The example projects are buildable with the real package managers. Replace
`<host>`, `<owner>` and `<PAT>` with your values.

| Registry | Project | Publish |
|---|---|---|
| npm | [`npm/hello/`](npm/hello/) | `npm publish --registry http://<host>/api/packages/npm/<owner>/` |
| pypi | [`pypi/hello-py/`](pypi/hello-py/) | `python -m build && twine upload --repository-url http://<host>/api/packages/pypi/<owner>/ dist/*` |
| composer | [`composer/hello-lib/`](composer/hello-lib/) | `curl -u <owner>:<PAT> -X PUT --data-binary @pkg.zip "http://<host>/api/packages/composer/<owner>/<vendor>/<name>?version=1.0.0"` |
| cargo | [`cargo/hello-lib/`](cargo/hello-lib/) | `cargo publish --registry gitdash` (after configuring `.cargo/config.toml`) |
| go | [`go/hello/`](go/hello/) | Upload the `<module>@<version>` zip to `http://<host>/api/packages/go/<owner>/<module>/@v/v1.0.0.zip` |
| rubygems | [`rubygems/hello/`](rubygems/hello/) | `gem build hello.gemspec && gem push hello-1.0.0.gem --host http://<owner>:<PAT>@<host>/api/packages/rubygems/<owner>` |
| maven | [`maven/hello/`](maven/hello/) | `mvn deploy` (with the `gitdash` server in `~/.m2/settings.xml`) |
| docker / OCI | [`docker/`](docker/) | `docker build -t hello:1.0 . && docker tag hello:1.0 <host>/<owner>/hello:1.0 && docker push <host>/<owner>/hello:1.0` |

Authenticate the clients with **Basic auth**: your username as the user and a
**PAT** (not your password) as the password. npm uses `_authToken` in `.npmrc`,
cargo uses `cargo login --registry gitdash`, pip/uv embed the credentials in the
index URL. Full, copy-pasteable commands and `.npmrc` / `Cargo.toml` /
`settings.xml` snippets live in [`docs/packages.md`](../../docs/packages.md).

## Layout

```
packages/
├── publish.py                 # stdlib-only publish + verify script
├── npm/hello/                 # package.json + index.js
├── pypi/hello-py/             # pyproject.toml + src/hello_py/
├── composer/hello-lib/        # composer.json + src/hello.php
├── cargo/hello-lib/           # Cargo.toml + src/lib.rs
├── go/hello/                  # go.mod + hello.go
├── rubygems/hello/            # hello.gemspec + lib/hello.rb
├── maven/hello/               # pom.xml + src/main/java/com/example/Hello.java
└── docker/                    # Dockerfile + hello.sh
```
