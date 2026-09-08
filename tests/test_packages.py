"""私有包注册表黑盒测试（npm / pypi / go / cargo / rubygems / composer / maven）。

所有包管理器客户端走 Basic 认证（密码 = PAT），测试覆盖：
- 未认证 401
- 非 owner 发布 403
- 发布 → 元数据/索引 → 下载闭环
"""
from __future__ import annotations

import base64
import gzip
import io
import json
import tarfile
import uuid

import pytest
import requests


@pytest.fixture
def pkg_user(user_factory, client_factory):
    """注册用户 + repo scope PAT，返回 (username, pat_token, client)。"""
    username, token, client = user_factory("pkg")
    resp = client.post("/tokens", json={"name": "pkg", "scopes": ["repo"]}, expect=201)
    return username, resp.json()["token"], client_factory(resp.json()["token"])


def basic(username: str, pat: str) -> dict:
    creds = base64.b64encode(f"{username}:{pat}".encode()).decode()
    return {"Authorization": f"Basic {creds}"}


# ---- 通用 ----


def test_packages_require_auth(base_url, pkg_user):
    username, _, _ = pkg_user
    r = requests.get(f"{base_url}/api/packages/npm/{username}/some-pkg", timeout=10)
    assert r.status_code == 401


def test_publish_forbidden_for_other_user(base_url, pkg_user, user_factory):
    username, pat, _ = pkg_user
    other, _, _ = user_factory("pkg")
    r = requests.put(
        f"{base_url}/api/packages/npm/{other}/lib",
        json={"name": "lib", "versions": {"1.0.0": {}}, "_attachments": {}},
        headers=basic(username, pat),
        timeout=10,
    )
    assert r.status_code == 403


# ---- npm ----


def test_npm_publish_packument_tarball(base_url, pkg_user):
    username, pat, _ = pkg_user
    pkg = f"mypkg-{uuid.uuid4().hex[:8]}"
    tarball = b"fake-tgz-content"
    body = {
        "_id": pkg,
        "name": pkg,
        "versions": {"1.0.0": {"name": pkg, "version": "1.0.0"}},
        "_attachments": {f"{pkg}-1.0.0.tgz": {"length": len(tarball), "data": base64.b64encode(tarball).decode()}},
    }
    r = requests.put(
        f"{base_url}/api/packages/npm/{username}/{pkg}",
        json=body, headers=basic(username, pat), timeout=10,
    )
    assert r.status_code == 201

    meta = requests.get(
        f"{base_url}/api/packages/npm/{username}/{pkg}",
        headers=basic(username, pat), timeout=10,
    ).json()
    assert meta["name"] == pkg
    assert meta["dist-tags"]["latest"] == "1.0.0"
    tar_url = meta["versions"]["1.0.0"]["dist"]["tarball"]
    dl = requests.get(tar_url, headers=basic(username, pat), timeout=10)
    assert dl.status_code == 200
    assert dl.content == tarball


# ---- pypi (twine) ----


def test_pypi_upload_simple_index_download(base_url, pkg_user):
    username, pat, _ = pkg_user
    pkg = f"mypkg_{uuid.uuid4().hex[:8]}"
    files = {"content": (f"{pkg}-1.0.0-py3-none-any.whl", b"wheel-bytes")}
    data = {"name": pkg, "version": "1.0.0"}
    r = requests.post(
        f"{base_url}/api/packages/pypi/{username}/",
        data=data, files=files, headers=basic(username, pat), timeout=10,
    )
    assert r.status_code == 200

    index = requests.get(
        f"{base_url}/api/packages/pypi/{username}/simple/", headers=basic(username, pat), timeout=10
    )
    assert pkg in index.text

    proj = requests.get(
        f"{base_url}/api/packages/pypi/{username}/simple/{pkg}/", headers=basic(username, pat), timeout=10
    )
    assert f"{pkg}-1.0.0-py3-none-any.whl" in proj.text
    dl_url = proj.text.split('href="')[1].split('"')[0]
    dl = requests.get(dl_url, headers=basic(username, pat), timeout=10)
    assert dl.content == b"wheel-bytes"


# ---- go (GOPROXY) ----


def _zip_bytes(files: dict[str, bytes]) -> bytes:
    import zipfile

    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as zf:
        for name, content in files.items():
            zf.writestr(name, content)
    return buf.getvalue()


def test_go_goproxy(base_url, pkg_user):
    username, pat, _ = pkg_user
    module = f"example.com/{username}/lib"
    zip_data = _zip_bytes({f"lib@v1.0.0/go.mod": b"module example.com/lib\n"})
    h = basic(username, pat)
    r = requests.put(
        f"{base_url}/api/packages/go/{username}/{module}/@v/v1.0.0.zip",
        data=zip_data, headers=h, timeout=10,
    )
    assert r.status_code == 201

    lst = requests.get(f"{base_url}/api/packages/go/{username}/{module}/@v/list", headers=h, timeout=10)
    assert "v1.0.0" in lst.text
    info = requests.get(f"{base_url}/api/packages/go/{username}/{module}/@v/v1.0.0.info", headers=h, timeout=10).json()
    assert info["Version"] == "v1.0.0"
    mod = requests.get(f"{base_url}/api/packages/go/{username}/{module}/@v/v1.0.0.mod", headers=h, timeout=10)
    assert mod.text.startswith("module ")
    got = requests.get(f"{base_url}/api/packages/go/{username}/{module}/@v/v1.0.0.zip", headers=h, timeout=10)
    assert got.content == zip_data


# ---- cargo ----


def test_cargo_publish_index_download(base_url, pkg_user):
    username, pat, _ = pkg_user
    crate = f"mycrate_{uuid.uuid4().hex[:8]}"
    h = basic(username, pat)
    base = f"{base_url}/api/packages/cargo/{username}"

    cfg = requests.get(f"{base}/config.json", headers=h, timeout=10).json()
    assert "/dl" in cfg["dl"]

    crate_bytes = b"crate-tarball"
    meta = f'{{"name":"{crate}","vers":"0.1.0","deps":[],"cksum":"","features":{{}}}}\n'.encode()
    r = requests.put(f"{base}/api/v1/crates/new", data=meta + crate_bytes, headers=h, timeout=10)
    assert r.status_code == 201

    idx = requests.get(f"{base}/index/{crate[:2]}/{crate[2:4]}/{crate}", headers=h, timeout=10)
    assert idx.status_code == 200
    entries = [json.loads(line) for line in idx.text.splitlines() if line.strip()]
    entry = entries[0]
    assert entry["vers"] == "0.1.0"
    dl = requests.get(f"{cfg['dl']}/{crate}/0.1.0/{crate}-0.1.0.crate", headers=h, timeout=10)
    assert dl.content == crate_bytes


# ---- rubygems ----


def test_gem_push_and_download(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    gem_name = f"mygem-{uuid.uuid4().hex[:8]}"

    # 构造最小 .gem：未压缩 tar，内含 metadata.gz（YAML）
    spec = f"--- !ruby/object:Gem::Specification\nname: {gem_name}\nversion: !ruby/object:Gem::Version\n  version: 1.0.0\n".encode()
    gzbuf = io.BytesIO()
    with gzip.GzipFile(fileobj=gzbuf, mode="wb") as gz:
        gz.write(spec)
    tbuf = io.BytesIO()
    with tarfile.open(fileobj=tbuf, mode="w") as tf:
        data = gzbuf.getvalue()
        ti = tarfile.TarInfo("metadata.gz")
        ti.size = len(data)
        tf.addfile(ti, io.BytesIO(data))
    gem_bytes = tbuf.getvalue()

    r = requests.post(f"{base_url}/api/packages/rubygems/{username}/api/v1/gems", data=gem_bytes, headers=h, timeout=10)
    assert r.status_code == 201

    dl = requests.get(f"{base_url}/api/packages/rubygems/{username}/gems/{gem_name}-1.0.0.gem", headers=h, timeout=10)
    assert dl.status_code == 200
    assert dl.content == gem_bytes


# ---- composer ----


def test_composer_upload_packages_json(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    vendor = username
    name = f"lib{uuid.uuid4().hex[:6]}"
    zip_data = b"composer-zip"
    r = requests.put(
        f"{base_url}/api/packages/composer/{username}/{vendor}/{name}?version=1.0.0",
        data=zip_data, headers=h, timeout=10,
    )
    assert r.status_code == 201

    pkgs = requests.get(f"{base_url}/api/packages/composer/{username}/packages.json", headers=h, timeout=10).json()
    entry = pkgs["packages"].get(f"{vendor}/{name}", {}).get("1.0.0")
    assert entry is not None
    dl = requests.get(entry["dist"]["url"], headers=h, timeout=10)
    assert dl.content == zip_data


# ---- maven ----


def test_maven_put_get(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    base = f"{base_url}/api/packages/maven/{username}"
    group = f"com/example/{username}"
    art = "mylib"

    pom = b"<project/>"
    r = requests.put(f"{base}/{group}/{art}/1.0.0/{art}-1.0.0.pom", data=pom, headers=h, timeout=10)
    assert r.status_code == 201

    got = requests.get(f"{base}/{group}/{art}/1.0.0/{art}-1.0.0.pom", headers=h, timeout=10)
    assert got.status_code == 200
    assert got.content == pom

    meta = b"<metadata/>"
    requests.put(f"{base}/{group}/{art}/maven-metadata.xml", data=meta, headers=h, timeout=10)
    got_meta = requests.get(f"{base}/{group}/{art}/maven-metadata.xml", headers=h, timeout=10)
    assert got_meta.content == meta


# ---- listing / delete ----


def test_package_list_and_delete(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    pkg = f"del-{uuid.uuid4().hex[:8]}"
    body = {
        "name": pkg,
        "versions": {"1.0.0": {}},
        "_attachments": {f"{pkg}-1.0.0.tgz": {"length": 3, "data": base64.b64encode(b"abc").decode()}},
    }
    requests.put(f"{base_url}/api/packages/npm/{username}/{pkg}", json=body, headers=h, timeout=10)

    listed = requests.get(f"{base_url}/api/packages/{username}/npm", headers=h, timeout=10).json()
    assert any(p["name"] == pkg for p in listed)

    r = requests.delete(f"{base_url}/api/packages/npm/{username}/{pkg}", headers=h, timeout=10)
    assert r.status_code == 204
    r2 = requests.get(f"{base_url}/api/packages/npm/{username}/{pkg}", headers=h, timeout=10)
    assert r2.status_code == 404


# ---- 扩展：integrity / dist-tags / yank / maven metadata / pypi json / audit / counts ----


def test_npm_integrity_dist_tags(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    pkg = f"itg-{uuid.uuid4().hex[:8]}"
    tarball = b"tarball-bytes"
    body = {
        "name": pkg,
        "versions": {"1.0.0": {}, "2.0.0": {}},
        "dist-tags": {"beta": "2.0.0"},
        "_attachments": {
            f"{pkg}-1.0.0.tgz": {"data": base64.b64encode(tarball).decode()},
        },
    }
    requests.put(f"{base_url}/api/packages/npm/{username}/{pkg}", json=body, headers=h, timeout=10)

    meta = requests.get(f"{base_url}/api/packages/npm/{username}/{pkg}", headers=h, timeout=10).json()
    v1 = meta["versions"]["1.0.0"]
    assert v1["dist"]["shasum"]  # sha1
    assert v1["dist"]["integrity"].startswith("sha512-")
    assert meta["dist-tags"].get("beta") == "2.0.0"

    r = requests.put(
        f"{base_url}/api/packages/npm/{username}/{pkg}/-/package/dist-tags/stable",
        json="1.0.0", headers=h, timeout=10,
    )
    assert r.status_code == 200
    tags = requests.get(
        f"{base_url}/api/packages/npm/{username}/{pkg}/-/package/dist-tags", headers=h, timeout=10
    ).json()
    assert tags["stable"] == "1.0.0"


def test_cargo_yank_unyank(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    crate = f"yankcrate{uuid.uuid4().hex[:6]}"
    meta = f'{{"name":"{crate}","vers":"0.2.0","deps":[],"cksum":"","features":{{}}}}\n'.encode()
    base = f"{base_url}/api/packages/cargo/{username}"
    requests.put(f"{base}/api/v1/crates/new", data=meta + b"crate", headers=h, timeout=10)

    r = requests.delete(f"{base}/api/v1/crates/{crate}/0.2.0/yank", headers=h, timeout=10)
    assert r.status_code == 200
    idx = requests.get(f"{base}/index/{crate[:2]}/{crate[2:4]}/{crate}", headers=h, timeout=10)
    assert '"yanked":true' in idx.text

    requests.put(f"{base}/api/v1/crates/{crate}/0.2.0/yank", headers=h, timeout=10)
    idx = requests.get(f"{base}/index/{crate[:2]}/{crate[2:4]}/{crate}", headers=h, timeout=10)
    assert '"yanked":false' in idx.text


def test_maven_auto_metadata_and_sha1(base_url, pkg_user):
    import hashlib
    username, pat, _ = pkg_user
    h = basic(username, pat)
    base = f"{base_url}/api/packages/maven/{username}"
    grp = f"com/ex{uuid.uuid4().hex[:4]}"
    for v in ("1.0.0", "1.1.0"):
        requests.put(f"{base}/{grp}/auto/{v}/auto-{v}.jar", data=f"jar-{v}".encode(), headers=h, timeout=10)

    meta = requests.get(f"{base}/{grp}/auto/maven-metadata.xml", headers=h, timeout=10)
    assert meta.status_code == 200
    assert "<version>1.1.0</version>" in meta.text
    assert "<latest>1.1.0</latest>" in meta.text

    sha1 = requests.get(f"{base}/{grp}/auto/1.1.0/auto-1.1.0.jar.sha1", headers=h, timeout=10)
    assert sha1.text.strip() == hashlib.sha1(b"jar-1.1.0").hexdigest()


def test_pypi_json_api(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    pkg = f"jsonpkg{uuid.uuid4().hex[:6]}"
    requests.post(
        f"{base_url}/api/packages/pypi/{username}/",
        data={"name": pkg, "version": "0.1.0"},
        files={"content": (f"{pkg}-0.1.0.tar.gz", b"sdists")},
        headers=h, timeout=10,
    )
    js = requests.get(
        f"{base_url}/api/packages/pypi/{username}/pypi/{pkg}/json", headers=h, timeout=10
    ).json()
    assert js["info"]["name"] == pkg
    rel = js["releases"]["0.1.0"][0]
    assert rel["digests"]["sha256"]
    assert rel["size"] == len(b"sdists")


def test_download_counter_and_audit(base_url, pkg_user):
    username, pat, _ = pkg_user
    h = basic(username, pat)
    pkg = f"cnt-{uuid.uuid4().hex[:8]}"
    body = {
        "name": pkg,
        "versions": {"1.0.0": {}},
        "_attachments": {f"{pkg}-1.0.0.tgz": {"data": base64.b64encode(b"xyz").decode()}},
    }
    requests.put(f"{base_url}/api/packages/npm/{username}/{pkg}", json=body, headers=h, timeout=10)

    tar_url = f"{base_url}/api/packages/npm/{username}/{pkg}/-/{pkg}-1.0.0.tgz"
    requests.get(tar_url, headers=h, timeout=10)
    requests.get(tar_url, headers=h, timeout=10)

    listed = requests.get(f"{base_url}/api/packages/{username}/npm", headers=h, timeout=10).json()
    row = next(p for p in listed if p["name"] == pkg)
    assert row["downloads"] >= 2

    audit = requests.get(f"{base_url}/api/packages/{username}/audit", headers=h, timeout=10).json()
    assert "publish" in [a["action"] for a in audit]


# ---- P1 改进项黑盒验证 ----


def test_npm_republish_conflict_epublishconflict(base_url, pkg_user):
    """npm 重复 publish 同版本 → 409 + EPUBLISHCONFLICT。"""
    username, pat, _ = pkg_user
    h = basic(username, pat)
    pkg = f"dup-{uuid.uuid4().hex[:8]}"
    body = {
        "name": pkg,
        "versions": {"1.0.0": {}},
        "_attachments": {f"{pkg}-1.0.0.tgz": {"data": base64.b64encode(b"abc").decode()}},
    }
    url = f"{base_url}/api/packages/npm/{username}/{pkg}"
    assert requests.put(url, json=body, headers=h, timeout=10).status_code == 201
    r = requests.put(url, json=body, headers=h, timeout=10)
    assert r.status_code == 409
    assert "EPUBLISHCONFLICT" in r.text


def test_npm_packument_maintainers_readme(base_url, pkg_user):
    """packument 含 maintainers / readme 字段（老工具兼容）。"""
    username, pat, _ = pkg_user
    h = basic(username, pat)
    pkg = f"mnt-{uuid.uuid4().hex[:8]}"
    body = {
        "name": pkg,
        "versions": {"1.0.0": {}},
        "_attachments": {f"{pkg}-1.0.0.tgz": {"data": base64.b64encode(b"abc").decode()}},
    }
    requests.put(f"{base_url}/api/packages/npm/{username}/{pkg}", json=body, headers=h, timeout=10)
    meta = requests.get(f"{base_url}/api/packages/npm/{username}/{pkg}", headers=h, timeout=10).json()
    assert meta["maintainers"] and meta["maintainers"][0]["name"]
    assert "readme" in meta


def test_head_requests_for_mirrors(base_url, pkg_user):
    """镜像探测类工具的 HEAD 请求：npm tarball / pypi simple / composer / cargo 均可。"""
    username, pat, _ = pkg_user
    h = basic(username, pat)
    # npm
    pkg = f"hd-{uuid.uuid4().hex[:8]}"
    body = {
        "name": pkg,
        "versions": {"1.0.0": {}},
        "_attachments": {f"{pkg}-1.0.0.tgz": {"data": base64.b64encode(b"abc").decode()}},
    }
    requests.put(f"{base_url}/api/packages/npm/{username}/{pkg}", json=body, headers=h, timeout=10)
    r = requests.head(f"{base_url}/api/packages/npm/{username}/{pkg}/-/{pkg}-1.0.0.tgz", headers=h, timeout=10)
    assert r.status_code == 200
    assert r.headers["Content-Type"] == "application/gzip"
    # pypi simple
    requests.post(
        f"{base_url}/api/packages/pypi/{username}/",
        data={"name": pkg, "version": "1.0.0"},
        files={"content": (f"{pkg}-1.0.0-py3-none-any.whl", b"w")},
        headers=h, timeout=10,
    )
    assert requests.head(f"{base_url}/api/packages/pypi/{username}/simple/", headers=h, timeout=10).status_code == 200
    assert requests.head(f"{base_url}/api/packages/pypi/{username}/simple/{pkg}/", headers=h, timeout=10).status_code == 200
    # composer packages.json / cargo index
    assert requests.head(f"{base_url}/api/packages/composer/{username}/packages.json", headers=h, timeout=10).status_code == 200
    crate = "headcrate"
    requests.put(
        f"{base_url}/api/packages/cargo/{username}/api/v1/crates/new",
        data=f'{{"name":"{crate}","vers":"0.1.0","cksum":""}}\n'.encode() + b"c",
        headers=h, timeout=10,
    )
    assert requests.head(
        f"{base_url}/api/packages/cargo/{username}/index/{crate[:2]}/{crate[2:4]}/{crate}",
        headers=h, timeout=10,
    ).status_code == 200


def test_maven_semver_sorting(base_url, pkg_user):
    """maven latest/release 按版本段比较：1.10.0 > 1.9.0，1.0.0 > 1.0.0-rc1。"""
    username, pat, _ = pkg_user
    h = basic(username, pat)
    base = f"{base_url}/api/packages/maven/{username}"
    grp = f"com/sem{uuid.uuid4().hex[:4]}"
    for v in ("1.9.0", "1.10.0", "1.0.0-rc1", "1.11.0-SNAPSHOT"):
        requests.put(f"{base}/{grp}/sv/{v}/sv-{v}.jar", data=f"jar-{v}".encode(), headers=h, timeout=10)
    meta = requests.get(f"{base}/{grp}/sv/maven-metadata.xml", headers=h, timeout=10)
    assert "<latest>1.11.0-SNAPSHOT</latest>" in meta.text
    assert "<release>1.10.0</release>" in meta.text  # release 排除 SNAPSHOT
    versions = meta.text.split("<versions>")[1].split("</versions>")[0]
    assert versions.index("1.9.0") < versions.index("1.10.0")
    assert versions.index("1.0.0-rc1") < versions.index("1.9.0")


def test_maven_snapshot_protocol(base_url, pkg_user):
    """SNAPSHOT：版本级 metadata 自动生成 + 时间戳文件名解析 + 非时间戳兜底。"""
    username, pat, _ = pkg_user
    h = basic(username, pat)
    base = f"{base_url}/api/packages/maven/{username}"
    grp = f"com/snap{uuid.uuid4().hex[:4]}"
    art = "mylib"
    # 上传时间戳制品（maven deploy 实际写法）
    ts_jar = f"{art}-1.0.0-20260908.101010-1.jar"
    requests.put(f"{base}/{grp}/{art}/1.0.0-SNAPSHOT/{ts_jar}", data=b"jar-ts", headers=h, timeout=10)
    requests.put(f"{base}/{grp}/{art}/1.0.0-SNAPSHOT/{art}-1.0.0-20260908.101010-1.pom", data=b"<pom/>", headers=h, timeout=10)

    # 版本级 metadata 自动生成
    meta = requests.get(f"{base}/{grp}/{art}/1.0.0-SNAPSHOT/maven-metadata.xml", headers=h, timeout=10)
    assert meta.status_code == 200
    assert "<timestamp>20260908.101010</timestamp>" in meta.text
    assert "<buildNumber>1</buildNumber>" in meta.text
    assert f"<value>1.0.0-20260908.101010-1</value>" in meta.text

    # 按时间戳文件名下载
    r = requests.get(f"{base}/{grp}/{art}/1.0.0-SNAPSHOT/{ts_jar}", headers=h, timeout=10)
    assert r.status_code == 200 and r.content == b"jar-ts"

    # 非时间戳文件名兜底（老客户端）
    r = requests.get(f"{base}/{grp}/{art}/1.0.0-SNAPSHOT/{art}-1.0.0-SNAPSHOT.jar", headers=h, timeout=10)
    assert r.status_code == 200 and r.content == b"jar-ts"

    # 校验和跟随时间戳文件
    import hashlib
    sha1 = requests.get(f"{base}/{grp}/{art}/1.0.0-SNAPSHOT/{ts_jar}.sha1", headers=h, timeout=10)
    assert sha1.text.strip() == hashlib.sha1(b"jar-ts").hexdigest()


def _make_crate(crate: str, cargo_toml: str) -> bytes:
    """构造 .crate（gzip tar，内含 {crate}-0.1.0/Cargo.toml）。"""
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w") as tf:
        data = cargo_toml.encode()
        ti = tarfile.TarInfo(f"{crate}-0.1.0/Cargo.toml")
        ti.size = len(data)
        tf.addfile(ti, io.BytesIO(data))
    gzbuf = io.BytesIO()
    with gzip.GzipFile(fileobj=gzbuf, mode="wb") as gz:
        gz.write(buf.getvalue())
    return gzbuf.getvalue()


def test_cargo_index_deps_features(base_url, pkg_user):
    """cargo 索引 deps/features 从 .crate 内 Cargo.toml 填充。"""
    username, pat, _ = pkg_user
    h = basic(username, pat)
    crate = f"dpcrate{uuid.uuid4().hex[:6]}"
    dep_crate = "serde"
    toml = f"""[package]
name = "{crate}"
version = "0.1.0"

[dependencies]
{dep_crate} = {{ version = "1.0", features = ["derive"], optional = true, default-features = false }}
rand = "0.8"

[dev-dependencies]
tempfile = "3.0"

[features]
default = ["std"]
std = []
"""
    crate_bytes = _make_crate(crate, toml)
    base = f"{base_url}/api/packages/cargo/{username}"
    cksum = __import__("hashlib").sha256(crate_bytes).hexdigest()
    meta = f'{{"name":"{crate}","vers":"0.1.0","deps":[],"cksum":"{cksum}","features":{{}}}}\n'.encode()
    r = requests.put(f"{base}/api/v1/crates/new", data=meta + crate_bytes, headers=h, timeout=10)
    assert r.status_code == 201

    idx = requests.get(f"{base}/index/{crate[:2]}/{crate[2:4]}/{crate}", headers=h, timeout=10)
    entry = json.loads(idx.text.splitlines()[0])
    deps = {d["name"]: d for d in entry["deps"]}
    assert deps[dep_crate]["req"] == "1.0"
    assert deps[dep_crate]["features"] == ["derive"]
    assert deps[dep_crate]["optional"] is True
    assert deps[dep_crate]["default_features"] is False
    assert deps["rand"]["req"] == "0.8"
    assert "kind" not in deps["rand"]
    assert deps["tempfile"]["kind"] == "dev"
    assert entry["features"] == {"default": ["std"], "std": []}


def test_go_upload_streaming_large_module(base_url, pkg_user):
    """go module zip 流式落盘：较大 zip 上传/下载内容一致。"""
    username, pat, _ = pkg_user
    h = basic(username, pat)
    module = f"example.com/{username}/biglib"
    import os as _os
    zip_data = _zip_bytes({
        "biglib@v1.2.0/go.mod": b"module example.com/biglib\n",
        "biglib@v1.2.0/big.bin": _os.urandom(8 << 20),  # 8MB
    })
    r = requests.put(
        f"{base_url}/api/packages/go/{username}/{module}/@v/v1.2.0.zip",
        data=zip_data, headers=h, timeout=60,
    )
    assert r.status_code == 201
    got = requests.get(f"{base_url}/api/packages/go/{username}/{module}/@v/v1.2.0.zip", headers=h, timeout=60)
    assert got.content == zip_data


def _make_wheel(pkg: str, requires_python: str) -> bytes:
    import zipfile
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as zf:
        zf.writestr(
            f"{pkg}-1.0.0.dist-info/METADATA",
            f"Metadata-Version: 2.1\nName: {pkg}\nVersion: 1.0.0\nRequires-Python: {requires_python}\n",
        )
    return buf.getvalue()


def test_pypi_simple_data_attributes(base_url, pkg_user):
    """pypi simple 链接带 data-requires-python 与 data-hashes（hash-checking 模式）。"""
    username, pat, _ = pkg_user
    h = basic(username, pat)
    pkg = f"rp_{uuid.uuid4().hex[:8]}"
    wheel = _make_wheel(pkg, ">=3.9")
    import hashlib
    requests.post(
        f"{base_url}/api/packages/pypi/{username}/",
        data={"name": pkg, "version": "1.0.0"},
        files={"content": (f"{pkg}-1.0.0-py3-none-any.whl", wheel)},
        headers=h, timeout=10,
    )
    proj = requests.get(f"{base_url}/api/packages/pypi/{username}/simple/{pkg}/", headers=h, timeout=10)
    assert 'data-requires-python="&gt;=3.9"' in proj.text
    assert f'data-hashes="sha256={hashlib.sha256(wheel).hexdigest()}"' in proj.text
