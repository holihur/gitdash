# Consuming the private packages

Consumer-side examples for the gitdash [private package registry](../../../docs/packages.md):
one project per ecosystem showing how to point the native client at your
instance and install a package. The matching publisher projects live one level
up in [`packages/`](../).

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

python3 examples/packages/consume/consume.py
#   ok  cargo     cargo add hello_lib_xxx --registry gitdash
#   ok  composer  composer require alice/hello-xxx
#   ...
#   7/7 registries consumed
```

## Native clients

### npm — [`npm/`](npm/)

`.npmrc` points npm at the registry and supplies the PAT:

```ini
registry=http://<host>/api/packages/npm/<owner>/
//<host>/:_authToken=<PAT>
```

```bash
cd examples/packages/consume/npm && npm install
```

### pypi — [`pypi/`](pypi/)

Credentials go in the index URL (or use `pip.conf`):

```bash
pip install hello-py \
  --index-url http://<user>:<PAT>@<host>/api/packages/pypi/<owner>/simple \
  --trusted-host <host>
```

### composer — [`composer/`](composer/)

```bash
cd examples/packages/consume/composer && composer install
```

### cargo — [`cargo/`](cargo/)

`.cargo/config.toml` registers the sparse index; `Cargo.toml` depends on the
crate through it:

```bash
cd examples/packages/consume/cargo && cargo run
```

### go — [`go/`](go/)

```bash
cd examples/packages/consume/go
go env -w GOPROXY="http://<user>:<PAT>@<host>/api/packages/go/<owner>,direct"
GOFLAGS=-insecure go run .     # -insecure only needed for plain HTTP
```

### rubygems — [`rubygems/`](rubygems/)

```bash
cd examples/packages/consume/rubygems && bundle install
```

### maven — [`maven/`](maven/)

`pom.xml` declares the repository + dependency, `settings.xml` holds the
credentials for `<repository id="gitdash">`:

```bash
cd examples/packages/consume/maven && mvn -s settings.xml dependency:resolve
```

### docker / OCI — [`docker/`](docker/)

```bash
docker login <host> -u <user> -p <PAT>
docker build -t consume-docker examples/packages/consume/docker
```

> Plain-HTTP registries must be reachable over HTTPS or listed under
> `insecure-registries` in `/etc/docker/daemon.json`. `127.0.0.1` is always
> allowed over HTTP.

## Layout

```
consume/
├── consume.py            # stdlib-only read-side verifier
├── npm/                  # package.json + .npmrc
├── pypi/                 # requirements.txt + pip.conf
├── composer/             # composer.json
├── cargo/                # Cargo.toml + .cargo/config.toml + src/
├── go/                   # go.mod + main.go
├── rubygems/             # Gemfile
├── maven/                # pom.xml + settings.xml
└── docker/               # Dockerfile
```
