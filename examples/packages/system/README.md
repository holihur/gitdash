# System package repositories (apt / yum / apk / brew / snap)

gitdash can host OS-level package repositories under a user or organization
namespace, next to the language registries. Unlike the language ecosystems,
system clients need an **index** (`Packages`, `repomd.xml`, `APKINDEX`, a
formula, …), so publishing takes the artifact **plus its metadata**:

```
POST /api/packages/{type}/{owner}/{repo}/publish
Authorization: Basic <owner>:<PAT>
Content-Type: multipart/form-data

  meta = {"name": "...", "version": "...", "arch": "...", "description": "...", ...}
  file = <the .deb / .rpm / .apk / bottle / .snap bytes>
```

The server stores the artifact and generates the native index on demand.
`examples/packages/publish.py` exercises all five types with plain HTTP.

## Publish

`meta` is JSON; the fields that matter per type:

| type | required | commonly used |
|---|---|---|
| `apt` | `name`, `version`, `arch` | `description`, `maintainer`, `depends`, `section`, `homepage` |
| `yum` | `name`, `version`, `release`, `arch` | `summary`, `description`, `license`, `provides`, `depends` |
| `apk` | `name`, `version`, `arch` | `description`, `license`, `depends`, `homepage`, `installed_size` |
| `brew` | `name`, `version`, `arch` | `description`, `homepage`, `license` |
| `snap` | `name`, `version`, `arch` | `description` |

```bash
HOST=http://127.0.0.1:8080
OWNER=alice
REPO=deb
PAT=gd_xxx

curl -u "$OWNER:$PAT" \
  -F 'meta={"name":"hello","version":"1.0.0","arch":"amd64","description":"hello"}' \
  -F file=@hello_1.0.0_amd64.deb \
  "$HOST/api/packages/apt/$OWNER/$REPO/publish"
```

## Install from the repository

The indexes are **unsigned**; point the native client at the repo and disable
signature checking (`[trusted=yes]`, `gpgcheck=0`, `--allow-untrusted`), which
is the usual self-hosted setup.

### apt (Debian / Ubuntu)

[`apt/sources.list`](apt/sources.list):

```
deb [trusted=yes] http://127.0.0.1:8080/api/packages/apt/alice/deb stable main
```

```bash
sudo cp apt/sources.list /etc/apt/sources.list.d/gitdash.list
sudo apt update && sudo apt install hello
```

### yum / dnf (RHEL / Fedora)

[`yum/gitdash.repo`](yum/gitdash.repo):

```
[gitdash]
name=gitdash alice/rpm
baseurl=http://127.0.0.1:8080/api/packages/yum/alice/rpm
enabled=1
gpgcheck=0
```

```bash
sudo cp yum/gitdash.repo /etc/yum.repos.d/gitdash.repo
sudo dnf install hello
```

### apk (Alpine)

[`apk/repositories`](apk/repositories):

```
http://127.0.0.1:8080/api/packages/apk/alice/alpine
```

```bash
sudo cp apk/repositories /etc/apk/repositories
sudo apk update && sudo apk add --allow-untrusted hello
```

### Homebrew

Homebrew taps are git repositories; point a tap at the gitdash-hosted formulas
or install a bottle directly from the generated formula API:

```bash
# formula JSON:
#   http://<host>/api/packages/brew/<owner>/<tap>/api/formula/<name>.json
brew install --formula <name>   # after wiring the tap
```

### snap

`snapd` talks to a store API, so gitdash exposes the published `.snap` files
and an index rather than impersonating the store:

```bash
curl -L http://127.0.0.1:8080/api/packages/snap/alice/store/index.json
curl -LO  http://127.0.0.1:8080/api/packages/snap/alice/store/download/hello_1.0.0_amd64.snap
sudo snap install --dangerous ./hello_1.0.0_amd64.snap
```

## Notes

- Packages default to the namespace visibility (private). Publish under a PAT
  with the `repo` scope and flip visibility to `public`/`anonymous` from the
  Packages page if the repo should be readable without login.
- `meta` may omit `md5`/`sha1`/`sha256`; the server computes them at publish
  time and embeds them in the generated indexes.
- Indexes are regenerated on every request, so a new upload is picked up
  immediately (no repodata refresh step).
