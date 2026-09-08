# Private Package Registry

gitdash ships a built-in private package registry covering **npm (node)**, **composer (PHP)**, **pypi (Python)**, **rubygems (Ruby)**, **Go modules**, **cargo (Rust)** and **Maven (Java)**.

Common rules:

- Namespace: `/api/packages/{type}/{owner}/...` where `owner` is a **user or organization**
- Auth: package manager clients use **Basic auth** — your username + a **personal access token** (PAT, `repo` scope) as the password. Create one under *Settings → Keys → PAT*
- Read: any authenticated user of the instance; if a package is linked to a repo (via the `X-Gitdash-Repo` header on publish), the repo's visibility applies
- Publish / delete / yank: the namespace owner or org members with the **owner** role
- Storage: file contents are stored content-addressed on disk (`data/packages-blobs`), metadata / download counts / audit log in the DB
- Size limit: 64 MB per file

Below, replace `<owner>` with your username/org, `<user>:<PAT>` with your credentials, `your-host:8080` with your server address.

---

## 1. npm (node)

**Publish**

```bash
npm config set //your-host:8080/:_authToken <PAT>

npm publish --registry http://your-host:8080/api/packages/npm/<owner>/
```

**Install** (whole namespace, supports `@scope` packages)

```bash
npm install <pkg> --registry http://your-host:8080/api/packages/npm/<owner>/
```

Or pin it in the project `.npmrc`:

```ini
@myteam:registry=http://your-host:8080/api/packages/npm/<owner>/
//your-host:8080/:_authToken=<PAT>
```

Scoped packages are published as `npm publish` with the name `@myteam/foo` (the `@scope%2Ffoo` URL encoding is handled automatically).

**End-to-end example**

```bash
mkdir hello && cd hello
cat > package.json <<'EOF'
{ "name": "hello", "version": "1.0.0", "files": ["index.js"] }
EOF
echo 'module.exports = () => "hello";' > index.js

npm publish --registry http://your-host:8080/api/packages/npm/alice/

# verify in another project
npm install hello --registry http://your-host:8080/api/packages/npm/alice/
node -e "console.log(require('hello')())"   # -> hello
```

## 2. composer (PHP)

**Publish**

```bash
curl -u <user>:<PAT> -X PUT --data-binary @pkg.zip \
  "http://your-host:8080/api/packages/composer/<owner>/<vendor>/<name>?version=1.0.0"
```

**Install** — add the repository to the consumer project's `composer.json`:

```json
{
  "repositories": [
    { "type": "composer", "url": "http://<user>:<PAT>@your-host:8080/api/packages/composer/<owner>" }
  ]
}
```

```bash
composer require <vendor>/<name>
```

**End-to-end example**

```bash
mkdir hello-lib && cd hello-lib
cat > composer.json <<'EOF'
{ "name": "alice/hello-lib", "version": "1.0.0", "autoload": { "files": ["src/hello.php"] } }
EOF
mkdir -p src && echo '<?php function hello() { return "hello"; }' > src/hello.php
zip -r pkg.zip composer.json src

curl -u alice:PAT -X PUT --data-binary @pkg.zip \
  "http://your-host:8080/api/packages/composer/alice/alice/hello-lib?version=1.0.0"

# verify in another project (after adding the repository to composer.json)
composer require alice/hello-lib
php -r "require 'vendor/autoload.php'; echo hello();"   # -> hello
```

## 3. pypi (python)

**Publish** with twine:

```bash
python -m build
twine upload --repository-url "http://your-host:8080/api/packages/pypi/<owner>/" dist/*
```

**Install** (pip / uv):

```bash
pip config set global.index-url "http://<user>:<PAT>@your-host:8080/api/packages/pypi/<owner>/simple"

pip install <pkg>
# one-off:
pip install <pkg> --index-url "http://<user>:<PAT>@your-host:8080/api/packages/pypi/<owner>/simple"
```

**End-to-end example**

```bash
mkdir hello-py && cd hello-py
cat > pyproject.toml <<'EOF'
[project]
name = "hello-py"
version = "1.0.0"
EOF
echo 'def hello(): return "hello"' > hello.py

python -m pip install build twine
python -m build
twine upload --repository-url "http://your-host:8080/api/packages/pypi/alice/" dist/*

# verify
pip install hello-py --index-url "http://alice:PAT@your-host:8080/api/packages/pypi/alice/simple"
python -c "import hello; print(hello.hello())"   # -> hello
```

## 4. rubygems

**Publish**

```bash
gem push mygem-1.0.0.gem --host http://<user>:<PAT>@your-host:8080/api/packages/rubygems/<owner>
```

**Install / use in a Gemfile**

```bash
gem sources --add http://<user>:<PAT>@your-host:8080/api/packages/rubygems/<owner>/
gem install mygem
```

```ruby
# Gemfile
source "http://<user>:<PAT>@your-host:8080/api/packages/rubygems/<owner>/" do
  gem "mygem"
end
```

Direct download: `GET /api/packages/rubygems/<owner>/gems/<name>-<version>.gem` (Basic auth).

**End-to-end example**

```bash
gem build hello.gemspec        # mygem-1.0.0.gem
gem push mygem-1.0.0.gem --host http://alice:PAT@your-host:8080/api/packages/rubygems/alice

# verify
gem install mygem --source http://alice:PAT@your-host:8080/api/packages/rubygems/alice
ruby -e "require 'mygem'; puts Mygem.hello"   # -> hello
```

## 5. go (Go modules)

**Publish** — upload the module zip (`go mod tidy` first; build the zip with e.g. `goreleaser` or `go mod download -x` tooling; the zip must contain `<module>@<version>/go.mod`):

```bash
curl -u <user>:<PAT> -X PUT --data-binary @module.zip \
  "http://your-host:8080/api/packages/go/<owner>/<module>/@v/v1.0.0.zip"
```

`go.mod` is extracted from the zip automatically; `@v/list`, `@latest` and `.info` follow the standard GOPROXY protocol.

**Install**

```bash
go env -w GOPROXY="http://<user>:<PAT>@your-host:8080/api/packages/go/<owner>,direct"
GOFLAGS=-insecure go get <module>@v1.0.0   # plain-HTTP server; drop -insecure with HTTPS
```

**End-to-end example** — module `example.com/alice/hello`:

```bash
mkdir hello && cd hello
cat > go.mod <<'EOF'
module example.com/alice/hello

go 1.22
EOF
cat > hello.go <<'EOF'
package hello

func Hello() string { return "hello" }
EOF

# build the module zip (layout: <module>@<version>/...)
go mod download golang.org/x/mod 2>/dev/null || true
mkdir -p "pkg/example.com/alice/hello@v1.0.0"
cp go.mod hello.go "pkg/example.com/alice/hello@v1.0.0/"
(cd pkg && zip -r ../module.zip example.com)

curl -u alice:PAT -X PUT --data-binary @module.zip \
  "http://your-host:8080/api/packages/go/alice/example.com/alice/hello/@v/v1.0.0.zip"

# verify
go env -w GOPROXY="http://alice:PAT@your-host:8080/api/packages/go/alice,direct"
mkdir -p /tmp/usehello && cd /tmp/usehello && go mod init use && \
  GOFLAGS=-insecure go get example.com/alice/hello@v1.0.0
```

## 6. cargo (rust)

Configure a registry in the consumer project's `.cargo/config.toml`:

```toml
[registries.gitdash]
index = "sparse+http://your-host:8080/api/packages/cargo/<owner>/index/"

[source.gitdash]
registry = "sparse+http://your-host:8080/api/packages/cargo/<owner>/index/"
```

**Publish**

```bash
cargo login --registry gitdash      # paste the PAT when prompted
cargo publish --registry gitdash
```

**Install / depend**

```bash
cargo add <crate> --registry gitdash
```

**End-to-end example**

```bash
cargo new hello-lib && cd hello-lib
cat >> Cargo.toml <<'EOF'
[lib]
path = "src/lib.rs"
EOF
echo 'pub fn hello() -> &'static str { "hello" }' > src/lib.rs

cargo login --registry gitdash          # paste the PAT
cargo publish --registry gitdash

# verify in another project
cargo add hello-lib --registry gitdash
```

## 7. maven (java — Maven & Gradle)

**Maven** — add server credentials in `~/.m2/settings.xml`:

```xml
<servers>
  <server><id>gitdash</id><username><user></username><password><PAT></password></server>
</servers>
```

Deploy (`pom.xml` distributionManagement) / fetch:

```xml
<distributionManagement>
  <repository><id>gitdash</id><url>http://your-host:8080/api/packages/maven/<owner></url></repository>
</distributionManagement>

<repositories>
  <repository><id>gitdash</id><url>http://your-host:8080/api/packages/maven/<owner></url></repository>
</repositories>
```

```bash
mvn deploy
mvn dependency:get -DremoteRepositories=gitdash -Dartifact=com.example:mylib:1.0.0
```

**Gradle**

```groovy
repositories {
    maven {
        url "http://your-host:8080/api/packages/maven/<owner>"
        credentials { username '<user>'; password '<PAT>' }
    }
}
```

Publishing is path-based, so any build tool that can PUT works:

```bash
curl -u <user>:<PAT> -T mylib-1.0.0.jar \
  "http://your-host:8080/api/packages/maven/<owner>/com/example/mylib/1.0.0/mylib-1.0.0.jar"
```

Downloads use the same paths via `GET` (artifact-level `maven-metadata.xml` is supported).

**End-to-end example**

```bash
mkdir hello-java && cd hello-java
cat > pom.xml <<'EOF'
<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>com.example</groupId>
  <artifactId>hello</artifactId>
  <version>1.0.0</version>
  <packaging>jar</packaging>
  <distributionManagement>
    <repository><id>gitdash</id><url>http://your-host:8080/api/packages/maven/alice</url></repository>
  </distributionManagement>
</project>
EOF
mkdir -p src/main/java/com/example
echo 'package com.example; public class Hello { public static String hi() { return "hello"; } }' \
  > src/main/java/com/example/Hello.java

mvn deploy          # uses ~/.m2/settings.xml credentials (id: gitdash)

# verify
mvn dependency:get -DremoteRepositories=gitdash \
  -Dartifact=com.example:hello:1.0.0
```

---

## Management API

- `GET /api/packages/{owner}` / `GET /api/packages/{owner}/{type}` — list packages with download counts (Bearer session/PAT also works)
- `DELETE /api/packages/{type}/{owner}/{name}` — delete a package (all versions)
- `GET /api/packages/{owner}/audit` — publish / delete / yank / dist-tag audit log
- `cargo yank` → `DELETE/PUT /api/packages/cargo/{owner}/api/v1/crates/{crate}/{version}/yank`
- Maven: `maven-metadata.xml` is auto-generated from uploaded artifacts; `.sha1` / `.md5` checksum files are served automatically
- Link a package to a repo (visibility follows the repo): send `X-Gitdash-Repo: <repo>` on publish
- Web UI: *Packages* page in the header (list / delete / recent activity)
- `GET /api/openapi.json` — full endpoint docs
