# 私有包仓库

gitdash 内置私有包仓库，覆盖 **npm (node)**、**composer (PHP)**、**pypi (Python)**、**rubygems (Ruby)**、**Go modules**、**cargo (Rust)** 与 **Maven (Java)**。

通用规则：

- 命名空间：`/api/packages/{type}/{owner}/...`，`owner` 为**用户或组织**
- 鉴权：包管理器客户端统一走 **Basic 认证** —— 用户名 + **个人访问令牌**（PAT，`repo` scope）作为密码。在「设置 → Keys → PAT」创建
- 读：本实例任意已认证用户；若包在发布时通过 `X-Gitdash-Repo` 头关联了仓库，则跟随该仓库可见性
- 发布 / 删除 / yank：命名空间 owner 本人或组织 **owner** 角色成员
- 存储：文件内容按内容寻址落盘（`data/packages-blobs`），元数据 / 下载计数 / 审计日志存 DB
- 单文件上限：64 MB

下文请将 `<owner>` 替换为用户名/组织名，`<user>:<PAT>` 替换为凭证，`your-host:8080` 替换为服务地址。

---

## 1. npm (node)

**发布**

```bash
npm config set //your-host:8080/:_authToken <PAT>

npm publish --registry http://your-host:8080/api/packages/npm/<owner>/
```

**安装**（整个命名空间，支持 `@scope` 包）

```bash
npm install <pkg> --registry http://your-host:8080/api/packages/npm/<owner>/
```

或在项目 `.npmrc` 固化：

```ini
@myteam:registry=http://your-host:8080/api/packages/npm/<owner>/
//your-host:8080/:_authToken=<PAT>
```

scope 包直接 `npm publish`（名字如 `@myteam/foo`，`@scope%2Ffoo` 的 URL 编码已自动处理）。

**端到端示例**

```bash
mkdir hello && cd hello
cat > package.json <<'EOF'
{ "name": "hello", "version": "1.0.0", "files": ["index.js"] }
EOF
echo 'module.exports = () => "hello";' > index.js

npm publish --registry http://your-host:8080/api/packages/npm/alice/

# 另一个项目中验证
npm install hello --registry http://your-host:8080/api/packages/npm/alice/
node -e "console.log(require('hello')())"   # -> hello
```

## 2. composer (PHP)

**发布**

```bash
curl -u <user>:<PAT> -X PUT --data-binary @pkg.zip \
  "http://your-host:8080/api/packages/composer/<owner>/<vendor>/<name>?version=1.0.0"
```

**安装** —— 在使用方的 `composer.json` 中添加仓库：

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

**端到端示例**

```bash
mkdir hello-lib && cd hello-lib
cat > composer.json <<'EOF'
{ "name": "alice/hello-lib", "version": "1.0.0", "autoload": { "files": ["src/hello.php"] } }
EOF
mkdir -p src && echo '<?php function hello() { return "hello"; }' > src/hello.php
zip -r pkg.zip composer.json src

curl -u alice:PAT -X PUT --data-binary @pkg.zip \
  "http://your-host:8080/api/packages/composer/alice/alice/hello-lib?version=1.0.0"

# 另一个项目中验证（先在 composer.json 加入仓库）
composer require alice/hello-lib
php -r "require 'vendor/autoload.php'; echo hello();"   # -> hello
```

## 3. pypi (python)

**发布**（twine）：

```bash
python -m build
twine upload --repository-url "http://your-host:8080/api/packages/pypi/<owner>/" dist/*
```

**安装**（pip / uv）：

```bash
pip config set global.index-url "http://<user>:<PAT>@your-host:8080/api/packages/pypi/<owner>/simple"

pip install <pkg>
# 一次性使用：
pip install <pkg> --index-url "http://<user>:<PAT>@your-host:8080/api/packages/pypi/<owner>/simple"
```

**端到端示例**

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

# 验证
pip install hello-py --index-url "http://alice:PAT@your-host:8080/api/packages/pypi/alice/simple"
python -c "import hello; print(hello.hello())"   # -> hello
```

## 4. rubygems

**发布**

```bash
gem push mygem-1.0.0.gem --host http://<user>:<PAT>@your-host:8080/api/packages/rubygems/<owner>
```

**安装 / Gemfile 使用**

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

直接下载：`GET /api/packages/rubygems/<owner>/gems/<name>-<version>.gem`（Basic 认证）。

**端到端示例**

```bash
gem build hello.gemspec        # 生成 mygem-1.0.0.gem
gem push mygem-1.0.0.gem --host http://alice:PAT@your-host:8080/api/packages/rubygems/alice

# 验证
gem install mygem --source http://alice:PAT@your-host:8080/api/packages/rubygems/alice
ruby -e "require 'mygem'; puts Mygem.hello"   # -> hello
```

## 5. go (Go modules)

**发布** —— 上传模块 zip（先 `go mod tidy`；zip 内需含 `<module>@<version>/go.mod`）：

```bash
curl -u <user>:<PAT> -X PUT --data-binary @module.zip \
  "http://your-host:8080/api/packages/go/<owner>/<module>/@v/v1.0.0.zip"
```

`go.mod` 会从 zip 中自动提取；`@v/list`、`@latest`、`.info` 遵循标准 GOPROXY 协议。

**安装**

```bash
go env -w GOPROXY="http://<user>:<PAT>@your-host:8080/api/packages/go/<owner>,direct"
GOFLAGS=-insecure go get <module>@v1.0.0   # 纯 HTTP 服务需 -insecure；HTTPS 可去掉
```

**端到端示例** —— 模块 `example.com/alice/hello`：

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

# 构造模块 zip（目录布局：<module>@<version>/...）
mkdir -p "pkg/example.com/alice/hello@v1.0.0"
cp go.mod hello.go "pkg/example.com/alice/hello@v1.0.0/"
(cd pkg && zip -r ../module.zip example.com)

curl -u alice:PAT -X PUT --data-binary @module.zip \
  "http://your-host:8080/api/packages/go/alice/example.com/alice/hello/@v/v1.0.0.zip"

# 验证
go env -w GOPROXY="http://alice:PAT@your-host:8080/api/packages/go/alice,direct"
mkdir -p /tmp/usehello && cd /tmp/usehello && go mod init use && \
  GOFLAGS=-insecure go get example.com/alice/hello@v1.0.0
```

## 6. cargo (rust)

在使用方项目的 `.cargo/config.toml` 中配置 registry：

```toml
[registries.gitdash]
index = "sparse+http://your-host:8080/api/packages/cargo/<owner>/index/"

[source.gitdash]
registry = "sparse+http://your-host:8080/api/packages/cargo/<owner>/index/"
```

**发布**

```bash
cargo login --registry gitdash      # 提示时粘贴 PAT
cargo publish --registry gitdash
```

**安装 / 引用**

```bash
cargo add <crate> --registry gitdash
```

**端到端示例**

```bash
cargo new hello-lib && cd hello-lib
echo 'pub fn hello() -> &'static str { "hello" }' > src/lib.rs

cargo login --registry gitdash          # 提示时粘贴 PAT
cargo publish --registry gitdash

# 另一个项目中验证
cargo add hello-lib --registry gitdash
```

## 7. maven (java — Maven & Gradle)

**Maven** —— 在 `~/.m2/settings.xml` 中添加凭证：

```xml
<servers>
  <server><id>gitdash</id><username><user></username><password><PAT></password></server>
</servers>
```

发布（`pom.xml` distributionManagement）/ 拉取：

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

发布为路径式，任何能发 PUT 的工具都可用：

```bash
curl -u <user>:<PAT> -T mylib-1.0.0.jar \
  "http://your-host:8080/api/packages/maven/<owner>/com/example/mylib/1.0.0/mylib-1.0.0.jar"
```

下载走同路径 `GET`（支持 artifact 级 `maven-metadata.xml`）。

**端到端示例**

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

mvn deploy          # 使用 ~/.m2/settings.xml 中的凭证（id: gitdash）

# 验证
mvn dependency:get -DremoteRepositories=gitdash \
  -Dartifact=com.example:hello:1.0.0
```

---

## 管理 API

- `GET /api/packages/{owner}` / `GET /api/packages/{owner}/{type}` — 列出包（含下载计数；Bearer session/PAT 亦可）
- `DELETE /api/packages/{type}/{owner}/{name}` — 删除包（全版本）
- `GET /api/packages/{owner}/audit` — 发布 / 删除 / yank / dist-tag 审计日志
- `cargo yank` → `DELETE/PUT /api/packages/cargo/{owner}/api/v1/crates/{crate}/{version}/yank`
- Maven：`maven-metadata.xml` 依据已上传制品自动生成；`.sha1` / `.md5` 校验文件自动提供
- 包关联仓库（可见性跟随仓库）：发布时带 `X-Gitdash-Repo: <repo>` 头
- Web UI：顶部导航「包仓库」页（列表 / 删除 / 最近操作）
- `GET /api/openapi.json` — 完整端点文档
