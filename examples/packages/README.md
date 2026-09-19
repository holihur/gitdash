# Package registry examples

Runnable examples for the gitdash [private package registry](../../docs/packages.md).
Each ecosystem directory pairs a **publisher** project with a **consumer**
project, so you can see the full round trip:

```
npm/hello/      <- publish      npm/consume/      <- install
pypi/hello-py/  <- publish      pypi/consume/     <- install
...
```

Two stdlib-only scripts drive the whole thing without any package manager
installed: `publish.py` (write side) and `consume.py` (read side).

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

Then pull them back the same way (read side):

```bash
export GITDASH_OWNER=alice   # namespace to consume (defaults to GITDASH_USER)
python3 examples/packages/consume.py

#   7/7 registries consumed
```

### One command for both layers

`e2e.py` runs the stdlib scripts **and** a real publish + install per ecosystem,
using the native client when it is on `PATH` (npm, cargo, pip). Missing tools
are reported as `skip`, so it is safe in CI:

```bash
export GITDASH_URL=http://127.0.0.1:8080 GITDASH_USER=alice GITDASH_PAT=<PAT>
python3 examples/packages/e2e.py

#   ok   publish.py + consume.py
#   ok   npm (native)
#   ok   cargo (native)
#   ok   pypi (native)
#   skip go (native) — go client needs HTTPS to send credentials
```

The standard-library round trip is also exercised by
[`tests/test_examples.py`](../../tests/test_examples.py), which runs in CI.

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

The read side (installing these packages from a project) is covered by the
`consume/` project next to each publisher — see
[CONSUME.md](CONSUME.md) for the native client commands.

## Layout

```
packages/
├── README.md
├── CONSUME.md                 # consumer-side setup per ecosystem
├── publish.py                 # stdlib-only publish + verify script
├── consume.py                 # stdlib-only consume (read) script
├── e2e.py                     # runs both layers + native clients
├── npm/
│   ├── hello/                 # package.json + index.js
│   └── consume/               # package.json + .npmrc
├── pypi/
│   ├── hello-py/              # pyproject.toml + src/hello_py/
│   └── consume/               # requirements.txt + pip.conf
├── composer/
│   ├── hello-lib/             # composer.json + src/hello.php
│   └── consume/               # composer.json
├── cargo/
│   ├── hello-lib/             # Cargo.toml + src/lib.rs
│   └── consume/               # Cargo.toml + .cargo/config.toml + src/
├── go/
│   ├── hello/                 # go.mod + hello.go
│   └── consume/               # go.mod + main.go
├── rubygems/
│   ├── hello/                 # hello.gemspec + lib/hello.rb
│   └── consume/               # Gemfile
├── maven/
│   ├── hello/                 # pom.xml + src/main/java/com/example/Hello.java
│   └── consume/               # pom.xml + settings.xml
└── docker/
    ├── Dockerfile + hello.sh  # build/push the image
    └── consume/               # FROM <host>/<owner>/hello:1.0
```
